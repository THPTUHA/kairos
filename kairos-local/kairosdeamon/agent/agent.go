package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/agent/broker"
	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/config"
	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/events"
	"github.com/THPTUHA/kairos/pkg/bufcb"
	"github.com/THPTUHA/kairos/pkg/circbuf"
	"github.com/THPTUHA/kairos/pkg/helper"
	"github.com/THPTUHA/kairos/pkg/logger"
	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/THPTUHA/kairos/server/plugin"
	"github.com/THPTUHA/kairos/server/plugin/proto"
	"github.com/THPTUHA/kairos/server/storage/models"
	"github.com/dop251/goja"
	"github.com/sirupsen/logrus"
	"github.com/soheilhy/cmux"
)

const (
	maxBufSize = 256000
)

var (
	runningExecutions sync.Map
)

type TaskActive struct {
	ExecutionID string
	Status      string
	StartAt     int64
}

const (
	Running  = "running"
	Schedule = "schedule"
	Finish   = "finish"
	Wait     = "wait"
)

type Agent struct {
	ExecutorPlugins  Plugins
	HTTPTransport    Transport
	Store            Storage
	hub              *Hub
	config           *AgentConfig
	sched            *Scheduler
	ready            bool
	shutdownCh       chan struct{}
	activeExecutions sync.Map
	listener         net.Listener
	taskCh           chan *workflow.CmdTask
	EventCh          chan *events.Event
	Broker           *broker.Broker
	Script           *workflow.FuncCall
	ClientName       string
	BrokerWfs        map[int64]*broker.Subscriber
	TaskActives      map[string]map[string]*TaskActive

	rmu    sync.RWMutex
	mu     sync.RWMutex
	logger *logrus.Entry
}

type AgentOption func(agent *Agent)

func NewAgent(config *AgentConfig, options ...AgentOption) *Agent {
	vm := goja.New()

	agent := &Agent{
		config:    config,
		taskCh:    make(chan *workflow.CmdTask),
		Broker:    broker.NewBroker(),
		BrokerWfs: make(map[int64]*broker.Subscriber),
		Script: &workflow.FuncCall{
			Call:  make(map[string]goja.Callable),
			Funcs: vm,
		},
		TaskActives: make(map[string]map[string]*TaskActive),
	}

	for _, option := range options {
		option(agent)
	}

	return agent
}

func (a *Agent) scheduleTasks() error {
	a.logger.Debug("agent: Starting scheduler")
	tasks, err := a.Store.GetTasks(nil)
	if err != nil {
		return err
	}

	a.sched.Start(tasks, a)
	return nil
}

func (a *Agent) setupBrokers() error {
	brokers, err := a.Store.GetBrokers()

	if err != nil {
		return err
	}
	for _, b := range brokers {
		b.Template.FuncCalls = a.Script
		a.setupBroker(b)
	}
	return nil
}

func (a *Agent) Start() error {
	a.config.NodeName = "agent"
	log := logger.InitLogger(a.config.LogLevel, a.config.NodeName)
	a.logger = log

	if err := a.config.normalizeAddrs(); err != nil && !errors.Is(err, ErrResolvingHost) {
		return err
	}

	addr := a.config.BindAddr
	l, err := net.Listen("tcp", addr)
	if err != nil {
		a.logger.Fatal(err)
	}
	a.listener = l

	a.StartServer()

	a.ready = true
	go a.handleEvent()
	go a.handleTaskEvent()
	a.setupScripts()
	a.initConnectServer()
	return nil
}

func (a *Agent) addPlugin(pluginName, cmdName string, args []string) error {
	fmt.Printf("name= %s, cmd=%s, args=%+v\n", pluginName, cmdName, args)
	_, err := a.ExecutorPlugins.PluginFactory(exec.Command(cmdName, args...), pluginName, plugin.ExecutorPluginName)
	return err
}

func (a *Agent) setupScripts() error {
	scripts, err := a.Store.GetScripts()
	if err != nil {
		a.logger.Error(err)
		return err
	}
	vm := goja.New()
	var script *workflow.FuncCall
	if a.Script != nil {
		script = a.Script
	} else {
		script = &workflow.FuncCall{
			Call:  make(map[string]goja.Callable),
			Funcs: vm,
		}
	}

	for _, s := range scripts {
		prog, err := goja.Compile("", s, true)
		if err != nil {
			a.logger.Error(err)
			return err
		}
		_, err = script.Funcs.RunProgram(prog)
		if err != nil {
			a.logger.Error(err)
			return err
		}
	}
	a.Script = script
	return nil
}

func (a *Agent) Stop() error {
	a.logger.Info("agent: Called stop, now stopping")

	if a.sched.Started() {
		<-a.sched.Stop().Done()
	}

	if err := a.Store.Shutdown(); err != nil {
		return err
	}

	return nil
}

func (a *Agent) handleTaskEvent() {
	for te := range a.taskCh {
		re := workflow.CmdReplyTask{
			DeliverID:  te.DeliverID,
			Group:      te.Group,
			Channel:    te.Channel,
			FinishPart: true,
			Part:       te.Part,
			Parent:     te.Parent,
		}
		// TODO save te

		if te.Task != nil {
			re.WorkflowID = te.Task.WorkflowID
			re.TaskID = te.Task.ID
		} else {
			re.WorkflowID = te.WorkflowID
		}

		a.logger.Infof("CMD = %d", te.Cmd)
		switch te.Cmd {
		case workflow.SetTaskCmd:
			fmt.Printf("[AGENT HANLE SetTaskCmd ] %+v\n", te)
			re.Cmd = workflow.ReplySetTaskCmd
			var task Task
			err := task.Setup(te.Task)
			if err != nil {
				a.logger.Error(err)
				re.Message = err.Error()
				re.Status = workflow.FaultSetTask
				go a.hub.Publish(&re)
			} else {
				err = a.SetTask(&task)
				if err != nil {
					a.logger.Error(err)
					re.Message = err.Error()
					re.Status = workflow.FaultSetTask
				}
				re.Status = workflow.SuccessSetTask
				go a.hub.Publish(&re)
			}

		case workflow.TriggerStartTaskCmd:
			fmt.Printf("[AGENT HANLE Trigger Start TaskCmd ] cmd=%d taskid=%d\n", te.Cmd, te.Task.ID)
			re.Cmd = workflow.ReplyStartTaskCmd
			task, err := a.GetTask(fmt.Sprint(te.Task.ID))
			if err != nil {
				re.Message = err.Error()
				re.Status = workflow.FaultTriggerTask
			} else {
				err = a.ScheduleTask(task)
				a.logger.Debug(fmt.Sprintf("Scheduler task %+v", task))
				if err != nil {
					re.Message = err.Error()
					re.Status = workflow.FaultTriggerTask
				}
			}
			re.Status = workflow.SuccessSetTask
			go a.hub.Publish(&re)
		case workflow.InputTaskCmd:
			fmt.Printf("[AGENT HANLE Input TaskCmd ] %+v\n", te)
			re.Cmd = workflow.ReplyInputTaskCmd
			re.Status = workflow.SuccessReceiveInputTaskCmd
			re.TaskID = te.Task.ID
			go a.hub.Publish(&re)
			task, err := a.Store.GetTask(fmt.Sprint(te.Task.ID), nil)
			if err != nil {
				fmt.Println("[AGENT GET TASK ERROR]", err)
			}
			if task.ID == "" {
				a.logger.Errorf("Task id = %d not found \n", te.Task.ID)
				continue
			}
			task.Agent = a
			task.logger = a.logger
			go task.RunTrigger(te)
			fmt.Printf("[AGENT SET QUEUE] taskname = %s, input=%s, from=%s\n", te.Task.Name, te.Task.Input, te.From)

		case workflow.RequestTaskRunSyncCmd:
			re.Cmd = workflow.ReplyRequestTaskSyncCmd
			fmt.Printf("[AGENT Request Task run sync ] %+v\n", te)
			// a.Store.SetQueue(fmt.Sprint(te.Task.WorkflowID), te.From, te.Task.Input)
			task, err := a.GetTask(fmt.Sprint(te.Task.ID))
			if err != nil {
				re.Result = &workflow.Result{
					Success: false,
					Output:  err.Error(),
				}
			}

			task.Agent = a
			result, err := task.RunSync(te)
			fmt.Printf("TASK RESULT-- %+v ERR = %+v\n", result, err)
			if err != nil {
				re.Result = &workflow.Result{
					Success: false,
					Output:  err.Error(),
				}
			} else {
				re.Result = result
			}

			a.hub.Publish(&re)
		case workflow.SetBrokerCmd:
			fmt.Printf("[AGENT Request Set Broker] %+v\n", te)
			re.Cmd = workflow.ReplySetBrokerCmd
			re.Status = workflow.SuccessSetBroker
			if te.Broker == nil {
				re.Status = workflow.FaultSetBroker
				re.Message = fmt.Sprintf("empty broker")
			} else {
				re.BrokerID = te.Broker.ID
				if err := a.Store.SetBroker(te.Broker); err != nil {
					re.Status = workflow.FaultSetBroker
					re.Message = err.Error()
				} else {
					valid := true
					if te.Broker.Template == nil {
						re.Status = workflow.FaultSetBroker
						re.Message = fmt.Sprintf("template empty")
						a.hub.Publish(&re)
						valid = false
					} else {
						te.Broker.Template.FuncCalls = a.Script
						Funcs := te.Broker.Template.FuncNotFound

						if len(Funcs) > 0 {
							for _, f := range Funcs {
								e := a.Script.Call[f]
								if e == nil {
									var m goja.Callable
									fc := a.Script.Funcs.Get(f)
									if fc == nil {
										re.Status = workflow.FaultSetBroker
										re.Message = fmt.Sprintf("function %s not found in broker %s on client %s", f, te.Broker.Name, a.ClientName)
										a.hub.Publish(&re)
										valid = false
										break
									}
									err = a.Script.Funcs.ExportTo(fc, &m)
									if err != nil {
										re.Status = workflow.FaultSetBroker
										re.Message = fmt.Sprintf("function %s not found in broker %s on client %s", f, te.Broker.Name, a.ClientName)
										a.hub.Publish(&re)
										valid = false
										break
									}
									a.Script.Call[f] = m
								}
							}
						}
					}
					if valid {
						te.Broker.Template.FuncCalls = a.Script
						go a.setupBroker(te.Broker)
					}
				}
				a.hub.Publish(&re)
			}
		case workflow.RequestDestroyWf:
			re.Cmd = workflow.ReplyDestroyWf
			re.Status = workflow.SuccessDestroyWorkflow
			err := a.DeleteWorkflow(te.WorkflowID)
			if err != nil {
				a.logger.Error(err)
				re.Status = workflow.FaultDestroyWorkflow
				re.Message = err.Error()
			}
			a.hub.Publish(&re)
		}
	}
}

func (a *Agent) DeleteWorkflow(wid int64) error {
	wfi, err := a.Store.GetWorkflow(fmt.Sprint(wid))
	if err != nil {
		return err
	}
	a.mu.Lock()
	for _, bid := range wfi.BrokerIDs {
		sub := a.BrokerWfs[bid]
		if sub != nil {
			a.Broker.Detach(sub)
		}
	}

	for _, tid := range wfi.TaskIDs {
		a.DeleteTask(fmt.Sprint(tid))
	}

	a.Store.DeleteWorkflow(fmt.Sprint(wid))
	defer a.mu.Unlock()
	return err
}

func (a *Agent) initConnectServer() {
	token, err := a.Store.GetMeta("token")
	if err != nil {
		a.logger.WithField("agent", "init connect server").Error(err)
		return
	}
	clientName, err := a.Store.GetMeta("clientname")
	clientID, err := a.Store.GetMeta("client_id")
	userID, err := a.Store.GetMeta("user_id")

	if a.hub.IsConnect() {
		a.hub.Disconnect()
	}
	if token == "" {
		a.logger.Warn("Please login to use it!")
		return
	}
	a.ClientName = clientName
	a.hub.HandleConnectServer(&config.Auth{
		Token:      token,
		ClientName: clientName,
		ClientID:   clientID,
		UserID:     userID,
	})
}

func (a *Agent) setupBroker(b *workflow.Broker) error {
	sub, err := broker.NewSubscriber()
	if err != nil {
		return err
	}
	a.BrokerWfs[b.ID] = sub

	for _, l := range b.Listens {
		a.Broker.Subscribe(sub, l)
		go sub.HandleMessage(func(v interface{}) {
			fmt.Printf("BrokerMessage bid=%d wid=%d value = %+v \n", b.ID, b.WorkflowID, v)
			if output, ok := v.(map[string]workflow.ReplyData); ok {
				if output[l]["workflow_id"] != b.WorkflowID {
					return
				}
				fmt.Printf("BROKER handle output = %+v brokerid = %d listen = %s\n", output[l], b.ID, l)
				tr := b.Template.NewRutime(output)
				exo := tr.Execute()
				for _, d := range exo.DeliverFlows {
					fmt.Printf("DELIVERFLOWS %+v\n", d)
					// reciever is task
					tasks, err := a.Store.GetTasks(&TaskOptions{
						Name:       workflow.GetRawName(d.Reciever),
						WorkflowID: b.WorkflowID,
					})
					if err != nil || len(tasks) == 0 {
						return
					}
					t := tasks[0]
					input, _ := json.Marshal(d.Msg)
					var re workflow.CmdTask
					re.Task = &workflow.Task{
						Input: string(input),
					}
					ex := NewExecution(t.ID, t.GetGroup())
					go a.Run(t, ex, &re)
				}
				input, err := json.Marshal(output)
				if err != nil {
					return
				}
				output, err := json.Marshal(exo)
				if err != nil {
					return
				}
				var status int
				if exo.Tracking.Err == "" {
					status = workflow.BrokerExecuteSuccess
				} else {
					status = workflow.BrokerExecuteFault
				}
				a.Store.SetBrokerRecord(&BrokerRecordLocal{
					BrokerRecord: models.BrokerRecord{
						Input:     string(input),
						Output:    string(output),
						BrokerID:  b.ID,
						CreatedAt: helper.GetTimeNow(),
						Status:    status,
					},
				})
				fmt.Printf("EXO--- %+v \n", exo)
			}
		})
	}
	return nil
}

func (a *Agent) handleEvent() {
	for {
		select {
		case e := <-a.EventCh:
			switch e.Cmd {
			case events.ConnectServerCmd:
				var auth config.Auth
				err := json.Unmarshal([]byte(e.Payload), &auth)
				if err != nil {
					a.logger.WithField("agent", "connect server").Error(err)
					return
				}
				a.Store.SetMeta("token", auth.Token)
				a.Store.SetMeta("clientname", auth.ClientName)
				a.Store.SetMeta("client_id", auth.ClientID)
				a.Store.SetMeta("user_id", auth.UserID)

				err = a.hub.HandleConnectServer(&auth)
				if err != nil {
					a.logger.WithField("agent", "connect server").Error(err)
					return
				}
			}
		}
	}
}

func (a *Agent) AgentConfig() *AgentConfig {
	return a.config
}

func (a *Agent) SetConfig(c *AgentConfig) {
	a.config = c
}

func (a *Agent) StartServer() {
	if a.Store == nil {
		s, err := NewStore(a.config.DataDir, a.logger, false)
		if err != nil {
			a.logger.WithError(err).Fatal("agent: Error initializing store")
		}
		a.Store = s
	}

	a.sched = NewScheduler(a.logger)

	if a.HTTPTransport == nil {
		a.HTTPTransport = NewTransport(a, a.logger)
	}
	a.HTTPTransport.ServeHTTP()

	tcpm := cmux.New(a.listener)
	go func() {
		a.setupBrokers()
		a.scheduleTasks()
	}()
	go func() {
		if err := tcpm.Serve(); err != nil {
			a.logger.Fatal(err)
		}
	}()
}

func (a *Agent) SetupScript(name, script string) error {
	prog, err := goja.Compile("", script, true)
	if err != nil {
		return err
	}
	_, err = a.Script.Funcs.RunProgram(prog)
	if err != nil {
		return err
	}

	err = a.Store.SetScript(name, script)
	if err != nil {
		return err
	}
	return err
}

func (a *Agent) dropScript(name string) error {
	err := a.Store.DeleteScript(name)
	if err != nil {
		return err
	}
	return a.setupScripts()
}

func (a *Agent) GetRunningTasks() int {
	task := 0
	runningExecutions.Range(func(k, v interface{}) bool {
		task = task + 1
		return true
	})
	return task
}

func (agent *Agent) ScheduleTask(task *Task) error {
	task.Agent = agent
	if task.Schedule == "" {
		if task.Wait == workflow.TaskWaitInput {
			return nil
		}
		go func() {
			task.Run()
		}()
	} else {
		if err := agent.sched.AddTask(task); err != nil {
			return err
		}
	}
	return nil
}

func (agent *Agent) SetTask(task *Task) error {
	if err := agent.Store.SetTask(task); err != nil {
		return err
	}
	return nil
}

func (agent *Agent) SetTaskAndRun(task *Task) error {

	if err := agent.Store.SetTask(task); err != nil {
		return err
	}
	task.Agent = agent
	task.logger = agent.logger
	go task.Run()

	return nil
}

func (agent *Agent) SetTaskAndSched(task *Task) error {

	if err := agent.Store.SetTask(task); err != nil {
		return err
	}
	task.Agent = agent
	if err := agent.sched.AddTask(task); err != nil {
		return err
	}

	return nil
}

func (agent *Agent) SetExecution(execution *Execution) error {
	return agent.Store.SetExecution(execution.TaskID, execution)
}

func (agent *Agent) GetTask(taskID string) (*Task, error) {
	t, err := agent.Store.GetTask(taskID, nil)
	return t, err
}

func (agent *Agent) DeleteTask(taskID string) error {
	agent.sched.RemoveTask(taskID)
	err := agent.Store.DeleteTask(taskID)
	return err
}

type statusAgentHelper struct {
	agent     *Agent
	execution *Execution
	task      *Task
	input     chan []byte
	buffer    *bufcb.Buffer
	circbuf   *circbuf.Buffer
}

func (s *statusAgentHelper) Update(b []byte, c bool) (int64, error) {
	fmt.Println("UDPATE DATA", string(b))
	var err error
	s.execution.Success = c
	if s.buffer != nil {
		_, err = s.buffer.Write(b)

	} else if s.circbuf != nil {
		_, err = s.circbuf.Write(b)
	}
	// time.Sleep(3 * time.Second)
	return 0, err
}

func (s *statusAgentHelper) Input() []byte {
	return <-s.input
}

func (s *statusAgentHelper) Send(data []byte) {
	s.input <- data
}

func (a *Agent) AddHub(h *Hub) {
	h.AddEventTask(a.taskCh)
	a.hub = h
}

func (a *Agent) Broadcast(t *workflow.CmdReplyTask) {
	go a.hub.Publish(t)
	replies := make(map[string]workflow.ReplyData)
	k := workflow.GetTaskName(t.TaskName)
	replies[k] = workflow.ReplyData{}
	replies[k]["workflow_id"] = t.WorkflowID
	if t.Result != nil {
		replies[k]["result"] = t.Result
	}

	go a.Broker.Broadcast(replies, k)
}

func (a *Agent) GetTasks(to *TaskOptions) ([]*Task, error) {
	tasks, err := a.Store.GetTasks(to)
	if err != nil {
		return nil, err
	}
	for _, t := range tasks {
		if a.TaskActives[t.ID] != nil && len(a.TaskActives[t.ID]) > 0 {
			t.Status = Running
		} else {
			t.Status = Wait
		}
	}

	return tasks, nil
}

func (a *Agent) UpdateTaskRecord(taskID string, ta *TaskActive) {
	a.rmu.RLock()
	defer a.rmu.RUnlock()
	fmt.Printf("Task id = %s, status = %s", taskID, ta.Status)
	if a.TaskActives[taskID] == nil {
		if ta.Status == Running || ta.Status == Schedule {
			a.TaskActives[taskID] = map[string]*TaskActive{ta.ExecutionID: ta}
		}
	} else {
		if ta.Status == Finish {
			delete(a.TaskActives[taskID], ta.ExecutionID)
		}
	}
}

func (agent *Agent) Run(task *Task, execution *Execution, re *workflow.CmdTask) error {
	agent.logger.WithFields(logrus.Fields{
		"task": task.Name,
	}).Info("agent: Starting task")
	agent.UpdateTaskRecord(task.ID, &TaskActive{
		Status:      Running,
		ExecutionID: execution.Id,
		StartAt:     helper.GetTimeNow(),
	})

	defer agent.UpdateTaskRecord(task.ID, &TaskActive{
		Status:      Finish,
		ExecutionID: execution.Id,
		StartAt:     helper.GetTimeNow(),
	})

	jex := task.Executor
	exc := make(map[string]string)

	for k, v := range task.ExecutorConfig {
		exc[k] = v
	}
	var input string
	if re != nil && exc != nil {
		if task.Schedule != "" {
			s, err := json.Marshal(re)
			if err != nil {
				return err
			}
			fmt.Println("SET QUEUE ", fmt.Sprintf("%s-%d", task.Name, task.WorkflowID))
			return agent.Store.SetQueue(fmt.Sprintf("%s-%d", task.Name, task.WorkflowID), string(s))
		}
		input = re.Task.Input
		// exc["deliver_id"] = fmt.Sprint(re.DeliverID)
		exc["worklow_id"] = fmt.Sprint(re.WorkflowID)
	} else if exc != nil {
		var ct workflow.CmdTask
		e, err := agent.Store.GetQueue(fmt.Sprintf("%s-%d", task.Name, task.WorkflowID))
		if err != nil {
			return err
		}
		if e != "" {
			err = json.Unmarshal([]byte(e), &ct)
			if err != nil {
				return err
			}
			input = ct.Task.Input
			fmt.Println("Get queue from ", fmt.Sprintf("%s-%d", task.Name, task.WorkflowID))
		}
	}

	if input != "" {
		var mp map[string]interface{}
		err := json.Unmarshal([]byte(input), &mp)
		if err != nil {
			return err
		}
		for k, v := range mp {
			if v, ok := v.(string); ok {
				exc[k] = v
			} else {
				str, err := json.Marshal(v)
				if err != nil {
					return err
				}
				exc[k] = string(str)
			}
		}
	}

	execution.StartedAt = time.Now()
	execution.NodeName = agent.config.NodeName
	execution.Id = execution.Key()
	agent.SetExecution(execution)

	exc["debug"] = ""

	if jex == "" {
		return errors.New("agent: No executor defined, nothing to do")
	}
	for k, _ := range agent.ExecutorPlugins.Executors {
		fmt.Printf("EXECUTOR:  %s \n", k)
	}

	if executor, ok := agent.ExecutorPlugins.Executors[jex]; ok {
		agent.logger.WithField("plugin", jex).Debug("agent: calling executor plugin")
		id, _ := strconv.ParseInt(task.ID, 10, 64)
		helper := &statusAgentHelper{
			agent:     agent,
			execution: execution,
			task:      task,
			input:     make(chan []byte),
			buffer: bufcb.NewBuffer(maxBufSize, func(b []byte) error {
				execution.Output = strings.TrimRight(string(b), "\u0000")
				t := task.ToCmdReplyTask()
				if execution.Offset == 0 {
					t.BeginPart = true
					execution.Part = task.GetPart()
					if re != nil {
						execution.Parent = re.Part
					}
				}
				execution.Offset++
				fmt.Println("BUFFER OUT : ", string(b))
				err := agent.SaveExecutorResult(execution)
				if err != nil {
					agent.logger.WithField("executor", "set").Error(err)
					return err
				}

				t.Cmd = workflow.ReplyOutputTaskCmd
				t.Result = execution.GetResult()
				t.Part = execution.Part
				t.Parent = execution.Parent
				if re != nil {
					t.Group = re.Group
					t.RunCount = re.RunCount
				}
				time.Sleep(time.Second)
				go agent.Broadcast(t)

				fmt.Printf("[ OUTPUT TASK ] %+v  RESULT = %+v\n", t, t.Result)
				return nil
			}),
		}

		out, err := executor.Execute(&proto.ExecuteRequest{
			TaskId: id,
			Config: exc,
		}, helper)

		if err == nil && out.Error != "" {
			err = errors.New(out.Error)
		}
		execution.FinishedAt = time.Now()
		fmt.Println("FINISH TASK----", out)
		if err != nil {
			agent.logger.WithError(err).WithField("task", task.Name).WithField("plugin", executor).Error("agent: command error output")
			execution.Success = false
			_, _ = helper.buffer.Write([]byte(err.Error() + "\n"))
		} else {
			execution.Success = true
		}

		if out != nil {
			_, _ = helper.buffer.Write(out.Output)
			helper.buffer.Flush()
		}

	} else {
		err := fmt.Errorf("deamon: specified executor is not present")
		execution.Success = false
		execution.Output = err.Error()
		execution.FinishedAt = time.Now()
		agent.logger.WithField("executor", jex).Error(err)
		execution.Offset++
		err = agent.SaveExecutorResult(execution)
		if err != nil {
			agent.logger.WithField("executor", "set").Error(err)
			return err
		}

		t := task.ToCmdReplyTask()
		t.Cmd = workflow.ReplyOutputTaskCmd
		t.Result = execution.GetResult()
		if re != nil {
			t.RunCount = re.RunCount
		}
		go agent.Broadcast(t)
	}

	return nil
}

type PluginStatus struct {
	Name   string `json:"name"`
	Status int    `json:"status"`
}

func (a *Agent) ListPlugin() []*PluginStatus {
	ps := make([]*PluginStatus, 0)

	for n, c := range a.ExecutorPlugins.ClientProtocols {
		err := (*c).Ping()
		if err != nil {
			a.logger.Error(err)
			ps = append(ps, &PluginStatus{
				Name:   n,
				Status: 0,
			})
		} else {
			ps = append(ps, &PluginStatus{
				Name:   n,
				Status: 1,
			})
		}
	}
	return ps
}

func (a *Agent) DeletePlugin(name string) error {
	c := a.ExecutorPlugins.Clients[name]
	if c != nil {
		(*c).Kill()
		delete(a.ExecutorPlugins.Clients, name)
		delete(a.ExecutorPlugins.Executors, name)
		delete(a.ExecutorPlugins.ClientProtocols, name)
	}

	return nil
}

func (agent *Agent) RunSync(task *Task, execution *Execution, re *workflow.CmdTask) (*workflow.Result, error) {
	agent.logger.WithFields(logrus.Fields{
		"task": task.Name,
	}).Info("agent: Starting task")

	jex := task.Executor
	exc := task.ExecutorConfig

	if re != nil {
		exc["input"] = re.Task.Input
		exc["deliver_id"] = fmt.Sprint(re.DeliverID)
		exc["worklow_id"] = fmt.Sprint(re.WorkflowID)
	}

	execution.StartedAt = time.Now()
	execution.NodeName = agent.config.NodeName
	execution.Id = execution.Key()
	agent.SetExecution(execution)

	exc["debug"] = ""

	if jex == "" {
		return nil, errors.New("agent: No executor defined, nothing to do")
	}
	fmt.Printf("excutor %+v", agent.ExecutorPlugins)

	if executor, ok := agent.ExecutorPlugins.Executors[jex]; ok {
		agent.logger.WithField("plugin", jex).Debug("agent: calling executor plugin")
		id, _ := strconv.ParseInt(task.ID, 10, 64)
		helper := &statusAgentHelper{
			agent:     agent,
			execution: execution,
			task:      task,
			input:     make(chan []byte),
			circbuf:   circbuf.NewBuffer(maxBufSize),
		}

		out, err := executor.Execute(&proto.ExecuteRequest{
			TaskId: id,
			Config: exc,
		}, helper)

		if err == nil && out.Error != "" {
			err = errors.New(out.Error)
		}
		if err != nil {
			agent.logger.WithError(err).WithField("task", task.Name).WithField("plugin", executor).Error("agent: command error output")
			execution.Success = false
			helper.circbuf.Write([]byte(err.Error()))
			return execution.GetResult(), err
		} else {
			execution.Success = true
		}

		if out != nil {
			_, err = helper.circbuf.Write(out.Output)
			if err != nil {
				return nil, err
			}
		}

		execution.FinishedAt = time.Now()
		execution.Output = helper.circbuf.String()
		execution.Offset++
		err = agent.SaveExecutorResult(execution)
		if err != nil {
			agent.logger.WithField("executor", "set").Error(err)
			return nil, err
		}

		return execution.GetResult(), nil
	} else {
		agent.logger.WithField("executor", jex).Error("agent: Specified executor is not present")
	}

	fmt.Println("FINSH EXECUTOR SYNC")
	return nil, fmt.Errorf("deamon: specified executor is not present")
}

func (a *Agent) SaveExecutorResult(e *Execution) error {
	_, err := a.Store.SetExecutionDone(e.TaskID, e)
	return err
}
