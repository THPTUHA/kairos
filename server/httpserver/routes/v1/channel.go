package v1

import (
	"github.com/THPTUHA/kairos/server/httpserver/controllers"
	"github.com/gin-gonic/gin"
)

func Channel(ginApp *gin.RouterGroup, ctr *controllers.Controller) {
	routeGroup := ginApp.Group("/channel")
	routeGroup.GET("/list", ctr.GetChannels)
	routeGroup.POST("/create", ctr.AddChannel)
}
