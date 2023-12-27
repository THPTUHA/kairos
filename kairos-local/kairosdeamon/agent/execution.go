package agent

import (
	"fmt"
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
	Group      string    `json:"group,omitempty"`
	Attempt    uint      `json:"attempt,omitempty"`
	Offset     int       `json:"offset,omitempty"`
	Part       string    `json:"part,omitempty"`
	Parent     string    `json:"parent,omitempty"`
}

func NewExecution(taskID string, group string) *Execution {
	return &Execution{
		TaskID:  taskID,
		Group:   group,
		Attempt: 1,
	}
}

func (e *Execution) Key() string {
	return fmt.Sprintf("%d-%s", e.StartedAt.UnixNano(), e.NodeName)
}

func (e *Execution) GetGroup() string {
	return e.Group
}

func (e *Execution) GetResult() *workflow.Result {
	var r workflow.Result
	r.Output = e.Output
	r.Success = e.Success
	r.Attempt = e.Attempt
	r.FinishedAt = e.FinishedAt.Unix()
	r.Offset = e.Offset

	if e.FinishedAt.Unix() < 0 {
		r.FinishedAt = 0
	}

	if r.Offset > 1 {
		r.StartedAt = 0
	} else {
		r.StartedAt = e.StartedAt.Unix()
	}
	return &r
}
