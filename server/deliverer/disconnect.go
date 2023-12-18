package deliverer

import "fmt"

type Disconnect struct {
	Code   uint32 `json:"code,omitempty"`
	Reason string `json:"reason"`
}

func (d Disconnect) String() string {
	return fmt.Sprintf("code: %d, reason: %s", d.Code, d.Reason)
}

func (d Disconnect) Error() string {
	return d.String()
}

var DisconnectConnectionClosed = Disconnect{
	Code:   3000,
	Reason: "connection closed",
}

var (
	DisconnectShutdown = Disconnect{
		Code:   3001,
		Reason: "shutdown",
	}
	DisconnectServerError = Disconnect{
		Code:   3004,
		Reason: "internal server error",
	}
	DisconnectExpired = Disconnect{
		Code:   3005,
		Reason: "connection expired",
	}
	DisconnectSubExpired = Disconnect{
		Code:   3006,
		Reason: "subscription expired",
	}
	DisconnectSlow = Disconnect{
		Code:   3008,
		Reason: "slow",
	}
	DisconnectWriteError = Disconnect{
		Code:   3009,
		Reason: "write error",
	}
	DisconnectInsufficientState = Disconnect{
		Code:   3010,
		Reason: "insufficient state",
	}
	DisconnectForceReconnect = Disconnect{
		Code:   3011,
		Reason: "force reconnect",
	}
	DisconnectNoPong = Disconnect{
		Code:   3012,
		Reason: "no pong",
	}
	DisconnectTooManyRequests = Disconnect{
		Code:   3013,
		Reason: "too many requests",
	}
)

var (
	DisconnectInvalidToken = Disconnect{
		Code:   3500,
		Reason: "invalid token",
	}
	DisconnectBadRequest = Disconnect{
		Code:   3501,
		Reason: "bad request",
	}
	DisconnectStale = Disconnect{
		Code:   3502,
		Reason: "stale",
	}
	DisconnectForceNoReconnect = Disconnect{
		Code:   3503,
		Reason: "force disconnect",
	}
	DisconnectConnectionLimit = Disconnect{
		Code:   3504,
		Reason: "connection limit",
	}
	DisconnectChannelLimit = Disconnect{
		Code:   3505,
		Reason: "channel limit",
	}
	DisconnectInappropriateProtocol = Disconnect{
		Code:   3506,
		Reason: "inappropriate protocol",
	}
	DisconnectPermissionDenied = Disconnect{
		Code:   3507,
		Reason: "permission denied",
	}
	DisconnectNotAvailable = Disconnect{
		Code:   3508,
		Reason: "not available",
	}
	DisconnectTooManyErrors = Disconnect{
		Code:   3509,
		Reason: "too many errors",
	}
)
