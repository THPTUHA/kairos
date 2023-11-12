package models

type User struct {
	ID        int    `json:"id"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	Token     string `json:"token"`
	SecretKey string `json:"secret_key"`
	ApiKey    string `json:"api_key"`
}
