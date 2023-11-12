package routers

import (
	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/controllers"
	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/events"
	"github.com/gin-gonic/gin"
)

func Setup(eventCh chan *events.Event) *gin.Engine {
	ginApp := gin.New()
	routeGroup := ginApp.Group("/apis")
	routeGroup.GET("/login", controllers.Login(eventCh))
	return ginApp
}
