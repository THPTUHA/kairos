package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/THPTUHA/kairos/pkg/cbqueue"
	"github.com/THPTUHA/kairos/pkg/helper"
	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/THPTUHA/kairos/server/httpserver/events"
	"github.com/THPTUHA/kairos/server/messaging"
	"github.com/THPTUHA/kairos/server/storage"
	"github.com/THPTUHA/kairos/server/storage/models"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/sirupsen/logrus"
)

var (
	ErrDeliverTimeout = errors.New("deliver timeout")
	ErrStopWorker     = errors.New("worker: stopped")
	ErrEmptyTask      = errors.New("woker: empty task")
)

// event cmd
const (
	DeliverBorkerCmd = iota
	DeleteWorkerCmd
	ReceiveDeliverTaskCmd
	ReceiveDeliverBrokerCmd
)

const (
	KairosPoint = iota
	ClientPoint
	ChannelPoint
	BrokerPoint
	TaskPoint
)

type Event struct {
	cmd     int
	cmdtask *workflow.CmdReplyTask
}

type DeliverErr struct {
	err      error
	From     *Point                 `json:"from"`
	Receiver *Point                 `json:"receiver"`
	Msg      *workflow.CmdTask      `json:"msg"`
	Reply    *workflow.CmdReplyTask `json:"reply"`
}
type taskDeliver struct {
	client string
	status int
}

type brokerDeliver struct {
	client string
	status int
}
type Worker struct {
	ID             int64
	eventChan      chan *Event
	deliverErrCh   chan *DeliverErr
	deliverCmd     int64
	mu             sync.RWMutex
	status         int
	conf           *WorkerConfig
	workflow       *workflow.Workflow
	requestsMu     sync.RWMutex
	cacheMu        sync.RWMutex
	requests       map[int64]request
	responsesIDs   map[int64]bool
	points         map[string]*Point
	taskDelivers   map[int64][]*taskDeliver
	brokerDelivers map[int64][]*brokerDeliver

	clientsActive     []*Point
	CBQueue           *cbqueue.CBQueue
	closeCh           chan struct{}
	Sched             *Scheduler
	flowMap           map[string]string
	fm                sync.RWMutex
	workerNatsOptions *workerNatsOptions
	natsConn          *nats.Conn
}

func NewWorker(workerID int64, workflow *workflow.Workflow, conf *WorkerConfig, Sched *Scheduler) *Worker {
	return &Worker{
		ID:                workerID,
		eventChan:         make(chan *Event),
		deliverErrCh:      make(chan *DeliverErr),
		status:            events.WorkerCreating,
		workflow:          workflow,
		workerNatsOptions: defaultNatsOptions(),
		conf:              conf,
		requests:          map[int64]request{},
		responsesIDs:      make(map[int64]bool),
		points:            make(map[string]*Point),
		taskDelivers:      make(map[int64][]*taskDeliver),
		brokerDelivers:    make(map[int64][]*brokerDeliver),
		flowMap:           make(map[string]string),
		Sched:             Sched,
		deliverCmd:        helper.GetTimeNow(),
		closeCh:           make(chan struct{}),
	}
}

func (w *Worker) IsRunning() bool {
	w.mu.Lock()
	status := w.status
	defer w.mu.Unlock()
	return status == workflow.Running
}

func (w *Worker) updateStatusDeliverTask(taskID int64, d *taskDeliver) {
	fmt.Printf("[updateStatusDeliverTask] taskid=%d d=%+v \n", taskID, d)
	w.mu.Lock()
	v, ok := w.taskDelivers[taskID]
	if !ok {
		v = make([]*taskDeliver, 0)
		v = append(v, d)
		w.taskDelivers[taskID] = v
	} else {
		exist := false
		for idx, e := range v {
			if fmt.Sprintf("%s%s", e.client, workflow.SubClient) == d.client {
				v[idx] = d
				w.taskDelivers[taskID] = v
				exist = true
			}
		}
		if !exist {
			v = append(v, d)
			w.taskDelivers[taskID] = v
		}
	}

	defer w.mu.Unlock()
}

func (w *Worker) updateStatusDeliverBroker(brokerID int64, d *brokerDeliver) {
	fmt.Printf("[updateStatusDeliverBroker] broker=%d d=%+v \n", brokerID, d)
	w.mu.Lock()
	v, ok := w.brokerDelivers[brokerID]
	if !ok {
		v = make([]*brokerDeliver, 0)
		v = append(v, d)
		w.brokerDelivers[brokerID] = v
	} else {
		exist := false
		for idx, e := range v {
			if fmt.Sprintf("%s%s", e.client, workflow.SubClient) == d.client {
				v[idx] = d
				w.brokerDelivers[brokerID] = v
				exist = true
			}
		}
		if !exist {
			v = append(v, d)
			w.brokerDelivers[brokerID] = v
		}
	}

	defer w.mu.Unlock()
}

func (w *Worker) allDelivered() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	for k, s := range w.taskDelivers {
		for _, e := range s {
			if e.status != workflow.SuccessSetTask {
				fmt.Printf(" TTTTTTask taskid =%d name=%s not deliver %d\n", k, e.client, e.status)
				return false
			}
		}
	}

	for k, s := range w.brokerDelivers {
		for _, e := range s {
			if e.status != workflow.SuccessSetBroker {
				fmt.Printf(" BBBBroker broker =%d name=%s not deliver %d\n", k, e.client, e.status)
				return false
			}
		}

	}
	return true
}

func (w *Worker) Destroy() bool {
	w.conf.Logger.Warnf("Starting destroy workflow = %s id = %d ......", w.workflow.Name, w.workflow.ID)
	var wait sync.WaitGroup
	w.setWfStatus(workflow.Destroying)
	success := true
	from := w.points["kairos"]

	for k, v := range w.points {
		if strings.HasSuffix(k, workflow.SubClient) {
			w.clientsActive = append(w.clientsActive, v)
		}
	}

	for _, ca := range w.clientsActive {
		var cmd = workflow.CmdTask{
			Cmd:        workflow.RequestDestroyWf,
			WorkflowID: w.workflow.ID,
		}
		p := ca

		wait.Add(1)
		go w.deliverAsync(from, p, &cmd, func(crt *workflow.CmdReplyTask, err error) {
			fmt.Println("Reciver delete", crt.Status, crt.RunOn)
			defer wait.Done()
			if err != nil {
				success = false
				w.wrapDeliverErr(from, p, &cmd, crt, fmt.Errorf("Client %s can't clear workflow, msg = %s ", p.Name, crt.Message))
				return
			}

			if crt.Status == workflow.SuccessDestroyWorkflow {
				w.mu.Lock()
				p.Status = PointDestroy
				w.logWorkflow(
					fmt.Sprintf("Client %s clear workflow ", p.Name),
					models.WFRecordScuccess,
					nil,
				)
				for _, p := range w.clientsActive {
					fmt.Printf("%+v\n", p)
				}
				w.conf.Logger.Warnf("Destroy success workflow = %s id = %d on client = %s ", w.workflow.Name, w.workflow.ID, p.Name)
				for _, p := range w.clientsActive {
					if p.Status != PointDestroy {
						w.mu.Unlock()
						return
					}
				}
				w.mu.Unlock()
				if _, err := storage.DropWorkflow(fmt.Sprint(w.workflow.UserID), fmt.Sprint(w.workflow.ID)); err != nil {
					w.conf.Logger.Errorf("can't destroy workflow id = %d, err=%s", w.workflow.ID, err.Error())
					success = false
				} else {
					mwf := workflow.MonitorWorkflow{
						Cmd:          workflow.DestroyWorkflow,
						UserID:       w.workflow.UserID,
						WorkflowID:   w.workflow.ID,
						WorkflowName: w.workflow.Name,
						Data: `{
							"status": "` + fmt.Sprint(workflow.Destroyed) + `"
						}`,
					}
					js, _ := json.Marshal(mwf)

					if err = w.natsConn.Publish(messaging.MONITOR_WORKFLOW, js); err != nil {
						w.conf.Logger.Errorf("can't show destroy workflow id = %d err=%s", w.workflow.ID, err.Error())
					}
					close(w.closeCh)
					close(w.eventChan)
				}

			} else {
				success = false
				w.wrapDeliverErr(from, p, &cmd, crt, fmt.Errorf("Client %s can't clear workflow, msg = %s ", p.Name, crt.Message))
			}
		})
	}
	wait.Wait()
	return success
}

const (
	PointNormal = iota
	PointDestroy
)

type Point struct {
	ID     int64  `json:"id"`
	Type   int    `json:"type"`
	Name   string `json:"name"`
	Status int    `json:"status"`
}

func (p *Point) getChannel() string {
	if p.Type == ClientPoint {
		return fmt.Sprintf("kairosdeamon-%d", p.ID)
	}
	if p.Type == ChannelPoint {
		return fmt.Sprintf("%s-%d", p.Name, p.ID)
	}
	return ""
}

type WorkerConfig struct {
	MaxAttempDeliverTask int
	TimeoutRetryDeliver  time.Duration
	DeliverTimeout       time.Duration
	DeliverDelay         time.Duration
	ReconnectWait        time.Duration
	MaxReconnects        int
	Logger               *logrus.Entry
}

type request struct {
	cb func(*workflow.CmdReplyTask, error)
}
type workerNatsOptions struct {
	url           string
	name          string
	reconnectWait time.Duration
	maxReconnects int
}

func defaultNatsOptions() *workerNatsOptions {
	optsDef := workerNatsOptions{
		name:          "worker",
		reconnectWait: 2 * time.Second,
		maxReconnects: 10,
	}
	return &optsDef

}

func optNatsWorker(o *workerNatsOptions) []nats.Option {
	opts := make([]nats.Option, 0)
	opts = append(opts, nats.Name(o.name))
	opts = append(opts, nats.MaxReconnects(o.maxReconnects))
	opts = append(opts, nats.ReconnectWait(o.reconnectWait))
	return opts
}

func (w *Worker) setPoints() {
	w.points["kairos"] = &Point{
		ID:   -1,
		Type: KairosPoint,
		Name: "kairos",
	}

	w.workflow.Brokers.Range(func(key string, broker *workflow.Broker) error {
		w.points[workflow.GetBrokerName(key)] = &Point{
			ID:   broker.ID,
			Type: BrokerPoint,
			Name: broker.Name,
		}
		return nil
	})

	w.workflow.Tasks.Range(func(key string, task *workflow.Task) error {
		w.points[workflow.GetTaskName(key)] = &Point{
			ID:   task.ID,
			Type: TaskPoint,
			Name: task.Name,
		}
		return nil
	})

	for _, c := range w.workflow.Channels {
		w.points[workflow.GetChannelName(c.Name)] = &Point{
			ID:   c.ID,
			Type: ChannelPoint,
			Name: c.Name,
		}
	}

	for _, c := range w.workflow.Clients {
		w.points[workflow.GetClientName(c.Name)] = &Point{
			ID:   c.ID,
			Type: ClientPoint,
			Name: c.Name,
		}
	}
}

func (w *Worker) run() {
	defer w.natsConn.Close()
	log.Debug().Msg(fmt.Sprintf("start worker workflow running name = %s, namespace = %s", w.workflow.Name, w.workflow.Namespace))
	err := w.pushAllCmdTask(workflow.SetTaskCmd)
	if err != nil {
		w.conf.Logger.WithField("worker", "run").Error(err)
	}
	err = w.deliverBrokers(workflow.SetBrokerCmd)
	if err != nil {
		w.conf.Logger.WithField("worker", "run").Error(err)
	}
	<-w.closeCh
	w.conf.Logger.Debugf("finish worker workflow running name = %s, namespace = %s", w.workflow.Name, w.workflow.Namespace)
}

func (w *Worker) retryDeliver() {
	for e := range w.deliverErrCh {
		w.conf.Logger.WithFields(logrus.Fields{
			"from":     e.From.Name,
			"receiver": e.Receiver.Name,
		})

		ne := e
		msg := ne.Msg
		w.CBQueue.Push(func(_ time.Duration) {
			reply := e.Reply
			if msg.Task != nil {
				reply.TaskID = msg.Task.ID
			}
			reply.WorkflowID = w.workflow.ID
			if reply.Message == "" && e.err != nil {
				reply.Message = e.err.Error()
			}

			fmt.Printf("[ERRR DELIVER ----] CMD= %d  task= %+v broker= %+v reciever= %+v\n", msg.Cmd, msg.Task, msg.Broker, e.Receiver)
			switch msg.Cmd {
			case workflow.SetTaskCmd:
				reply.Cmd = workflow.ReplySetTaskCmd
				reply.Status = workflow.FaultSetTask
				w.logWorkflow(
					fmt.Sprintf("Task %s can't setup on client %s, msg = %s ",
						msg.Task.Name,
						msg.From,
						reply.Message),
					models.WFRecordFault,
					ne,
				)
			case workflow.TriggerStartTaskCmd:
				reply.Cmd = workflow.ReplyStartTaskCmd
				reply.Status = workflow.FaultTriggerTask
				w.logWorkflow(
					fmt.Sprintf("Task %s can't trigger on client %s, msg = %s ",
						msg.Task.Name,
						msg.From,
						reply.Message),
					models.WFRecordFault,
					ne,
				)
			case workflow.InputTaskCmd:
				reply.Cmd = workflow.ReplyInputTaskCmd
				reply.Status = workflow.FaultInputTask
				w.logWorkflow(
					fmt.Sprintf("Result of task %s can't deliver to %s, msg = %s ",
						msg.Task.Name,
						e.Receiver.Name,
						reply.Message),
					models.WFRecordFault,
					ne,
				)
			case workflow.SetBrokerCmd:
				reply.Cmd = workflow.ReplySetBrokerCmd
				reply.Status = workflow.FaultSetBroker
				w.logWorkflow(
					fmt.Sprintf("Broker %s can't setup on client %s, msg = %s ",
						msg.Broker.Name,
						e.Receiver.Name,
						reply.Message),
					models.WFRecordFault,
					ne,
				)
			case workflow.RequestDestroyWf:
				reply.Cmd = workflow.ReplyDestroyWf
				reply.Status = workflow.FaultDestroyWorkflow
				w.logWorkflow(
					reply.Message,
					models.WFRecordFault,
					ne,
				)
			}
			reply.DeliverID = msg.DeliverID
			m, _ := json.Marshal(reply)
			w.logMessageFlowReply(e.From, e.Receiver, m, time.Now().UnixMilli()-msg.SendAt, "")
		})
	}
}

func (w *Worker) wrapDeliverErr(from *Point, receiver *Point, cmd *workflow.CmdTask, reply *workflow.CmdReplyTask, err error) {
	w.deliverErrCh <- &DeliverErr{
		From:     from,
		Receiver: receiver,
		Reply:    reply,
		Msg:      cmd,
		err:      err,
	}
}

func (w *Worker) setWfStatus(status int) {
	fmt.Printf("[SET STATUS WORKFLOW] %d user = %d\n", status, w.workflow.UserID)
	w.workflow.Status = status
	w.status = status
	msg := workflow.MonitorWorkflow{
		Cmd:        workflow.SetStatusWorkflow,
		UserID:     w.workflow.UserID,
		WorkflowID: w.workflow.ID,
		Data: `{
			"status": "` + fmt.Sprint(w.workflow.Status) + `"
		}`,
	}
	js, _ := json.Marshal(msg)
	err := w.natsConn.Publish(messaging.MONITOR_WORKFLOW, js)
	if err != nil {
		w.conf.Logger.Error(err)
	}
}

func (w *Worker) Run() error {
	w.conf.Logger.Debug(fmt.Sprintf("worker %d run ", w.ID))
	natConn, err := nats.Connect(nats.DefaultURL, optNatsWorker(defaultNatsOptions())...)
	if err != nil {
		return err
	}
	w.natsConn = natConn
	err = w.reciveMessage()
	if err != nil {
		return err
	}
	err = w.reciveMessageSync()
	if err != nil {
		return err
	}

	w.status = workflow.Running
	go w.setWfStatus(workflow.Delivering)
	go w.retryDeliver()

	w.setPoints()
	fmt.Println("----set points")
	for k, p := range w.points {
		fmt.Println(k, p)
	}
	fmt.Println("----end points")
	go w.run()

	w.CBQueue = &cbqueue.CBQueue{}
	w.CBQueue.Cond = sync.NewCond(&w.CBQueue.Mu)
	go w.CBQueue.Dispatch()

	for {
		select {
		case e, ok := <-w.eventChan:
			if !ok {
				return err
			}
			switch e.cmd {
			case DeleteWorkerCmd:
				log.Debug().Msg(fmt.Sprintf("delete worker id =%d, workflow running name = %s, namespace = %s", w.ID, w.workflow.Name, w.workflow.Namespace))

				w.mu.Lock()
				w.CBQueue.Close()
				w.CBQueue = nil
				w.mu.Unlock()

				close(w.closeCh)
				return nil
			case ReceiveDeliverTaskCmd:
				w.updateStatusDeliverTask(e.cmdtask.TaskID, &taskDeliver{
					status: e.cmdtask.Status,
					client: e.cmdtask.RunOn,
				})
				if w.allDelivered() {
					fmt.Printf("[ALL TASK DELIVER SUCCESS]\n")
					go w.setWfStatus(workflow.Running)
					w.logWorkflow(
						fmt.Sprintf("Success setup workflow"),
						models.WFRecordScuccess,
						nil,
					)
					w.pushAllCmdTask(workflow.TriggerStartTaskCmd)
				}
			case ReceiveDeliverBrokerCmd:
				w.updateStatusDeliverBroker(e.cmdtask.BrokerID, &brokerDeliver{
					status: e.cmdtask.Status,
					client: e.cmdtask.RunOn,
				})
				if w.allDelivered() {
					fmt.Printf("[ALL TASK DELIVER SUCCESS]\n")
					go w.setWfStatus(workflow.Running)
					w.logWorkflow(
						fmt.Sprintf("Success setup workflow"),
						models.WFRecordScuccess,
						nil,
					)
					w.pushAllCmdTask(workflow.TriggerStartTaskCmd)
				}
			}
		}
	}
}

func (w *Worker) reciveMessageSync() error {
	_, err := w.natsConn.Subscribe(fmt.Sprintf("%s-%d", messaging.REQUEST_WORKER_SYNC, w.workflow.UserID), func(msg *nats.Msg) {

		var reply workflow.CmdReplyTask
		err := json.Unmarshal(msg.Data, &reply)
		if err != nil {
			w.conf.Logger.Error(err)
		}
		fmt.Println("RUN HERE SYNC-----", w.workflow.ID)
		fmt.Printf("REPLY--%+v\n", reply)
		if (err != nil || reply.WorkflowID != w.workflow.ID) &&
			!strings.HasSuffix(reply.RunOn, workflow.SubChannel) {
			return
		}

		reply.Status = workflow.SuccessReceiveOutputTaskCmd
		fmt.Printf("[SYNC BROKER] ============= %+v\n", reply)
		var req workflow.CmdTask
		req.Cmd = workflow.InputTaskCmd
		req.DeliverID = w.nextCmdID()
		reply.WorkflowID = w.workflow.ID

		req.From = reply.RunOn
		resultCh := make(chan []byte)
		req.WorkflowID = reply.WorkflowID
		w.computeMsg(&reply, &req, resultCh)
		result := <-resultCh
		msg.Respond([]byte(result))
	})
	return err
}

func (w *Worker) reciveMessage() error {
	_, err := w.natsConn.Subscribe(fmt.Sprint(w.workflow.UserID), func(msg *nats.Msg) {
		var reply workflow.CmdReplyTask
		err := json.Unmarshal(msg.Data, &reply)
		if (err != nil || reply.WorkflowID != w.workflow.ID) &&
			strings.HasSuffix(reply.RunOn, workflow.SubChannel) &&
			strings.HasSuffix(reply.RunOn, workflow.SubClient) {
			return
		}

		fmt.Println("[ NATS REPLY !!]")
		if reply.DeliverID == DeliverBorkerCmd {
			reply.Status = workflow.SuccessReceiveOutputTaskCmd
			reply.WorkflowID = w.workflow.ID
			fmt.Printf("[ REPLY MSG SEND TO BROKER] ============= %+v\n", reply)
			var req = workflow.CmdTask{
				Cmd:        workflow.InputTaskCmd,
				From:       reply.RunOn,
				WorkflowID: reply.WorkflowID,
				Part:       w.getPart(reply.Part),
			}

			if reply.RunCount != 0 {
				w.requestsMu.RLock()
				ok := w.responsesIDs[reply.RunCount]
				fmt.Println("RUNCOUNT---", reply.RunCount, ok)
				if !ok {
					w.requestsMu.RUnlock()
					return
				}
				delete(w.responsesIDs, reply.RunCount)
				w.requestsMu.RUnlock()
			}

			w.computeMsg(&reply, &req, nil)
		} else {
			reply.WorkflowID = w.workflow.ID

			w.requestsMu.RLock()
			req, ok := w.requests[reply.DeliverID]
			w.requestsMu.RUnlock()
			w.removeRequest(reply.DeliverID)
			if ok {
				if req.cb != nil {
					req.cb(&reply, err)
				}
			}
		}
	})
	return err
}

func (w *Worker) computeMsg(reply *workflow.CmdReplyTask, req *workflow.CmdTask, resultCh chan []byte) {
	// TODO TEST
	// if !w.IsRunning() {
	// 	w.conf.Logger.Error("worker not running")
	// 	return
	// }
	w.workflow.Brokers.Range(func(key string, broker *workflow.Broker) error {
		from := Point{
			ID:   broker.ID,
			Type: BrokerPoint,
			Name: broker.Name,
		}

		if reply.WorkflowID != w.workflow.ID {
			return fmt.Errorf("invalid message")
		}

		if (broker.IsListen(reply.TaskName) || broker.IsListen(reply.RunOn)) && len(broker.Clients) == 0 {
			bg := w.getBrokerGroup()
			if reply.Group == "" {
				reply.Group = w.getGroup()
				reply.Start = true
				fmt.Println("RUN HERE VC", reply.Group)
			}
			req.Group = reply.Group
			if reply.Part == "" && strings.HasSuffix(reply.RunOn, workflow.SubChannel) {
				reply.Part = w.getPart("")
			}

			m, _ := json.Marshal(reply)
			w.conf.Logger.Debugf("Reply runon=%s sendAt=%d Now=%d", reply.RunOn, reply.SendAt, helper.GetTimeNow())
			sender := w.points[reply.RunOn]
			if sender == nil {
				return fmt.Errorf("can't find sender")
			}

			w.CBQueue.Push(func(duration time.Duration) {
				w.logMessageFlowReply(sender, &from, m, reply.SendAt, bg)
			})

			req.Task = &workflow.Task{
				ID:         reply.TaskID,
				WorkflowID: reply.WorkflowID,
				Name:       reply.TaskName,
			}

			if strings.HasSuffix(reply.RunOn, workflow.SubTask) {
				if reply.Result == nil {
					err := fmt.Errorf("reply from %s empty result", reply.RunOn)
					w.conf.Logger.Error(err)
					return err
				}
				w.conf.Logger.Infof("Reciver message from %s value= %+v", reply.RunOn, reply.Result)
			}

			if strings.HasSuffix(reply.RunOn, workflow.SubChannel) {
				if reply.Content == nil {
					err := fmt.Errorf("reply from %s empty content", reply.RunOn)
					w.conf.Logger.Error(err)
					return err
				}
				w.conf.Logger.Infof("Reciver message from %s value= %+v", reply.RunOn, reply.Content)
			}

			if len(broker.Flows.Endpoints) > 0 {
				for _, endpoint := range broker.Flows.Endpoints {
					receiver := w.points[endpoint]
					if receiver.Type == TaskPoint {
						w.workflow.Tasks.Range(func(_ string, task *workflow.Task) error {
							if receiver.ID == task.ID {
								for _, c := range task.Clients {
									receiver = w.points[workflow.GetClientName(c)]
									go w.deliverInputTaskCmd(&from, receiver, req, reply, nil, bg)
								}
							}
							return nil
						})
					} else {
						go w.deliverInputTaskCmd(&from, receiver, req, reply, nil, bg)
					}

				}
			} else {
				if !broker.Queue {
					broker.DynamicVars = make(map[string]*workflow.CmdReplyTask)
					fmt.Println("RUN TO EMPTY QUEUE BROKER")
					if strings.HasSuffix(reply.RunOn, workflow.SubChannel) {
						w.cacheMu.RLock()
						c := strings.ToLower(reply.RunOn)
						broker.DynamicVars[c] = reply
						w.cacheMu.RUnlock()
					}

					if strings.HasSuffix(reply.RunOn, workflow.SubClient) {
						w.cacheMu.RLock()
						c := strings.ToLower(reply.RunOn)
						broker.DynamicVars[c] = reply
						w.cacheMu.RUnlock()

						if w.workflow.Tasks.Exists(reply.TaskName) {
							w.cacheMu.RLock()
							c := strings.ToLower(workflow.GetTaskName(reply.TaskName))
							broker.DynamicVars[c] = reply
							w.cacheMu.RUnlock()
						}
					}

				} else {
					setBrokerQueue(strings.ToLower(reply.RunOn), reply, reply.WorkflowID)
					if strings.HasSuffix(reply.RunOn, workflow.SubClient) && reply.TaskName != "" {
						setBrokerQueue(strings.ToLower(workflow.GetTaskName(reply.TaskName)), reply, reply.WorkflowID)
					}

					vars := make(map[string]bool)
					for k := range broker.Template.ListenVars {
						k = strings.TrimPrefix(strings.ToLower(workflow.GetRootDefaultVar(k)), ".")
						vars[k] = true
					}

					bqs, err := storage.GetKVQueues(vars, w.workflow.ID)
					if err != nil {
						fmt.Printf("[ GetKVQueues ERR ] %+v\n", err)
						return nil
					}

					for _, bg := range bqs {
						var r workflow.CmdReplyTask
						err := json.Unmarshal([]byte(bg.Value), &r)
						if err != nil {
							fmt.Printf("[ Unmarshal To CmdReplyTask ERR ] %+v\n", err)
							// Log Err
							return nil
						}
						broker.DynamicVars[bg.Key] = &r
					}
				}
				replies := make(map[string]workflow.ReplyData)

				for k, v := range broker.DynamicVars {
					replies[k] = workflow.ReplyData{}
					if v.Content != nil {
						replies[k]["content"] = v.Content
					}

					if v.Result != nil {
						replies[k]["result"] = v.Result
					}
					fmt.Println("DEBG DYNAMIC VAR", k)
				}

				trun := broker.Template.NewRutime(replies)
				exeOutput := trun.Execute()
				delivers := exeOutput.DeliverFlows
				w.CBQueue.Push(func(duration time.Duration) {
					// input, err := json.Marshal(replies)
					// if err != nil {
					// 	w.conf.Logger.Error(err)
					// 	return
					// }
					// output, err := json.Marshal(exeOutput)
					// if err != nil {
					// 	w.conf.Logger.Error(err)
					// 	return
					// }
					// var status int
					// if exeOutput.Tracking.Err == "" {
					// 	status = workflow.BrokerExecuteSuccess
					// } else {
					// 	status = workflow.BrokerExecuteFault
					// }
					// w.saveBrokerRecord(broker.ID, string(input), string(output), status)
				})

				// TODO Get Vars
				// if err == nil {
				// 	w.cacheMu.Lock()
				// 	for k := range broker.DynamicVars {
				// 		broker.DynamicVars[k] = nil
				// 	}
				// 	ids := make([]int64, 0)
				// 	for _, bq := range bqs {
				// 		ids = append(ids, bq.ID)
				// 	}
				// 	err := storage.UsedKVQueue(ids)
				// 	if err != nil {
				// 		fmt.Printf("[Used KV QUeue] %s\n", err)
				// 	}
				// 	w.cacheMu.Unlock()
				// } else if err == workflow.ErrorVariableNotReady {
				// 	// TODO save queue
				// 	w.cacheMu.Lock()
				// 	w.cacheMu.Unlock()
				// }
				for _, v := range delivers {
					fmt.Printf("DELIVER Reciever= %s  Msg= %+v\n", v.Reciever, v.Msg)
				}
				fmt.Printf("[WORKER DELIVERS] %+v\n", delivers)
				w.handleDeliver(&from, delivers, reply, req, trun, resultCh, exeOutput, bg)
			}
		}
		return nil
	})
}
func (w *Worker) handleDeliver(from *Point, delivers []*workflow.DeliverFlow, reply *workflow.CmdReplyTask, req *workflow.CmdTask, trun *workflow.TemplateRuntime, resultCh chan []byte, exeOutput *workflow.ExecOutput, bg string) {
	hasSync := false
	for _, path := range delivers {
		fmt.Printf("[ DEBUG OUTPUT TASK] %+v\n", path)
		w.conf.Logger.Infof("Path: from= %s to receiver= %s msg= %+v", from.Name, path.Reciever, path.Msg)
		receiver := w.points[path.Reciever]
		if receiver == nil {
			w.conf.Logger.Error(fmt.Errorf("can't find receiver %s", path.Reciever))
			return
		}
		reqDeliver := *req
		reqDeliver.Part = w.getPart(reply.Part)
		reqDeliver.Parent = reply.Part
		if strings.HasSuffix(path.Reciever, workflow.SubChannel) {
			reqDeliver.Message = path.Msg
		} else if strings.HasSuffix(path.Reciever, workflow.SubTask) {
			input, ok := path.Msg.(string)
			if !ok {
				b, _ := json.Marshal(path.Msg)
				input = string(b)
			}
			reqDeliver.Task = &workflow.Task{
				ID:         receiver.ID,
				WorkflowID: reply.WorkflowID,
				Name:       receiver.Name,
				Input:      input,
			}
		} else if strings.HasSuffix(path.Reciever, workflow.SubClient) {

		}

		fmt.Printf("NEW deliver %+v task = %+v\n", reqDeliver, reqDeliver.Task)

		w.conf.Logger.Debugf("request %+v", reqDeliver)
		w.conf.Logger.Debugf("reply %+v", reply)
		if path.Send == workflow.SENDU {
			reqDeliver.DeliverID = w.nextCmdID()
			reqDeliver.RunCount = reqDeliver.DeliverID
			w.requestsMu.RLock()
			w.responsesIDs[reqDeliver.DeliverID] = true
			w.requestsMu.RUnlock()
			fmt.Println("SENDU---", reqDeliver.DeliverID)
		}

		if path.Send == workflow.SENDSYNC {
			hasSync = true
			reqDeliver.Cmd = workflow.RequestTaskRunSyncCmd
			if receiver.Type == TaskPoint {
				w.workflow.Tasks.Range(func(_ string, task *workflow.Task) error {
					if receiver.ID == task.ID {
						for _, c := range task.Clients {
							cr := w.points[workflow.GetClientName(c)]
							go w.deliverRequestRunTaskSync(from, cr, reply, &reqDeliver, trun, resultCh, exeOutput, bg)
						}
					}
					return nil
				})
			} else {
				go w.deliverInputTaskCmd(from, receiver, &reqDeliver, reply, exeOutput, bg)
			}
		} else {
			if receiver.Type == TaskPoint {
				w.workflow.Tasks.Range(func(_ string, task *workflow.Task) error {
					if receiver.ID == task.ID {
						for _, c := range task.Clients {
							cr := w.points[workflow.GetClientName(c)]
							go w.deliverInputTaskCmd(from, cr, &reqDeliver, reply, exeOutput, bg)
						}
					}
					return nil
				})
			} else {
				go w.deliverInputTaskCmd(from, receiver, &reqDeliver, reply, exeOutput, bg)
			}
		}
		if resultCh != nil && !hasSync {
			resultCh <- []byte("")
		}
	}
}

type ReplyTask struct {
	Result *workflow.Result `json:"result"`
}

type ReplyChannel struct {
	Content *workflow.Content `json:"content"`
}

func (w *Worker) deliverRequestRunTaskSync(from *Point, receiver *Point, reply *workflow.CmdReplyTask, req *workflow.CmdTask, trun *workflow.TemplateRuntime, resultCh chan []byte, exeOutput *workflow.ExecOutput, bg string) {
	err := w.deliverAsync(from, receiver, req, func(crt *workflow.CmdReplyTask, err error) {
		fmt.Printf("[CALLBACK deliverRequestRunTaskCmd ] result = %+v err = %+v\n", crt.Result, err)

		if err != nil {
			w.wrapDeliverErr(from, receiver, req, crt, err)
			trun.ExpInput = append(trun.ExpInput, err.Error())
		} else {
			if receiver.Type == TaskPoint || receiver.Type == ClientPoint {
				trun.ExpInput = append(trun.ExpInput, ReplyTask{
					Result: crt.Result,
				})
			} else {
				trun.ExpInput = append(trun.ExpInput, ReplyChannel{
					Content: reply.Content,
				})
			}
		}

		exeOutput := trun.Execute()
		delivers := exeOutput.DeliverFlows
		track := exeOutput.Tracking
		result := exeOutput.Result
		fmt.Printf("OUTPUT %+v  tracking %+v\n", exeOutput, track)
		if track.Err != "" && resultCh != nil {
			resultCh <- []byte(track.Err)
		}
		if len(delivers) > 0 {
			w.handleDeliver(from, delivers, reply, req, trun, resultCh, exeOutput, bg)
		} else {
			if resultCh != nil {
				resultCh <- result
			}
		}

		// m, _ := json.Marshal(crt)

		// w.CBQueue.Push(func(duration time.Duration) {
		// 	w.logMessageFlowReply(receiver, from, m, helper.GetTimeNow()-req.SendAt)
		// })
	})
	if err != nil {
		var crt workflow.CmdReplyTask
		w.wrapDeliverErr(from, receiver, req, &crt, err)
	}
}

func (w *Worker) deliverInputTaskCmd(from, receiver *Point, req *workflow.CmdTask, reply *workflow.CmdReplyTask, exeOutput *workflow.ExecOutput, bg string) {
	fmt.Printf("[BROKER %+v DELIVER OUTPUT TASK BY ENDPOINT] \n", receiver)
	w.CBQueue.Push(func(duration time.Duration) {
		if reply.Cmd == workflow.ReplyOutputTaskCmd {
			w.saveTaskRecord(reply)
		}
		var traking string
		if exeOutput != nil {
			eo, _ := json.Marshal(exeOutput)
			traking = string(eo)
		}
		w.logMessageFlowRequest(from, receiver, req, duration.Milliseconds(), traking, bg)
	})

	err := w.deliverAsync(from, receiver, req, func(crt *workflow.CmdReplyTask, err error) {
		fmt.Printf("[CALLBACK DELIVER InputTaskCmd ] res = %+v err = %+v\n", req, err)
		if err != nil {
			w.wrapDeliverErr(from, receiver, req, crt, err)
			return
		}
		m, _ := json.Marshal(crt)

		w.CBQueue.Push(func(duration time.Duration) {
			w.logMessageFlowReply(receiver, from, m, crt.SendAt, "")
		})
	})
	if err != nil {
		var crt workflow.CmdReplyTask
		w.wrapDeliverErr(from, receiver, req, &crt, err)
	}
}

func (w *Worker) addRequest(id int64, cb func(*workflow.CmdReplyTask, error)) {
	w.requestsMu.Lock()
	defer w.requestsMu.Unlock()
	w.requests[id] = request{cb}
}

func (w *Worker) nextCmdID() int64 {
	return atomic.AddInt64(&w.deliverCmd, 1)
}

func (w *Worker) getPart(replyPart string) string {
	w.fm.RLock()
	defer w.fm.RUnlock()
	if replyPart == "" {
		id := w.nextCmdID()
		p := fmt.Sprintf("part-%d-%d", w.workflow.ID, id)
		return p
	}
	reqPart, ok := w.flowMap[replyPart]
	if !ok {
		id := w.nextCmdID()
		p := fmt.Sprintf("part-%d-%d", w.workflow.ID, id)
		w.flowMap[replyPart] = p
		return p
	}
	return reqPart
}

func (w *Worker) getGroup() string {
	id := w.nextCmdID()
	return fmt.Sprintf("kairos-%d-%d", w.workflow.ID, id)
}

func (w *Worker) getBrokerGroup() string {
	id := w.nextCmdID()
	return fmt.Sprintf("broker-%d-%d", w.workflow.ID, id)
}

func (w *Worker) deliver(cmd *workflow.CmdTask) error {
	d, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	err = w.natsConn.Publish(messaging.DELIVERER_TASK, d)
	return err
}

func (w *Worker) removeRequest(id int64) {
	w.requestsMu.Lock()
	defer w.requestsMu.Unlock()
	delete(w.requests, id)
}

func (w *Worker) deliverAsync(from *Point, receiver *Point, cmd *workflow.CmdTask, cb func(*workflow.CmdReplyTask, error)) error {
	cmd.Channel = receiver.getChannel()

	cmd.Status = workflow.PendingDeliver
	cmd.SendAt = time.Now().UnixMilli()
	fmt.Println("DELIVERID BEFORE-------------------- ", cmd.DeliverID)
	if cmd.DeliverID == 0 {
		cmd.DeliverID = w.nextCmdID()
	}
	fmt.Println("DELIVERID -------------------- ", cmd.DeliverID)
	w.addRequest(cmd.DeliverID, cb)

	err := w.deliver(cmd)
	fmt.Println("________________________")
	fmt.Printf("%+v\n task= %+v \n", cmd, cmd.Task)
	fmt.Println("________________________")
	if err != nil {
		w.conf.Logger.Error(err)
		return err
	}
	go func() {
		defer w.removeRequest(cmd.DeliverID)
		select {
		case <-time.After(w.conf.DeliverTimeout):
			w.requestsMu.RLock()
			req, ok := w.requests[cmd.DeliverID]
			w.requestsMu.RUnlock()
			if !ok {
				return
			}
			var reply workflow.CmdReplyTask
			req.cb(&reply, ErrDeliverTimeout)
		case <-w.closeCh:
			w.requestsMu.RLock()
			req, ok := w.requests[cmd.DeliverID]
			w.requestsMu.RUnlock()
			if !ok {
				return
			}
			var reply workflow.CmdReplyTask
			req.cb(&reply, ErrStopWorker)
		}
	}()
	return err
}

func (w *Worker) deliverTaskToClient(from *Point, receiver *Point, msg *workflow.CmdTask) error {
	msg.Part = w.getPart("")
	w.CBQueue.Push(func(duration time.Duration) {
		w.logMessageFlowRequest(from, receiver, msg, duration.Milliseconds(), "", "")
	})
	err := w.deliverAsync(from, receiver, msg, func(r *workflow.CmdReplyTask, err error) {
		w.checkCmdReply(from, receiver, msg, r, err, false)
	})
	if err != nil {
		w.conf.Logger.Error(err)
	}
	return err
}

func (w *Worker) checkCmdReply(from *Point, receiver *Point, msg *workflow.CmdTask, r *workflow.CmdReplyTask, err error, ignore bool) error {
	if err != nil {
		w.conf.Logger.Error(err)
		if !ignore {
			w.wrapDeliverErr(from, receiver, msg, r, err)
		}
		return err
	}
	switch r.Cmd {
	case workflow.ReplySetTaskCmd:
		fmt.Printf("[CALLBACK DELIVER Reply SetTaskCmd] REPLY= %+v\n", r)
		w.CBQueue.Push(func(duration time.Duration) {
			m, _ := json.Marshal(r)
			w.logMessageFlowReply(receiver, from, m, msg.SendAt, "")
		})

		w.eventChan <- &Event{
			cmd:     ReceiveDeliverTaskCmd,
			cmdtask: r,
		}
	case workflow.ReplyStartTaskCmd:
		fmt.Printf("[CALLBACK DELIVER Reply StartTaskCmd] REPLY= %+v\n", r)
		m, _ := json.Marshal(r)
		w.CBQueue.Push(func(duration time.Duration) {
			w.logMessageFlowReply(receiver, from, m, msg.SendAt, "")
		})
	case workflow.ReplySetBrokerCmd:
		fmt.Printf("[CALLBACK DELIVER Reply SetBrokerCmd] REPLY= %+v\n", r)
		m, err := json.Marshal(r)
		if err != nil {
			w.conf.Logger.Error(err)
			return err
		}

		if r.Status == workflow.FaultSetBroker {
			if !ignore {
				w.wrapDeliverErr(from, receiver, msg, r, nil)
			}
			return fmt.Errorf("set broker %s error", receiver.Name)
		}

		w.CBQueue.Push(func(duration time.Duration) {
			w.logMessageFlowReply(receiver, from, m, msg.SendAt, "")
		})
		w.eventChan <- &Event{
			cmd:     ReceiveDeliverBrokerCmd,
			cmdtask: r,
		}
	}
	return nil
}

func (w *Worker) deliverTask(from *Point, task *workflow.Task, cmd workflow.RequestActionTask) error {
	fmt.Printf(" deliverTask %+v\n", task.Clients)
	var err error
	for _, c := range task.Clients {
		w.updateStatusDeliverTask(task.ID, &taskDeliver{
			client: c,
			status: workflow.PendingDeliver,
		})
		cmd := workflow.CmdTask{
			Task:       task,
			Cmd:        cmd,
			WorkflowID: w.workflow.ID,
		}
		ch := w.points[workflow.GetClientName(c)].Name
		cmd.DeliverID = w.nextCmdID()
		cmd.Channel = ch

		clientID, err := strconv.ParseInt(task.Metadata[c], 10, 64)
		if err != nil {
			return err
		}
		receiver := Point{
			ID:   clientID,
			Type: ClientPoint,
			Name: c,
		}

		err = w.deliverTaskToClient(from, &receiver, &cmd)
	}
	return err
}

func (w *Worker) Recover() error {
	w.setWfStatus(workflow.Recovering)
	success := true
	var wg sync.WaitGroup
	for {
		wfrs, err := storage.GetWorkflowRecordsRecovery(w.workflow.ID, 10)
		fmt.Println("Recover number ---", len(wfrs))

		if err != nil {
			w.conf.Logger.Error(err)
			return err
		}
		if len(wfrs) == 0 {
			break
		}
		for _, e := range wfrs {
			if e.DeliverErr == "" {
				continue
			}
			wg.Add(1)
			var deliverErr DeliverErr
			fmt.Println("DELIVERRR---", e.DeliverErr)
			err = json.Unmarshal([]byte(e.DeliverErr), &deliverErr)
			if err != nil {
				w.conf.Logger.Error(err)
				wg.Done()
				return err
			}
			w.deliverAsync(deliverErr.From, deliverErr.Receiver, deliverErr.Msg, func(crt *workflow.CmdReplyTask, err error) {
				defer wg.Done()
				if err != nil {
					w.conf.Logger.Error(err)
					success = false
					return
				}
				err = w.checkCmdReply(deliverErr.From, deliverErr.Receiver, deliverErr.Msg, crt, err, true)
				if err != nil {
					success = false
					w.conf.Logger.Error(err)
					return
				}
				err = storage.SetWfRecovered(e.ID)
				if err != nil {
					success = false
					return
				}
			})
		}

		wg.Wait()
		if !success {
			w.logWorkflow(
				fmt.Sprintf("Fault recover workflow"),
				models.WFRecordFault,
				nil,
			)
			msg := workflow.MonitorWorkflow{
				Cmd:        workflow.RecoverWorkflow,
				UserID:     w.workflow.UserID,
				WorkflowID: w.workflow.ID,
				Data: `{
					"status": "` + fmt.Sprint(success) + `"
				}`,
			}
			js, _ := json.Marshal(msg)
			err := w.natsConn.Publish(messaging.MONITOR_WORKFLOW, js)
			if err != nil {
				w.conf.Logger.Error(err)
			}
			return nil
		}
	}

	if success {
		w.setWfStatus(workflow.Running)
		w.logWorkflow(
			fmt.Sprintf("Success recover workflow"),
			models.WFRecordScuccess,
			nil,
		)

		msg := workflow.MonitorWorkflow{
			Cmd:        workflow.RecoverWorkflow,
			UserID:     w.workflow.UserID,
			WorkflowID: w.workflow.ID,
			Data: `{
				"status": "` + fmt.Sprint(success) + `"
			}`,
		}
		js, _ := json.Marshal(msg)
		err := w.natsConn.Publish(messaging.MONITOR_WORKFLOW, js)

		if err != nil {
			w.conf.Logger.Error(err)
		}
	}

	return nil
}

func (w *Worker) deliverBrokers(cmd workflow.RequestActionTask) error {
	from := w.points["kairos"]
	err := w.workflow.Brokers.Range(func(key string, broker *workflow.Broker) error {
		for _, c := range broker.Clients {
			w.updateStatusDeliverBroker(broker.ID, &brokerDeliver{
				client: c,
				status: workflow.PendingDeliver,
			})
			cmd := workflow.CmdTask{
				Cmd:        cmd,
				Broker:     broker,
				WorkflowID: w.workflow.ID,
			}
			f, err := json.Marshal(cmd)

			err = json.Unmarshal(f, &cmd)
			if err != nil {
				w.conf.Logger.Error(err)
				return err
			}
			point := w.points[workflow.GetClientName(c)]
			cmd.Channel = point.Name

			receiver := point

			err = w.deliverTaskToClient(from, receiver, &cmd)
			if err != nil {
				w.conf.Logger.Error(err)
				return err
			}
		}
		return nil
	})

	return err
}

func (w *Worker) pushAllCmdTask(cmd workflow.RequestActionTask) error {
	from := w.points["kairos"]

	err := w.workflow.Tasks.Range(func(key string, task *workflow.Task) error {
		w.conf.Logger.Debug(fmt.Sprintf("task name = %s, executor: %s", key, task.Executor))
		switch task.Executor {
		case workflow.PubSubTask:
			arrs := make([]map[string]interface{}, 0)
			maps := make(map[string]interface{})
			if err := json.Unmarshal([]byte(task.Payload), &arrs); err != nil {
				err = json.Unmarshal([]byte(task.Payload), &maps)
				if err != nil {
					return err
				}
			}

			if len(maps) != 0 {
				arrs = append(arrs, maps)
			}

			task.Execute = func() {
				for _, m := range arrs {
					data := ""
					switch m["data"].(type) {
					case string:
						data = m["data"].(string)
					default:
						str, _ := json.Marshal(m["data"])
						data = string(str)
					}
					reciever := w.points[m["sub"].(string)]
					cmd := &workflow.CmdTask{
						Cmd:       workflow.InputTaskCmd,
						DeliverID: w.nextCmdID(),
						Task: &workflow.Task{
							ID:         task.ID,
							Name:       task.Name,
							Input:      data,
							WorkflowID: task.WorkflowID,
						},
						Channel: reciever.Name,
					}

					fmt.Printf("DELIVER PUB SUB TASK TO FROM %+v TO %+v \n", from, reciever)
					w.deliverAsync(from, reciever, cmd, func(crt *workflow.CmdReplyTask, err error) {
						//TODO
						fmt.Println("[RECIVER CMD REPLY TASK]", err)
					})
				}
			}
			w.Sched.AddTask(task)
		case workflow.HttpHookTask:

		default:
			fmt.Printf("[KAIROS PUSH TASK TO CLIENT] %s : task = %s, taskid= %d\n", task.Executor, task.Name, task.ID)

			err := w.deliverTask(from, task, cmd)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

type LogMessageReply struct {
	ID           int64                  `json:"id"`
	From         *Point                 `json:"from"`
	Receiver     *Point                 `json:"receiver"`
	Msg          *workflow.CmdReplyTask `json:"msg"`
	CreatedAt    int64                  `json:"created_at"`
	Flow         int                    `json:"flow"`
	ResponseSize int                    `json:"response_size"`
	ReceiveAt    int64                  `json:"receive_at"`
}

func (w *Worker) logMessageFlowReply(from *Point, receiver *Point, mrs []byte, receiverAt int64, broker_group string) {
	var mf models.MessageFlow
	var msg workflow.CmdReplyTask
	json.Unmarshal(mrs, &msg)
	var m []byte
	var err error
	if msg.Result != nil {
		m, err = json.Marshal(msg.Result)
		if err != nil {
			w.conf.Logger.Error(err)
			return
		}
		if msg.Result.StartedAt != 0 {
			mf.BeginPart = true
		} else if msg.Result.FinishedAt != 0 {
			mf.FinishPart = true
		}
	} else if msg.Content != nil {
		mf.BeginPart = true
		mf.FinishPart = true
		m, err = json.Marshal(msg.Content)
		if err != nil {
			w.conf.Logger.Error(err)
			return
		}
	} else {
		mf.BeginPart = msg.BeginPart
		mf.FinishPart = msg.FinishPart
	}
	mf.SenderID = from.ID
	mf.SenderType = from.Type
	mf.SenderName = from.Name
	mf.ReceiverID = receiver.ID
	mf.ReceiverType = receiver.Type
	mf.ReceiverName = receiver.Name
	mf.WorkflowID = msg.WorkflowID
	mf.CreatedAt = helper.GetTimeNow()
	mf.Message = string(m)
	mf.Status = msg.Status
	mf.Flow = models.RecieverFlow
	mf.DeliverID = msg.DeliverID
	mf.ResponseSize = len(mrs)
	mf.Cmd = int(msg.Cmd)
	mf.Start = msg.Start
	mf.Group = msg.Group
	mf.ReceiveAt = receiverAt
	mf.TaskName = msg.TaskName
	mf.TaskID = msg.TaskID
	mf.Part = msg.Part
	mf.Parent = msg.Parent
	mf.BrokerGroup = broker_group
	fmt.Printf("mf= %+v\n", mf)
	id, err := storage.LogMessageFlow(&mf)

	if err != nil {
		w.conf.Logger.Error(err)
	}
	mf.ID = id

	data, err := json.Marshal(&mf)
	if err != nil {
		w.conf.Logger.Error(err)
		return
	}
	mwf := workflow.MonitorWorkflow{
		Cmd:          workflow.LogMessageFlow,
		UserID:       w.workflow.UserID,
		WorkflowID:   w.workflow.ID,
		WorkflowName: w.workflow.Name,
		Data:         string(data),
	}
	js, _ := json.Marshal(mwf)

	err = w.natsConn.Publish(messaging.MONITOR_WORKFLOW, js)

	if err != nil {
		fmt.Println("[MONITOR WORKFLOW REPLY ERROR] ", err)
	}
}

type LogMessage struct {
	ID          int64             `json:"id"`
	From        *Point            `json:"from"`
	Receiver    *Point            `json:"receiver"`
	Msg         *workflow.CmdTask `json:"msg"`
	CreatedAt   int64             `json:"created_at"`
	Flow        int               `json:"flow"`
	RequestSize int               `json:"request_size"`
	SendAt      int64             `json:"send_at"`
}

func (w *Worker) logMessageFlowRequest(from *Point, receiver *Point, msg *workflow.CmdTask, sendAt int64, tracking string, broker_group string) {
	var m []byte
	if receiver.Type == ChannelPoint {
		fmt.Printf("FUCK CHANNEL %+v", msg)
		if s, ok := msg.Message.(string); ok {
			m = []byte(s)
		} else {
			m, _ = json.Marshal(msg.Message)
		}

	} else {
		m, _ = json.Marshal(msg.Task)
	}
	var mf models.MessageFlow
	mf.SenderID = from.ID
	mf.SenderType = from.Type
	mf.SenderName = from.Name
	mf.ReceiverID = receiver.ID
	mf.ReceiverType = receiver.Type
	mf.ReceiverName = receiver.Name
	mf.WorkflowID = msg.WorkflowID
	mf.CreatedAt = helper.GetTimeNow()
	mf.Message = string(m)
	mf.Status = msg.Status
	mf.Flow = models.DeliverFlow
	mf.DeliverID = msg.DeliverID
	mf.SendAt = sendAt
	mf.Cmd = int(msg.Cmd)
	mf.Group = msg.Group
	mf.TaskID = msg.Task.ID
	mf.TaskName = msg.Task.Name
	mf.Parent = msg.Parent
	mf.Part = msg.Part
	mf.BeginPart = true
	e, _ := json.Marshal(msg)
	mf.ResponseSize = len(e)
	mf.BrokerGroup = broker_group
	id, err := storage.LogMessageFlow(&mf)
	if err != nil {
		w.conf.Logger.Errorf(fmt.Sprint("[SAVE TASK REQUEST FLOW ERR]", err))
	}

	mf.ID = id

	data, err := json.Marshal(&mf)
	if err != nil {
		fmt.Println(" [MARSHAL LOGMESS ERR]", err)
		return
	}
	mwf := workflow.MonitorWorkflow{
		Cmd:          workflow.LogMessageFlow,
		UserID:       w.workflow.UserID,
		WorkflowID:   w.workflow.ID,
		WorkflowName: w.workflow.Name,
		Data:         string(data),
	}
	js, _ := json.Marshal(mwf)

	err = w.natsConn.Publish(messaging.MONITOR_WORKFLOW, js)

	if err != nil {
		fmt.Println("[MONITOR WORKFLOW ERROR] ", err)
	}

}

func setBrokerQueue(key string, value *workflow.CmdReplyTask, workflowID int64) {
	var bq models.BrokerQueue
	v, _ := json.Marshal(value)
	bq.WorkflowID = workflowID
	bq.Value = string(v)
	bq.CreatedAt = helper.GetTimeNow()
	bq.Key = key
	err := storage.SetBrokerQueue(&bq)
	fmt.Printf("[SAVE BROKER QUEUE] %+v lengh = %d \n", err, len(bq.Value))
}

func (w *Worker) saveTaskRecord(reply *workflow.CmdReplyTask) {
	point, ok := w.points[reply.RunOn]
	if !ok {
		w.conf.Logger.Errorf("RunOn %s is not found", reply.RunOn)
		return
	}
	var status int
	if reply.Result.Success {
		status = 1
	}
	if err := storage.AddTaskRecord(&models.TaskRecord{
		Status:     status,
		Output:     reply.Result.Output,
		TaskID:     reply.TaskID,
		ClientID:   point.ID,
		StartedAt:  reply.Result.StartedAt,
		FinishedAt: reply.Result.FinishedAt,
		CreatedAt:  helper.GetTimeNow(),
	}); err != nil {
		w.conf.Logger.Error(err)
	}
}

func (w *Worker) saveBrokerRecord(brokerID int64, input string, output string, status int) {
	if err := storage.InsertBrokerRecord(&models.BrokerRecord{
		Input:     input,
		Output:    output,
		BrokerID:  brokerID,
		CreatedAt: helper.GetTimeNow(),
		Status:    status,
	}); err != nil {
		w.conf.Logger.Error(err)
	}
}

func (w *Worker) logWorkflow(record string, status int, de *DeliverErr) {
	var data string
	var err error

	isRecovered := true
	if de != nil {
		js, err := json.Marshal(de)
		if err != nil {
			w.conf.Logger.Error(err)
			return
		}
		isRecovered = false
		data = string(js)
	}
	err = storage.InsertWorkflowRecord(&models.WorkflowRecords{
		CreatedAt:   helper.GetTimeNow(),
		WorkflowID:  w.workflow.ID,
		Record:      record,
		Status:      status,
		DeliverErr:  data,
		IsRecovered: isRecovered,
	})
	if err != nil {
		w.conf.Logger.Error(err)
	}
}
