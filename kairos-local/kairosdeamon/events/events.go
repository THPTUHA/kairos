package events

type Event struct {
	Cmd     int
	Payload string
}

const (
	ConnectServerCmd = iota
	SubscribeServerCmd
)
