package deliverer

import "github.com/THPTUHA/kairos/pkg/protocol/deliverprotocol"

type ProtocolType string

func (t ProtocolType) toProto() deliverprotocol.Type {
	return deliverprotocol.Type(t)
}

const (
	// ProtocolTypeJSON means JSON-based deliverprotocol.
	ProtocolTypeJSON ProtocolType = "json"
	// ProtocolTypeProtobuf means Protobuf deliverprotocol.
	ProtocolTypeProtobuf ProtocolType = "protobuf"
)

const (
	// ProtocolVersion2 is the current stable client deliverprotocol.
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

// ProtocolVersion defines deliverprotocol behavior.
type ProtocolVersion uint8

// TransportInfo has read-only transport description methods. Some of these methods
// can modify the behaviour of Client.
type TransportInfo interface {
	Name() string
	Protocol() ProtocolType
	PingPongConfig() PingPongConfig
}

// Transport abstracts a connection transport between server and client.
// It does not contain Read method as reading can be handled by connection
// handler code (for example by WebsocketHandler.ServeHTTP).
type Transport interface {
	TransportInfo
	// Write should write single push data into a connection. Every byte slice
	// here is a single Reply (or Push for unidirectional transport) encoded
	// according transport ProtocolType.
	Write([]byte) error
	// WriteMany should write data into a connection. Every byte slice here is a
	// single Reply (or Push for unidirectional transport) encoded according
	// transport ProtocolType.
	// The reason why we have both Write and WriteMany here is to have a path
	// without extra allocations for massive broadcasts (since variadic args cause
	// allocation).
	WriteMany(...[]byte) error
	// Close must close transport. Transport implementation can optionally
	// handle Disconnect passed here. For example builtin WebSocket transport
	// sends Disconnect as part of websocket.CloseMessage.
	Close(Disconnect) error
}
