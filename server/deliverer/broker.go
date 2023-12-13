package deliverer

import (
	"context"
)

type Publication struct {
	Data []byte
	Info *ClientInfo
	Tags map[string]string
}

type ClientInfo struct {
	ClientID string
	UserID   string
	ConnInfo []byte
	ChanInfo []byte
	ChanRole int32
}

type BrokerEventHandler interface {
	HandlePublication(ch string, pub *Publication) error
	HandleControl(data []byte) error
}

type PublishOptions struct {
	ClientInfo *ClientInfo
	Tags       map[string]string
}

type Broker interface {
	Run(BrokerEventHandler) error
	Subscribe(ch string) error
	Unsubscribe(ch string) error
	Publish(ch string, data []byte, opts PublishOptions) error
	PublishControl(data []byte, nodeID, shardKey string) error
}

type Closer interface {
	Close(ctx context.Context) error
}
