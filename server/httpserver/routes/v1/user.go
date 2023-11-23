package v1

import (
	"github.com/THPTUHA/kairos/server/httpserver/controllers"
	"github.com/gin-gonic/gin"
)

func User(ginApp *gin.RouterGroup, ctr *controllers.Controller) {
	routeGroup := ginApp.Group("/user")
	routeGroup.GET("/info", ctr.InfoUser)
}
