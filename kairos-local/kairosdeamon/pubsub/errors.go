package pubsub

import (
	"errors"
	"fmt"
)

var (
	ErrTimeout                  = errors.New("timeout")
	ErrClientDisconnected       = errors.New("client disconnected")
	ErrClientClosed             = errors.New("client closed")
	ErrSubscriptionUnsubscribed = errors.New("subscription unsubscribed")
	ErrDuplicateSubscription    = errors.New("duplicate subscription")
	ErrUnauthorized             = errors.New("unauthorized")
)

type RefreshError struct {
	Err error
}

func (r RefreshError) Error() string {
	return fmt.Sprintf("refresh error: %v", r.Err)
}

type TransportError struct {
	Err error
}

func (t TransportError) Error() string {
	return fmt.Sprintf("transport error: %v", t.Err)
}

type ConfigurationError struct {
	Err error
}

func (r ConfigurationError) Error() string {
	return fmt.Sprintf("configuration error: %v", r.Err)
}

type ConnectError struct {
	Err error
}

func (c ConnectError) Error() string {
	return fmt.Sprintf("connect error: %v", c.Err)
}

type Error struct {
	Code    uint32
	Message string
}

func (e Error) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

type SubscriptionSubscribeError struct {
	Err error
}

func (s SubscriptionSubscribeError) Error() string {
	return fmt.Sprintf("subscribe error: %v", s.Err)
}

type SubscriptionRefreshError struct {
	Err error
}

func (s SubscriptionRefreshError) Error() string {
	return fmt.Sprintf("refresh error: %v", s.Err)
}
