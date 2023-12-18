package pubsub

import (
	"context"
	"net"
	"net/http"
	"time"
)

type Config struct {
	Token              string
	GetToken           func(ConnectionTokenEvent) (string, error)
	Data               []byte
	Header             http.Header
	Name               string
	NetDialContext     func(ctx context.Context, network, addr string) (net.Conn, error)
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	HandshakeTimeout   time.Duration
	MaxServerPingDelay time.Duration
	EnableCompression  bool
}
