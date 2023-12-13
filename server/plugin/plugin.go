package plugin

import (
	"context"
	"errors"
	"net/rpc"

	"google.golang.org/grpc"
)

type Plugin interface {
	Server(*MuxBroker) (interface{}, error)
	Client(*MuxBroker, *rpc.Client) (interface{}, error)
}

type GRPCPlugin interface {
	GRPCServer(*GRPCBroker, *grpc.Server) error
	GRPCClient(context.Context, *GRPCBroker, *grpc.ClientConn) (interface{}, error)
}

type NetRPCUnsupportedPlugin struct{}

func (p NetRPCUnsupportedPlugin) Server(*MuxBroker) (interface{}, error) {
	return nil, errors.New("net/rpc plugin protocol not supported")
}

func (p NetRPCUnsupportedPlugin) Client(*MuxBroker, *rpc.Client) (interface{}, error) {
	return nil, errors.New("net/rpc plugin protocol not supported")
}

type InputTask struct {
	DeliverID  int64  `json:"deliver_id"`
	WorkflowID int64  `json:"workflow_id"`
	Input      string `json:"input"`
}

var PluginMap = map[string]Plugin{
	"executor": &ExecutorPlugin{},
}
