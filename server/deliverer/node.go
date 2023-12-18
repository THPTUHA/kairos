package deliverer

import (
	"context"
	"errors"
	"hash/fnv"
	"os"
	"sync"
	"time"

	"github.com/THPTUHA/kairos/pkg/protocol/deliverprotocol"
	"github.com/THPTUHA/kairos/server/deliverer/internal/controlpb"
	"github.com/THPTUHA/kairos/server/deliverer/internal/controlproto"
	"github.com/THPTUHA/kairos/server/deliverer/internal/dissolve"
	"github.com/THPTUHA/kairos/server/deliverer/internal/nowtime"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
)

type Node struct {
	mu              sync.RWMutex
	uid             string
	startedAt       int64
	config          Config
	hub             *Hub
	broker          Broker
	presenceManager PresenceManager
	nodes           *nodeRegistry
	shutdown        bool
	shutdownCh      chan struct{}
	clientEvents    *eventHub
	logger          *logger
	controlEncoder  controlproto.Encoder
	controlDecoder  controlproto.Decoder
	subLocks        map[int]*sync.Mutex
	subDissolver    *dissolve.Dissolver

	nowTimeGetter nowtime.Getter
}

const (
	numSubLocks            = 16384
	numSubDissolverWorkers = 64
)

func NewNode(c Config) (*Node, error) {
	if c.ClientPresenceUpdateInterval == 0 {
		c.ClientPresenceUpdateInterval = 27 * time.Second
	}
	if c.ClientChannelPositionCheckDelay == 0 {
		c.ClientChannelPositionCheckDelay = 40 * time.Second
	}
	if c.ClientExpiredCloseDelay == 0 {
		c.ClientExpiredCloseDelay = 25 * time.Second
	}
	if c.ClientExpiredSubCloseDelay == 0 {
		c.ClientExpiredSubCloseDelay = 25 * time.Second
	}
	if c.ClientStaleCloseDelay == 0 {
		c.ClientStaleCloseDelay = 15 * time.Second
	}
	if c.ClientQueueMaxSize == 0 {
		c.ClientQueueMaxSize = 1048576 // 1MB by default.
	}
	if c.ClientChannelLimit == 0 {
		c.ClientChannelLimit = 128
	}
	if c.ChannelMaxLength == 0 {
		c.ChannelMaxLength = 255
	}

	uidObj, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}
	uid := uidObj.String()

	subLocks := make(map[int]*sync.Mutex, numSubLocks)
	for i := 0; i < numSubLocks; i++ {
		subLocks[i] = &sync.Mutex{}
	}

	if c.Name == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, err
		}
		c.Name = hostname
	}

	var lg *logger
	if c.LogHandler != nil {
		lg = newLogger(c.LogLevel, c.LogHandler)
	}

	n := &Node{
		uid:            uid,
		nodes:          newNodeRegistry(uid),
		config:         c,
		hub:            newHub(lg),
		startedAt:      time.Now().Unix(),
		shutdownCh:     make(chan struct{}),
		logger:         lg,
		controlEncoder: controlproto.NewProtobufEncoder(),
		controlDecoder: controlproto.NewProtobufDecoder(),
		clientEvents:   &eventHub{},
		subLocks:       subLocks,
		subDissolver:   dissolve.New(numSubDissolverWorkers),
		nowTimeGetter:  nowtime.Get,
	}
	b, err := NewMemoryBroker(n, MemoryBrokerConfig{})
	if err != nil {
		return nil, err
	}
	n.SetBroker(b)

	m, err := NewMemoryPresenceManager(n, MemoryPresenceManagerConfig{})
	if err != nil {
		return nil, err
	}
	n.SetPresenceManager(m)

	return n, nil
}

func index(s string, numBuckets int) int {
	if numBuckets == 1 {
		return 0
	}
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(s))
	return int(hash.Sum64() % uint64(numBuckets))
}

func (n *Node) Config() Config {
	return n.config
}

func (n *Node) ID() string {
	return n.uid
}

func (n *Node) subLock(ch string) *sync.Mutex {
	return n.subLocks[index(ch, numSubLocks)]
}

func (n *Node) SetBroker(b Broker) {
	n.broker = b
}

func (n *Node) SetPresenceManager(m PresenceManager) {
	n.presenceManager = m
}

func (n *Node) Hub() *Hub {
	return n.hub
}

func (n *Node) Run() error {
	if err := n.broker.Run(&brokerEventHandler{n}); err != nil {
		return err
	}

	err := n.pubNode("")
	if err != nil {
		n.logger.log(newLogEntry(LogLevelError, "error publishing node control command", map[string]any{"error": err.Error()}))
		return err
	}
	go n.sendNodePing()
	go n.cleanNodeInfo()
	return n.subDissolver.Run()
}

func (n *Node) Log(entry LogEntry) {
	n.logger.log(entry)
}

func (n *Node) LogEnabled(level LogLevel) bool {
	return n.logger.enabled(level)
}

func (n *Node) Shutdown(ctx context.Context) error {
	n.mu.Lock()
	if n.shutdown {
		n.mu.Unlock()
		return nil
	}
	n.shutdown = true
	close(n.shutdownCh)
	n.mu.Unlock()
	cmd := &controlpb.Command{
		Uid:      n.uid,
		Shutdown: &controlpb.Shutdown{},
	}
	_ = n.publishControl(cmd, "")
	if closer, ok := n.broker.(Closer); ok {
		defer func() { _ = closer.Close(ctx) }()
	}
	if n.presenceManager != nil {
		if closer, ok := n.presenceManager.(Closer); ok {
			defer func() { _ = closer.Close(ctx) }()
		}
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = n.subDissolver.Close()
	}()
	go func() {
		defer wg.Done()
		_ = n.hub.shutdown(ctx)
	}()
	wg.Wait()
	return ctx.Err()
}

func (n *Node) NotifyShutdown() chan struct{} {
	return n.shutdownCh
}

func (n *Node) sendNodePing() {
	for {
		select {
		case <-n.shutdownCh:
			return
		case <-time.After(nodeInfoPublishInterval):
			err := n.pubNode("")
			if err != nil {
				n.logger.log(newLogEntry(LogLevelError, "error publishing node control command", map[string]any{"error": err.Error()}))
			}
		}
	}
}

func (n *Node) cleanNodeInfo() {
	for {
		select {
		case <-n.shutdownCh:
			return
		case <-time.After(nodeInfoCleanInterval):
			n.nodes.clean(nodeInfoMaxDelay)
		}
	}
}

type Info struct {
	Nodes []NodeInfo
}

type NodeInfo struct {
	UID         string
	Name        string
	Version     string
	NumClients  uint32
	NumUsers    uint32
	NumSubs     uint32
	NumChannels uint32
	Uptime      uint32
	Data        []byte
}

func (n *Node) Info() (Info, error) {
	nodes := n.nodes.list()
	nodeResults := make([]NodeInfo, len(nodes))
	for i, nd := range nodes {
		info := NodeInfo{
			UID:         nd.Uid,
			Name:        nd.Name,
			Version:     nd.Version,
			NumClients:  nd.NumClients,
			NumUsers:    nd.NumUsers,
			NumSubs:     nd.NumSubs,
			NumChannels: nd.NumChannels,
			Uptime:      nd.Uptime,
			Data:        nd.Data,
		}
		nodeResults[i] = info
	}

	return Info{
		Nodes: nodeResults,
	}, nil
}

func (n *Node) handleControl(data []byte) error {
	cmd, err := n.controlDecoder.DecodeCommand(data)
	if err != nil {
		n.logger.log(newLogEntry(LogLevelError, "error decoding control command", map[string]any{"error": err.Error()}))
		return err
	}

	if cmd.Uid == n.uid {
		return nil
	}

	uid := cmd.Uid

	if cmd.Node != nil {
		return n.nodeCmd(cmd.Node)
	} else if cmd.Shutdown != nil {
		return n.shutdownCmd(uid)
	} else if cmd.Unsubscribe != nil {
		cmd := cmd.Unsubscribe
		return n.hub.unsubscribe(cmd.User, cmd.Channel, Unsubscribe{Code: cmd.Code, Reason: cmd.Reason}, cmd.Client)
	} else if cmd.Subscribe != nil {
		cmd := cmd.Subscribe
		return n.hub.subscribe(cmd.User, cmd.Channel, cmd.Client, WithExpireAt(cmd.ExpireAt), WithChannelInfo(cmd.ChannelInfo), WithEmitPresence(cmd.EmitPresence), WithEmitJoinLeave(cmd.EmitJoinLeave), WithPushJoinLeave(cmd.PushJoinLeave), WithSubscribeData(cmd.Data), WithSubscribeSource(uint8(cmd.Source)))
	} else if cmd.Disconnect != nil {
		cmd := cmd.Disconnect
		return n.hub.disconnect(cmd.User, Disconnect{Code: cmd.Code, Reason: cmd.Reason}, cmd.Client, cmd.Whitelist)
	} else if cmd.Refresh != nil {
		cmd := cmd.Refresh
		return n.hub.refresh(cmd.User, cmd.Client, WithRefreshExpired(cmd.Expired), WithRefreshExpireAt(cmd.ExpireAt), WithRefreshInfo(cmd.Info))
	}
	return nil
}

func (n *Node) handlePublication(ch string, pub *Publication) error {
	numSubscribers := n.hub.NumSubscribers(ch)
	hasCurrentSubscribers := numSubscribers > 0
	if !hasCurrentSubscribers {
		return nil
	}
	return n.hub.BroadcastPublication(ch, pub)
}

func (n *Node) publish(ch string, data []byte, opts ...PublishOption) (PublishResult, error) {
	pubOpts := &PublishOptions{}
	for _, opt := range opts {
		opt(pubOpts)
	}
	err := n.broker.Publish(ch, data, *pubOpts)
	return PublishResult{}, err
}

type PublishResult struct {
}

func (n *Node) Publish(channel string, data []byte, opts ...PublishOption) (PublishResult, error) {
	return n.publish(channel, data, opts...)
}

var errNotificationHandlerNotRegistered = errors.New("notification handler not registered")

func (n *Node) publishControl(cmd *controlpb.Command, nodeID string) error {
	data, err := n.controlEncoder.EncodeCommand(cmd)
	if err != nil {
		return err
	}
	return n.broker.PublishControl(data, nodeID, "")
}

func (n *Node) pubNode(nodeID string) error {
	var data []byte
	n.mu.RLock()
	node := &controlpb.Node{
		Uid:         n.uid,
		Name:        n.config.Name,
		Version:     n.config.Version,
		NumClients:  uint32(n.hub.NumClients()),
		NumUsers:    uint32(n.hub.NumUsers()),
		NumChannels: uint32(n.hub.NumChannels()),
		NumSubs:     uint32(n.hub.NumSubscriptions()),
		Uptime:      uint32(time.Now().Unix() - n.startedAt),
		Data:        data,
	}

	n.mu.RUnlock()

	cmd := &controlpb.Command{
		Uid:  n.uid,
		Node: node,
	}

	err := n.nodeCmd(node)
	if err != nil {
		n.logger.log(newLogEntry(LogLevelError, "error handling node command", map[string]any{"error": err.Error()}))
	}

	return n.publishControl(cmd, nodeID)
}

func (n *Node) pubSubscribe(user string, ch string, opts SubscribeOptions) error {
	subscribe := &controlpb.Subscribe{
		User:          user,
		Channel:       ch,
		EmitPresence:  opts.EmitPresence,
		EmitJoinLeave: opts.EmitJoinLeave,
		PushJoinLeave: opts.PushJoinLeave,
		ChannelInfo:   opts.ChannelInfo,
		ExpireAt:      opts.ExpireAt,
		Client:        opts.clientID,
		Data:          opts.Data,
		Source:        uint32(opts.Source),
	}
	cmd := &controlpb.Command{
		Uid:       n.uid,
		Subscribe: subscribe,
	}
	return n.publishControl(cmd, "")
}

func (n *Node) pubRefresh(user string, opts RefreshOptions) error {
	refresh := &controlpb.Refresh{
		User:     user,
		Expired:  opts.Expired,
		ExpireAt: opts.ExpireAt,
		Client:   opts.clientID,
		Session:  opts.sessionID,
		Info:     opts.Info,
	}
	cmd := &controlpb.Command{
		Uid:     n.uid,
		Refresh: refresh,
	}
	return n.publishControl(cmd, "")
}

func (n *Node) pubUnsubscribe(user string, ch string, unsubscribe Unsubscribe, clientID, sessionID string) error {
	unsub := &controlpb.Unsubscribe{
		User:    user,
		Channel: ch,
		Code:    unsubscribe.Code,
		Reason:  unsubscribe.Reason,
		Client:  clientID,
		Session: sessionID,
	}
	cmd := &controlpb.Command{
		Uid:         n.uid,
		Unsubscribe: unsub,
	}
	return n.publishControl(cmd, "")
}

func (n *Node) pubDisconnect(user string, disconnect Disconnect, clientID string, whitelist []string) error {
	protoDisconnect := &controlpb.Disconnect{
		User:      user,
		Whitelist: whitelist,
		Code:      disconnect.Code,
		Reason:    disconnect.Reason,
		Client:    clientID,
	}
	cmd := &controlpb.Command{
		Uid:        n.uid,
		Disconnect: protoDisconnect,
	}
	return n.publishControl(cmd, "")
}

func (n *Node) addClient(c *Client) error {
	return n.hub.add(c)
}

func (n *Node) removeClient(c *Client) error {
	return n.hub.remove(c)
}

func (n *Node) InfoUsers(userIDs []string) []*UserInfo {
	uids := make([]*UserInfo, 0)
	for _, c := range userIDs {
		uids = append(uids, n.hub.InfoUser(c))
	}
	return uids
}

func (n *Node) addSubscription(ch string, c *Client) error {
	mu := n.subLock(ch)
	mu.Lock()
	defer mu.Unlock()
	first, err := n.hub.addSub(ch, c)
	if err != nil {
		return err
	}
	if first {
		err := n.broker.Subscribe(ch)
		if err != nil {
			_, _ = n.hub.removeSub(ch, c)
			return err
		}
	}
	return nil
}

func (n *Node) removeSubscription(ch string, c *Client) error {
	mu := n.subLock(ch)
	mu.Lock()
	defer mu.Unlock()
	empty, err := n.hub.removeSub(ch, c)
	if err != nil {
		return err
	}
	if empty {
		submittedAt := time.Now()
		_ = n.subDissolver.Submit(func() error {
			timeSpent := time.Since(submittedAt)
			if timeSpent < time.Second {
				time.Sleep(time.Second - timeSpent)
			}
			mu := n.subLock(ch)
			mu.Lock()
			defer mu.Unlock()
			empty := n.hub.NumSubscribers(ch) == 0
			if empty {
				err := n.broker.Unsubscribe(ch)
				if err != nil {
					time.Sleep(500 * time.Millisecond)
				}
				return err
			}
			return nil
		})
	}
	return nil
}

func (n *Node) nodeCmd(node *controlpb.Node) error {
	isNewNode := n.nodes.add(node)
	if isNewNode && node.Uid != n.uid {
		_ = n.pubNode(node.Uid)
	}
	return nil
}

func (n *Node) shutdownCmd(nodeID string) error {
	n.nodes.remove(nodeID)
	return nil
}

func (n *Node) Subscribe(userID string, channel string, opts ...SubscribeOption) error {
	subscribeOpts := &SubscribeOptions{}
	for _, opt := range opts {
		opt(subscribeOpts)
	}

	err := n.hub.subscribe(userID, channel, subscribeOpts.clientID, opts...)
	if err != nil {
		return err
	}
	return n.pubSubscribe(userID, channel, *subscribeOpts)
}

func (n *Node) Unsubscribe(userID string, channel string, opts ...UnsubscribeOption) error {
	unsubscribeOpts := &UnsubscribeOptions{}
	for _, opt := range opts {
		opt(unsubscribeOpts)
	}
	customUnsubscribe := unsubscribeServer
	if unsubscribeOpts.unsubscribe != nil {
		customUnsubscribe = *unsubscribeOpts.unsubscribe
	}

	err := n.hub.unsubscribe(userID, channel, customUnsubscribe, unsubscribeOpts.clientID)
	if err != nil {
		return err
	}
	return n.pubUnsubscribe(userID, channel, customUnsubscribe, unsubscribeOpts.clientID, unsubscribeOpts.sessionID)
}

func (n *Node) Disconnect(userID string, opts ...DisconnectOption) error {
	disconnectOpts := &DisconnectOptions{}
	for _, opt := range opts {
		opt(disconnectOpts)
	}
	customDisconnect := DisconnectForceNoReconnect
	if disconnectOpts.Disconnect != nil {
		customDisconnect = *disconnectOpts.Disconnect
	}
	err := n.hub.disconnect(userID, customDisconnect, disconnectOpts.clientID, disconnectOpts.ClientWhitelist)
	if err != nil {
		return err
	}
	return n.pubDisconnect(userID, customDisconnect, disconnectOpts.clientID, disconnectOpts.ClientWhitelist)
}

func (n *Node) Refresh(userID string, opts ...RefreshOption) error {
	refreshOpts := &RefreshOptions{}
	for _, opt := range opts {
		opt(refreshOpts)
	}
	err := n.hub.refresh(userID, refreshOpts.clientID, opts...)
	if err != nil {
		return err
	}
	return n.pubRefresh(userID, *refreshOpts)
}

func (n *Node) addPresence(ch string, uid string, info *ClientInfo) error {
	if n.presenceManager == nil {
		return nil
	}
	return n.presenceManager.AddPresence(ch, uid, info)
}

func (n *Node) removePresence(ch string, uid string) error {
	if n.presenceManager == nil {
		return nil
	}
	return n.presenceManager.RemovePresence(ch, uid)
}

var (
	presenceGroup      singleflight.Group
	presenceStatsGroup singleflight.Group
)

type PresenceResult struct {
	Presence map[string]*ClientInfo
}

func (n *Node) presence(ch string) (PresenceResult, error) {
	presence, err := n.presenceManager.Presence(ch)
	if err != nil {
		return PresenceResult{}, err
	}
	return PresenceResult{Presence: presence}, nil
}

func (n *Node) Presence(ch string) (PresenceResult, error) {
	if n.presenceManager == nil {
		return PresenceResult{}, ErrorNotAvailable
	}
	if n.config.UseSingleFlight {
		result, err, _ := presenceGroup.Do(ch, func() (any, error) {
			return n.presence(ch)
		})
		return result.(PresenceResult), err
	}
	return n.presence(ch)
}

func infoFromProto(v *deliverprotocol.ClientInfo) *ClientInfo {
	if v == nil {
		return nil
	}
	info := &ClientInfo{
		ClientID: v.GetClient(),
		UserID:   v.GetUser(),
	}
	if len(v.ConnInfo) > 0 {
		info.ConnInfo = v.ConnInfo
	}
	if len(v.ChanInfo) > 0 {
		info.ChanInfo = v.ChanInfo
	}
	return info
}

func infoToProto(v *ClientInfo) *deliverprotocol.ClientInfo {
	if v == nil {
		return nil
	}
	info := &deliverprotocol.ClientInfo{
		Client: v.ClientID,
		User:   v.UserID,
	}
	if len(v.ConnInfo) > 0 {
		info.ConnInfo = v.ConnInfo
	}
	if len(v.ChanInfo) > 0 {
		info.ChanInfo = v.ChanInfo
	}
	return info
}

func pubToProto(pub *Publication) *deliverprotocol.Publication {
	if pub == nil {
		return nil
	}
	return &deliverprotocol.Publication{
		Data: pub.Data,
		Info: infoToProto(pub.Info),
		Tags: pub.Tags,
	}
}

func pubFromProto(pub *deliverprotocol.Publication) *Publication {
	if pub == nil {
		return nil
	}
	return &Publication{
		Data: pub.Data,
		Info: infoFromProto(pub.GetInfo()),
		Tags: pub.GetTags(),
	}
}

type PresenceStatsResult struct {
	PresenceStats
}

func (n *Node) presenceStats(ch string) (PresenceStatsResult, error) {
	presenceStats, err := n.presenceManager.PresenceStats(ch)
	if err != nil {
		return PresenceStatsResult{}, err
	}
	return PresenceStatsResult{PresenceStats: presenceStats}, nil
}

func (n *Node) PresenceStats(ch string) (PresenceStatsResult, error) {
	if n.presenceManager == nil {
		return PresenceStatsResult{}, ErrorNotAvailable
	}
	if n.config.UseSingleFlight {
		result, err, _ := presenceStatsGroup.Do(ch, func() (any, error) {
			return n.presenceStats(ch)
		})
		return result.(PresenceStatsResult), err
	}
	return n.presenceStats(ch)
}

type nodeRegistry struct {
	mu         sync.RWMutex
	currentUID string
	nodes      map[string]*controlpb.Node
	updates    map[string]int64
}

func newNodeRegistry(currentUID string) *nodeRegistry {
	return &nodeRegistry{
		currentUID: currentUID,
		nodes:      make(map[string]*controlpb.Node),
		updates:    make(map[string]int64),
	}
}

func (r *nodeRegistry) list() []*controlpb.Node {
	r.mu.RLock()
	nodes := make([]*controlpb.Node, len(r.nodes))
	i := 0
	for _, info := range r.nodes {
		nodes[i] = info
		i++
	}
	r.mu.RUnlock()
	return nodes
}

func (r *nodeRegistry) size() int {
	r.mu.RLock()
	size := len(r.nodes)
	r.mu.RUnlock()
	return size
}

func (r *nodeRegistry) get(uid string) (*controlpb.Node, bool) {
	r.mu.RLock()
	info, ok := r.nodes[uid]
	r.mu.RUnlock()
	return info, ok
}

func (r *nodeRegistry) add(info *controlpb.Node) bool {
	var isNewNode bool
	r.mu.Lock()
	if node, ok := r.nodes[info.Uid]; ok {
		if info.Metrics != nil {
			r.nodes[info.Uid] = info
		} else {
			r.nodes[info.Uid] = &controlpb.Node{
				Uid:         info.Uid,
				Name:        info.Name,
				Version:     info.Version,
				NumClients:  info.NumClients,
				NumUsers:    info.NumUsers,
				NumChannels: info.NumChannels,
				Uptime:      info.Uptime,
				Data:        info.Data,
				NumSubs:     info.NumSubs,
				Metrics:     node.Metrics,
			}
		}
	} else {
		r.nodes[info.Uid] = info
		isNewNode = true
	}
	r.updates[info.Uid] = time.Now().Unix()
	r.mu.Unlock()
	return isNewNode
}

func (r *nodeRegistry) remove(uid string) {
	r.mu.Lock()
	delete(r.nodes, uid)
	delete(r.updates, uid)
	r.mu.Unlock()
}

func (r *nodeRegistry) clean(delay time.Duration) {
	r.mu.Lock()
	for uid := range r.nodes {
		if uid == r.currentUID {
			continue
		}
		updated, ok := r.updates[uid]
		if !ok {
			delete(r.nodes, uid)
			continue
		}
		if time.Now().Unix()-updated > int64(delay.Seconds()) {
			delete(r.nodes, uid)
			delete(r.updates, uid)
		}
	}
	r.mu.Unlock()
}

type eventHub struct {
	connectingHandler     ConnectingHandler
	connectHandler        ConnectHandler
	transportWriteHandler TransportWriteHandler
	commandReadHandler    CommandReadHandler
}

func (n *Node) OnConnecting(handler ConnectingHandler) {
	n.clientEvents.connectingHandler = handler
}

func (n *Node) OnConnect(handler ConnectHandler) {
	n.clientEvents.connectHandler = handler
}

func (n *Node) OnTransportWrite(handler TransportWriteHandler) {
	n.clientEvents.transportWriteHandler = handler
}

func (n *Node) OnCommandRead(handler CommandReadHandler) {
	n.clientEvents.commandReadHandler = handler
}

type brokerEventHandler struct {
	node *Node
}

func (h *brokerEventHandler) HandlePublication(ch string, pub *Publication) error {
	if pub == nil {
		panic("nil Publication received, this must never happen")
	}
	return h.node.handlePublication(ch, pub)
}

func (h *brokerEventHandler) HandleControl(data []byte) error {
	return h.node.handleControl(data)
}
