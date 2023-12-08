package pubsub

import (
	"time"

	"github.com/jpillora/backoff"
)

type reconnectStrategy interface {
	timeBeforeNextAttempt(attempt int) time.Duration
}

type backoffReconnect struct {
	Factor   float64
	Jitter   bool
	MinDelay time.Duration
	MaxDelay time.Duration
}

var defaultBackoffReconnect = &backoffReconnect{
	MinDelay: 200 * time.Millisecond,
	MaxDelay: 20 * time.Second,
	Factor:   2,
	Jitter:   true,
}

func (r *backoffReconnect) timeBeforeNextAttempt(attempt int) time.Duration {
	b := &backoff.Backoff{
		Min:    r.MinDelay,
		Max:    r.MaxDelay,
		Factor: r.Factor,
		Jitter: r.Jitter,
	}
	return b.ForAttempt(float64(attempt))
}
