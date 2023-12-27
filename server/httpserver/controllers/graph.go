package controllers

import (
	"net/http"
	"strconv"

	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/THPTUHA/kairos/server/storage"
	"github.com/THPTUHA/kairos/server/storage/models"
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

func (ctr *Controller) GetTimeLine(c *gin.Context) {
	userID, _ := c.Get("userID")
	uid, err := strconv.ParseInt(userID.(string), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	mf, err := storage.GetMessageFlowsByUserID(uid)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "graph",
		"data":    mf,
	})
}

func (ctr *Controller) GetGroupID(c *gin.Context) {
	groupID := c.Query("group")

	mf, err := storage.GetMessageFlowsByGroupID(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "graph",
		"data":    mf,
	})
}

type body struct {
	Parts   []any `json:"parts"`
	Parents []any `json:"parents"`
}

func (ctr *Controller) DetailPoint(c *gin.Context) {
	var ps body
	err := c.BindJSON(&ps)
	inputs := make([]*models.MessageFlow, 0)
	outputs := make([]*models.MessageFlow, 0)
	if len(ps.Parts) > 0 {
		inputs, err = storage.GetMessageFlowsByPart(ps.Parts)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"err": err.Error(),
			})
			return
		}
	}
	if len(ps.Parents) > 0 {
		outputs, err = storage.GetMessageFlowsByParent(ps.Parents)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"err": err.Error(),
			})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "graph",
		"inputs":  inputs,
		"outputs": outputs,
	})
}
