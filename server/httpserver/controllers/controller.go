package controllers

import (
	"github.com/THPTUHA/kairos/server/httpserver/auth"
	"github.com/THPTUHA/kairos/server/httpserver/pubsub"
	"github.com/THPTUHA/kairos/server/httpserver/runner"
	"github.com/nats-io/nats.go"
)

type ControllerConfig struct {
	WfRunner     *runner.Runner
	PubSubCh     chan *pubsub.PubSubPayload
	TokenService *auth.TokenManager
	Nats         *nats.Conn
}

type Controller struct {
	wfRunner     *runner.Runner
	pubSubCh     chan *pubsub.PubSubPayload
	nats         *nats.Conn
	TokenService *auth.TokenManager
}

func NewController(ctrconf *ControllerConfig) *Controller {
	return &Controller{
		wfRunner:     ctrconf.WfRunner,
		pubSubCh:     ctrconf.PubSubCh,
		TokenService: ctrconf.TokenService,
		nats:         ctrconf.Nats,
	}
}
