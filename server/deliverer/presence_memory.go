package deliverer

import (
	"context"
	"sync"
)

type MemoryPresenceManager struct {
	node        *Node
	config      MemoryPresenceManagerConfig
	presenceHub *presenceHub
}

var _ PresenceManager = (*MemoryPresenceManager)(nil)

type MemoryPresenceManagerConfig struct{}

func NewMemoryPresenceManager(n *Node, c MemoryPresenceManagerConfig) (*MemoryPresenceManager, error) {
	return &MemoryPresenceManager{
		node:        n,
		config:      c,
		presenceHub: newPresenceHub(),
	}, nil
}

func (m *MemoryPresenceManager) AddPresence(ch string, uid string, info *ClientInfo) error {
	return m.presenceHub.add(ch, uid, info)
}

func (m *MemoryPresenceManager) RemovePresence(ch string, uid string) error {
	return m.presenceHub.remove(ch, uid)
}

func (m *MemoryPresenceManager) Presence(ch string) (map[string]*ClientInfo, error) {
	return m.presenceHub.get(ch)
}

func (m *MemoryPresenceManager) PresenceStats(ch string) (PresenceStats, error) {
	return m.presenceHub.getStats(ch)
}

func (m *MemoryPresenceManager) Close(_ context.Context) error {
	return nil
}

type presenceHub struct {
	sync.RWMutex
	presence map[string]map[string]*ClientInfo
}

func newPresenceHub() *presenceHub {
	return &presenceHub{
		presence: make(map[string]map[string]*ClientInfo),
	}
}

func (h *presenceHub) add(ch string, uid string, info *ClientInfo) error {
	h.Lock()
	defer h.Unlock()

	_, ok := h.presence[ch]
	if !ok {
		h.presence[ch] = make(map[string]*ClientInfo)
	}
	h.presence[ch][uid] = info
	return nil
}

func (h *presenceHub) remove(ch string, uid string) error {
	h.Lock()
	defer h.Unlock()

	if _, ok := h.presence[ch]; !ok {
		return nil
	}
	if _, ok := h.presence[ch][uid]; !ok {
		return nil
	}

	delete(h.presence[ch], uid)

	// clean up map if needed
	if len(h.presence[ch]) == 0 {
		delete(h.presence, ch)
	}

	return nil
}

func (h *presenceHub) get(ch string) (map[string]*ClientInfo, error) {
	h.RLock()
	defer h.RUnlock()

	presence, ok := h.presence[ch]
	if !ok {
		// return empty map
		return nil, nil
	}

	data := make(map[string]*ClientInfo, len(presence))
	for k, v := range presence {
		data[k] = v
	}
	return data, nil
}

func (h *presenceHub) getStats(ch string) (PresenceStats, error) {
	h.RLock()
	defer h.RUnlock()

	presence, ok := h.presence[ch]
	if !ok {
		// return empty map
		return PresenceStats{}, nil
	}

	numClients := len(presence)
	numUsers := 0
	uniqueUsers := map[string]struct{}{}

	for _, info := range presence {
		userID := info.UserID
		if _, ok := uniqueUsers[userID]; !ok {
			uniqueUsers[userID] = struct{}{}
			numUsers++
		}
	}

	return PresenceStats{
		NumClients: numClients,
		NumUsers:   numUsers,
	}, nil
}
