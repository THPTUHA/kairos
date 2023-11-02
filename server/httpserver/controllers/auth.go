package controllers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/THPTUHA/kairos/server/httpserver/auth"
	"github.com/THPTUHA/kairos/server/httpserver/config"
	"github.com/THPTUHA/kairos/server/httpserver/helper"
	"github.com/THPTUHA/kairos/server/httpserver/pubsub"
	"github.com/THPTUHA/kairos/server/storage"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func Login(c *gin.Context) {
	useCountID := c.Query("user_count_id")
	if useCountID == "" {
		c.JSON(http.StatusForbidden, gin.H{
			"message": "user_count_id not found",
		})
		return
	}

	if !strings.HasPrefix(useCountID, config.KairosDeamon) && strings.HasPrefix(useCountID, config.KairosWeb) {
		c.JSON(http.StatusForbidden, gin.H{
			"message": "user_count_id invalid",
		})
		return
	}
	fmt.Printf("Login:-------%s\n", useCountID)
	session := sessions.Default(c)
	session.Set(auth.StateKey, useCountID)
	session.Save()
	c.Writer.Write([]byte(helper.AutoRedirctUrl(auth.GetLoginURL(useCountID))))
}

func Auth(authChan chan *pubsub.PubSubPayload) func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		val, exist := ctx.Get("user")
		userCountID := ctx.Query(auth.StateKey)
		eventChan := make(chan pubsub.Event)
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
				fmt.Printf("Email user login %v, UserID=%d, userCountID=%s\n", val, userID, userCountID)
				if err != nil {
					ctx.JSON(http.StatusBadRequest, nil)
				}

				if strings.HasPrefix(userCountID, config.KairosWeb) {
					ctx.Writer.Write([]byte(helper.AutoRedirctUrl(fmt.Sprintf("%s/verify?token=%s", config.KairosWebURL, token.AccessToken))))
					eventChan <- pubsub.SuccessEvent
					return
				} else if strings.HasPrefix(userCountID, config.KairosDeamon) {
					authChan <- &pubsub.PubSubPayload{
						UserCountID: userCountID,
						Data:        token.AccessToken,
						Cmd:         pubsub.AuthCmd,
						Fn: func(event pubsub.Event) {
							eventChan <- event
						},
					}
				}
			}

		}
		select {
		case <-time.After(config.AuthTimeout):
			ctx.Writer.Write([]byte("Timeout login"))
			return
		case m := <-eventChan:
			if m == pubsub.SuccessEvent {
				ctx.Writer.Write([]byte("Login successful"))
			} else {
				ctx.Writer.Write([]byte("Login error"))
			}
			return
		}
	}
}
