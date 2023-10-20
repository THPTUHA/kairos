package rule

import (
	"sync"
)

// TODO
type Config struct {
}

// Container ...
type Container struct {
	mu     sync.RWMutex
	config Config
}

// NewContainer ...
func NewContainer(config Config) (*Container, error) {
	preparedConfig, err := getPreparedConfig(config)
	if err != nil {
		return nil, err
	}
	return &Container{
		config: *preparedConfig,
	}, nil
}

func getPreparedConfig(config Config) (*Config, error) {
	if err := config.Validate(); err != nil {
		return &config, err
	}
	config, err := buildCompiledRegexes(config)
	if err != nil {
		return &config, err
	}
	return &config, nil
}

// TODO
// Validate validates config and returns error if problems found
func (c *Config) Validate() error {
	return nil
}

// TODO
func buildCompiledRegexes(config Config) (Config, error) {

	return config, nil
}
