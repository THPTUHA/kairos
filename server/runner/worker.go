package runner

import (
	"database/sql"
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
	"github.com/dop251/goja"
	"github.com/jpillora/backoff"
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

const (
	TriggerDeliver = iota
	TriggerScheduled
	TriggerFinish
	TriggerRunning
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
	fc             *workflow.FuncCall
	triggerCh      chan *workflow.Trigger

	clientsActive     []*Point
	CBQueue           *cbqueue.CBQueue
	closeCh           chan struct{}
	Sched             *Scheduler
	flowMap           map[string]string
	requestMap        map[string]string
	fm                sync.RWMutex
	backoffDeliver    backoffDeliver
	workerNatsOptions *workerNatsOptions
	natsConn          *nats.Conn
}

func NewWorker(workerID int64, wf *workflow.Workflow, conf *WorkerConfig, Sched *Scheduler) *Worker {
	return &Worker{
		ID:                workerID,
		eventChan:         make(chan *Event),
		deliverErrCh:      make(chan *DeliverErr),
		status:            events.WorkerCreating,
		workflow:          wf,
		workerNatsOptions: defaultNatsOptions(),
		triggerCh:         make(chan *workflow.Trigger),
		conf:              conf,
		requests:          map[int64]request{},
		responsesIDs:      make(map[int64]bool),
		points:            make(map[string]*Point),
		taskDelivers:      make(map[int64][]*taskDeliver),
		brokerDelivers:    make(map[int64][]*brokerDeliver),
		requestMap:        make(map[string]string),
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
			if fmt.Sprintf("%s%s", e.client, workflow.SubClient) == d.client || e.client == "kairos" {
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
				w.conf.Logger.Warnf("taskid =%d client=%s not deliver status=%d\n", k, e.client, e.status)
				return false
			}
		}
	}

	for k, s := range w.brokerDelivers {
		for _, e := range s {
			if e.status != workflow.SuccessSetBroker {
				w.conf.Logger.Warnf("broker =%d client=%s not deliver status=%d\n", k, e.client, e.status)
				return false
			}
		}

	}
	return true
}

func (w *Worker) Destroy() bool {
	w.conf.Logger.Warnf("Starting destroy workflow = %s id = %d ......", w.workflow.Name, w.workflow.ID)
	w.Sched.StopTask()
	w.Sched.StopBroker()
	w.Sched.StopChannel()
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
				w.wrapDeliverErr(from, p, &cmd, crt, fmt.Errorf("Client '%s' can't clear workflow, msg = %s ", p.Name, crt.Message))
				return
			}

			if crt.Status == workflow.SuccessDestroyWorkflow {
				w.mu.Lock()
				p.Status = PointDestroy
				w.logWorkflow(
					fmt.Sprintf("Client '%s' clear workflow ", p.Name),
					models.WFRecordScuccess,
					nil,
					-1,
				)

				w.conf.Logger.Warnf("Destroy success workflow = '%s' id = %d on client = '%s' ", w.workflow.Name, w.workflow.ID, p.Name)
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
				w.wrapDeliverErr(from, p, &cmd, crt, fmt.Errorf("Client '%s' can't clear workflow, msg = %s ", p.Name, crt.Message))
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
	NatURL               string
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

func (w *Worker) getNameType(t int) string {
	if t == BrokerPoint {
		return "broker"
	}
	if t == ChannelPoint {
		return "channel"
	}
	if t == TaskPoint {
		return "task"
	}
	return "server"
}

func (w *Worker) retryDeliver() {
	for e := range w.deliverErrCh {
		w.conf.Logger.WithFields(logrus.Fields{
			"from":     e.From.Name,
			"receiver": e.Receiver.Name,
		})

		ne := e
		msg := ne.Msg
		errStatus := -99
		w.CBQueue.Push(func(_ time.Duration) {
			reply := ne.Reply
			if msg.Task != nil {
				reply.TaskID = msg.Task.ID
			}
			reply.WorkflowID = w.workflow.ID
			if reply.Message == "" && e.err != nil {
				reply.Message = e.err.Error()
			}
			reply.Status = errStatus
			w.conf.Logger.Errorf("[ERRR DELIVER ----] CMD= %d  task= %+v broker= %+v receiver= %+v, message= %s deliver_id = %d \n", msg.Cmd, msg.Task, msg.Broker, e.Receiver, msg.Message, reply.DeliverID)
			switch msg.Cmd {
			case workflow.SetTaskCmd:
				reply.Cmd = workflow.ReplySetTaskCmd
				w.handleRetryDeliver(fmt.Sprintf("Task '%s' can't setup on client '%s', msg = %s ",
					msg.Task.Name,
					e.Receiver.Name,
					reply.Message), ne)

			case workflow.TriggerStartTaskCmd:
				reply.Cmd = workflow.ReplyStartTaskCmd
				w.handleRetryDeliver(fmt.Sprintf("Task '%s' can't trigger on client '%s', msg = %s ",
					msg.Task.Name,
					msg.From,
					reply.Message), ne)

			case workflow.InputTaskCmd:
				reply.Cmd = workflow.ReplyInputTaskCmd
				if msg.Task != nil {
					w.handleRetryDeliver(fmt.Sprintf("Can't deliver to '%s'(%s), msg = %s ",
						e.Receiver.Name,
						w.getNameType(e.Receiver.Type),
						reply.Message),
						ne,
					)
				}

			case workflow.SetBrokerCmd:
				reply.Cmd = workflow.ReplySetBrokerCmd
				w.handleRetryDeliver(fmt.Sprintf("Broker '%s' can't setup on client '%s', msg = %s ",
					msg.Broker.Name,
					e.Receiver.Name,
					reply.Message), ne)

			case workflow.RequestDestroyWf:
				reply.Cmd = workflow.ReplyDestroyWf
				if msg.Broker != nil {
					w.handleRetryDeliver(fmt.Sprintf("Broker '%s' can't setup on client '%s', msg = %s ",
						msg.Broker.Name,
						e.Receiver.Name,
						reply.Message), ne)
				}
			}
		})
	}
}

func (w *Worker) handleRetryDeliver(record string, e *DeliverErr) {
	msg := e.Msg
	reply := e.Reply
	if msg.Retry != nil {
		if msg.Attempt > 3 {
			w.conf.Logger.Errorf("Attempt deliver more than 3")
			return
		}
		if msg.Retry.ID == 0 {
			id, err := storage.GetWorkflowRecord(w.workflow.ID, msg.DeliverID)
			if err != nil {
				w.conf.Logger.Error(err)
				return
			}
			msg.Retry.ID = id
		}
		msg.Attempt++
		msg.Retry.Attempt++

		err := storage.SetWfRecovered(msg.Retry.ID, msg.Retry.Attempt, false)
		if err != nil {
			w.conf.Logger.Error(err)
			return
		}
		err = storage.UpdateAttemptDeliver(msg.DeliverID, msg.Retry.Attempt)
		if err != nil {
			w.conf.Logger.Error(err)
			return
		}
		d := w.backoffDeliver.timeBeforeNextAttempt(msg.Retry.Attempt)

		time.AfterFunc(d, func() {
			w.conf.Logger.Warnf("Run herer----- FROM=%+v RECEIVER=%+v", e.From, e.Receiver)
			err = w.deliverAsync(e.From, e.Receiver, msg, func(reply *workflow.CmdReplyTask, err error) {
				reply.Group = msg.Group
				reply.Parent = msg.Parent
				reply.Part = msg.Part
				if err != nil {
					w.wrapDeliverErr(e.From, e.Receiver, e.Msg, reply, err)
					return
				}
				if msg.Retry != nil {
					storage.SetWfRecovered(msg.Retry.ID, int(msg.Retry.Attempt), true)
				}
				m, _ := json.Marshal(reply)
				w.logMessageFlowReply(e.Receiver, e.From, m, helper.GetTimeNow(), fmt.Sprint(msg.DeliverID))
			})
		})
	} else {
		reply.Group = msg.Group
		reply.Parent = msg.Parent
		reply.Part = msg.Part
		m, _ := json.Marshal(reply)
		err := storage.UpdateAttemptDeliver(msg.DeliverID, msg.Attempt)
		if err != nil {
			w.conf.Logger.Error(err)
		}
		w.logMessageFlowReply(e.Receiver, e.From, m, time.Now().UnixMilli()-msg.SendAt, "")
		id, err := storage.GetWorkflowRecord(w.workflow.ID, msg.DeliverID)
		if err != nil {
			if err == sql.ErrNoRows {
				w.conf.Logger.Warnf("ATTEMPT FIRST")
				id := w.logWorkflow(
					record,
					models.WFRecordFault,
					e,
					msg.DeliverID,
				)
				msg.Retry = &workflow.Retry{
					ID: id,
				}
			} else {
				w.conf.Logger.Error(err)
				return
			}
		} else {
			msg.Retry = &workflow.Retry{
				ID: id,
			}
		}

		msg.Attempt++
		msg.Retry.Attempt = msg.Attempt
		msg.DeliverID = reply.DeliverID
		err = w.deliverAsync(e.From, e.Receiver, msg, func(reply *workflow.CmdReplyTask, err error) {
			if err != nil {
				w.wrapDeliverErr(e.From, e.Receiver, e.Msg, reply, err)
				return
			}
			if msg.Retry != nil {
				storage.SetWfRecovered(msg.Retry.ID, int(msg.Retry.Attempt)+1, true)
			}
			m, _ := json.Marshal(reply)
			w.logMessageFlowReply(e.Receiver, e.From, m, helper.GetTimeNow(), fmt.Sprint(msg.DeliverID))
		})
		if err != nil {
			w.conf.Logger.Error(err)
		}
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

func (w *Worker) InitFC() (*workflow.FuncCall, error) {
	funcs, err := storage.GetAllFunctionsByUserID(w.workflow.UserID)
	if err != nil {
		return nil, err
	}
	vm := goja.New()
	fc := &workflow.FuncCall{
		Call:  make(map[string]goja.Callable),
		Funcs: vm,
	}

	for _, f := range funcs {
		prog, err := goja.Compile("", f.Content, true)
		if err != nil {
			return nil, err
		}
		_, err = fc.Funcs.RunProgram(prog)
		if err != nil {
			return nil, err
		}
		v := fc.Funcs.Get(f.Name)
		if v == nil {
			return nil, fmt.Errorf("can't found function %s ", f.Name)
		}

		var fr goja.Callable
		err = fc.Funcs.ExportTo(v, &fr)
		if err != nil {
			return nil, err
		}
		fc.Call[f.Name] = fr
	}

	return fc, nil
}

func (w *Worker) handleTrigger(trigger workflow.Trigger) {
	if trigger.Client != "" {
		var cmd workflow.CmdTask
		cmd.Cmd = workflow.TriggerCmd
		cmd.Trigger = &trigger
		cmd.WorkflowID = w.workflow.ID
		cmd.DeliverID = w.nextCmdID()
		cmd.Attempt = 1
		from := w.points["kairos"]
		receiver := w.points[workflow.GetClientName(trigger.Client)]
		err := w.deliverAsync(from, receiver, &cmd, func(crt *workflow.CmdReplyTask, err error) {
			if err != nil {
				w.wrapDeliverErr(from, receiver, &cmd, crt, err)
				return
			}
			if cmd.Retry != nil {
				storage.SetWfRecovered(cmd.Retry.ID, int(cmd.Retry.Attempt)+1, true)
			}
			w.CBQueue.Push(func(duration time.Duration) {
				w.UpdateTrigger(trigger.ID, TriggerScheduled)
			})
		})
		if err != nil {
			w.conf.Logger.Error(err)
		}
	} else {
		if trigger.Type == "task" {
			if trigger.Action == "delete" {
				w.Sched.RemoveTask(trigger.ObjectID)
				return
			}
			var task *workflow.Task
			w.workflow.Tasks.Range(func(key string, value *workflow.Task) error {
				if value.ID == trigger.ObjectID {
					task = value
				}
				return nil
			})
			if task == nil {
				w.conf.Logger.Errorf("trigger task id = %d not found", trigger.ObjectID)
				return
			}

			w.Sched.AddTask(w, task, trigger.Input)
			w.UpdateTrigger(trigger.ID, TriggerScheduled)
		}

		if trigger.Type == "broker" {
			if trigger.Action == "delete" {
				w.Sched.RemoveBroker(trigger.ObjectID)
				return
			}
			var broker *workflow.Broker
			w.workflow.Brokers.Range(func(key string, value *workflow.Broker) error {
				if value.ID == trigger.ObjectID {
					broker = value
				}
				return nil
			})
			if broker == nil {
				w.conf.Logger.Errorf("trigger broker id = %d not found", trigger.ObjectID)
				return
			}

			w.Sched.AddBroker(w, *broker, &trigger)
			w.UpdateTrigger(trigger.ID, TriggerScheduled)
		}

		if trigger.Type == "channel" {
			if trigger.Action == "delete" {
				w.Sched.RemoveChannel(trigger.ObjectID)
				return
			}
			for _, c := range w.workflow.Channels {
				if c.ID == trigger.ObjectID {
					w.Sched.AddChannel(w, *c, &trigger)
					w.UpdateTrigger(trigger.ID, TriggerScheduled)
					return
				}
			}
			w.conf.Logger.Errorf("not found channel id=%d ", trigger.ObjectID)
		}
	}
}

func (w *Worker) handleTriggerSchedule(close chan bool) {
	w.workflow.Brokers.Range(func(key string, value *workflow.Broker) error {
		value.TriggerCh = w.triggerCh
		return nil
	})
	w.workflow.Tasks.Range(func(key string, value *workflow.Task) error {
		// value.TriggerCh = w.triggerCh
		return nil
	})
	for _, c := range w.workflow.Channels {
		c.TriggerCh = w.triggerCh
	}
	for trigger := range w.triggerCh {
		w.conf.Logger.Infof("Trigger object %+v", trigger)

		if trigger != nil {
			switch trigger.Type {
			case "task":
				var task *workflow.Task
				w.workflow.Tasks.Range(func(key string, value *workflow.Task) error {
					if value.ID == trigger.ObjectID {
						task = value
					}
					return nil
				})
				if task == nil {
					w.conf.Logger.Errorf("trigger task id = %d not found", trigger.ObjectID)
				} else {
					w.RunTask(task, trigger.Input)
					w.UpdateTrigger(trigger.ID, TriggerScheduled)
				}

			case "broker":
				var broker *workflow.Broker
				w.workflow.Brokers.Range(func(key string, value *workflow.Broker) error {
					if value.ID == trigger.ObjectID {
						broker = value
					}
					return nil
				})
				if broker == nil {
					w.conf.Logger.Errorf("trigger broker id = %d not found", trigger.ObjectID)
				} else {
					w.RunBroker(broker, trigger.Input, trigger.ID)
					w.UpdateTrigger(trigger.ID, TriggerScheduled)
				}
			case "channel":
				for _, c := range w.workflow.Channels {
					if c.ID == trigger.ObjectID {
						w.RunChannel(c, trigger.Input, trigger.ID)
						w.UpdateTrigger(trigger.ID, TriggerScheduled)
					}
				}
			}
		}
		w.conf.Logger.Warnf("Finish trigger")
	}

	<-close
	w.conf.Logger.Warn("Close trigger")
}

func (w *Worker) Run() error {
	w.conf.Logger.Debug(fmt.Sprintf("worker %d run ", w.ID))
	natConn, err := nats.Connect(w.conf.NatURL, optNatsWorker(defaultNatsOptions())...)
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
	fc, err := w.InitFC()
	if err != nil {
		return err
	}
	w.fc = fc
	w.workflow.Brokers.Range(func(key string, value *workflow.Broker) error {
		value.Template.SetFuncs(fc)
		return nil
	})

	w.status = workflow.Running

	if err != nil {
		w.conf.Logger.Error(err)
		return err
	}
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
	closeCh := make(chan bool)
	go w.CBQueue.Dispatch()
	go w.handleTriggerSchedule(closeCh)

	defer close(closeCh)

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
						fmt.Sprintf("Workflow setup success"),
						models.WFRecordScuccess,
						nil,
						-1,
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
						fmt.Sprintf("Workflow setup success"),
						models.WFRecordScuccess,
						nil,
						-1,
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
	if err != nil {
		return err
	}

	return nil
}

func (w *Worker) RunChannel(channel *workflow.Channel, input string, triggerID int64) {
	from := w.points["kairos"]
	p := w.points[workflow.GetChannelName(channel.Name)]
	req := workflow.CmdTask{
		Cmd:        workflow.InputTaskCmd,
		Message:    input,
		Group:      w.getGroup(),
		Part:       w.getPart("", p.ID, ClientPoint),
		Start:      true,
		Channel:    channel.Name,
		WorkflowID: w.workflow.ID,
		StartInput: input,
		Trigger: &workflow.Trigger{
			ID: triggerID,
		},
	}
	go w.deliverInputTaskCmd(from, p, &req, nil, nil, "")
	return
}

func (w *Worker) RunTask(task *workflow.Task, input string) {

}

func (w *Worker) RunBroker(broker *workflow.Broker, input string, triggerID int64) {
	group := w.getGroup()
	part := w.getPart("", 0, 0)
	reply := workflow.CmdReplyTask{
		Cmd:        -1,
		BeginPart:  true,
		StartInput: input,
		Start:      true,
		RunOn:      workflow.GetBrokerName(broker.Name),
		BrokerID:   broker.ID,
		BrokerName: broker.Name,
		Group:      group,
		Part:       part,
		WorkflowID: w.workflow.ID,
		Trigger: &workflow.Trigger{
			ID: triggerID,
		},
	}
	var req workflow.CmdTask
	req.Cmd = workflow.InputTaskCmd

	req.From = reply.RunOn
	resultCh := make(chan []byte)
	req.WorkflowID = reply.WorkflowID
	req.Group = reply.Group
	req.StartInput = reply.StartInput
	var brokerInput map[string]workflow.ReplyData
	err := json.Unmarshal([]byte(input), &brokerInput)
	if err != nil {
		w.conf.Logger.Error(err)
	}
	go w.processBroker(broker, *w.points[workflow.GetBrokerName(broker.Name)], &reply, &req, &brokerInput, resultCh)
	<-resultCh
	w.conf.Logger.Warnf("Finish broker")
}

func (w *Worker) handleLogFromDaem(log *workflow.LogDaemon) {
	if log.Reply != nil {
		w.conf.Logger.Warnf("LOGGER  REPLY =  %+v", log.Reply)
		reply := log.Reply
		reply.DeliverID = -2
		from := w.points[reply.RunOn]
		to := w.points[workflow.GetBrokerName(reply.BrokerName)]
		m, err := json.Marshal(reply)
		if err != nil {
			w.conf.Logger.Error(err)
			return
		}
		if from == nil {
			w.conf.Logger.Errorf("Not found from name=%s", reply.TaskName)
			return
		}
		if to == nil {
			w.conf.Logger.Errorf("Not found from name=%s", reply.BrokerName)
			return
		}

		w.logMessageFlowReply(from, to, m, log.SendAt, log.BrokerGroup)
	} else if log.Request != nil {
		w.conf.Logger.Warnf("LOGGER  REQUEST =  %+v", log.Request)
		request := log.Request
		request.DeliverID = -2
		from := w.points[workflow.GetBrokerName(log.BrokerName)]
		// để giống chiều từ server đến daemon.
		to := w.points[log.RunOn]
		if from == nil {
			w.conf.Logger.Errorf("Not found from name=%s", log.BrokerName)
			return
		}
		if to == nil {
			w.conf.Logger.Errorf("Not found from name=%s", request.Task.Name)
			return
		}
		w.logMessageFlowRequest(from, to, request, log.SendAt, log.Tracking, log.BrokerGroup)
	}
}

func (w *Worker) reciveMessage() error {
	_, err := w.natsConn.Subscribe(fmt.Sprintf("w-%d", w.workflow.ID), func(msg *nats.Msg) {
		var reply workflow.CmdReplyTask
		err := json.Unmarshal(msg.Data, &reply)
		if (err != nil || reply.WorkflowID != w.workflow.ID) &&
			strings.HasSuffix(reply.RunOn, workflow.SubChannel) &&
			strings.HasSuffix(reply.RunOn, workflow.SubClient) {
			return
		}

		fmt.Printf("[ NATS REPLY !!] %+v\n", reply)
		if reply.Cmd == workflow.ReplyLogWfCmd {
			var log workflow.LogDaemon
			if err := json.Unmarshal(msg.Data, &log); err == nil {
				w.handleLogFromDaem(&log)
			} else {
				w.conf.Logger.Error(err)
			}
			return
		}

		if reply.DeliverID == DeliverBorkerCmd {
			reply.Status = workflow.SuccessReceiveOutputTaskCmd
			reply.WorkflowID = w.workflow.ID
			fmt.Printf("[ REPLY MSG SEND TO BROKER] ============= %+v\n", reply)
			var req = workflow.CmdTask{
				Cmd:        workflow.InputTaskCmd,
				From:       reply.RunOn,
				WorkflowID: reply.WorkflowID,
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

	_, err = w.natsConn.Subscribe(fmt.Sprintf("%s-%d", messaging.TRIGGER, w.workflow.ID), func(msg *nats.Msg) {
		var trigger workflow.Trigger
		err := json.Unmarshal(msg.Data, &trigger)
		if err != nil {
			w.conf.Logger.Error(err)
		}
		w.conf.Logger.Infof("Reciver trigger %+v", trigger)
		w.handleTrigger(trigger)
	})

	subs := make(map[string]bool)
	for _, c := range w.workflow.Clients {
		subs[fmt.Sprintf("%s-%d@%d", workflow.KairosDaemon, c.ID, w.workflow.UserID)] = true
	}

	for _, c := range w.workflow.Channels {
		subs[fmt.Sprintf("%s-%d@%d", c.Name, c.ID, w.workflow.UserID)] = true
	}

	for sub := range subs {
		_, err = w.natsConn.Subscribe(sub, func(msg *nats.Msg) {
			w.conf.Logger.Warnf("Receiver message from %s", msg.Subject)
			ids := strings.Split(msg.Subject, "@")
			uid, _ := strconv.ParseInt(ids[1], 10, 64)
			cids := strings.Split(ids[0], "-")
			cid, _ := strconv.ParseInt(cids[1], 10, 64)
			w.syncData(cids[0], uid, cid, string(msg.Data))
		})
		if err != nil {
			w.conf.Logger.Error(err)
			return err
		}
	}
	return err
}

func (w *Worker) syncData(cType string, userID, clientID int64, status string) {
	w.conf.Logger.Errorf(cType)
	if status == "connect" {
		if cType == workflow.KairosDaemon {

		} else {
			mfs, err := storage.GetMessageFlowNotDeliver(clientID, ChannelPoint, 0)
			w.conf.Logger.Error(len(mfs))
			if err != nil {
				w.conf.Logger.Error(err)
				return
			}
			for _, mf := range mfs {
				reqs, err := storage.GetMessageFlowDeliver(mf.DeliverID)
				if err != nil {
					w.conf.Logger.Error(err)
					continue
				}
				if len(reqs) != 1 {
					w.conf.Logger.Errorf("Deliver can't found deliverID = %d", mf.DeliverID)
					continue
				}
				req := reqs[0]
				cmdReq := workflow.CmdTask{
					Cmd: workflow.RequestActionTask(req.Cmd),
					Task: &workflow.Task{
						ID:    req.TaskID,
						Name:  req.TaskName,
						Input: req.Message,
					},
					Message: req.Message,
					Retry: &workflow.Retry{
						Attempt: req.Attempt,
					},
					DeliverID:  req.DeliverID,
					WorkflowID: req.WorkflowID,
					Attempt:    0,
					Group:      req.Group,
					Start:      req.Start,
					StartInput: req.StartInput,
					Parent:     req.Parent,
					Part:       req.Part,
				}
				cmdReply := workflow.CmdReplyTask{
					Cmd:       workflow.ResponseActionTask(mf.Cmd),
					Group:     mf.Group,
					DeliverID: mf.DeliverID,
					Parent:    mf.Parent,
					Part:      mf.Part,
				}

				w.handleRetryDeliver("", &DeliverErr{
					From: &Point{
						ID:   req.SenderID,
						Type: req.SenderType,
						Name: req.SenderName,
					},
					Receiver: &Point{
						ID:   req.ReceiverID,
						Type: req.ReceiverType,
						Name: req.ReceiverName,
					},
					Msg:   &cmdReq,
					Reply: &cmdReply,
				})
			}
		}
	}
}

func (w *Worker) processBroker(broker *workflow.Broker, from Point, reply *workflow.CmdReplyTask, req *workflow.CmdTask, input *map[string]workflow.ReplyData, resultCh chan []byte) error {
	bg := w.getBrokerGroup() // tính thời điểm vào broker
	sender := w.points[reply.RunOn]
	m, _ := json.Marshal(reply)

	req.Task = &workflow.Task{
		ID:         reply.TaskID,
		WorkflowID: reply.WorkflowID,
		Name:       reply.TaskName,
	}

	w.CBQueue.Push(func(duration time.Duration) {
		w.logMessageFlowReply(sender, &from, m, reply.SendAt, bg)
	})

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
		if input != nil {
			for k, v := range *input {
				replies[k] = v
			}
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
		fmt.Printf("[WORKER DELIVERS] %+v exeOutput = %+v \n", delivers, exeOutput)
		w.handleDeliver(&from, delivers, reply, req, trun, resultCh, exeOutput, bg)
	}
	return nil
}

func (w *Worker) computeMsg(reply *workflow.CmdReplyTask, req *workflow.CmdTask, resultCh chan []byte) {
	// TODO TEST
	// if !w.IsRunning() {
	// 	w.conf.Logger.Error("worker not running")
	// 	return
	// }

	if reply.Group == "" {
		w.conf.Logger.Errorf("Empty group runon=%s wid=%d", reply.RunOn, reply.WorkflowID)
		return
	}

	req.Group = reply.Group

	m, _ := json.Marshal(reply)

	sender := w.points[reply.RunOn]
	if sender == nil {
		w.conf.Logger.Errorf("unknow %s", reply.RunOn)
		return
	}

	isHasDest := false
	if reply.TaskInput != "" {
		w.conf.Logger.Warnf("TaskInput %+v", reply.TaskInput)
		w.CBQueue.Push(func(duration time.Duration) {
			w.logMessageFlowReply(sender, sender, m, reply.SendAt, "")
		})
		if reply.Start {
			isHasDest = true
		}
		reply.Start = false
	}

	w.workflow.Brokers.Range(func(key string, broker *workflow.Broker) error {
		// reply.Part = w.getReplyPart(reply.Parent, broker.ID, BrokerPoint)
		from := Point{
			ID:   broker.ID,
			Type: BrokerPoint,
			Name: broker.Name,
		}

		if reply.WorkflowID != w.workflow.ID {
			w.conf.Logger.Error("invalid message")
			return fmt.Errorf("invalid message")
		}

		if (broker.IsListen(reply.TaskName) || broker.IsListen(reply.RunOn)) && len(broker.Clients) == 0 {
			newReply := *reply
			newReply.Parent = reply.Part
			newReply.Part = w.getPart(reply.Part, broker.ID, BrokerPoint)
			isHasDest = true
			w.conf.Logger.Debugf("Reply runon=%s sendAt=%d Now=%d", newReply.RunOn, newReply.SendAt, helper.GetTimeNow())

			if strings.HasSuffix(newReply.RunOn, workflow.SubTask) {
				if newReply.Result == nil {
					err := fmt.Errorf("newReply from %s empty result", newReply.RunOn)
					w.conf.Logger.Error(err)
					return err
				}
				w.conf.Logger.Infof("Reciver message from %s value= %+v", newReply.RunOn, newReply.Result)
			}

			if strings.HasSuffix(newReply.RunOn, workflow.SubChannel) {
				if newReply.Content == nil {
					err := fmt.Errorf("newReply from %s empty content", newReply.RunOn)
					w.conf.Logger.Error(err)
					return err
				}
				w.conf.Logger.Infof("Reciver message from %s value= %+v", newReply.RunOn, newReply.Content)
			}

			w.processBroker(broker, from, &newReply, req, nil, resultCh)
		}
		return nil
	})

	if !isHasDest {
		w.CBQueue.Push(func(duration time.Duration) {
			from := w.points["kairos"]
			w.logMessageFlowReply(sender, from, m, reply.SendAt, "")
		})
	}
}

func (w *Worker) handleDeliver(from *Point, delivers []*workflow.DeliverFlow, reply *workflow.CmdReplyTask, req *workflow.CmdTask, trun *workflow.TemplateRuntime, resultCh chan []byte, exeOutput *workflow.ExecOutput, bg string) {
	hasSync := false
	for _, path := range delivers {
		receivers := strings.Split(path.Reciever, ",")
		for _, rev := range receivers {
			fmt.Printf("[ DEBUG OUTPUT TASK] %+v\n", path)
			w.conf.Logger.Infof("Path: from= %s to receiver= %s msg= %+v", from.Name, rev, path.Msg)
			receiver := w.points[rev]
			if receiver == nil {
				w.conf.Logger.Error(fmt.Errorf("can't find receiver %s", rev))
				return
			}
			reqDeliver := *req
			reqDeliver.WorkflowID = w.workflow.ID
			if reply != nil {
				reqDeliver.Part = w.getPart(reply.Part, receiver.ID, receiver.Type)
				reqDeliver.Parent = reply.Part
			}

			if strings.HasSuffix(rev, workflow.SubChannel) {
				reqDeliver.Message = path.Msg
			} else if strings.HasSuffix(rev, workflow.SubTask) {
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
			} else if strings.HasSuffix(rev, workflow.SubClient) {

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
		}
	}

	if len(delivers) == 0 {
		reqDeliver := *req
		reqDeliver.WorkflowID = w.workflow.ID
		if reply != nil {
			tracking, err := json.Marshal(exeOutput)
			if err != nil {
				w.conf.Logger.Error(err)
			}
			reqDeliver.Part = w.getPart(reply.Part, from.ID, from.Type)
			reqDeliver.Parent = reply.Part
			reqDeliver.Message = string(tracking)
			go w.deliverInputTaskCmd(from, from, &reqDeliver, reply, exeOutput, bg)
		}
	}

	if resultCh != nil && !hasSync {
		resultCh <- []byte("")
	}
}

type ReplyTask struct {
	Result *workflow.Result `json:"result"`
}

type ReplyChannel struct {
	Content *workflow.Content `json:"content"`
}

func (w *Worker) deliverRequestRunTaskSync(from *Point, receiver *Point, reply *workflow.CmdReplyTask, req *workflow.CmdTask, trun *workflow.TemplateRuntime, resultCh chan []byte, exeOutput *workflow.ExecOutput, bg string) {
	if req.DeliverID == 0 {
		req.DeliverID = w.nextCmdID()
	}
	if req.Attempt == 0 {
		req.Attempt++
	}

	err := w.deliverAsync(from, receiver, req, func(crt *workflow.CmdReplyTask, err error) {
		fmt.Printf("[CALLBACK deliverRequestRunTaskCmd ] result = %+v err = %+v\n", crt.Result, err)

		if err != nil {
			w.wrapDeliverErr(from, receiver, req, crt, err)
			trun.ExpInput = append(trun.ExpInput, err.Error())
		} else {
			if req.Retry != nil {
				storage.SetWfRecovered(req.Retry.ID, int(req.Retry.Attempt)+1, true)
			}
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
	if req.DeliverID == 0 {
		req.DeliverID = w.nextCmdID()
	}
	if req.Attempt == 0 {
		req.Attempt++
	}
	w.CBQueue.Push(func(duration time.Duration) {
		if reply != nil && reply.Cmd == workflow.ReplyOutputTaskCmd {
			w.saveTaskRecord(reply)
		}
		var tracking string
		if exeOutput != nil {
			eo, _ := json.Marshal(exeOutput)
			tracking = string(eo)
		}

		w.logMessageFlowRequest(from, receiver, req, duration.Milliseconds(), tracking, bg)
	})

	if !(from.ID == receiver.ID && from.Name == receiver.Name && from.Type == receiver.Type) {
		err := w.deliverAsync(from, receiver, req, func(crt *workflow.CmdReplyTask, err error) {
			fmt.Printf("[CALLBACK DELIVER InputTaskCmd ] res = %+v err = %+v\n", crt, err)
			if err != nil {
				w.wrapDeliverErr(from, receiver, req, crt, err)
				return
			}
			crt.Group = req.Group
			crt.Parent = req.Parent
			crt.Part = req.Part
			m, _ := json.Marshal(crt)
			if req.Retry != nil {
				storage.SetWfRecovered(req.Retry.ID, int(req.Retry.Attempt)+1, true)
			}

			w.CBQueue.Push(func(duration time.Duration) {
				w.logMessageFlowReply(receiver, from, m, crt.SendAt, "")
			})
		})

		if err != nil {
			var crt workflow.CmdReplyTask
			w.wrapDeliverErr(from, receiver, req, &crt, err)
		}
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

func (w *Worker) getPart(replyPart string, rvID int64, rvType int) string {
	w.fm.RLock()
	defer w.fm.RUnlock()
	if replyPart == "" {
		id := w.nextCmdID()
		p := fmt.Sprintf("part-%d-%d", w.workflow.ID, id)
		return p
	}
	my := fmt.Sprintf("%s@%d-%d", replyPart, rvID, rvType)
	reqPart, ok := w.flowMap[my]
	if !ok {
		id := w.nextCmdID()
		p := fmt.Sprintf("part-%d-%d", w.workflow.ID, id)
		w.flowMap[my] = p
		return p
	}
	return reqPart
}

func (w *Worker) getReplyPart(parentPart string, rvID int64, rvType int) string {
	w.fm.RLock()
	defer w.fm.RUnlock()
	if parentPart == "" {
		id := w.nextCmdID()
		p := fmt.Sprintf("part-%d-%d", w.workflow.ID, id)
		return p
	}
	my := fmt.Sprintf("%s@%d-%d", parentPart, rvID, rvType)
	reqPart, ok := w.requestMap[my]
	if !ok {
		id := w.nextCmdID()
		p := fmt.Sprintf("part-%d-%d", w.workflow.ID, id)
		w.requestMap[my] = p
		return p
	}
	return reqPart
}

func (w *Worker) getGroup() string {
	id := w.nextCmdID()
	return fmt.Sprintf("kairos-%d-%d-0", w.workflow.ID, id)
}

func (w *Worker) getBrokerGroup() string {
	id := w.nextCmdID()
	return fmt.Sprintf("kairosbroker-%d-%d-0", w.workflow.ID, id)
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
	cmdDeliver := *cmd
	cmdDeliver.Channel = receiver.getChannel()

	cmdDeliver.Status = workflow.PendingDeliver
	cmdDeliver.SendAt = time.Now().UnixMilli()
	fmt.Println("DELIVERID BEFORE-------------------- ", cmdDeliver.DeliverID)
	if cmdDeliver.DeliverID == 0 {
		cmdDeliver.DeliverID = w.nextCmdID()
	}
	fmt.Println("DELIVERID -------------------- ", cmdDeliver.DeliverID)
	w.addRequest(cmdDeliver.DeliverID, cb)

	err := w.deliver(&cmdDeliver)
	fmt.Println("________________________")
	fmt.Printf("%+v\n task= %+v \n", cmdDeliver, cmdDeliver.Task)
	fmt.Println("________________________")
	if err != nil {
		w.conf.Logger.Error(err)
		return err
	}
	go func() {
		defer w.removeRequest(cmdDeliver.DeliverID)
		select {
		case <-time.After(w.conf.DeliverTimeout):
			w.requestsMu.RLock()
			req, ok := w.requests[cmdDeliver.DeliverID]
			w.requestsMu.RUnlock()
			if !ok {
				return
			}
			var reply workflow.CmdReplyTask
			reply.DeliverID = cmdDeliver.DeliverID
			req.cb(&reply, ErrDeliverTimeout)
			return
		case <-w.closeCh:
			w.requestsMu.RLock()
			req, ok := w.requests[cmdDeliver.DeliverID]
			w.requestsMu.RUnlock()
			if !ok {
				return
			}
			var reply workflow.CmdReplyTask
			reply.DeliverID = cmdDeliver.DeliverID
			req.cb(&reply, ErrStopWorker)
		}
	}()
	return err
}

func (w *Worker) deliverTaskToClient(from *Point, receiver *Point, msg *workflow.CmdTask) error {
	if msg.DeliverID == 0 {
		msg.DeliverID = w.nextCmdID()
	}

	if msg.Attempt == 0 {
		msg.Attempt++
	}
	msg.Part = w.getPart("", -1, -1)
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
				err = storage.SetWfRecovered(e.ID, e.Retry+1, true)
				if err != nil {
					success = false
					return
				}
			})
		}

		wg.Wait()
		if !success {
			w.logWorkflow(
				fmt.Sprintf("Fault recovery workflow"),
				models.WFRecordFault,
				nil,
				-1,
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
			fmt.Sprintf("Successful Recovery Workflow"),
			models.WFRecordScuccess,
			nil,
			-1,
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
			w.updateStatusDeliverTask(task.ID, &taskDeliver{
				client: "kairos",
				status: workflow.PendingDeliver,
			})
			maps := make(map[string]interface{})
			if err := json.Unmarshal([]byte(task.Payload), &maps); err != nil {
				return err
			}

			if cmd == workflow.TriggerStartTaskCmd {
				task.Execute = func(ct *workflow.CmdTask) {
					from := w.points[workflow.GetTaskName(task.Name)]
					outputs := make(map[string]interface{})
					if ct != nil && ct.Task.Output != "" {
						err := json.Unmarshal([]byte(ct.Task.Output), &outputs)
						if err != nil {
							w.conf.Logger.Error(err)
							return
						}
					}
					for k, v := range maps {
						outputs[k] = v
					}
					data := ""
					switch outputs["data"].(type) {
					case string:
						data = outputs["data"].(string)
					default:
						str, _ := json.Marshal(outputs["data"])
						data = string(str)
					}
					objectName := outputs["sub"].(string)
					receiver := w.points[objectName]
					if receiver == nil {
						w.conf.Logger.Errorf("sub %s not exist", objectName)
						return
					}
					cmd := &workflow.CmdTask{
						Cmd:       workflow.InputTaskCmd,
						DeliverID: w.nextCmdID(),
						Group:     w.getGroup(),
						Channel:   receiver.Name,
					}

					if strings.HasSuffix(objectName, workflow.SubTask) {
						cmd.Task = &workflow.Task{
							ID:         receiver.ID,
							Name:       receiver.Name,
							Output:     data,
							WorkflowID: w.workflow.ID,
						}
					} else if strings.HasSuffix(objectName, workflow.SubChannel) {
						cmd.Channel = receiver.Name
						cmd.Message = data
					}

					// if ct != nil {
					// 	cmd.Group = ct.Group
					// 	cmd.Parent = ct.Part
					// 	cmd.Part = w.getPart(ct.Part)
					// } else {
					// 	cmd.Group = w.getGroup()
					// 	cmd.Part = w.getPart("")
					// 	cmd.Start = true
					// }
					delivers := make([]*workflow.DeliverFlow, 0)
					delivers = append(delivers, &workflow.DeliverFlow{
						Reciever: objectName,
						Send:     "send",
						Msg:      data,
					})
					w.handleDeliver(from, delivers, nil, cmd, nil, nil, nil, "")
					// w.CBQueue.Push(func(duration time.Duration) {
					// 	w.logMessageFlowRequest(from, receiver, cmd, duration.Milliseconds(), "", "")
					// })
					// w.deliverAsync(from, receiver, cmd, func(crt *workflow.CmdReplyTask, err error) {
					// 	if err != nil {
					// 		w.conf.Logger.Errorf("PUBSUB DELIVER ERR= %s", err.Error())
					// 		return
					// 	}
					// 	crt.Parent = cmd.Parent
					// 	crt.Part = cmd.Part
					// 	crt.Group = cmd.Group
					// 	crt.FinishPart = true
					// 	m, _ := json.Marshal(crt)
					// 	w.CBQueue.Push(func(duration time.Duration) {
					// 		w.logMessageFlowReply(receiver, from, m, duration.Milliseconds(), "")
					// 	})
					// 	//TODO
					// 	fmt.Println("[RECIVER CMD REPLY TASK]", crt)
					// })
				}
				w.Sched.AddTask(w, task, "")
				w.conf.Logger.Info("START Trigger")
			}
			w.Sched.AddTask(w, task, "")
			cmdReply := workflow.ReplySetTaskCmd
			status := workflow.SuccessSetTask
			if cmd == workflow.TriggerStartTaskCmd {
				cmdReply = workflow.ReplyStartTaskCmd
				status = workflow.SuccessTriggerTask
			}
			w.eventChan <- &Event{
				cmd: ReceiveDeliverTaskCmd,
				cmdtask: &workflow.CmdReplyTask{
					Cmd:    cmdReply,
					Status: status,
					RunOn:  "kairos",
					TaskID: task.ID,
				},
			}
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

func (w *Worker) UpdateTrigger(triggerID int64, newStatus int) {
	err := storage.UpdateStatusByID(triggerID, newStatus)
	if err != nil {
		w.conf.Logger.Error(err)
	}
	w.sendTriggerStatus(triggerID, newStatus)
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
		}
		if msg.Result.FinishedAt != 0 {
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

	if msg.DeliverID == -2 {
		mf.Flow = models.LogFlow
	} else if receiver.Type == KairosPoint {
		mf.Flow = models.KairosLogFlow
	} else {
		mf.Flow = models.RecieverFlow
	}

	if msg.Trigger != nil {
		mf.TriggerID = msg.Trigger.ID
	}

	mf.SenderID = from.ID
	mf.SenderType = from.Type
	mf.SenderName = from.Name
	mf.ReceiverID = receiver.ID
	mf.ReceiverType = receiver.Type
	mf.ReceiverName = receiver.Name
	mf.WorkflowID = w.workflow.ID
	mf.CreatedAt = helper.GetTimeNow()
	mf.Message = string(m)
	mf.Status = msg.Status
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
	mf.StartInput = msg.StartInput

	fmt.Printf("mf= %+v\n", mf)
	var id int64
	did, err := strconv.ParseInt(broker_group, 10, 64)
	if err == nil {
		w.conf.Logger.Errorf("RUN HERE %d", did)
		if err = storage.DeleteMessageFlow(did); err != nil {
			w.conf.Logger.Error(err)
		}
	}

	id, err = storage.LogMessageFlow(&mf)
	if err != nil {
		w.Sched.logger.Error(err)
	}
	mf.ID = id

	if did == 0 {
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

}

func (w *Worker) logMessageFlowRequest(from *Point, receiver *Point, msg *workflow.CmdTask, sendAt int64, tracking string, broker_group string) {
	var m []byte
	if msg.Task == nil && msg.Broker == nil && msg.Channel == "" {
		w.conf.Logger.Errorf("empty task and broker on request %+v", msg)
		return
	}

	var mf models.MessageFlow
	if receiver.Type == ChannelPoint {
		if s, ok := msg.Message.(string); ok {
			m = []byte(s)
		} else {
			m, _ = json.Marshal(msg.Message)
		}
	} else if msg.Task != nil {
		m, _ = json.Marshal(msg.Task)
		mf.TaskID = msg.Task.ID
		mf.TaskName = msg.Task.Name
	} else if msg.Broker != nil {
		m, _ = json.Marshal(msg.Broker)
		mf.TaskID = -1
	}

	if msg.DeliverID == -2 {
		mf.Flow = models.LogFlow
		mf.FinishPart = true
	} else {
		mf.Flow = models.DeliverFlow
	}

	if msg.Trigger != nil {
		mf.TriggerID = msg.Trigger.ID
	}
	mf.SenderID = from.ID
	mf.SenderType = from.Type
	mf.SenderName = from.Name
	mf.ReceiverID = receiver.ID
	mf.ReceiverType = receiver.Type
	mf.ReceiverName = receiver.Name
	mf.WorkflowID = w.workflow.ID
	mf.CreatedAt = helper.GetTimeNow()
	mf.Message = string(m)
	mf.Status = msg.Status
	mf.DeliverID = msg.DeliverID
	mf.SendAt = sendAt
	mf.Cmd = int(msg.Cmd)
	mf.Group = msg.Group
	mf.Start = msg.Start
	mf.StartInput = msg.StartInput
	mf.Parent = msg.Parent
	mf.Part = msg.Part
	mf.Attempt = msg.Attempt
	mf.BeginPart = true
	e, _ := json.Marshal(msg)
	mf.ResponseSize = len(e)
	mf.BrokerGroup = broker_group
	mf.Tracking = tracking
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
		w.conf.Logger.Error(err)
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

func (w *Worker) logWorkflow(record string, status int, de *DeliverErr, deliverID int64) int64 {
	var data string
	var err error

	isRecovered := true
	if de != nil {
		js, err := json.Marshal(de)
		if err != nil {
			w.conf.Logger.Error(err)
			return -1
		}
		isRecovered = false
		data = string(js)
	}
	rcid, err := storage.InsertWorkflowRecord(&models.WorkflowRecords{
		CreatedAt:   helper.GetTimeNow(),
		WorkflowID:  w.workflow.ID,
		Record:      record,
		Status:      status,
		DeliverErr:  data,
		IsRecovered: isRecovered,
		DeliverID:   deliverID,
	})
	if err != nil {
		w.conf.Logger.Error(err)
	}

	return rcid
}

func (w *Worker) getDelivererDelay(attempt int) time.Duration {
	return w.backoffDeliver.timeBeforeNextAttempt(attempt)
}

type backoffDeliver struct {
	Factor   float64
	Jitter   bool
	MinDelay time.Duration
	MaxDelay time.Duration
}

var defaultBackoffDeliver = &backoffDeliver{
	MinDelay: 200 * time.Millisecond,
	MaxDelay: 20 * time.Second,
	Factor:   2,
	Jitter:   true,
}

func (r *backoffDeliver) timeBeforeNextAttempt(attempt int) time.Duration {
	b := &backoff.Backoff{
		Min:    r.MinDelay,
		Max:    r.MaxDelay,
		Factor: r.Factor,
		Jitter: r.Jitter,
	}
	return b.ForAttempt(float64(attempt))
}

func (w *Worker) sendTriggerStatus(id int64, status int) {
	mwf := workflow.MonitorWorkflow{
		Cmd:          workflow.TriggerStatusWorkflow,
		UserID:       w.workflow.UserID,
		WorkflowID:   w.workflow.ID,
		WorkflowName: w.workflow.Name,
		Data: `{
			"id": "` + fmt.Sprint(id) + `",
			"status": "` + fmt.Sprint(status) + `"
		}`,
	}
	js, _ := json.Marshal(mwf)

	if err := w.natsConn.Publish(messaging.MONITOR_WORKFLOW, js); err != nil {
		w.conf.Logger.Errorf("can't send trigger status id = %d err=%s", id, err.Error())
	}
}
