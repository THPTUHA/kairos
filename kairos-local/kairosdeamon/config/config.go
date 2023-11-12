package config

var (
	ServerPort          = 3111
	LoginEndpoint       = "http://localhost:8001/apis/v1/login"
	ServerPubSubEnpoint = "ws://localhost:8002/server/pubsub"
	PubSubEnpoint       = "ws://localhost:8003/pubsub"
	Token               = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhY2Nlc3NfdXVpZCI6IjhmMWY4ZmYzLTk1MTAtNGJkNi05MGIxLWE2ODUxMzlhYzMyNSIsImV4cCI6MTcwMTk3MTk4NCwidXNlcl9pZCI6IjIiLCJ1c2VyX25hbWUiOiJuZ3V5ZW5taW5obmdoaWFkZXZAZ21haWwuY29tIn0.0RgPW3IEyyLhP-jhs1njDWJYL_pli1JvpAHwUfn5NKI"
	LogLevel            = "debug"
	NodeName            = "kairosdeamon"

	ClientName = ""
	KairosName = ""
)
