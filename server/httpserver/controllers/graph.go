package controllers

import (
	"net/http"

	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/THPTUHA/kairos/server/storage"
	"github.com/gin-gonic/gin"
)

type graphReq struct {
	WorkflowIDs []int64 `json:"workflow_ids"`
	Type        int     `json:"type"`
}

func (ctr *Controller) GetGraph(c *gin.Context) {
	userID, _ := c.Get("userID")
	var gR graphReq
	err := c.BindJSON(&gR)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}
	var wfs []*workflow.Workflow
	for _, id := range gR.WorkflowIDs {
		wf, err := storage.DetailWorkflow(id, userID.(string))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"err": err.Error(),
			})
			return
		}
		wfs = append(wfs, wf)
	}
	c.JSON(http.StatusOK, gin.H{
		"message":   "graph",
		"workflows": wfs,
	})
}

func (ctr *Controller) GraphData(c *gin.Context) {
	var gR graphReq
	err := c.BindJSON(&gR)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}
	edges, err := storage.PerformCalculation(gR.WorkflowIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "graph",
		"data":    edges,
	})
}
