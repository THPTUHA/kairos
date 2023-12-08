package models

type BrokerRecord struct {
	ID        int    `json:"id"`
	Status    int    `json:"status"`
	Input     string `json:"input"`
	Output    string `json:"output"`
	BrokerID  int64  `json:"broker_id"`
	CreatedAt int64  `json:"created_at"`
}
