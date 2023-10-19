package agent

import "github.com/THPTUHA/kairos/server/plugin/proto"

type AgentServer struct {
	proto.AgentServer
	agent *Agent
}

func NewAgentServer(agent *Agent) proto.AgentServer {
	return &AgentServer{
		agent: agent,
	}
}
