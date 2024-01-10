package models

type Trigger struct {
	ID         int64  `json:"id"`
	WorkflowID int64  `json:"workflow_id"`
	ObjectID   int64  `json:"object_id"`
	Type       string `json:"type"`
	Schedule   string `json:"schedule"`
	Input      string `json:"input"`
	Status     int    `json:"status"`
	TriggerAt  int64  `json:"trigger_at"`
	Client     string `json:"client"`
	Name       string `json:"name"`
}
