package agent

import (
	"fmt"
	"strconv"
	"time"
)

// Execution type holds all of the details of a specific Execution.
type Execution struct {
	// Id is the Key for this execution
	Id string `json:"id,omitempty"`

	// Name of the task this executions refers to.
	TaskID string `json:"task_id,omitempty"`
	// Start time of the execution.
	StartedAt time.Time `json:"started_at,omitempty"`

	// When the execution finished running.
	FinishedAt time.Time `json:"finished_at,omitempty"`

	// If this execution executed successfully.
	Success bool `json:"success"`

	// Partial output of the execution.
	Output string `json:"output,omitempty"`

	// Node name of the node that run this execution.
	NodeName string `json:"node_name,omitempty"`

	// Execution group to what this execution belongs to.
	Group int64 `json:"group,omitempty"`

	// Retry attempt of this execution.
	Attempt uint `json:"attempt,omitempty"`
}

func NewExecution(taskID string) *Execution {
	return &Execution{
		TaskID:  taskID,
		Group:   time.Now().UnixNano(),
		Attempt: 1,
	}
}

// Key wil generate the execution Id for an execution.
func (e *Execution) Key() string {
	return fmt.Sprintf("%d-%s", e.StartedAt.UnixNano(), e.NodeName)
}

// GetGroup is the getter for the execution group.
func (e *Execution) GetGroup() string {
	return strconv.FormatInt(e.Group, 10)
}
