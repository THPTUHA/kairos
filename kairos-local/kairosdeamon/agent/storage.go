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
	SetQueue(workflowID, k, v string) error
	GetQueue(workflowID string, value *map[string]string) error
	SetBroker(broker *workflow.Broker) error
	GetBrokers() ([]*workflow.Broker, error)
}
