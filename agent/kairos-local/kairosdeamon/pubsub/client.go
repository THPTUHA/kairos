package pubsub

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/centrifugal/protocol"
	"github.com/rs/zerolog/log"
)

// State of client connection.
type State string

// Describe client connection states.
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
	protocolType      protocol.Type
	config            Config
	token             string
	data              protocol.Raw
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
	sendPong          bool
	delayPing         chan struct{}
	closeCh           chan struct{}
	connectFutures    map[uint64]connectFuture
	cbQueue           *cbQueue
	reconnectTimer    *time.Timer
	refreshTimer      *time.Timer
	refreshRequired   bool
}

func newClient(endpoint string, isProtobuf bool, config Config) *Client {
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
		config.Name = "go"
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

	protocolType := protocol.TypeJSON
	if isProtobuf {
		protocolType = protocol.TypeProtobuf
	}

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

	// Queue to run callbacks on.
	client.cbQueue = &cbQueue{}
	client.cbQueue.cond = sync.NewCond(&client.cbQueue.mu)
	go client.cbQueue.dispatch()

	return client
}

// Connect dials to server and sends connect message. Will return an error if first
// dial with a server failed. In case of failure client will automatically reconnect.
// To temporary disconnect from a server call Client.Disconnect.
func (c *Client) Connect() error {
	return c.startConnecting()
}

func (c *Client) runHandlerSync(fn func()) {
	waitCh := make(chan struct{})
	c.mu.RLock()
	c.cbQueue.push(func(delay time.Duration) {
		defer close(waitCh)
		fn()
	})
	c.mu.RUnlock()
	<-waitCh
}

func (c *Client) startConnecting() error {
	log.Debug().Msg("startConnecting")
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

func (c *Client) getReconnectDelay() time.Duration {
	return c.reconnectStrategy.timeBeforeNextAttempt(c.reconnectAttempts)
}

func (c *Client) refreshToken() (string, error) {
	handler := c.config.GetToken
	if handler == nil {
		c.handleError(ConfigurationError{Err: errors.New("GetToken must be set to handle expired token")})
		return "", ErrUnauthorized
	}
	return handler(ConnectionTokenEvent{})
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

func (c *Client) runHandlerAsync(fn func()) {
	c.cbQueue.push(func(delay time.Duration) {
		fn()
	})
}

// lock must be held outside.
func (c *Client) resolveConnectFutures(err error) {
	for _, fut := range c.connectFutures {
		fut.fn(err)
		close(fut.closeCh)
	}
	c.connectFutures = make(map[uint64]connectFuture)
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

func (c *Client) readOnce(t transport) error {
	reply, disconnect, err := t.Read()
	if err != nil {
		log.Error().Stack().Err(err).Msg("read one")
		go c.handleDisconnect(disconnect)
		return err
	}
	c.handle(reply)
	return nil
}

func (c *Client) handle(reply *protocol.Reply) {
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
		log.Info().Msg("hanle reply  clone from server")
		if reply.Push == nil {
			// Ping from server, send pong if needed.
			select {
			case c.delayPing <- struct{}{}:
			default:
			}
			c.mu.RLock()
			sendPong := c.sendPong
			c.mu.RUnlock()
			if sendPong {
				cmd := &protocol.Command{}
				_ = c.send(cmd)
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

// TODO
func (c *Client) handlePush(push *protocol.Push) {
	channel := push.Channel
	c.mu.RLock()
	sub, ok := c.subs[channel]
	c.mu.RUnlock()
	switch {
	case push.Message != nil:
		_ = c.handleMessage(push.Message)
	case push.Pub != nil:
		if !ok {
			c.handleServerPublication(channel, push.Pub)
			return
		}
		sub.handlePublication(push.Pub)
	// case push.Unsubscribe != nil:
	// 	if !ok {
	// 		c.handleServerUnsub(channel, push.Unsubscribe)
	// 		return
	// 	}
	// 	sub.handleUnsubscribe(push.Unsubscribe)
	// case push.Join != nil:
	// 	if !ok {
	// 		c.handleServerJoin(channel, push.Join)
	// 		return
	// 	}
	// 	sub.handleJoin(push.Join.Info)
	// case push.Leave != nil:
	// 	if !ok {
	// 		c.handleServerLeave(channel, push.Leave)
	// 		return
	// 	}
	// 	sub.handleLeave(push.Leave.Info)
	case push.Subscribe != nil:
		if ok {
			// Client-side subscription exists.
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

func (c *Client) handleServerPublication(channel string, pub *protocol.Publication) {
	c.mu.Lock()
	serverSub, ok := c.serverSubs[channel]
	if !ok {
		c.mu.Unlock()
		return
	}
	if serverSub.Recoverable && pub.Offset > 0 {
		serverSub.Offset = pub.Offset
	}
	c.mu.Unlock()

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

func (c *Client) handleServerSub(channel string, sub *protocol.Subscribe) {
	c.mu.Lock()
	_, ok := c.serverSubs[channel]
	if ok {
		c.mu.Unlock()
		return
	}
	c.serverSubs[channel] = &serverSub{
		Offset:      sub.Offset,
		Epoch:       sub.Epoch,
		Recoverable: sub.Recoverable,
	}
	c.mu.Unlock()

	var handler ServerSubscribedHandler
	if c.events != nil && c.events.onServerSubscribe != nil {
		handler = c.events.onServerSubscribe
	}
	if handler != nil {
		c.runHandlerSync(func() {
			ev := ServerSubscribedEvent{
				Channel:     channel,
				Positioned:  sub.GetPositioned(),
				Recoverable: sub.GetRecoverable(),
				Data:        sub.GetData(),
			}
			if ev.Positioned || ev.Recoverable {
				ev.StreamPosition = &StreamPosition{
					Epoch:  sub.GetEpoch(),
					Offset: sub.GetOffset(),
				}
			}
			handler(ev)
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

func (c *Client) reader(t transport, disconnectCh chan struct{}) {
	defer close(disconnectCh)
	for {
		err := c.readOnce(t)
		if err != nil {
			return
		}
	}
}

func (c *Client) startReconnecting() error {
	log.Debug().Msg("startReconnecting")
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
		TLSConfig:         c.config.TLSConfig,
		HandshakeTimeout:  c.config.HandshakeTimeout,
		EnableCompression: c.config.EnableCompression,
		CookieJar:         c.config.CookieJar,
		Header:            c.config.Header,
	}

	u := c.endpoints[round%len(c.endpoints)]
	t, err := newWebsocketTransport(u, c.protocolType, wsConfig)
	if err != nil {
		log.Error().Stack().Err(err).Msg("reconnect 1")
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
			log.Error().Stack().Err(err).Msg("reconnect 2")
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

	err = c.sendConnect(func(res *protocol.ConnectResult, err error) {
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
				log.Error().Stack().Err(err).Msg("reconnect 3")

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
			} else if isServerError(err) && !isTemporaryError(err) {
				log.Error().Stack().Err(err).Msg("reconnect 5")
				var serverError *Error
				if errors.As(err, &serverError) {
					c.moveToDisconnected(serverError.Code, serverError.Message)
				}
				return
			} else {
				log.Error().Stack().Err(err).Msg("reconnect 4")
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
				Version:  res.Version,
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
			if ok {
				sub.Epoch = subRes.Epoch
				sub.Recoverable = subRes.Recoverable
			} else {
				sub = &serverSub{
					Epoch:       subRes.Epoch,
					Offset:      subRes.Offset,
					Recoverable: subRes.Recoverable,
				}
			}
			if len(subRes.Publications) == 0 {
				sub.Offset = subRes.Offset
			}
			c.serverSubs[channel] = sub
			c.mu.Unlock()

			if subscribeHandler != nil {
				c.runHandlerSync(func() {
					ev := ServerSubscribedEvent{
						Channel:       channel,
						Data:          subRes.GetData(),
						Recovered:     subRes.GetRecovered(),
						WasRecovering: subRes.GetWasRecovering(),
						Positioned:    subRes.GetPositioned(),
						Recoverable:   subRes.GetRecoverable(),
					}
					if ev.Positioned || ev.Recoverable {
						ev.StreamPosition = &StreamPosition{
							Epoch:  subRes.GetEpoch(),
							Offset: subRes.GetOffset(),
						}
					}
					subscribeHandler(ev)
				})
			}
			if publishHandler != nil {
				c.runHandlerSync(func() {
					for _, pub := range subRes.Publications {
						c.mu.Lock()
						if sub, ok := c.serverSubs[channel]; ok {
							sub.Offset = pub.Offset
						}
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
		// Successfully connected – can reset reconnect attempts.
		c.reconnectAttempts = 0

		if c.state != StateConnected {
			return
		}

		if res.Ping > 0 {
			c.sendPong = res.Pong
			log.Info().Msg(fmt.Sprintf("ping interval %d\n", res.Ping))
			go c.waitServerPing(disconnectCh, res.Ping)
		}
		c.resubscribe()
	})
	if err != nil {
		log.Error().Stack().Err(err).Msg("reconnect 5")
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

func (c *Client) resubscribe() {
	for _, sub := range c.subs {
		sub.resubscribe()
	}
}

func (c *Client) waitServerPing(disconnectCh chan struct{}, pingInterval uint32) {
	log.Info().Msg("wait server ping")
	timeout := c.config.MaxServerPingDelay + time.Duration(pingInterval)*time.Second
	for {
		select {
		case <-c.delayPing:
			log.Info().Msg("reciver ping from server")
		case <-time.After(timeout):
			log.Error().Msg("time out server ping")
			go c.handleDisconnect(&disconnect{Code: connectingNoPing, Reason: "no ping", Reconnect: true})
		case <-disconnectCh:
			return
		}
	}
}

// TODO
func (c *Client) sendRefresh() {
	// token, err := c.refreshToken()
	// if err != nil {
	// 	if errors.Is(err, ErrUnauthorized) {
	// 		c.moveToDisconnected(disconnectedUnauthorized, "unauthorized")
	// 		return
	// 	}
	// 	c.handleError(RefreshError{err})
	// 	c.mu.Lock()
	// 	defer c.mu.Unlock()
	// 	c.handleRefreshError()
	// 	return
	// }
	// c.mu.Lock()
	// c.token = token
	// c.mu.Unlock()

	// cmd := &protocol.Command{
	// 	Id: c.nextCmdID(),
	// }
	// params := &protocol.RefreshRequest{
	// 	Token: c.token,
	// }
	// cmd.Refresh = params

	// _ = c.sendAsync(cmd, func(r *protocol.Reply, err error) {
	// 	if err != nil {
	// 		c.handleError(RefreshError{err})
	// 		c.mu.Lock()
	// 		defer c.mu.Unlock()
	// 		c.handleRefreshError()
	// 		return
	// 	}
	// 	if r.Error != nil {
	// 		c.mu.Lock()
	// 		if c.state != StateConnected {
	// 			c.mu.Unlock()
	// 			return
	// 		}
	// 		if r.Error.Temporary {
	// 			c.handleError(RefreshError{err})
	// 			c.refreshTimer = time.AfterFunc(10*time.Second, c.sendRefresh)
	// 			c.mu.Unlock()
	// 		} else {
	// 			c.mu.Unlock()
	// 			c.moveToDisconnected(r.Error.Code, r.Error.Message)
	// 		}
	// 		return
	// 	}
	// 	expires := r.Refresh.Expires
	// 	ttl := r.Refresh.Ttl
	// 	if expires {
	// 		c.mu.Lock()
	// 		if c.state == StateConnected {
	// 			c.refreshTimer = time.AfterFunc(time.Duration(ttl)*time.Second, c.sendRefresh)
	// 		}
	// 		c.mu.Unlock()
	// 	}
	// })
}

func (c *Client) send(cmd *protocol.Command) error {
	transport := c.transport
	if transport == nil {
		return ErrClientDisconnected
	}
	err := transport.Write(cmd, c.config.WriteTimeout)
	if err != nil {
		log.Error().Stack().Err(err).Msg("send cmd to server")
		go c.handleDisconnect(&disconnect{Code: connectingTransportClosed, Reason: "write error", Reconnect: true})
		return io.EOF
	}
	return nil
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

func (c *Client) nextCmdID() uint32 {
	return atomic.AddUint32(&c.cmdID, 1)
}

func (c *Client) sendAsync(cmd *protocol.Command, cb func(*protocol.Reply, error)) error {
	c.addRequest(cmd.Id, cb)

	err := c.send(cmd)
	if err != nil {
		log.Error().Stack().Err(err).Msg("send cmd")
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
			log.Debug().Msg("timeout sendAsync")
			req.cb(nil, ErrTimeout)
		case <-closeCh:
			log.Debug().Msg("close chan")
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

func (c *Client) sendConnect(fn func(*protocol.ConnectResult, error)) error {
	cmd := &protocol.Command{
		Id: c.nextCmdID(),
	}

	req := &protocol.ConnectRequest{}
	req.Token = c.token
	req.Name = c.config.Name
	req.Version = c.config.Version
	req.Data = c.data

	if len(c.serverSubs) > 0 {
		subs := make(map[string]*protocol.SubscribeRequest)
		for channel, serverSub := range c.serverSubs {
			if !serverSub.Recoverable {
				continue
			}
			subs[channel] = &protocol.SubscribeRequest{
				Recover: true,
				Epoch:   serverSub.Epoch,
				Offset:  serverSub.Offset,
			}
		}
		req.Subs = subs
	}
	cmd.Connect = req

	return c.sendAsync(cmd, func(reply *protocol.Reply, err error) {
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

type StreamPosition struct {
	Offset uint64
	Epoch  string
}

type serverSub struct {
	Offset      uint64
	Epoch       string
	Recoverable bool
}

type request struct {
	cb func(*protocol.Reply, error)
}

type reconnectStrategy interface {
	timeBeforeNextAttempt(attempt int) time.Duration
}

type connectFuture struct {
	fn      func(error)
	closeCh chan struct{}
}

func (c *Client) addRequest(id uint32, cb func(*protocol.Reply, error)) {
	c.requestsMu.Lock()
	defer c.requestsMu.Unlock()
	c.requests[id] = request{cb}
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

func isTemporaryError(err error) bool {
	if e, ok := err.(*Error); ok && e.Temporary {
		return true
	}
	return false
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

func (c *Client) isClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state == StateClosed
}

func (c *Client) isConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state == StateConnected
}

// Disconnect client from server. It's still possible to connect again later. If
// you don't need Client anymore – use Client.Close.
func (c *Client) Disconnect() error {
	if c.isClosed() {
		return ErrClientClosed
	}
	c.moveToDisconnected(disconnectedDisconnectCalled, "disconnect called")
	return nil
}

func (c *Client) handleMessage(msg *protocol.Message) error {
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

type UnsubscribeResult struct{}

func (c *Client) isSubscribed(channel string) bool {
	c.mu.RLock()
	_, ok := c.subs[channel]
	c.mu.RUnlock()
	return ok
}

func (c *Client) sendUnsubscribe(channel string, fn func(UnsubscribeResult, error)) {
	params := &protocol.UnsubscribeRequest{
		Channel: channel,
	}

	cmd := &protocol.Command{
		Id: c.nextCmdID(),
	}
	cmd.Unsubscribe = params

	err := c.sendAsync(cmd, func(r *protocol.Reply, err error) {
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

func (c *Client) sendSubRefresh(channel string, token string, fn func(*protocol.SubRefreshResult, error)) {
	cmd := &protocol.Command{
		Id: c.nextCmdID(),
	}
	params := &protocol.SubRefreshRequest{
		Channel: channel,
		Token:   token,
	}
	cmd.SubRefresh = params

	_ = c.sendAsync(cmd, func(r *protocol.Reply, err error) {
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

func (c *Client) sendSubscribe(channel string, data []byte, recover bool, streamPos StreamPosition, token string, positioned bool, recoverable bool, joinLeave bool, fn func(res *protocol.SubscribeResult, err error)) error {
	params := &protocol.SubscribeRequest{
		Channel: channel,
	}

	if recover {
		params.Recover = true
		if streamPos.Offset > 0 {
			params.Offset = streamPos.Offset
		}
		params.Epoch = streamPos.Epoch
	}
	params.Token = token
	params.Data = data
	params.Positioned = positioned
	params.Recoverable = recoverable
	params.JoinLeave = joinLeave

	cmd := &protocol.Command{
		Id: c.nextCmdID(),
	}
	cmd.Subscribe = params

	return c.sendAsync(cmd, func(reply *protocol.Reply, err error) {
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
