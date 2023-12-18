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

type PublicationEvent struct {
	Publication
}

type SubscribedEvent struct {
	Data []byte
}
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

func (s *Subscription) OnSubscribed(handler SubscribedHandler) {
	s.events.onSubscribed = handler
}

func (s *Subscription) OnSubscribing(handler SubscribingHandler) {
	s.events.onSubscribing = handler
}

func (s *Subscription) OnUnsubscribed(handler UnsubscribedHandler) {
	s.events.onUnsubscribe = handler
}

func (s *Subscription) OnError(handler SubscriptionErrorHandler) {
	s.events.onError = handler
}
