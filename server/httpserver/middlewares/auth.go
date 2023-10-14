package middlewares

import (
	"net/http"

	"github.com/THPTUHA/kairos/server/httpserver/auth"
	"github.com/gin-gonic/gin"
)

func Authorize() gin.HandlerFunc {
	return func(c *gin.Context) {
		acc, err := auth.ExtractAccess(c.Request)
		if err != nil {
			c.JSON(http.StatusUnauthorized, "unauthorized")
			c.Abort()
			return
		}
		c.Set("user_id", acc.UserId)
		c.Next()
	}
}
