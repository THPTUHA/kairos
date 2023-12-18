package deliverer

import "github.com/THPTUHA/kairos/pkg/protocol/deliverprotocol"

type ProtocolType string

func (t ProtocolType) toProto() deliverprotocol.Type {
	return deliverprotocol.Type(t)
}

const (
	ProtocolTypeJSON ProtocolType = "json"
)

const (
	ProtocolVersion2 ProtocolVersion = 2
)

const (
	PushFlagConnect uint64 = 1 << iota
	PushFlagDisconnect
	PushFlagSubscribe
	PushFlagJoin
	PushFlagLeave
	PushFlagUnsubscribe
	PushFlagPublication
	PushFlagMessage
)

type ProtocolVersion uint8

type TransportInfo interface {
	Name() string
	Protocol() ProtocolType
	PingPongConfig() PingPongConfig
}

type Transport interface {
	TransportInfo
	Write([]byte) error
	WriteMany(...[]byte) error
	Close(Disconnect) error
}
