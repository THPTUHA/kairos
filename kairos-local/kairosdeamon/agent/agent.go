package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/config"
	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/events"
	"github.com/THPTUHA/kairos/pkg/circbuf"
	"github.com/THPTUHA/kairos/pkg/logger"
	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/THPTUHA/kairos/server/plugin"
	"github.com/THPTUHA/kairos/server/plugin/proto"
	"github.com/sirupsen/logrus"
	"github.com/soheilhy/cmux"
)

const (
	maxBufSize = 256000
)

var (
	runningExecutions sync.Map
)

type Agent struct {
	ProcessorPlugins map[string]Processor

	ExecutorPlugins map[string]plugin.Executor

	HTTPTransport Transport

	Store Storage
	hub   *Hub

	config     *AgentConfig
	sched      *Scheduler
	ready      bool
	shutdownCh chan struct{}

	activeExecutions sync.Map

	listener net.Listener
	taskCh   chan *workflow.CmdTask
	EventCh  chan *events.Event

	logger *logrus.Entry
}

// ProcessorFactory is a function type that creates a new instance
// of a processor.
type ProcessorFactory func() (Processor, error)

type AgentOption func(agent *Agent)

func NewAgent(config *AgentConfig, options ...AgentOption) *Agent {
	agent := &Agent{
		config: config,
		taskCh: make(chan *workflow.CmdTask),
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
			TaskID:     te.Task.ID,
			WorkflowID: te.Task.WorkflowID,
			DeliverID:  te.DeliverID,
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
				fmt.Printf("[AGENT Schedule task] %+v\n", task)
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
			// a.Store.SetExecution()
			go a.hub.Publish(&re)
			m, _ := json.Marshal(te.Task)
			a.Store.SetQueue(fmt.Sprint(te.Task.WorkflowID), te.From, string(m))
			a.Store.SetQueue(fmt.Sprint(te.Task.WorkflowID), workflow.GetTaskName(te.Task.Name), string(m))

			tasks, err := a.Store.GetTasks(&TaskOptions{NoScheduler: true})
			if err != nil {
				fmt.Println("[AGENT GET TASK ERROR]", err)
			}

			for _, task := range tasks {
				fmt.Printf("AGENT RUN TASK NO SCHEDULE %+v\n", task)
				if len(task.Deps) > 0 {
					task.Agent = a
					go task.Run()
				}
			}

			fmt.Printf("[AGENT SET QUEUE] taskname = %s, input=%s, from=%s\n", te.Task.Name, te.Task.Input, te.From)
			// TODO check
		}
	}
}

func (a *Agent) initConnectServer() {
	token, err := a.Store.GetMeta("token")
	if err != nil {
		a.logger.WithField("agent", "init connect server").Error(err)
		return
	}
	fmt.Println("Token-----", token)
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
	if task.Schedule == "" {
		return nil
	}
	task.Agent = agent
	if err := agent.sched.AddTask(task); err != nil {
		return err
	}
	return nil
}

func (agent *Agent) SetTask(task *Task) error {
	if err := agent.Store.SetTask(task); err != nil {
		return err
	}
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
	return agent.Store.GetTask(taskID, nil)
}

func (agent *Agent) DeleteTask(taskID string) error {
	agent.sched.RemoveTask(taskID)
	err := agent.Store.DeleteTask(taskID)
	return err
}

type statusAgentHelper struct {
	execution *Execution
}

func (s *statusAgentHelper) Update(b []byte, c bool) (int64, error) {
	s.execution.Output = string(b)
	return 0, nil
}

func (a *Agent) AddHub(h *Hub) {
	h.AddEventTask(a.taskCh)
	a.hub = h
}

func (agent *Agent) Run(task *Task, execution *Execution) error {
	agent.logger.WithFields(logrus.Fields{
		"task": task.Name,
	}).Info("agent: Starting task")

	output, _ := circbuf.NewBuffer(maxBufSize)

	var success bool

	jex := task.Executor
	exc := task.ExecutorConfig

	execution.StartedAt = time.Now()
	execution.NodeName = agent.config.NodeName
	execution.Id = execution.Key()
	agent.SetExecution(execution)
	exc["debug"] = ""
	if jex == "" {
		return errors.New("agent: No executor defined, nothing to do")
	}
	fmt.Printf("excutor %+v", agent.ExecutorPlugins)

	if executor, ok := agent.ExecutorPlugins[jex]; ok {
		agent.logger.WithField("plugin", jex).Debug("agent: calling executor plugin")
		id, _ := strconv.ParseInt(task.ID, 10, 64)
		out, err := executor.Execute(&proto.ExecuteRequest{
			TaskId: id,
			Config: exc,
		}, &statusAgentHelper{
			execution: execution,
		})

		if err == nil && out.Error != "" {
			err = errors.New(out.Error)
		}
		if err != nil {
			agent.logger.WithError(err).WithField("task", task.Name).WithField("plugin", executor).Error("agent: command error output")
			success = false
			_, _ = output.Write([]byte(err.Error() + "\n"))
		} else {
			success = true
		}

		if out != nil {
			_, _ = output.Write(out.Output)
		}
	} else {
		agent.logger.WithField("executor", jex).Error("agent: Specified executor is not present")
		_, _ = output.Write([]byte("agent: Specified executor is not present"))
	}

	execution.FinishedAt = time.Now()
	execution.Success = success
	execution.Output = output.String()
	_, err := agent.Store.SetExecutionDone(task.ID, execution)
	if err != nil {
		agent.logger.WithField("executor", "set").Error(err)
	}

	t := task.ToCmdReplyTask()
	t.Cmd = workflow.ReplyOutputTaskCmd
	t.Result = execution.GetResult()
	go agent.hub.Publish(t)
	fmt.Printf("[ OUTPUT TASK ] %+v  RESULT = %+v\n", t, t.Result)
	return nil
}
