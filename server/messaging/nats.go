package messaging

import (
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

type NatsSubConfig struct {
	URL           string
	Name          string
	ReconnectWait time.Duration
	MaxReconnects int

	Logger *logrus.Entry
}

type NatsSub struct {
	config *NatsSubConfig
}

func NewNatsSub(config *NatsSubConfig) *NatsSub {
	return &NatsSub{
		config: config,
	}
}

func optNats(o *NatsSubConfig) []nats.Option {
	opts := make([]nats.Option, 0)
	opts = append(opts, nats.Name(o.Name))
	opts = append(opts, nats.MaxReconnects(o.MaxReconnects))
	opts = append(opts, nats.ReconnectWait(o.ReconnectWait))
	return opts
}

func (ns *NatsSub) Subscribes(subs map[string]nats.MsgHandler, signals chan os.Signal) {
	nc, err := nats.Connect(*&ns.config.URL, optNats(ns.config)...)
	if err != nil {
		ns.config.Logger.Error(err)
		return
	}
	for subject, cb := range subs {
		_, err := nc.Subscribe(subject, cb)
		if err != nil {
			ns.config.Logger.Error(err)
			return
		}
	}
	nc.Flush()
	<-signals
	ns.config.Logger.Warn("stop subscribe nat")
}
