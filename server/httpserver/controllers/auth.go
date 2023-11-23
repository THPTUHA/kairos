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
	"github.com/THPTUHA/kairos/server/storage/models"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func (ctr *Controller) Login(c *gin.Context) {
	useCountID := c.Query("user_count_id")
	if useCountID == "" {
		c.JSON(http.StatusForbidden, gin.H{
			"message": "user_count_id not found",
		})
		return
	}

	if !strings.HasPrefix(useCountID, config.KairosDeamon) && !strings.HasPrefix(useCountID, config.KairosWeb) {
		c.JSON(http.StatusForbidden, gin.H{
			"message": "user_count_id invalid",
		})
		return
	}
	session := sessions.Default(c)
	session.Set(auth.StateKey, useCountID)
	session.Save()
	c.Writer.Write([]byte(helper.AutoRedirctUrl(auth.GetLoginURL(useCountID))))
}

func (ctr *Controller) Auth(ctx *gin.Context) {
	val, exist := ctx.Get("user")
	userCountID := ctx.Query(auth.StateKey)
	eventChan := make(chan pubsub.Event)
	if exist && val != "" {
		if val, ok := val.(string); ok {
			avatar, exist := ctx.Get("avatar")
			if !exist {
				avatar = ""
			}
			var userID int64
			err := storage.Get().QueryRow(fmt.Sprintf("SELECT id FROM users where email = '%s'", val)).Scan(&userID)
			if err == sql.ErrNoRows {
				username := strings.Split(val, "@")[0]
				err = storage.Get().QueryRow(fmt.Sprintf("INSERT INTO users(username,full_name,email, avatar) VALUES ('%s','%s','%s','%s') RETURNING id", username, username, val, avatar)).Scan(&userID)
				if err != nil {
					ctx.JSON(http.StatusBadRequest, err.Error())
					return
				}
			} else if err != nil {
				row := storage.Get().QueryRow(fmt.Sprintf("SELECT id FROM users where email = '%s'", val))
				row.Scan(&userID)

				fmt.Printf("Email user login %v, UserID=%d, userCountID=%s\n", val, userID, userCountID)

			} else {
				if err != nil {
					ctx.JSON(http.StatusBadRequest, err.Error())
					return
				}
			}

			if strings.HasPrefix(userCountID, config.KairosWeb) {
				token, err := ctr.TokenService.CreateToken(strconv.Itoa(int(userID)), "0", fmt.Sprint(models.KairosUser))
				if err != nil {
					ctx.JSON(http.StatusBadRequest, nil)
				}
				ctx.Writer.Write([]byte(helper.AutoRedirctUrl(fmt.Sprintf("%s/verify?token=%s", config.KairosWebURL, token.AccessToken))))
				return
			} else if strings.HasPrefix(userCountID, config.KairosDeamon) {
				fmt.Println(" Run here!!! ")

				items := strings.Split(userCountID, "@")

				if items[1] == "" {
					items[1] = items[0]
				}
				clientID, err := storage.SetClient(&models.Client{
					Name:        items[1],
					UserID:      userID,
					ActiveSince: helper.GetTimeNow(),
					CreatedAt:   helper.GetTimeNow(),
				})

				fmt.Println("CLIENT ID", clientID)
				if err != nil {
					ctx.Writer.Write([]byte(err.Error()))
					return
				}
				token, err := ctr.TokenService.CreateToken(
					fmt.Sprint(userID),
					fmt.Sprint(clientID),
					fmt.Sprint(models.ClientUser),
				)

				if err != nil {
					ctx.Writer.Write([]byte(err.Error()))
					return
				}

				ctr.pubSubCh <- &pubsub.PubSubPayload{
					ClientID:    clientID,
					UserID:      userID,
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
