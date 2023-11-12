package controllers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"

	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/THPTUHA/kairos/server/httpserver/events"
	"github.com/THPTUHA/kairos/server/storage"
	"github.com/gin-gonic/gin"
)

func CreateWorkflow(c *gin.Context) {
	userID, _ := c.Get("userID")
	var workflowFile workflow.WorkflowFile
	err := c.BindJSON(&workflowFile)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	if workflowFile.Tasks.Len() == 0 && workflowFile.Brokers.Len() == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Must task or borker",
		})
		return
	}

	if workflowFile.Namespace == "" {
		workflowFile.Namespace = "default"
	}

	vars := workflow.GetKeyValueVars(workflowFile.Vars)
	workflowFile.Tasks.Range(func(key string, value *workflow.Task) error {
		value.Complie(vars)
		return nil
	})

	err = workflowFile.Tasks.Range(func(key string, value *workflow.Task) error {
		for _, c := range value.Clients {
			_, err := storage.GetClient(&storage.ClientQuery{
				UserID: userID.(string),
				Name:   c,
			})

			if err != nil {
				if err == sql.ErrNoRows {
					return fmt.Errorf("Client %s not exist", c)
				}
				return err
			}
		}
		return nil
	})

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	wid, err := storage.CreateWorkflow(userID.(string), &workflowFile)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Create workflow success",
	})

	wf, _ := storage.DetailWorkflow(wid)
	err = wf.Tasks.Range(func(key string, value *workflow.Task) error {
		value.Domains = map[string]string{}
		for _, c := range value.Clients {
			client, err := storage.GetClient(&storage.ClientQuery{
				UserID: userID.(string),
				Name:   c,
			})

			if err != nil {
				if err == sql.ErrNoRows {
					return fmt.Errorf("Client %s not exist", c)
				}
				return err
			}
			value.Domains[c] = client.KairosName
		}
		return nil
	})
	events.WfChan <- &events.WfEvent{
		Cmd:      events.WfCmdCreate,
		Workflow: wf,
	}
}

func DropWorkflow(c *gin.Context) {
	userID, _ := c.Get("userID")
	id, exist := c.Params.Get("id")

	if !exist {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Empty id",
		})
		return
	}

	wid, err := storage.DropWorkflow(userID.(string), id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}
	var wf workflow.Workflow
	wf.ID = wid

	c.JSON(http.StatusOK, gin.H{
		"message": "Droping workflow",
	})
	events.WfChan <- &events.WfEvent{
		Cmd:      events.WfCmdDelete,
		Workflow: &wf,
	}
}

func ListWorkflow(c *gin.Context) {
	userID, _ := c.Get("userID")
	wfs, err := storage.ListWorkflow(userID.(string), nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "List workflow",
		"workflows": wfs,
	})
}

func StopWorkflow(c *gin.Context) {
	id, exist := c.Params.Get("id")

	if !exist {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Empty id",
		})
		return
	}
	var wf workflow.Workflow
	wid, err := strconv.ParseInt(id, 10, 64)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	wf.ID = wid

	c.JSON(http.StatusOK, gin.H{
		"message": "Droping workflow",
	})

	events.WfChan <- &events.WfEvent{
		Cmd:      events.WfCmdDelete,
		Workflow: &wf,
	}
}

func StartWorkflow(c *gin.Context) {
	id, exist := c.Params.Get("id")

	if !exist {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Empty id",
		})
		return
	}

	var wf workflow.Workflow
	wid, err := strconv.ParseInt(id, 10, 64)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}
	wf.ID = wid

	c.JSON(http.StatusOK, gin.H{
		"message": "Starting workflow",
	})

	events.WfChan <- &events.WfEvent{
		Cmd:      events.WfCmdStart,
		Workflow: &wf,
	}
}

func DetailWorkflow(c *gin.Context) {
	id, exist := c.Params.Get("id")

	if !exist {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Empty id",
		})
		return
	}

	wid, err := strconv.ParseInt(id, 10, 64)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}
	wf, err := storage.DetailWorkflowModel(wid)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Detail workflow",
		"workflow": wf,
	})

}
