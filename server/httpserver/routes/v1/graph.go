package v1

import (
	"github.com/THPTUHA/kairos/server/httpserver/controllers"
	"github.com/gin-gonic/gin"
)

func Graph(ginApp *gin.RouterGroup, ctr *controllers.Controller) {
	routeGroup := ginApp.Group("/graph")
	routeGroup.POST("/get", ctr.GetGraph)
	routeGroup.POST("/data", ctr.GraphData)
	routeGroup.GET("/timeline", ctr.GetTimeLine)
	routeGroup.GET("/group", ctr.GetGroupID)
	routeGroup.POST("/part", ctr.DetailPoint)
}
