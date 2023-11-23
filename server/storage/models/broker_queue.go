package models

type BrokerQueue struct {
	ID         int64  `json:"id"`
	Key        string `json:"key"`
	Value      string `json:"value"`
	WorkflowID int64  `json:"workflow_id"`
	Used       bool   `json:"used"`
	CreatedAt  int64  `json:"created_at"`
}
