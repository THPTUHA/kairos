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
	// ErrorExpired indicates that connection expired (no token involved).
	ErrorExpired = &Error{
		Code:    110,
		Message: "expired",
	}

	// ErrorInternal means server error, if returned this is a signal
	// that something went wrong with server itself and client most probably
	// not guilty.
	ErrorInternal = &Error{
		Code:      100,
		Message:   "internal server error",
		Temporary: true,
	}
	ErrorPermissionDenied = &Error{
		Code:    103,
		Message: "permission denied",
	}

	// ErrorBadRequest says that server can not process received
	// data because it is malformed. Retrying request does not make sense.
	ErrorBadRequest = &Error{
		Code:    107,
		Message: "bad request",
	}

	// ErrorUnrecoverablePosition means that stream does not contain required
	// range of publications to fulfill a history query. This can happen due to
	// expiration, size limitation or due to wrong epoch.
	ErrorUnrecoverablePosition = &Error{
		Code:    112,
		Message: "unrecoverable position",
	}
	ErrorNotAvailable = &Error{
		Code:    108,
		Message: "not available",
	}
	// ErrorAlreadySubscribed returned when client wants to subscribe on channel
	// it already subscribed to.
	ErrorAlreadySubscribed = &Error{
		Code:    105,
		Message: "already subscribed",
	}
	// ErrorLimitExceeded says that some sort of limit exceeded, server logs should
	// give more detailed information. See also ErrorTooManyRequests which is more
	// specific for rate limiting purposes.
	ErrorLimitExceeded = &Error{
		Code:    106,
		Message: "limit exceeded",
	}
)
