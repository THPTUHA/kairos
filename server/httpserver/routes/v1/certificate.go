package v1

import (
	"github.com/THPTUHA/kairos/server/httpserver/controllers"
	"github.com/gin-gonic/gin"
)

func Certificate(ginApp *gin.RouterGroup, ctr *controllers.Controller) {
	routeGroup := ginApp.Group("/certificate")
	routeGroup.POST("/create", ctr.CreateCertificate)
	routeGroup.GET("/list", ctr.ListCertificate)
	routeGroup.GET("/permit", ctr.PermisCert)

}
