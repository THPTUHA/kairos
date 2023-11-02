package agent

import (
	"io"

	"github.com/THPTUHA/kairos/pkg/workflow"
)

type Storage interface {
	SetTask(task *Task) error
	DeleteTask(id int64) (*Task, error)
	SetExecution(execution *Execution) (string, error)
	SetExecutionDone(execution *Execution) (bool, error)
	GetTasks(options *TaskOptions) ([]*Task, error)
	GetTask(id int64, options *TaskOptions) (*Task, error)
	GetExecutionGroup(execution *Execution, opts *ExecutionOptions) ([]*Execution, error)
	GetExecutions(taskID int64, opts *ExecutionOptions) ([]*Execution, error)
	Snapshot(w io.WriteCloser) error
	Shutdown() error
	Restore(r io.ReadCloser) error
}

type DeamonStorage interface {
	SetTask(task *Task) error
	DeleteTask(id int64) (*Task, error)
	SetExecution(execution *Execution) (string, error)
	SetExecutionDone(execution *Execution) (bool, error)
	GetTasks(options *TaskOptions) ([]*Task, error)
	GetTask(id int64, options *TaskOptions) (*Task, error)
	GetExecutionGroup(execution *Execution, opts *ExecutionOptions) ([]*Execution, error)
	Snapshot(w io.WriteCloser) error
	Shutdown() error
	Restore(r io.ReadCloser) error
	GetCollections(options *CollectionOptions) ([]*workflow.Collection, error)
	SetCollection(collection *workflow.Collection) error
	GetWorkflows(options *WorkflowOptions) ([]*workflow.Workflow, error)
}
