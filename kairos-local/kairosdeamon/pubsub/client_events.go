package pubsub

type ConnectionTokenEvent struct {
}
type SubscriptionTokenEvent struct {
	Channel string
}

type ServerPublicationEvent struct {
	Channel string
	Publication
}

type ServerSubscribedEvent struct {
	Channel string
	Data    []byte
}

type ServerUnsubscribedEvent struct {
	Channel string
}

type ServerSubscribingEvent struct {
	Channel string
}

type ConnectedEvent struct {
	ClientID string
	Version  string
	Data     []byte
}

type ConnectingEvent struct {
	Code   uint32
	Reason string
}

type DisconnectedEvent struct {
	Code   uint32
	Reason string
}

type ErrorEvent struct {
	Error error
}

type MessageEvent struct {
	Data []byte
}

type ConnectingHandler func(ConnectingEvent)

type ConnectedHandler func(ConnectedEvent)
type DisconnectHandler func(DisconnectedEvent)
type MessageHandler func(MessageEvent)
type ServerPublicationHandler func(ServerPublicationEvent)
type ServerSubscribedHandler func(ServerSubscribedEvent)
type ServerSubscribingHandler func(ServerSubscribingEvent)
type ServerUnsubscribedHandler func(ServerUnsubscribedEvent)
type ErrorHandler func(ErrorEvent)
type eventHub struct {
	onConnected          ConnectedHandler
	onDisconnected       DisconnectHandler
	onConnecting         ConnectingHandler
	onError              ErrorHandler
	onMessage            MessageHandler
	onServerSubscribe    ServerSubscribedHandler
	onServerSubscribing  ServerSubscribingHandler
	onServerUnsubscribed ServerUnsubscribedHandler
	onServerPublication  ServerPublicationHandler
}

func newEventHub() *eventHub {
	return &eventHub{}
}

func (c *Client) OnConnected(handler ConnectedHandler) {
	c.events.onConnected = handler
}

func (c *Client) OnConnecting(handler ConnectingHandler) {
	c.events.onConnecting = handler
}

func (c *Client) OnDisconnected(handler DisconnectHandler) {
	c.events.onDisconnected = handler
}
func (c *Client) OnError(handler ErrorHandler) {
	c.events.onError = handler
}

func (c *Client) OnMessage(handler MessageHandler) {
	c.events.onMessage = handler
}

func (c *Client) OnPublication(handler ServerPublicationHandler) {
	c.events.onServerPublication = handler
}

func (c *Client) OnSubscribed(handler ServerSubscribedHandler) {
	c.events.onServerSubscribe = handler
}

func (c *Client) OnSubscribing(handler ServerSubscribingHandler) {
	c.events.onServerSubscribing = handler
}

func (c *Client) OnUnsubscribed(handler ServerUnsubscribedHandler) {
	c.events.onServerUnsubscribed = handler
}
