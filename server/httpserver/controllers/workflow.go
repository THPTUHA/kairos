package controllers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/THPTUHA/kairos/pkg/orderedmap"
	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/THPTUHA/kairos/server/httpserver/events"
	"github.com/THPTUHA/kairos/server/storage"
	"github.com/gin-gonic/gin"
)

func (ctr *Controller) CreateWorkflow(c *gin.Context) {
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

	rawData, err := json.Marshal(&workflowFile)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	// standar
	tasks := orderedmap.New[string, *workflow.Task]()

	err = workflowFile.Tasks.Range(func(key string, value *workflow.Task) error {
		key = strings.ToLower(key)
		for idx, c := range value.Clients {
			c = strings.ToLower(c)
			value.Clients[idx] = c
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
		tasks.Set(key, value)
		return nil
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	workflowFile.Tasks = workflow.Tasks{
		OrderedMap: tasks,
	}

	vars := orderedmap.New[string, *workflow.Var]()
	workflowFile.Vars.Range(func(key string, value *workflow.Var) error {
		vars.Set(strings.ToLower(key), value)
		return nil
	})
	workflowFile.Vars = &workflow.Vars{
		OrderedMap: vars,
	}

	brokers := orderedmap.New[string, *workflow.Broker]()
	workflowFile.Brokers.Range(func(key string, value *workflow.Broker) error {
		for idx, v := range value.Listens {
			value.Listens[idx] = strings.ToLower(v)
		}
		brokers.Set(strings.ToLower(key), value)
		return nil
	})

	workflowFile.Brokers = workflow.Brokers{
		OrderedMap: brokers,
	}

	err = workflowFile.Compile()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"compile worlfow file error": err.Error(),
		})
		return
	}

	wid, err := storage.CreateWorkflow(userID.(string), &workflowFile, string(rawData))
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

	// wf.Vars.Range(func(key string, value *workflow.Var) error {
	// 	fmt.Println(key, value)
	// 	return nil
	// })

	// wf.Tasks.Range(func(key string, value *workflow.Task) error {
	// 	fmt.Println(key)
	// 	fmt.Printf("%+v\n", value)
	// 	return nil
	// })

	wf.Compile()
	// fmt.Println("after comp")
	// wf.Tasks.Range(func(key string, value *workflow.Task) error {
	// 	fmt.Println(key)
	// 	fmt.Printf("%+v\n", value)
	// 	return nil
	// })

	events.Get() <- &events.WfEvent{
		Cmd:      events.WfCmdCreate,
		Workflow: wf,
	}
}

func (ctr *Controller) DropWorkflow(c *gin.Context) {
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
	events.Get() <- &events.WfEvent{
		Cmd:      events.WfCmdDelete,
		Workflow: &wf,
	}
}

func (ctr *Controller) ListWorkflow(c *gin.Context) {
	userID, _ := c.Get("userID")
	wfs, err := storage.ListWorkflow(userID.(string), nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}
	ctr.wfRunner.SetWfStatus(wfs)

	c.JSON(http.StatusOK, gin.H{
		"message":   "List workflow",
		"workflows": wfs,
	})
}

func (ctr *Controller) StopWorkflow(c *gin.Context) {
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

	events.Get() <- &events.WfEvent{
		Cmd:      events.WfCmdDelete,
		Workflow: &wf,
	}
}

func (ctr *Controller) StartWorkflow(c *gin.Context) {
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

	events.Get() <- &events.WfEvent{
		Cmd:      events.WfCmdStart,
		Workflow: &wf,
	}
}

func (ctr *Controller) DetailWorkflow(c *gin.Context) {
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
