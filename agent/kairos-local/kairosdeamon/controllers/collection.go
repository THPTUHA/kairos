package controllers

import (
	"net/http"

	"github.com/THPTUHA/kairos/agent"
	"github.com/THPTUHA/kairos/agent/kairos-local/kairosdeamon/storage"
	"github.com/gin-gonic/gin"
)

func ListCollection(c *gin.Context) {
	var options agent.CollectionOptions
	err := c.BindJSON(&options)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	cs, err := storage.Store.GetCollections(&options)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"collections": cs,
		"message":     "list collection",
	})
}

func ListWorkflowCollection(c *gin.Context) {
	var options agent.WorkflowOptions
	err := c.BindJSON(&options)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	wfs, err := storage.Store.GetWorkflows(&options)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"workflows": wfs,
		"message":   "list workflow",
	})
}
