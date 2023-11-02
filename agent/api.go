package agent

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/buntdb"
	"google.golang.org/grpc/status"
)

const (
	pretty        = "pretty"
	apiPathPrefix = "v1"
)

// Transport is the interface that wraps the ServeHTTP method.
type Transport interface {
	ServeHTTP()
}

// HTTPTransport stores pointers to an agent and a gin Engine.
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
	v1.POST("/tasks", h.taskCreateOrUpdateHandler)
	v1.GET("/view/:task/executions", h.executionsHandler)
}

func (h *HTTPTransport) taskCreateOrUpdateHandler(c *gin.Context) {
	// Init the Task object with defaults
	task := Task{}

	// Parse values from JSON
	if err := c.BindJSON(&task); err != nil {
		_, _ = c.Writer.WriteString(fmt.Sprintf("Unable to parse payload: %s.", err))
		h.logger.Error(err)
		return
	}

	// Validate task
	if err := task.Validate(); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		_, _ = c.Writer.WriteString(fmt.Sprintf("Task contains invalid value: %s.", err))
		return
	}

	// Call gRPC SetTask
	if err := h.agent.GRPCClient.SetTask(&task); err != nil {
		s := status.Convert(err)
		c.AbortWithStatus(http.StatusInternalServerError)
		_, _ = c.Writer.WriteString(s.Message())
		return
	}

	// Immediately run the task if so requested
	// if _, exists := c.GetQuery("runoncreate"); exists {
	// 	go func() {
	// 		if _, err := h.agent.GRPCClient.RunTask(task.Key); err != nil {
	// 			h.logger.WithError(err).Error("api: Unable to run task.")
	// 		}
	// 	}()
	// }

	c.Header("Location", fmt.Sprintf("%s/%s", c.Request.RequestURI, task.Key))
	renderJSON(c, http.StatusCreated, &task)
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
	taskID, err := strconv.Atoi(c.Param("task"))

	if err != nil {
		_ = c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	sort := c.DefaultQuery("_sort", "")
	if sort == "id" {
		sort = "started_at"
	}
	order := c.DefaultQuery("_order", "DESC")
	outputSizeLimit, err := strconv.Atoi(c.DefaultQuery("output_size_limit", ""))
	if err != nil {
		outputSizeLimit = -1
	}

	job, err := h.agent.Store.GetTask(int64(taskID), nil)
	fmt.Printf("TaskID = %d", taskID)
	if err != nil {
		_ = c.AbortWithError(http.StatusNotFound, err)
		return
	}

	executions, err := h.agent.Store.GetExecutions(job.ID,
		&ExecutionOptions{
			Sort:     sort,
			Order:    order,
			Timezone: job.GetTimeLocation(),
		},
	)
	if err == buntdb.ErrNotFound {
		executions = make([]*Execution, 0)
	} else if err != nil {
		h.logger.Error(err)
		return
	}

	apiExecutions := make([]*apiExecution, len(executions))
	for j, execution := range executions {
		apiExecutions[j] = &apiExecution{execution, false}
		if outputSizeLimit > -1 {
			// truncate execution output
			size := len(execution.Output)
			if size > outputSizeLimit {
				apiExecutions[j].Output = apiExecutions[j].Output[size-outputSizeLimit:]
				apiExecutions[j].OutputTruncated = true
			}
		}
	}

	c.Header("X-Total-Count", strconv.Itoa(len(executions)))
	renderJSON(c, http.StatusOK, apiExecutions)
}
