package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/THPTUHA/kairos/agent/kairos-local/kairosdeamon/config"
	"github.com/THPTUHA/kairos/agent/kairos-local/kairosdeamon/pubsub"
	"github.com/THPTUHA/kairos/pkg/utils"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

type LoginResponse struct {
	AccessToken string `json:"access_token"`
}

type MessageResponse struct {
	UserCountID string `json:"user_count_id"`
}

func Login(c *gin.Context) {
	var wait sync.WaitGroup
	wait.Add(1)
	clientPS := pubsub.NewClient(config.PubSubEndpoint, pubsub.Config{})
	clientPS.OnConnected(func(e pubsub.ConnectedEvent) {
		// sub, err := clientPS.NewSubscription(userID, pubsub.SubscriptionConfig{
		// 	Recoverable: true,
		// 	JoinLeave:   true,
		// })

		// sub.OnPublication(func(e pubsub.PublicationEvent) {
		// 	var pr *PublicationResponse
		// 	err := json.Unmarshal(e.Data, &pr)
		// 	if err != nil {
		// 		return
		// 	}
		// 	log.Info().Msg(fmt.Sprintf("UserCountID %s", pr.UserCountID))
		// 	clientPS.Disconnect()
		// })

		// err = sub.Subscribe()

		// if err != nil {
		// 	c.JSON(http.StatusBadRequest, gin.H{
		// 		"err": err.Error(),
		// 	})
		// 	clientPS.Disconnect()
		// 	return
		// }
		// utils.OpenBrowser(fmt.Sprintf("%s?user_count_id=%s", config.LoginEndpoint, userID))
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
			log.Info().Msg(fmt.Sprintf("LoginResponse %s", lr.AccessToken))
			// save token
			config.Token = lr.AccessToken
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
	c.JSON(http.StatusOK, gin.H{
		"message": "login success",
	})
}
