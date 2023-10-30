package controllers

import (
	"fmt"
	"net/http"

	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/gin-gonic/gin"
)

func ApplyYaml(c *gin.Context) {
	userID := c.GetString("user_id")
	fmt.Printf("userID = %s", userID)
	var collection workflow.Collection
	err := c.BindJSON(&collection)
	// get user...
	collection.Username = "abc"

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// save collection
	// notify to watcher scheduler job

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"collection_id": collection.ID,
		},
		"message": "apply success",
	})
}
