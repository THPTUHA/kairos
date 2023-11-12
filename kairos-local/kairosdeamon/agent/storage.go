package agent

import (
	"io"
)

type Storage interface {
	SetTask(task *Task) error
	DeleteTask(id string) (*Task, error)
	SetExecution(execution *Execution) (string, error)
	SetExecutionDone(execution *Execution) (bool, error)
	GetTasks(options *TaskOptions) ([]*Task, error)
	GetTask(id string, options *TaskOptions) (*Task, error)
	GetExecutionGroup(execution *Execution, opts *ExecutionOptions) ([]*Execution, error)
	GetExecutions(taskID string, opts *ExecutionOptions) ([]*Execution, error)
	Snapshot(w io.WriteCloser) error
	Shutdown() error
	Restore(r io.ReadCloser) error
}
