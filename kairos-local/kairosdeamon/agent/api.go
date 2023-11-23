package agent

import (
	"fmt"
	"net/http"
	"strconv"

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
	v1.GET("/tasks", h.tasksHandler)
	v1.GET("/:task/task", h.tasksGetHandler)
	v1.DELETE("/:task/delete", h.tasksDeleteHandler)
	v1.POST("/tasks", h.taskCreateOrUpdateHandler)
	v1.GET("/view/:task/executions", h.executionsHandler)
	v1.POST("/queue", h.queueTest)
	v1.GET("/:task/queue", h.queueGetTest)
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

func (h *HTTPTransport) taskCreateOrUpdateHandler(c *gin.Context) {
	task := Task{}

	if err := c.BindJSON(&task); err != nil {
		_, _ = c.Writer.WriteString(fmt.Sprintf("Unable to parse payload: %s.", err))
		h.logger.Error(err)
		return
	}

	if err := task.Validate(); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		_, _ = c.Writer.WriteString(fmt.Sprintf("Task contains invalid value: %s.", err))
		return
	}

	if err := h.agent.SetTaskAndSched(&task); err != nil {
		s := status.Convert(err)
		c.AbortWithStatus(http.StatusInternalServerError)
		_, _ = c.Writer.WriteString(s.Message())
		return
	}

	c.Header("Location", fmt.Sprintf("%s/%s", c.Request.RequestURI, task.Name))
	renderJSON(c, http.StatusCreated, &task)
}

func (h *HTTPTransport) tasksDeleteHandler(c *gin.Context) {
	taskID := c.Param("task")
	err := h.agent.DeleteTask(taskID)
	if err != nil {
		renderJSON(c, http.StatusBadRequest, err.Error())
	}
	renderJSON(c, http.StatusOK, nil)
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
	metadata := c.QueryMap("metadata")
	sort := c.DefaultQuery("_sort", "id")
	if sort == "id" {
		sort = "name"
	}
	order := c.DefaultQuery("_order", "ASC")
	q := c.Query("q")

	tasks, err := h.agent.Store.GetTasks(
		&TaskOptions{
			Metadata: metadata,
			Sort:     sort,
			Order:    order,
			Query:    q,
			Status:   c.Query("status"),
			Disabled: c.Query("disabled"),
		},
	)
	if err != nil {
		h.logger.WithError(err).Error("api: Unable to get tasks, store not reachable.")
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
