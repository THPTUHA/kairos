package agent

import "github.com/THPTUHA/kairos/kairos-local/kairosdeamon/events"

func WithPlugins(plugins *Plugins) AgentOption {
	return func(agent *Agent) {
		agent.ExecutorPlugins = *plugins
	}
}

func WithEventCh(eventCh chan *events.Event) AgentOption {
	return func(agent *Agent) {
		agent.EventCh = eventCh
	}
}
