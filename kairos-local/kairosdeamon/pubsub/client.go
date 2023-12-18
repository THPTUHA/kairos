package pubsub

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/THPTUHA/kairos/pkg/cbqueue"
	"github.com/THPTUHA/kairos/pkg/protocol/deliverprotocol"
)

type State string

const (
	StateDisconnected State = "disconnected"
	StateConnecting   State = "connecting"
	StateConnected    State = "connected"
	StateClosed       State = "closed"
)

type Client struct {
	futureID          uint64
	cmdID             uint32
	mu                sync.RWMutex
	endpoints         []string
	round             int
	protocolType      deliverprotocol.Type
	config            Config
	token             string
	data              deliverprotocol.Raw
	transport         transport
	state             State
	subs              map[string]*Subscription
	serverSubs        map[string]*serverSub
	requestsMu        sync.RWMutex
	requests          map[uint32]request
	receive           chan []byte
	reconnectAttempts int
	reconnectStrategy reconnectStrategy
	events            *eventHub
	delayPing         chan struct{}
	closeCh           chan struct{}
	connectFutures    map[uint64]connectFuture
	CBQueue           *cbqueue.CBQueue
	reconnectTimer    *time.Timer
	refreshTimer      *time.Timer
	refreshRequired   bool
}

func NewClient(endpoint string, config Config) *Client {
	return newClient(endpoint, config)
}

func newClient(endpoint string, config Config) *Client {
	if config.ReadTimeout == 0 {
		config.ReadTimeout = 5 * time.Second
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = time.Second
	}
	if config.HandshakeTimeout == 0 {
		config.HandshakeTimeout = time.Second
	}
	if config.MaxServerPingDelay == 0 {
		config.MaxServerPingDelay = 10 * time.Second
	}
	if config.Header == nil {
		config.Header = http.Header{}
	}
	if config.Name == "" {
		config.Name = "kairosdeamon"
	}
	// We support setting multiple endpoints to try in round-robin fashion. But
	// for now this feature is not documented and used for internal tests. In most
	// cases there should be a single public server WS endpoint.
	endpoints := strings.Split(endpoint, ",")
	if len(endpoints) == 0 {
		panic("connection endpoint required")
	}
	rand.Shuffle(len(endpoints), func(i, j int) {
		endpoints[i], endpoints[j] = endpoints[j], endpoints[i]
	})
	for _, e := range endpoints {
		if !strings.HasPrefix(e, "ws") {
			panic(fmt.Sprintf("unsupported connection endpoint: %s", e))
		}
	}

	protocolType := deliverprotocol.TypeJSON

	client := &Client{
		endpoints:         endpoints,
		config:            config,
		state:             StateDisconnected,
		protocolType:      protocolType,
		subs:              make(map[string]*Subscription),
		serverSubs:        make(map[string]*serverSub),
		requests:          make(map[uint32]request),
		reconnectStrategy: defaultBackoffReconnect,
		delayPing:         make(chan struct{}, 32),
		events:            newEventHub(),
		connectFutures:    make(map[uint64]connectFuture),
		token:             config.Token,
		data:              config.Data,
	}

	client.CBQueue = &cbqueue.CBQueue{}
	client.CBQueue.Cond = sync.NewCond(&client.CBQueue.Mu)
	go client.CBQueue.Dispatch()

	return client
}

func (c *Client) Connect() error {
	return c.startConnecting()
}

func (c *Client) Disconnect() error {
	if c.isClosed() {
		return ErrClientClosed
	}
	c.moveToDisconnected(disconnectedDisconnectCalled, "disconnect called")
	return nil
}

func (c *Client) Close() {
	if c.isClosed() {
		return
	}
	c.moveToDisconnected(disconnectedDisconnectCalled, "disconnect called")
	c.moveToClosed()
}

func (c *Client) State() State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *Client) SetToken(token string) {
	c.mu.Lock()
	c.token = token
	c.mu.Unlock()
}

func (c *Client) NewSubscription(channel string, config ...SubscriptionConfig) (*Subscription, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var sub *Subscription
	if _, ok := c.subs[channel]; ok {
		return nil, ErrDuplicateSubscription
	}
	sub = newSubscription(c, channel, config...)
	c.subs[channel] = sub
	return sub, nil
}

func (c *Client) RemoveSubscription(sub *Subscription) error {
	if sub.State() != SubStateUnsubscribed {
		return errors.New("subscription must be unsubscribed to be removed")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.subs, sub.Channel)
	return nil
}

func (c *Client) GetSubscription(channel string) (*Subscription, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.subs[channel]
	return s, ok
}

func (c *Client) Subscriptions() map[string]*Subscription {
	subs := make(map[string]*Subscription)
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, v := range c.subs {
		subs[k] = v
	}
	return subs
}

func (c *Client) Send(ctx context.Context, data []byte) error {
	if c.isClosed() {
		return ErrClientClosed
	}
	errCh := make(chan error, 1)
	c.onConnect(func(err error) {
		if err != nil {
			errCh <- err
			return
		}
		select {
		case <-ctx.Done():
			errCh <- ctx.Err()
			return
		default:
		}
		cmd := &deliverprotocol.Command{}
		params := &deliverprotocol.SendRequest{
			Data: data,
		}
		cmd.Send = params
		errCh <- c.send(cmd)
	})

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (c *Client) nextCmdID() uint32 {
	return atomic.AddUint32(&c.cmdID, 1)
}

func (c *Client) isConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state == StateConnected
}

func (c *Client) isClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state == StateClosed
}

func (c *Client) isSubscribed(channel string) bool {
	c.mu.RLock()
	_, ok := c.subs[channel]
	c.mu.RUnlock()
	return ok
}

func (c *Client) moveToDisconnected(code uint32, reason string) {
	c.mu.Lock()
	if c.state == StateDisconnected || c.state == StateClosed {
		c.mu.Unlock()
		return
	}
	if c.transport != nil {
		_ = c.transport.Close()
		c.transport = nil
	}

	prevState := c.state
	c.state = StateDisconnected
	c.clearConnectedState()
	c.resolveConnectFutures(ErrClientDisconnected)

	subsToUnsubscribe := make([]*Subscription, 0, len(c.subs))
	for _, s := range c.subs {
		s.mu.Lock()
		if s.state == SubStateUnsubscribed {
			s.mu.Unlock()
			continue
		}
		s.mu.Unlock()
		subsToUnsubscribe = append(subsToUnsubscribe, s)
	}
	serverSubsToUnsubscribe := make([]string, 0, len(c.serverSubs))
	for ch := range c.serverSubs {
		serverSubsToUnsubscribe = append(serverSubsToUnsubscribe, ch)
	}
	c.mu.Unlock()

	for _, s := range subsToUnsubscribe {
		s.moveToSubscribing(subscribingTransportClosed, "transport closed")
	}

	if prevState == StateConnected {
		var serverSubscribingHandler ServerSubscribingHandler
		if c.events != nil && c.events.onServerSubscribing != nil {
			serverSubscribingHandler = c.events.onServerSubscribing
		}
		if serverSubscribingHandler != nil {
			c.runHandlerAsync(func() {
				for _, ch := range serverSubsToUnsubscribe {
					serverSubscribingHandler(ServerSubscribingEvent{Channel: ch})
				}
			})
		}
	}

	var handler DisconnectHandler
	if c.events != nil && c.events.onDisconnected != nil {
		handler = c.events.onDisconnected
	}
	if handler != nil {
		c.runHandlerAsync(func() {
			event := DisconnectedEvent{Code: code, Reason: reason}
			handler(event)
		})
	}
}

func (c *Client) moveToConnecting(code uint32, reason string) {
	c.mu.Lock()
	if c.state == StateDisconnected || c.state == StateClosed || c.state == StateConnecting {
		c.mu.Unlock()
		return
	}
	if c.transport != nil {
		_ = c.transport.Close()
		c.transport = nil
	}

	c.state = StateConnecting
	c.clearConnectedState()
	c.resolveConnectFutures(ErrClientDisconnected)

	subsToUnsubscribe := make([]*Subscription, 0, len(c.subs))
	for _, s := range c.subs {
		s.mu.Lock()
		if s.state == SubStateUnsubscribed {
			s.mu.Unlock()
			continue
		}
		s.mu.Unlock()
		subsToUnsubscribe = append(subsToUnsubscribe, s)
	}
	serverSubsToUnsubscribe := make([]string, 0, len(c.serverSubs))
	for ch := range c.serverSubs {
		serverSubsToUnsubscribe = append(serverSubsToUnsubscribe, ch)
	}
	c.mu.Unlock()

	for _, s := range subsToUnsubscribe {
		s.moveToSubscribing(subscribingTransportClosed, "transport closed")
	}

	var serverSubscribingHandler ServerSubscribingHandler
	if c.events != nil && c.events.onServerSubscribing != nil {
		serverSubscribingHandler = c.events.onServerSubscribing
	}
	if serverSubscribingHandler != nil {
		c.runHandlerSync(func() {
			for _, ch := range serverSubsToUnsubscribe {
				serverSubscribingHandler(ServerSubscribingEvent{Channel: ch})
			}
		})
	}

	var handler ConnectingHandler
	if c.events != nil && c.events.onConnecting != nil {
		handler = c.events.onConnecting
	}
	if handler != nil {
		c.runHandlerSync(func() {
			event := ConnectingEvent{Code: code, Reason: reason}
			handler(event)
		})
	}

	c.mu.Lock()
	if c.state != StateConnecting {
		c.mu.Unlock()
		return
	}
	c.reconnectAttempts++
	reconnectDelay := c.getReconnectDelay()
	c.reconnectTimer = time.AfterFunc(reconnectDelay, func() {
		_ = c.startReconnecting()
	})
	c.mu.Unlock()
}

func (c *Client) moveToClosed() {
	c.mu.Lock()
	if c.state == StateClosed {
		c.mu.Unlock()
		return
	}
	c.state = StateClosed

	subsToUnsubscribe := make([]*Subscription, 0, len(c.subs))
	for _, s := range c.subs {
		s.mu.Lock()
		if s.state == SubStateUnsubscribed {
			s.mu.Unlock()
			continue
		}
		s.mu.Unlock()
		subsToUnsubscribe = append(subsToUnsubscribe, s)
	}
	serverSubsToUnsubscribe := make([]string, 0, len(c.serverSubs))
	for ch := range c.serverSubs {
		serverSubsToUnsubscribe = append(serverSubsToUnsubscribe, ch)
	}
	c.mu.Unlock()

	for _, s := range subsToUnsubscribe {
		s.moveToUnsubscribed(unsubscribedClientClosed, "client closed")
	}

	var serverUnsubscribedHandler ServerUnsubscribedHandler
	if c.events != nil && c.events.onServerUnsubscribed != nil {
		serverUnsubscribedHandler = c.events.onServerUnsubscribed
	}
	if serverUnsubscribedHandler != nil {
		c.runHandlerAsync(func() {
			for _, ch := range serverSubsToUnsubscribe {
				serverUnsubscribedHandler(ServerUnsubscribedEvent{Channel: ch})
			}
		})
	}

	c.mu.Lock()
	c.CBQueue.Close()
	c.CBQueue = nil
	c.mu.Unlock()
}

func (c *Client) handleError(err error) {
	var handler ErrorHandler
	if c.events != nil && c.events.onError != nil {
		handler = c.events.onError
	}
	if handler != nil {
		c.runHandlerSync(func() {
			handler(ErrorEvent{Error: err})
		})
	}
}

func (c *Client) clearConnectedState() {
	if c.reconnectTimer != nil {
		c.reconnectTimer.Stop()
		c.reconnectTimer = nil
	}
	if c.refreshTimer != nil {
		c.refreshTimer.Stop()
		c.refreshTimer = nil
	}
	if c.closeCh != nil {
		close(c.closeCh)
		c.closeCh = nil
	}

	c.requestsMu.Lock()
	reqs := make(map[uint32]request, len(c.requests))
	for uid, req := range c.requests {
		reqs[uid] = req
	}
	c.requests = make(map[uint32]request)
	c.requestsMu.Unlock()

	for _, req := range reqs {
		if req.cb != nil {
			go req.cb(nil, ErrClientDisconnected)
		}
	}
}

func (c *Client) handleDisconnect(d *disconnect) {
	if d == nil {
		d = &disconnect{
			Code:      connectingTransportClosed,
			Reason:    "transport closed",
			Reconnect: true,
		}
	}
	if d.Reconnect {
		c.moveToConnecting(d.Code, d.Reason)
	} else {
		c.moveToDisconnected(d.Code, d.Reason)
	}
}

func (c *Client) waitServerPing(disconnectCh chan struct{}, pingInterval uint32) {
	timeout := c.config.MaxServerPingDelay + time.Duration(pingInterval)*time.Second
	for {
		select {
		case <-c.delayPing:
		case <-time.After(timeout):
			fmt.Println("*******************ping timeout")
			go c.handleDisconnect(&disconnect{Code: connectingNoPing, Reason: "no ping", Reconnect: true})
		case <-disconnectCh:
			return
		}
	}
}

func (c *Client) readOnce(t transport) error {
	reply, disconnect, err := t.Read()
	if err != nil {
		go c.handleDisconnect(disconnect)
		return err
	}
	c.handle(reply)
	return nil
}

func (c *Client) reader(t transport, disconnectCh chan struct{}) {
	defer close(disconnectCh)
	for {
		err := c.readOnce(t)
		if err != nil {
			return
		}
	}
}

func (c *Client) runHandlerSync(fn func()) {
	waitCh := make(chan struct{})
	c.mu.RLock()
	c.CBQueue.Push(func(delay time.Duration) {
		defer close(waitCh)
		fn()
	})
	c.mu.RUnlock()
	<-waitCh
}

func (c *Client) runHandlerAsync(fn func()) {
	c.CBQueue.Push(func(delay time.Duration) {
		fn()
	})
}

func (c *Client) handle(reply *deliverprotocol.Reply) {
	fmt.Printf("[DEAMON HANDLE RELY FROM DELIVER]\n")
	if reply.Id > 0 {
		c.requestsMu.RLock()
		req, ok := c.requests[reply.Id]
		c.requestsMu.RUnlock()
		if ok {
			if req.cb != nil {
				req.cb(reply, nil)
			}
		}
		c.removeRequest(reply.Id)
	} else {
		if reply.Push == nil {
			select {
			case c.delayPing <- struct{}{}:
			}
			return
		}
		c.mu.Lock()
		if c.state != StateConnected {
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()
		c.handlePush(reply.Push)
	}
}

func (c *Client) handleMessage(msg *deliverprotocol.Message) error {
	var handler MessageHandler
	if c.events != nil && c.events.onMessage != nil {
		handler = c.events.onMessage
	}
	if handler != nil {
		event := MessageEvent{Data: msg.Data}
		c.runHandlerSync(func() {
			handler(event)
		})
	}
	return nil
}

func (c *Client) handlePush(push *deliverprotocol.Push) {
	channel := push.Channel
	c.mu.RLock()
	sub, ok := c.subs[channel]
	c.mu.RUnlock()
	switch {
	case push.Message != nil:
		_ = c.handleMessage(push.Message)
	case push.Unsubscribe != nil:
		if !ok {
			c.handleServerUnsub(channel, push.Unsubscribe)
			return
		}
		sub.handleUnsubscribe(push.Unsubscribe)
	case push.Pub != nil:
		if !ok {
			c.handleServerPublication(channel, push.Pub)
			return
		}
		sub.handlePublication(push.Pub)
	case push.Subscribe != nil:
		if ok {
			return
		}
		c.handleServerSub(channel, push.Subscribe)
		return
	case push.Disconnect != nil:
		code := push.Disconnect.Code
		reconnect := code < 3500 || code >= 5000 || (code >= 4000 && code < 4500)
		if reconnect {
			c.moveToConnecting(code, push.Disconnect.Reason)
		} else {
			c.moveToDisconnected(code, push.Disconnect.Reason)
		}
	default:
	}
}

func (c *Client) handleServerPublication(channel string, pub *deliverprotocol.Publication) {
	var handler ServerPublicationHandler
	if c.events != nil && c.events.onServerPublication != nil {
		handler = c.events.onServerPublication
	}
	if handler != nil {
		c.runHandlerSync(func() {
			handler(ServerPublicationEvent{Channel: channel, Publication: pubFromProto(pub)})
		})
	}
}

func (c *Client) handleServerSub(channel string, sub *deliverprotocol.Subscribe) {
	c.mu.Lock()
	_, ok := c.serverSubs[channel]
	if ok {
		c.mu.Unlock()
		return
	}
	c.serverSubs[channel] = &serverSub{}
	c.mu.Unlock()

	var handler ServerSubscribedHandler
	if c.events != nil && c.events.onServerSubscribe != nil {
		handler = c.events.onServerSubscribe
	}
	if handler != nil {
		c.runHandlerSync(func() {
			ev := ServerSubscribedEvent{
				Channel: channel,
				Data:    sub.GetData(),
			}
			handler(ev)
		})
	}
}

func (c *Client) handleServerUnsub(channel string, _ *deliverprotocol.Unsubscribe) {
	c.mu.Lock()
	_, ok := c.serverSubs[channel]
	if ok {
		delete(c.serverSubs, channel)
	}
	c.mu.Unlock()
	if !ok {
		return
	}

	var handler ServerUnsubscribedHandler
	if c.events != nil && c.events.onServerUnsubscribed != nil {
		handler = c.events.onServerUnsubscribed
	}
	if handler != nil {
		c.runHandlerSync(func() {
			handler(ServerUnsubscribedEvent{Channel: channel})
		})
	}
}

func (c *Client) getReconnectDelay() time.Duration {
	return c.reconnectStrategy.timeBeforeNextAttempt(c.reconnectAttempts)
}

func (c *Client) startReconnecting() error {
	c.mu.Lock()
	c.round++
	round := c.round
	if c.state != StateConnecting {
		c.mu.Unlock()
		return nil
	}
	refreshRequired := c.refreshRequired
	token := c.token
	getTokenFunc := c.config.GetToken
	c.mu.Unlock()

	wsConfig := websocketConfig{
		NetDialContext:    c.config.NetDialContext,
		HandshakeTimeout:  c.config.HandshakeTimeout,
		EnableCompression: c.config.EnableCompression,
		Header:            c.config.Header,
	}

	u := c.endpoints[round%len(c.endpoints)]
	t, err := newWebsocketTransport(u, c.protocolType, wsConfig)
	if err != nil {
		c.handleError(TransportError{err})
		c.mu.Lock()
		if c.state != StateConnecting {
			c.mu.Unlock()
			return nil
		}
		c.reconnectAttempts++
		reconnectDelay := c.getReconnectDelay()
		c.reconnectTimer = time.AfterFunc(reconnectDelay, func() {
			_ = c.startReconnecting()
		})
		c.mu.Unlock()
		return err
	}

	if refreshRequired || (token == "" && getTokenFunc != nil) {
		newToken, err := c.refreshToken()
		if err != nil {
			if errors.Is(err, ErrUnauthorized) {
				c.moveToDisconnected(disconnectedUnauthorized, "unauthorized")
				return nil
			}
			c.handleError(RefreshError{err})
			c.mu.Lock()
			if c.state != StateConnecting {
				_ = t.Close()
				c.mu.Unlock()
				return nil
			}
			c.reconnectAttempts++
			reconnectDelay := c.getReconnectDelay()
			c.reconnectTimer = time.AfterFunc(reconnectDelay, func() {
				_ = c.startReconnecting()
			})
			c.mu.Unlock()
			return err
		} else {
			c.mu.Lock()
			c.token = newToken
			if c.state != StateConnecting {
				c.mu.Unlock()
				return nil
			}
			c.mu.Unlock()
		}
	}

	c.mu.Lock()
	if c.state != StateConnecting {
		_ = t.Close()
		c.mu.Unlock()
		return nil
	}
	c.refreshRequired = false
	disconnectCh := make(chan struct{})
	c.receive = make(chan []byte, 64)
	c.transport = t

	go c.reader(t, disconnectCh)

	err = c.sendConnect(func(res *deliverprotocol.ConnectResult, err error) {
		c.mu.Lock()
		if c.state != StateConnecting {
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()
		if err != nil {
			c.handleError(ConnectError{err})
			_ = t.Close()
			if isTokenExpiredError(err) {
				c.mu.Lock()
				defer c.mu.Unlock()
				if c.state != StateConnecting {
					return
				}
				c.refreshRequired = true
				c.reconnectAttempts++
				reconnectDelay := c.getReconnectDelay()
				c.reconnectTimer = time.AfterFunc(reconnectDelay, func() {
					_ = c.startReconnecting()
				})
				return
			} else if isServerError(err) {
				var serverError *Error
				if errors.As(err, &serverError) {
					c.moveToDisconnected(serverError.Code, serverError.Message)
				}
				return
			} else {
				c.mu.Lock()
				defer c.mu.Unlock()
				if c.state != StateConnecting {
					return
				}
				c.reconnectAttempts++
				reconnectDelay := c.getReconnectDelay()
				c.reconnectTimer = time.AfterFunc(reconnectDelay, func() {
					_ = c.startReconnecting()
				})
				return
			}
		}
		c.mu.Lock()
		if c.state != StateConnecting {
			_ = t.Close()
			c.mu.Unlock()
			return
		}
		c.state = StateConnected

		if res.Expires {
			c.refreshTimer = time.AfterFunc(time.Duration(res.Ttl)*time.Second, c.sendRefresh)
		}
		c.resolveConnectFutures(nil)
		c.mu.Unlock()

		if c.events != nil && c.events.onConnected != nil {
			handler := c.events.onConnected
			ev := ConnectedEvent{
				ClientID: res.Client,
				Data:     res.Data,
			}
			c.runHandlerSync(func() {
				handler(ev)
			})
		}

		var subscribeHandler ServerSubscribedHandler
		if c.events != nil && c.events.onServerSubscribe != nil {
			subscribeHandler = c.events.onServerSubscribe
		}

		var publishHandler ServerPublicationHandler
		if c.events != nil && c.events.onServerPublication != nil {
			publishHandler = c.events.onServerPublication
		}

		for channel, subRes := range res.Subs {
			c.mu.Lock()
			sub, ok := c.serverSubs[channel]
			if !ok {
				sub = &serverSub{}
			}
			c.serverSubs[channel] = sub
			c.mu.Unlock()

			if subscribeHandler != nil {
				c.runHandlerSync(func() {
					ev := ServerSubscribedEvent{
						Channel: channel,
						Data:    subRes.GetData(),
					}
					subscribeHandler(ev)
				})
			}
			if publishHandler != nil {
				c.runHandlerSync(func() {
					for _, pub := range subRes.Publications {
						c.mu.Lock()
						c.serverSubs[channel] = sub
						c.mu.Unlock()
						publishHandler(ServerPublicationEvent{Channel: channel, Publication: pubFromProto(pub)})
					}
				})
			}
		}

		for ch := range c.serverSubs {
			if _, ok := res.Subs[ch]; !ok {
				var serverUnsubscribedHandler ServerUnsubscribedHandler
				if c.events != nil && c.events.onServerSubscribing != nil {
					serverUnsubscribedHandler = c.events.onServerUnsubscribed
				}
				if serverUnsubscribedHandler != nil {
					c.runHandlerSync(func() {
						serverUnsubscribedHandler(ServerUnsubscribedEvent{Channel: ch})
					})
				}
			}
		}

		c.mu.Lock()
		defer c.mu.Unlock()
		c.reconnectAttempts = 0

		if c.state != StateConnected {
			return
		}

		if res.Ping > 0 {
			go c.waitServerPing(disconnectCh, res.Ping)
		}
		c.resubscribe()
	})
	if err != nil {
		_ = t.Close()
		c.reconnectAttempts++
		reconnectDelay := c.getReconnectDelay()
		c.reconnectTimer = time.AfterFunc(reconnectDelay, func() {
			_ = c.startReconnecting()
		})
		c.handleError(ConnectError{err})
	}
	c.mu.Unlock()
	return err
}

func (c *Client) startConnecting() error {
	c.mu.Lock()
	if c.state == StateClosed {
		c.mu.Unlock()
		return ErrClientClosed
	}
	if c.state == StateConnected || c.state == StateConnecting {
		c.mu.Unlock()
		return nil
	}
	if c.closeCh == nil {
		c.closeCh = make(chan struct{})
	}
	c.state = StateConnecting
	c.mu.Unlock()

	var handler ConnectingHandler
	if c.events != nil && c.events.onConnecting != nil {
		handler = c.events.onConnecting
	}
	if handler != nil {
		c.runHandlerSync(func() {
			event := ConnectingEvent{Code: connectingConnectCalled, Reason: "connect called"}
			handler(event)
		})
	}

	return c.startReconnecting()
}

func (c *Client) resubscribe() {
	for _, sub := range c.subs {
		sub.resubscribe()
	}
}

func isTokenExpiredError(err error) bool {
	if e, ok := err.(*Error); ok && e.Code == 109 {
		return true
	}
	return false
}

func isServerError(err error) bool {
	if _, ok := err.(*Error); ok {
		return true
	}
	return false
}

func (c *Client) refreshToken() (string, error) {
	handler := c.config.GetToken
	if handler == nil {
		c.handleError(ConfigurationError{Err: errors.New("GetToken must be set to handle expired token")})
		return "", ErrUnauthorized
	}
	return handler(ConnectionTokenEvent{})
}

func (c *Client) sendRefresh() {
	token, err := c.refreshToken()
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			c.moveToDisconnected(disconnectedUnauthorized, "unauthorized")
			return
		}
		c.handleError(RefreshError{err})
		c.mu.Lock()
		defer c.mu.Unlock()
		c.handleRefreshError()
		return
	}
	c.mu.Lock()
	c.token = token
	c.mu.Unlock()

	cmd := &deliverprotocol.Command{
		Id: c.nextCmdID(),
	}
	params := &deliverprotocol.RefreshRequest{
		Token: c.token,
	}
	cmd.Refresh = params

	_ = c.sendAsync(cmd, func(r *deliverprotocol.Reply, err error) {
		if err != nil {
			c.handleError(RefreshError{err})
			c.mu.Lock()
			defer c.mu.Unlock()
			c.handleRefreshError()
			return
		}
		if r.Error != nil {
			c.mu.Lock()
			if c.state != StateConnected {
				c.mu.Unlock()
				return
			}
			c.mu.Unlock()
			c.moveToDisconnected(r.Error.Code, r.Error.Message)
			return
		}
		expires := r.Refresh.Expires
		ttl := r.Refresh.Ttl
		if expires {
			c.mu.Lock()
			if c.state == StateConnected {
				c.refreshTimer = time.AfterFunc(time.Duration(ttl)*time.Second, c.sendRefresh)
			}
			c.mu.Unlock()
		}
	})
}

// Lock must be held outside.
func (c *Client) handleRefreshError() {
	if c.state != StateConnected {
		return
	}
	c.refreshTimer = time.AfterFunc(10*time.Second, c.sendRefresh)
}

func (c *Client) sendSubRefresh(channel string, token string, fn func(*deliverprotocol.SubRefreshResult, error)) {
	cmd := &deliverprotocol.Command{
		Id: c.nextCmdID(),
	}
	params := &deliverprotocol.SubRefreshRequest{
		Channel: channel,
		Token:   token,
	}
	cmd.SubRefresh = params

	_ = c.sendAsync(cmd, func(r *deliverprotocol.Reply, err error) {
		if err != nil {
			fn(nil, err)
			return
		}
		if r.Error != nil {
			fn(nil, errorFromProto(r.Error))
			return
		}
		fn(r.SubRefresh, nil)
	})
}

func (c *Client) sendConnect(fn func(*deliverprotocol.ConnectResult, error)) error {
	cmd := &deliverprotocol.Command{
		Id: c.nextCmdID(),
	}

	req := &deliverprotocol.ConnectRequest{}
	req.Token = c.token
	req.Name = c.config.Name
	req.Data = c.data

	if len(c.serverSubs) > 0 {
		subs := make(map[string]*deliverprotocol.SubscribeRequest)
		req.Subs = subs
	}
	cmd.Connect = req

	return c.sendAsync(cmd, func(reply *deliverprotocol.Reply, err error) {
		if err != nil {
			fn(nil, err)
			return
		}
		if reply.Error != nil {
			fn(nil, errorFromProto(reply.Error))
			return
		}
		fn(reply.Connect, nil)
	})
}

func (c *Client) sendSubscribe(channel string, data []byte, token string, joinLeave bool, fn func(res *deliverprotocol.SubscribeResult, err error)) error {
	params := &deliverprotocol.SubscribeRequest{
		Channel: channel,
	}
	params.Token = token
	params.Data = data
	params.JoinLeave = joinLeave

	cmd := &deliverprotocol.Command{
		Id: c.nextCmdID(),
	}
	cmd.Subscribe = params

	return c.sendAsync(cmd, func(reply *deliverprotocol.Reply, err error) {
		if err != nil {
			fn(nil, err)
			return
		}
		if reply.Error != nil {
			fn(nil, errorFromProto(reply.Error))
			return
		}
		fn(reply.Subscribe, nil)
	})
}

func (c *Client) nextFutureID() uint64 {
	return atomic.AddUint64(&c.futureID, 1)
}

type connectFuture struct {
	fn      func(error)
	closeCh chan struct{}
}

func newConnectFuture(fn func(error)) connectFuture {
	return connectFuture{fn: fn, closeCh: make(chan struct{})}
}

func (c *Client) resolveConnectFutures(err error) {
	for _, fut := range c.connectFutures {
		fut.fn(err)
		close(fut.closeCh)
	}
	c.connectFutures = make(map[uint64]connectFuture)
}

func (c *Client) onConnect(fn func(err error)) {
	c.mu.Lock()
	if c.state == StateConnected {
		c.mu.Unlock()
		fn(nil)
	} else if c.state == StateDisconnected {
		c.mu.Unlock()
		fn(ErrClientDisconnected)
	} else {
		defer c.mu.Unlock()
		id := c.nextFutureID()
		fut := newConnectFuture(fn)
		c.connectFutures[id] = fut
		go func() {
			select {
			case <-fut.closeCh:
			case <-time.After(c.config.ReadTimeout):
				c.mu.Lock()
				defer c.mu.Unlock()
				fut, ok := c.connectFutures[id]
				if !ok {
					return
				}
				delete(c.connectFutures, id)
				fut.fn(ErrTimeout)
			}
		}()
	}
}

type PublishResult struct{}

func (c *Client) Publish(channel string, data []byte) (PublishResult, error) {
	if c.isClosed() {
		return PublishResult{}, ErrClientClosed
	}
	resCh := make(chan PublishResult, 1)
	errCh := make(chan error, 1)
	c.publish(channel, data, func(result PublishResult, err error) {
		resCh <- result
		errCh <- err
	})
	select {
	case res := <-resCh:
		fmt.Println("---------PublishResult-----", res)
		return res, <-errCh
	}
}

func (c *Client) publish(channel string, data []byte, fn func(PublishResult, error)) {
	c.onConnect(func(err error) {
		if err != nil {
			fn(PublishResult{}, err)
			return
		}
		c.sendPublish(channel, data, fn)
	})
}

func (c *Client) sendPublish(channel string, data []byte, fn func(PublishResult, error)) {
	params := &deliverprotocol.PublishRequest{
		Channel: channel,
		Data:    deliverprotocol.Raw(data),
	}

	cmd := &deliverprotocol.Command{
		Id: c.nextCmdID(),
	}
	cmd.Publish = params
	err := c.sendAsync(cmd, func(r *deliverprotocol.Reply, err error) {
		if err != nil {
			fn(PublishResult{}, err)
			return
		}
		if r.Error != nil {
			fn(PublishResult{}, errorFromProto(r.Error))
			return
		}
		fn(PublishResult{}, nil)
	})
	if err != nil {
		fn(PublishResult{}, err)
	}
}

type PresenceResult struct {
	Clients map[string]ClientInfo
}

func (c *Client) Presence(ctx context.Context, channel string) (PresenceResult, error) {
	if c.isClosed() {
		return PresenceResult{}, ErrClientClosed
	}
	resCh := make(chan PresenceResult, 1)
	errCh := make(chan error, 1)
	c.presence(ctx, channel, func(result PresenceResult, err error) {
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

func (c *Client) presence(ctx context.Context, channel string, fn func(PresenceResult, error)) {
	c.onConnect(func(err error) {
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
		c.sendPresence(channel, fn)
	})
}

func (c *Client) sendPresence(channel string, fn func(PresenceResult, error)) {
	params := &deliverprotocol.PresenceRequest{
		Channel: channel,
	}

	cmd := &deliverprotocol.Command{
		Id: c.nextCmdID(),
	}
	cmd.Presence = params

	err := c.sendAsync(cmd, func(r *deliverprotocol.Reply, err error) {
		if err != nil {
			fn(PresenceResult{}, err)
			return
		}
		if r.Error != nil {
			fn(PresenceResult{}, errorFromProto(r.Error))
			return
		}

		p := make(map[string]ClientInfo)

		for uid, info := range r.Presence.Presence {
			p[uid] = infoFromProto(info)
		}
		fn(PresenceResult{Clients: p}, nil)
	})
	if err != nil {
		fn(PresenceResult{}, err)
	}
}

type PresenceStats struct {
	NumClients int
	NumUsers   int
}

type PresenceStatsResult struct {
	PresenceStats
}

func (c *Client) PresenceStats(ctx context.Context, channel string) (PresenceStatsResult, error) {
	if c.isClosed() {
		return PresenceStatsResult{}, ErrClientClosed
	}
	resCh := make(chan PresenceStatsResult, 1)
	errCh := make(chan error, 1)
	c.presenceStats(ctx, channel, func(result PresenceStatsResult, err error) {
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

func (c *Client) presenceStats(ctx context.Context, channel string, fn func(PresenceStatsResult, error)) {
	c.onConnect(func(err error) {
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
		c.sendPresenceStats(channel, fn)
	})
}

func (c *Client) sendPresenceStats(channel string, fn func(PresenceStatsResult, error)) {
	params := &deliverprotocol.PresenceStatsRequest{
		Channel: channel,
	}

	cmd := &deliverprotocol.Command{
		Id: c.nextCmdID(),
	}
	cmd.PresenceStats = params

	err := c.sendAsync(cmd, func(r *deliverprotocol.Reply, err error) {
		if err != nil {
			fn(PresenceStatsResult{}, err)
			return
		}
		if r.Error != nil {
			fn(PresenceStatsResult{}, errorFromProto(r.Error))
			return
		}
		fn(PresenceStatsResult{PresenceStats{
			NumClients: int(r.PresenceStats.NumClients),
			NumUsers:   int(r.PresenceStats.NumUsers),
		}}, nil)
	})
	if err != nil {
		fn(PresenceStatsResult{}, err)
		return
	}
}

type UnsubscribeResult struct{}

func (c *Client) unsubscribe(channel string, fn func(UnsubscribeResult, error)) {
	if !c.isSubscribed(channel) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state != StateConnected {
		return
	}
	c.sendUnsubscribe(channel, fn)
}

func (c *Client) sendUnsubscribe(channel string, fn func(UnsubscribeResult, error)) {
	params := &deliverprotocol.UnsubscribeRequest{
		Channel: channel,
	}

	cmd := &deliverprotocol.Command{
		Id: c.nextCmdID(),
	}
	cmd.Unsubscribe = params

	err := c.sendAsync(cmd, func(r *deliverprotocol.Reply, err error) {
		if err != nil {
			fn(UnsubscribeResult{}, err)
			return
		}
		if r.Error != nil {
			fn(UnsubscribeResult{}, errorFromProto(r.Error))
			return
		}
		fn(UnsubscribeResult{}, nil)
	})
	if err != nil {
		fn(UnsubscribeResult{}, err)
	}
}

func (c *Client) sendAsync(cmd *deliverprotocol.Command, cb func(*deliverprotocol.Reply, error)) error {
	c.addRequest(cmd.Id, cb)

	err := c.send(cmd)
	if err != nil {
		return err
	}
	go func() {
		c.mu.Lock()
		closeCh := c.closeCh
		c.mu.Unlock()
		defer c.removeRequest(cmd.Id)
		select {
		case <-time.After(c.config.ReadTimeout):
			c.requestsMu.RLock()
			req, ok := c.requests[cmd.Id]
			c.requestsMu.RUnlock()
			if !ok {
				return
			}
			fmt.Println("-----------timeout----", cmd.String())
			req.cb(nil, ErrTimeout)
		case <-closeCh:
			c.requestsMu.RLock()
			req, ok := c.requests[cmd.Id]
			c.requestsMu.RUnlock()
			if !ok {
				return
			}
			req.cb(nil, ErrClientDisconnected)
		}
	}()
	return nil
}

func (c *Client) send(cmd *deliverprotocol.Command) error {
	transport := c.transport
	if transport == nil {
		return ErrClientDisconnected
	}
	err := transport.Write(cmd, c.config.WriteTimeout)
	if err != nil {
		go c.handleDisconnect(&disconnect{Code: connectingTransportClosed, Reason: "write error", Reconnect: true})
		return io.EOF
	}
	return nil
}

type request struct {
	cb func(*deliverprotocol.Reply, error)
}

func (c *Client) addRequest(id uint32, cb func(*deliverprotocol.Reply, error)) {
	c.requestsMu.Lock()
	defer c.requestsMu.Unlock()
	c.requests[id] = request{cb}
}

func (c *Client) removeRequest(id uint32) {
	c.requestsMu.Lock()
	defer c.requestsMu.Unlock()
	delete(c.requests, id)
}

type disconnect struct {
	Code      uint32
	Reason    string
	Reconnect bool
}

type serverSub struct{}
