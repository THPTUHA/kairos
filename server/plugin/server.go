package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"os/user"
	"runtime"
	"sort"
	"strconv"
	"strings"

	hclog "github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
)

type HandshakeConfig struct {
	ProtocolVersion uint
}

type PluginSet map[string]Plugin

type ServeConfig struct {
	HandshakeConfig
	Plugins          PluginSet
	VersionedPlugins map[int]PluginSet
	GRPCServer       func([]grpc.ServerOption) *grpc.Server
	Logger           hclog.Logger
}

func unixSocketConfigFromEnv() UnixSocketConfig {
	return UnixSocketConfig{
		Group:     os.Getenv(EnvUnixSocketGroup),
		socketDir: os.Getenv(EnvUnixSocketDir),
	}
}

func protocolVersion(opts *ServeConfig) (int, Protocol, PluginSet) {
	protoVersion := int(opts.ProtocolVersion)
	pluginSet := opts.Plugins
	protoType := ProtocolGRPC
	var clientVersions []int
	if vs := os.Getenv("PLUGIN_PROTOCOL_VERSIONS"); vs != "" {
		for _, s := range strings.Split(vs, ",") {
			v, err := strconv.Atoi(s)
			if err != nil {
				fmt.Fprintf(os.Stderr, "server sent invalid plugin version %q", s)
				continue
			}
			clientVersions = append(clientVersions, v)
		}
	}

	sort.Sort(sort.Reverse(sort.IntSlice(clientVersions)))
	if opts.VersionedPlugins == nil {
		opts.VersionedPlugins = make(map[int]PluginSet)
	}

	if pluginSet != nil {
		opts.VersionedPlugins[protoVersion] = pluginSet
	}

	var versions []int
	for v := range opts.VersionedPlugins {
		versions = append(versions, v)
	}

	sort.Sort(sort.Reverse(sort.IntSlice(versions)))

	for _, version := range versions {
		protoVersion = version
		pluginSet = opts.VersionedPlugins[version]

		if opts.GRPCServer != nil {
			break
		}

		for _, clientVersion := range clientVersions {
			if clientVersion == protoVersion {
				return protoVersion, protoType, pluginSet
			}
		}
	}

	return protoVersion, protoType, pluginSet
}

func Serve(opts *ServeConfig) {
	exitCode := -1
	defer func() {
		if exitCode >= 0 {
			os.Exit(exitCode)
		}

	}()

	protoVersion, protoType, pluginSet := protocolVersion(opts)

	logger := opts.Logger
	if logger == nil {
		logger = hclog.New(&hclog.LoggerOptions{
			Level:      hclog.Trace,
			Output:     os.Stderr,
			JSONFormat: true,
		})
	}

	listener, err := serverListener(unixSocketConfigFromEnv())
	if err != nil {
		logger.Error("plugin init error", "error", err)
		return
	}

	defer func() {
		listener.Close()
	}()

	doneCh := make(chan struct{})

	var stdout_r, stderr_r io.Reader
	stdout_r, stdout_w, err := os.Pipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error preparing plugin: %s\n", err)
		os.Exit(1)
	}
	stderr_r, stderr_w, err := os.Pipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error preparing plugin: %s\n", err)
		os.Exit(1)
	}

	var server ServerProtocol
	switch protoType {
	case ProtocolGRPC:
		server = &GRPCServer{
			Plugins: pluginSet,
			Server:  opts.GRPCServer,
			Stdout:  stdout_r,
			Stderr:  stderr_r,
			DoneCh:  doneCh,
			logger:  logger,
		}

	default:
		panic("unknown server protocol: " + protoType)
	}

	if err := server.Init(); err != nil {
		logger.Error("protocol init", "error", err)
		return
	}

	logger.Debug("plugin address", "network", listener.Addr().Network(), "address", listener.Addr().String())

	const grpcBrokerMultiplexingSupported = true
	protocolLine := fmt.Sprintf("%d|%s|%s|%s",
		protoVersion,
		listener.Addr().Network(),
		listener.Addr().String(),
		protoType,
	)

	fmt.Printf("%s\n", protocolLine)
	os.Stdout.Sync()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	go func() {
		count := 0
		for {
			<-ch
			count++
			logger.Trace("plugin received interrupt signal, ignoring", "count", count)
		}
	}()
	defer func(out, err *os.File) {
		os.Stdout = out
		os.Stderr = err
	}(os.Stdout, os.Stderr)
	os.Stdout = stdout_w
	os.Stderr = stderr_w

	go server.Serve(listener)

	ctx := context.Background()
	select {
	case <-ctx.Done():
		listener.Close()
		if s, ok := server.(*GRPCServer); ok {
			s.Stop()
		}

		<-doneCh

	case <-doneCh:
	}
}

func serverListener(unixSocketCfg UnixSocketConfig) (net.Listener, error) {
	if runtime.GOOS == "windows" {
		return serverListener_tcp()
	}

	return serverListener_unix(unixSocketCfg)
}

func serverListener_tcp() (net.Listener, error) {
	envMinPort := os.Getenv("PLUGIN_MIN_PORT")
	envMaxPort := os.Getenv("PLUGIN_MAX_PORT")

	var minPort, maxPort int64
	var err error

	switch {
	case len(envMinPort) == 0:
		minPort = 0
	default:
		minPort, err = strconv.ParseInt(envMinPort, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("Couldn't get value from PLUGIN_MIN_PORT: %v", err)
		}
	}

	switch {
	case len(envMaxPort) == 0:
		maxPort = 0
	default:
		maxPort, err = strconv.ParseInt(envMaxPort, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("Couldn't get value from PLUGIN_MAX_PORT: %v", err)
		}
	}

	if minPort > maxPort {
		return nil, fmt.Errorf("PLUGIN_MIN_PORT value of %d is greater than PLUGIN_MAX_PORT value of %d", minPort, maxPort)
	}

	for port := minPort; port <= maxPort; port++ {
		address := fmt.Sprintf("127.0.0.1:%d", port)
		listener, err := net.Listen("tcp", address)
		if err == nil {
			return listener, nil
		}
	}

	return nil, errors.New("Couldn't bind plugin TCP listener")
}

func serverListener_unix(unixSocketCfg UnixSocketConfig) (net.Listener, error) {
	tf, err := os.CreateTemp(unixSocketCfg.socketDir, "plugin")
	if err != nil {
		return nil, err
	}
	path := tf.Name()

	if err := tf.Close(); err != nil {
		return nil, err
	}
	if err := os.Remove(path); err != nil {
		return nil, err
	}

	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}

	if unixSocketCfg.Group != "" {
		err = setGroupWritable(path, unixSocketCfg.Group, 0o660)
		if err != nil {
			return nil, err
		}
	}

	return newDeleteFileListener(l, path), nil
}

func setGroupWritable(path, groupString string, mode os.FileMode) error {
	groupID, err := strconv.Atoi(groupString)
	if err != nil {
		group, err := user.LookupGroup(groupString)
		if err != nil {
			return fmt.Errorf("failed to find gid from %q: %w", groupString, err)
		}
		groupID, err = strconv.Atoi(group.Gid)
		if err != nil {
			return fmt.Errorf("failed to parse %q group's gid as an integer: %w", groupString, err)
		}
	}

	err = os.Chown(path, -1, groupID)
	if err != nil {
		return err
	}

	err = os.Chmod(path, mode)
	if err != nil {
		return err
	}

	return nil
}

type rmListener struct {
	net.Listener
	close func() error
}

func newDeleteFileListener(ln net.Listener, path string) *rmListener {
	return &rmListener{
		Listener: ln,
		close: func() error {
			return os.Remove(path)
		},
	}
}

func (l *rmListener) Close() error {
	if err := l.Listener.Close(); err != nil {
		return err
	}

	return l.close()
}
