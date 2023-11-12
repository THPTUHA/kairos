package models

type Vars struct {
	ID         int64  `json:"id"`
	Key        string `json:"key"`
	Value      string `json:"value"`
	WorkflowID int64  `json:"workflow_id"`
}
