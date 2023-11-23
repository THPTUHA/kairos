package config

// var (
// 	ServerPort          = 3111
// 	LoginEndpoint       = "http://localhost:8001/apis/v1/login"
// 	ServerPubSubEnpoint = "ws://localhost:8002/server/pubsub"
// 	PubSubEnpoint       = "ws://localhost:8003/pubsub"
// 	LogLevel            = "debug"
// 	NodeName            = "kairosdeamon"
// 	AgentHTTPAddrPort   = 8080
// 	AgentDefaultPort    = 6868
// 	AgentDataDir        = "kairos.boltdb"
// )

type Configs struct {
	ServerPort          int    `yaml:"serverPort"`
	LoginEndpoint       string `yaml:"loginEndpoint"`
	ServerPubSubEnpoint string `yaml:"serverPubSubEnpoint"`
	PubSubEnpoint       string `yaml:"pubSubEnpoint"`
	LogLevel            string `yaml:"logLevel"`
	NodeName            string `yaml:"nodeName"`
	AgentHTTPAddrPort   int    `yaml:"agentHTTPAddrPort"`
	AgentDefaultPort    int    `yaml:"agentDefaultPort"`
	AgentDataDir        string `yaml:"agentDataDir"`
}

type Auth struct {
	Token      string `json:"token"`
	ClientName string `json:"client_name"`
	ClientID   string `json:"client_id"`
	UserID     string `json:"user_id"`
}
