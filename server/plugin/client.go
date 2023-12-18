package plugin

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/THPTUHA/kairos/server/plugin/internal/cmdrunner"
	"github.com/THPTUHA/kairos/server/plugin/runner"
	hclog "github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
)

var Killed uint32 = 0

var managedClients = make([]*Client, 0, 5)
var managedClientsLock sync.Mutex

var (
	ErrProcessNotFound           = cmdrunner.ErrProcessNotFound
	ErrChecksumsDoNotMatch       = errors.New("checksums did not match")
	ErrSecureConfigNoChecksum    = errors.New("no checksum provided")
	ErrSecureConfigNoHash        = errors.New("no hash implementation provided")
	ErrSecureConfigAndReattach   = errors.New("only one of Reattach or SecureConfig can be set")
	ErrGRPCBrokerMuxNotSupported = errors.New("client requested gRPC broker multiplexing but plugin does not support the feature")
)

const defaultPluginLogBufferSize = 64 * 1024

type Client struct {
	config          *ClientConfig
	exited          bool
	l               sync.Mutex
	address         net.Addr
	runner          runner.AttachedRunner
	client          ClientProtocol
	protocol        Protocol
	logger          hclog.Logger
	doneCtx         context.Context
	ctxCancel       context.CancelFunc
	clientWaitGroup sync.WaitGroup
	stderrWaitGroup sync.WaitGroup
	processKilled   bool

	unixSocketCfg UnixSocketConfig
}

func (c *Client) ID() string {
	c.l.Lock()
	defer c.l.Unlock()

	if c.runner != nil {
		return c.runner.ID()
	}

	return ""
}

type ClientConfig struct {
	HandshakeConfig
	Plugins             PluginSet
	VersionedPlugins    map[int]PluginSet
	Cmd                 *exec.Cmd
	Managed             bool
	MinPort, MaxPort    uint
	StartTimeout        time.Duration
	Stderr              io.Writer
	SyncStdout          io.Writer
	SyncStderr          io.Writer
	AllowedProtocols    []Protocol
	Logger              hclog.Logger
	PluginLogBufferSize int

	GRPCDialOptions     []grpc.DialOption
	GRPCBrokerMultiplex bool
	UnixSocketConfig    *UnixSocketConfig
}

type UnixSocketConfig struct {
	Group     string
	TempDir   string
	socketDir string
}

func CleanupClients() {
	atomic.StoreUint32(&Killed, 1)
	var wg sync.WaitGroup
	managedClientsLock.Lock()
	for _, client := range managedClients {
		wg.Add(1)

		go func(client *Client) {
			client.Kill()
			wg.Done()
		}(client)
	}
	managedClientsLock.Unlock()

	wg.Wait()
}

func NewClient(config *ClientConfig) (c *Client) {
	if config.MinPort == 0 && config.MaxPort == 0 {
		config.MinPort = 10000
		config.MaxPort = 25000
	}

	if config.StartTimeout == 0 {
		config.StartTimeout = 1 * time.Minute
	}

	if config.Stderr == nil {
		config.Stderr = ioutil.Discard
	}

	if config.SyncStdout == nil {
		config.SyncStdout = io.Discard
	}
	if config.SyncStderr == nil {
		config.SyncStderr = io.Discard
	}

	if config.AllowedProtocols == nil {
		config.AllowedProtocols = []Protocol{}
	}

	if config.Logger == nil {
		config.Logger = hclog.New(&hclog.LoggerOptions{
			Output: hclog.DefaultOutput,
			Level:  hclog.Trace,
			Name:   "plugin",
		})
	}

	if config.PluginLogBufferSize == 0 {
		config.PluginLogBufferSize = defaultPluginLogBufferSize
	}

	c = &Client{
		config: config,
		logger: config.Logger,
	}
	if config.Managed {
		managedClientsLock.Lock()
		managedClients = append(managedClients, c)
		managedClientsLock.Unlock()
	}

	return
}

func (c *Client) Client() (ClientProtocol, error) {
	_, err := c.Start()
	if err != nil {
		return nil, err
	}

	c.l.Lock()
	defer c.l.Unlock()

	if c.client != nil {
		return c.client, nil
	}

	switch c.protocol {
	case ProtocolGRPC:
		c.client, err = newGRPCClient(c.doneCtx, c)

	default:
		return nil, fmt.Errorf("unknown server protocol: %s", c.protocol)
	}

	if err != nil {
		c.client = nil
		return nil, err
	}

	return c.client, nil
}

func (c *Client) Exited() bool {
	c.l.Lock()
	defer c.l.Unlock()
	return c.exited
}

func (c *Client) killed() bool {
	c.l.Lock()
	defer c.l.Unlock()
	return c.processKilled
}

func (c *Client) Kill() {
	c.l.Lock()
	runner := c.runner
	addr := c.address
	hostSocketDir := c.unixSocketCfg.socketDir
	c.l.Unlock()

	if runner == nil || runner.ID() == "" {
		return
	}

	defer func() {
		c.clientWaitGroup.Wait()

		if hostSocketDir != "" {
			os.RemoveAll(hostSocketDir)
		}
		c.l.Lock()
		c.runner = nil
		c.l.Unlock()
	}()

	graceful := false
	if addr != nil {
		client, err := c.Client()
		if err == nil {
			err = client.Close()
			graceful = err == nil
			if err != nil {
				c.logger.Warn("error closing client during Kill", "err", err)
			}
		} else {
			c.logger.Error("client", "error", err)
		}
	}
	if graceful {
		select {
		case <-c.doneCtx.Done():
			c.logger.Debug("plugin exited")
			return
		case <-time.After(2 * time.Second):
		}
	}

	c.logger.Warn("plugin failed to exit gracefully")
	if err := runner.Kill(context.Background()); err != nil {
		c.logger.Debug("error killing plugin", "error", err)
	}

	c.l.Lock()
	c.processKilled = true
	c.l.Unlock()
}

func (c *Client) Start() (addr net.Addr, err error) {
	c.l.Lock()
	defer c.l.Unlock()

	if c.address != nil {
		return c.address, nil
	}

	{
		var mutuallyExclusiveOptions int
		if c.config.Cmd != nil {
			mutuallyExclusiveOptions += 1
		}
		if mutuallyExclusiveOptions != 1 {
			return nil, fmt.Errorf("exactly one of Cmd, or Reattach, or RunnerFunc must be set")
		}

	}

	if c.config.VersionedPlugins == nil {
		c.config.VersionedPlugins = make(map[int]PluginSet)
	}

	version := int(c.config.ProtocolVersion)

	if _, ok := c.config.VersionedPlugins[version]; !ok && c.config.Plugins != nil {
		c.config.VersionedPlugins[version] = c.config.Plugins
	}

	var versionStrings []string
	for v := range c.config.VersionedPlugins {
		versionStrings = append(versionStrings, strconv.Itoa(v))
	}

	env := []string{
		fmt.Sprintf("PLUGIN_MIN_PORT=%d", c.config.MinPort),
		fmt.Sprintf("PLUGIN_MAX_PORT=%d", c.config.MaxPort),
		fmt.Sprintf("PLUGIN_PROTOCOL_VERSIONS=%s", strings.Join(versionStrings, ",")),
	}

	cmd := c.config.Cmd
	if cmd == nil {
		cmd = exec.Command("")
	}
	cmd.Env = append(cmd.Env, env...)
	cmd.Stdin = os.Stdin

	if c.config.UnixSocketConfig != nil {
		c.unixSocketCfg = *c.config.UnixSocketConfig
	}

	if c.unixSocketCfg.Group != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", EnvUnixSocketGroup, c.unixSocketCfg.Group))
	}

	var runner runner.Runner
	runner, err = cmdrunner.NewCmdRunner(c.logger, cmd)
	if err != nil {
		return nil, err
	}

	c.runner = runner
	startCtx, startCtxCancel := context.WithTimeout(context.Background(), c.config.StartTimeout)
	defer startCtxCancel()
	err = runner.Start(startCtx)
	if err != nil {
		return nil, err
	}

	defer func() {
		rErr := recover()

		if err != nil || rErr != nil {
			runner.Kill(context.Background())
		}

		if rErr != nil {
			panic(rErr)
		}
	}()

	c.doneCtx, c.ctxCancel = context.WithCancel(context.Background())
	c.clientWaitGroup.Add(1)
	c.stderrWaitGroup.Add(1)
	go c.logStderr(runner.Name(), runner.Stderr())

	c.clientWaitGroup.Add(1)
	go func() {
		defer c.ctxCancel()

		defer c.clientWaitGroup.Done()
		c.stderrWaitGroup.Wait()

		err := runner.Wait(context.Background())
		if err != nil {
			c.logger.Error("plugin process exited", "plugin", runner.Name(), "id", runner.ID(), "error", err.Error())
		} else {
			c.logger.Info("plugin process exited", "plugin", runner.Name(), "id", runner.ID())
		}

		os.Stderr.Sync()

		c.l.Lock()
		defer c.l.Unlock()
		c.exited = true
	}()

	linesCh := make(chan string)
	c.clientWaitGroup.Add(1)
	go func() {
		defer c.clientWaitGroup.Done()
		defer close(linesCh)

		scanner := bufio.NewScanner(runner.Stdout())
		for scanner.Scan() {
			linesCh <- scanner.Text()
		}
		if scanner.Err() != nil {
			c.logger.Error("error encountered while scanning stdout", "error", scanner.Err())
		}
	}()

	c.clientWaitGroup.Add(1)
	defer func() {
		go func() {
			defer c.clientWaitGroup.Done()
			for range linesCh {
			}
		}()
	}()

	timeout := time.After(c.config.StartTimeout)

	c.logger.Debug("waiting for RPC address", "plugin", runner.Name())
	select {
	case <-timeout:
		err = errors.New("timeout while waiting for plugin to start")
	case <-c.doneCtx.Done():
		err = errors.New("plugin exited before we could connect")
	case line, ok := <-linesCh:
		fmt.Println("LINE----", line)
		line = strings.TrimSpace(line)
		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			errText := fmt.Sprintf("Unrecognized remote plugin message: %s", line)
			if !ok {
				errText += "\n" + "Failed to read any lines from plugin's stdout"
			}
			err = errors.New(errText)
			return
		}

		version, pluginSet, err := c.checkProtoVersion(parts[0])
		if err != nil {
			return addr, err
		}

		c.config.Plugins = pluginSet
		c.logger.Debug("using plugin", "version", version)

		network, address, err := runner.PluginToHost(parts[1], parts[2])
		if err != nil {
			return addr, err
		}

		switch network {
		case "tcp":
			addr, err = net.ResolveTCPAddr("tcp", address)
		case "unix":
			addr, err = net.ResolveUnixAddr("unix", address)
		default:
			err = fmt.Errorf("Unknown address type: %s", address)
		}

		if len(parts) >= 4 {
			c.protocol = Protocol(parts[3])
		}

		found := false
		for _, p := range c.config.AllowedProtocols {
			if p == c.protocol {
				found = true
				break
			}
		}
		if !found {
			err = fmt.Errorf("Unsupported plugin protocol %q. Supported: %v",
				c.protocol, c.config.AllowedProtocols)
			return addr, err
		}

	}

	c.address = addr
	return
}

func (c *Client) checkProtoVersion(protoVersion string) (int, PluginSet, error) {
	serverVersion, err := strconv.Atoi(protoVersion)
	if err != nil {
		return 0, nil, fmt.Errorf("Error parsing protocol version %q: %s", protoVersion, err)
	}

	var clientVersions []int
	for version, plugins := range c.config.VersionedPlugins {
		clientVersions = append(clientVersions, version)

		if serverVersion != version {
			continue
		}
		return version, plugins, nil
	}

	return 0, nil, fmt.Errorf("Incompatible API version with plugin. "+
		"Plugin version: %d, Client versions: %d", serverVersion, clientVersions)
}

func (c *Client) Protocol() Protocol {
	_, err := c.Start()
	if err != nil {
		return ProtocolInvalid
	}

	return c.protocol
}

func netAddrDialer(addr net.Addr) func(string, time.Duration) (net.Conn, error) {
	return func(_ string, _ time.Duration) (net.Conn, error) {
		conn, err := net.Dial(addr.Network(), addr.String())
		if err != nil {
			return nil, err
		}
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			tcpConn.SetKeepAlive(true)
		}

		return conn, nil
	}
}

func (c *Client) dialer(_ string, timeout time.Duration) (net.Conn, error) {
	conn, err := netAddrDialer(c.address)("", timeout)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (c *Client) logStderr(name string, r io.Reader) {
	defer c.clientWaitGroup.Done()
	defer c.stderrWaitGroup.Done()
	l := c.logger.Named(filepath.Base(name))

	reader := bufio.NewReaderSize(r, c.config.PluginLogBufferSize)
	continuation := false

	for {
		line, isPrefix, err := reader.ReadLine()
		switch {
		case err == io.EOF:
			return
		case err != nil:
			l.Error("reading plugin stderr", "error", err)
			return
		}

		c.config.Stderr.Write(line)

		if isPrefix || continuation {
			l.Debug(string(line))
			if !isPrefix {
				c.config.Stderr.Write([]byte{'\n'})
			}

			continuation = isPrefix
			continue
		}

		c.config.Stderr.Write([]byte{'\n'})

		entry, err := parseJSON(line)
		if err != nil {
			switch line := string(line); {
			case strings.HasPrefix(line, "[TRACE]"):
				l.Trace(line)
			case strings.HasPrefix(line, "[DEBUG]"):
				l.Debug(line)
			case strings.HasPrefix(line, "[INFO]"):
				l.Info(line)
			case strings.HasPrefix(line, "[WARN]"):
				l.Warn(line)
			case strings.HasPrefix(line, "[ERROR]"):
				l.Error(line)
			default:
				l.Debug(line)
			}
		} else {
			out := flattenKVPairs(entry.KVPairs)

			out = append(out, "timestamp", entry.Timestamp.Format(hclog.TimeFormat))
			switch hclog.LevelFromString(entry.Level) {
			case hclog.Trace:
				l.Trace(entry.Message, out...)
			case hclog.Debug:
				l.Debug(entry.Message, out...)
			case hclog.Info:
				l.Info(entry.Message, out...)
			case hclog.Warn:
				l.Warn(entry.Message, out...)
			case hclog.Error:
				l.Error(entry.Message, out...)
			default:
				l.Debug(string(line))
			}
		}
	}
}
