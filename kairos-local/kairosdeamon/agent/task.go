package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"text/template"
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
	Retries        int                         `json:"retries"`
	Processors     map[string]Config           `json:"processors"`
	Executor       string                      `json:"executor"`
	ExecutorConfig plugin.ExecutorPluginConfig `json:"executor_config"`
	Status         string                      `json:"status"`
	Next           time.Time                   `json:"next"`
	Ephemeral      bool                        `json:"ephemeral"`
	ExpiresAt      ntime.NullableTime          `json:"expires_at"`
	Deps           []string                    `json:"deps"`

	UserDefineVars map[string]string `json:"user_define_vars,omitempty"`
	DynamicVars    map[string]bool   `json:"dynamic_vars,omitempty"`

	logger *logrus.Entry
}

func (t *Task) Setup(wt *workflow.Task) error {
	var ec plugin.ExecutorPluginConfig
	fmt.Println("...............")
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
	t.Deps = wt.Deps
	t.DynamicVars = wt.DynamicVars
	t.UserDefineVars = wt.UserDefineVars
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

	// TODO check thời gian còn lại của schedule, nếu hết hạn mà chưa đủ điều kiện chạy thì set về no schedule
	fmt.Printf("Dynamic Variable %+v\n", t.DynamicVars)
	if len(t.Deps) > 0 && len(t.DynamicVars) > 0 {
		vars := make(map[string]string)
		err := t.Agent.Store.GetQueue(fmt.Sprint(t.WorkflowID), &vars)
		fmt.Printf("[run 1] %+v\n", vars)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"task":   t.Name,
				"action": "check runnable",
			}).Error(err)
			return false
		}
		fmt.Println("[run 2]")
		tempVars := map[string]string{}
		accept := true
		for k := range t.DynamicVars {
			c := workflow.GetRootDefaultVar(k)
			fmt.Println("???", c)
			value, ok := vars[c]
			if !ok {
				accept = false
				break
			}
			if strings.HasSuffix(k, workflow.SubTaskOutPut) {
				var t workflow.Task
				err := json.Unmarshal([]byte(value), &t)
				if err != nil {
					logger.WithFields(logrus.Fields{
						"task":   t.Name,
						"action": "unmarshal task",
					}).Error(err)
					return false
				}
				tempVars[k] = t.Input
			}
			// TODO Check INPUT
		}
		fmt.Printf("[run 3] %+v\n", tempVars)
		if !accept {
			for k, v := range vars {
				t.Agent.Store.SetQueue(fmt.Sprint(t.WorkflowID), k, v)
				if err != nil {
					logger.WithFields(logrus.Fields{
						"task":   t.Name,
						"action": "rolback queue",
					}).Error(err)
				}
			}
			return false
		}

		fmt.Println("[run 4]")
		tmp := t.ExecutorConfig
		c, err := json.Marshal(t.ExecutorConfig)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"task":   t.Name,
				"action": "marshal executor",
			}).Error(err)
			return false
		}
		fmt.Println("[run 5]")
		// TODO compile config
		tmpl, err := template.New("compile").Parse(string(c))
		if err != nil {
			logger.WithFields(logrus.Fields{
				"task":   t.Name,
				"action": "compile executor",
			}).Error(err)
			return false
		}
		fmt.Println("[run 6]")
		var buf bytes.Buffer
		// to upper
		parse := make(map[string]string)
		for k, v := range tempVars {
			parse[strings.ToUpper(k)] = v
		}
		err = tmpl.Execute(&buf, &parse)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"task":   t.Name,
				"action": "execute compile executor",
			}).Error(err)
			return false
		}
		fmt.Println("[run 7]")
		err = json.Unmarshal(buf.Bytes(), &tmp)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"task":   t.Name,
				"action": "unmarshal executor",
			}).Error(err)
			return false
		}
		fmt.Println("[run 8]")
		t.ExecutorConfig = tmp
		fmt.Printf("[FUCKING GO HERE] config = %+v\n", tmp)
	}
	return true
}

func (t *Task) ToBytes() ([]byte, error) {
	return json.Marshal(&t)
}

// Impletation corn
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

		ex := NewExecution(t.ID)

		if err := t.Agent.Run(t, ex); err != nil {
			t.logger.WithError(err).Error("task: Error running task")
		}
	}
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
	return fmt.Sprintf("\"Task: %s, id: %s, scheduled at: %s\"", t.Name, t.ID, t.Schedule)
}

func (t *Task) GetTimeLocation() *time.Location {
	loc, _ := time.LoadLocation(t.Timezone)
	return loc
}
