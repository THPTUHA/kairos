package routes

import (
	"github.com/THPTUHA/kairos/server/httpserver/auth"
	"github.com/THPTUHA/kairos/server/httpserver/controllers"
	"github.com/THPTUHA/kairos/server/httpserver/middlewares"
	"github.com/THPTUHA/kairos/server/httpserver/pubsub"
	"github.com/THPTUHA/kairos/server/httpserver/routes/ui"
	v1 "github.com/THPTUHA/kairos/server/httpserver/routes/v1"
	"github.com/THPTUHA/kairos/server/httpserver/runner"
	"github.com/gin-gonic/gin"
)

type Route struct {
	ginApp   *gin.Engine
	pubsub   chan *pubsub.PubSubPayload
	wfRunner *runner.Runner
}

func (r *Route) initialize() {
	rootPath := r.ginApp.Group("/")
	t := ui.UI(rootPath)
	r.ginApp.SetHTMLTemplate(t)

	routeGroup := r.ginApp.Group("/apis/v1")
	tokenService := auth.NewTokenService()

	ctr := controllers.NewController(&controllers.ControllerConfig{
		PubSubCh:     r.pubsub,
		WfRunner:     r.wfRunner,
		TokenService: tokenService,
	})

	v1.Auth(routeGroup, ctr)
	routeGroup.Use(auth.GoogleAuth())
	routeGroup.GET("/auth", ctr.Auth)

	privateGroup := r.ginApp.Group("/apis/v1/service")
	privateGroup.Use(middlewares.Authorize())
	privateGroup.POST("/apply", ctr.ApplyYaml)
	v1.Workflow(privateGroup, ctr)
	v1.Client(privateGroup, ctr)
	v1.Channel(privateGroup, ctr)
	v1.User(privateGroup, ctr)
	v1.Certificate(privateGroup, ctr)
}

func (r *Route) Build() *gin.Engine {
	r.initialize()
	return r.ginApp
}

func (r *Route) Run(path string) error {
	return r.ginApp.Run(path)
}

type RouteConfig struct {
	Pubsub   chan *pubsub.PubSubPayload
	WfRunner *runner.Runner
}

func New(conf *RouteConfig) *Route {
	ginApp := gin.New()
	ginApp.Use(gin.Recovery())
	ginApp.Use(middlewares.CORSMiddleware())

	return &Route{
		ginApp:   ginApp,
		pubsub:   conf.Pubsub,
		wfRunner: conf.WfRunner,
	}
}
