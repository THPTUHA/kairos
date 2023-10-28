package routers

import (
	"github.com/THPTUHA/kairos/agent/kairos-local/kairosdeamon/controllers"
	"github.com/gin-gonic/gin"
)

func Setup() *gin.Engine {
	ginApp := gin.New()
	routeGroup := ginApp.Group("/apis")

	routeGroup.POST("/apply-collection", controllers.ApplyYaml)
	return ginApp
}
