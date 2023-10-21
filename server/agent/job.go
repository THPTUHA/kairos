package agent

import (
	"errors"
	"fmt"
	"time"

	"github.com/THPTUHA/kairos/server/agent/ntime"
	"github.com/THPTUHA/kairos/server/plugin"
	"github.com/THPTUHA/kairos/server/plugin/proto"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/buntdb"
)

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

	// Display name of the job. If present, displayed instead of the name
	DisplayName string `json:"displayname"`

	// The timezone where the cron expression will be evaluated in.
	// Empty means local time.
	Timezone string `json:"timezone"`

	// Cron expression for the job. When to run the job.
	Schedule string `json:"schedule"`

	// Arbitrary string indicating the owner of the job.
	Owner string `json:"owner"`

	// Email address to use for notifications.
	OwnerEmail string `json:"owner_email"`

	// Number of successful executions of this job.
	SuccessCount int `json:"success_count"`

	// Number of errors running this job.
	ErrorCount int `json:"error_count"`

	// Last time this job executed successfully.
	LastSuccess ntime.NullableTime `json:"last_success"`

	// Last time this job failed.
	LastError ntime.NullableTime `json:"last_error"`

	// Is this job disabled?
	Disabled bool `json:"disabled"`

	// Tags of the target servers to run this job against.
	Tags map[string]string `json:"tags"`

	// Job metadata describes the job and allows filtering from the API.
	Metadata map[string]string `json:"metadata"`

	// Pointer to the calling agent.
	Agent *Agent `json:"-"`

	// Number of times to retry a job that failed an execution.
	Retries uint `json:"retries"`

	// Jobs that are dependent upon this one will be run after this job runs.
	DependentJobs []string `json:"dependent_jobs"`

	// Job pointer that are dependent upon this one
	ChildJobs []*Job `json:"-"`

	// Job id of job that this job is dependent upon.
	ParentJob string `json:"parent_job"`

	// Processors to use for this job.
	Processors map[string]plugin.Config `json:"processors"`

	// Concurrency policy for this job (allow, forbid).
	Concurrency string `json:"concurrency"`

	// Executor plugin to be used in this job.
	Executor string `json:"executor"`

	// Configuration arguments for the specific executor.
	ExecutorConfig plugin.ExecutorPluginConfig `json:"executor_config"`

	// Computed job status.
	Status string `json:"status"`

	// Computed next execution.
	Next time.Time `json:"next"`

	// Delete the job after the first successful execution.
	Ephemeral bool `json:"ephemeral"`

	// The job will not be executed after this time.
	ExpiresAt ntime.NullableTime `json:"expires_at"`

	logger *logrus.Entry
}

// NewJobFromProto create a new Job from a PB Job struct
func NewJobFromProto(in *proto.Job, logger *logrus.Entry) *Job {
	job := &Job{
		ID:             in.Name,
		Name:           in.Name,
		DisplayName:    in.Displayname,
		Timezone:       in.Timezone,
		Schedule:       in.Schedule,
		Owner:          in.Owner,
		OwnerEmail:     in.OwnerEmail,
		SuccessCount:   int(in.SuccessCount),
		ErrorCount:     int(in.ErrorCount),
		Disabled:       in.Disabled,
		Tags:           in.Tags,
		Retries:        uint(in.Retries),
		DependentJobs:  in.DependentJobs,
		ParentJob:      in.ParentJob,
		Concurrency:    in.Concurrency,
		Executor:       in.Executor,
		ExecutorConfig: in.ExecutorConfig,
		Status:         in.Status,
		Metadata:       in.Metadata,
		Next:           in.GetNext().AsTime(),
		Ephemeral:      in.Ephemeral,
		logger:         logger,
	}
	if in.GetLastSuccess().GetHasValue() {
		t := in.GetLastSuccess().GetTime().AsTime()
		job.LastSuccess.Set(t)
	}
	if in.GetLastError().GetHasValue() {
		t := in.GetLastError().GetTime().AsTime()
		job.LastError.Set(t)
	}
	if in.GetExpiresAt().GetHasValue() {
		t := in.GetExpiresAt().GetTime().AsTime()
		job.ExpiresAt.Set(t)
	}

	procs := make(map[string]plugin.Config)
	for k, v := range in.Processors {
		if len(v.Config) == 0 {
			v.Config = make(map[string]string)
		}
		procs[k] = v.Config
	}
	job.Processors = procs

	return job
}

// Validate validates whether all values in the job are acceptable.
func (j *Job) Validate() error {
	if j.Name == "" {
		return fmt.Errorf("name cannot be empty")
	}

	if valid, chr := isSlug(j.Name); !valid {
		return fmt.Errorf("name contains illegal character '%s'", chr)
	}

	if j.ParentJob == j.Name {
		return ErrSameParent
	}

	// Validate schedule, allow empty schedule if parent job set.
	if j.Schedule != "" || j.ParentJob == "" {
		if _, err := extcron.Parse(j.Schedule); err != nil {
			return fmt.Errorf("%s: %s", ErrScheduleParse.Error(), err)
		}
	}

	if j.Concurrency != ConcurrencyAllow && j.Concurrency != ConcurrencyForbid && j.Concurrency != "" {
		return ErrWrongConcurrency
	}

	// An empty string is a valid timezone for LoadLocation
	if _, err := time.LoadLocation(j.Timezone); err != nil {
		return err
	}

	if j.Executor == "shell" && j.ExecutorConfig["timeout"] != "" {
		_, err := time.ParseDuration(j.ExecutorConfig["timeout"])
		if err != nil {
			return fmt.Errorf("Error parsing job timeout value")
		}
	}

	return nil
}

// GetParent returns the parent job of a job
func (j *Job) GetParent(store *Store) (*Job, error) {
	if j.Name == j.ParentJob {
		return nil, ErrSameParent
	}

	if j.ParentJob == "" {
		return nil, ErrNoParent
	}

	parentJob, err := store.GetJob(j.ParentJob, nil)
	if err != nil {
		if err == buntdb.ErrNotFound {
			return nil, ErrParentJobNotFound
		}
		return nil, err

	}

	return parentJob, nil
}
