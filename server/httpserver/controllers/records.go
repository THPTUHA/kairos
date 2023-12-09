package controllers

import (
	"net/http"
	"strconv"

	"github.com/THPTUHA/kairos/server/storage"
	"github.com/gin-gonic/gin"
)

func (ctr *Controller) GetTaskRecord(c *gin.Context) {
	taskID, exist := c.Params.Get("task_id")
	if !exist {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Empty task id",
		})
		return
	}
	tid, err := strconv.ParseInt(taskID, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	trs, err := storage.GetTaskRecords(tid, 0, 10)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "list task record",
		"task_records": trs,
	})
}

func (ctr *Controller) GetClientRecord(c *gin.Context) {
	clientID, exist := c.Params.Get("client_id")
	if !exist {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Empty task id",
		})
		return
	}
	cid, err := strconv.ParseInt(clientID, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	crs, err := storage.GetTaskRecords(0, cid, 10)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "list client record",
		"client_records": crs,
	})
}

func (ctr *Controller) GetBrokerRecord(c *gin.Context) {
	brokerID, exist := c.Params.Get("broker_id")
	if !exist {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Empty task id",
		})
		return
	}
	bid, err := strconv.ParseInt(brokerID, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}
	brokers, err := storage.GetBrokers(&storage.BrokerQuery{
		ID: bid,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	if len(brokers) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"err": "Broker not found",
		})
		return
	}

	brs, err := storage.GetBrokerRecords(bid, 10)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "list broker record",
		"broker_records": brs,
		"broker":         brokers[0],
	})
}

type MessageFlowOptions struct {
	WorkflowName string `json:"workflow_name"`
}

func (ctr *Controller) GetMessageFlows(c *gin.Context) {
	userID, _ := c.Get("userID")
	uid, err := strconv.ParseInt(userID.(string), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	var mfo MessageFlowOptions
	err = c.BindJSON(&mfo)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	mfs, err := storage.GetMessageFlows(uid, mfo.WorkflowName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "list broker record",
		"msg_flows": mfs,
	})
}
