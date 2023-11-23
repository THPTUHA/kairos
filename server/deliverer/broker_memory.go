package deliverer

import (
	"sync"
)

type MemoryBroker struct {
	node         *Node
	eventHandler BrokerEventHandler

	pubLocks map[int]*sync.Mutex

	closeOnce sync.Once
	closeCh   chan struct{}
}

const numPubLocks = 4096

type MemoryBrokerConfig struct{}

func NewMemoryBroker(n *Node, _ MemoryBrokerConfig) (*MemoryBroker, error) {
	pubLocks := make(map[int]*sync.Mutex, numPubLocks)
	for i := 0; i < numPubLocks; i++ {
		pubLocks[i] = &sync.Mutex{}
	}
	closeCh := make(chan struct{})
	b := &MemoryBroker{
		node:     n,
		pubLocks: pubLocks,
		closeCh:  closeCh,
	}
	return b, nil
}

func (b *MemoryBroker) pubLock(ch string) *sync.Mutex {
	return b.pubLocks[index(ch, numPubLocks)]
}
func (b *MemoryBroker) Publish(ch string, data []byte, opts PublishOptions) (StreamPosition, error) {
	mu := b.pubLock(ch)
	mu.Lock()
	defer mu.Unlock()

	pub := &Publication{
		Data: data,
		Info: opts.ClientInfo,
		Tags: opts.Tags,
	}
	return StreamPosition{}, b.eventHandler.HandlePublication(ch, pub, StreamPosition{})
}

func (b *MemoryBroker) PublishControl(data []byte, _, _ string) error {
	return b.eventHandler.HandleControl(data)
}

func (b *MemoryBroker) PublishJoin(ch string, info *ClientInfo) error {
	return b.eventHandler.HandleJoin(ch, info)
}

func (b *MemoryBroker) PublishLeave(ch string, info *ClientInfo) error {
	return b.eventHandler.HandleLeave(ch, info)
}

func (b *MemoryBroker) Subscribe(_ string) error {
	return nil
}

func (b *MemoryBroker) Unsubscribe(_ string) error {
	return nil
}

func (b *MemoryBroker) Run(h BrokerEventHandler) error {
	b.eventHandler = h
	return nil
}
