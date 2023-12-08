package deliverer

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/THPTUHA/kairos/pkg/protocol/deliverprotocol"
	"github.com/THPTUHA/kairos/server/deliverer/internal/queue"
	"github.com/THPTUHA/kairos/server/deliverer/internal/recovery"
	"github.com/THPTUHA/kairos/server/deliverer/internal/saferand"
	"github.com/THPTUHA/kairos/server/storage/models"

	"github.com/google/uuid"
)

var jsonPingReply = []byte(`{}`)

var randSource *saferand.Rand

func init() {
	randSource = saferand.New(time.Now().UnixNano())
}

type clientEventHub struct {
	aliveHandler         AliveHandler
	disconnectHandler    DisconnectHandler
	subscribeHandler     SubscribeHandler
	unsubscribeHandler   UnsubscribeHandler
	publishHandler       PublishHandler
	refreshHandler       RefreshHandler
	messageHandler       MessageHandler
	presenceHandler      PresenceHandler
	presenceStatsHandler PresenceStatsHandler
}

func (c *Client) OnAlive(h AliveHandler) {
	c.eventHub.aliveHandler = h
}

func (c *Client) OnRefresh(h RefreshHandler) {
	c.eventHub.refreshHandler = h
}

func (c *Client) OnDisconnect(h DisconnectHandler) {
	c.eventHub.disconnectHandler = h
}

func (c *Client) OnMessage(h MessageHandler) {
	c.eventHub.messageHandler = h
}

func (c *Client) OnSubscribe(h SubscribeHandler) {
	c.eventHub.subscribeHandler = h
}

func (c *Client) OnUnsubscribe(h UnsubscribeHandler) {
	c.eventHub.unsubscribeHandler = h
}

func (c *Client) OnPublish(h PublishHandler) {
	c.eventHub.publishHandler = h
}

func (c *Client) OnPresence(h PresenceHandler) {
	c.eventHub.presenceHandler = h
}

func (c *Client) OnPresenceStats(h PresenceStatsHandler) {
	c.eventHub.presenceStatsHandler = h
}

const (
	flagSubscribed uint8 = 1 << iota
	flagEmitPresence
	flagEmitJoinLeave
	flagPushJoinLeave
	flagServerSide
	flagClientSideRefresh
)

// ChannelContext contains extra context for channel connection subscribed to.
// Note: this struct is aligned to consume less memory.
type ChannelContext struct {
	info              []byte
	expireAt          int64
	positionCheckTime int64
	flags             uint8
	role              int32
	Source            uint8
}

func channelHasFlag(flags, flag uint8) bool {
	return flags&flag != 0
}

type timerOp uint8

const (
	timerOpStale    timerOp = 1
	timerOpPresence timerOp = 2
	timerOpExpire   timerOp = 3
	timerOpPing     timerOp = 4
)

type status uint8

const (
	statusConnecting status = 1
	statusConnected  status = 2
	statusClosed     status = 3
)

// ConnectRequest can be used in a unidirectional connection case to
// pass initial connection information from a client-side.
type ConnectRequest struct {
	// Token is an optional token from a client.
	Token string
	// Data is an optional custom data from a client.
	Data []byte
	// Name of a client.
	Name string
	// Version of a client.
	Version string
	// Subs is a map with channel subscription state (for recovery on connect).
	Subs map[string]SubscribeRequest
}

// SubscribeRequest contains state of subscription to a channel.
type SubscribeRequest struct {
	// Recover enables publication recovery for a channel.
	Recover bool
	// Epoch last seen by a client.
	Epoch string
	// Offset last seen by a client.
	Offset uint64
}

func (r *ConnectRequest) toProto() *deliverprotocol.ConnectRequest {
	if r == nil {
		return nil
	}
	req := &deliverprotocol.ConnectRequest{
		Token: r.Token,
		Data:  r.Data,
		Name:  r.Name,
	}
	if len(r.Subs) > 0 {
		subs := make(map[string]*deliverprotocol.SubscribeRequest, len(r.Subs))
		for k, _ := range r.Subs {
			subs[k] = &deliverprotocol.SubscribeRequest{}
		}
		req.Subs = subs
	}
	return req
}

// Client represents client connection to server.
type Client struct {
	mu                sync.RWMutex
	connectMu         sync.Mutex // allows syncing connect with disconnect.
	presenceMu        sync.Mutex // allows syncing presence routine with client closing.
	ctx               context.Context
	transport         Transport
	node              *Node
	exp               int64
	channels          map[string]ChannelContext
	messageWriter     *writer
	pubSubSync        *recovery.PubSubSync
	uid               string
	user              string
	info              []byte
	storage           map[string]any
	storageMu         sync.Mutex
	authenticated     bool
	clientSideRefresh bool
	status            status
	timerOp           timerOp
	nextPresence      int64
	nextExpire        int64
	nextPing          int64
	lastSeen          int64
	lastPing          int64
	pingInterval      time.Duration
	pongTimeout       time.Duration
	eventHub          *clientEventHub
	timer             *time.Timer
	startWriterOnce   sync.Once
	replyWithoutQueue bool
	unusable          bool
}

type ClientCloseFunc func() error

func NewClient(ctx context.Context, n *Node, t Transport) (*Client, ClientCloseFunc, error) {
	uidObject, err := uuid.NewRandom()
	if err != nil {
		return nil, nil, err
	}
	uid := uidObject.String()

	client := &Client{
		ctx:        ctx,
		uid:        uid,
		node:       n,
		transport:  t,
		channels:   make(map[string]ChannelContext),
		pubSubSync: recovery.NewPubSubSync(),
		status:     statusConnecting,
		eventHub:   &clientEventHub{},
	}

	staleCloseDelay := n.config.ClientStaleCloseDelay
	if staleCloseDelay > 0 {
		client.mu.Lock()
		client.timerOp = timerOpStale
		client.timer = time.AfterFunc(staleCloseDelay, client.onTimerOp)
		client.mu.Unlock()
	}
	return client, func() error { return client.close(DisconnectConnectionClosed) }, nil
}

var uniErrorCodeToDisconnect = map[uint32]Disconnect{
	ErrorExpired.Code:          DisconnectExpired,
	ErrorPermissionDenied.Code: DisconnectPermissionDenied,
}

func extractUnidirectionalDisconnect(err error) Disconnect {
	switch t := err.(type) {
	case *Disconnect:
		return *t
	case Disconnect:
		return t
	case *Error:
		if d, ok := uniErrorCodeToDisconnect[t.Code]; ok {
			return d
		}
		return DisconnectServerError
	default:
		return DisconnectServerError
	}
}

func (c *Client) getDisconnectPushReply(d Disconnect) ([]byte, error) {
	disconnect := &deliverprotocol.Disconnect{
		Code:   d.Code,
		Reason: d.Reason,
	}
	return c.encodeReply(&deliverprotocol.Reply{
		Push: &deliverprotocol.Push{
			Disconnect: disconnect,
		},
	})
}

func hasFlag(flags, flag uint64) bool {
	return flags&flag != 0
}

func (c *Client) issueCommandReadEvent(cmd *deliverprotocol.Command, size int) error {
	if c.node.clientEvents.commandReadHandler != nil {
		return c.node.clientEvents.commandReadHandler(c, CommandReadEvent{
			Command:     cmd,
			CommandSize: size,
		})
	}
	return nil
}

func (c *Client) onTimerOp() {
	c.mu.Lock()
	if c.status == statusClosed {
		c.mu.Unlock()
		return
	}
	timerOp := c.timerOp
	c.mu.Unlock()
	switch timerOp {
	case timerOpStale:
		c.closeStale()
	case timerOpPresence:
		c.updatePresence()
	case timerOpExpire:
		c.expire()
	case timerOpPing:
		c.sendPing()
	}
}

func (c *Client) scheduleNextTimer() {
	if c.status == statusClosed {
		return
	}
	c.stopTimer()
	var minEventTime int64
	var nextTimerOp timerOp
	var needTimer bool
	if c.nextExpire > 0 {
		nextTimerOp = timerOpExpire
		minEventTime = c.nextExpire
		needTimer = true
	}
	if c.nextPresence > 0 && (minEventTime == 0 || c.nextPresence < minEventTime) {
		nextTimerOp = timerOpPresence
		minEventTime = c.nextPresence
		needTimer = true
	}
	if c.nextPing > 0 && (minEventTime == 0 || c.nextPing < minEventTime) {
		nextTimerOp = timerOpPing
		minEventTime = c.nextPing
		needTimer = true
	}
	if needTimer {
		c.timerOp = nextTimerOp
		afterDuration := time.Duration(minEventTime-time.Now().UnixNano()) * time.Nanosecond
		c.timer = time.AfterFunc(afterDuration, c.onTimerOp)
	}
}

// Lock must be held outside.
func (c *Client) stopTimer() {
	if c.timer != nil {
		c.timer.Stop()
	}
}

func (c *Client) sendPing() {
	c.mu.Lock()
	c.lastPing = time.Now().Unix()
	c.mu.Unlock()
	_ = c.transportEnqueue(jsonPingReply, "", deliverprotocol.FrameTypeServerPing)
	c.mu.Lock()
	c.addPingUpdate(false)
	c.mu.Unlock()
}

func (c *Client) addPingUpdate(isFirst bool) {
	delay := c.pingInterval
	if isFirst {
		pingNanoseconds := c.pingInterval.Nanoseconds()
		delay = time.Duration(randSource.Int63n(pingNanoseconds)) * time.Nanosecond
	}
	c.nextPing = time.Now().Add(delay).UnixNano()
	c.scheduleNextTimer()
}

func (c *Client) addPresenceUpdate() {
	c.nextPresence = time.Now().Add(c.node.config.ClientPresenceUpdateInterval).UnixNano()
	c.scheduleNextTimer()
}

func (c *Client) addExpireUpdate(after time.Duration) {
	c.nextExpire = time.Now().Add(after).UnixNano()
	c.scheduleNextTimer()
}

func (c *Client) closeStale() {
	c.mu.RLock()
	authenticated := c.authenticated
	unusable := c.unusable
	closed := c.status == statusClosed
	c.mu.RUnlock()
	if (!authenticated || unusable) && !closed {
		_ = c.close(DisconnectStale)
	}
}

func (c *Client) transportEnqueue(data []byte, ch string, frameType deliverprotocol.FrameType) error {
	item := queue.Item{
		Data:      data,
		FrameType: frameType,
	}
	if c.node.config.GetChannelNamespaceLabel != nil {
		item.Channel = ch
	}
	disconnect := c.messageWriter.enqueue(item)
	if disconnect != nil {
		go func() { _ = c.close(*disconnect) }()
		return io.EOF
	}
	return nil
}

// updateChannelPresence updates client presence info for channel so it
// won't expire until client disconnect.
func (c *Client) updateChannelPresence(ch string, chCtx ChannelContext) error {
	if !channelHasFlag(chCtx.flags, flagEmitPresence) {
		return nil
	}
	c.mu.RLock()
	if _, ok := c.channels[ch]; !ok {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()
	return c.node.addPresence(ch, c.uid, &ClientInfo{
		ClientID: c.uid,
		UserID:   c.user,
		ConnInfo: c.info,
		ChanInfo: chCtx.info,
	})
}

func (c *Client) Context() context.Context {
	return c.ctx
}

func (c *Client) checkSubscriptionExpiration(channel string, channelContext ChannelContext, delay time.Duration, resultCB func(bool)) {
	now := c.node.nowTimeGetter().Unix()
	expireAt := channelContext.expireAt
	clientSideRefresh := channelHasFlag(channelContext.flags, flagClientSideRefresh)
	if expireAt > 0 && now > expireAt+int64(delay.Seconds()) {
		// Subscription expired.
		if clientSideRefresh {
			resultCB(false)
			return
		}
		return
	}
	resultCB(true)
}

// updatePresence used for various periodic actions we need to do with client connections.
func (c *Client) updatePresence() {
	c.presenceMu.Lock()
	defer c.presenceMu.Unlock()
	config := c.node.config
	c.mu.Lock()
	unusable := c.unusable
	if c.status == statusClosed {
		c.mu.Unlock()
		return
	}
	channels := make(map[string]ChannelContext, len(c.channels))
	for channel, channelContext := range c.channels {
		if !channelHasFlag(channelContext.flags, flagSubscribed) {
			continue
		}
		channels[channel] = channelContext
	}
	c.mu.Unlock()

	if unusable {
		go c.closeStale()
		return
	}

	if c.eventHub.aliveHandler != nil {
		c.eventHub.aliveHandler()
	}

	for channel, channelContext := range channels {
		err := c.updateChannelPresence(channel, channelContext)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error updating presence for channel", map[string]any{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
		}

		c.checkSubscriptionExpiration(channel, channelContext, config.ClientExpiredSubCloseDelay, func(result bool) {
			if !result {
				serverSide := channelHasFlag(channelContext.flags, flagServerSide)
				if c.isAsyncUnsubscribe(serverSide) {
					go func(ch string) { c.handleAsyncUnsubscribe(ch, unsubscribeExpired) }(channel)
				} else {
					go func() { _ = c.close(DisconnectSubExpired) }()
				}
			}
		})
	}
	c.mu.Lock()
	c.addPresenceUpdate()
	c.mu.Unlock()
}

func (c *Client) ID() string {
	return c.uid
}

func (c *Client) UserID() string {
	return c.user
}

func (c *Client) Info() []byte {
	c.mu.Lock()
	info := make([]byte, len(c.info))
	copy(info, c.info)
	c.mu.Unlock()
	return info
}

func (c *Client) Transport() TransportInfo {
	return c.transport
}

func (c *Client) Channels() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	channels := make([]string, 0, len(c.channels))
	for ch, ctx := range c.channels {
		if !channelHasFlag(ctx.flags, flagSubscribed) {
			continue
		}
		channels = append(channels, ch)
	}
	return channels
}

// ChannelsWithContext returns a map of channels client connection currently subscribed to
// with a ChannelContext.
func (c *Client) ChannelsWithContext() map[string]ChannelContext {
	c.mu.RLock()
	defer c.mu.RUnlock()
	channels := make(map[string]ChannelContext, len(c.channels))
	for ch, ctx := range c.channels {
		if !channelHasFlag(ctx.flags, flagSubscribed) {
			continue
		}
		channels[ch] = ctx
	}
	return channels
}

// IsSubscribed returns true if client subscribed to a channel.
func (c *Client) IsSubscribed(ch string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	ctx, ok := c.channels[ch]
	return ok && channelHasFlag(ctx.flags, flagSubscribed)
}

// Send data to client. This sends an asynchronous message – data will be
// just written to connection. on client side this message can be handled
// with Message handler.
func (c *Client) Send(data []byte) error {
	replyData, err := c.getSendPushReply(data)
	if err != nil {
		return err
	}
	return c.transportEnqueue(replyData, "", deliverprotocol.FrameTypePushMessage)
}

func (c *Client) encodeReply(reply *deliverprotocol.Reply) ([]byte, error) {
	protoType := c.transport.Protocol().toProto()
	encoder := deliverprotocol.GetReplyEncoder(protoType)
	return encoder.Encode(reply)
}

func (c *Client) getSendPushReply(data []byte) ([]byte, error) {
	p := &deliverprotocol.Message{
		Data: data,
	}
	return c.encodeReply(&deliverprotocol.Reply{
		Push: &deliverprotocol.Push{
			Message: p,
		},
	})
}

// Unsubscribe allows unsubscribing client from channel.
func (c *Client) Unsubscribe(ch string, unsubscribe ...Unsubscribe) {
	if len(unsubscribe) > 1 {
		panic("Client.Unsubscribe called with more than 1 unsubscribe argument")
	}
	c.mu.RLock()
	if c.status == statusClosed {
		c.mu.RUnlock()
		return
	}
	c.mu.RUnlock()

	unsub := unsubscribeServer
	if len(unsubscribe) > 0 {
		unsub = unsubscribe[0]
	}

	err := c.unsubscribe(ch, unsub, nil)
	if err != nil {
		go c.Disconnect(DisconnectServerError)
		return
	}
	_ = c.sendUnsubscribe(ch, unsub)
}

func (c *Client) sendUnsubscribe(ch string, unsub Unsubscribe) error {
	replyData, err := c.getUnsubscribePushReply(ch, unsub)
	if err != nil {
		return err
	}
	_ = c.transportEnqueue(replyData, ch, deliverprotocol.FrameTypePushUnsubscribe)
	return nil
}

func (c *Client) getUnsubscribePushReply(ch string, unsub Unsubscribe) ([]byte, error) {
	p := &deliverprotocol.Unsubscribe{
		Code:   unsub.Code,
		Reason: unsub.Reason,
	}
	return c.encodeReply(&deliverprotocol.Reply{
		Push: &deliverprotocol.Push{
			Channel:     ch,
			Unsubscribe: p,
		},
	})
}

func (c *Client) Disconnect(disconnect ...Disconnect) {
	if len(disconnect) > 1 {
		panic("Client.Disconnect called with more than 1 argument")
	}
	go func() {
		if len(disconnect) == 0 {
			_ = c.close(DisconnectForceNoReconnect)
		} else {
			_ = c.close(disconnect[0])
		}
	}()
}

func (c *Client) close(disconnect Disconnect) error {
	c.startWriter(0, 0, 0)
	c.presenceMu.Lock()
	defer c.presenceMu.Unlock()
	c.connectMu.Lock()
	defer c.connectMu.Unlock()
	c.mu.Lock()
	if c.status == statusClosed {
		c.mu.Unlock()
		return nil
	}
	prevStatus := c.status
	c.status = statusClosed

	c.stopTimer()

	channels := make(map[string]ChannelContext, len(c.channels))
	for channel, channelContext := range c.channels {
		channels[channel] = channelContext
	}
	c.mu.Unlock()

	if len(channels) > 0 {
		// Unsubscribe from all channels.
		unsub := unsubscribeDisconnect
		for channel := range channels {
			err := c.unsubscribe(channel, unsub, &disconnect)
			if err != nil {
				c.node.logger.log(newLogEntry(LogLevelError, "error unsubscribing client from channel", map[string]any{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
			}
		}
	}

	c.mu.RLock()
	authenticated := c.authenticated
	c.mu.RUnlock()

	if authenticated {
		err := c.node.removeClient(c)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error removing client", map[string]any{"user": c.user, "client": c.uid, "error": err.Error()}))
		}
	}

	if disconnect.Code != DisconnectConnectionClosed.Code {
		if replyData, err := c.getDisconnectPushReply(disconnect); err == nil {
			_ = c.transportEnqueue(replyData, "", deliverprotocol.FrameTypePushDisconnect)
		}
	}

	// close writer and send messages remaining in writer queue if any.
	_ = c.messageWriter.close(disconnect != DisconnectConnectionClosed && disconnect != DisconnectSlow)

	_ = c.transport.Close(disconnect)

	if disconnect.Code != DisconnectConnectionClosed.Code {
		c.node.logger.log(newLogEntry(LogLevelDebug, "closing client connection", map[string]any{"client": c.uid, "user": c.user, "reason": disconnect.Reason}))
	}
	if c.eventHub.disconnectHandler != nil && prevStatus == statusConnected {
		c.eventHub.disconnectHandler(DisconnectEvent{
			Disconnect: disconnect,
		})
	}
	return nil
}

func (c *Client) clientInfo(ch string) *ClientInfo {
	var channelInfo deliverprotocol.Raw
	channelContext, ok := c.channels[ch]
	if !ok {
		return nil
	}

	if ok && channelHasFlag(channelContext.flags, flagSubscribed) {
		channelInfo = channelContext.info
	}
	return &ClientInfo{
		ClientID: c.uid,
		UserID:   c.user,
		ConnInfo: c.info,
		ChanInfo: channelInfo,
		ChanRole: channelContext.role,
	}
}

func (c *Client) HandleCommand(cmd *deliverprotocol.Command, cmdProtocolSize int) bool {
	c.mu.Lock()
	if c.status == statusClosed {
		c.mu.Unlock()
		return false
	}
	unusable := c.unusable
	c.mu.Unlock()

	if unusable {
		go func() { _ = c.close(DisconnectBadRequest) }()
		return false
	}

	select {
	case <-c.ctx.Done():
		return false
	default:
	}

	disconnect, proceed := c.dispatchCommand(cmd, cmdProtocolSize)

	select {
	case <-c.ctx.Done():
		return false
	default:
	}
	if disconnect != nil {
		if disconnect.Code != DisconnectConnectionClosed.Code {
			c.node.logger.log(newLogEntry(LogLevelInfo, "disconnect after handling command", map[string]any{"command": fmt.Sprintf("%v", cmd), "client": c.ID(), "user": c.UserID(), "reason": disconnect.Reason}))
		}
		go func() { _ = c.close(*disconnect) }()
		return false
	}
	return proceed
}

func (c *Client) handleCommandDispatchError(ch string, cmd *deliverprotocol.Command, frameType deliverprotocol.FrameType, err error, started time.Time) (*Disconnect, bool) {
	switch t := err.(type) {
	case *Disconnect:
		return t, false
	case Disconnect:
		return &t, false
	default:
		if cmd.Connect != nil {
			c.mu.Lock()
			c.unusable = true
			c.mu.Unlock()
		}
		errorReply := &deliverprotocol.Reply{Error: toClientErr(err).toProto()}
		c.writeError(ch, frameType, cmd, errorReply, nil)
		return nil, cmd.Connect == nil
	}
}

func (c *Client) dispatchCommand(cmd *deliverprotocol.Command, cmdSize int) (*Disconnect, bool) {
	c.mu.Lock()
	if c.status == statusClosed {
		c.mu.Unlock()
		return nil, false
	}
	c.mu.Unlock()
	isConnect := cmd.Connect != nil
	if !c.authenticated && !isConnect {
		return &DisconnectBadRequest, false
	}

	var metricChannel string
	var frameType deliverprotocol.FrameType

	if cmd.Id == 0 && cmd.Send == nil {
		// Now as pong processed make sure that command has id > 0 (except Send).
		return &DisconnectBadRequest, false
	}

	started := time.Now()

	if cmd.Connect != nil {
		frameType = deliverprotocol.FrameTypeConnect
	} else if cmd.Subscribe != nil {
		metricChannel = cmd.Subscribe.Channel
		frameType = deliverprotocol.FrameTypeSubscribe
	} else if cmd.Unsubscribe != nil {
		metricChannel = cmd.Unsubscribe.Channel
		frameType = deliverprotocol.FrameTypeUnsubscribe
	} else if cmd.Publish != nil {
		metricChannel = cmd.Publish.Channel
		frameType = deliverprotocol.FrameTypePublish
	} else if cmd.Presence != nil {
		metricChannel = cmd.Presence.Channel
		frameType = deliverprotocol.FrameTypePresence
	} else if cmd.PresenceStats != nil {
		metricChannel = cmd.PresenceStats.Channel
		frameType = deliverprotocol.FrameTypePresenceStats
	} else if cmd.Send != nil {
		frameType = deliverprotocol.FrameTypeSend
	} else if cmd.Refresh != nil {
		frameType = deliverprotocol.FrameTypeRefresh
	} else if cmd.SubRefresh != nil {
		metricChannel = cmd.SubRefresh.Channel
		frameType = deliverprotocol.FrameTypeSubRefresh
	} else {
		return &DisconnectBadRequest, false
	}

	var handleErr error

	handleErr = c.issueCommandReadEvent(cmd, cmdSize)
	if handleErr != nil {
		return c.handleCommandDispatchError(metricChannel, cmd, frameType, handleErr, started)
	}

	if cmd.Connect != nil {
		handleErr = c.handleConnect(cmd.Connect, cmd, started, nil)
	} else if cmd.Subscribe != nil {
		handleErr = c.handleSubscribe(cmd.Subscribe, cmd, started, nil)
	} else if cmd.Unsubscribe != nil {
		handleErr = c.handleUnsubscribe(cmd.Unsubscribe, cmd, started, nil)
	} else if cmd.Publish != nil {
		handleErr = c.handlePublish(cmd.Publish, cmd, started, nil)
	} else if cmd.Presence != nil {
		handleErr = c.handlePresence(cmd.Presence, cmd, started, nil)
	} else if cmd.PresenceStats != nil {
		handleErr = c.handlePresenceStats(cmd.PresenceStats, cmd, started, nil)
	} else if cmd.Send != nil {
		handleErr = c.handleSend(cmd.Send, cmd, started)
	} else if cmd.Refresh != nil {
		handleErr = c.handleRefresh(cmd.Refresh, cmd, started, nil)
	} else {
		return &DisconnectBadRequest, false
	}
	if handleErr != nil {
		return c.handleCommandDispatchError(metricChannel, cmd, frameType, handleErr, started)
	}
	return nil, true
}

func (c *Client) writeEncodedPush(rep *deliverprotocol.Reply, rw *replyWriter, ch string, frameType deliverprotocol.FrameType) {
	encoder := deliverprotocol.GetPushEncoder(c.transport.Protocol().toProto())
	var err error
	data, err := encoder.Encode(rep.Push)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error encoding connect push", map[string]any{"push": fmt.Sprintf("%v", rep.Push), "client": c.ID(), "user": c.UserID(), "error": err.Error()}))
		go func() { _ = c.close(DisconnectInappropriateProtocol) }()
		return
	}
	_ = c.transportEnqueue(data, ch, frameType)
	if rw != nil {
		rw.write(rep)
	}
}

func (c *Client) writeEncodedCommandReply(ch string, frameType deliverprotocol.FrameType, cmd *deliverprotocol.Command, rep *deliverprotocol.Reply, rw *replyWriter) {
	rep.Id = cmd.Id
	if rep.Error != nil {
		if c.node.LogEnabled(LogLevelInfo) {
			c.node.logger.log(newLogEntry(LogLevelInfo, "client command error", map[string]any{"reply": fmt.Sprintf("%v", rep), "command": fmt.Sprintf("%v", cmd), "client": c.ID(), "user": c.UserID(), "error": rep.Error.Message, "code": rep.Error.Code}))
		}
	}

	protoType := c.transport.Protocol().toProto()
	replyEncoder := deliverprotocol.GetReplyEncoder(protoType)

	replyData, err := replyEncoder.Encode(rep)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error encoding reply", map[string]any{"reply": fmt.Sprintf("%v", rep), "client": c.ID(), "user": c.UserID(), "error": err.Error()}))
		go func() { _ = c.close(DisconnectInappropriateProtocol) }()
		return
	}

	item := queue.Item{Data: replyData, FrameType: frameType}
	if ch != "" && c.node.config.GetChannelNamespaceLabel != nil && c.node.config.ChannelNamespaceLabelForTransportMessagesSent {
		item.Channel = ch
	}

	if c.replyWithoutQueue {
		err = c.messageWriter.config.WriteFn(item)
		if err != nil {
			go func() { _ = c.close(DisconnectWriteError) }()
		}
	} else {
		disconnect := c.messageWriter.enqueue(item)
		if disconnect != nil {
			go func() { _ = c.close(*disconnect) }()
		}
	}
	if rw != nil {
		rw.write(rep)
	}
}

func (c *Client) checkExpired() {
	c.mu.RLock()
	closed := c.status == statusClosed
	clientSideRefresh := c.clientSideRefresh
	exp := c.exp
	c.mu.RUnlock()
	if closed || exp == 0 {
		return
	}
	now := time.Now().Unix()
	ttl := exp - now

	if !clientSideRefresh && c.eventHub.refreshHandler != nil {
		if ttl > 0 {
			c.mu.Lock()
			if c.status != statusClosed {
				c.addExpireUpdate(time.Duration(ttl) * time.Second)
			}
			c.mu.Unlock()
		}
	}

	if ttl > 0 {
		// Connection was successfully refreshed.
		return
	}

	_ = c.close(DisconnectExpired)
}

func (c *Client) expire() {
	c.mu.RLock()
	closed := c.status == statusClosed
	clientSideRefresh := c.clientSideRefresh
	exp := c.exp
	c.mu.RUnlock()
	if closed || exp == 0 {
		return
	}
	if !clientSideRefresh && c.eventHub.refreshHandler != nil {
		cb := func(reply RefreshReply, err error) {
			if err != nil {
				switch t := err.(type) {
				case *Disconnect:
					_ = c.close(*t)
					return
				case Disconnect:
					_ = c.close(t)
					return
				default:
					_ = c.close(DisconnectServerError)
					return
				}
			}
			if reply.Expired {
				_ = c.close(DisconnectExpired)
				return
			}
			if reply.ExpireAt > 0 {
				c.mu.Lock()
				c.exp = reply.ExpireAt
				if reply.Info != nil {
					c.info = reply.Info
				}
				c.mu.Unlock()
			}
			c.checkExpired()
		}
		c.eventHub.refreshHandler(RefreshEvent{}, cb)
	} else {
		c.checkExpired()
	}
}

func (c *Client) handleConnect(req *deliverprotocol.ConnectRequest, cmd *deliverprotocol.Command, started time.Time, rw *replyWriter) error {
	_, err := c.connectCmd(req, cmd, started, rw)
	if err != nil {
		return err
	}
	c.triggerConnect()
	c.scheduleOnConnectTimers()
	return nil
}

func (c *Client) triggerConnect() {
	c.connectMu.Lock()
	defer c.connectMu.Unlock()
	if c.status != statusConnecting {
		return
	}
	if c.node.clientEvents.connectHandler == nil {
		c.status = statusConnected
		return
	}
	c.node.clientEvents.connectHandler(c)
	c.status = statusConnected
}

func (c *Client) scheduleOnConnectTimers() {
	c.mu.Lock()
	c.addPresenceUpdate()
	if c.exp > 0 {
		expireAfter := time.Duration(c.exp-time.Now().Unix()) * time.Second
		if c.clientSideRefresh {
			conf := c.node.config
			expireAfter += conf.ClientExpiredCloseDelay
		}
		c.addExpireUpdate(expireAfter)
	}
	if c.pingInterval > 0 {
		c.addPingUpdate(true)
	}
	c.mu.Unlock()
}

func (c *Client) Refresh(opts ...RefreshOption) error {
	refreshOptions := &RefreshOptions{}
	for _, opt := range opts {
		opt(refreshOptions)
	}
	if refreshOptions.Expired {
		go func() { _ = c.close(DisconnectExpired) }()
		return nil
	}

	expireAt := refreshOptions.ExpireAt
	info := refreshOptions.Info

	res := &deliverprotocol.Refresh{
		Expires: expireAt > 0,
	}

	ttl := expireAt - time.Now().Unix()

	if ttl > 0 {
		res.Ttl = uint32(ttl)
	}

	if expireAt > 0 {
		// connection check enabled
		if ttl > 0 {
			// connection refreshed, update client timestamp and set new expiration timeout
			c.mu.Lock()
			c.exp = expireAt
			if len(info) > 0 {
				c.info = info
			}
			duration := time.Duration(ttl)*time.Second + c.node.config.ClientExpiredCloseDelay
			c.addExpireUpdate(duration)
			c.mu.Unlock()
		} else {
			go func() { _ = c.close(DisconnectExpired) }()
			return nil
		}
	} else {
		c.mu.Lock()
		c.exp = 0
		c.mu.Unlock()
	}

	replyData, err := c.getRefreshPushReply(res)
	if err != nil {
		return err
	}
	return c.transportEnqueue(replyData, "", deliverprotocol.FrameTypePushRefresh)
}

func (c *Client) getRefreshPushReply(res *deliverprotocol.Refresh) ([]byte, error) {
	return c.encodeReply(&deliverprotocol.Reply{
		Push: &deliverprotocol.Push{
			Refresh: res,
		},
	})
}

func (c *Client) releaseRefreshCommandReply(reply *deliverprotocol.Reply) {
	deliverprotocol.ReplyPool.ReleaseRefreshReply(reply)
}

func (c *Client) getRefreshCommandReply(res *deliverprotocol.RefreshResult) (*deliverprotocol.Reply, error) {
	return deliverprotocol.ReplyPool.AcquireRefreshReply(res), nil
}

func (c *Client) handleRefresh(req *deliverprotocol.RefreshRequest, cmd *deliverprotocol.Command, started time.Time, rw *replyWriter) error {
	if c.eventHub.refreshHandler == nil {
		return ErrorNotAvailable
	}

	if req.Token == "" {
		return c.logDisconnectBadRequest("client token required to refresh")
	}

	c.mu.RLock()
	clientSideRefresh := c.clientSideRefresh
	c.mu.RUnlock()

	if !clientSideRefresh {
		return c.logDisconnectBadRequest("server-side refresh expected")
	}

	event := RefreshEvent{
		ClientSideRefresh: true,
		Token:             req.Token,
	}

	cb := func(reply RefreshReply, err error) {
		if err != nil {
			c.writeDisconnectOrErrorFlush("", deliverprotocol.FrameTypeRefresh, cmd, err, started, rw)
			return
		}

		if reply.Expired {
			c.writeDisconnectOrErrorFlush("", deliverprotocol.FrameTypeRefresh, cmd, DisconnectExpired, started, rw)
			return
		}

		expireAt := reply.ExpireAt
		info := reply.Info

		res := &deliverprotocol.RefreshResult{
			Expires: expireAt > 0,
		}

		ttl := expireAt - time.Now().Unix()

		if ttl > 0 {
			res.Ttl = uint32(ttl)
		}

		if expireAt > 0 {
			// connection check enabled
			if ttl > 0 {
				// connection refreshed, update client timestamp and set new expiration timeout
				c.mu.Lock()
				c.exp = expireAt
				if len(info) > 0 {
					c.info = info
				}
				duration := time.Duration(ttl)*time.Second + c.node.config.ClientExpiredCloseDelay
				c.addExpireUpdate(duration)
				c.mu.Unlock()
			} else {
				c.writeDisconnectOrErrorFlush("", deliverprotocol.FrameTypeRefresh, cmd, ErrorExpired, started, rw)
				return
			}
		}

		protoReply, err := c.getRefreshCommandReply(res)
		if err != nil {
			c.logWriteInternalErrorFlush("", deliverprotocol.FrameTypeRefresh, cmd, err, "error encoding refresh", started, rw)
			return
		}
		c.writeEncodedCommandReply("", deliverprotocol.FrameTypeRefresh, cmd, protoReply, rw)
		c.releaseRefreshCommandReply(protoReply)
	}

	c.eventHub.refreshHandler(event, cb)
	return nil
}

// onSubscribeError cleans up a channel from client channels if an error during subscribe happened.
// Channel kept in a map during subscribe request to check for duplicate subscription attempts.
func (c *Client) onSubscribeError(channel string) {
	c.mu.Lock()
	_, ok := c.channels[channel]
	delete(c.channels, channel)
	c.mu.Unlock()
	if ok {
		_ = c.node.removeSubscription(channel, c)
	}
}

func (c *Client) handleSubscribe(req *deliverprotocol.SubscribeRequest, cmd *deliverprotocol.Command, started time.Time, rw *replyWriter) error {
	if c.eventHub.subscribeHandler == nil {
		return ErrorNotAvailable
	}

	replyError, disconnect := c.validateSubscribeRequest(req)
	if disconnect != nil || replyError != nil {
		if disconnect != nil {
			return *disconnect
		}
		return replyError
	}

	event := SubscribeEvent{
		Channel:   req.Channel,
		Token:     req.Token,
		Data:      req.Data,
		JoinLeave: req.JoinLeave,
	}

	cb := func(reply SubscribeReply, err error) {
		if reply.SubscriptionReady != nil {
			defer close(reply.SubscriptionReady)
		}

		if err != nil {
			c.onSubscribeError(req.Channel)
			c.writeDisconnectOrErrorFlush(req.Channel, deliverprotocol.FrameTypeSubscribe, cmd, err, started, rw)
			return
		}

		ctx := c.subscribeCmd(req, reply, cmd, false, started, rw)

		if ctx.disconnect != nil {
			c.onSubscribeError(req.Channel)
			c.writeDisconnectOrErrorFlush(req.Channel, deliverprotocol.FrameTypeSubscribe, cmd, ctx.disconnect, started, rw)
			return
		}
		if ctx.err != nil {
			c.onSubscribeError(req.Channel)
			c.writeDisconnectOrErrorFlush(req.Channel, deliverprotocol.FrameTypeSubscribe, cmd, ctx.err, started, rw)
			return
		}

		if channelHasFlag(ctx.channelContext.flags, flagEmitJoinLeave) && ctx.clientInfo != nil {
			go func() { _ = c.node.publishJoin(req.Channel, ctx.clientInfo) }()
		}
	}
	c.eventHub.subscribeHandler(event, cb)
	return nil
}

func (c *Client) getSubscribedChannelContext(channel string) (ChannelContext, bool) {
	c.mu.RLock()
	ctx, okChannel := c.channels[channel]
	c.mu.RUnlock()
	if !okChannel || !channelHasFlag(ctx.flags, flagSubscribed) {
		return ChannelContext{}, false
	}
	return ctx, true
}

func (c *Client) releaseSubRefreshCommandReply(reply *deliverprotocol.Reply) {
	deliverprotocol.ReplyPool.ReleaseSubRefreshReply(reply)
}

func (c *Client) getSubRefreshCommandReply(res *deliverprotocol.SubRefreshResult) (*deliverprotocol.Reply, error) {
	return deliverprotocol.ReplyPool.AcquireSubRefreshReply(res), nil
}

func (c *Client) handleUnsubscribe(req *deliverprotocol.UnsubscribeRequest, cmd *deliverprotocol.Command, started time.Time, rw *replyWriter) error {
	channel := req.Channel
	if channel == "" {
		return c.logDisconnectBadRequest("channel required for unsubscribe")
	}

	if err := c.unsubscribe(channel, unsubscribeClient, nil); err != nil {
		return err
	}

	protoReply, err := c.getUnsubscribeCommandReply(&deliverprotocol.UnsubscribeResult{})
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error encoding unsubscribe", map[string]any{"error": err.Error()}))
		return DisconnectServerError
	}
	c.writeEncodedCommandReply(channel, deliverprotocol.FrameTypeUnsubscribe, cmd, protoReply, rw)
	c.releaseUnsubscribeCommandReply(protoReply)
	return nil
}

func (c *Client) releaseUnsubscribeCommandReply(reply *deliverprotocol.Reply) {
	deliverprotocol.ReplyPool.ReleaseUnsubscribeReply(reply)
}

func (c *Client) getUnsubscribeCommandReply(res *deliverprotocol.UnsubscribeResult) (*deliverprotocol.Reply, error) {
	return deliverprotocol.ReplyPool.AcquireUnsubscribeReply(res), nil
}

func (c *Client) handlePublish(req *deliverprotocol.PublishRequest, cmd *deliverprotocol.Command, started time.Time, rw *replyWriter) error {
	if c.eventHub.publishHandler == nil {
		return ErrorNotAvailable
	}

	channel := req.Channel
	data := req.Data

	if channel == "" || len(data) == 0 {
		return c.logDisconnectBadRequest("channel and data required for publish")
	}

	c.mu.RLock()
	info := c.clientInfo(channel)
	c.mu.RUnlock()

	// No permission
	if info == nil || (info.ChanRole != models.WriteRole && info.ChanRole != models.ReadWriteRole) {
		return ErrorPermissionDenied
	}

	event := PublishEvent{
		Channel:    channel,
		Data:       data,
		ClientInfo: info,
	}

	cb := func(_ PublishReply, err error) {
		if err != nil {
			c.writeDisconnectOrErrorFlush(channel, deliverprotocol.FrameTypePublish, cmd, err, started, rw)
			return
		}

		protoReply, err := c.getPublishCommandReply(&deliverprotocol.PublishResult{})
		if err != nil {
			c.logWriteInternalErrorFlush(channel, deliverprotocol.FrameTypePublish, cmd, err, "error encoding publish", started, rw)
			return
		}
		c.writeEncodedCommandReply(channel, deliverprotocol.FrameTypePublish, cmd, protoReply, rw)
		c.releasePublishCommandReply(protoReply)
	}

	c.eventHub.publishHandler(event, cb)
	return nil
}

func (c *Client) releasePublishCommandReply(reply *deliverprotocol.Reply) {
	deliverprotocol.ReplyPool.ReleasePublishReply(reply)
}

func (c *Client) getPublishCommandReply(res *deliverprotocol.PublishResult) (*deliverprotocol.Reply, error) {
	return deliverprotocol.ReplyPool.AcquirePublishReply(res), nil
}

func (c *Client) handlePresence(req *deliverprotocol.PresenceRequest, cmd *deliverprotocol.Command, started time.Time, rw *replyWriter) error {
	if c.eventHub.presenceHandler == nil {
		return ErrorNotAvailable
	}

	channel := req.Channel
	if channel == "" {
		return c.logDisconnectBadRequest("channel required for presence")
	}

	event := PresenceEvent{
		Channel: channel,
	}

	cb := func(reply PresenceReply, err error) {
		if err != nil {
			c.writeDisconnectOrErrorFlush(channel, deliverprotocol.FrameTypePresence, cmd, err, started, rw)
			return
		}

		var presence map[string]*ClientInfo
		if reply.Result == nil {
			result, err := c.node.Presence(event.Channel)
			if err != nil {
				c.logWriteInternalErrorFlush(channel, deliverprotocol.FrameTypePresence, cmd, err, "error getting presence", started, rw)
				return
			}
			presence = result.Presence
		} else {
			presence = reply.Result.Presence
		}

		protoPresence := make(map[string]*deliverprotocol.ClientInfo, len(presence))
		for k, v := range presence {
			protoPresence[k] = infoToProto(v)
		}

		protoReply, err := c.getPresenceCommandReply(&deliverprotocol.PresenceResult{
			Presence: protoPresence,
		})
		if err != nil {
			c.logWriteInternalErrorFlush(channel, deliverprotocol.FrameTypePresence, cmd, err, "error encoding presence", started, rw)
			return
		}
		c.writeEncodedCommandReply(channel, deliverprotocol.FrameTypePresence, cmd, protoReply, rw)
		c.releasePresenceCommandReply(protoReply)
	}

	c.eventHub.presenceHandler(event, cb)
	return nil
}

func (c *Client) releasePresenceCommandReply(reply *deliverprotocol.Reply) {
	deliverprotocol.ReplyPool.ReleasePresenceReply(reply)
}

func (c *Client) getPresenceCommandReply(res *deliverprotocol.PresenceResult) (*deliverprotocol.Reply, error) {
	return deliverprotocol.ReplyPool.AcquirePresenceReply(res), nil
}

func (c *Client) handlePresenceStats(req *deliverprotocol.PresenceStatsRequest, cmd *deliverprotocol.Command, started time.Time, rw *replyWriter) error {
	if c.eventHub.presenceStatsHandler == nil {
		return ErrorNotAvailable
	}

	channel := req.Channel
	if channel == "" {
		return c.logDisconnectBadRequest("channel required for presence stats")
	}

	event := PresenceStatsEvent{
		Channel: channel,
	}

	cb := func(reply PresenceStatsReply, err error) {
		if err != nil {
			c.writeDisconnectOrErrorFlush(channel, deliverprotocol.FrameTypePresenceStats, cmd, err, started, rw)
			return
		}

		var presenceStats PresenceStats
		if reply.Result == nil {
			result, err := c.node.PresenceStats(event.Channel)
			if err != nil {
				c.logWriteInternalErrorFlush(channel, deliverprotocol.FrameTypePresenceStats, cmd, err, "error getting presence stats", started, rw)
				return
			}
			presenceStats = result.PresenceStats
		} else {
			presenceStats = reply.Result.PresenceStats
		}

		protoReply, err := c.getPresenceStatsCommandReply(&deliverprotocol.PresenceStatsResult{
			NumClients: uint32(presenceStats.NumClients),
			NumUsers:   uint32(presenceStats.NumUsers),
		})
		if err != nil {
			c.logWriteInternalErrorFlush(channel, deliverprotocol.FrameTypePresence, cmd, err, "error encoding presence stats", started, rw)
			return
		}
		c.writeEncodedCommandReply(channel, deliverprotocol.FrameTypePresenceStats, cmd, protoReply, rw)
		c.releasePresenceStatsCommandReply(protoReply)
	}

	c.eventHub.presenceStatsHandler(event, cb)
	return nil
}

func (c *Client) releasePresenceStatsCommandReply(reply *deliverprotocol.Reply) {
	deliverprotocol.ReplyPool.ReleasePresenceStatsReply(reply)
}

func (c *Client) getPresenceStatsCommandReply(res *deliverprotocol.PresenceStatsResult) (*deliverprotocol.Reply, error) {
	return deliverprotocol.ReplyPool.AcquirePresenceStatsReply(res), nil
}

var emptyReply = &deliverprotocol.Reply{}

func (c *Client) writeError(ch string, frameType deliverprotocol.FrameType, cmd *deliverprotocol.Command, errorReply *deliverprotocol.Reply, rw *replyWriter) {
	c.writeEncodedCommandReply(ch, frameType, cmd, errorReply, rw)
}

func (c *Client) writeDisconnectOrErrorFlush(ch string, frameType deliverprotocol.FrameType, cmd *deliverprotocol.Command, replyError error, started time.Time, rw *replyWriter) {
	switch t := replyError.(type) {
	case *Disconnect:
		go func() { _ = c.close(*t) }()
		return
	case Disconnect:
		go func() { _ = c.close(t) }()
		return
	default:
		errorReply := &deliverprotocol.Reply{Error: toClientErr(replyError).toProto()}
		c.writeError(ch, frameType, cmd, errorReply, rw)
	}
}

type replyWriter struct {
	write func(*deliverprotocol.Reply)
}

func (c *Client) handleSend(req *deliverprotocol.SendRequest, cmd *deliverprotocol.Command, started time.Time) error {
	// Send handler is a bit special since it's a one way command: client does not expect any reply.
	if c.eventHub.messageHandler == nil {
		// Return DisconnectNotAvailable here since otherwise client won't even know
		// server does not have asynchronous message handler set.
		return DisconnectNotAvailable
	}
	c.eventHub.messageHandler(MessageEvent{
		Data: req.Data,
	})
	return nil
}

func (c *Client) unlockServerSideSubscriptions(subCtxMap map[string]subscribeContext) {
	for channel := range subCtxMap {
		c.pubSubSync.StopBuffering(channel)
	}
}

func (c *Client) connectCmd(req *deliverprotocol.ConnectRequest, cmd *deliverprotocol.Command, started time.Time, rw *replyWriter) (*deliverprotocol.ConnectResult, error) {
	c.mu.RLock()
	authenticated := c.authenticated
	closed := c.status == statusClosed
	c.mu.RUnlock()

	if closed {
		return nil, DisconnectConnectionClosed
	}

	if authenticated {
		return nil, c.logDisconnectBadRequest("client already authenticated")
	}

	config := c.node.config
	userConnectionLimit := config.UserConnectionLimit
	channelLimit := config.ClientChannelLimit

	var (
		credentials       *Credentials
		authData          deliverprotocol.Raw
		subscriptions     map[string]SubscribeOptions
		clientSideRefresh bool
	)

	if c.node.clientEvents.connectingHandler != nil {
		e := ConnectEvent{
			ClientID:  c.ID(),
			Data:      req.Data,
			Token:     req.Token,
			Name:      req.Name,
			Transport: c.transport,
		}
		if len(req.Subs) > 0 {
			channels := make([]string, 0, len(req.Subs))
			for ch := range req.Subs {
				channels = append(channels, ch)
			}
			e.Channels = channels
		}
		reply, err := c.node.clientEvents.connectingHandler(c.ctx, e)
		if err != nil {
			c.startWriter(0, 0, 0)
			return nil, err
		}
		if reply.PingPongConfig != nil {
			c.pingInterval, c.pongTimeout = getPingPongPeriodValues(*reply.PingPongConfig)
		} else {
			c.pingInterval, c.pongTimeout = getPingPongPeriodValues(c.transport.PingPongConfig())
		}
		c.replyWithoutQueue = reply.ReplyWithoutQueue
		c.startWriter(reply.WriteDelay, reply.MaxMessagesInFrame, reply.QueueInitialCap)

		if reply.Credentials != nil {
			credentials = reply.Credentials
		}
		c.storage = reply.Storage
		if reply.Context != nil {
			c.mu.Lock()
			c.ctx = reply.Context
			c.mu.Unlock()
		}
		if reply.Data != nil {
			authData = reply.Data
		}
		clientSideRefresh = reply.ClientSideRefresh
		if len(reply.Subscriptions) > 0 {
			subscriptions = make(map[string]SubscribeOptions, len(reply.Subscriptions))
			for ch, opts := range reply.Subscriptions {
				if ch == "" {
					continue
				}
				subscriptions[ch] = opts
			}
		}
	} else {
		c.startWriter(0, 0, 0)
		c.pingInterval, c.pongTimeout = getPingPongPeriodValues(c.transport.PingPongConfig())
	}

	if channelLimit > 0 && len(subscriptions) > channelLimit {
		return nil, DisconnectChannelLimit
	}

	if credentials == nil {
		// Try to find Credentials in context.
		if cred, ok := GetCredentials(c.ctx); ok {
			credentials = cred
		}
	}

	var (
		expires bool
		ttl     uint32
	)

	c.mu.Lock()
	c.clientSideRefresh = clientSideRefresh
	c.mu.Unlock()

	if credentials == nil {
		return nil, c.logDisconnectBadRequest("client credentials not found")
	}

	c.mu.Lock()
	c.user = credentials.UserID
	c.info = credentials.Info
	c.exp = credentials.ExpireAt

	user := c.user
	exp := c.exp
	closed = c.status == statusClosed
	c.mu.Unlock()

	if closed {
		return nil, DisconnectConnectionClosed
	}

	if c.node.LogEnabled(LogLevelDebug) {
		c.node.logger.log(newLogEntry(LogLevelDebug, "client authenticated", map[string]any{"client": c.uid, "user": c.user}))
	}

	if userConnectionLimit > 0 && user != "" && len(c.node.hub.UserConnections(user)) >= userConnectionLimit {
		c.node.logger.log(newLogEntry(LogLevelInfo, "limit of connections for user reached", map[string]any{"user": user, "client": c.uid, "limit": userConnectionLimit}))
		return nil, DisconnectConnectionLimit
	}

	c.mu.RLock()
	if exp > 0 {
		expires = true
		now := time.Now().Unix()
		if exp < now {
			c.mu.RUnlock()
			c.node.logger.log(newLogEntry(LogLevelInfo, "connection expiration must be greater than now", map[string]any{"client": c.uid, "user": c.UserID()}))
			return nil, ErrorExpired
		}
		ttl = uint32(exp - now)
	}
	c.mu.RUnlock()

	res := &deliverprotocol.ConnectResult{
		Expires: expires,
		Ttl:     ttl,
	}

	if c.pingInterval > 0 {
		res.Ping = uint32(c.pingInterval.Seconds())
	}
	// Client successfully connected.
	c.mu.Lock()
	c.authenticated = true
	c.mu.Unlock()

	err := c.node.addClient(c)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error adding client", map[string]any{"client": c.uid, "error": err.Error()}))
		return nil, DisconnectServerError
	}

	if !clientSideRefresh {
		// Server will do refresh itself.
		res.Expires = false
		res.Ttl = 0
	}

	res.Client = c.uid
	if authData != nil {
		res.Data = authData
	}

	var subCtxMap map[string]subscribeContext
	if len(subscriptions) > 0 {
		var subMu sync.Mutex
		subCtxMap = make(map[string]subscribeContext, len(subscriptions))
		subs := make(map[string]*deliverprotocol.SubscribeResult, len(subscriptions))
		var subDisconnect *Disconnect
		var subError *Error
		var wg sync.WaitGroup

		wg.Add(len(subscriptions))
		for ch, opts := range subscriptions {
			go func(ch string, opts SubscribeOptions) {
				defer wg.Done()
				subCmd := &deliverprotocol.SubscribeRequest{
					Channel: ch,
				}
				subCtx := c.subscribeCmd(subCmd, SubscribeReply{Options: opts}, nil, true, started, nil)
				subMu.Lock()
				subs[ch] = subCtx.result
				subCtxMap[ch] = subCtx
				if subCtx.disconnect != nil {
					subDisconnect = subCtx.disconnect
				}
				if subCtx.err != nil {
					subError = subCtx.err
				}
				subMu.Unlock()
			}(ch, opts)
		}
		wg.Wait()

		if subDisconnect != nil || subError != nil {
			c.unlockServerSideSubscriptions(subCtxMap)
			for channel := range subCtxMap {
				c.onSubscribeError(channel)
			}
			if subDisconnect != nil {
				return nil, subDisconnect
			}
			return nil, subError
		}
		res.Subs = subs
	}

	protoReply, err := c.getConnectCommandReply(res)
	if err != nil {
		c.unlockServerSideSubscriptions(subCtxMap)
		c.node.logger.log(newLogEntry(LogLevelError, "error encoding connect", map[string]any{"error": err.Error()}))
		return nil, DisconnectServerError
	}
	c.writeEncodedCommandReply("", deliverprotocol.FrameTypeConnect, cmd, protoReply, rw)
	defer c.releaseConnectCommandReply(protoReply)

	c.mu.Lock()
	for channel, subCtx := range subCtxMap {
		c.channels[channel] = subCtx.channelContext
	}
	c.mu.Unlock()

	c.unlockServerSideSubscriptions(subCtxMap)

	if len(subCtxMap) > 0 {
		for channel, subCtx := range subCtxMap {
			go func(channel string, subCtx subscribeContext) {
				if channelHasFlag(subCtx.channelContext.flags, flagEmitJoinLeave) && subCtx.clientInfo != nil {
					_ = c.node.publishJoin(channel, subCtx.clientInfo)
				}
			}(channel, subCtx)
		}
	}

	return res, nil
}

func (c *Client) startWriter(batchDelay time.Duration, maxMessagesInFrame int, queueInitialCap int) {
	c.startWriterOnce.Do(func() {
		var writeMu sync.Mutex
		messageWriterConf := writerConfig{
			MaxQueueSize: c.node.config.ClientQueueMaxSize,
			WriteFn: func(item queue.Item) error {
				if c.node.clientEvents.transportWriteHandler != nil {
					pass := c.node.clientEvents.transportWriteHandler(c, TransportWriteEvent(item))
					if !pass {
						return nil
					}
				}
				writeMu.Lock()
				defer writeMu.Unlock()
				if err := c.transport.Write(item.Data); err != nil {
					switch v := err.(type) {
					case *Disconnect:
						go func() { _ = c.close(*v) }()
					case Disconnect:
						go func() { _ = c.close(v) }()
					default:
						go func() { _ = c.close(DisconnectWriteError) }()
					}
					return err
				}
				return nil
			},
			WriteManyFn: func(items ...queue.Item) error {
				messages := make([][]byte, 0, len(items))
				for i := 0; i < len(items); i++ {
					if c.node.clientEvents.transportWriteHandler != nil {
						pass := c.node.clientEvents.transportWriteHandler(c, TransportWriteEvent(items[i]))
						if !pass {
							continue
						}
					}
					messages = append(messages, items[i].Data)
				}
				writeMu.Lock()
				defer writeMu.Unlock()
				if err := c.transport.WriteMany(messages...); err != nil {
					switch v := err.(type) {
					case *Disconnect:
						go func() { _ = c.close(*v) }()
					case Disconnect:
						go func() { _ = c.close(v) }()
					default:
						go func() { _ = c.close(DisconnectWriteError) }()
					}
					return err
				}
				return nil
			},
		}

		c.messageWriter = newWriter(messageWriterConf, queueInitialCap)
		go c.messageWriter.run(batchDelay, maxMessagesInFrame)
	})
}

func (c *Client) releaseConnectCommandReply(reply *deliverprotocol.Reply) {
	deliverprotocol.ReplyPool.ReleaseConnectReply(reply)
}

func (c *Client) getConnectCommandReply(res *deliverprotocol.ConnectResult) (*deliverprotocol.Reply, error) {
	return deliverprotocol.ReplyPool.AcquireConnectReply(res), nil
}

func (c *Client) Subscribe(channel string, opts ...SubscribeOption) error {
	if channel == "" {
		return fmt.Errorf("channel is empty")
	}
	channelLimit := c.node.config.ClientChannelLimit
	c.mu.RLock()
	numChannels := len(c.channels)
	c.mu.RUnlock()
	if channelLimit > 0 && numChannels >= channelLimit {
		go func() { _ = c.close(DisconnectChannelLimit) }()
		return nil
	}

	subCmd := &deliverprotocol.SubscribeRequest{
		Channel: channel,
	}
	subscribeOpts := &SubscribeOptions{}
	for _, opt := range opts {
		opt(subscribeOpts)
	}
	subCtx := c.subscribeCmd(subCmd, SubscribeReply{
		Options: *subscribeOpts,
	}, nil, true, time.Time{}, nil)
	if subCtx.err != nil {
		c.onSubscribeError(subCmd.Channel)
		return subCtx.err
	}
	defer c.pubSubSync.StopBuffering(channel)
	c.mu.Lock()
	c.channels[channel] = subCtx.channelContext
	c.mu.Unlock()

	replyData, err := c.getSubscribePushReply(channel, subCtx.result)
	if err != nil {
		return err
	}
	err = c.transportEnqueue(replyData, channel, deliverprotocol.FrameTypePushSubscribe)
	if err != nil {
		return err
	}
	if channelHasFlag(subCtx.channelContext.flags, flagEmitJoinLeave) && subCtx.clientInfo != nil {
		_ = c.node.publishJoin(channel, subCtx.clientInfo)
	}
	return nil
}

func (c *Client) getSubscribePushReply(channel string, res *deliverprotocol.SubscribeResult) ([]byte, error) {
	sub := &deliverprotocol.Subscribe{
		Data: res.Data,
	}
	return c.encodeReply(&deliverprotocol.Reply{
		Push: &deliverprotocol.Push{
			Channel:   channel,
			Subscribe: sub,
		},
	})
}

func (c *Client) validateSubscribeRequest(cmd *deliverprotocol.SubscribeRequest) (*Error, *Disconnect) {
	channel := cmd.Channel
	if channel == "" {
		c.node.logger.log(newLogEntry(LogLevelInfo, "channel required for subscribe", map[string]any{"user": c.user, "client": c.uid}))
		return nil, &DisconnectBadRequest
	}

	config := c.node.config
	channelMaxLength := config.ChannelMaxLength
	channelLimit := config.ClientChannelLimit

	if channelMaxLength > 0 && len(channel) > channelMaxLength {
		c.node.logger.log(newLogEntry(LogLevelInfo, "channel too long", map[string]any{"max": channelMaxLength, "channel": channel, "user": c.user, "client": c.uid}))
		return ErrorBadRequest, nil
	}

	c.mu.Lock()
	numChannels := len(c.channels)
	_, ok := c.channels[channel]
	if ok {
		c.mu.Unlock()
		c.node.logger.log(newLogEntry(LogLevelInfo, "client already subscribed on channel", map[string]any{"channel": channel, "user": c.user, "client": c.uid}))
		return ErrorAlreadySubscribed, nil
	}
	if channelLimit > 0 && numChannels >= channelLimit {
		c.mu.Unlock()
		c.node.logger.log(newLogEntry(LogLevelInfo, "maximum limit of channels per client reached", map[string]any{"limit": channelLimit, "user": c.user, "client": c.uid}))
		return ErrorLimitExceeded, nil
	}
	// Put channel to a map to track duplicate subscriptions. This channel should
	// be removed from a map upon an error during subscribe.
	c.channels[channel] = ChannelContext{}
	c.mu.Unlock()

	return nil, nil
}

func errorDisconnectContext(replyError *Error, disconnect *Disconnect) subscribeContext {
	ctx := subscribeContext{}
	if disconnect != nil {
		ctx.disconnect = disconnect
		return ctx
	}
	ctx.err = replyError
	return ctx
}

type subscribeContext struct {
	result         *deliverprotocol.SubscribeResult
	clientInfo     *ClientInfo
	err            *Error
	disconnect     *Disconnect
	channelContext ChannelContext
}

func (c *Client) subscribeCmd(req *deliverprotocol.SubscribeRequest, reply SubscribeReply, cmd *deliverprotocol.Command, serverSide bool, started time.Time, rw *replyWriter) subscribeContext {
	ctx := subscribeContext{}
	res := &deliverprotocol.SubscribeResult{}
	if reply.Options.ExpireAt > 0 {
		ttl := reply.Options.ExpireAt - time.Now().Unix()
		if ttl <= 0 {
			c.node.logger.log(newLogEntry(LogLevelInfo, "subscription expiration must be greater than now", map[string]any{"client": c.uid, "user": c.UserID()}))
			return errorDisconnectContext(ErrorExpired, nil)
		}
		if reply.ClientSideRefresh {
			res.Expires = true
			res.Ttl = uint32(ttl)
		}
	}

	if reply.Options.Data != nil {
		res.Data = reply.Options.Data
	}

	info := &ClientInfo{
		ClientID: c.uid,
		UserID:   c.user,
		ConnInfo: c.info,
		ChanInfo: reply.Options.ChannelInfo,
		ChanRole: reply.Options.Role,
	}

	channel := req.Channel

	fmt.Printf(" MY CHANNEL------ %s  %+v\n", channel, c.channels)
	if strings.Contains(channel, ":") {
		valid := false
		item := strings.Split(channel, ":")
		for k := range c.channels {
			pre := strings.Split(k, "-")
			if pre[0] == item[0] {
				valid = true
				channel = fmt.Sprintf("%s-%s:%s", item[0], pre[1], item[1])
				info.ChanRole = c.channels[k].role
				break
			}
		}
		if !valid {
			return subscribeContext{
				err:        ErrorPermissionDenied,
				disconnect: &DisconnectBadRequest,
			}
		}
	}
	fmt.Println("CCCC----", channel)
	err := c.node.addSubscription(channel, c)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error adding subscription", map[string]any{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
		c.pubSubSync.StopBuffering(channel)
		if clientErr, ok := err.(*Error); ok && clientErr != ErrorInternal {
			return errorDisconnectContext(clientErr, nil)
		}
		ctx.disconnect = &DisconnectServerError
		return ctx
	}

	if reply.Options.EmitPresence {
		err = c.node.addPresence(channel, c.uid, info)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error adding presence", map[string]any{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
			c.pubSubSync.StopBuffering(channel)
			ctx.disconnect = &DisconnectServerError
			return ctx
		}
	}

	if !serverSide {
		// Write subscription reply only if initiated by client.
		protoReply, err := c.getSubscribeCommandReply(res)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding subscribe", map[string]any{"error": err.Error()}))
			if !serverSide {
				// Will be called later in case of server side sub.
				c.pubSubSync.StopBuffering(channel)
			}
			ctx.disconnect = &DisconnectServerError
			return ctx
		}

		// Need to flush data from writer so subscription response is
		// sent before any subscription publication.
		c.writeEncodedCommandReply(channel, deliverprotocol.FrameTypeSubscribe, cmd, protoReply, rw)
		defer c.releaseSubscribeCommandReply(protoReply)
	}

	var channelFlags uint8
	channelFlags |= flagSubscribed
	if serverSide {
		channelFlags |= flagServerSide
	}
	if reply.ClientSideRefresh {
		channelFlags |= flagClientSideRefresh
	}
	if reply.Options.EmitPresence {
		channelFlags |= flagEmitPresence
	}
	if reply.Options.EmitJoinLeave {
		channelFlags |= flagEmitJoinLeave
	}
	if reply.Options.PushJoinLeave {
		channelFlags |= flagPushJoinLeave
	}

	channelContext := ChannelContext{
		info:     reply.Options.ChannelInfo,
		flags:    channelFlags,
		expireAt: reply.Options.ExpireAt,
		Source:   reply.Options.Source,
		role:     info.ChanRole,
	}

	if !serverSide {
		// In case of server-side sub this will be done later by the caller.
		c.mu.Lock()
		c.channels[channel] = channelContext
		c.mu.Unlock()
		// Stop syncing recovery and PUB/SUB.
		// In case of server side subscription we will do this later.
		c.pubSubSync.StopBuffering(channel)
	}

	if c.node.logger.enabled(LogLevelDebug) {
		c.node.logger.log(newLogEntry(LogLevelDebug, "client subscribed to channel", map[string]any{"client": c.uid, "user": c.user, "channel": req.Channel}))
	}

	ctx.result = res
	ctx.clientInfo = info
	ctx.channelContext = channelContext
	return ctx
}

func (c *Client) releaseSubscribeCommandReply(reply *deliverprotocol.Reply) {
	deliverprotocol.ReplyPool.ReleaseSubscribeReply(reply)
}

func (c *Client) getSubscribeCommandReply(res *deliverprotocol.SubscribeResult) (*deliverprotocol.Reply, error) {
	return deliverprotocol.ReplyPool.AcquireSubscribeReply(res), nil
}

func (c *Client) handleInsufficientState(ch string, serverSide bool) {
	if c.isAsyncUnsubscribe(serverSide) {
		c.handleAsyncUnsubscribe(ch, unsubscribeInsufficientState)
	} else {
		c.handleInsufficientStateDisconnect()
	}
}

func (c *Client) isAsyncUnsubscribe(serverSide bool) bool {
	return !serverSide
}

func (c *Client) handleInsufficientStateDisconnect() {
	_ = c.close(DisconnectInsufficientState)
}

func (c *Client) handleAsyncUnsubscribe(ch string, unsub Unsubscribe) {
	err := c.unsubscribe(ch, unsub, nil)
	if err != nil {
		_ = c.close(DisconnectServerError)
		return
	}
	err = c.sendUnsubscribe(ch, unsub)
	if err != nil {
		_ = c.close(DisconnectWriteError)
		return
	}
}

func (c *Client) writePublicationUpdatePosition(ch string, pub *deliverprotocol.Publication, data []byte) error {
	c.mu.Lock()
	channelContext, ok := c.channels[ch]
	if !ok || !channelHasFlag(channelContext.flags, flagSubscribed) {
		c.mu.Unlock()
		return nil
	}

	channelContext.positionCheckTime = time.Now().Unix()
	c.channels[ch] = channelContext
	c.mu.Unlock()
	return c.transportEnqueue(data, ch, deliverprotocol.FrameTypePushPublication)
}

func (c *Client) writePublication(ch string, pub *deliverprotocol.Publication, data []byte) error {
	c.pubSubSync.SyncPublication(ch, pub, func() {
		_ = c.writePublicationUpdatePosition(ch, pub, data)
	})
	return nil
}

func (c *Client) writeJoin(ch string, join *deliverprotocol.Join, data []byte) error {
	c.mu.RLock()
	channelContext, ok := c.channels[ch]
	if !ok || !channelHasFlag(channelContext.flags, flagSubscribed) {
		c.mu.RUnlock()
		return nil
	}
	if !channelHasFlag(channelContext.flags, flagPushJoinLeave) {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()
	return c.transportEnqueue(data, ch, deliverprotocol.FrameTypePushJoin)
}

func (c *Client) writeLeave(ch string, leave *deliverprotocol.Leave, data []byte) error {
	c.mu.RLock()
	channelContext, ok := c.channels[ch]
	if !ok || !channelHasFlag(channelContext.flags, flagSubscribed) {
		c.mu.RUnlock()
		return nil
	}
	if !channelHasFlag(channelContext.flags, flagPushJoinLeave) {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()
	return c.transportEnqueue(data, ch, deliverprotocol.FrameTypePushLeave)
}

func (c *Client) unsubscribe(channel string, unsubscribe Unsubscribe, disconnect *Disconnect) error {
	c.mu.RLock()
	info := c.clientInfo(channel)
	chCtx, ok := c.channels[channel]
	c.mu.RUnlock()
	if !ok {
		return nil
	}

	serverSide := channelHasFlag(chCtx.flags, flagServerSide)

	c.mu.Lock()
	delete(c.channels, channel)
	c.mu.Unlock()

	if channelHasFlag(chCtx.flags, flagEmitPresence) && channelHasFlag(chCtx.flags, flagSubscribed) {
		err := c.node.removePresence(channel, c.uid)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error removing channel presence", map[string]any{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
		}
	}

	if channelHasFlag(chCtx.flags, flagEmitJoinLeave) && channelHasFlag(chCtx.flags, flagSubscribed) {
		_ = c.node.publishLeave(channel, info)
	}

	if err := c.node.removeSubscription(channel, c); err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error removing subscription", map[string]any{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
		return err
	}

	if channelHasFlag(chCtx.flags, flagSubscribed) {
		if c.eventHub.unsubscribeHandler != nil {
			c.eventHub.unsubscribeHandler(UnsubscribeEvent{
				Channel:     channel,
				ServerSide:  serverSide,
				Unsubscribe: unsubscribe,
				Disconnect:  disconnect,
			})
		}
	}

	if c.node.logger.enabled(LogLevelDebug) {
		c.node.logger.log(newLogEntry(LogLevelDebug, "client unsubscribed from channel", map[string]any{"channel": channel, "user": c.user, "client": c.uid}))
	}

	return nil
}

func (c *Client) logDisconnectBadRequest(message string) error {
	c.node.logger.log(newLogEntry(LogLevelInfo, message, map[string]any{"user": c.user, "client": c.uid}))
	return DisconnectBadRequest
}

func (c *Client) logWriteInternalErrorFlush(ch string, frameType deliverprotocol.FrameType, cmd *deliverprotocol.Command, err error, message string, started time.Time, rw *replyWriter) {

	if clientErr, ok := err.(*Error); ok {
		errorReply := &deliverprotocol.Reply{Error: clientErr.toProto()}
		c.writeError(ch, frameType, cmd, errorReply, rw)
		return
	}
	c.node.logger.log(newLogEntry(LogLevelError, message, map[string]any{"error": err.Error()}))

	errorReply := &deliverprotocol.Reply{Error: ErrorInternal.toProto()}
	c.writeError(ch, frameType, cmd, errorReply, rw)
}

func toClientErr(err error) *Error {
	if clientErr, ok := err.(*Error); ok {
		return clientErr
	}
	return ErrorInternal
}
