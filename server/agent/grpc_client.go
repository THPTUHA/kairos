package agent

import (
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// KiarosGRPCClient defines the interface that any gRPC client for
// kairos should implement.
type KiarosGRPCClient interface {
}

// GRPCClient is the local implementation of the KiarosGRPCClient interface.
type GRPCClient struct {
	dialOpt []grpc.DialOption
	agent   *Agent
	logger  *logrus.Entry
}

// NewGRPCClient returns a new instance of the gRPC client.
func NewGRPCClient(dialOpt grpc.DialOption, agent *Agent, logger *logrus.Entry) KiarosGRPCClient {
	if dialOpt == nil {
		dialOpt = grpc.WithInsecure()
	}
	return &GRPCClient{
		dialOpt: []grpc.DialOption{
			dialOpt,
			grpc.WithBlock(),
		},
		agent:  agent,
		logger: logger,
	}
}
