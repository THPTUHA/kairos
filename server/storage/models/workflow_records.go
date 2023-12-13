package models

const (
	WFRecordFault = iota
	WFRecordScuccess
)

type WorkflowRecords struct {
	ID          int64  `json:"id"`
	WorkflowID  int64  `json:"workflow_id"`
	Record      string `json:"record"`
	CreatedAt   int64  `json:"created_at"`
	Status      int    `json:"status"`
	DeliverErr  string `json:"deliver_err"`
	IsRecovered bool   `json:"is_recovered"`
}
