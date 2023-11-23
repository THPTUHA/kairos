package controllers

import (
	"github.com/THPTUHA/kairos/server/httpserver/auth"
	"github.com/THPTUHA/kairos/server/httpserver/pubsub"
	"github.com/THPTUHA/kairos/server/httpserver/runner"
)

type ControllerConfig struct {
	WfRunner     *runner.Runner
	PubSubCh     chan *pubsub.PubSubPayload
	TokenService *auth.TokenManager
}

type Controller struct {
	wfRunner     *runner.Runner
	pubSubCh     chan *pubsub.PubSubPayload
	TokenService *auth.TokenManager
}

func NewController(ctrconf *ControllerConfig) *Controller {
	return &Controller{
		wfRunner:     ctrconf.WfRunner,
		pubSubCh:     ctrconf.PubSubCh,
		TokenService: ctrconf.TokenService,
	}
}
