package models

const (
	KairosUser = iota
	ClientUser
	ChannelUser
)

type User struct {
	ID        int    `json:"id"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	Avatar    string `json:"avatar"`
	Token     string `json:"token"`
	SecretKey string `json:"secret_key"`
	ApiKey    string `json:"api_key"`
}
