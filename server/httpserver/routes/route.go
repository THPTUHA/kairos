package routes

import (
	"github.com/THPTUHA/kairos/server/httpserver/middlewares"
	v1 "github.com/THPTUHA/kairos/server/httpserver/routes/v1"
	"github.com/gin-gonic/gin"
)

func initialize(ginApp *gin.Engine) {
	routeGroup := ginApp.Group("/v1")

	v1.Auth(routeGroup)
}

func Build() *gin.Engine {
	ginApp := gin.New()
	ginApp.Use(gin.Recovery())
	ginApp.Use(middlewares.CORSMiddleware())
	initialize(ginApp)

	return ginApp
}
