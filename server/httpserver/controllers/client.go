package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/THPTUHA/kairos/pkg/helper"
	"github.com/THPTUHA/kairos/server/messaging"
	"github.com/THPTUHA/kairos/server/storage"
	"github.com/THPTUHA/kairos/server/storage/models"
	"github.com/gin-gonic/gin"
)

type UserInfo struct {
	ID     string `json:"id"`
	Online bool   `json:"online"`
}

func (ctr *Controller) GetClients(c *gin.Context) {
	userID, _ := c.Get("userID")

	clients, err := storage.GetAllClient(userID.(string), nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}
	clientIDs := make([]string, 0)
	for _, c := range clients {
		clientIDs = append(clientIDs, fmt.Sprintf("kairosdeamon-%d@%s", c.ID, userID.(string)))
	}
	data, _ := json.Marshal(clientIDs)
	msg, err := ctr.nats.Request(messaging.CLIENT_STATUS, data, 3*time.Second)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}
	uis := make([]*UserInfo, 0)
	err = json.Unmarshal(msg.Data, &uis)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	for _, c := range clients {
		id := fmt.Sprintf("kairosdeamon-%d@%s", c.ID, userID.(string))
		for _, u := range uis {
			if id == u.ID {
				if u.Online {
					c.Status = 1
				}
				break
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "All client",
		"clients": clients,
	})
}

func (ctr *Controller) CreateClient(c *gin.Context) {
	userID, _ := c.Get("userID")
	var client models.Client
	err := c.BindJSON(&client)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}
	uid, _ := strconv.ParseInt(userID.(string), 10, 64)

	client.CreatedAt = helper.GetTimeNow()
	client.UserID = uid
	id, err := storage.CreateClient(&client)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Create client success",
		"client_id": id,
	})
}
