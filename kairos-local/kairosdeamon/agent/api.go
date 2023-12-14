package agent

import (
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/status"
)

const (
	pretty = "pretty"
)

type Transport interface {
	ServeHTTP()
}

type HTTPTransport struct {
	Engine *gin.Engine

	agent  *Agent
	logger *logrus.Entry
}

// NewTransport creates an HTTPTransport with a bound agent.
func NewTransport(a *Agent, log *logrus.Entry) *HTTPTransport {
	return &HTTPTransport{
		agent:  a,
		logger: log,
	}
}

// TODO
func (h *HTTPTransport) ServeHTTP() {
	h.Engine = gin.Default()
	rootPath := h.Engine.Group("/")
	h.APIRoutes(rootPath)

	go func() {
		if err := h.Engine.Run(h.agent.config.HTTPAddr); err != nil {
			h.logger.WithError(err).Error("api: Error starting HTTP server")
		}
	}()
}

func (h *HTTPTransport) APIRoutes(r *gin.RouterGroup, middleware ...gin.HandlerFunc) {
	v1 := r.Group("/v1")
	v1.POST("/tasks/list", h.tasksHandler)
	v1.GET("/:task/task", h.tasksGetHandler)
	v1.DELETE("/task/:task/delete", h.tasksDeleteHandler)
	v1.DELETE("/broker/:broker/delete", h.brokerDeleteHanddler)
	v1.DELETE("/workflow/:workflow/delete", h.workflowDeleteHandler)

	v1.GET("/workflow/:workflow/detail", h.workflowDetailHandler)

	v1.POST("/runtask", h.runTask)
	v1.POST("/tasks", h.taskCreateOrUpdateHandler)
	v1.GET("/view/:task/executions", h.executionsHandler)
	v1.POST("/queue", h.queueTest)
	v1.GET("/:task/queue", h.queueGetTest)
	v1.POST("/broker/record", h.brokerRecordHandler)

	v1.POST("/script/:name/upload", h.uploadScript)
	v1.GET("/script/:name/show", h.showScript)
	v1.GET("/script/list", h.listScript)
	v1.DELETE("/script/:name/drop", h.dropScript)

	v1.GET("/plugin/list", h.listPlugin)
	v1.DELETE("/plugin/:name/delete", h.deletePlugin)
	v1.POST("/plugin/add", h.addPlugin)
}

type Queue struct {
	TaskID string `json:"task_id"`
	V      string
	K      string
}

func (h *HTTPTransport) queueTest(c *gin.Context) {
	q := Queue{}
	if err := c.BindJSON(&q); err != nil {
		_, _ = c.Writer.WriteString(fmt.Sprintf("Unable to parse payload: %s.", err))
		h.logger.Error(err)
		return
	}
	err := h.agent.Store.SetQueue(q.TaskID, q.K, q.V)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		_, _ = c.Writer.WriteString(fmt.Sprintf("Queue contains invalid value: %s.", err))
		return
	}
	renderJSON(c, http.StatusCreated, &q)
}

func (h *HTTPTransport) uploadScript(c *gin.Context) {
	name := c.Param("name")
	multipartReader, err := c.Request.MultipartReader()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for {
		part, err := multipartReader.NextPart()
		if err != nil {
			break
		}

		data, err := io.ReadAll(part)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		err = h.agent.SetupScript(name, string(data))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		fmt.Println("Read data:", string(data))
	}

	c.JSON(http.StatusOK, gin.H{"message": "Data read successfully."})
}

func (h *HTTPTransport) showScript(c *gin.Context) {
	name := c.Param("name")
	s, err := h.agent.Store.GetScript(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, s)
}

func (h *HTTPTransport) listScript(c *gin.Context) {
	s, err := h.agent.Store.ListScript()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, s)
}

func (h *HTTPTransport) listPlugin(c *gin.Context) {
	p := h.agent.ListPlugin()
	c.JSON(http.StatusOK, p)
}

type Plugin struct {
	Name string   `json:"name"`
	Cmd  string   `json:"cmd"`
	Args []string `json:"args"`
}

func (h *HTTPTransport) addPlugin(c *gin.Context) {
	var p Plugin
	if err := c.BindJSON(&p); err != nil {
		_, _ = c.Writer.WriteString(fmt.Sprintf("Unable to parse payload: %s.", err))
		h.logger.Error(err)
		return
	}
	e := h.agent.addPlugin(p.Name, p.Name, p.Args)
	c.JSON(http.StatusOK, e)
}

func (h *HTTPTransport) deletePlugin(c *gin.Context) {
	name := c.Param("name")
	p := h.agent.DeletePlugin(name)
	c.JSON(http.StatusOK, p)
}

func (h *HTTPTransport) dropScript(c *gin.Context) {
	name := c.Param("name")
	err := h.agent.dropScript(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, "Delete script successfully")
}

func (h *HTTPTransport) queueGetTest(c *gin.Context) {
	taskID := c.Param("task")
	m := make(map[string]string)
	err := h.agent.Store.GetQueue(taskID, &m)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		_, _ = c.Writer.WriteString(fmt.Sprintf("Queue contains invalid value: %s.", err))
		return
	}

	renderJSON(c, http.StatusCreated, m)
}

func (h *HTTPTransport) runTask(c *gin.Context) {
	cmdtask := workflow.CmdTask{}
	if err := c.BindJSON(&cmdtask); err != nil {
		_, _ = c.Writer.WriteString(fmt.Sprintf("Unable to parse payload: %s.", err))
		h.logger.Error(err)
		return
	}
	fmt.Println("GO TO HERE----")
	t, _ := h.agent.GetTask(fmt.Sprint(cmdtask.Task.ID))
	fmt.Printf("TASK CALL %+v\n", *t)
	ex := NewExecution(t.ID)
	h.agent.Run(t, ex, &cmdtask)
}

func (h *HTTPTransport) taskCreateOrUpdateHandler(c *gin.Context) {
	task := Task{}

	if err := c.BindJSON(&task); err != nil {
		_, _ = c.Writer.WriteString(fmt.Sprintf("Unable to parse payload: %s.", err))
		h.logger.Error(err)
		return
	}

	fmt.Printf("CREATE TASK %+v\n", task)
	if err := task.Validate(); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		_, _ = c.Writer.WriteString(fmt.Sprintf("Task contains invalid value: %s.", err))
		return
	}

	if task.Schedule == "" {
		if err := h.agent.SetTaskAndRun(&task); err != nil {
			s := status.Convert(err)
			c.AbortWithStatus(http.StatusInternalServerError)
			_, _ = c.Writer.WriteString(s.Message())
			return
		}
	} else {
		if err := h.agent.SetTaskAndSched(&task); err != nil {
			s := status.Convert(err)
			c.AbortWithStatus(http.StatusInternalServerError)
			_, _ = c.Writer.WriteString(s.Message())
			return
		}
	}

	c.Header("Location", fmt.Sprintf("%s/%s", c.Request.RequestURI, task.Name))
	renderJSON(c, http.StatusCreated, &task)
}

func (h *HTTPTransport) tasksDeleteHandler(c *gin.Context) {
	taskID := c.Param("task")
	err := h.agent.DeleteTask(taskID)
	if err != nil {
		renderJSON(c, http.StatusBadRequest, err.Error())
		return
	}
	renderJSON(c, http.StatusOK, nil)
}

func (h *HTTPTransport) brokerDeleteHanddler(c *gin.Context) {
	borkerID := c.Param("broker")
	err := h.agent.Store.DeleteBroker(borkerID)
	if err != nil {
		renderJSON(c, http.StatusBadRequest, err.Error())
		return
	}
	renderJSON(c, http.StatusOK, nil)
}

func (h *HTTPTransport) workflowDeleteHandler(c *gin.Context) {
	workflowID := c.Param("workflow")
	id, err := strconv.ParseInt(workflowID, 10, 64)
	if err != nil {
		renderJSON(c, http.StatusBadRequest, err.Error())
		return
	}
	err = h.agent.DeleteWorkflow(id)
	if err != nil {
		renderJSON(c, http.StatusBadRequest, err.Error())
		return
	}
	renderJSON(c, http.StatusOK, nil)
}

func (h *HTTPTransport) workflowDetailHandler(c *gin.Context) {
	workflowID := c.Param("workflow")
	wfi, err := h.agent.Store.GetWorkflow(workflowID)
	if err != nil {
		renderJSON(c, http.StatusBadRequest, err.Error())
		return
	}
	renderJSON(c, http.StatusOK, wfi)
}

func (h *HTTPTransport) brokerRecordHandler(c *gin.Context) {
	var bro BrOptions
	if err := c.BindJSON(&bro); err != nil {
		_, _ = c.Writer.WriteString(fmt.Sprintf("Unable to parse payload: %s.", err))
		h.logger.Error(err)
		return
	}

	bros, err := h.agent.Store.GetBrokerRecord(&bro)
	if err != nil {
		renderJSON(c, http.StatusBadRequest, err.Error())
		return
	}

	renderJSON(c, http.StatusOK, bros)
}

func (h *HTTPTransport) tasksGetHandler(c *gin.Context) {
	taskID := c.Param("task")
	task, err := h.agent.GetTask(taskID)
	if err != nil {
		renderJSON(c, http.StatusBadRequest, err.Error())
	}
	renderJSON(c, http.StatusOK, task)
}

func (h *HTTPTransport) tasksHandler(c *gin.Context) {
	var to TaskOptions
	if err := c.BindJSON(&to); err != nil {
		_, _ = c.Writer.WriteString(fmt.Sprintf("Unable to parse payload: %s.", err))
		h.logger.Error(err)
		return
	}

	tasks, err := h.agent.Store.GetTasks(&to)
	if err != nil {
		_, _ = c.Writer.WriteString(fmt.Sprintf("Unable to get tasks: %s.", err))
		return
	}

	start, ok := c.GetQuery("_start")
	if !ok {
		start = "0"
	}
	s, _ := strconv.Atoi(start)

	end, ok := c.GetQuery("_end")
	e := 0
	if !ok {
		e = len(tasks)
	} else {
		e, _ = strconv.Atoi(end)
		if e > len(tasks) {
			e = len(tasks)
		}
	}

	c.Header("X-Total-Count", strconv.Itoa(len(tasks)))
	renderJSON(c, http.StatusOK, tasks[s:e])
}

func renderJSON(c *gin.Context, status int, v interface{}) {
	if _, ok := c.GetQuery(pretty); ok {
		c.IndentedJSON(status, v)
	} else {
		c.JSON(status, v)
	}
}

type apiExecution struct {
	*Execution
	OutputTruncated bool `json:"output_truncated"`
}

func (h *HTTPTransport) executionsHandler(c *gin.Context) {
	taskID := c.Param("task")

	sort := c.DefaultQuery("_sort", "")
	if sort == "id" {
		sort = "started_at"
	}
	order := c.DefaultQuery("_order", "DESC")
	executions, err := h.agent.Store.GetExecutions(taskID,
		&ExecutionOptions{
			Sort:  sort,
			Order: order,
		},
	)
	if err == ErrNotFound {
		executions = make([]*Execution, 0)
	} else if err != nil {
		h.logger.Error(err)
		return
	}

	apiExecutions := make([]*apiExecution, len(executions))
	for j, execution := range executions {
		apiExecutions[j] = &apiExecution{execution, false}
	}

	c.Header("X-Total-Count", strconv.Itoa(len(executions)))
	renderJSON(c, http.StatusOK, apiExecutions)
}
