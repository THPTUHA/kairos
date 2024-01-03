package agent

import "github.com/THPTUHA/kairos/pkg/workflow"

type Storage interface {
	SetMeta(key, value string) error
	GetMeta(key string) (string, error)
	SetTask(task *Task) error
	DeleteTask(id string) error
	SetExecution(taskID string, execution *Execution) error
	SetExecutionDone(taskID string, execution *Execution) (bool, error)
	GetTasks(options *TaskOptions) ([]*Task, error)
	GetTask(id string, options *TaskOptions) (*Task, error)
	GetExecutions(taskID string, opts *ExecutionOptions) ([]*Execution, error)
	Shutdown() error
	SetQueue(k, v string) error
	GetQueue(k string) (string, error)
	SetBroker(broker *workflow.Broker) error
	GetBrokers() ([]*workflow.Broker, error)
	GetBroker(id string) (*workflow.Broker, error)
	DeleteBroker(id string) error
	SetBrokerRecord(br *BrokerRecordLocal) error
	GetBrokerRecord(options *BrOptions) ([]*BrokerRecordLocal, error)
	DeleteWorkflow(id string) error
	GetWorkflow(id string) (*WorkflowInfo, error)

	SetScript(name, script string) error
	GetScript(name string) (string, error)
	GetScripts() ([]string, error)
	DeleteScript(name string) error
	ListScript() ([]string, error)
}
