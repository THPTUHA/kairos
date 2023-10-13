package v1

import (
	"github.com/THPTUHA/kairos/server/serverhttp/controllers"
	"github.com/gin-gonic/gin"
)

func Auth(ginApp *gin.RouterGroup) {
	routeGroup := ginApp.Group("/auth")

	routeGroup.POST("/login", controllers.Login)
}
