package agent

import (
	"errors"
	"expvar"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/THPTUHA/kairos/pkg/circbuf"
	"github.com/THPTUHA/kairos/pkg/logger"
	"github.com/THPTUHA/kairos/server/plugin"
	"github.com/THPTUHA/kairos/server/plugin/proto"
	"github.com/hashicorp/raft"
	"github.com/hashicorp/serf/serf"
	"github.com/sirupsen/logrus"
	"github.com/soheilhy/cmux"
)

const (
	raftTimeout      = 30 * time.Second
	raftLogCacheSize = 512
	minRaftProtocol  = 3
	// maxBufSize limits how much data we collect from a handler.
	maxBufSize = 256000
)

var (
	expNode = expvar.NewString("node")

	runningExecutions sync.Map
)

type RaftStore interface {
	raft.StableStore
	raft.LogStore
	Close() error
}

// Node is a shorter, more descriptive name for serf.Member
type Node = serf.Member

// Agent is the main struct that represents a dkron agent
type Agent struct {
	// ProcessorPlugins maps processor plugins
	ProcessorPlugins map[string]Processor

	ExecutorPlugins map[string]plugin.Executor

	HTTPTransport Transport

	Store              Storage
	hub                *Hub
	MemberEventHandler func(serf.Event)

	config     *AgentConfig
	sched      *Scheduler
	ready      bool
	shutdownCh chan struct{}

	peers      map[string][]*ServerParts
	localPeers map[raft.ServerAddress]*ServerParts
	peerLock   sync.RWMutex

	activeExecutions sync.Map

	listener net.Listener
	taskCh   chan *TaskEvent

	// logger is the log entry to use fo all logging calls
	logger *logrus.Entry
}

// ProcessorFactory is a function type that creates a new instance
// of a processor.
type ProcessorFactory func() (Processor, error)

type AgentOption func(agent *Agent)

func NewAgent(config *AgentConfig, options ...AgentOption) *Agent {
	agent := &Agent{
		config: config,
		taskCh: make(chan *TaskEvent),
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
	time.Sleep(1000 * time.Second)
	return nil
}

func (a *Agent) Start() error {
	a.config.NodeName = "agent"
	log := logger.InitLogger(a.config.LogLevel, a.config.NodeName)
	a.logger = log

	// Normalize configured addresses
	if err := a.config.normalizeAddrs(); err != nil && !errors.Is(err, ErrResolvingHost) {
		return err
	}

	// Expose the node name
	expNode.Set(a.config.NodeName)
	addr := a.bindRPCAddr()
	l, err := net.Listen("tcp", addr)
	if err != nil {
		a.logger.Fatal(err)
	}
	a.listener = l

	a.StartServer()

	a.ready = true
	go a.handleTaskEvent()
	go a.hub.Run()
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
		switch te.Cmd {
		case CreateTaskCmd:
			a.SetTask(te.Task)
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
		s, err := NewStore(a.logger, false)
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

func filterArray(arr []Node, filterFunc func(Node) bool) []Node {
	for i := len(arr) - 1; i >= 0; i-- {
		if !filterFunc(arr[i]) {
			arr[i] = arr[len(arr)-1]
			arr = arr[:len(arr)-1]
		}
	}
	return arr
}

func (a *Agent) bindRPCAddr() string {
	bindIP, _, _ := a.config.AddrParts(a.config.BindAddr)
	return net.JoinHostPort(bindIP, strconv.Itoa(a.config.Port))
}

func (a *Agent) GetRunningTasks() int {
	task := 0
	runningExecutions.Range(func(k, v interface{}) bool {
		task = task + 1
		return true
	})
	return task
}

func (agent *Agent) SetTask(task *Task) error {

	if err := agent.Store.SetTask(task); err != nil {
		return err
	}
	task.Agent = agent
	if err := agent.sched.AddTask(task); err != nil {
		return err
	}

	return nil
}

func (agent *Agent) SetExecution(execution *Execution) (string, error) {
	return agent.Store.SetExecution(execution)
}

func (agent *Agent) DeleteTask(taskID string) error {
	agent.sched.RemoveTask(taskID)
	return nil
}

type statusAgentHelper struct {
	execution *Execution
}

func (s *statusAgentHelper) Update(b []byte, c bool) (int64, error) {
	s.execution.Output = string(b)
	return 0, nil
}

func (a *Agent) AddHub(h *Hub) {
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
	exc["debug"] = ""
	if jex == "" {
		return errors.New("agent: No executor defined, nothing to do")
	}
	fmt.Printf("excutor %+v", agent.ExecutorPlugins)

	if executor, ok := agent.ExecutorPlugins[jex]; ok {
		agent.logger.WithField("plugin", jex).Debug("agent: calling executor plugin")
		runningExecutions.Store(execution.GetGroup(), execution)
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
	agent.logger.WithField("done task", output.String()).Debug()
	runningExecutions.Delete(execution.GetGroup())

	return nil
}
