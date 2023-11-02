package models

type Collections struct {
	ID        int64  `json:"id"`
	RawData   string `json:"string"`
	UserID    int64  `json:"user_id"`
	CreatedAt int    `json:"created_at"`
	UpdatedAt int    `json:"updated_at"`
}
