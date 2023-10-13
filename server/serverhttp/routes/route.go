package routes

import (
	v1 "github.com/THPTUHA/kairos/server/serverhttp/routes/v1"
	"github.com/gin-gonic/gin"
)

func Init(ginApp *gin.Engine) {
	routeGroup := ginApp.Group("/v1")

	v1.Auth(routeGroup)
}
