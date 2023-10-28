package main

import (
	kplugin "github.com/THPTUHA/kairos/server/plugin"
	"github.com/hashicorp/go-plugin"
)

func main() {
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: kplugin.Handshake,
		Plugins: map[string]plugin.Plugin{
			"executor": &kplugin.ExecutorPlugin{Executor: &HTTP{}},
		},

		// A non-nil value here enables gRPC serving for this plugin...
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
