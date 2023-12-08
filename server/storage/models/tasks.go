package models

const (
	TaksPending = iota
)

type Task struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Schedule   string `json:"schedule"`
	Timezone   string `json:"timezone"`
	Clients    string `json:"clients"`
	Retries    int    `json:"retries"`
	Executor   string `json:"executor"`
	Duration   string `json:"duration"`
	WorkflowID int64  `json:"workflow_id"`
	Status     int    `json:"status"`
	Payload    string `json:"payload"`
	ExpiresAt  string `json:"expires_at"`
	Wait       string `json:"wait"`
}
