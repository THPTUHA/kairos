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
	UserID      string `json:"user_id"`
	ClientID    string `json:"client_id"`
}

type MessageResponse struct {
	UserCountID string `json:"user_count_id"`
}

func Login(eventCh chan *events.Event) func(c *gin.Context) {
	return func(c *gin.Context) {
		var auth config.Auth
		clientName := c.Query("name")
		var wait sync.WaitGroup
		wait.Add(1)
		cf, _ := config.Get()
		clientPS := pubsub.NewClient(fmt.Sprintf("%s?name=%s", cf.ServerPubSubEnpoint, clientName), pubsub.Config{})
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

			sub, err := clientPS.NewSubscription(mr.UserCountID, pubsub.SubscriptionConfig{})

			if err != nil {
				wait.Done()
				c.JSON(http.StatusBadRequest, gin.H{
					"err": err.Error(),
				})
				return
			}

			sub.OnSubscribed(func(e pubsub.SubscribedEvent) {
				log.Printf("Subscribed on channel %s: ", sub.Channel)
				utils.OpenBrowser(fmt.Sprintf("%s?user_count_id=%s", cf.LoginEndpoint, mr.UserCountID))
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

				auth.ClientName = items[1]
				auth.Token = lr.AccessToken
				auth.ClientID = lr.ClientID
				auth.UserID = lr.UserID
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
		// TODO add token, clientName, kairosName as meta data
		d, _ := json.Marshal(auth)
		eventCh <- &events.Event{
			Cmd:     events.ConnectServerCmd,
			Payload: string(d),
		}
		c.JSON(http.StatusOK, gin.H{
			"message": "login success",
		})
	}
}
