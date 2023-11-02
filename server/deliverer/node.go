package deliverer

import (
	"fmt"
	"hash/fnv"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/THPTUHA/kairos/server/deliverer/internal/controlpb"
	"github.com/THPTUHA/kairos/server/deliverer/internal/controlproto"
	"github.com/THPTUHA/kairos/server/deliverer/internal/dissolve"
	"github.com/THPTUHA/kairos/server/deliverer/internal/nowtime"
	"github.com/centrifugal/protocol"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
)

type Node struct {
	mu sync.RWMutex
	// unique id for this node.
	uid string
	// startedAt is unix time of node start.
	startedAt int64
	// config for node.
	config Config
	// hub to manage client connections.
	hub *Hub
	// broker is responsible for PUB/SUB and history streaming mechanics.
	broker Broker
	// presenceManager is responsible for presence information management.
	presenceManager PresenceManager
	// nodes contains registry of known nodes.
	nodes *nodeRegistry
	// shutdown is a flag which is only true when node is going to shut down.
	shutdown bool
	// shutdownCh is a channel which is closed when node shutdown initiated.
	shutdownCh chan struct{}
	// clientEvents to manage event handlers attached to node.
	clientEvents *eventHub
	// logger allows to log throughout library code and proxy log entries to
	// configured log handler.
	logger *logger
	// cache control encoder in Node.
	controlEncoder controlproto.Encoder
	// cache control decoder in Node.
	controlDecoder controlproto.Decoder
	// subLocks synchronizes access to adding/removing subscriptions.
	subLocks map[int]*sync.Mutex
	// subDissolver used to reliably clear unused subscriptions in Broker.
	subDissolver *dissolve.Dissolver

	// nowTimeGetter provides access to current time.
	nowTimeGetter nowtime.Getter

	surveyHandler  SurveyHandler
	surveyRegistry map[uint64]chan survey
	surveyMu       sync.RWMutex
	surveyID       uint64

	nodeInfoSendHandler NodeInfoSendHandler

	emulationSurveyHandler *emulationSurveyHandler
}

// eventHub allows binding client event handlers.
// All eventHub methods are not goroutine-safe and supposed
// to be called once before Node Run called.
type eventHub struct {
	connectingHandler       ConnectingHandler
	connectHandler          ConnectHandler
	transportWriteHandler   TransportWriteHandler
	commandReadHandler      CommandReadHandler
	commandProcessedHandler CommandProcessedHandler
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

// PublishResult returned from Publish operation.
type PublishResult struct {
	StreamPosition
}

// PresenceResult wraps presence.
type PresenceResult struct {
	Presence map[string]*ClientInfo
}

// PresenceStatsResult wraps presence stats.
type PresenceStatsResult struct {
	PresenceStats
}

// HistoryResult contains Publications and current stream top StreamPosition.
type HistoryResult struct {
	// StreamPosition embedded here describes current stream top offset and epoch.
	StreamPosition
	// Publications extracted from history storage according to HistoryFilter.
	Publications []*Publication
}

// CommandProcessedEvent contains protocol.Command processed by Client. Command and
// Reply types and their fields in the event MAY BE POOLED by Centrifuge, so code
// which wants to use them AFTER CommandProcessedHandler handler returns MUST MAKE A
// COPY.
type CommandProcessedEvent struct {
	// Command which was processed. May be pooled - see comment of CommandProcessedEvent.
	Command *protocol.Command
	// Disconnect may be set if Command processing resulted into disconnection.
	Disconnect *Disconnect
	// Reply to the command. Reply may be pooled - see comment of CommandProcessedEvent.
	// This Reply may be nil in the following cases:
	// 1. For Send command since send commands do not have replies
	// 2. When Disconnect field of CommandProcessedEvent is not nil
	// 3. When unidirectional transport connects (we create Connect Command artificially
	// with id: 1 and we never send replies to unidirectional transport, only pushes).
	Reply *protocol.Reply
	// Started is a time command was passed to Client for processing.
	Started time.Time
}

// CommandProcessedHandler allows setting a callback which will be called after
// Client processed a protocol.Command. This exists mostly for real-time connection
// tracing purposes. CommandProcessedHandler may be called after the corresponding
// Reply written to the connection and TransportWriteHandler called. But for tracing
// purposes this seems tolerable as commands and replies may be matched by id.
// Also, carefully read docs for CommandProcessedEvent to avoid possible bugs.
type CommandProcessedHandler func(*Client, CommandProcessedEvent)

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
	if c.HistoryMetaTTL == 0 {
		c.HistoryMetaTTL = 30 * 24 * time.Hour // 30 days by default.
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
		surveyRegistry: make(map[uint64]chan survey),
	}

	n.emulationSurveyHandler = newEmulationSurveyHandler(n)

	b, err := NewMemoryBroker(n, MemoryBrokerConfig{})
	if err != nil {
		return nil, err
	}
	n.SetBroker(b)
	return n, nil
}

// SetBroker allows setting Broker implementation to use.
func (n *Node) SetBroker(b Broker) {
	n.broker = b
}

func newNodeRegistry(currentUID string) *nodeRegistry {
	return &nodeRegistry{
		currentUID: currentUID,
		nodes:      make(map[string]*controlpb.Node),
		updates:    make(map[string]int64),
	}
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

// nodeCmd handles node control command i.e. updates information about known nodes.
func (n *Node) nodeCmd(node *controlpb.Node) error {
	isNewNode := n.nodes.add(node)
	if isNewNode && node.Uid != n.uid {
		// New Node in cluster
		_ = n.pubNode(node.Uid)
	}
	return nil
}

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
	if n.nodeInfoSendHandler != nil {
		reply := n.nodeInfoSendHandler()
		data = reply.Data
	}
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

// OnConnect allows setting ConnectHandler.
// ConnectHandler called after client connection successfully established,
// authenticated and Connect Reply already sent to client. This is a place where
// application can start communicating with client.
func (n *Node) OnConnect(handler ConnectHandler) {
	n.clientEvents.connectHandler = handler
}

type brokerEventHandler struct {
	node *Node
}

// handlePublication handles messages published into channel and
// coming from Broker. The goal of method is to deliver this message
// to all clients on this node currently subscribed to channel.
func (n *Node) handlePublication(ch string, pub *Publication, sp StreamPosition) error {
	numSubscribers := n.hub.NumSubscribers(ch)
	hasCurrentSubscribers := numSubscribers > 0
	if !hasCurrentSubscribers {
		return nil
	}
	return n.hub.BroadcastPublication(ch, pub, sp)
}

// HandlePublication coming from Broker.
func (h *brokerEventHandler) HandlePublication(ch string, pub *Publication, sp StreamPosition) error {
	if pub == nil {
		panic("nil Publication received, this must never happen")
	}
	return h.node.handlePublication(ch, pub, sp)
}

// handleJoin handles join messages - i.e. broadcasts it to
// interested local clients subscribed to channel.
func (n *Node) handleJoin(ch string, info *ClientInfo) error {
	numSubscribers := n.hub.NumSubscribers(ch)
	hasCurrentSubscribers := numSubscribers > 0
	if !hasCurrentSubscribers {
		return nil
	}
	return n.hub.broadcastJoin(ch, info)
}

// HandleJoin coming from Broker.
func (h *brokerEventHandler) HandleJoin(ch string, info *ClientInfo) error {
	if info == nil {
		panic("nil join ClientInfo received, this must never happen")
	}
	return h.node.handleJoin(ch, info)
}

func (n *Node) handleLeave(ch string, info *ClientInfo) error {
	numSubscribers := n.hub.NumSubscribers(ch)
	hasCurrentSubscribers := numSubscribers > 0
	if !hasCurrentSubscribers {
		return nil
	}
	return n.hub.broadcastLeave(ch, info)
}

// HandleLeave coming from Broker.
func (h *brokerEventHandler) HandleLeave(ch string, info *ClientInfo) error {
	if info == nil {
		panic("nil leave ClientInfo received, this must never happen")
	}
	return h.node.handleLeave(ch, info)
}

// TODo
// handleControl handles messages from control channel - control messages used for internal
// communication between nodes to share state or proto.
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

	// uid := cmd.Uid

	// control proto v2.
	if cmd.Node != nil {
		return n.nodeCmd(cmd.Node)
	} else if cmd.Subscribe != nil {
		cmd := cmd.Subscribe
		var recoverSince *StreamPosition
		if cmd.RecoverSince != nil {
			recoverSince = &StreamPosition{Offset: cmd.RecoverSince.Offset, Epoch: cmd.RecoverSince.Epoch}
		}
		return n.hub.subscribe(cmd.User, cmd.Channel, cmd.Client, cmd.Session, WithExpireAt(cmd.ExpireAt), WithChannelInfo(cmd.ChannelInfo), WithEmitPresence(cmd.EmitPresence), WithEmitJoinLeave(cmd.EmitJoinLeave), WithPushJoinLeave(cmd.PushJoinLeave), WithPositioning(cmd.Position), WithRecovery(cmd.Recover), WithSubscribeData(cmd.Data), WithRecoverSince(recoverSince), WithSubscribeSource(uint8(cmd.Source)))
	}
	// else if cmd.Shutdown != nil {
	// 	return n.shutdownCmd(uid)
	// } else if cmd.Unsubscribe != nil {
	// 	cmd := cmd.Unsubscribe
	// 	return n.hub.unsubscribe(cmd.User, cmd.Channel, Unsubscribe{Code: cmd.Code, Reason: cmd.Reason}, cmd.Client, cmd.Session)
	// } else if cmd.Disconnect != nil {
	// 	cmd := cmd.Disconnect
	// 	return n.hub.disconnect(cmd.User, Disconnect{Code: cmd.Code, Reason: cmd.Reason}, cmd.Client, cmd.Session, cmd.Whitelist)
	// } else if cmd.SurveyRequest != nil {
	// 	cmd := cmd.SurveyRequest
	// 	return n.handleSurveyRequest(uid, cmd)
	// } else if cmd.SurveyResponse != nil {
	// 	cmd := cmd.SurveyResponse
	// 	return n.handleSurveyResponse(uid, cmd)
	// } else if cmd.Notification != nil {
	// 	cmd := cmd.Notification
	// 	return n.handleNotification(uid, cmd)
	// } else if cmd.Refresh != nil {
	// 	cmd := cmd.Refresh
	// 	return n.hub.refresh(cmd.User, cmd.Client, cmd.Session, WithRefreshExpired(cmd.Expired), WithRefreshExpireAt(cmd.ExpireAt), WithRefreshInfo(cmd.Info))
	// }
	n.logger.log(newLogEntry(LogLevelError, "unknown control command", map[string]any{"command": fmt.Sprintf("%#v", cmd)}))
	return nil
}

// HandleControl coming from Broker.
func (h *brokerEventHandler) HandleControl(data []byte) error {
	return h.node.handleControl(data)
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

// addClient registers authenticated connection in clientConnectionHub
// this allows to make operations with user connection on demand.
func (n *Node) addClient(c *Client) error {
	return n.hub.add(c)
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

// LogEnabled allows check whether a LogLevel enabled or not.
func (n *Node) LogEnabled(level LogLevel) bool {
	return n.logger.enabled(level)
}

// NotifyShutdown returns a channel which will be closed on node shutdown.
func (n *Node) NotifyShutdown() chan struct{} {
	return n.shutdownCh
}

// ID returns unique Node identifier. This is a UUID v4 value.
func (n *Node) ID() string {
	return n.uid
}

func (n *Node) subLock(ch string) *sync.Mutex {
	return n.subLocks[index(ch, numSubLocks)]
}

// publishJoin allows publishing join message into channel when someone subscribes on it
// or leave message when someone unsubscribes from channel.
func (n *Node) publishJoin(ch string, info *ClientInfo) error {
	return n.broker.PublishJoin(ch, info)
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

// addPresence proxies presence adding to PresenceManager.
func (n *Node) addPresence(ch string, uid string, info *ClientInfo) error {
	if n.presenceManager == nil {
		return nil
	}
	return n.presenceManager.AddPresence(ch, uid, info)
}

var (
	presenceGroup      singleflight.Group
	presenceStatsGroup singleflight.Group
	historyGroup       singleflight.Group
)

// History allows extracting Publications in channel.
// The channel must belong to namespace where history is on.
func (n *Node) History(ch string, opts ...HistoryOption) (HistoryResult, error) {
	historyOpts := &HistoryOptions{}
	for _, opt := range opts {
		opt(historyOpts)
	}
	if n.config.UseSingleFlight {
		var builder strings.Builder
		builder.WriteString("channel:")
		builder.WriteString(ch)
		if historyOpts.Filter.Since != nil {
			builder.WriteString(",offset:")
			builder.WriteString(strconv.FormatUint(historyOpts.Filter.Since.Offset, 10))
			builder.WriteString(",epoch:")
			builder.WriteString(historyOpts.Filter.Since.Epoch)
		}
		builder.WriteString(",limit:")
		builder.WriteString(strconv.Itoa(historyOpts.Filter.Limit))
		builder.WriteString(",reverse:")
		builder.WriteString(strconv.FormatBool(historyOpts.Filter.Reverse))
		builder.WriteString(",meta_ttl:")
		builder.WriteString(historyOpts.MetaTTL.String())
		key := builder.String()

		result, err, _ := historyGroup.Do(key, func() (any, error) {
			return n.history(ch, historyOpts)
		})
		return result.(HistoryResult), err
	}
	return n.history(ch, historyOpts)
}

func (n *Node) history(ch string, opts *HistoryOptions) (HistoryResult, error) {
	if opts.Filter.Reverse && opts.Filter.Since != nil && opts.Filter.Since.Offset == 0 {
		return HistoryResult{}, ErrorBadRequest
	}
	pubs, streamTop, err := n.broker.History(ch, *opts)
	if err != nil {
		return HistoryResult{}, err
	}
	if opts.Filter.Since != nil {
		sinceEpoch := opts.Filter.Since.Epoch
		epochOK := sinceEpoch == "" || sinceEpoch == streamTop.Epoch
		if !epochOK {
			return HistoryResult{
				StreamPosition: streamTop,
				Publications:   pubs,
			}, ErrorUnrecoverablePosition
		}
	}
	return HistoryResult{
		StreamPosition: streamTop,
		Publications:   pubs,
	}, nil
}

// recoverHistory recovers publications since StreamPosition last seen by client.
func (n *Node) recoverHistory(ch string, since StreamPosition, historyMetaTTL time.Duration) (HistoryResult, error) {
	limit := NoLimit
	maxPublicationLimit := n.config.RecoveryMaxPublicationLimit
	if maxPublicationLimit > 0 {
		limit = maxPublicationLimit
	}
	return n.History(ch, WithHistoryFilter(HistoryFilter{
		Limit: limit,
		Since: &since,
	}), WithHistoryMetaTTL(historyMetaTTL))
}

// streamTop returns current stream top StreamPosition for a channel.
func (n *Node) streamTop(ch string, historyMetaTTL time.Duration) (StreamPosition, error) {
	historyResult, err := n.History(ch, WithHistoryMetaTTL(historyMetaTTL))
	if err != nil {
		return StreamPosition{}, err
	}
	return historyResult.StreamPosition, nil
}

// Publish sends data to all clients subscribed on channel at this moment. All running
// nodes will receive Publication and send it to all local channel subscribers.
//
// Data expected to be valid marshaled JSON or any binary payload.
// Connections that work over JSON protocol can not handle binary payloads.
// Connections that work over Protobuf protocol can work both with JSON and binary payloads.
//
// So the rule here: if you have channel subscribers that work using JSON
// protocol then you can not publish binary data to these channel.
//
// Channels in Centrifuge are ephemeral and its settings not persisted over different
// publish operations. So if you want to have a channel with history stream behind you
// need to provide WithHistory option on every publish. To simplify working with different
// channels you can make some type of publish wrapper in your own code.
//
// The returned PublishResult contains embedded StreamPosition that describes
// position inside stream Publication was added too. For channels without history
// enabled (i.e. when Publications only sent to PUB/SUB system) StreamPosition will
// be an empty struct (i.e. PublishResult.Offset will be zero).
func (n *Node) Publish(channel string, data []byte, opts ...PublishOption) (PublishResult, error) {
	return n.publish(channel, data, opts...)
}

func (n *Node) publish(ch string, data []byte, opts ...PublishOption) (PublishResult, error) {
	pubOpts := &PublishOptions{}
	for _, opt := range opts {
		opt(pubOpts)
	}
	streamPos, err := n.broker.Publish(ch, data, *pubOpts)
	if err != nil {
		return PublishResult{}, err
	}
	return PublishResult{StreamPosition: streamPos}, nil
}

func (n *Node) removePresence(ch string, uid string) error {
	if n.presenceManager == nil {
		return nil
	}
	return n.presenceManager.RemovePresence(ch, uid)
}

// publishLeave allows publishing join message into channel when someone subscribes on it
// or leave message when someone unsubscribes from channel.
func (n *Node) publishLeave(ch string, info *ClientInfo) error {
	return n.broker.PublishLeave(ch, info)
}
