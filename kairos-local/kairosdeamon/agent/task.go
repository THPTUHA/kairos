package agent

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/THPTUHA/kairos/pkg/extcron"
	"github.com/THPTUHA/kairos/pkg/ntime"
	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/THPTUHA/kairos/server/plugin"
	"github.com/sirupsen/logrus"
)

const (
	StatusNotSet          = ""
	StatusSuccess         = "success"
	StatusRunning         = "running"
	StatusFailed          = "failed"
	StatusPartiallyFailed = "partially_failed"
)

type TaskEvent struct {
	Cmd     workflow.RequestActionTask
	CmdTask *workflow.CmdTask
}

type Task struct {
	ID             string                      `json:"id"`
	Name           string                      `json:"name"`
	WorkflowID     int64                       `json:"workflow_id"`
	Timezone       string                      `json:"timezone"`
	Schedule       string                      `json:"schedule"`
	SuccessCount   int                         `json:"success_count"`
	ErrorCount     int                         `json:"error_count"`
	LastSuccess    ntime.NullableTime          `json:"last_success"`
	LastError      ntime.NullableTime          `json:"last_error"`
	Disabled       bool                        `json:"disabled"`
	Metadata       map[string]string           `json:"metadata"`
	Agent          *Agent                      `json:"-"`
	Trigger        *workflow.Trigger           `json:"trigger,omitempty"`
	Retries        int                         `json:"retries"`
	Executor       string                      `json:"executor"`
	ExecutorConfig plugin.ExecutorPluginConfig `json:"executor_config"`
	Status         string                      `json:"status"`
	Next           time.Time                   `json:"next"`
	Ephemeral      bool                        `json:"ephemeral"`
	ExpiresAt      ntime.NullableTime          `json:"expires_at"`
	UserDefineVars map[string]string           `json:"user_define_vars,omitempty"`
	Wait           string                      `json:"wait"`
	Input          string                      `json:"input"`
	Result         chan *workflow.CmdReplyTask `json:"-"`
	logger         *logrus.Entry
}

func (t *Task) Setup(wt *workflow.Task) error {
	var ec plugin.ExecutorPluginConfig
	fmt.Println(wt.Payload)
	err := json.Unmarshal([]byte(wt.Payload), &ec)
	if err != nil {
		return err
	}
	t.ID = fmt.Sprint(wt.ID)
	t.Name = wt.Name
	t.WorkflowID = wt.WorkflowID
	t.Timezone = wt.Timezone
	t.Schedule = wt.Schedule
	t.Metadata = wt.Metadata
	t.Retries = wt.Retries
	t.Executor = wt.Executor
	t.ExecutorConfig = ec
	t.UserDefineVars = wt.UserDefineVars
	t.Wait = wt.Wait
	t.Input = wt.Input
	return nil
}

func (t *Task) ToCmdTask() *workflow.CmdTask {
	var cmd workflow.CmdTask
	id, _ := strconv.ParseInt(t.ID, 10, 64)
	var task workflow.Task
	task.ID = id
	task.WorkflowID = t.WorkflowID
	cmd.Task = &task
	return &cmd
}

func (t *Task) ToCmdReplyTask() *workflow.CmdReplyTask {
	var cmd workflow.CmdReplyTask
	id, _ := strconv.ParseInt(t.ID, 10, 64)
	cmd.TaskID = id
	cmd.WorkflowID = t.WorkflowID
	cmd.TaskName = t.Name
	return &cmd
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

func (t *Task) ToBytes() ([]byte, error) {
	return json.Marshal(&t)
}

func (t *Task) RequestRun(re *workflow.CmdTask) {
	t.logger.WithFields(logrus.Fields{
		"task": t.Name,
		"id":   t.ID,
	}).Debug("start request task")

	if t.Agent == nil {
		t.logger.Fatal("task: agent not set")
	}

	if t.isRunnable(t.logger) {
		t.logger.WithFields(logrus.Fields{
			"task":     t.Name,
			"schedule": t.Schedule,
		}).Debug("task: Run task")

		cronInspect.Set(t.ID, t)

		ex := NewExecution(t.ID, re.Group)

		if err := t.Agent.Run(t, ex, re); err != nil {
			t.logger.WithError(err).Error("task: Error running task")
		}
	}
}

func (t *Task) Run() {
	t.logger.WithFields(logrus.Fields{
		"task": t.Name,
		"id":   t.ID,
	}).Debug("start task")

	if t.Agent == nil {
		t.logger.Fatal("task: agent not set")
	}

	if t.isRunnable(t.logger) {
		t.logger.WithFields(logrus.Fields{
			"task":     t.Name,
			"schedule": t.Schedule,
		}).Debug("task: Run task")

		cronInspect.Set(t.ID, t)

		ex := NewExecution(t.ID, t.Agent.getGroup(t.WorkflowID))

		if err := t.Agent.Run(t, ex, nil); err != nil {
			t.logger.WithError(err).Error("task: Error running task")
		}
	}
}

func (t *Task) RunTrigger(re *workflow.CmdTask) error {
	t.logger.WithFields(logrus.Fields{
		"task": t.Name,
		"id":   t.ID,
	}).Debug("start task sync")

	if t.Agent == nil {
		t.logger.Fatal("task: agent not set")
	}

	if t.isRunnable(t.logger) {
		t.logger.WithFields(logrus.Fields{
			"task":     t.Name,
			"schedule": t.Schedule,
		}).Debug("task: Run task")

		cronInspect.Set(t.ID, t)

		ex := NewExecution(t.ID, re.Group)

		if err := t.Agent.Run(t, ex, re); err != nil {
			t.logger.WithError(err).Error("task: Error running task")
			return err
		}
	}
	return fmt.Errorf("task can't run")
}

func (t *Task) RunSync(re *workflow.CmdTask) (*workflow.Result, error) {
	t.logger.WithFields(logrus.Fields{
		"task": t.Name,
		"id":   t.ID,
	}).Debug("start task sync")

	if t.Agent == nil {
		t.logger.Fatal("task: agent not set")
	}

	if t.isRunnable(t.logger) {
		t.logger.WithFields(logrus.Fields{
			"task":     t.Name,
			"schedule": t.Schedule,
		}).Debug("task: Run task")

		cronInspect.Set(t.ID, t)

		ex := NewExecution(t.ID, t.Agent.getGroup(t.WorkflowID))

		if result, err := t.Agent.RunSync(t, ex, re); err != nil {
			t.logger.WithError(err).Error("task: Error running task")
			return result, err
		} else {
			return result, nil
		}
	}
	return nil, fmt.Errorf("task can't run")
}

func (t *Task) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("id cannot be empty")
	}

	if t.Schedule != "" {
		if _, err := extcron.Parse(t.Schedule); err != nil {
			return fmt.Errorf("%s: %s", ErrScheduleParse.Error(), err)
		}
	}

	if _, err := time.LoadLocation(t.Timezone); err != nil {
		return err
	}

	if t.Executor == "shell" && t.ExecutorConfig["timeout"] != "" {
		if timeout, ok := t.ExecutorConfig["timeout"].(string); ok {
			_, err := time.ParseDuration(timeout)
			if err != nil {
				return fmt.Errorf("Error parsing task timeout value")
			}
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
	return fmt.Sprintf("\"Task: %s, id: %s, scheduled at: %s\"", t.Name, t.ID, t.Schedule)
}

func (t *Task) GetTimeLocation() *time.Location {
	loc, _ := time.LoadLocation(t.Timezone)
	return loc
}
