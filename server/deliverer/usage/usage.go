package usage

import (
	"context"
	"sync"
	"time"

	"github.com/THPTUHA/kairos/server/deliverer/rule"
	"github.com/centrifugal/centrifuge"
)

// Sender can send anonymous usage stats. Centrifugo does not collect any sensitive info.
// Only impersonal counters to estimate installation size distribution and feature use.
type Sender struct {
	mu             sync.RWMutex
	node           *centrifuge.Node
	rules          *rule.Container
	features       Features
	maxNumNodes    int
	maxNumClients  int
	maxNumChannels int
	lastSentAt     int64
}

// NewSender creates usage stats sender.
func NewSender(node *centrifuge.Node, rules *rule.Container, features Features) *Sender {
	return &Sender{
		node:     node,
		rules:    rules,
		features: features,
	}
}

// Features is a helper struct to build metrics.
type Features struct {
	// Build info.
	Version string
	Edition string

	// Engine or broker usage.
	Engine     string
	EngineMode string
	Broker     string
	BrokerMode string

	// Transports.
	Websocket     bool
	HTTPStream    bool
	SSE           bool
	SockJS        bool
	UniWebsocket  bool
	UniGRPC       bool
	UniSSE        bool
	UniHTTPStream bool

	// Proxies.
	ConnectProxy         bool
	RefreshProxy         bool
	SubscribeProxy       bool
	PublishProxy         bool
	RPCProxy             bool
	SubRefreshProxy      bool
	SubscribeStreamProxy bool

	// Uses GRPC server API.
	GrpcAPI bool
	// Admin interface enabled.
	Admin bool
	// Uses automatic personal channel subscribe.
	SubscribeToPersonal bool

	// PRO features.
	ClickhouseAnalytics bool
	UserStatus          bool
	Throttling          bool
	Singleflight        bool
}

// Start sending usage stats. How it works:
// First send in between 24-48h from node start.
// After the initial delay has passed: every hour check last time stats were sent by all
// the nodes in a Centrifugo cluster. If no points were sent in last 24h, then push metrics
// and update push time on all nodes (broadcast current time). There is still a chance of
// duplicate data sending – but should be rare and tolerable for the purpose.
func (s *Sender) Start(ctx context.Context) {
	firstTimeSend := time.Now().Add(initialDelay)
	if s.isDev() {
		s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "usage stats: schedule next send", map[string]any{"delay": initialDelay.String()}))
	}

	// Wait 1/4 of a delay to randomize hourly ticks on different nodes.
	select {
	case <-ctx.Done():
		return
	case <-time.After(initialDelay / 4):
	}

	if s.isDev() {
		s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "usage stats: start periodic ticks", map[string]any{}))
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(tickInterval):
			if s.isDev() {
				s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "usage stats: updating max values", map[string]any{}))
			}
			err := s.updateMaxValues()
			if err != nil {
				if s.isDev() {
					s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "usage stats: error updating max values", map[string]any{"error": err.Error()}))
				}
				continue
			}

			if time.Now().Before(firstTimeSend) {
				if s.isDev() {
					s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "usage stats: too early to send first time", map[string]any{}))
				}
				continue
			}

			s.mu.RLock()
			lastSentAt := s.lastSentAt
			s.mu.RUnlock()
			if lastSentAt > 0 {
				s.broadcastLastSentAt()
			}

			if lastSentAt > 0 && time.Now().Unix() <= lastSentAt+int64(sendInterval.Seconds()) {
				if s.isDev() {
					s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "usage stats: too early to send", map[string]any{}))
				}
				continue
			}

			if s.isDev() {
				s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "usage stats: sending usage stats", map[string]any{}))
			}
			metrics, err := s.prepareMetrics()
			if err != nil {
				if s.isDev() {
					s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "usage stats: error preparing metrics", map[string]any{"error": err.Error()}))
				}
				continue
			}
			err = s.sendUsageStats(metrics, build.UsageStatsEndpoint, build.UsageStatsToken)
			if err != nil {
				if s.isDev() {
					s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "usage stats: error sending", map[string]any{"error": err.Error()}))
				}
				continue
			}
			s.mu.Lock()
			s.lastSentAt = time.Now().Unix()
			s.resetMaxValues()
			s.mu.Unlock()
			s.broadcastLastSentAt()
		}
	}
}
