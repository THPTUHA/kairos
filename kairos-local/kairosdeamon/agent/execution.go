package agent

import (
	"fmt"
	"strconv"
	"time"

	"github.com/THPTUHA/kairos/pkg/workflow"
)

type Execution struct {
	Id         string    `json:"id,omitempty"`
	TaskID     string    `json:"task_id,omitempty"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	Success    bool      `json:"success"`
	Output     string    `json:"output,omitempty"`
	NodeName   string    `json:"node_name,omitempty"`
	Group      int64     `json:"group,omitempty"`
	Attempt    uint      `json:"attempt,omitempty"`
	Offset     int       `json:"offset,omitempty"`
}

func NewExecution(taskID string) *Execution {
	return &Execution{
		TaskID:  taskID,
		Group:   time.Now().UnixNano(),
		Attempt: 1,
	}
}

func (e *Execution) Key() string {
	return fmt.Sprintf("%d-%s", e.StartedAt.UnixNano(), e.NodeName)
}

func (e *Execution) GetGroup() string {
	return strconv.FormatInt(e.Group, 10)
}

func (e *Execution) GetResult() *workflow.Result {
	var r workflow.Result
	r.Output = e.Output
	r.Success = e.Success
	r.Attempt = e.Attempt
	r.StartedAt = e.StartedAt.Unix()
	r.FinishedAt = e.FinishedAt.Unix()
	r.Offset = e.Offset

	if e.FinishedAt.Unix() < 0 {
		r.FinishedAt = 0
	}
	return &r
}
