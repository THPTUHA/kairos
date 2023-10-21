package plugin

import "github.com/THPTUHA/kairos/server/plugin/proto"

type StatusHelper interface {
	Update([]byte, bool) (int64, error)
}

// Executor is the interface that we're exposing as a plugin.
type Executor interface {
	Execute(args *proto.ExecuteRequest, cb StatusHelper) (*proto.ExecuteResponse, error)
}

// ExecutorPluginConfig is the plugin config
type ExecutorPluginConfig map[string]string
