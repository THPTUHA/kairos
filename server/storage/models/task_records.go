package models

type TaskRecord struct {
	ID           int64  `json:"id"`
	Status       int    `json:"status"`
	Input        string `json:"input"`
	Output       string `json:"output"`
	TaskID       int64  `json:"task_id"`
	Attemp       int    `json:"attemp"`
	SuccessCount int    `json:"success_count"`
	ErrorCount   int    `json:"error_count"`
	StartedAt    int    `json:"started_at"`
	FinishedAt   int    `json:"finished_at"`
	LastSuccess  int    `json:"last_success"`
	LastError    int    `json:"last_error"`
	CreatedAt    int64  `json:"created_at"`
}
