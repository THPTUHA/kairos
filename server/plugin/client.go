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
	"github.com/THPTUHA/kairos/server/plugin/internal/grpcmux"
	"github.com/THPTUHA/kairos/server/plugin/runner"
	hclog "github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
)

// If this is 1, then we've called CleanupClients. This can be used
// by plugin RPC implementations to change error behavior since you
// can expected network connection errors at this point. This should be
// read by using sync/atomic.
var Killed uint32 = 0

var managedClients = make([]*Client, 0, 5)
var managedClientsLock sync.Mutex

// Error types
var (
	// ErrProcessNotFound is returned when a client is instantiated to
	// reattach to an existing process and it isn't found.
	ErrProcessNotFound = cmdrunner.ErrProcessNotFound

	// ErrChecksumsDoNotMatch is returned when binary's checksum doesn't match
	// the one provided in the SecureConfig.
	ErrChecksumsDoNotMatch = errors.New("checksums did not match")

	// ErrSecureNoChecksum is returned when an empty checksum is provided to the
	// SecureConfig.
	ErrSecureConfigNoChecksum = errors.New("no checksum provided")

	// ErrSecureNoHash is returned when a nil Hash object is provided to the
	// SecureConfig.
	ErrSecureConfigNoHash = errors.New("no hash implementation provided")

	// ErrSecureConfigAndReattach is returned when both Reattach and
	// SecureConfig are set.
	ErrSecureConfigAndReattach = errors.New("only one of Reattach or SecureConfig can be set")

	// ErrGRPCBrokerMuxNotSupported is returned when the client requests
	// multiplexing over the gRPC broker, but the plugin does not support the
	// feature. In most cases, this should be resolvable by updating and
	// rebuilding the plugin, or restarting the plugin with
	// ClientConfig.GRPCBrokerMultiplex set to false.
	ErrGRPCBrokerMuxNotSupported = errors.New("client requested gRPC broker multiplexing but plugin does not support the feature")
)

// defaultPluginLogBufferSize is the default size of the buffer used to read from stderr for plugin log lines.
const defaultPluginLogBufferSize = 64 * 1024

// Client handles the lifecycle of a plugin application. It launches
// plugins, connects to them, dispenses interface implementations, and handles
// killing the process.
//
// Plugin hosts should use one Client for each plugin executable. To
// dispense a plugin type, use the `Client.Client` function, and then
// cal `Dispense`. This awkward API is mostly historical but is used to split
// the client that deals with subprocess management and the client that
// does RPC management.
//
// See NewClient and ClientConfig for using a Client.
type Client struct {
	config    *ClientConfig
	exited    bool
	l         sync.Mutex
	address   net.Addr
	runner    runner.AttachedRunner
	client    ClientProtocol
	protocol  Protocol
	logger    hclog.Logger
	doneCtx   context.Context
	ctxCancel context.CancelFunc

	// clientWaitGroup is used to manage the lifecycle of the plugin management
	// goroutines.
	clientWaitGroup sync.WaitGroup

	// stderrWaitGroup is used to prevent the command's Wait() function from
	// being called before we've finished reading from the stderr pipe.
	stderrWaitGroup sync.WaitGroup

	// processKilled is used for testing only, to flag when the process was
	// forcefully killed.
	processKilled bool

	unixSocketCfg UnixSocketConfig

	grpcMuxerOnce sync.Once
	grpcMuxer     *grpcmux.GRPCClientMuxer
}

// ID returns a unique ID for the running plugin. By default this is the process
// ID (pid), but it could take other forms if RunnerFunc was provided.
func (c *Client) ID() string {
	c.l.Lock()
	defer c.l.Unlock()

	if c.runner != nil {
		return c.runner.ID()
	}

	return ""
}

// ClientConfig is the configuration used to initialize a new
// plugin client. After being used to initialize a plugin client,
// that configuration must not be modified again.
type ClientConfig struct {
	// HandshakeConfig is the configuration that must match servers.
	HandshakeConfig

	// Plugins are the plugins that can be consumed.
	// The implied version of this PluginSet is the Handshake.ProtocolVersion.
	Plugins PluginSet

	// VersionedPlugins is a map of PluginSets for specific protocol versions.
	// These can be used to negotiate a compatible version between client and
	// server. If this is set, Handshake.ProtocolVersion is not required.
	VersionedPlugins map[int]PluginSet

	// One of the following must be set, but not both.
	//
	// Cmd is the unstarted subprocess for starting the plugin. If this is
	// set, then the Client starts the plugin process on its own and connects
	// to it.
	//
	// Reattach is configuration for reattaching to an existing plugin process
	// that is already running. This isn't common.
	Cmd      *exec.Cmd
	Reattach *ReattachConfig

	// RunnerFunc allows consumers to provide their own implementation of
	// runner.Runner and control the context within which a plugin is executed.
	// The cmd argument will have been copied from the config and populated with
	// environment variables that a go-plugin server expects to read such as
	// AutoMTLS certs and the magic cookie key.
	RunnerFunc func(l hclog.Logger, cmd *exec.Cmd, tmpDir string) (runner.Runner, error)

	// Managed represents if the client should be managed by the
	// plugin package or not. If true, then by calling CleanupClients,
	// it will automatically be cleaned up. Otherwise, the client
	// user is fully responsible for making sure to Kill all plugin
	// clients. By default the client is _not_ managed.
	Managed bool

	// The minimum and maximum port to use for communicating with
	// the subprocess. If not set, this defaults to 10,000 and 25,000
	// respectively.
	MinPort, MaxPort uint

	// StartTimeout is the timeout to wait for the plugin to say it
	// has started successfully.
	StartTimeout time.Duration

	// If non-nil, then the stderr of the client will be written to here
	// (as well as the log). This is the original os.Stderr of the subprocess.
	// This isn't the output of synced stderr.
	Stderr io.Writer

	// SyncStdout, SyncStderr can be set to override the
	// respective os.Std* values in the plugin. Care should be taken to
	// avoid races here. If these are nil, then this will be set to
	// ioutil.Discard.
	SyncStdout io.Writer
	SyncStderr io.Writer

	// AllowedProtocols is a list of allowed protocols. If this isn't set,
	// then only netrpc is allowed. This is so that older go-plugin systems
	// can show friendly errors if they see a plugin with an unknown
	// protocol.
	//
	// By setting this, you can cause an error immediately on plugin start
	// if an unsupported protocol is used with a good error message.
	//
	// If this isn't set at all (nil value), then only net/rpc is accepted.
	// This is done for legacy reasons. You must explicitly opt-in to
	// new protocols.
	AllowedProtocols []Protocol

	// Logger is the logger that the client will used. If none is provided,
	// it will default to hclog's default logger.
	Logger hclog.Logger

	// PluginLogBufferSize is the buffer size(bytes) to read from stderr for plugin log lines.
	// If this is 0, then the default of 64KB is used.
	PluginLogBufferSize int

	// GRPCDialOptions allows plugin users to pass custom grpc.DialOption
	// to create gRPC connections. This only affects plugins using the gRPC
	// protocol.
	GRPCDialOptions []grpc.DialOption

	// GRPCBrokerMultiplex turns on multiplexing for the gRPC broker. The gRPC
	// broker will multiplex all brokered gRPC servers over the plugin's original
	// listener socket instead of making a new listener for each server. The
	// go-plugin library currently only includes a Go implementation for the
	// server (i.e. plugin) side of gRPC broker multiplexing.
	//
	// Does not support reattaching.
	//
	// Multiplexed gRPC streams MUST be established sequentially, i.e. after
	// calling AcceptAndServe from one side, wait for the other side to Dial
	// before calling AcceptAndServe again.
	GRPCBrokerMultiplex bool

	// UnixSocketConfig configures additional options for any Unix sockets
	// that are created. Not normally required. Not supported on Windows.
	UnixSocketConfig *UnixSocketConfig
}

type UnixSocketConfig struct {
	// If set, go-plugin will change the owner of any Unix sockets created to
	// this group, and set them as group-writable. Can be a name or gid. The
	// client process must be a member of this group or chown will fail.
	Group string

	// TempDir specifies the base directory to use when creating a plugin-specific
	// temporary directory. It is expected to already exist and be writable. If
	// not set, defaults to the directory chosen by os.MkdirTemp.
	TempDir string

	// The directory to create Unix sockets in. Internally created and managed
	// by go-plugin and deleted when the plugin is killed. Will be created
	// inside TempDir if specified.
	socketDir string
}

// ReattachConfig is used to configure a client to reattach to an
// already-running plugin process. You can retrieve this information by
// calling ReattachConfig on Client.
type ReattachConfig struct {
	Protocol        Protocol
	ProtocolVersion int
	Addr            net.Addr
	Pid             int
}

// This makes sure all the managed subprocesses are killed and properly
// logged. This should be called before the parent process running the
// plugins exits.
//
// This must only be called _once_.
func CleanupClients() {
	// Set the killed to true so that we don't get unexpected panics
	atomic.StoreUint32(&Killed, 1)

	// Kill all the managed clients in parallel and use a WaitGroup
	// to wait for them all to finish up.
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

// NewClient creates a new plugin client which manages the lifecycle of an external
// plugin and gets the address for the RPC connection.
//
// The client must be cleaned up at some point by calling Kill(). If
// the client is a managed client (created with ClientConfig.Managed) you
// can just call CleanupClients at the end of your program and they will
// be properly cleaned.
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

// Tells whether or not the underlying process has exited.
func (c *Client) Exited() bool {
	c.l.Lock()
	defer c.l.Unlock()
	return c.exited
}

// killed is used in tests to check if a process failed to exit gracefully, and
// needed to be killed.
func (c *Client) killed() bool {
	c.l.Lock()
	defer c.l.Unlock()
	return c.processKilled
}

// End the executing subprocess (if it is running) and perform any cleanup
// tasks necessary such as capturing any remaining logs and so on.
//
// This method blocks until the process successfully exits.
//
// This method can safely be called multiple times.
func (c *Client) Kill() {
	// Grab a lock to read some private fields.
	c.l.Lock()
	runner := c.runner
	addr := c.address
	hostSocketDir := c.unixSocketCfg.socketDir
	c.l.Unlock()

	// If there is no runner or ID, there is nothing to kill.
	if runner == nil || runner.ID() == "" {
		return
	}

	defer func() {
		// Wait for the all client goroutines to finish.
		c.clientWaitGroup.Wait()

		if hostSocketDir != "" {
			os.RemoveAll(hostSocketDir)
		}

		// Make sure there is no reference to the old process after it has been
		// killed.
		c.l.Lock()
		c.runner = nil
		c.l.Unlock()
	}()

	// We need to check for address here. It is possible that the plugin
	// started (process != nil) but has no address (addr == nil) if the
	// plugin failed at startup. If we do have an address, we need to close
	// the plugin net connections.
	graceful := false
	if addr != nil {
		// Close the client to cleanly exit the process.
		client, err := c.Client()
		if err == nil {
			err = client.Close()

			// If there is no error, then we attempt to wait for a graceful
			// exit. If there was an error, we assume that graceful cleanup
			// won't happen and just force kill.
			graceful = err == nil
			if err != nil {
				// If there was an error just log it. We're going to force
				// kill in a moment anyways.
				c.logger.Warn("error closing client during Kill", "err", err)
			}
		} else {
			c.logger.Error("client", "error", err)
		}
	}

	// If we're attempting a graceful exit, then we wait for a short period
	// of time to allow that to happen. To wait for this we just wait on the
	// doneCh which would be closed if the process exits.
	if graceful {
		select {
		case <-c.doneCtx.Done():
			c.logger.Debug("plugin exited")
			return
		case <-time.After(2 * time.Second):
		}
	}

	// If graceful exiting failed, just kill it
	c.logger.Warn("plugin failed to exit gracefully")
	if err := runner.Kill(context.Background()); err != nil {
		c.logger.Debug("error killing plugin", "error", err)
	}

	c.l.Lock()
	c.processKilled = true
	c.l.Unlock()
}

// Start the underlying subprocess, communicating with it to negotiate
// a port for RPC connections, and returning the address to connect via RPC.
//
// This method is safe to call multiple times. Subsequent calls have no effect.
// Once a client has been started once, it cannot be started again, even if
// it was killed.
func (c *Client) Start() (addr net.Addr, err error) {
	c.l.Lock()
	defer c.l.Unlock()

	if c.address != nil {
		return c.address, nil
	}

	// If one of cmd or reattach isn't set, then it is an error. We wrap
	// this in a {} for scoping reasons, and hopeful that the escape
	// analysis will pop the stack here.
	{
		var mutuallyExclusiveOptions int
		if c.config.Cmd != nil {
			mutuallyExclusiveOptions += 1
		}
		if c.config.Reattach != nil {
			mutuallyExclusiveOptions += 1
		}
		if c.config.RunnerFunc != nil {
			mutuallyExclusiveOptions += 1
		}
		if mutuallyExclusiveOptions != 1 {
			return nil, fmt.Errorf("exactly one of Cmd, or Reattach, or RunnerFunc must be set")
		}

		if c.config.GRPCBrokerMultiplex && c.config.Reattach != nil {
			return nil, fmt.Errorf("gRPC broker multiplexing is not supported with Reattach config")
		}
	}

	if c.config.Reattach != nil {
		return c.reattach()
	}

	if c.config.VersionedPlugins == nil {
		c.config.VersionedPlugins = make(map[int]PluginSet)
	}

	// handle all plugins as versioned, using the handshake config as the default.
	version := int(c.config.ProtocolVersion)

	// Make sure we're not overwriting a real version 0. If ProtocolVersion was
	// non-zero, then we have to just assume the user made sure that
	// VersionedPlugins doesn't conflict.
	if _, ok := c.config.VersionedPlugins[version]; !ok && c.config.Plugins != nil {
		c.config.VersionedPlugins[version] = c.config.Plugins
	}

	var versionStrings []string
	for v := range c.config.VersionedPlugins {
		versionStrings = append(versionStrings, strconv.Itoa(v))
	}

	env := []string{
		fmt.Sprintf("%s=%s", c.config.MagicCookieKey, c.config.MagicCookieValue),
		fmt.Sprintf("PLUGIN_MIN_PORT=%d", c.config.MinPort),
		fmt.Sprintf("PLUGIN_MAX_PORT=%d", c.config.MaxPort),
		fmt.Sprintf("PLUGIN_PROTOCOL_VERSIONS=%s", strings.Join(versionStrings, ",")),
	}
	fmt.Printf("MULTIPLEX--- %+v\n", c.config)

	cmd := c.config.Cmd
	if cmd == nil {
		// It's only possible to get here if RunnerFunc is non-nil, but we'll
		// still use cmd as a spec to populate metadata for the external
		// implementation to consume.
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
	switch {
	case c.config.RunnerFunc != nil:
		c.unixSocketCfg.socketDir, err = os.MkdirTemp(c.unixSocketCfg.TempDir, "plugin-dir")
		if err != nil {
			return nil, err
		}
		// os.MkdirTemp creates folders with 0o700, so if we have a group
		// configured we need to make it group-writable.
		if c.unixSocketCfg.Group != "" {
			err = setGroupWritable(c.unixSocketCfg.socketDir, c.unixSocketCfg.Group, 0o770)
			if err != nil {
				return nil, err
			}
		}
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", EnvUnixSocketDir, c.unixSocketCfg.socketDir))
		c.logger.Trace("created temporary directory for unix sockets", "dir", c.unixSocketCfg.socketDir)

		runner, err = c.config.RunnerFunc(c.logger, cmd, c.unixSocketCfg.socketDir)
		if err != nil {
			return nil, err
		}
	default:
		runner, err = cmdrunner.NewCmdRunner(c.logger, cmd)
		if err != nil {
			return nil, err
		}

	}

	c.runner = runner
	startCtx, startCtxCancel := context.WithTimeout(context.Background(), c.config.StartTimeout)
	defer startCtxCancel()
	err = runner.Start(startCtx)
	if err != nil {
		return nil, err
	}

	// Make sure the command is properly cleaned up if there is an error
	defer func() {
		rErr := recover()

		if err != nil || rErr != nil {
			runner.Kill(context.Background())
		}

		if rErr != nil {
			panic(rErr)
		}
	}()

	// Create a context for when we kill
	c.doneCtx, c.ctxCancel = context.WithCancel(context.Background())

	// Start goroutine that logs the stderr
	c.clientWaitGroup.Add(1)
	c.stderrWaitGroup.Add(1)
	// logStderr calls Done()
	go c.logStderr(runner.Name(), runner.Stderr())

	c.clientWaitGroup.Add(1)
	go func() {
		// ensure the context is cancelled when we're done
		defer c.ctxCancel()

		defer c.clientWaitGroup.Done()

		// wait to finish reading from stderr since the stderr pipe reader
		// will be closed by the subsequent call to cmd.Wait().
		c.stderrWaitGroup.Wait()

		// Wait for the command to end.
		err := runner.Wait(context.Background())
		if err != nil {
			c.logger.Error("plugin process exited", "plugin", runner.Name(), "id", runner.ID(), "error", err.Error())
		} else {
			// Log and make sure to flush the logs right away
			c.logger.Info("plugin process exited", "plugin", runner.Name(), "id", runner.ID())
		}

		os.Stderr.Sync()

		// Set that we exited, which takes a lock
		c.l.Lock()
		defer c.l.Unlock()
		c.exited = true
	}()

	// Start a goroutine that is going to be reading the lines
	// out of stdout
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

	// Make sure after we exit we read the lines from stdout forever
	// so they don't block since it is a pipe.
	// The scanner goroutine above will close this, but track it with a wait
	// group for completeness.
	c.clientWaitGroup.Add(1)
	defer func() {
		go func() {
			defer c.clientWaitGroup.Done()
			for range linesCh {
			}
		}()
	}()

	// Some channels for the next step
	timeout := time.After(c.config.StartTimeout)

	// Start looking for the address
	c.logger.Debug("waiting for RPC address", "plugin", runner.Name())
	select {
	case <-timeout:
		err = errors.New("timeout while waiting for plugin to start")
	case <-c.doneCtx.Done():
		err = errors.New("plugin exited before we could connect")
	case line, ok := <-linesCh:
		// Trim the line and split by "|" in order to get the parts of
		// the output.
		line = strings.TrimSpace(line)
		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			errText := fmt.Sprintf("Unrecognized remote plugin message: %s", line)
			if !ok {
				errText += "\n" + "Failed to read any lines from plugin's stdout"
			}
			additionalNotes := runner.Diagnose(context.Background())
			if additionalNotes != "" {
				errText += "\n" + additionalNotes
			}
			err = errors.New(errText)
			return
		}

		// Test the API version
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

func (c *Client) reattach() (net.Addr, error) {
	reattachFunc := cmdrunner.ReattachFunc(c.config.Reattach.Pid, c.config.Reattach.Addr)

	r, err := reattachFunc()
	if err != nil {
		return nil, err
	}

	// Create a context for when we kill
	c.doneCtx, c.ctxCancel = context.WithCancel(context.Background())

	c.clientWaitGroup.Add(1)
	// Goroutine to mark exit status
	go func(r runner.AttachedRunner) {
		defer c.clientWaitGroup.Done()

		// ensure the context is cancelled when we're done
		defer c.ctxCancel()

		// Wait for the process to die
		r.Wait(context.Background())

		// Log so we can see it
		c.logger.Debug("reattached plugin process exited")

		// Mark it
		c.l.Lock()
		defer c.l.Unlock()
		c.exited = true
	}(r)

	// Set the address and protocol
	c.address = c.config.Reattach.Addr
	c.protocol = c.config.Reattach.Protocol
	c.runner = r

	return c.address, nil
}

// checkProtoVersion returns the negotiated version and PluginSet.
// This returns an error if the server returned an incompatible protocol
// version, or an invalid handshake response.
func (c *Client) checkProtoVersion(protoVersion string) (int, PluginSet, error) {
	serverVersion, err := strconv.Atoi(protoVersion)
	if err != nil {
		return 0, nil, fmt.Errorf("Error parsing protocol version %q: %s", protoVersion, err)
	}

	// record these for the error message
	var clientVersions []int

	// all versions, including the legacy ProtocolVersion have been added to
	// the versions set
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

// ReattachConfig returns the information that must be provided to NewClient
// to reattach to the plugin process that this client started. This is
// useful for plugins that detach from their parent process.
//
// If this returns nil then the process hasn't been started yet. Please
// call Start or Client before calling this.
//
// Clients who specified a RunnerFunc will need to populate their own
// ReattachFunc in the returned ReattachConfig before it can be used.
func (c *Client) ReattachConfig() *ReattachConfig {
	c.l.Lock()
	defer c.l.Unlock()

	if c.address == nil {
		return nil
	}

	if c.config.Cmd != nil && c.config.Cmd.Process == nil {
		return nil
	}

	// If we connected via reattach, just return the information as-is
	if c.config.Reattach != nil {
		return c.config.Reattach
	}

	reattach := &ReattachConfig{
		Protocol: c.protocol,
		Addr:     c.address,
	}

	if c.config.Cmd != nil && c.config.Cmd.Process != nil {
		reattach.Pid = c.config.Cmd.Process.Pid
	}

	return reattach
}

// Protocol returns the protocol of server on the remote end. This will
// start the plugin process if it isn't already started. Errors from
// starting the plugin are surpressed and ProtocolInvalid is returned. It
// is recommended you call Start explicitly before calling Protocol to ensure
// no errors occur.
func (c *Client) Protocol() Protocol {
	_, err := c.Start()
	if err != nil {
		return ProtocolInvalid
	}

	return c.protocol
}

func netAddrDialer(addr net.Addr) func(string, time.Duration) (net.Conn, error) {
	return func(_ string, _ time.Duration) (net.Conn, error) {
		// Connect to the client
		conn, err := net.Dial(addr.Network(), addr.String())
		if err != nil {
			return nil, err
		}
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			// Make sure to set keep alive so that the connection doesn't die
			tcpConn.SetKeepAlive(true)
		}

		return conn, nil
	}
}

// dialer is compatible with grpc.WithDialer and creates the connection
// to the plugin.
func (c *Client) dialer(_ string, timeout time.Duration) (net.Conn, error) {
	muxer, err := c.getGRPCMuxer(c.address)
	if err != nil {
		return nil, err
	}

	var conn net.Conn
	if muxer.Enabled() {
		conn, err = muxer.Dial()
		if err != nil {
			return nil, err
		}
	} else {
		conn, err = netAddrDialer(c.address)("", timeout)
		if err != nil {
			return nil, err
		}
	}

	return conn, nil
}

func (c *Client) getGRPCMuxer(addr net.Addr) (*grpcmux.GRPCClientMuxer, error) {
	if c.protocol != ProtocolGRPC || !c.config.GRPCBrokerMultiplex {
		return nil, nil
	}

	var err error
	c.grpcMuxerOnce.Do(func() {
		c.grpcMuxer, err = grpcmux.NewGRPCClientMuxer(c.logger, addr)
	})
	if err != nil {
		return nil, err
	}

	return c.grpcMuxer, nil
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
