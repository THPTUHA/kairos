package agent

import "google.golang.org/grpc"

// AgentGRPCClient defines the interface that any gRPC client for
// kairos should implement.
type AgentGRPCClient interface {
}

// GRPCClient is the local implementation of the AgentGRPCClient interface.
type GRPCClient struct {
	dialOpt []grpc.DialOption
	agent   *Agent
}

func NewGRPCClient(dialOpt grpc.DialOption, agent *Agent) AgentGRPCClient {
	if dialOpt == nil {
		dialOpt = grpc.WithInsecure()
	}
	return &GRPCClient{
		dialOpt: []grpc.DialOption{
			dialOpt,
			grpc.WithBlock(),
		},
		agent: agent,
	}
}
