package controllers

import (
	"github.com/THPTUHA/kairos/pkg/logger"
	"github.com/THPTUHA/kairos/server/httpserver/auth"
	"github.com/THPTUHA/kairos/server/httpserver/pubsub"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

type ControllerConfig struct {
	PubSubCh     chan *pubsub.PubSubPayload
	TokenService *auth.TokenManager
	Nats         *nats.Conn
}

type Controller struct {
	pubSubCh     chan *pubsub.PubSubPayload
	nats         *nats.Conn
	TokenService *auth.TokenManager
	Log          *logrus.Entry
}

func NewController(ctrconf *ControllerConfig) *Controller {
	return &Controller{
		pubSubCh:     ctrconf.PubSubCh,
		TokenService: ctrconf.TokenService,
		nats:         ctrconf.Nats,
		Log:          logger.InitLogger(logrus.DebugLevel.String(), "controller"),
	}
}
