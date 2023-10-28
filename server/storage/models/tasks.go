package models

type Task struct {
	ID         int64  `json:"id"`
	Key        string `json:"key"`
	Schedule   string `json:"schedule"`
	Timezone   string `json:"timezone"`
	Executor   string `json:"executor"`
	TimeOut    int    `json:"timeout"`
	Retries    int    `json:"retries"`
	Inputs     string `json:"inputs"`
	Run        string `json:"run"`
	WorkflowID int64  `json:"workflow_id"`
	Status     int    `json:"status"`
	Strict     bool   `json:"strict"`
	EnvExec    string `json:"env_exec"`
	ExpiresAt  int    `json:"expires_at"`
	CreatedAt  int    `json:"created_at"`
	UpdatedAt  int    `json:"updated_at"`
}
