package cmd

import (
	"github.com/THPTUHA/kairos/server/plugin"
)

type Plugins struct {
	Processors map[string]plugin.Processor
	Executors  map[string]plugin.Executor
	LogLevel   string
	NodeName   string
}

// TODO
func (p *Plugins) DiscoverPlugins() error {
	p.Processors = make(map[string]plugin.Processor)
	p.Executors = make(map[string]plugin.Executor)
	// pluginDir := filepath.Join("/etc", "kairos", "plugins")

	// if viper.ConfigFileUsed() != "" {
	// 	pluginDir = filepath.Join(filepath.Dir(viper.ConfigFileUsed()), "plugins")
	// }

	return nil
}
