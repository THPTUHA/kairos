package main

import (
	"github.com/THPTUHA/kairos/server/plugin"
)

func main() {
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: plugin.Handshake,
		Plugins: map[string]plugin.Plugin{
			"executor": &plugin.ExecutorPlugin{Executor: &Sql{}},
		},

		GRPCServer: plugin.DefaultGRPCServer,
	})
}
