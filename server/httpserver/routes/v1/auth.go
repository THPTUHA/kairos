package v1

import (
	"github.com/THPTUHA/kairos/server/httpserver/auth"
	"github.com/THPTUHA/kairos/server/httpserver/config"
	"github.com/THPTUHA/kairos/server/httpserver/controllers"
	"github.com/gin-gonic/gin"
)

func Auth(ginApp *gin.RouterGroup, ctr *controllers.Controller, cfg *config.Configs) {

	secret := []byte("secret")
	sessionName := "kairossession"

	scopes := []string{
		"https://www.googleapis.com/auth/userinfo.email",
	}

	auth.Setup("http://localhost:8001/apis/v1/auth", scopes, secret, cfg)
	ginApp.Use(auth.Session(sessionName))

	ginApp.GET("/login", ctr.Login)
}
