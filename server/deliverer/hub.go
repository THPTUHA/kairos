package deliverer

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/THPTUHA/kairos/pkg/protocol/deliverprotocol"
	"github.com/THPTUHA/kairos/server/storage/models"
)

const numHubShards = 64

type Hub struct {
	connShards [numHubShards]*connShard
	subShards  [numHubShards]*subShard
	sessionsMu sync.RWMutex
	sessions   map[string]*Client
}

func newHub(logger *logger) *Hub {
	h := &Hub{
		sessions: map[string]*Client{},
	}
	for i := 0; i < numHubShards; i++ {
		h.connShards[i] = newConnShard()
		h.subShards[i] = newSubShard(logger)
	}
	return h
}

func (h *Hub) clientBySession(session string) (*Client, bool) {
	h.sessionsMu.RLock()
	defer h.sessionsMu.RUnlock()
	c, ok := h.sessions[session]
	return c, ok
}

func (h *Hub) shutdown(ctx context.Context) error {
	sem := make(chan struct{}, hubShutdownSemaphoreSize)

	var errMu sync.Mutex
	var shutdownErr error

	var wg sync.WaitGroup
	wg.Add(numHubShards)
	for i := 0; i < numHubShards; i++ {
		go func(i int) {
			defer wg.Done()
			err := h.connShards[i].shutdown(ctx, sem)
			if err != nil {
				errMu.Lock()
				if shutdownErr == nil {
					shutdownErr = err
				}
				errMu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	return shutdownErr
}

// Add connection into clientHub connections registry.
func (h *Hub) add(c *Client) error {
	h.sessionsMu.Lock()
	h.sessionsMu.Unlock()
	return h.connShards[index(c.UserID(), numHubShards)].add(c)
}

// Remove connection from clientHub connections registry.
func (h *Hub) remove(c *Client) error {
	h.sessionsMu.Lock()
	h.sessionsMu.Unlock()
	return h.connShards[index(c.UserID(), numHubShards)].remove(c)
}

// Connections returns all user connections to the current Node.
func (h *Hub) Connections() map[string]*Client {
	conns := make(map[string]*Client)
	for _, shard := range h.connShards {
		shard.mu.RLock()
		for clientID, c := range shard.conns {
			conns[clientID] = c
		}
		shard.mu.RUnlock()
	}
	return conns
}

func (h *Hub) UserConnections(userID string) map[string]*Client {
	return h.connShards[index(userID, numHubShards)].userConnections(userID)
}

func (h *Hub) InfoUser(userID string) *UserInfo {
	return h.connShards[index(userID, numHubShards)].infoUser(userID)
}

func (h *Hub) refresh(userID string, clientID string, opts ...RefreshOption) error {
	return h.connShards[index(userID, numHubShards)].refresh(userID, clientID, opts...)
}

func (h *Hub) subscribe(userID string, ch string, clientID string, opts ...SubscribeOption) error {
	return h.connShards[index(userID, numHubShards)].subscribe(userID, ch, clientID, opts...)
}

func (h *Hub) unsubscribe(userID string, ch string, unsubscribe Unsubscribe, clientID string) error {
	return h.connShards[index(userID, numHubShards)].unsubscribe(userID, ch, unsubscribe, clientID)
}

func (h *Hub) disconnect(userID string, disconnect Disconnect, clientID string, whitelist []string) error {
	return h.connShards[index(userID, numHubShards)].disconnect(userID, disconnect, clientID, whitelist)
}

func (h *Hub) addSub(ch string, c *Client) (bool, error) {
	return h.subShards[index(ch, numHubShards)].addSub(ch, c)
}

func (h *Hub) removeSub(ch string, c *Client) (bool, error) {
	return h.subShards[index(ch, numHubShards)].removeSub(ch, c)
}

func (h *Hub) BroadcastPublication(ch string, pub *Publication) error {
	return h.subShards[index(ch, numHubShards)].broadcastPublication(ch, pubToProto(pub))
}

func (h *Hub) NumSubscribers(ch string) int {
	return h.subShards[index(ch, numHubShards)].NumSubscribers(ch)
}

func (h *Hub) Channels() []string {
	channels := make([]string, 0, h.NumChannels())
	for i := 0; i < numHubShards; i++ {
		channels = append(channels, h.subShards[i].Channels()...)
	}
	return channels
}

func (h *Hub) NumClients() int {
	var total int
	for i := 0; i < numHubShards; i++ {
		total += h.connShards[i].NumClients()
	}
	return total
}

func (h *Hub) NumUsers() int {
	var total int
	for i := 0; i < numHubShards; i++ {
		// users do not overlap among shards.
		total += h.connShards[i].NumUsers()
	}
	return total
}

func (h *Hub) NumSubscriptions() int {
	var total int
	for i := 0; i < numHubShards; i++ {
		// users do not overlap among shards.
		total += h.subShards[i].NumSubscriptions()
	}
	return total
}

func (h *Hub) NumChannels() int {
	var total int
	for i := 0; i < numHubShards; i++ {
		total += h.subShards[i].NumChannels()
	}
	return total
}

type connShard struct {
	mu    sync.RWMutex
	conns map[string]*Client
	users map[string]map[string]struct{}
}

func newConnShard() *connShard {
	return &connShard{
		conns: make(map[string]*Client),
		users: make(map[string]map[string]struct{}),
	}
}

const (
	hubShutdownSemaphoreSize = 128
)

func (h *connShard) shutdown(ctx context.Context, sem chan struct{}) error {
	advice := DisconnectShutdown
	h.mu.RLock()
	// At this moment node won't accept new client connections, so we can
	// safely copy existing clients and release lock.
	clients := make([]*Client, 0, len(h.conns))
	for _, client := range h.conns {
		clients = append(clients, client)
	}
	h.mu.RUnlock()

	closeFinishedCh := make(chan struct{}, len(clients))
	finished := 0

	if len(clients) == 0 {
		return nil
	}

	for _, client := range clients {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		go func(cc *Client) {
			defer func() { <-sem }()
			defer func() { closeFinishedCh <- struct{}{} }()
			_ = cc.close(advice)
		}(client)
	}

	for {
		select {
		case <-closeFinishedCh:
			finished++
			if finished == len(clients) {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func stringInSlice(str string, slice []string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

func (h *connShard) subscribe(user string, ch string, clientID string, opts ...SubscribeOption) error {
	userConnections := h.userConnections(user)

	var firstErr error
	var errMu sync.Mutex

	var wg sync.WaitGroup
	for _, c := range userConnections {
		if clientID != "" && c.ID() != clientID {
			continue
		}
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			err := c.Subscribe(ch, opts...)
			errMu.Lock()
			defer errMu.Unlock()
			if err != nil && err != io.EOF && firstErr == nil {
				firstErr = err
			}
		}(c)
	}
	wg.Wait()
	return firstErr
}

func (h *connShard) refresh(user string, clientID string, opts ...RefreshOption) error {
	userConnections := h.userConnections(user)

	var firstErr error
	var errMu sync.Mutex

	var wg sync.WaitGroup
	for _, c := range userConnections {
		if clientID != "" && c.ID() != clientID {
			continue
		}
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			err := c.Refresh(opts...)
			errMu.Lock()
			defer errMu.Unlock()
			if err != nil && err != io.EOF && firstErr == nil {
				firstErr = err
			}
		}(c)
	}
	wg.Wait()
	return firstErr
}

func (h *connShard) unsubscribe(user string, ch string, unsubscribe Unsubscribe, clientID string) error {
	userConnections := h.userConnections(user)

	var wg sync.WaitGroup
	for _, c := range userConnections {
		if clientID != "" && c.ID() != clientID {
			continue
		}
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			c.Unsubscribe(ch, unsubscribe)
		}(c)
	}
	wg.Wait()
	return nil
}

func (h *connShard) disconnect(user string, disconnect Disconnect, clientID string, whitelist []string) error {
	userConnections := h.userConnections(user)

	var firstErr error
	var errMu sync.Mutex

	var wg sync.WaitGroup
	for _, c := range userConnections {
		if stringInSlice(c.ID(), whitelist) {
			continue
		}
		if clientID != "" && c.ID() != clientID {
			continue
		}
		wg.Add(1)
		go func(cc *Client) {
			defer wg.Done()
			err := cc.close(disconnect)
			errMu.Lock()
			defer errMu.Unlock()
			if err != nil && err != io.EOF && firstErr == nil {
				firstErr = err
			}
		}(c)
	}
	wg.Wait()
	return firstErr
}

type UserInfo struct {
	ID     string `json:"id"`
	Online bool   `json:"online"`
}

func (h *connShard) infoUser(userID string) *UserInfo {
	online := false
	if len(h.userConnections(userID)) > 0 {
		online = true
	}
	return &UserInfo{
		ID:     userID,
		Online: online,
	}
}

func (h *connShard) userConnections(userID string) map[string]*Client {
	h.mu.RLock()
	defer h.mu.RUnlock()

	userConnections, ok := h.users[userID]
	if !ok {
		return map[string]*Client{}
	}

	conns := make(map[string]*Client, len(userConnections))
	for uid := range userConnections {
		c, ok := h.conns[uid]
		if !ok {
			continue
		}
		conns[uid] = c
	}

	return conns
}

func (h *connShard) add(c *Client) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	uid := c.ID()
	user := c.UserID()

	h.conns[uid] = c

	if _, ok := h.users[user]; !ok {
		h.users[user] = make(map[string]struct{})
	}
	h.users[user][uid] = struct{}{}
	return nil
}

func (h *connShard) remove(c *Client) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	uid := c.ID()
	user := c.UserID()

	delete(h.conns, uid)

	if _, ok := h.users[user]; !ok {
		return nil
	}
	if _, ok := h.users[user][uid]; !ok {
		return nil
	}

	delete(h.users[user], uid)

	if len(h.users[user]) == 0 {
		delete(h.users, user)
	}

	return nil
}

func (h *connShard) NumClients() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	total := 0
	for _, clientConnections := range h.users {
		total += len(clientConnections)
	}
	return total
}

func (h *connShard) NumUsers() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.users)
}

type subShard struct {
	mu     sync.RWMutex
	subs   map[string]map[string]*Client
	logger *logger
}

func newSubShard(logger *logger) *subShard {
	return &subShard{
		subs:   make(map[string]map[string]*Client),
		logger: logger,
	}
}

func (h *subShard) addSub(ch string, c *Client) (bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	uid := c.ID()

	_, ok := h.subs[ch]
	if !ok {
		h.subs[ch] = make(map[string]*Client)
	}
	h.subs[ch][uid] = c
	if !ok {
		return true, nil
	}
	return false, nil
}

func (h *subShard) removeSub(ch string, c *Client) (bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	uid := c.ID()

	if _, ok := h.subs[ch]; !ok {
		return true, nil
	}
	if _, ok := h.subs[ch][uid]; !ok {
		return true, nil
	}

	delete(h.subs[ch], uid)

	if len(h.subs[ch]) == 0 {
		delete(h.subs, ch)
		return true, nil
	}

	return false, nil
}

type encodeError struct {
	client string
	user   string
	error  error
}

func (h *subShard) broadcastPublication(channel string, pub *deliverprotocol.Publication) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	channelSubscribers, ok := h.subs[channel]
	if !ok {
		return nil
	}

	var (
		jsonReply     []byte
		jsonEncodeErr *encodeError
	)

	for _, c := range channelSubscribers {
		channelContext, ok := c.channels[channel]
		fmt.Println(" Client run here ", channelContext.role)
		if !ok || channelContext.role != models.ReadRole && channelContext.role != models.ReadWriteRole && !strings.HasPrefix(channel, "kairosdeamon") {
			continue
		}
		if jsonEncodeErr != nil {
			go func(c *Client) { c.Disconnect(DisconnectInappropriateProtocol) }(c)
			continue
		}
		if jsonReply == nil {
			push := &deliverprotocol.Push{Channel: channel, Pub: pub}
			var err error
			jsonReply, err = deliverprotocol.DefaultJsonReplyEncoder.Encode(&deliverprotocol.Reply{Push: push})
			if err != nil {
				jsonEncodeErr = &encodeError{client: c.ID(), user: c.UserID(), error: err}
				go func(c *Client) { c.Disconnect(DisconnectInappropriateProtocol) }(c)
				continue
			}
		}
		fmt.Println("...........Pulish to client......", pub)
		_ = c.writePublication(channel, pub, jsonReply)
	}

	if jsonEncodeErr != nil && h.logger.enabled(LogLevelWarn) {
		h.logger.log(NewLogEntry(LogLevelWarn, "inappropriate deliverprotocol publication", map[string]any{
			"channel": channel,
			"user":    jsonEncodeErr.user,
			"client":  jsonEncodeErr.client,
			"error":   jsonEncodeErr.error,
		}))
	}

	return nil
}

func (h *subShard) NumChannels() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}

func (h *subShard) NumSubscriptions() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	total := 0
	for _, subscriptions := range h.subs {
		total += len(subscriptions)
	}
	return total
}

func (h *subShard) Channels() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	channels := make([]string, len(h.subs))
	i := 0
	for ch := range h.subs {
		channels[i] = ch
		i++
	}
	return channels
}

func (h *subShard) NumSubscribers(ch string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conns, ok := h.subs[ch]
	if !ok {
		return 0
	}
	return len(conns)
}
