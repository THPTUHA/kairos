package models

type Client struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	UserID      int64  `json:"user_id"`
	CreatedAt   int64  `json:"created_at"`
	ActiveSince int64  `json:"active_since"`
	Status      int    `json:"status"`
}
