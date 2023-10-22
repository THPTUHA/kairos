package agent

import "io"

type Storage interface {
	SetJob(job *Job, copyDependentJobs bool) error
	DeleteJob(name string) (*Job, error)
	SetExecution(execution *Execution) (string, error)
	SetExecutionDone(execution *Execution) (bool, error)
	GetJobs(options *JobOptions) ([]*Job, error)
	GetJob(name string, options *JobOptions) (*Job, error)
	Snapshot(w io.WriteCloser) error
	Shutdown() error
	Restore(r io.ReadCloser) error
}
