package agent

import (
	"errors"
	"fmt"
	"time"

	"github.com/THPTUHA/kairos/pkg/extcron"
	"github.com/THPTUHA/kairos/pkg/ntime"
	"github.com/THPTUHA/kairos/server/plugin"
	"github.com/sirupsen/logrus"
)

const (
	CreateTaskCmd = iota
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

type TaskEvent struct {
	Cmd  int
	Task *Task
}

type Task struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	WorkflowID int64  `json:"workflow_id"`

	// The timezone where the cron expression will be evaluated in.
	// Empty means local time.
	Timezone string `json:"timezone"`

	// Cron expression for the task. When to run the task.
	Schedule string `json:"schedule"`

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
	Processors map[string]Config `json:"processors"`

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

func (t *Task) isRunnable(logger *logrus.Entry) bool {
	if t.Disabled || (t.ExpiresAt.HasValue() && time.Now().After(t.ExpiresAt.Get())) {
		logger.WithFields(logrus.Fields{
			"id":   t.ID,
			"task": t.Name,
		}).Debug("task: Skipping execution because task is disabled or expired")
		return false
	}

	return true
}

// Impletation corn
func (t *Task) Run() {
	t.logger.WithFields(logrus.Fields{
		"task": t.Name,
		"id":   t.ID,
	}).Debug("start task")
	// As this function should comply with the Task interface of the cron package we will use
	// the agent property on execution, this is why it need to check if it's set and otherwise fail.
	if t.Agent == nil {
		t.logger.Fatal("task: agent not set")
	}

	// Check if it's runnable
	if t.isRunnable(t.logger) {
		t.logger.WithFields(logrus.Fields{
			"task":     t.Name,
			"schedule": t.Schedule,
		}).Debug("task: Run task")

		cronInspect.Set(t.ID, t)

		// Simple execution wrapper
		ex := NewExecution(t.ID)

		if err := t.Agent.Run(t, ex); err != nil {
			t.logger.WithError(err).Error("task: Error running task")
		}
	}
}

// Validate validates whether all values in the task are acceptable.
func (t *Task) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("id cannot be empty")
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
	return fmt.Sprintf("\"Task: %s, scheduled at: %s, tags:%v\"", t.ID, t.Schedule, t.Tags)
}

func (t *Task) GetTimeLocation() *time.Location {
	loc, _ := time.LoadLocation(t.Timezone)
	return loc
}
