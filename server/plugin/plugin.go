package plugin

import (
	"context"

	"google.golang.org/grpc"
)

type Plugin interface {
}

type GRPCPlugin interface {
	GRPCServer(*GRPCBroker, *grpc.Server) error
	GRPCClient(context.Context, *GRPCBroker, *grpc.ClientConn) (interface{}, error)
}

type NetRPCUnsupportedPlugin struct{}

type InputTask struct {
	DeliverID  int64  `json:"deliver_id"`
	WorkflowID int64  `json:"workflow_id"`
	Input      string `json:"input"`
}

var PluginMap = map[string]Plugin{
	"executor": &ExecutorPlugin{},
}
