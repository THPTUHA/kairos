package models

const (
	WorkflowPending = iota
	WorkflowScheduling
	WorkflowRunning
	WorkflowStopped
)

type Workflow struct {
	ID        int64  `json:"id"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Status    int    `json:"status"`
	Version   string `json:"version"`
	RawData   string `json:"raw_data"`
	CreatedAt int    `json:"created_at"`
	UpdatedAt int    `json:"updated_at"`
	UserID    int64  `json:"user_id"`
}
