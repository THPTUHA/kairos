package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/config"
	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/events"
	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/pubsub"
	"github.com/THPTUHA/kairos/pkg/utils"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

type LoginResponse struct {
	AccessToken string `json:"access_token"`
	UserCountID string `json:"user_count_id"`
}

type MessageResponse struct {
	UserCountID string `json:"user_count_id"`
}

// TODO lưu lại kairos name client
func Login(eventCh chan *events.Event) func(c *gin.Context) {
	return func(c *gin.Context) {
		clientName := c.Query("name")
		var wait sync.WaitGroup
		wait.Add(1)
		clientPS := pubsub.NewClient(fmt.Sprintf("%s?name=%s", config.ServerPubSubEnpoint, clientName), pubsub.Config{})
		clientPS.OnConnected(func(e pubsub.ConnectedEvent) {
		})

		clientPS.OnMessage(func(e pubsub.MessageEvent) {
			log.Printf("Message from server: %s", string(e.Data))
			var mr *MessageResponse
			err := json.Unmarshal(e.Data, &mr)
			if err != nil {
				return
			}
			log.Info().Msg(fmt.Sprintf("UserCountID %s", mr.UserCountID))

			sub, err := clientPS.NewSubscription(mr.UserCountID, pubsub.SubscriptionConfig{
				Recoverable: true,
				JoinLeave:   true,
			})

			if err != nil {
				wait.Done()
				c.JSON(http.StatusBadRequest, gin.H{
					"err": err.Error(),
				})
				return
			}

			sub.OnSubscribed(func(e pubsub.SubscribedEvent) {
				log.Printf("Subscribed on channel %s, (was recovering: %v, recovered: %v)", sub.Channel, e.WasRecovering, e.Recovered)
				utils.OpenBrowser(fmt.Sprintf("%s?user_count_id=%s", config.LoginEndpoint, mr.UserCountID))
			})

			sub.OnPublication(func(e pubsub.PublicationEvent) {
				var lr *LoginResponse
				err := json.Unmarshal(e.Data, &lr)
				if err != nil {
					log.Error().Stack().Err(err).Msg("OnPublication json Unmarshal")
					return
				}

				// lưu kairosname + nodenmae
				u := lr.UserCountID
				items := strings.Split(u, "@")

				if items[1] == "" {
					items[1] = items[0]
				}

				config.KairosName = items[0]
				config.ClientName = items[1]
				config.Token = lr.AccessToken

				log.Info().Msg(fmt.Sprintf("LoginResponse = %s, KariosName = %s, ClientName =%s\n",
					lr.AccessToken, config.KairosName, config.ClientName))

				wait.Done()
			})

			err = sub.Subscribe()
			if err != nil {
				wait.Done()
				c.JSON(http.StatusBadRequest, gin.H{
					"err": err.Error(),
				})
				return
			}
		})

		err := clientPS.Connect()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"err": err.Error(),
			})
			return
		}
		wait.Wait()
		clientPS.Disconnect()
		eventCh <- &events.Event{
			Cmd: events.ConnectServerCmd,
		}
		c.JSON(http.StatusOK, gin.H{
			"message": "login success",
		})
	}
}
