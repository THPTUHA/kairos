package deliverer

import (
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Version                         string
	Name                            string
	LogLevel                        LogLevel
	LogHandler                      LogHandler
	ClientPresenceUpdateInterval    time.Duration
	ClientExpiredCloseDelay         time.Duration
	ClientExpiredSubCloseDelay      time.Duration
	ClientStaleCloseDelay           time.Duration
	ClientChannelPositionCheckDelay time.Duration
	ClientQueueMaxSize              int
	ClientChannelLimit              int
	UserConnectionLimit             int
	ChannelMaxLength                int
	RecoveryMaxPublicationLimit     int
	UseSingleFlight                 bool

	GetChannelNamespaceLabel                      func(channel string) string
	ChannelNamespaceLabelForTransportMessagesSent bool
}

type PingPongConfig struct {
	PingInterval time.Duration
	PongTimeout  time.Duration
}

const (
	nodeInfoPublishInterval = 3 * time.Second
	nodeInfoCleanInterval   = nodeInfoPublishInterval * 3
	nodeInfoMaxDelay        = nodeInfoPublishInterval*2 + time.Second
)

func getPingPongPeriodValues(config PingPongConfig) (time.Duration, time.Duration) {
	pingInterval := config.PingInterval
	if pingInterval < 0 {
		pingInterval = 0
	} else if pingInterval == 0 {
		pingInterval = 25 * time.Second
	}
	pongTimeout := config.PongTimeout
	if pongTimeout < 0 {
		pongTimeout = 0
	} else if pongTimeout == 0 {
		pongTimeout = 10 * time.Second
	}
	return pingInterval, pongTimeout
}

type ServerConfig struct {
	Auth struct {
		HmacSecret string `yaml:"hmacsecret"`
		HmrfSecret string `yaml:"hmrfsecret"`
	}
	Deliverer struct {
		Port int `yaml:"port"`
	}

	Nats struct {
		URL  string `yaml:"url"`
		Name string `yaml:"name"`
	}
}

func SetConfig(f string) (*ServerConfig, error) {
	config := &ServerConfig{}
	file, err := os.ReadFile(f)
	if err != nil {
		return nil, err
	}
	err = GetYaml(file, config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func GetYaml(f []byte, s interface{}) error {
	y := yaml.Unmarshal(f, s)
	return y
}
