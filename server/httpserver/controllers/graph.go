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

func (ctr *Controller) GetGroupList(c *gin.Context) {
	groupID := c.Query("group")
	limit := c.Query("limit")

	mf, err := storage.GetGroupList(groupID, limit)
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
	Parts      []any  `json:"parts"`
	Parent     string `json:"parent"`
	ReceiverID int64  `json:"receiver_id"`
}

func (ctr *Controller) DetailPoint(c *gin.Context) {
	var ps body
	err := c.BindJSON(&ps)
	inputs := make([]*models.MessageFlow, 0)
	outputs := make([]*models.MessageFlow, 0)
	// phần từ đầu vào đến con
	inputs, err = storage.GetMessageFlowsByParent(ps.Parent, ps.ReceiverID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	outputs, err = storage.GetMessageFlowsByParts(ps.Parts)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "graph",
		"inputs":  inputs,
		"outputs": outputs,
	})
}
