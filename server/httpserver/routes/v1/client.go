package v1

import (
	"github.com/THPTUHA/kairos/server/httpserver/controllers"
	"github.com/gin-gonic/gin"
)

func Client(ginApp *gin.RouterGroup, ctr *controllers.Controller) {
	routeGroup := ginApp.Group("/client")
	routeGroup.GET("/list", ctr.GetClients)
	routeGroup.POST("/create", ctr.CreateClient)
}
