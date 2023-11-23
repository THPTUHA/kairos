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
	ErrTimeout    = errors.New("worker: timeout")
	ErrStopWorker = errors.New("worker: stop")
	ErrEmptyTask  = errors.New("woker: empty task")
)

// event cmd
const (
	DeliverBorkerCmd = iota
	DeleteWorkerCmd
	ReceiveDeliverTaskCmd
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
	from     *Point
	receiver *Point
	msg      *workflow.CmdTask
	reply    *workflow.CmdReplyTask
}
type taskDeliver struct {
	client string
	status int
}
type Worker struct {
	ID           int64
	eventChan    chan *Event
	deliverErrCh chan *DeliverErr
	deliverCmd   int64
	mu           sync.RWMutex
	status       int
	conf         *WorkerConfig
	workflow     *workflow.Workflow
	requestsMu   sync.RWMutex
	cacheMu      sync.RWMutex
	requests     map[int64]request
	points       map[string]*Point
	taskDelivers map[int64][]*taskDeliver
	CBQueue      *cbqueue.CBQueue
	closeCh      chan struct{}
	Sched        *Scheduler

	natsOptions *natsOptions
	natsConn    *nats.Conn
}

func NewWorker(workerID int64, workflow *workflow.Workflow, conf *WorkerConfig, Sched *Scheduler) *Worker {
	return &Worker{
		ID:           workerID,
		eventChan:    make(chan *Event),
		deliverErrCh: make(chan *DeliverErr),
		status:       events.WorkerCreating,
		workflow:     workflow,
		natsOptions:  defaultNatsOptions(),
		conf:         conf,
		requests:     map[int64]request{},
		points:       make(map[string]*Point),
		taskDelivers: make(map[int64][]*taskDeliver),
		Sched:        Sched,
	}
}

func (w *Worker) IsRunning() bool {
	w.mu.Lock()
	status := w.status
	defer w.mu.Unlock()
	return status == events.WorkerRunning
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

func (w *Worker) allTaskDelivered() bool {
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
	return true
}

func (w *Worker) Destroy() {
	w.eventChan <- &Event{
		cmd: events.DeleteWorker,
	}
}

type Point struct {
	ID   int64  `json:"id"`
	Type int    `json:"type"`
	Name string `json:"name"`
}

type WorkerConfig struct {
	MaxAttempDeliverTask int
	TimeoutRetryDeliver  time.Duration
	DeliverTimeout       time.Duration
	DeliverDelay         time.Duration
	Logger               *logrus.Entry
}

type request struct {
	cb func(*workflow.CmdReplyTask, error)
}
type natsOptions struct {
	url           string
	name          string
	reconnectWait time.Duration
	maxReconnects int
}

func defaultNatsOptions() *natsOptions {
	optsDef := natsOptions{
		name:          "worker",
		reconnectWait: 2 * time.Second,
		maxReconnects: 10,
	}
	return &optsDef

}

func optNats(o *natsOptions) []nats.Option {
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
	<-w.closeCh
	log.Debug().Msg(fmt.Sprintf("finish worker workflow running name = %s, namespace = %s", w.workflow.Name, w.workflow.Namespace))
}

func (w *Worker) retryDeliver() {
	for e := range w.deliverErrCh {
		fmt.Printf("[ERRR DELIVER ----] %+v\n", e)
		w.conf.Logger.WithFields(logrus.Fields{
			"from":     e.from.Name,
			"receiver": e.receiver.Name,
			"taskid":   e.msg.Task.ID,
			"err":      e.err.Error(),
		})

		msg := e.msg
		w.CBQueue.Push(func(_ time.Duration) {
			reply := e.reply
			reply.TaskID = msg.Task.ID
			reply.WorkflowID = msg.Task.WorkflowID
			if reply.Message == "" {
				reply.Message = e.err.Error()
			}

			switch msg.Cmd {
			case workflow.SetTaskCmd:
				reply.Cmd = workflow.ReplySetTaskCmd
				reply.Status = workflow.FaultSetTask
			case workflow.TriggerStartTaskCmd:
				reply.Cmd = workflow.ReplyStartTaskCmd
				reply.Status = workflow.FaultTriggerTask
			case workflow.InputTaskCmd:
				reply.Cmd = workflow.ReplyInputTaskCmd
				reply.Status = workflow.FaultInputTask
			}
			m, _ := json.Marshal(reply)
			w.logMessageFlowReply(e.from, e.receiver, m, helper.GetTimeNow()-msg.SendAt)
		})

		// TODO retry here
	}
}

func (w *Worker) wrapDeliverErr(from *Point, receiver *Point, cmd *workflow.CmdTask, reply *workflow.CmdReplyTask, err error) {
	w.deliverErrCh <- &DeliverErr{
		from:     from,
		receiver: receiver,
		reply:    reply,
		msg:      cmd,
		err:      err,
	}
}

func (w *Worker) setWfStatus(status int) {
	fmt.Printf("[SET STATUS WORKFLOW] %d user = %d\n", status, w.workflow.UserID)
	w.workflow.Status = status
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
	fmt.Println(" NATS ERROR ", err)
}

func (w *Worker) Run() error {
	w.conf.Logger.Debug(fmt.Sprintf("worker %d run ", w.ID))
	natConn, err := nats.Connect(nats.DefaultURL, optNats(defaultNatsOptions())...)
	if err != nil {
		return err
	}
	w.natsConn = natConn
	err = w.reciveMessage()
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
					client: e.cmdtask.RunIn,
				})

				if w.allTaskDelivered() {
					// TODO Send start and go to start
					fmt.Printf("[ALL TASK DELIVER SUCCESS]\n")
					go w.setWfStatus(workflow.Running)
					w.pushAllCmdTask(workflow.TriggerStartTaskCmd)
				}
			}
		}
	}
}

func (w *Worker) reciveMessage() error {
	_, err := w.natsConn.Subscribe(fmt.Sprint(w.workflow.UserID), func(msg *nats.Msg) {
		// TODO cho đi qua broker => trả về điểm đến => thực hiện lưu message và deliver
		var reply workflow.CmdReplyTask
		err := json.Unmarshal(msg.Data, &reply)
		if err != nil || reply.WorkflowID != w.workflow.ID {
			return
		}
		// Output
		fmt.Println("[ NATS REPLY !!]")
		if reply.DeliverID == DeliverBorkerCmd {
			reply.Status = workflow.SuccessReceiveOutputTaskCmd
			fmt.Printf("[ REPLY MSG SEND TO BROKER] ============= %+v\n", reply)
			var req workflow.CmdTask
			req.Cmd = workflow.InputTaskCmd
			req.DeliverID = w.nextCmdID()
			// result task
			req.From = reply.RunIn

			w.workflow.Brokers.Range(func(key string, broker *workflow.Broker) error {
				from := Point{
					ID:   broker.ID,
					Type: BrokerPoint,
					Name: broker.Name,
				}

				fmt.Println("brokder-------", broker.Listens, broker.Flows.Endpoints, reply.TaskName, broker.IsListen(reply.TaskName))
				if broker.IsListen(reply.TaskName) || broker.IsListen(reply.RunIn) {
					m, _ := json.Marshal(reply)
					w.CBQueue.Push(func(duration time.Duration) {
						sender := w.points[reply.RunIn]
						// task -> broker
						w.logMessageFlowReply(sender, &from, m, helper.GetTimeNow()-reply.SendAt)
					})
					if len(broker.Flows.Endpoints) > 0 {
						req.Task = &workflow.Task{
							ID:         reply.TaskID,
							WorkflowID: reply.WorkflowID,
							Name:       reply.TaskName,
							Input:      reply.Result.Output,
						}

						for _, endpoint := range broker.Flows.Endpoints {
							receiver := w.points[endpoint]
							if receiver.Type == TaskPoint {
								w.workflow.Tasks.Range(func(_ string, task *workflow.Task) error {
									if receiver.ID == task.ID {
										for _, c := range task.Clients {
											receiver = w.points[workflow.GetClientName(c)]
											go w.deliverInputTaskCmd(&from, receiver, &req, &reply, key)
										}
									}
									return nil
								})
							} else {
								go w.deliverInputTaskCmd(&from, receiver, &req, &reply, key)
							}

						}
					} else {
						var bqs []*models.BrokerQueue
						if !broker.Queue {
							if strings.HasSuffix(reply.RunIn, workflow.SubChannel) {
								w.cacheMu.RLock()
								c := strings.ToLower(reply.RunIn)
								broker.DynamicVars[c] = &reply
								w.cacheMu.RUnlock()
							}

							if strings.HasSuffix(reply.RunIn, workflow.SubClient) {
								w.cacheMu.RLock()
								c := strings.ToLower(reply.RunIn)
								broker.DynamicVars[c] = &reply
								w.cacheMu.RUnlock()

								if w.workflow.Tasks.Exists(reply.TaskName) {
									w.cacheMu.RLock()
									c := strings.ToLower(workflow.GetTaskName(reply.TaskName))
									broker.DynamicVars[c] = &reply
									w.cacheMu.RUnlock()
								}
							}

						} else {
							setBrokerQueue(strings.ToLower(reply.RunIn), &reply, reply.WorkflowID)
							if strings.HasSuffix(reply.RunIn, workflow.SubClient) && reply.TaskName != "" {
								setBrokerQueue(strings.ToLower(workflow.GetTaskName(reply.TaskName)), &reply, reply.WorkflowID)
							}

							// HERE
							vars := make(map[string]bool)
							for k := range broker.Exps.ListenVars {
								k = strings.TrimPrefix(strings.ToLower(workflow.GetRootDefaultVar(k)), ".")
								vars[k] = true
							}

							bqs, err = storage.GetKVQueues(vars, w.workflow.ID)
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
						for k, _ := range broker.DynamicVars {
							fmt.Println("DEBG DYNAMIC VAR", k)
						}
						// TODO Get Vars
						delivers, err := broker.Exps.Execute(broker.DynamicVars)
						if err == nil {
							w.cacheMu.Lock()
							for k := range broker.DynamicVars {
								broker.DynamicVars[k] = nil
							}
							ids := make([]int64, 0)
							for _, bq := range bqs {
								ids = append(ids, bq.ID)
							}
							err := storage.UsedKVQueue(ids)
							if err != nil {
								fmt.Printf("[Used KV QUeue] %s\n", err)
							}
							w.cacheMu.Unlock()
						} else if err == workflow.ErrorVariableNotReady {
							// TODO save queue
							w.cacheMu.Lock()
							w.cacheMu.Unlock()
						}

						for _, path := range delivers {
							fmt.Printf("[ DEBUG OUTPUT TASK] %+v\n", path)

							receiver := w.points[path.Reciever]
							req.Task = &workflow.Task{
								ID:         path.Msg.TaskID,
								WorkflowID: path.Msg.WorkflowID,
								Name:       path.Msg.TaskName,
								Input:      path.Msg.Result.Output,
							}

							// TODO set message
							if receiver.Type == TaskPoint {
								w.workflow.Tasks.Range(func(_ string, task *workflow.Task) error {
									if receiver.ID == task.ID {

										for _, c := range task.Clients {
											cr := w.points[workflow.GetClientName(c)]
											go w.deliverInputTaskCmd(&from, cr, &req, &reply, key)
										}
									}
									return nil
								})
							} else {
								go w.deliverInputTaskCmd(&from, receiver, &req, &reply, key)
							}
						}

						fmt.Printf("[WORKER DELIVERS] %+v ERR=%+v \n", delivers, err)
					}
				}
				return nil
			})
		} else {
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

func (w *Worker) deliverInputTaskCmd(from, receiver *Point, req *workflow.CmdTask, reply *workflow.CmdReplyTask, key string) {
	// TODO add client and test
	fmt.Printf("[BROKER %s DELIVER OUTPUT TASK BY ENDPOINT TO%+v, %s] \n", key, receiver, req.Task.Name)
	// deliver to
	req.Channel = receiver.Name
	w.CBQueue.Push(func(duration time.Duration) {
		cf := w.points[workflow.GetClientName(reply.RunIn)]
		if reply.Cmd == workflow.ReplyOutputTaskCmd {
			// Save record here
			fmt.Println("[DEBUG SAVE MESSAGE]", from, cf, req)
			// saveTaskRelpyFlow(cf, &from, &reply)
		}
		w.logMessageFlowRequest(from, receiver, req, duration.Milliseconds())
	})

	err := w.deliverAsync(from, receiver, req, func(crt *workflow.CmdReplyTask, err error) {
		fmt.Printf("[CALLBACK DELIVER InputTaskCmd ] res = %+v err = %+v\n", req, err)
		// TODO  save record
		if err != nil {
			w.wrapDeliverErr(from, receiver, req, crt, err)
			return
		}
		m, _ := json.Marshal(crt)

		w.CBQueue.Push(func(duration time.Duration) {
			w.logMessageFlowReply(receiver, from, m, helper.GetTimeNow()-req.SendAt)
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
	if receiver.Type == ClientPoint {
		cmd.Channel = fmt.Sprintf("kairosdeamon-%d", receiver.ID)
	} else if receiver.Type == TaskPoint {
		cmd.Channel = fmt.Sprintf("%s.%d", receiver.Name, receiver.ID)
	}
	cmd.Status = workflow.PendingDeliver
	cmd.SendAt = helper.GetTimeNow()
	// TODO save message
	w.addRequest(cmd.DeliverID, cb)

	err := w.deliver(cmd)
	if err != nil {
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
			fmt.Printf("[TIMEOUT DELIVER %+v]\n", cmd)
			var reply workflow.CmdReplyTask
			req.cb(&reply, ErrTimeout)
		case <-w.closeCh:
			log.Debug().Msg("close chan")
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
	fmt.Println("deliverTaskToClient 1")
	w.CBQueue.Push(func(duration time.Duration) {
		w.logMessageFlowRequest(from, receiver, msg, duration.Milliseconds())
	})
	fmt.Println("deliverTaskToClient 2")
	err := w.deliverAsync(from, receiver, msg, func(r *workflow.CmdReplyTask, err error) {
		fmt.Println("deliverTaskToClient 3")
		if err != nil {
			fmt.Printf("[ERR] DELIVER RESPONSE: %s\n", err)
			w.wrapDeliverErr(from, receiver, msg, r, err)
			return
		}
		// TODO handle reply
		switch r.Cmd {
		case workflow.ReplySetTaskCmd:
			fmt.Printf("[CALLBACK DELIVER Reply SetTaskCmd] REPLY= %+v\n", r)
			w.CBQueue.Push(func(duration time.Duration) {
				m, _ := json.Marshal(r)
				w.logMessageFlowReply(receiver, from, m, helper.GetTimeNow()-msg.SendAt)
			})

			w.eventChan <- &Event{
				cmd:     ReceiveDeliverTaskCmd,
				cmdtask: r,
			}
		case workflow.ReplyStartTaskCmd:
			fmt.Printf("[CALLBACK DELIVER Reply StartTaskCmd] REPLY= %+v\n", r)
			m, _ := json.Marshal(r)
			w.CBQueue.Push(func(duration time.Duration) {
				// TODO add record task ready
				w.logMessageFlowReply(receiver, from, m, helper.GetTimeNow()-msg.SendAt)
			})
		}

	})
	return err
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
			Task: task,
			Cmd:  cmd,
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
		case workflow.WebHookTask:

		case workflow.HttpTask:
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
	ElapsedTime  int64                  `json:"elapsed_time"`
}

func (w *Worker) logMessageFlowReply(from *Point, receiver *Point, mrs []byte, elapsedTime int64) {
	var mf models.MessageFlow
	var msg workflow.CmdReplyTask
	json.Unmarshal(mrs, &msg)
	mf.SenderID = from.ID
	mf.SenderType = from.Type
	mf.ReceiverID = receiver.ID
	mf.ReceiverType = receiver.Type
	mf.WorkflowID = msg.WorkflowID
	mf.CreatedAt = helper.GetTimeNow()
	mf.Message = msg.Message
	mf.Status = msg.Status
	mf.Flow = models.RecieverFlow
	mf.DeliverID = msg.DeliverID
	mf.ElapsedTime = elapsedTime
	mf.ResponseSize = len(mrs)

	id, err := storage.LogMessageFlow(&mf)
	if err != nil {
		fmt.Println("[SAVE TASK REPLY FLOW ERR]", err)
	}
	fmt.Printf("DELIVER MASSGE %+v \n", msg.Result)
	lmr := LogMessageReply{
		ID:           id,
		From:         from,
		Receiver:     receiver,
		Msg:          &msg,
		Flow:         mf.Flow,
		ResponseSize: len(mrs),
		ElapsedTime:  elapsedTime,
	}
	// fmt.Printf("[SAVE TASK REPLY FLOW] %+v Err= %+v\n", mf, err)
	data, err := json.Marshal(&lmr)
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
	ElapsedTime int64             `json:"elapsed_time"`
}

func (w *Worker) logMessageFlowRequest(from *Point, receiver *Point, msg *workflow.CmdTask, elapsedTime int64) {
	m, _ := json.Marshal(msg.Task)
	var mf models.MessageFlow
	mf.SenderID = from.ID
	mf.SenderType = from.Type
	mf.ReceiverID = receiver.ID
	mf.ReceiverType = receiver.Type
	mf.WorkflowID = msg.Task.WorkflowID
	mf.CreatedAt = helper.GetTimeNow()
	mf.Message = string(m)
	mf.Status = msg.Status
	mf.Flow = models.DeliverFlow
	mf.DeliverID = msg.DeliverID
	mf.ElapsedTime = elapsedTime
	e, _ := json.Marshal(msg)
	mf.ResponseSize = len(e)
	id, err := storage.LogMessageFlow(&mf)
	if err != nil {
		fmt.Println("[SAVE TASK REQUEST FLOW ERR]", err)
	}

	lm := LogMessage{
		ID:          id,
		From:        from,
		Receiver:    receiver,
		Msg:         msg,
		Flow:        mf.Flow,
		RequestSize: len(e),
		ElapsedTime: elapsedTime,
	}

	data, err := json.Marshal(&lm)
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

	// fmt.Printf("[SAVE TASK REQUEST FLOW] %+v Err= %+v\n", mf, err)
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
