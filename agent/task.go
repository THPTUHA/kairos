package agent

import (
	"errors"
	"fmt"
	"time"

	"github.com/THPTUHA/kairos/agent/ntime"
	"github.com/THPTUHA/kairos/pkg/extcron"
	"github.com/THPTUHA/kairos/pkg/helper"
	"github.com/THPTUHA/kairos/server/plugin"
	"github.com/THPTUHA/kairos/server/plugin/proto"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	// ErrNoAgent is returned when the task's agent is nil.
	ErrNoAgent = errors.New("no agent defined")
	// ErrNoCommand is returned when attempting to store a task that has no command.
	ErrNoCommand = errors.New("unspecified command for task")
)

const (
	// StatusNotSet is the initial task status.
	StatusNotSet = ""
	// StatusSuccess is status of a task whose last run was a success.
	StatusSuccess = "success"
	// StatusRunning is status of a task whose last run has not finished.
	StatusRunning = "running"
	// StatusFailed is status of a task whose last run was not successful on any nodes.
	StatusFailed = "failed"
	// StatusPartiallyFailed is status of a task whose last run was successful on only some nodes.
	StatusPartiallyFailed = "partially_failed"
)

type Task struct {
	ID         int64  `json:"id"`
	Key        string `json:"key"`
	WorkflowID int64

	// The timezone where the cron expression will be evaluated in.
	// Empty means local time.
	Timezone string `json:"timezone"`

	// Cron expression for the task. When to run the task.
	Schedule string `json:"schedule"`

	// Email address to use for notifications.
	OwnerEmail string `json:"owner_email"`

	// Number of successful executions of this task.
	SuccessCount int `json:"success_count"`

	// Number of errors running this task.
	ErrorCount int `json:"error_count"`

	// Last time this task executed successfully.
	LastSuccess ntime.NullableTime `json:"last_success"`

	// Last time this task failed.
	LastError ntime.NullableTime `json:"last_error"`

	// Is this task disabled?
	Disabled bool `json:"disabled"`

	// Tags of the target servers to run this task against.
	Tags map[string]string `json:"tags"`

	// Task metadata describes the task and allows filtering from the API.
	Metadata map[string]string `json:"metadata"`

	// Pointer to the calling agent.
	Agent *Agent `json:"-"`

	// Number of times to retry a task that failed an execution.
	Retries uint `json:"retries"`

	// Processors to use for this task.
	Processors map[string]plugin.Config `json:"processors"`

	// Executor plugin to be used in this task.
	Executor string `json:"executor"`

	// Configuration arguments for the specific executor.
	ExecutorConfig plugin.ExecutorPluginConfig `json:"executor_config"`

	// Computed task status.
	Status string `json:"status"`

	// Computed next execution.
	Next time.Time `json:"next"`

	// Delete the task after the first successful execution.
	Ephemeral bool `json:"ephemeral"`

	// The task will not be executed after this time.
	ExpiresAt ntime.NullableTime `json:"expires_at"`

	logger *logrus.Entry
}

// NewTaskFromProto create a new Task from a PB Task struct
func NewTaskFromProto(in *proto.Task, logger *logrus.Entry) *Task {
	task := &Task{
		ID:             in.Id,
		Key:            in.Key,
		Timezone:       in.Timezone,
		Schedule:       in.Schedule,
		OwnerEmail:     in.OwnerEmail,
		SuccessCount:   int(in.SuccessCount),
		ErrorCount:     int(in.ErrorCount),
		Disabled:       in.Disabled,
		Tags:           in.Tags,
		Retries:        uint(in.Retries),
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
		task.LastSuccess.Set(t)
	}
	if in.GetLastError().GetHasValue() {
		t := in.GetLastError().GetTime().AsTime()
		task.LastError.Set(t)
	}
	if in.GetExpiresAt().GetHasValue() {
		t := in.GetExpiresAt().GetTime().AsTime()
		task.ExpiresAt.Set(t)
	}

	procs := make(map[string]plugin.Config)
	for k, v := range in.Processors {
		if len(v.Config) == 0 {
			v.Config = make(map[string]string)
		}
		procs[k] = v.Config
	}
	task.Processors = procs

	return task
}

func (t *Task) isRunnable(logger *logrus.Entry) bool {
	if t.Disabled || (t.ExpiresAt.HasValue() && time.Now().After(t.ExpiresAt.Get())) {
		logger.WithField("task", t.Key).
			Debug("task: Skipping execution because task is disabled or expired")
		return false
	}

	if t.Agent.GlobalLock {
		logger.WithField("task", t.Key).
			Warning("task: Skipping execution because active global lock")
		return false
	}

	return true
}

// Impletation corn
func (t *Task) Run() {
	// As this function should comply with the Task interface of the cron package we will use
	// the agent property on execution, this is why it need to check if it's set and otherwise fail.
	if t.Agent == nil {
		t.logger.Fatal("task: agent not set")
	}

	// Check if it's runnable
	if t.isRunnable(t.logger) {
		t.logger.WithFields(logrus.Fields{
			"task":     t.Key,
			"schedule": t.Schedule,
		}).Debug("task: Run task")

		cronInspect.Set(t.Key, t)

		// Simple execution wrapper
		ex := NewExecution(t.ID)

		if _, err := t.Agent.Run(t.ID, ex); err != nil {
			t.logger.WithError(err).Error("task: Error running task")
		}
	}
}

// Validate validates whether all values in the task are acceptable.
func (t *Task) Validate() error {
	if t.Key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	if valid, chr := helper.IsSlug(t.Key); !valid {
		return fmt.Errorf("key contains illegal character '%s'", chr)
	}

	if t.Schedule != "" {
		if _, err := extcron.Parse(t.Schedule); err != nil {
			return fmt.Errorf("%s: %s", ErrScheduleParse.Error(), err)
		}
	}

	// An empty string is a valid timezone for LoadLocation
	if _, err := time.LoadLocation(t.Timezone); err != nil {
		return err
	}

	if t.Executor == "shell" && t.ExecutorConfig["timeout"] != "" {
		_, err := time.ParseDuration(t.ExecutorConfig["timeout"])
		if err != nil {
			return fmt.Errorf("Error parsing task timeout value")
		}
	}

	return nil
}

// GetNext returns the task's next schedule from now
func (t *Task) GetNext() (time.Time, error) {
	if t.Schedule != "" {
		s, err := extcron.Parse(t.Schedule)
		if err != nil {
			return time.Time{}, err
		}
		return s.Next(time.Now()), nil
	}

	return time.Time{}, nil
}

func (t *Task) String() string {
	return fmt.Sprintf("\"Task: %s, scheduled at: %s, tags:%v\"", t.Key, t.Schedule, t.Tags)
}

func (t *Task) GetTimeLocation() *time.Location {
	loc, _ := time.LoadLocation(t.Timezone)
	return loc
}

// ToProto return the corresponding representation of this Task in proto struct
func (t *Task) ToProto() *proto.Task {
	lastSuccess := &proto.Task_NullableTime{
		HasValue: t.LastSuccess.HasValue(),
	}
	if t.LastSuccess.HasValue() {
		lastSuccess.Time = timestamppb.New(t.LastSuccess.Get())
	}
	lastError := &proto.Task_NullableTime{
		HasValue: t.LastError.HasValue(),
	}
	if t.LastError.HasValue() {
		lastError.Time = timestamppb.New(t.LastError.Get())
	}

	next := timestamppb.New(t.Next)

	expiresAt := &proto.Task_NullableTime{
		HasValue: t.ExpiresAt.HasValue(),
	}
	if t.ExpiresAt.HasValue() {
		expiresAt.Time = timestamppb.New(t.ExpiresAt.Get())
	}

	processors := make(map[string]*proto.PluginConfig)
	for k, v := range t.Processors {
		processors[k] = &proto.PluginConfig{Config: v}
	}
	return &proto.Task{
		Id:             t.ID,
		Key:            t.Key,
		Timezone:       t.Timezone,
		Schedule:       t.Schedule,
		OwnerEmail:     t.OwnerEmail,
		SuccessCount:   int32(t.SuccessCount),
		ErrorCount:     int32(t.ErrorCount),
		Disabled:       t.Disabled,
		Tags:           t.Tags,
		Retries:        uint32(t.Retries),
		Processors:     processors,
		Executor:       t.Executor,
		ExecutorConfig: t.ExecutorConfig,
		Status:         t.Status,
		Metadata:       t.Metadata,
		LastSuccess:    lastSuccess,
		LastError:      lastError,
		Next:           next,
		Ephemeral:      t.Ephemeral,
		ExpiresAt:      expiresAt,
	}
}
