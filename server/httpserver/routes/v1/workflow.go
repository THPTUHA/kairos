package v1

import (
	"github.com/THPTUHA/kairos/server/httpserver/controllers"
	"github.com/gin-gonic/gin"
)

func Workflow(ginApp *gin.RouterGroup, ctr *controllers.Controller) {
	routeGroup := ginApp.Group("/workflow")
	routeGroup.POST("/apply", ctr.CreateWorkflow)
	routeGroup.DELETE("/:id/drop", ctr.DropWorkflow)
	routeGroup.GET("/list", ctr.ListWorkflow)
	routeGroup.GET("/:id/detail", ctr.DetailWorkflow)

}
