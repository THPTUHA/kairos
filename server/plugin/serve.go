package plugin

const (
	ExecutorPluginName = "executor"
)

var Handshake = HandshakeConfig{
	ProtocolVersion: 1,
}

type ServeOpts struct {
	Executor Executor
}

func Serves(opts *ServeOpts) {
	Serve(&ServeConfig{
		HandshakeConfig: Handshake,
		Plugins:         pluginMap(opts),
	})
}

func pluginMap(opts *ServeOpts) map[string]Plugin {
	return map[string]Plugin{}
}
