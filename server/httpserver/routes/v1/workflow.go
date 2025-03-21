package v1

import (
	"github.com/THPTUHA/kairos/server/httpserver/controllers"
	"github.com/gin-gonic/gin"
)

func Workflow(ginApp *gin.RouterGroup, ctr *controllers.Controller) {
	routeGroup := ginApp.Group("/workflow")
	routeGroup.POST("/apply", ctr.CreateWorkflow)
	routeGroup.POST("/:id/drop", ctr.DropWorkflow)

	routeGroup.GET("/list", ctr.ListWorkflow)
	routeGroup.GET("/:id/detail", ctr.DetailWorkflow)
	routeGroup.GET("/:id/record", ctr.GetWfRecord)
	routeGroup.GET("/:id/recover", ctr.RecoverWorkflow)
	routeGroup.POST("/request/sync", ctr.RequestSyncWorkflow)
	routeGroup.GET("/:id/objects", ctr.GetObjectOnWorkflow)

	routeGroup.POST("/trigger", ctr.TriggerWorkflow)
	routeGroup.GET("/trigger/delete", ctr.DeleteTriggerWorkflow)
	routeGroup.POST("/trigger/list", ctr.ListTrigger)
}
