package agent

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
	SetQueue(taskID, k, v string) error
	GetQueue(taskID string, value *map[string]string) error
}
