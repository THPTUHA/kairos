package agent

import "google.golang.org/grpc"

// KairosGRPCClient defines the interface that any gRPC client for
// kairos should implement.
type KairosGRPCClient interface {
}

// GRPCClient is the local implementation of the KairosGRPCClient interface.
type GRPCClient struct {
	dialOpt []grpc.DialOption
	agent   *Agent
}

func NewGRPCClient(dialOpt grpc.DialOption, agent *Agent) KairosGRPCClient {
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
