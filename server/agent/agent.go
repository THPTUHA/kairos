package agent

import (
	"crypto/tls"
	"errors"
	"expvar"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/THPTUHA/kairos/server/plugin"
	"github.com/THPTUHA/kairos/server/plugin/proto"
	"github.com/hashicorp/memberlist"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"github.com/hashicorp/serf/serf"
	"github.com/soheilhy/cmux"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type Plugins struct {
	Processors map[string]plugin.Processor
	Executors  map[string]plugin.Executor
}

type Agent struct {
	// ProcessorPlugins maps processor plugins
	ProcessorPlugins map[string]plugin.Processor

	//ExecutorPlugins maps executor plugins
	ExecutorPlugins map[string]plugin.Executor
	config          *AgentConfig
	serf            *serf.Serf
	retryJoinCh     chan error
	ready           bool
	shutdownCh      chan struct{}

	// peers is used to track the known Agent servers. This is
	// used for region forwarding and clustering.
	peers      map[string][]*ServerParts
	localPeers map[raft.ServerAddress]*ServerParts
	eventCh    chan serf.Event
	sched      *Scheduler

	listener net.Listener

	// TLSConfig allows setting a TLS config for transport
	TLSConfig *tls.Config

	// GRPCClient interface for setting the GRPC client
	GRPCClient AgentGRPCClient

	// GRPCServer interface for setting the GRPC server
	GRPCServer ClusterAgentGRPCServer

	// Store interface to set the storage engine
	Store Storage

	// raftLayer provides network layering of the raft RPC along with
	// the Dkron gRPC transport layer.
	raftLayer     *RaftLayer
	raftInmem     *raft.InmemStore
	raftTransport *raft.NetworkTransport

	// The raft instance is used among Dkron nodes within the
	// region to protect operations that require strong consistency
	leaderCh <-chan bool
	raft     *raft.Raft

	// reconcileCh is used to pass events from the serf handler
	// into the leader manager. Mostly used to handle when servers
	// join/leave from the region.
	reconcileCh chan serf.Member
	raftStore   RaftStore

	ProAppliers        LogAppliers
	MemberEventHandler func(serf.Event)
}

type RaftStore interface {
	raft.StableStore
	raft.LogStore
	Close() error
}

const (
	minRaftProtocol = 3
	// raftLogCacheSize is the maximum number of logs to cache in-memory.
	// This is used to reduce disk I/O for the recently committed entries.
	raftLogCacheSize = 512
	raftTimeout      = 30 * time.Second
)

var (
	expNode           = expvar.NewString("node")
	runningExecutions sync.Map
)

var agent *Agent

// AgentOption type that defines agent options
type AgentOption func(agent *Agent)

func NewAgent(config *AgentConfig, options ...AgentOption) *Agent {
	agent := &Agent{
		config:      config,
		retryJoinCh: make(chan error),
	}

	for _, option := range options {
		option(agent)
	}

	return agent
}

// WithPlugins option to set plugins to the agent
func WithPlugins(plugins Plugins) AgentOption {
	return func(agent *Agent) {
		agent.ProcessorPlugins = plugins.Processors
		agent.ExecutorPlugins = plugins.Executors
	}
}

func (a *Agent) StartServer() {
	if a.Store == nil {
		s, err := NewStore()
		if err != nil {
			log.Fatalln("dkron: Error initializing store")
		}
		a.Store = s
	}

	a.sched = NewScheduler()

	// Create a cmux object.
	tcpm := cmux.New(a.listener)
	var grpcl, raftl net.Listener

	// Declare a plain RaftLayer
	a.raftLayer = NewRaftLayer()

	// Declare the match for gRPC
	grpcl = tcpm.MatchWithWriters(cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"))

	// Declare the match for raft RPC
	raftl = tcpm.Match(cmux.Any())

	if a.GRPCServer == nil {
		a.GRPCServer = NewGRPCServer(a)
	}

	if err := a.GRPCServer.Serve(grpcl); err != nil {
		log.Fatalln("agent: RPC server failed to start")
	}

	if err := a.raftLayer.Open(raftl); err != nil {
		log.Fatalln(err)
	}

	if err := a.setupRaft(); err != nil {
		log.Fatal("agent: Raft layer failed to start", err)
	}

	// Start serving everything
	go func() {
		if err := tcpm.Serve(); err != nil {
			log.Fatal(err)
		}
	}()
	go a.monitorLeadership()

}

func (a *Agent) setupRaft() error {
	if a.config.BootstrapExpect > 0 {
		if a.config.BootstrapExpect == 1 {
			a.config.Bootstrap = true
		}
	}
	logger := ioutil.Discard

	transport := raft.NewNetworkTransport(a.raftLayer, 3, raftTimeout, logger)
	a.raftTransport = transport

	config := raft.DefaultConfig()

	// Raft performance
	raftMultiplier := a.config.RaftMultiplier
	if raftMultiplier < 1 || raftMultiplier > 10 {
		return fmt.Errorf("raft-multiplier cannot be %d. Must be between 1 and 10", raftMultiplier)
	}

	config.HeartbeatTimeout = config.HeartbeatTimeout * time.Duration(raftMultiplier)
	config.ElectionTimeout = config.ElectionTimeout * time.Duration(raftMultiplier)
	config.LeaderLeaseTimeout = config.LeaderLeaseTimeout * time.Duration(a.config.RaftMultiplier)

	config.LogOutput = logger
	config.LocalID = raft.ServerID(a.config.NodeName)

	// Build an all in-memory setup for dev mode, otherwise prepare a full
	// disk-based setup.
	var logStore raft.LogStore
	var stableStore raft.StableStore
	var snapshots raft.SnapshotStore

	if a.config.DevMode {
		store := raft.NewInmemStore()
		a.raftInmem = store
		stableStore = store
		logStore = store
		snapshots = raft.NewDiscardSnapshotStore()
	} else {
		var err error
		// Create the snapshot store. This allows the Raft to truncate the log to
		// mitigate the issue of having an unbounded replicated log.
		snapshots, err = raft.NewFileSnapshotStore(filepath.Join(a.config.DataDir, "raft"), 3, logger)
		if err != nil {
			return fmt.Errorf("file snapshot store: %s", err)
		}

		// Create the BoltDB backend
		if a.raftStore == nil {
			s, err := raftboltdb.NewBoltStore(filepath.Join(a.config.DataDir, "raft", "raft.db"))
			if err != nil {
				return fmt.Errorf("error creating new raft store: %s", err)
			}
			a.raftStore = s
		}
		stableStore = a.raftStore
		// Wrap the store in a LogCache to improve performance
		cacheStore, err := raft.NewLogCache(raftLogCacheSize, a.raftStore)
		if err != nil {
			a.raftStore.Close()
			return err
		}
		logStore = cacheStore
		// Check for peers.json file for recovery
		peersFile := filepath.Join(a.config.DataDir, "raft", "peers.json")
		if _, err := os.Stat(peersFile); err == nil {
			fmt.Println("found peers.json file, recovering Raft configuration...")
			var configuration raft.Configuration
			configuration, err = raft.ReadConfigJSON(peersFile)
			if err != nil {
				return fmt.Errorf("recovery failed to parse peers.json: %v", err)
			}
			store, err := NewStore()
			if err != nil {
				log.Fatalln("agent: Error initializing store")
			}
			tmpFsm := newFSM(store, nil)
			if err := raft.RecoverCluster(config, tmpFsm,
				logStore, stableStore, snapshots, transport, configuration); err != nil {
				return fmt.Errorf("recovery failed: %v", err)
			}
			if err := os.Remove(peersFile); err != nil {
				return fmt.Errorf("recovery failed to delete peers.json, please delete manually (see peers.info for details): %v", err)
			}
			fmt.Println("deleted peers.json file after successful recovery")
		}
	}

	// If we are in bootstrap or dev mode and the state is clean then we can
	// bootstrap now.
	if a.config.Bootstrap || a.config.DevMode {
		hasState, err := raft.HasExistingState(logStore, stableStore, snapshots)
		if err != nil {
			return err
		}
		if !hasState {
			configuration := raft.Configuration{
				Servers: []raft.Server{
					{
						ID:      config.LocalID,
						Address: transport.LocalAddr(),
					},
				},
			}
			if err := raft.BootstrapCluster(config, logStore, stableStore, snapshots, transport, configuration); err != nil {
				return err
			}
		}
	}

	// Instantiate the Raft systems. The second parameter is a finite state machine
	// which stores the actual kv pairs and is operated upon through Apply().
	fsm := newFSM(a.Store, a.ProAppliers)
	rft, err := raft.NewRaft(config, fsm, logStore, stableStore, snapshots, transport)
	if err != nil {
		return fmt.Errorf("new raft: %s", err)
	}
	a.leaderCh = rft.LeaderCh()
	a.raft = rft
	return nil
}

// setupSerf is used to create the agent we use
func (a *Agent) setupSerf() (*serf.Serf, error) {
	config := a.config
	// Init peer list
	a.localPeers = make(map[raft.ServerAddress]*ServerParts)
	a.peers = make(map[string][]*ServerParts)

	bindIP, bindPort, err := config.AddrParts(config.BindAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid bind address: %s", err)
	}

	var advertiseIP string
	var advertisePort int
	if config.AdvertiseAddr != "" {
		advertiseIP, advertisePort, err = config.AddrParts(config.AdvertiseAddr)
		if err != nil {
			return nil, fmt.Errorf("invalid advertise address: %s", err)
		}
	}

	encryptKey, err := config.EncryptBytes()
	if err != nil {
		return nil, fmt.Errorf("invalid encryption key: %s", err)
	}

	serfConfig := serf.DefaultConfig()
	serfConfig.Init()
	serfConfig.Tags = a.config.Tags
	serfConfig.Tags["role"] = "kairos"
	serfConfig.Tags["dc"] = a.config.Datacenter
	serfConfig.Tags["region"] = a.config.Region
	serfConfig.Tags["version"] = Version

	if a.config.Server {
		serfConfig.Tags["server"] = strconv.FormatBool(a.config.Server)
	}

	if a.config.Bootstrap {
		serfConfig.Tags["bootstrap"] = "1"
	}
	if a.config.BootstrapExpect != 0 {
		serfConfig.Tags["expect"] = fmt.Sprintf("%d", a.config.BootstrapExpect)
	}

	switch config.Profile {
	case "lan":
		serfConfig.MemberlistConfig = memberlist.DefaultLANConfig()
	case "wan":
		serfConfig.MemberlistConfig = memberlist.DefaultWANConfig()
	case "local":
		serfConfig.MemberlistConfig = memberlist.DefaultLocalConfig()
	default:
		return nil, fmt.Errorf("unknown profile: %s", config.Profile)
	}

	serfConfig.MemberlistConfig.BindAddr = bindIP
	serfConfig.MemberlistConfig.BindPort = bindPort
	serfConfig.MemberlistConfig.AdvertiseAddr = advertiseIP
	serfConfig.MemberlistConfig.AdvertisePort = advertisePort
	serfConfig.MemberlistConfig.SecretKey = encryptKey
	serfConfig.NodeName = config.NodeName
	serfConfig.Tags = config.Tags
	serfConfig.CoalescePeriod = 3 * time.Second
	serfConfig.QuiescentPeriod = time.Second
	serfConfig.UserCoalescePeriod = 3 * time.Second
	serfConfig.UserQuiescentPeriod = time.Second
	serfConfig.ReconnectTimeout, err = time.ParseDuration(config.SerfReconnectTimeout)

	if err != nil {
		log.Fatalln(err)
	}

	// Create a channel to listen for events from Serf
	a.eventCh = make(chan serf.Event, 2048)
	serfConfig.EventCh = a.eventCh

	fmt.Println("agent: Agent agent starting")
	// Create serf first
	serf, err := serf.Create(serfConfig)
	if err != nil {
		log.Fatalln(err)
		return nil, err
	}
	return serf, nil
}

func (a *Agent) Start() error {
	// Normalize configured addresses
	if err := a.config.normalizeAddrs(); err != nil && !errors.Is(err, ErrResolvingHost) {
		return err
	}

	s, err := a.setupSerf()
	if err != nil {
		return fmt.Errorf("agent: Can not setup serf, %s", err)
	}
	a.serf = s

	// start retry join
	if len(a.config.RetryJoinLAN) > 0 {
		a.retryJoinLAN()
	} else {
		_, err := a.join(a.config.StartJoin, true)
		if err != nil {
			log.Fatalln("agent: Can not join")
		}
	}

	// Expose the node name
	expNode.Set(a.config.NodeName)

	//Use the value of "RPCPort" if AdvertiseRPCPort has not been set
	if a.config.AdvertiseRPCPort <= 0 {
		a.config.AdvertiseRPCPort = a.config.RPCPort
	}

	// Create a listener for RPC subsystem
	addr := a.bindRPCAddr()
	l, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalln(err)
	}
	a.listener = l

	if a.config.Server {
		a.StartServer()
	} else {
		opts := []grpc.ServerOption{}
		if a.TLSConfig != nil {
			tc := credentials.NewTLS(a.TLSConfig)
			opts = append(opts, grpc.Creds(tc))
		}

		grpcServer := grpc.NewServer(opts...)
		as := NewAgentServer(a)
		proto.RegisterAgentServer(grpcServer, as)
		go func() {
			if err := grpcServer.Serve(l); err != nil {
				log.Fatalln(err)
			}
		}()
	}

	if a.GRPCClient == nil {
		a.GRPCClient = NewGRPCClient(nil, a)
	}

	tags := a.serf.LocalMember().Tags
	tags["rpc_addr"] = a.advertiseRPCAddr() // Address that clients will use to RPC to servers
	tags["port"] = strconv.Itoa(a.config.AdvertiseRPCPort)

	if err := a.serf.SetTags(tags); err != nil {
		return fmt.Errorf("agent: Error setting tags: %w", err)
	}

	go a.eventLoop()
	a.ready = true

	return nil
}

// This function is called when a client request the RPCAddress
// of the current member.
// in marathon, it would return the host's IP and advertise RPC port
func (a *Agent) advertiseRPCAddr() string {
	bindIP := a.serf.LocalMember().Addr
	return net.JoinHostPort(bindIP.String(), strconv.Itoa(a.config.AdvertiseRPCPort))
}

// Join asks the Serf instance to join. See the Serf.Join function.
func (a *Agent) join(addrs []string, replay bool) (n int, err error) {
	n, err = a.serf.Join(addrs, !replay)
	if n > 0 {
		fmt.Printf("agent: joined: %d nodes", n)
	}
	if err != nil {
		fmt.Printf("agent: error joining: %v", err)
	}
	return
}

// JoinLAN is used to have Agent join the inner-DC pool
// The target address should be another node inside the DC
// listening on the Serf LAN address
func (a *Agent) JoinLAN(addrs []string) (int, error) {
	return a.serf.Join(addrs, true)
}

// RetryJoinCh is a channel that transports errors
// from the retry join process.
func (a *Agent) RetryJoinCh() <-chan error {
	return a.retryJoinCh
}

// UpdateTags updates the tag configuration for this agent
func (a *Agent) UpdateTags(tags map[string]string) {
	// Preserve reserved tags
	currentTags := a.serf.LocalMember().Tags
	for _, tagName := range []string{"role", "version", "server", "bootstrap", "expect", "port", "rpc_addr"} {
		if val, exists := currentTags[tagName]; exists {
			tags[tagName] = val
		}
	}
	tags["dc"] = a.config.Datacenter
	tags["region"] = a.config.Region

	// Set new collection of tags
	err := a.serf.SetTags(tags)
	if err != nil {
		fmt.Println("Setting tags unsuccessful")
	}
}

func (a *Agent) Stop() error {
	if a.config.Server {
		if a.sched.Started() {
			<-a.sched.Stop().Done()
		}

		_ = a.raft.Shutdown()

		if err := a.Store.Shutdown(); err != nil {
			return err
		}
	}

	if err := a.serf.Leave(); err != nil {
		return err
	}

	if err := a.serf.Shutdown(); err != nil {
		return err
	}

	return nil
}

func (a *Agent) GetRunningJobs() int {
	job := 0
	runningExecutions.Range(func(k, v interface{}) bool {
		job = job + 1
		return true
	})
	return job
}

// Get bind address for RPC
func (a *Agent) bindRPCAddr() string {
	bindIP, _, _ := a.config.AddrParts(a.config.BindAddr)
	return net.JoinHostPort(bindIP, strconv.Itoa(a.config.RPCPort))
}

// Listens to events from Serf and handle the event.
func (a *Agent) eventLoop() {
	serfShutdownCh := a.serf.ShutdownCh()
	fmt.Println("agent: Listen for events")
	for {
		select {
		case e := <-a.eventCh:
			fmt.Println("agent: Received even")
			// Log all member events
			if me, ok := e.(serf.MemberEvent); ok {
				if a.MemberEventHandler != nil {
					a.MemberEventHandler(e)
				}
				fmt.Println("agent: Event ", e.EventType())
				// serfEventHandler is used to handle events from the serf cluster
				switch e.EventType() {
				case serf.EventMemberJoin:
					a.nodeJoin(me)
					a.localMemberEvent(me)
				case serf.EventMemberLeave, serf.EventMemberFailed:
					a.nodeFailed(me)
					a.localMemberEvent(me)
				case serf.EventMemberReap:
					a.localMemberEvent(me)
				case serf.EventMemberUpdate, serf.EventUser, serf.EventQuery: // Ignore
				default:
					fmt.Println("agent: Unhandled serf event")
				}
			}
		case <-serfShutdownCh:
			fmt.Println("agent: Serf shutdown detected, quitting")
			return
		}
	}
}
