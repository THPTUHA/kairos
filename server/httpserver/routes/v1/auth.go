package v1

import (
	"github.com/THPTUHA/kairos/server/httpserver/auth"
	"github.com/THPTUHA/kairos/server/httpserver/controllers"
	"github.com/gin-gonic/gin"
)

func Auth(ginApp *gin.RouterGroup) {

	secret := []byte("secret")
	sessionName := "kairossession"

	scopes := []string{
		"https://www.googleapis.com/auth/userinfo.email",
	}

	auth.Setup("http://localhost:8001/apis/v1/auth", scopes, secret)
	ginApp.Use(auth.Session(sessionName))

	ginApp.GET("/login", controllers.Login)
}
