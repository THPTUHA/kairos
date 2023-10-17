package agent

import (
	"crypto/tls"
	"errors"
	"expvar"
	"fmt"
	"log"
	"net"
	"strconv"
	"time"

	"github.com/THPTUHA/kairos/server/plugin"
	"github.com/THPTUHA/kairos/server/plugin/proto"
	"github.com/hashicorp/memberlist"
	"github.com/hashicorp/raft"
	"github.com/hashicorp/serf/serf"
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
	GRPCClient KairosGRPCClient

	// Store interface to set the storage engine
	Store Storage
}

var (
	expNode = expvar.NewString("node")
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

// TODO
func (a *Agent) StartServer() {
	if a.Store == nil {
		s, err := NewStore()
		if err != nil {
			log.Fatalln("dkron: Error initializing store")
		}
		a.Store = s
	}

	a.sched = NewScheduler()
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
// TODO
func (a *Agent) UpdateTags(tags map[string]string) {

}

// TODO
func (a *Agent) Stop() error {
	return nil
}

// TODO
func (a *Agent) GetRunningJobs() int {
	return 0
}

// Get bind address for RPC
func (a *Agent) bindRPCAddr() string {
	bindIP, _, _ := a.config.AddrParts(a.config.BindAddr)
	return net.JoinHostPort(bindIP, strconv.Itoa(a.config.RPCPort))
}

// Listens to events from Serf and handle the event.
func (a *Agent) eventLoop() {

}
