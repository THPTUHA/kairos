package recovery

import (
	"sync"
	"sync/atomic"

	"github.com/THPTUHA/kairos/pkg/protocol/deliverprotocol"
)

// PubSubSync wraps logic to synchronize recovery with PUB/SUB.
type PubSubSync struct {
	subSyncMu sync.RWMutex
	subSync   map[string]*subscribeState
}

type subscribeState struct {
	// The following fields help us to synchronize PUB/SUB and history messages
	// during publication recovery process in channel.
	inSubscribe     uint32
	pubBufferMu     sync.Mutex
	pubBufferLocked bool
	pubBuffer       []*deliverprotocol.Publication
}

// NewPubSubSync creates new PubSubSyncer.
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

// StartBuffering ...
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
	s.pubBufferMu.Lock() // Since this point and until StopBuffering pubBufferMu will be locked so that SyncPublication waits till pubBufferMu unlocking.
	pubs := make([]*deliverprotocol.Publication, len(s.pubBuffer))
	copy(pubs, s.pubBuffer)
	s.pubBuffer = nil
	return pubs
}

// SyncPublication ...
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
		// client currently in process of subscribing to the channel. In this case we keep
		// publications in a slice buffer. Publications from this temporary buffer will be sent in
		// subscribe reply.
		s.pubBufferMu.Lock()
		if atomic.LoadUint32(&s.inSubscribe) == 1 {
			// Sync point not reached yet - put Publication to tmp slice.
			s.pubBuffer = append(s.pubBuffer, pub)
			s.pubBufferMu.Unlock()
			return
		}
		// Sync point already passed - send Publication into connection.
		s.pubBufferMu.Unlock()
	}
	syncedFn()
}
