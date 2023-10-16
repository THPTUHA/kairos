package plugin

import "github.com/THPTUHA/kairos/server/plugin/proto"

// Processor is an interface that wraps the Process method.
// Plugins must implement this interface.
type Processor interface {
	// Main plugin method, will be called when an execution is done.
	Process(args *ProcessorArgs) proto.Execution
}

// ProcessorArgs holds the Execution and PluginConfig for a Processor.
type ProcessorArgs struct {
	// The execution to pass to the processor
	Execution proto.Execution
	// The configuration for this plugin call
	Config Config
}

// Config holds a map of the plugin configuration data structure.
type Config map[string]string
