package recovery

import (
	"sync"
	"sync/atomic"

	"github.com/THPTUHA/kairos/pkg/protocol/deliverprotocol"
)

type PubSubSync struct {
	subSyncMu sync.RWMutex
	subSync   map[string]*subscribeState
}

type subscribeState struct {
	inSubscribe     uint32
	pubBufferMu     sync.Mutex
	pubBufferLocked bool
	pubBuffer       []*deliverprotocol.Publication
}

func NewPubSubSync() *PubSubSync {
	return &PubSubSync{
		subSync: make(map[string]*subscribeState),
	}
}

func (c *PubSubSync) StopBuffering(channel string) {
	c.subSyncMu.Lock()
	defer c.subSyncMu.Unlock()
	s, ok := c.subSync[channel]
	if !ok {
		return
	}
	atomic.StoreUint32(&s.inSubscribe, 0)
	if s.pubBufferLocked {
		s.pubBufferMu.Unlock()
	}
	delete(c.subSync, channel)
}

func (c *PubSubSync) StartBuffering(channel string) {
	c.subSyncMu.Lock()
	defer c.subSyncMu.Unlock()
	s := &subscribeState{}
	c.subSync[channel] = s
	atomic.StoreUint32(&s.inSubscribe, 1)
}

func (c *PubSubSync) LockBufferAndReadBuffered(channel string) []*deliverprotocol.Publication {
	c.subSyncMu.Lock()
	s, ok := c.subSync[channel]
	if !ok {
		c.subSyncMu.Unlock()
		return nil
	}
	s.pubBufferLocked = true
	c.subSyncMu.Unlock()
	s.pubBufferMu.Lock()
	pubs := make([]*deliverprotocol.Publication, len(s.pubBuffer))
	copy(pubs, s.pubBuffer)
	s.pubBuffer = nil
	return pubs
}

func (c *PubSubSync) SyncPublication(channel string, pub *deliverprotocol.Publication, syncedFn func()) {
	c.subSyncMu.Lock()
	s, ok := c.subSync[channel]
	if !ok {
		c.subSyncMu.Unlock()
		syncedFn()
		return
	}
	c.subSyncMu.Unlock()

	if atomic.LoadUint32(&s.inSubscribe) == 1 {
		s.pubBufferMu.Lock()
		if atomic.LoadUint32(&s.inSubscribe) == 1 {
			s.pubBuffer = append(s.pubBuffer, pub)
			s.pubBufferMu.Unlock()
			return
		}
		s.pubBufferMu.Unlock()
	}
	syncedFn()
}
