package main

import (
	kplugin "github.com/THPTUHA/kairos/server/plugin"
	"github.com/hashicorp/go-plugin"
)

func main() {
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: kplugin.Handshake,
		Plugins: map[string]plugin.Plugin{
			"executor": &kplugin.ExecutorPlugin{Executor: &Script{}},
		},

		GRPCServer: plugin.DefaultGRPCServer,
	})
}
