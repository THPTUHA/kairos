package pubsub

type LeaveEvent struct {
	ClientInfo
}

type JoinEvent struct {
	ClientInfo
}

type UnsubscribedEvent struct {
	Code   uint32
	Reason string
}

type SubscribingEvent struct {
	Code   uint32
	Reason string
}

type SubscriptionErrorEvent struct {
	Error error
}

// PublicationEvent has info about received channel Publication.
type PublicationEvent struct {
	Publication
}

// SubscribedEvent is an event context passed
// to subscribe success callback.
type SubscribedEvent struct {
	Data []byte
}

// subscriptionEventHub contains callback functions that will be called when
// corresponding event happens with subscription to channel.
type subscriptionEventHub struct {
	onSubscribing SubscribingHandler
	onSubscribed  SubscribedHandler
	onUnsubscribe UnsubscribedHandler
	onError       SubscriptionErrorHandler
	onPublication PublicationHandler
	onJoin        JoinHandler
	onLeave       LeaveHandler
}

type JoinHandler func(JoinEvent)
type LeaveHandler func(LeaveEvent)
type UnsubscribedHandler func(UnsubscribedEvent)
type SubscribingHandler func(SubscribingEvent)
type SubscribedHandler func(SubscribedEvent)
type SubscriptionErrorHandler func(SubscriptionErrorEvent)
type PublicationHandler func(PublicationEvent)

func newSubscriptionEventHub() *subscriptionEventHub {
	return &subscriptionEventHub{}
}

func (s *Subscription) OnPublication(handler PublicationHandler) {
	s.events.onPublication = handler
}

// OnSubscribed allows setting SubscribedHandler to SubEventHandler.
func (s *Subscription) OnSubscribed(handler SubscribedHandler) {
	s.events.onSubscribed = handler
}

// OnSubscribing allows setting SubscribingHandler to SubEventHandler.
func (s *Subscription) OnSubscribing(handler SubscribingHandler) {
	s.events.onSubscribing = handler
}

// OnUnsubscribed allows setting UnsubscribedHandler to SubEventHandler.
func (s *Subscription) OnUnsubscribed(handler UnsubscribedHandler) {
	s.events.onUnsubscribe = handler
}

// OnError allows setting SubscriptionErrorHandler to SubEventHandler.
func (s *Subscription) OnError(handler SubscriptionErrorHandler) {
	s.events.onError = handler
}

// OnJoin allows setting JoinHandler to SubEventHandler.
func (s *Subscription) OnJoin(handler JoinHandler) {
	s.events.onJoin = handler
}

// OnLeave allows setting LeaveHandler to SubEventHandler.
func (s *Subscription) OnLeave(handler LeaveHandler) {
	s.events.onLeave = handler
}
