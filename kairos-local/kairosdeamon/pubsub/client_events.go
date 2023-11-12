package pubsub

// ConnectionTokenEvent may contain some useful contextual information in the future.
// For now, it's empty.
type ConnectionTokenEvent struct {
}

// SubscriptionTokenEvent contains info required to get subscription token when
// client wants to subscribe on private channel.
type SubscriptionTokenEvent struct {
	Channel string
}

// eventHub has all event handlers for client.
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
	onServerJoin         ServerJoinHandler
	onServerLeave        ServerLeaveHandler
}

// ConnectingEvent is a connecting event context passed to OnConnecting callback.
type ConnectingEvent struct {
	Code   uint32
	Reason string
}

// ConnectingHandler is an interface describing how to handle connecting event.
type ConnectingHandler func(ConnectingEvent)

// ConnectedEvent is a connected event context passed to OnConnected callback.
type ConnectedEvent struct {
	ClientID string
	Version  string
	Data     []byte
}

// ConnectedHandler is an interface describing how to handle connect event.
type ConnectedHandler func(ConnectedEvent)

// DisconnectedEvent is a disconnected event context passed to OnDisconnected callback.
type DisconnectedEvent struct {
	Code   uint32
	Reason string
}

// DisconnectHandler is an interface describing how to handle moveToDisconnected event.
type DisconnectHandler func(DisconnectedEvent)

// MessageEvent is an event for async message from server to client.
type MessageEvent struct {
	Data []byte
}

// MessageHandler is an interface describing how to handle async message from server.
type MessageHandler func(MessageEvent)

// ServerPublicationEvent has info about received channel Publication.
type ServerPublicationEvent struct {
	Channel string
	Publication
}

// ServerPublicationHandler is an interface describing how to handle Publication from
// server-side subscriptions.
type ServerPublicationHandler func(ServerPublicationEvent)

type ServerSubscribedEvent struct {
	Channel        string
	WasRecovering  bool
	Recovered      bool
	Recoverable    bool
	Positioned     bool
	StreamPosition *StreamPosition
	Data           []byte
}

// ServerSubscribedHandler is an interface describing how to handle subscribe events from
// server-side subscriptions.
type ServerSubscribedHandler func(ServerSubscribedEvent)

// ServerSubscribingEvent is an event passed to subscribing event handler.
type ServerSubscribingEvent struct {
	Channel string
}

// ServerSubscribingHandler is an interface describing how to handle subscribing events for
// server-side subscriptions.
type ServerSubscribingHandler func(ServerSubscribingEvent)

// ServerUnsubscribedEvent is an event passed to unsubscribe event handler.
type ServerUnsubscribedEvent struct {
	Channel string
}

// ServerUnsubscribedHandler is an interface describing how to handle unsubscribe events from
// server-side subscriptions.
type ServerUnsubscribedHandler func(ServerUnsubscribedEvent)

// ServerJoinEvent has info about user who left channel.
type ServerJoinEvent struct {
	Channel string
	ClientInfo
}

// ServerJoinHandler is an interface describing how to handle Join events from
// server-side subscriptions.
type ServerJoinHandler func(ServerJoinEvent)

// ServerLeaveEvent has info about user who joined channel.
type ServerLeaveEvent struct {
	Channel string
	ClientInfo
}

// ServerLeaveHandler is an interface describing how to handle Leave events from
// server-side subscriptions.
type ServerLeaveHandler func(ServerLeaveEvent)

// ErrorEvent is an error event context passed to OnError callback.
type ErrorEvent struct {
	Error error
}

// ErrorHandler is an interface describing how to handle error event.
type ErrorHandler func(ErrorEvent)

// newEventHub initializes new eventHub.
func newEventHub() *eventHub {
	return &eventHub{}
}

// OnConnected is a function to handle connect event.
func (c *Client) OnConnected(handler ConnectedHandler) {
	c.events.onConnected = handler
}

// OnMessage allows processing async message from server to client.
func (c *Client) OnMessage(handler MessageHandler) {
	c.events.onMessage = handler
}
