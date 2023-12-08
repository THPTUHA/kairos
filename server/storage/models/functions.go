package models

type Function struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
	Name      string `json:"name"`
}
