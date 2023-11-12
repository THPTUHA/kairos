package pubsub

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/centrifugal/protocol"
	"github.com/rs/zerolog/log"
)

// Different states of Subscription.
const (
	SubStateUnsubscribed SubState = "unsubscribed"
	SubStateSubscribing  SubState = "subscribing"
	SubStateSubscribed   SubState = "subscribed"
)

// SubState represents state of Subscription.
type SubState string

type subFuture struct {
	fn      func(error)
	closeCh chan struct{}
}

// Subscription represents client subscription to channel. DO NOT initialize this struct
// directly, instead use Client.NewSubscription method to create channel subscriptions.
type Subscription struct {
	futureID uint64 // Keep atomic on top!

	mu     sync.RWMutex
	client *Client

	// Channel for a subscription.
	Channel string

	state SubState

	events     *subscriptionEventHub
	offset     uint64
	epoch      string
	recover    bool
	subFutures map[uint64]subFuture
	data       []byte

	positioned  bool
	recoverable bool
	joinLeave   bool

	token    string
	getToken func(SubscriptionTokenEvent) (string, error)

	resubscribeAttempts int
	resubscribeStrategy reconnectStrategy

	resubscribeTimer *time.Timer
	refreshTimer     *time.Timer
}

type SubscriptionConfig struct {
	// Data is an arbitrary data to pass to a server in each subscribe request.
	Data []byte
	// Token for Subscription.
	Token string
	// GetToken called to get or refresh private channel subscription token.
	GetToken func(SubscriptionTokenEvent) (string, error)
	// Positioned flag asks server to make Subscription positioned. Only makes sense
	// in channels with history stream on.
	Positioned bool
	// Recoverable flag asks server to make Subscription recoverable. Only makes sense
	// in channels with history stream on.
	Recoverable bool
	// JoinLeave flag asks server to push join/leave messages.
	JoinLeave bool
}

func newSubscription(c *Client, channel string, config ...SubscriptionConfig) *Subscription {
	s := &Subscription{
		Channel:             channel,
		client:              c,
		state:               SubStateUnsubscribed,
		events:              newSubscriptionEventHub(),
		subFutures:          make(map[uint64]subFuture),
		resubscribeStrategy: defaultBackoffReconnect,
	}
	if len(config) == 1 {
		cfg := config[0]
		s.token = cfg.Token
		s.getToken = cfg.GetToken
		s.data = cfg.Data
		s.positioned = cfg.Positioned
		s.recoverable = cfg.Recoverable
		s.joinLeave = cfg.JoinLeave
	}
	return s
}

func (s *Subscription) moveToSubscribing(code uint32, reason string) {
	s.mu.Lock()
	s.resubscribeAttempts = 0
	if s.resubscribeTimer != nil {
		s.resubscribeTimer.Stop()
	}
	if s.refreshTimer != nil {
		s.refreshTimer.Stop()
	}
	needEvent := s.state != SubStateSubscribing
	s.state = SubStateSubscribing
	s.mu.Unlock()

	if needEvent && s.events != nil && s.events.onSubscribing != nil {
		handler := s.events.onSubscribing
		s.client.runHandlerAsync(func() {
			handler(SubscribingEvent{
				Code:   code,
				Reason: reason,
			})
		})
	}
}

func (s *Subscription) Subscribe() error {
	if s.client.isClosed() {
		return ErrClientClosed
	}
	s.mu.Lock()
	if s.state == SubStateSubscribed || s.state == SubStateSubscribing {
		s.mu.Unlock()
		return nil
	}
	s.state = SubStateSubscribing
	s.mu.Unlock()

	if s.events != nil && s.events.onSubscribing != nil {
		handler := s.events.onSubscribing
		s.client.runHandlerAsync(func() {
			handler(SubscribingEvent{
				Code:   subscribingSubscribeCalled,
				Reason: "subscribe called",
			})
		})
	}

	if !s.client.isConnected() {
		return nil
	}
	s.resubscribe()
	return nil
}

// Lock must be held outside.
func (s *Subscription) resolveSubFutures(err error) {
	for _, fut := range s.subFutures {
		fut.fn(err)
		close(fut.closeCh)
	}
	s.subFutures = make(map[uint64]subFuture)
}

func (s *Subscription) moveToSubscribed(res *protocol.SubscribeResult) {
	log.Debug().Msg("Start moveToSubscribed ")
	s.mu.Lock()
	if s.state != SubStateSubscribing {
		s.mu.Unlock()
		return
	}
	s.state = SubStateSubscribed
	if res.Expires {
		s.scheduleSubRefresh(res.Ttl)
	}
	if res.Recoverable {
		s.recover = true
	}
	s.resubscribeAttempts = 0
	if s.resubscribeTimer != nil {
		s.resubscribeTimer.Stop()
	}
	s.resolveSubFutures(nil)
	s.offset = res.Offset
	s.epoch = res.Epoch
	s.mu.Unlock()

	if s.events != nil && s.events.onSubscribed != nil {
		handler := s.events.onSubscribed
		ev := SubscribedEvent{
			Data:          res.GetData(),
			Recovered:     res.GetRecovered(),
			WasRecovering: res.GetWasRecovering(),
			Recoverable:   res.GetRecoverable(),
			Positioned:    res.GetPositioned(),
		}
		if ev.Positioned || ev.Recoverable {
			ev.StreamPosition = &StreamPosition{
				Epoch:  res.GetEpoch(),
				Offset: res.GetOffset(),
			}
		}
		s.client.runHandlerSync(func() {
			log.Debug().Msg("----runHandlerSync")

			handler(ev)
		})
	}

	if len(res.Publications) > 0 {
		s.client.runHandlerSync(func() {
			pubs := res.Publications
			for i := 0; i < len(pubs); i++ {
				pub := res.Publications[i]
				s.mu.Lock()
				if s.state != SubStateSubscribed {
					s.mu.Unlock()
					return
				}
				if pub.Offset > 0 {
					s.offset = pub.Offset
				}
				s.mu.Unlock()
				var handler PublicationHandler
				if s.events != nil && s.events.onPublication != nil {
					handler = s.events.onPublication
				}
				if handler != nil {
					handler(PublicationEvent{Publication: pubFromProto(pub)})
				}
			}
		})
	}
}

func (s *Subscription) moveToUnsubscribed(code uint32, reason string) {
	s.mu.Lock()
	s.resubscribeAttempts = 0
	if s.resubscribeTimer != nil {
		s.resubscribeTimer.Stop()
	}
	if s.refreshTimer != nil {
		s.refreshTimer.Stop()
	}

	needEvent := s.state != SubStateUnsubscribed
	s.state = SubStateUnsubscribed
	s.mu.Unlock()

	if needEvent && s.events != nil && s.events.onUnsubscribe != nil {
		handler := s.events.onUnsubscribe
		s.client.runHandlerAsync(func() {
			handler(UnsubscribedEvent{
				Code:   code,
				Reason: reason,
			})
		})
	}
}

func (s *Subscription) getSubscriptionToken(channel string) (string, error) {
	handler := s.getToken
	if handler != nil {
		ev := SubscriptionTokenEvent{
			Channel: channel,
		}
		return handler(ev)
	}
	return "", errors.New("GetToken must be set to get subscription token")
}

func (s *Subscription) unsubscribe(code uint32, reason string, sendUnsubscribe bool) {
	s.moveToUnsubscribed(code, reason)
	if sendUnsubscribe {
		s.client.unsubscribe(s.Channel, func(result UnsubscribeResult, err error) {
			if err != nil {
				go s.client.handleDisconnect(&disconnect{Code: connectingUnsubscribeError, Reason: "unsubscribe error", Reconnect: true})
				return
			}
		})
	}
}

func (s *Subscription) scheduleSubRefresh(ttl uint32) {
	if s.state != SubStateSubscribed {
		return
	}
	s.refreshTimer = time.AfterFunc(time.Duration(ttl)*time.Second, func() {
		s.mu.Lock()
		if s.state != SubStateSubscribed {
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()

		token, err := s.getSubscriptionToken(s.Channel)
		if err != nil {
			if errors.Is(err, ErrUnauthorized) {
				s.unsubscribe(unsubscribedUnauthorized, "unauthorized", true)
				return
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			s.emitError(SubscriptionRefreshError{Err: err})
			s.scheduleSubRefresh(10)
			return
		}
		if token == "" {
			s.unsubscribe(unsubscribedUnauthorized, "unauthorized", true)
			return
		}

		s.client.sendSubRefresh(s.Channel, token, func(result *protocol.SubRefreshResult, err error) {
			if err != nil {
				s.emitError(SubscriptionSubscribeError{Err: err})
				var serverError *Error
				if errors.As(err, &serverError) {
					if serverError.Temporary {
						s.mu.Lock()
						defer s.mu.Unlock()
						s.scheduleSubRefresh(10)
						return
					} else {
						s.unsubscribe(serverError.Code, serverError.Message, true)
						return
					}
				} else {
					s.mu.Lock()
					defer s.mu.Unlock()
					s.scheduleSubRefresh(10)
					return
				}
			}
			if result.Expires {
				s.mu.Lock()
				s.scheduleSubRefresh(result.Ttl)
				s.mu.Unlock()
			}
		})
	})
}

func (s *Subscription) emitError(err error) {
	if s.events != nil && s.events.onError != nil {
		handler := s.events.onError
		s.client.runHandlerSync(func() {
			handler(SubscriptionErrorEvent{Error: err})
		})
	}
}

func (s *Subscription) scheduleResubscribe() {
	delay := s.resubscribeStrategy.timeBeforeNextAttempt(s.resubscribeAttempts)
	s.resubscribeAttempts++
	s.resubscribeTimer = time.AfterFunc(delay, func() {
		s.mu.Lock()
		if s.state != SubStateSubscribing {
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		s.resubscribe()
	})
}
func (s *Subscription) subscribeError(err error) {
	s.mu.Lock()
	if s.state != SubStateSubscribing {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	if err == ErrTimeout {
		go s.client.handleDisconnect(&disconnect{Code: connectingSubscribeTimeout, Reason: "subscribe timeout", Reconnect: true})
		return
	}

	s.emitError(SubscriptionSubscribeError{Err: err})

	var serverError *Error
	if errors.As(err, &serverError) {
		if serverError.Code == 109 { // Token expired.
			s.mu.Lock()
			s.token = ""
			s.scheduleResubscribe()
			s.mu.Unlock()
		} else if serverError.Temporary {
			s.mu.Lock()
			s.scheduleResubscribe()
			s.mu.Unlock()
		} else {
			s.mu.Lock()
			s.resolveSubFutures(err)
			s.mu.Unlock()
			s.unsubscribe(serverError.Code, serverError.Message, false)
		}
	} else {
		s.mu.Lock()
		s.scheduleResubscribe()
		s.mu.Unlock()
	}
}

func (s *Subscription) resubscribe() {
	log.Debug().Msg("Start resubscribe")
	s.mu.Lock()
	if s.state != SubStateSubscribing {
		s.mu.Unlock()
		return
	}
	token := s.token
	s.mu.Unlock()

	if token == "" && s.getToken != nil {
		var err error
		token, err = s.getSubscriptionToken(s.Channel)
		if err != nil {
			if errors.Is(err, ErrUnauthorized) {
				s.unsubscribe(unsubscribedUnauthorized, "unauthorized", false)
				return
			}
			s.subscribeError(err)
			return
		}
		s.mu.Lock()
		if token == "" {
			s.mu.Unlock()
			s.unsubscribe(unsubscribedUnauthorized, "unauthorized", false)
			return
		}
		s.token = token
		s.mu.Unlock()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != SubStateSubscribing {
		return
	}

	var isRecover bool
	var sp StreamPosition
	if s.recover {
		isRecover = true
		sp.Offset = s.offset
		sp.Epoch = s.epoch
	}

	log.Debug().Msg("sendSubscribe")
	err := s.client.sendSubscribe(s.Channel, s.data, isRecover, sp, token, s.positioned, s.recoverable, s.joinLeave, func(res *protocol.SubscribeResult, err error) {
		if err != nil {
			log.Error().Stack().Err(err).Msg(" subscribe error")

			s.subscribeError(err)
			return
		}
		s.moveToSubscribed(res)
	})
	if err != nil {
		s.scheduleResubscribe()
	}
}

func (s *Subscription) handlePublication(pub *protocol.Publication) {
	s.mu.Lock()
	if s.state != SubStateSubscribed {
		s.mu.Unlock()
		return
	}
	if pub.Offset > 0 {
		s.offset = pub.Offset
	}
	s.mu.Unlock()

	var handler PublicationHandler
	if s.events != nil && s.events.onPublication != nil {
		handler = s.events.onPublication
	}
	if handler == nil {
		return
	}
	s.client.runHandlerSync(func() {
		handler(PublicationEvent{Publication: pubFromProto(pub)})
	})
}

type PublishResult struct{}

func (s *Subscription) Publish(ctx context.Context, data []byte) (PublishResult, error) {
	s.mu.Lock()
	if s.state == SubStateUnsubscribed {
		s.mu.Unlock()
		return PublishResult{}, ErrSubscriptionUnsubscribed
	}
	s.mu.Unlock()

	resCh := make(chan PublishResult, 1)
	errCh := make(chan error, 1)
	s.publish(ctx, data, func(result PublishResult, err error) {
		resCh <- result
		errCh <- err
	})
	select {
	case <-ctx.Done():
		return PublishResult{}, ctx.Err()
	case res := <-resCh:
		return res, <-errCh
	}
}

func (s *Subscription) publish(ctx context.Context, data []byte, fn func(PublishResult, error)) {
	s.onSubscribe(func(err error) {
		select {
		case <-ctx.Done():
			fn(PublishResult{}, ctx.Err())
			return
		default:
		}
		if err != nil {
			fn(PublishResult{}, err)
			return
		}
		s.client.publish(ctx, s.Channel, data, fn)
	})
}

func newSubFuture(fn func(error)) subFuture {
	return subFuture{fn: fn, closeCh: make(chan struct{})}
}

func (s *Subscription) nextFutureID() uint64 {
	return atomic.AddUint64(&s.futureID, 1)
}

func (s *Subscription) onSubscribe(fn func(err error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == SubStateSubscribed {
		go fn(nil)
	} else if s.state == SubStateUnsubscribed {
		go fn(ErrSubscriptionUnsubscribed)
	} else {
		id := s.nextFutureID()
		fut := newSubFuture(fn)
		s.subFutures[id] = fut
		go func() {
			select {
			case <-fut.closeCh:
			case <-time.After(s.client.config.ReadTimeout):
				s.mu.Lock()
				defer s.mu.Unlock()
				fut, ok := s.subFutures[id]
				if !ok {
					return
				}
				delete(s.subFutures, id)
				fut.fn(ErrTimeout)
			}
		}()
	}
}

func (s *Subscription) Unsubscribe() error {
	if s.client.isClosed() {
		return ErrClientClosed
	}
	s.unsubscribe(unsubscribedUnsubscribeCalled, "unsubscribe called", true)
	return nil
}
