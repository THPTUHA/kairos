package models

type Certificate struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	UserID    int64  `json:"user_id"`
	APIKey    string `json:"api_key"`
	SecretKey string `json:"secret_key"`
	ExpireAt  int64  `json:"expire_at"`
	CreatedAt int64  `json:"created_at"`
}
