package v1

import (
	"github.com/THPTUHA/kairos/server/httpserver/controllers"
	"github.com/gin-gonic/gin"
)

func Workflow(ginApp *gin.RouterGroup) {
	routeGroup := ginApp.Group("/workflow")
	routeGroup.POST("/apply", controllers.CreateWorkflow)
	routeGroup.DELETE("/:id/drop", controllers.DropWorkflow)
	routeGroup.GET("/list", controllers.ListWorkflow)
	routeGroup.GET("/:id/detail", controllers.DetailWorkflow)

}
