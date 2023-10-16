package agent

import flag "github.com/spf13/pflag"

type AgentConfig struct {
	// NodeName is the name we register as. Defaults to hostname.
	NodeName string `mapstructure:"node-name"`
	Server   bool
	// LogLevel is the log verbosity level used.
	// It can be (debug|info|warn|error|fatal|panic).
	LogLevel string `mapstructure:"log-level"`

	// Tags are used to attach key/value metadata to a node.
	Tags map[string]string `mapstructure:"tags"`
}

func DefaultConfig() *AgentConfig {
	return &AgentConfig{}
}

func ConfigAgentFlagSet() *flag.FlagSet {
	// c := DefaultConfig()
	cmdFlags := flag.NewFlagSet("agent flagset", flag.ContinueOnError)
	cmdFlags.Bool("server", false,
		"This node is running in server mode")
	return cmdFlags
}
