package controllers

import (
	"net/http"
	"strconv"

	"github.com/THPTUHA/kairos/pkg/helper"
	"github.com/THPTUHA/kairos/server/storage"
	"github.com/THPTUHA/kairos/server/storage/models"
	"github.com/gin-gonic/gin"
)

func (ctr *Controller) GetClients(c *gin.Context) {
	userID, _ := c.Get("userID")

	clients, err := storage.GetAllClient(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "All client",
		"clients": &clients,
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
