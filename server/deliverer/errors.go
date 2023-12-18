package deliverer

import (
	"fmt"

	"github.com/THPTUHA/kairos/pkg/protocol/deliverprotocol"
)

type Error struct {
	Code      uint32
	Message   string
	Temporary bool
}

func (e *Error) toProto() *deliverprotocol.Error {
	return &deliverprotocol.Error{
		Code:    e.Code,
		Message: e.Message,
	}
}

func (e *Error) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

var (
	ErrorExpired = &Error{
		Code:    110,
		Message: "expired",
	}

	ErrorInternal = &Error{
		Code:      100,
		Message:   "internal server error",
		Temporary: true,
	}
	ErrorPermissionDenied = &Error{
		Code:    103,
		Message: "permission denied",
	}
	ErrorBadRequest = &Error{
		Code:    107,
		Message: "bad request",
	}
	ErrorUnrecoverablePosition = &Error{
		Code:    112,
		Message: "unrecoverable position",
	}
	ErrorNotAvailable = &Error{
		Code:    108,
		Message: "not available",
	}
	ErrorAlreadySubscribed = &Error{
		Code:    105,
		Message: "already subscribed",
	}
	ErrorLimitExceeded = &Error{
		Code:    106,
		Message: "limit exceeded",
	}
)
