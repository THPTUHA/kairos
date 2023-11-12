package models

type Client struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	KairosName string `json:"kairos_name"`
	UserID     int64  `json:"user_id"`
}
