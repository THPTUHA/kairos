package models

type TaskRecord struct {
	ID         int    `json:"id"`
	Status     int    `json:"status"`
	Output     string `json:"output"`
	TaskID     int64  `json:"task_id"`
	StartedAt  int64  `json:"started_at"`
	FinishedAt int64  `json:"finished_at"`
	ClientID   int64  `json:"client_id"`
	CreatedAt  int64  `json:"created_at"`
}
