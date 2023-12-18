package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"

	"github.com/THPTUHA/kairos/server/plugin/internal/plugin"
	hclog "github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

const GRPCServiceName = "plugin"

func DefaultGRPCServer(opts []grpc.ServerOption) *grpc.Server {
	return grpc.NewServer(opts...)
}

type GRPCServer struct {
	Plugins map[string]Plugin
	Server  func([]grpc.ServerOption) *grpc.Server

	DoneCh chan struct{}
	Stdout io.Reader
	Stderr io.Reader

	config      GRPCServerConfig
	server      *grpc.Server
	broker      *GRPCBroker
	stdioServer *grpcStdioServer

	logger hclog.Logger
}

func (s *GRPCServer) Init() error {
	var opts []grpc.ServerOption
	s.server = s.Server(opts)

	healthCheck := health.NewServer()
	healthCheck.SetServingStatus(
		GRPCServiceName, grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(s.server, healthCheck)

	reflection.Register(s.server)

	brokerServer := newGRPCBrokerServer()
	plugin.RegisterGRPCBrokerServer(s.server, brokerServer)
	s.broker = newGRPCBroker(brokerServer, unixSocketConfigFromEnv(), nil)
	go s.broker.Run()

	controllerServer := &grpcControllerServer{server: s}
	plugin.RegisterGRPCControllerServer(s.server, controllerServer)

	s.stdioServer = newGRPCStdioServer(s.logger, s.Stdout, s.Stderr)
	plugin.RegisterGRPCStdioServer(s.server, s.stdioServer)

	for k, raw := range s.Plugins {
		p, ok := raw.(GRPCPlugin)
		if !ok {
			return fmt.Errorf("%q is not a GRPC-compatible plugin", k)
		}

		if err := p.GRPCServer(s.broker, s.server); err != nil {
			return fmt.Errorf("error registering %q: %s", k, err)
		}
	}

	return nil
}

func (s *GRPCServer) Stop() {
	s.server.Stop()

	if s.broker != nil {
		s.broker.Close()
		s.broker = nil
	}
}

func (s *GRPCServer) GracefulStop() {
	s.server.GracefulStop()

	if s.broker != nil {
		s.broker.Close()
		s.broker = nil
	}
}

func (s *GRPCServer) Config() string {
	var buf bytes.Buffer

	if err := json.NewEncoder(&buf).Encode(s.config); err != nil {
		panic(err)
	}

	return buf.String()
}

func (s *GRPCServer) Serve(lis net.Listener) {
	defer close(s.DoneCh)
	err := s.server.Serve(lis)
	if err != nil {
		s.logger.Error("grpc server", "error", err)
	}
}

type GRPCServerConfig struct {
	StdoutAddr string `json:"stdout_addr"`
	StderrAddr string `json:"stderr_addr"`
}
