package main

import (
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

type natsSubConfig struct {
	url           string
	name          string
	reconnectWait time.Duration
	maxReconnects int

	logger *logrus.Entry
}

type natsSub struct {
	config *natsSubConfig
	Con    *nats.Conn
}

func NewNatsSub(config *natsSubConfig) *natsSub {
	nc, err := nats.Connect(config.url, optNats(config)...)
	if err != nil {
		config.logger.Error(err)
		return nil
	}
	return &natsSub{
		config: config,
		Con:    nc,
	}
}

func optNats(o *natsSubConfig) []nats.Option {
	opts := make([]nats.Option, 0)
	opts = append(opts, nats.Name(o.name))
	opts = append(opts, nats.MaxReconnects(o.maxReconnects))
	opts = append(opts, nats.ReconnectWait(o.reconnectWait))
	return opts
}

func (ns *natsSub) Subscribes(subs map[string]nats.MsgHandler, signals chan os.Signal) {
	for subject, cb := range subs {
		_, err := ns.Con.Subscribe(subject, cb)
		if err != nil {
			ns.config.logger.Error(err)
			return
		}
	}
	// ns.con.Flush()
	<-signals
	ns.config.logger.Warn("stop subscribe nat")
}
