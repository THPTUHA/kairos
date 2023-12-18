package controllers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/THPTUHA/kairos/pkg/orderedmap"
	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/THPTUHA/kairos/server/httpserver/events"
	"github.com/THPTUHA/kairos/server/messaging"
	"github.com/THPTUHA/kairos/server/storage"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
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
		c.JSON(http.StatusOK, gin.H{
			"err": "Must task or borker",
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

	clientM, err := storage.GetAllClient(userID.(string), nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	channelM, err := storage.GetChannels(&storage.ChannelOptions{UserID: userID.(string)})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}
	workflowFile.Channels = make([]*workflow.Channel, 0)
	for _, c := range channelM {
		workflowFile.Channels = append(workflowFile.Channels, &workflow.Channel{
			ID:   c.ID,
			Name: c.Name,
		})
	}

	workflowFile.Clients = make([]*workflow.Client, 0)
	for _, c := range clientM {
		workflowFile.Clients = append(workflowFile.Clients, &workflow.Client{
			ID:   c.ID,
			Name: c.Name,
		})
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
		c.JSON(http.StatusOK, gin.H{
			"err": err.Error(),
		})
		return
	}

	workflowFile.Tasks = workflow.Tasks{
		OrderedMap: tasks,
	}

	if workflowFile.Vars != nil {
		vars := orderedmap.New[string, *workflow.Var]()
		workflowFile.Vars.Range(func(key string, value *workflow.Var) error {
			vars.Set(strings.ToLower(key), value)
			return nil
		})
		workflowFile.Vars = &workflow.Vars{
			OrderedMap: vars,
		}

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
	uid, _ := strconv.ParseInt(userID.(string), 10, 64)
	fs, err := storage.FindFunctionsByUserID(uid)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"err": err.Error(),
		})
		return
	}
	scriptCode := ""
	for _, f := range fs {
		scriptCode += f.Content + "\n"
	}

	err = workflowFile.Compile(nil)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"err": err.Error(),
		})
		return
	}

	wid, err := storage.CreateWorkflow(userID.(string), &workflowFile, string(rawData))
	if err != nil {
		if wid != 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"err": err.Error(),
			})
		} else {
			c.JSON(http.StatusOK, gin.H{
				"err": err.Error(),
			})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Create workflow success",
	})

	wf, err := storage.DetailWorkflow(wid, userID.(string))
	if err != nil {
		log.Err(err)
		return
	}
	// wf.Vars.Range(func(key string, value *workflow.Var) error {
	// 	fmt.Println(key, value)
	// 	return nil
	// })

	// wf.Tasks.Range(func(key string, value *workflow.Task) error {
	// 	fmt.Println(key)
	// 	fmt.Printf("%+v\n", value)
	// 	return nil
	// })
	err = wf.Compile(nil)
	if err != nil {
		ctr.Log.Error(err)
		return
	}

	wf.Brokers.Range(func(_ string, b *workflow.Broker) error {
		fmt.Printf("BROKER CCC %+v\n", b)
		return nil
	})

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

	var wf workflow.Workflow
	wf.ID = wid

	events.Get() <- &events.WfEvent{
		Cmd:      events.WfCmdDelete,
		Workflow: &wf,
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Droping workflow",
	})
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
	userID, _ := c.Get("userID")
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
	wf, err := storage.DetailWorkflow(wid, userID.(string))
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

type Request struct {
	Cmd     int         `json:"cmd"`
	RunOn   string      `json:"run_on"`
	Content interface{} `json:"content"`
	SendAt  int64       `json:"send_at"`
}

func (ctr *Controller) RequestSyncWorkflow(c *gin.Context) {
	userID, exist := c.Get("userID")
	apiKey := c.GetHeader("api-key")

	if !exist || userID == "" || apiKey == "" {
		c.JSON(http.StatusUnauthorized, nil)
		return
	}

	var req Request
	err := c.BindJSON(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	data, _ := json.Marshal(req)
	result, err := ctr.nats.Request(fmt.Sprintf("%s-%s", messaging.REQUEST_WORKER_SYNC, userID), data, 10*time.Second)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	if result == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"result": string(result.Data),
	})
}

func (ctr *Controller) GetWfRecord(c *gin.Context) {
	id, exist := c.Params.Get("id")

	if !exist {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Empty id",
		})
		return
	}
	wid, _ := strconv.ParseInt(id, 10, 64)
	records, err := storage.GetWorkflowRecords(wid, 100)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"records": records,
	})
}

func (ctr *Controller) RecoverWorkflow(c *gin.Context) {
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

	var wf workflow.Workflow
	wf.ID = wid

	events.Get() <- &events.WfEvent{
		Cmd:      events.WfCmdRecover,
		Workflow: &wf,
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Recover workflow",
	})
}
