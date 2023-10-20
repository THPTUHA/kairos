package api

import (
	"github.com/centrifugal/centrifuge"
	"google.golang.org/grpc"
)

// GRPCAPIServiceConfig for GRPC API Service.
type GRPCAPIServiceConfig struct {
	UseOpenTelemetry      bool
	UseTransportErrorMode bool
}

// RegisterGRPCServerAPI registers GRPC API service in provided GRPC server.
func RegisterGRPCServerAPI(n *centrifuge.Node, apiExecutor *Executor, server *grpc.Server, config GRPCAPIServiceConfig) error {
	RegisterCentrifugoApiServer(server, newGRPCAPIService(n, apiExecutor, config))
	return nil
}
