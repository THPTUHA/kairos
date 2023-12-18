package pubsub

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/THPTUHA/kairos/pkg/protocol/deliverprotocol"
)

type SubState string

const (
	SubStateUnsubscribed SubState = "unsubscribed"
	SubStateSubscribing  SubState = "subscribing"
	SubStateSubscribed   SubState = "subscribed"
)

type SubscriptionConfig struct {
	Data     []byte
	Token    string
	GetToken func(SubscriptionTokenEvent) (string, error)
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
	}
	return s
}

type Subscription struct {
	futureID uint64

	mu     sync.RWMutex
	client *Client

	Channel string

	state SubState

	events     *subscriptionEventHub
	subFutures map[uint64]subFuture
	data       []byte

	joinLeave bool

	token    string
	getToken func(SubscriptionTokenEvent) (string, error)

	resubscribeAttempts int
	resubscribeStrategy reconnectStrategy

	resubscribeTimer *time.Timer
	refreshTimer     *time.Timer
}

func (s *Subscription) State() SubState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

type subFuture struct {
	fn      func(error)
	closeCh chan struct{}
}

func newSubFuture(fn func(error)) subFuture {
	return subFuture{fn: fn, closeCh: make(chan struct{})}
}

func (s *Subscription) nextFutureID() uint64 {
	return atomic.AddUint64(&s.futureID, 1)
}

func (s *Subscription) resolveSubFutures(err error) {
	for _, fut := range s.subFutures {
		fut.fn(err)
		close(fut.closeCh)
	}
	s.subFutures = make(map[uint64]subFuture)
}

func (s *Subscription) Publish(ctx context.Context, data []byte) (PublishResult, error) {
	s.mu.Lock()
	if s.state == SubStateUnsubscribed {
		s.mu.Unlock()
		return PublishResult{}, ErrSubscriptionUnsubscribed
	}
	s.mu.Unlock()

	resCh := make(chan PublishResult, 1)
	errCh := make(chan error, 1)
	s.publish(data, func(result PublishResult, err error) {
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

func (s *Subscription) Presence(ctx context.Context) (PresenceResult, error) {
	s.mu.Lock()
	if s.state == SubStateUnsubscribed {
		s.mu.Unlock()
		return PresenceResult{}, ErrSubscriptionUnsubscribed
	}
	s.mu.Unlock()

	resCh := make(chan PresenceResult, 1)
	errCh := make(chan error, 1)
	s.presence(ctx, func(result PresenceResult, err error) {
		resCh <- result
		errCh <- err
	})
	select {
	case <-ctx.Done():
		return PresenceResult{}, ctx.Err()
	case res := <-resCh:
		return res, <-errCh
	}
}

func (s *Subscription) PresenceStats(ctx context.Context) (PresenceStatsResult, error) {
	s.mu.Lock()
	if s.state == SubStateUnsubscribed {
		s.mu.Unlock()
		return PresenceStatsResult{}, ErrSubscriptionUnsubscribed
	}
	s.mu.Unlock()

	resCh := make(chan PresenceStatsResult, 1)
	errCh := make(chan error, 1)
	s.presenceStats(ctx, func(result PresenceStatsResult, err error) {
		resCh <- result
		errCh <- err
	})
	select {
	case <-ctx.Done():
		return PresenceStatsResult{}, ctx.Err()
	case res := <-resCh:
		return res, <-errCh
	}
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

func (s *Subscription) publish(data []byte, fn func(PublishResult, error)) {
	s.onSubscribe(func(err error) {
		if err != nil {
			fn(PublishResult{}, err)
			return
		}
		s.client.publish(s.Channel, data, fn)
	})
}

func (s *Subscription) presence(ctx context.Context, fn func(PresenceResult, error)) {
	s.onSubscribe(func(err error) {
		select {
		case <-ctx.Done():
			fn(PresenceResult{}, ctx.Err())
			return
		default:
		}
		if err != nil {
			fn(PresenceResult{}, err)
			return
		}
		s.client.presence(ctx, s.Channel, fn)
	})
}

func (s *Subscription) presenceStats(ctx context.Context, fn func(PresenceStatsResult, error)) {
	s.onSubscribe(func(err error) {
		select {
		case <-ctx.Done():
			fn(PresenceStatsResult{}, ctx.Err())
			return
		default:
		}
		if err != nil {
			fn(PresenceStatsResult{}, err)
			return
		}
		s.client.presenceStats(ctx, s.Channel, fn)
	})
}

func (s *Subscription) Unsubscribe() error {
	if s.client.isClosed() {
		return ErrClientClosed
	}
	s.unsubscribe(unsubscribedUnsubscribeCalled, "unsubscribe called", true)
	return nil
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

func (s *Subscription) moveToSubscribed(res *deliverprotocol.SubscribeResult) {
	s.mu.Lock()
	if s.state != SubStateSubscribing {
		s.mu.Unlock()
		return
	}
	s.state = SubStateSubscribed
	if res.Expires {
		s.scheduleSubRefresh(res.Ttl)
	}
	s.resubscribeAttempts = 0
	if s.resubscribeTimer != nil {
		s.resubscribeTimer.Stop()
	}
	s.resolveSubFutures(nil)
	s.mu.Unlock()

	if s.events != nil && s.events.onSubscribed != nil {
		handler := s.events.onSubscribed
		ev := SubscribedEvent{
			Data: res.GetData(),
		}
		s.client.runHandlerSync(func() {
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

// Lock must be held outside.
func (s *Subscription) emitError(err error) {
	if s.events != nil && s.events.onError != nil {
		handler := s.events.onError
		s.client.runHandlerSync(func() {
			handler(SubscriptionErrorEvent{Error: err})
		})
	}
}

func (s *Subscription) handlePublication(pub *deliverprotocol.Publication) {
	s.mu.Lock()
	if s.state != SubStateSubscribed {
		s.mu.Unlock()
		return
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

func (s *Subscription) handleJoin(info *deliverprotocol.ClientInfo) {
	var handler JoinHandler
	if s.events != nil && s.events.onJoin != nil {
		handler = s.events.onJoin
	}
	if handler != nil {
		s.client.runHandlerSync(func() {
			handler(JoinEvent{ClientInfo: infoFromProto(info)})
		})
	}
}

func (s *Subscription) handleLeave(info *deliverprotocol.ClientInfo) {
	var handler LeaveHandler
	if s.events != nil && s.events.onLeave != nil {
		handler = s.events.onLeave
	}
	if handler != nil {
		s.client.runHandlerSync(func() {
			handler(LeaveEvent{ClientInfo: infoFromProto(info)})
		})
	}
}

func (s *Subscription) handleUnsubscribe(unsubscribe *deliverprotocol.Unsubscribe) {
	if unsubscribe.Code < 2500 {
		s.moveToUnsubscribed(unsubscribe.Code, unsubscribe.Reason)
	} else {
		s.moveToSubscribing(unsubscribe.Code, unsubscribe.Reason)
		s.resubscribe()
	}
}

func (s *Subscription) resubscribe() {
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

	err := s.client.sendSubscribe(s.Channel, s.data, token, s.joinLeave, func(res *deliverprotocol.SubscribeResult, err error) {
		if err != nil {
			s.subscribeError(err)
			return
		}
		s.moveToSubscribed(res)
	})
	if err != nil {
		s.scheduleResubscribe()
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

		s.client.sendSubRefresh(s.Channel, token, func(result *deliverprotocol.SubRefreshResult, err error) {
			if err != nil {
				s.emitError(SubscriptionSubscribeError{Err: err})
				var serverError *Error
				if errors.As(err, &serverError) {
					s.unsubscribe(serverError.Code, serverError.Message, true)
					return
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
