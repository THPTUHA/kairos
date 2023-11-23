package pubsub

var (
	AuthCmd = "authcmd"
)

type Event struct {
	Message string
	Payload string
}

type PubSubPayload struct {
	UserID      int64
	ClientID    int64
	UserCountID string
	Cmd         string
	Data        string
	Fn          func(event Event)
}

var (
	SuccessEvent = Event{Message: "successful"}
)
