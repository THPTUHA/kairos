package agent

import (
	"github.com/THPTUHA/kairos/server/plugin/proto"
	"github.com/sirupsen/logrus"
)

// GRPCAgentServer is the local implementation of the gRPC server interface.
type AgentServer struct {
	proto.AgentServer
	agent  *Agent
	logger *logrus.Entry
}

// NewServer creates and returns an instance of a DkronGRPCServer implementation
func NewAgentServer(agent *Agent, logger *logrus.Entry) proto.AgentServer {
	return &AgentServer{
		agent:  agent,
		logger: logger,
	}
}
