package controllers

import (
	"fmt"
	"net/http"

	"github.com/THPTUHA/kairos/agent"
	"github.com/THPTUHA/kairos/agent/kairos-local/kairosdeamon/storage"
	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/gin-gonic/gin"
)

func ApplyYaml(c *gin.Context) {
	var collection workflow.Collection
	err := c.BindJSON(&collection)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	err = storage.Store.SetCollection(&collection)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// TODO check liệu kv storage có lưu trong disk ko
	cs, err := storage.Store.GetCollections(&agent.CollectionOptions{Key: "abc"})
	fmt.Printf("%+v\n", cs[0].Workflows)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "apply success",
	})
}
