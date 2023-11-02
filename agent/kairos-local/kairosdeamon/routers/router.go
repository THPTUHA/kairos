package routers

import (
	"github.com/THPTUHA/kairos/agent/kairos-local/kairosdeamon/controllers"
	"github.com/gin-gonic/gin"
)

func Setup() *gin.Engine {
	ginApp := gin.New()
	routeGroup := ginApp.Group("/apis")
	routeGroup.GET("/login", controllers.Login)
	routeGroup.POST("/apply-collection", controllers.ApplyYaml)

	workflowGroup := routeGroup.Group("/collection")
	workflowGroup.POST("/list", controllers.ListCollection)
	workflowGroup.POST("/workflow/list", controllers.ListWorkflowCollection)
	return ginApp
}
