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
	// cache control encoder in Node.
	controlEncoder controlproto.Encoder
	// cache control decoder in Node.
	controlDecoder controlproto.Decoder
	// subLocks synchronizes access to adding/removing subscriptions.
	subLocks     map[int]*sync.Mutex
	subDissolver *dissolve.Dissolver

	nowTimeGetter nowtime.Getter
}

const (
	numSubLocks            = 16384
	numSubDissolverWorkers = 64
)

// New creates Node with provided Config.
func New(c Config) (*Node, error) {
	if c.NodeInfoMetricsAggregateInterval == 0 {
		c.NodeInfoMetricsAggregateInterval = 60 * time.Second
	}
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

// index chooses bucket number in range [0, numBuckets).
func index(s string, numBuckets int) int {
	if numBuckets == 1 {
		return 0
	}
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(s))
	return int(hash.Sum64() % uint64(numBuckets))
}

// Config returns Node's Config.
func (n *Node) Config() Config {
	return n.config
}

// ID returns unique Node identifier. This is a UUID v4 value.
func (n *Node) ID() string {
	return n.uid
}

func (n *Node) subLock(ch string) *sync.Mutex {
	return n.subLocks[index(ch, numSubLocks)]
}

// SetBroker allows setting Broker implementation to use.
func (n *Node) SetBroker(b Broker) {
	n.broker = b
}

// SetPresenceManager allows setting PresenceManager to use.
func (n *Node) SetPresenceManager(m PresenceManager) {
	n.presenceManager = m
}

// Hub returns node's Hub.
func (n *Node) Hub() *Hub {
	return n.hub
}

// Run performs node startup actions. At moment must be called once on start
// after Broker set to Node.
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

// Log allows logging a LogEntry.
func (n *Node) Log(entry LogEntry) {
	n.logger.log(entry)
}

// LogEnabled allows check whether a LogLevel enabled or not.
func (n *Node) LogEnabled(level LogLevel) bool {
	return n.logger.enabled(level)
}

// Shutdown sets shutdown flag to Node so handlers could stop accepting
// new requests and disconnects clients with shutdown reason.
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

// NotifyShutdown returns a channel which will be closed on node shutdown.
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

// Info contains information about all known server nodes.
type Info struct {
	Nodes []NodeInfo
}

// NodeInfo contains information about node.
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

// Info returns aggregated stats from all nodes.
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
		// Sent by this node.
		return nil
	}

	uid := cmd.Uid

	// control proto v2.
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

func (n *Node) handleJoin(ch string, info *ClientInfo) error {
	numSubscribers := n.hub.NumSubscribers(ch)
	hasCurrentSubscribers := numSubscribers > 0
	if !hasCurrentSubscribers {
		return nil
	}
	return n.hub.broadcastJoin(ch, info)
}
func (n *Node) handleLeave(ch string, info *ClientInfo) error {
	numSubscribers := n.hub.NumSubscribers(ch)
	hasCurrentSubscribers := numSubscribers > 0
	if !hasCurrentSubscribers {
		return nil
	}
	return n.hub.broadcastLeave(ch, info)
}

func (n *Node) publish(ch string, data []byte, opts ...PublishOption) (PublishResult, error) {
	pubOpts := &PublishOptions{}
	for _, opt := range opts {
		opt(pubOpts)
	}
	err := n.broker.Publish(ch, data, *pubOpts)
	return PublishResult{}, err
}

// PublishResult returned from Publish operation.
type PublishResult struct {
}

func (n *Node) Publish(channel string, data []byte, opts ...PublishOption) (PublishResult, error) {
	return n.publish(channel, data, opts...)
}

// publishJoin allows publishing join message into channel when someone subscribes on it
// or leave message when someone unsubscribes from channel.
func (n *Node) publishJoin(ch string, info *ClientInfo) error {
	return n.broker.PublishJoin(ch, info)
}

func (n *Node) publishLeave(ch string, info *ClientInfo) error {
	return n.broker.PublishLeave(ch, info)
}

var errNotificationHandlerNotRegistered = errors.New("notification handler not registered")

// publishControl publishes message into control channel so all running
// nodes will receive and handle it.
func (n *Node) publishControl(cmd *controlpb.Command, nodeID string) error {
	data, err := n.controlEncoder.EncodeCommand(cmd)
	if err != nil {
		return err
	}
	return n.broker.PublishControl(data, nodeID, "")
}

// pubNode sends control message to all nodes - this message
// contains information about current node.
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

// pubUnsubscribe publishes unsubscribe control message to all nodes – so all
// nodes could unsubscribe user from channel.
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

// pubDisconnect publishes disconnect control message to all nodes – so all
// nodes could disconnect user from server.
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

// addClient registers authenticated connection in clientConnectionHub
// this allows to make operations with user connection on demand.
func (n *Node) addClient(c *Client) error {
	return n.hub.add(c)
}

// removeClient removes client connection from connection registry.
func (n *Node) removeClient(c *Client) error {
	return n.hub.remove(c)
}

// addSubscription registers subscription of connection on channel in both
// Hub and Broker.
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

// removeSubscription removes subscription of connection on channel
// from Hub and Broker.
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
					// Cool down a bit since broker is not ready to process unsubscription.
					time.Sleep(500 * time.Millisecond)
				}
				return err
			}
			return nil
		})
	}
	return nil
}

// nodeCmd handles node control command i.e. updates information about known nodes.
func (n *Node) nodeCmd(node *controlpb.Node) error {
	isNewNode := n.nodes.add(node)
	if isNewNode && node.Uid != n.uid {
		// New Node in cluster
		_ = n.pubNode(node.Uid)
	}
	return nil
}

// shutdownCmd handles shutdown control command sent when node leaves cluster.
func (n *Node) shutdownCmd(nodeID string) error {
	n.nodes.remove(nodeID)
	return nil
}

// Subscribe subscribes user to a channel.
// Note, that OnSubscribe event won't be called in this case
// since this is a server-side subscription. If user have been already
// subscribed to a channel then its subscription will be updated and
// subscribe notification will be sent to a client-side.
func (n *Node) Subscribe(userID string, channel string, opts ...SubscribeOption) error {
	subscribeOpts := &SubscribeOptions{}
	for _, opt := range opts {
		opt(subscribeOpts)
	}
	// Subscribe on this node.
	err := n.hub.subscribe(userID, channel, subscribeOpts.clientID, opts...)
	if err != nil {
		return err
	}
	// Send subscribe control message to other nodes.
	return n.pubSubscribe(userID, channel, *subscribeOpts)
}

// Unsubscribe unsubscribes user from a channel.
// If a channel is empty string then user will be unsubscribed from all channels.
func (n *Node) Unsubscribe(userID string, channel string, opts ...UnsubscribeOption) error {
	unsubscribeOpts := &UnsubscribeOptions{}
	for _, opt := range opts {
		opt(unsubscribeOpts)
	}
	customUnsubscribe := unsubscribeServer
	if unsubscribeOpts.unsubscribe != nil {
		customUnsubscribe = *unsubscribeOpts.unsubscribe
	}

	// Unsubscribe on this node.
	err := n.hub.unsubscribe(userID, channel, customUnsubscribe, unsubscribeOpts.clientID)
	if err != nil {
		return err
	}
	// Send unsubscribe control message to other nodes.
	return n.pubUnsubscribe(userID, channel, customUnsubscribe, unsubscribeOpts.clientID, unsubscribeOpts.sessionID)
}

// Disconnect allows closing all user connections on all nodes.
func (n *Node) Disconnect(userID string, opts ...DisconnectOption) error {
	disconnectOpts := &DisconnectOptions{}
	for _, opt := range opts {
		opt(disconnectOpts)
	}
	// Disconnect user from this node
	customDisconnect := DisconnectForceNoReconnect
	if disconnectOpts.Disconnect != nil {
		customDisconnect = *disconnectOpts.Disconnect
	}
	err := n.hub.disconnect(userID, customDisconnect, disconnectOpts.clientID, disconnectOpts.ClientWhitelist)
	if err != nil {
		return err
	}
	// Send disconnect control message to other nodes
	return n.pubDisconnect(userID, customDisconnect, disconnectOpts.clientID, disconnectOpts.ClientWhitelist)
}

// Refresh user connection.
// Without any options will make user connections non-expiring.
// Note, that OnRefresh event won't be called in this case
// since this is a server-side refresh.
func (n *Node) Refresh(userID string, opts ...RefreshOption) error {
	refreshOpts := &RefreshOptions{}
	for _, opt := range opts {
		opt(refreshOpts)
	}
	// Refresh on this node.
	err := n.hub.refresh(userID, refreshOpts.clientID, opts...)
	if err != nil {
		return err
	}
	// Send refresh control message to other nodes.
	return n.pubRefresh(userID, *refreshOpts)
}

// addPresence proxies presence adding to PresenceManager.
func (n *Node) addPresence(ch string, uid string, info *ClientInfo) error {
	if n.presenceManager == nil {
		return nil
	}
	return n.presenceManager.AddPresence(ch, uid, info)
}

// removePresence proxies presence removing to PresenceManager.
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

// PresenceResult wraps presence.
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

// Presence returns a map with information about active clients in channel.
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

// PresenceStatsResult wraps presence stats.
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

// PresenceStats returns presence stats from PresenceManager.
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
	// mu allows synchronizing access to node registry.
	mu sync.RWMutex
	// currentUID keeps uid of current node
	currentUID string
	// nodes is a map with information about known nodes.
	nodes map[string]*controlpb.Node
	// updates track time we last received ping from node. Used to clean up nodes map.
	updates map[string]int64
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
			// No need to clean info for current node.
			continue
		}
		updated, ok := r.updates[uid]
		if !ok {
			// As we do all operations with nodes under lock this should never happen.
			delete(r.nodes, uid)
			continue
		}
		if time.Now().Unix()-updated > int64(delay.Seconds()) {
			// Too many seconds since this node have been last seen - remove it from map.
			delete(r.nodes, uid)
			delete(r.updates, uid)
		}
	}
	r.mu.Unlock()
}

// eventHub allows binding client event handlers.
// All eventHub methods are not goroutine-safe and supposed
// to be called once before Node Run called.
type eventHub struct {
	connectingHandler     ConnectingHandler
	connectHandler        ConnectHandler
	transportWriteHandler TransportWriteHandler
	commandReadHandler    CommandReadHandler
}

// OnConnecting allows setting ConnectingHandler.
// ConnectingHandler will be called when client sends Connect command to server.
// In this handler server can reject connection or provide Credentials for it.
func (n *Node) OnConnecting(handler ConnectingHandler) {
	n.clientEvents.connectingHandler = handler
}

func (n *Node) OnConnect(handler ConnectHandler) {
	n.clientEvents.connectHandler = handler
}

// OnTransportWrite allows setting TransportWriteHandler. This should be done before Node.Run called.
func (n *Node) OnTransportWrite(handler TransportWriteHandler) {
	n.clientEvents.transportWriteHandler = handler
}

// OnCommandRead allows setting CommandReadHandler. This should be done before Node.Run called.
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

// HandleJoin coming from Broker.
func (h *brokerEventHandler) HandleJoin(ch string, info *ClientInfo) error {
	if info == nil {
		panic("nil join ClientInfo received, this must never happen")
	}
	return h.node.handleJoin(ch, info)
}

func (h *brokerEventHandler) HandleLeave(ch string, info *ClientInfo) error {
	if info == nil {
		panic("nil leave ClientInfo received, this must never happen")
	}
	return h.node.handleLeave(ch, info)
}

// HandleControl coming from Broker.
func (h *brokerEventHandler) HandleControl(data []byte) error {
	return h.node.handleControl(data)
}
