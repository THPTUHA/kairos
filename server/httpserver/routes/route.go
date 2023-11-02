package routes

import (
	"github.com/THPTUHA/kairos/server/httpserver/auth"
	"github.com/THPTUHA/kairos/server/httpserver/controllers"
	"github.com/THPTUHA/kairos/server/httpserver/middlewares"
	"github.com/THPTUHA/kairos/server/httpserver/pubsub"
	"github.com/THPTUHA/kairos/server/httpserver/routes/ui"
	v1 "github.com/THPTUHA/kairos/server/httpserver/routes/v1"
	"github.com/gin-gonic/gin"
)

func initialize(ginApp *gin.Engine, pubsub chan *pubsub.PubSubPayload) {
	rootPath := ginApp.Group("/")
	t := ui.UI(rootPath)
	ginApp.SetHTMLTemplate(t)

	routeGroup := ginApp.Group("/apis/v1")

	v1.Auth(routeGroup)
	routeGroup.Use(auth.GoogleAuth())
	routeGroup.GET("/auth", controllers.Auth(pubsub))

	privateGroup := ginApp.Group("/apis/v1/workflow")
	privateGroup.Use(middlewares.Authorize())
	privateGroup.POST("/apply", controllers.ApplyYaml)
}

func Build(pubsub chan *pubsub.PubSubPayload) *gin.Engine {
	ginApp := gin.New()
	ginApp.Use(gin.Recovery())
	ginApp.Use(middlewares.CORSMiddleware())
	initialize(ginApp, pubsub)

	return ginApp
}
