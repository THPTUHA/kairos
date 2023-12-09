package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/agent/broker"
	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/config"
	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/events"
	"github.com/THPTUHA/kairos/pkg/bufcb"
	"github.com/THPTUHA/kairos/pkg/circbuf"
	"github.com/THPTUHA/kairos/pkg/logger"
	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/THPTUHA/kairos/server/plugin"
	"github.com/THPTUHA/kairos/server/plugin/proto"
	"github.com/sirupsen/logrus"
	"github.com/soheilhy/cmux"
)

const (
	maxBufSize = 1000
)

var (
	runningExecutions sync.Map
)

type Agent struct {
	ExecutorPlugins  map[string]plugin.Executor
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
	RunCount         int64
	Broker           *broker.Broker

	logger *logrus.Entry
}

type ProcessorFactory func() (Processor, error)

type AgentOption func(agent *Agent)

func NewAgent(config *AgentConfig, options ...AgentOption) *Agent {
	agent := &Agent{
		config: config,
		taskCh: make(chan *workflow.CmdTask),
		Broker: broker.NewBroker(),
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
	a.initConnectServer()
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
			TaskID:    te.Task.ID,
			DeliverID: te.DeliverID,
			Channel:   te.Channel,
		}
		if te.Task != nil {
			re.WorkflowID = te.Task.WorkflowID
			re.TaskID = te.Task.ID
		} else {
			re.WorkflowID = te.WorkflowID
		}

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
			go a.hub.Publish(&re)

			// a.Store.SetQueue(fmt.Sprint(te.Task.WorkflowID), te.From, te.Task.Input)
			// a.Store.SetQueue(fmt.Sprint(te.Task.WorkflowID), workflow.GetTaskName(te.Task.Name), te.Task.Input)

			// tasks, err := a.Store.GetTasks(&TaskOptions{NoScheduler: true})
			// if err != nil {
			// 	fmt.Println("[AGENT GET TASK ERROR]", err)
			// }

			// for _, task := range tasks {
			// 	fmt.Printf("AGENT RUN TASK NO SCHEDULE %+v\n", task)
			// 	task.Agent = a
			// 	go task.Run()
			// }
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
			// TODO check
		case workflow.RequestTaskRunSyncCmd:
			re.Cmd = workflow.ReplyRequestTaskSyncCmd
			fmt.Printf("[AGENT Request Task run sync ] %+v\n", te)
			a.Store.SetQueue(fmt.Sprint(te.Task.WorkflowID), te.From, te.Task.Input)
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
			re.Cmd = workflow.ReplySetBrokerCmd
			if te.Broker == nil {
				re.Status = workflow.FaultSetBroker
				re.Message = fmt.Sprintf("Empty broker")
			} else {
				// te.Broker.Template.FuncNotFound
				err := a.Store.SetBroker(te.Broker)
				if err != nil {
					re.Status = workflow.FaultSetBroker
					re.Message = err.Error()
				}
			}
			a.hub.Publish(&re)
		}
	}
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
	a.hub.HandleConnectServer(&config.Auth{
		Token:      token,
		ClientName: clientName,
		ClientID:   clientID,
		UserID:     userID,
	})
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
		a.scheduleTasks()
	}()
	go func() {
		if err := tcpm.Serve(); err != nil {
			a.logger.Fatal(err)
		}
	}()
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
	var err error
	if s.buffer != nil {
		_, err = s.buffer.Write(b)

	} else if s.circbuf != nil {
		_, err = s.circbuf.Write(b)
	}
	s.execution.Success = c
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

func (agent *Agent) Run(task *Task, execution *Execution, re *workflow.CmdTask) error {
	agent.logger.WithFields(logrus.Fields{
		"task": task.Name,
	}).Info("agent: Starting task")

	execution.RunCount = atomic.AddInt64(&agent.RunCount, 1)

	jex := task.Executor
	exc := task.ExecutorConfig

	if re != nil && exc != nil {
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
		return errors.New("agent: No executor defined, nothing to do")
	}

	if executor, ok := agent.ExecutorPlugins[jex]; ok {
		agent.logger.WithField("plugin", jex).Debug("agent: calling executor plugin")
		id, _ := strconv.ParseInt(task.ID, 10, 64)
		helper := &statusAgentHelper{
			agent:     agent,
			execution: execution,
			task:      task,
			input:     make(chan []byte),
			buffer: bufcb.NewBuffer(maxBufSize, func(b []byte) error {
				execution.FinishedAt = time.Now()
				execution.Output = string(b)
				execution.Offset++
				fmt.Println(" BUFFER OUT---", string(b))
				err := agent.SaveExecutorResult(execution)
				if err != nil {
					agent.logger.WithField("executor", "set").Error(err)
					return err
				}

				t := task.ToCmdReplyTask()
				t.Cmd = workflow.ReplyOutputTaskCmd
				t.Result = execution.GetResult()
				go agent.hub.Publish(t)

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
		go agent.hub.Publish(t)
	}

	return nil
}

func (agent *Agent) RunSync(task *Task, execution *Execution, re *workflow.CmdTask) (*workflow.Result, error) {
	agent.logger.WithFields(logrus.Fields{
		"task": task.Name,
	}).Info("agent: Starting task")

	execution.RunCount = atomic.AddInt64(&agent.RunCount, 1)
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

	if executor, ok := agent.ExecutorPlugins[jex]; ok {
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
		fmt.Println(" BUFFER OUT SYNC---", execution.Output)
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
