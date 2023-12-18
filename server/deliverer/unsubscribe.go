package deliverer

type Unsubscribe struct {
	Code   uint32 `json:"code"`
	Reason string `json:"reason,omitempty"`
}

var (
	unsubscribeClient = Unsubscribe{
		Code:   UnsubscribeCodeClient,
		Reason: "client unsubscribed",
	}
	unsubscribeDisconnect = Unsubscribe{
		Code:   UnsubscribeCodeDisconnect,
		Reason: "client disconnected",
	}
	unsubscribeServer = Unsubscribe{
		Code:   UnsubscribeCodeServer,
		Reason: "server unsubscribe",
	}

	unsubscribeExpired = Unsubscribe{
		Code:   UnsubscribeCodeExpired,
		Reason: "subscription expired",
	}
)

const (
	UnsubscribeCodeClient     uint32 = 0
	UnsubscribeCodeDisconnect uint32 = 1
	UnsubscribeCodeServer     uint32 = 2000
	UnsubscribeCodeExpired    uint32 = 2501
)
