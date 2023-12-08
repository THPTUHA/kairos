package v1

import (
	"github.com/THPTUHA/kairos/server/httpserver/controllers"
	"github.com/gin-gonic/gin"
)

func Record(ginApp *gin.RouterGroup, ctr *controllers.Controller) {
	routeGroup := ginApp.Group("/record")
	routeGroup.GET("/task/:task_id", ctr.GetTaskRecord)
	routeGroup.GET("/client/:client_id", ctr.GetClientRecord)
	routeGroup.GET("/broker/:broker_id", ctr.GetBrokerRecord)
	routeGroup.POST("/message_flows", ctr.GetMessageFlows)
}
