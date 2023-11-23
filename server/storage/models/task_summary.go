package models

type TaskSummary struct {
	ID           int   `json:"id"`
	Status       int   `json:"status"`
	TaskID       int   `json:"task_id"`
	Attempt      int   `json:"attemp"`
	SuccessCount int   `json:"success_count"`
	ErrorCount   int   `json:"error_count"`
	LastSuccess  int   `json:"last_success"`
	LastError    int   `json:"last_error"`
	CreatedAt    int64 `json:"created_at"`
}
