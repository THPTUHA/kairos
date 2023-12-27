package controllers

import (
	"encoding/json"
	"errors"
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

func (ctr *Controller) GetChannels(c *gin.Context) {
	userID, _ := c.Get("userID")

	channels, err := storage.GetChannels(&storage.ChannelOptions{
		UserID: userID.(string),
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "get channels",
		"channels": &channels,
	})
}

func (ctr *Controller) AddChannel(c *gin.Context) {
	userID, _ := c.Get("userID")
	var channel models.Channel
	err := c.BindJSON(&channel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}
	cs, err := storage.GetChannels(&storage.ChannelOptions{
		UserID: userID.(string),
		Name:   channel.Name,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}
	if len(cs) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": errors.New(fmt.Sprintf("Channel name = %s existed", channel.Name)),
		})
		return
	}

	id, err := strconv.ParseInt(userID.(string), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	channel.UserID = id
	channel.CreatedAt = helper.GetTimeNow()
	id, err = storage.CreateChannel(&channel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "create channel success",
		"channel_id": id,
	})
}

func (ctr *Controller) DeleteChannel(c *gin.Context) {
	userID, _ := c.Get("userID")
	channelName, exist := c.Params.Get("channel")
	if !exist {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Empty channel params",
		})
		return
	}

	var ch models.Channel
	ch.Name = channelName
	uid, _ := strconv.ParseInt(userID.(string), 10, 64)
	ch.UserID = uid
	data, err := json.Marshal(ch)
	if err != nil {
		ctr.Log.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	_, err = ctr.nats.Request(messaging.REMOVE_CHANNEL, data, 3*time.Second)
	if err != nil {
		ctr.Log.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	err = storage.DeleteChannels(userID.(string), channelName)
	if err != nil {
		ctr.Log.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "delete channel successful",
	})
}
