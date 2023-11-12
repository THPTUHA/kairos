package agent

func WithPlugins(plugins Plugins) AgentOption {
	return func(agent *Agent) {
		agent.ProcessorPlugins = plugins.Processors
		agent.ExecutorPlugins = plugins.Executors
	}
}
