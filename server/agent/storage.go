package agent

import "io"

type Storage interface {
	SetJob(job *Job, copyDependentJobs bool) error
	GetJobs(options *JobOptions) ([]*Job, error)
	Snapshot(w io.WriteCloser) error
	Shutdown() error
	Restore(r io.ReadCloser) error
}
