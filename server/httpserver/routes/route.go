package routes

import (
	"github.com/THPTUHA/kairos/server/httpserver/auth"
	"github.com/THPTUHA/kairos/server/httpserver/helper"
	"github.com/THPTUHA/kairos/server/httpserver/middlewares"
	"github.com/THPTUHA/kairos/server/httpserver/routes/ui"
	v1 "github.com/THPTUHA/kairos/server/httpserver/routes/v1"
	"github.com/gin-gonic/gin"
	goauth "google.golang.org/api/oauth2/v2"
)

func initialize(ginApp *gin.Engine) {
	rootPath := ginApp.Group("/")
	t := ui.UI(rootPath)
	ginApp.SetHTMLTemplate(t)

	routeGroup := ginApp.Group("/apis/v1")

	v1.Auth(routeGroup)
	routeGroup.Use(auth.GoogleAuth())
	routeGroup.GET("/auth", func(ctx *gin.Context) {
		ctx.Writer.Write([]byte(helper.AutoRedirctUrl("http://localhost:8001/apis/v1/pass")))
	})

	routeGroup.GET("/pass", func(ctx *gin.Context) {
		var (
			res goauth.Userinfo
			ok  bool
		)

		val := ctx.MustGet("user")
		if res, ok = val.(goauth.Userinfo); !ok {
			res = goauth.Userinfo{Name: "no user"}
		}
		ctx.JSON(200, gin.H{"message": res.Email})
	})
}

func Build() *gin.Engine {
	ginApp := gin.New()
	ginApp.Use(gin.Recovery())
	ginApp.Use(middlewares.CORSMiddleware())
	initialize(ginApp)

	return ginApp
}
