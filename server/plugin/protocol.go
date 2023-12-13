package plugin

import (
	"io"
	"net"
)

type Protocol string

const (
	ProtocolInvalid Protocol = ""
	ProtocolGRPC    Protocol = "grpc"
)

type ServerProtocol interface {
	Init() error
	Config() string
	Serve(net.Listener)
}

type ClientProtocol interface {
	io.Closer
	Dispense(string) (interface{}, error)
	Ping() error
}
