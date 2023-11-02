package pubsub

var (
	AuthCmd = "authcmd"
)

type Event struct {
	Message string
}

type PubSubPayload struct {
	UserCountID string
	Cmd         string
	Data        string
	Fn          func(event Event)
}

var (
	SuccessEvent = Event{Message: "successful"}
)
