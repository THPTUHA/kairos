package agent

import (
	"net"

	"github.com/THPTUHA/kairos/server/plugin/proto"
	"google.golang.org/grpc"
)

// ClusterAgentGRPCServer defines the basics that a gRPC server should implement.
type ClusterAgentGRPCServer interface {
	proto.ClusterAgentServer
	Serve(net.Listener) error
}

// GRPCServer is the local implementation of the gRPC server interface.
type GRPCServer struct {
	proto.ClusterAgentServer
	agent *Agent
}

// Serve creates and start a new gRPC dkron server
func (grpcs *GRPCServer) Serve(lis net.Listener) error {
	grpcServer := grpc.NewServer()
	proto.RegisterClusterAgentServer(grpcServer, grpcs)
	as := NewAgentServer(grpcs.agent)
	proto.RegisterAgentServer(grpcServer, as)
	go grpcServer.Serve(lis)
	return nil
}

func NewGRPCServer(agent *Agent) ClusterAgentGRPCServer {
	return &GRPCServer{
		agent: agent,
	}
}
