package routes

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/THPTUHA/kairos/server/httpserver/auth"
	"github.com/THPTUHA/kairos/server/httpserver/controllers"
	"github.com/THPTUHA/kairos/server/httpserver/helper"
	"github.com/THPTUHA/kairos/server/httpserver/middlewares"
	"github.com/THPTUHA/kairos/server/httpserver/routes/ui"
	v1 "github.com/THPTUHA/kairos/server/httpserver/routes/v1"
	"github.com/THPTUHA/kairos/server/storage"
	"github.com/gin-gonic/gin"
)

func initialize(ginApp *gin.Engine) {
	rootPath := ginApp.Group("/")
	t := ui.UI(rootPath)
	ginApp.SetHTMLTemplate(t)

	routeGroup := ginApp.Group("/apis/v1")

	v1.Auth(routeGroup)
	routeGroup.Use(auth.GoogleAuth())
	routeGroup.GET("/auth", func(ctx *gin.Context) {
		val, exist := ctx.Get("user")

		if exist && val != "" {

			if val, ok := val.(string); ok {

				var userID int64
				row := storage.Get().QueryRow(fmt.Sprintf("SELECT id FROM users where email = '%s'", val))
				err := row.Scan(&userID)
				if err != nil {
					username := strings.Split(val, "@")[0]
					if err == sql.ErrNoRows {
						_, err := storage.Get().Exec(fmt.Sprintf("INSERT INTO users(username,full_name,email) VALUES ('%s','%s','%s');", username, username, val))
						if err != nil {
							ctx.JSON(http.StatusBadRequest, nil)
							return
						}
						row := storage.Get().QueryRow(fmt.Sprintf("SELECT id FROM users where email = '%s'", val))
						row.Scan(&userID)
					}
				}

				tokenService := auth.NewTokenService()
				token, err := tokenService.CreateToken(strconv.Itoa(int(userID)), val)
				fmt.Printf("Email user login %v, UserID=%d\n", val, userID)
				if err != nil {
					ctx.JSON(http.StatusBadRequest, nil)
				}

				ctx.Writer.Write([]byte(helper.AutoRedirctUrl("http://localhost:3000/verify?token=" + token.AccessToken)))
			}

		}

		ctx.Writer.Write([]byte("Must login"))
	})

	privateGroup := ginApp.Group("/apis/v1/workflow")
	privateGroup.Use(middlewares.Authorize())
	privateGroup.POST("/apply", controllers.ApplyYaml)
}

func Build() *gin.Engine {
	ginApp := gin.New()
	ginApp.Use(gin.Recovery())
	ginApp.Use(middlewares.CORSMiddleware())
	initialize(ginApp)

	return ginApp
}
