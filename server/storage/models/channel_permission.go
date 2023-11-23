package models

const (
	ReadRole = iota + 1
	WriteRole
	ReadWriteRole
)

type ChannelPermission struct {
	ID        int64 `json:"id"`
	CertID    int64 `json:"cert_id"`
	Role      int   `json:"role"`
	ChannelID int64 `json:"channel_id"`
}
