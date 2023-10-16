package agent

import "github.com/THPTUHA/kairos/server/plugin"

type Plugins struct {
	Processors map[string]plugin.Processor
	Executors  map[string]plugin.Executor
}

type Agent struct {
	// ProcessorPlugins maps processor plugins
	ProcessorPlugins map[string]plugin.Processor

	//ExecutorPlugins maps executor plugins
	ExecutorPlugins map[string]plugin.Executor
	config          *AgentConfig
	retryJoinCh     chan error
}

var agent *Agent

// AgentOption type that defines agent options
type AgentOption func(agent *Agent)

func NewAgent(config *AgentConfig, options ...AgentOption) *Agent {
	agent := &Agent{
		config:      config,
		retryJoinCh: make(chan error),
	}

	for _, option := range options {
		option(agent)
	}

	return agent
}

// WithPlugins option to set plugins to the agent
func WithPlugins(plugins Plugins) AgentOption {
	return func(agent *Agent) {
		agent.ProcessorPlugins = plugins.Processors
		agent.ExecutorPlugins = plugins.Executors
	}
}

func (a *Agent) Start() error {
	return nil
}

// RetryJoinCh is a channel that transports errors
// from the retry join process.
func (a *Agent) RetryJoinCh() <-chan error {
	return a.retryJoinCh
}

// UpdateTags updates the tag configuration for this agent
// TODO
func (a *Agent) UpdateTags(tags map[string]string) {

}

// TODO
func (a *Agent) Stop() error {
	return nil
}

// TODO
func (a *Agent) GetRunningJobs() int {
	return 0
}
