package broker

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type Subscribers map[string]*Subscriber

type Subscriber struct {
	id        string
	messages  chan *Message
	createdAt int64
	destroyed bool
	topics    map[string]bool
	lock      sync.RWMutex
}

func NewSubscriber() (*Subscriber, error) {
	id := make([]byte, 50)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}

	return &Subscriber{
		id:        hex.EncodeToString(id),
		messages:  make(chan *Message),
		createdAt: time.Now().UnixNano(),
		destroyed: false,
		lock:      sync.RWMutex{},
		topics:    map[string]bool{},
	}, nil
}

func (s *Subscriber) GetID() string {
	s.lock.RLock()
	defer s.lock.RUnlock()

	return s.id
}

func (s *Subscriber) GetCreatedAt() int64 {
	s.lock.RLock()
	defer s.lock.RUnlock()

	return s.createdAt
}

func (s *Subscriber) AddTopic(topic string) {
	s.lock.Lock()
	s.topics[topic] = true
	s.lock.Unlock()
}

func (s *Subscriber) RemoveTopic(topic string) {
	s.lock.Lock()
	delete(s.topics, topic)
	s.lock.Unlock()
}

func (s *Subscriber) GetTopics() []string {
	s.lock.RLock()
	subscriberTopics := s.topics
	s.lock.RUnlock()

	topics := []string{}
	for topic := range subscriberTopics {
		topics = append(topics, topic)
	}
	return topics
}

func (s *Subscriber) GetMessages() <-chan *Message {
	return s.messages
}

func (s *Subscriber) Signal(m *Message) *Subscriber {
	s.lock.RLock()
	if !s.destroyed {
		s.messages <- m
	}
	s.lock.RUnlock()

	return s
}

func (s *Subscriber) destroy() {
	s.lock.Lock()
	s.destroyed = true
	s.lock.Unlock()

	close(s.messages)
}

func (s *Subscriber) HandleMessage(fn func(v interface{})) {
	for {
		if msg, ok := <-s.messages; ok {
			fn(msg.payload)
		} else {
			break
		}
	}
}
