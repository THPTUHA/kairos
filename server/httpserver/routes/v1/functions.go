package v1

import (
	"github.com/THPTUHA/kairos/server/httpserver/controllers"
	"github.com/gin-gonic/gin"
)

func Functions(ginApp *gin.RouterGroup, ctr *controllers.Controller) {
	routeGroup := ginApp.Group("/functions")
	routeGroup.POST("/create", ctr.CreateFunction)
	routeGroup.GET("/list", ctr.ListFunction)
	routeGroup.GET("/delete", ctr.DeleteFunction)
	routeGroup.POST("/debug", ctr.Debug)
}
