package broker

import (
	"sync"
	"time"
)

type Broker struct {
	subscribers Subscribers
	sLock       sync.RWMutex

	topics map[string]Subscribers
	tLock  sync.RWMutex
}

func NewBroker() *Broker {
	return &Broker{
		subscribers: Subscribers{},
		sLock:       sync.RWMutex{},
		topics:      map[string]Subscribers{},
		tLock:       sync.RWMutex{},
	}
}

func (b *Broker) Attach() (*Subscriber, error) {
	s, err := NewSubscriber()

	if err != nil {
		return nil, err
	}

	b.sLock.Lock()
	b.subscribers[s.GetID()] = s
	b.sLock.Unlock()

	return s, nil
}

func (b *Broker) Subscribe(s *Subscriber, topics ...string) {
	b.tLock.Lock()
	defer b.tLock.Unlock()

	for _, topic := range topics {
		if nil == b.topics[topic] {
			b.topics[topic] = Subscribers{}
		}
		s.AddTopic(topic)
		b.topics[topic][s.id] = s
	}

}

func (b *Broker) Unsubscribe(s *Subscriber, topics ...string) {
	for _, topic := range topics {
		b.tLock.Lock()
		if nil == b.topics[topic] {
			b.tLock.Unlock()
			continue
		}
		delete(b.topics[topic], s.id)
		b.tLock.Unlock()
		s.RemoveTopic(topic)
	}
}

func (b *Broker) Detach(s *Subscriber) {
	s.destroy()
	b.sLock.Lock()
	b.Unsubscribe(s, s.GetTopics()...)
	delete(b.subscribers, s.id)
	defer b.sLock.Unlock()
}

func (b *Broker) Broadcast(payload interface{}, topics ...string) {
	for _, topic := range topics {
		if b.Subscribers(topic) < 1 {
			continue
		}
		b.tLock.RLock()
		for _, s := range b.topics[topic] {
			m := &Message{
				topic:     topic,
				payload:   payload,
				createdAt: time.Now().UnixNano(),
			}
			go (func(s *Subscriber) {
				s.Signal(m)
			})(s)
		}
		b.tLock.RUnlock()
	}
}

func (b *Broker) Subscribers(topic string) int {
	b.tLock.RLock()
	defer b.tLock.RUnlock()
	return len(b.topics[topic])
}

func (b *Broker) GetTopics() []string {
	b.tLock.RLock()
	brokerTopics := b.topics
	b.tLock.RUnlock()

	topics := []string{}
	for topic := range brokerTopics {
		topics = append(topics, topic)
	}

	return topics
}
