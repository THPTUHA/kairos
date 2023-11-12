package pubsub

func NewClient(endpoint string, config Config) *Client {
	return newClient(endpoint, false, config)
}
