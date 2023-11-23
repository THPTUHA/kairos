package pubsub

import (
	"time"

	"github.com/THPTUHA/kairos/pkg/protocol/deliverprotocol"
)

type transport interface {
	Read() (*deliverprotocol.Reply, *disconnect, error)
	Write(cmd *deliverprotocol.Command, timeout time.Duration) error
	Close() error
}
