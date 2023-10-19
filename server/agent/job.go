package agent

import "errors"

var (
	// ErrParentJobNotFound is returned when the parent job is not found.
	ErrParentJobNotFound = errors.New("specified parent job not found")
	// ErrNoAgent is returned when the job's agent is nil.
	ErrNoAgent = errors.New("no agent defined")
	// ErrSameParent is returned when the job's parent is itself.
	ErrSameParent = errors.New("the job can not have itself as parent")
	// ErrNoParent is returned when the job has no parent.
	ErrNoParent = errors.New("the job doesn't have a parent job set")
	// ErrNoCommand is returned when attempting to store a job that has no command.
	ErrNoCommand = errors.New("unspecified command for job")
	// ErrWrongConcurrency is returned when Concurrency is set to a non existing setting.
	ErrWrongConcurrency = errors.New("invalid concurrency policy value, use \"allow\" or \"forbid\"")
)

// Job describes a scheduled Job.
type Job struct {
	// Job id. Must be unique, it's a copy of name.
	ID string `json:"id"`
	// Job name. Must be unique, acts as the id.
	Name string `json:"name"`
}
