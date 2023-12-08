package models

const (
	BrokerPending = iota
	BrokerRunning
)

type Broker struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Listens    string `json:"listens"`
	Flows      string `json:"flows"`
	WorkflowID int64  `json:"workflow_id"`
	Status     int    `json:"status"`
	Used       bool   `json:"used"`
}
