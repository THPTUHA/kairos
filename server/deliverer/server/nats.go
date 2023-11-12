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
}

func NewNatsSub(config *natsSubConfig) *natsSub {
	return &natsSub{
		config: config,
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
	nc, err := nats.Connect(*&ns.config.url, optNats(ns.config)...)
	if err != nil {
		ns.config.logger.Error(err)
		return
	}
	for subject, cb := range subs {
		_, err := nc.Subscribe(subject, cb)
		if err != nil {
			ns.config.logger.Error(err)
			return
		}
	}
	nc.Flush()
	<-signals
	ns.config.logger.Warn("stop subscribe nat")
}
