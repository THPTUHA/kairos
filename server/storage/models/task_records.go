package models

type TaskRecord struct {
	ID         int    `json:"id"`
	Status     int    `json:"status"`
	Input      string `json:"input"`
	Output     string `json:"output"`
	TaskID     int    `json:"task_id"`
	StartedAt  int    `json:"started_at"`
	FinishedAt int    `json:"finished_at"`
	CreatedAt  int64  `json:"created_at"`
}
