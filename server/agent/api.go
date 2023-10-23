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
	v1.GET("/jobs", h.jobsHandler)
	v1.POST("/jobs", h.jobCreateOrUpdateHandler)
}

func (h *HTTPTransport) jobCreateOrUpdateHandler(c *gin.Context) {
	// Init the Job object with defaults
	job := Job{
		Concurrency: ConcurrencyAllow,
	}

	// Parse values from JSON
	if err := c.BindJSON(&job); err != nil {
		_, _ = c.Writer.WriteString(fmt.Sprintf("Unable to parse payload: %s.", err))
		h.logger.Error(err)
		return
	}

	// Validate job
	if err := job.Validate(); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		_, _ = c.Writer.WriteString(fmt.Sprintf("Job contains invalid value: %s.", err))
		return
	}

	// Call gRPC SetJob
	if err := h.agent.GRPCClient.SetJob(&job); err != nil {
		s := status.Convert(err)
		if s.Message() == ErrParentJobNotFound.Error() {
			c.AbortWithStatus(http.StatusNotFound)
		} else {
			c.AbortWithStatus(http.StatusInternalServerError)
		}
		_, _ = c.Writer.WriteString(s.Message())
		return
	}

	// Immediately run the job if so requested
	// if _, exists := c.GetQuery("runoncreate"); exists {
	// 	go func() {
	// 		if _, err := h.agent.GRPCClient.RunJob(job.Name); err != nil {
	// 			h.logger.WithError(err).Error("api: Unable to run job.")
	// 		}
	// 	}()
	// }

	c.Header("Location", fmt.Sprintf("%s/%s", c.Request.RequestURI, job.Name))
	renderJSON(c, http.StatusCreated, &job)
}

func (h *HTTPTransport) jobsHandler(c *gin.Context) {
	metadata := c.QueryMap("metadata")
	sort := c.DefaultQuery("_sort", "id")
	if sort == "id" {
		sort = "name"
	}
	order := c.DefaultQuery("_order", "ASC")
	q := c.Query("q")

	jobs, err := h.agent.Store.GetJobs(
		&JobOptions{
			Metadata: metadata,
			Sort:     sort,
			Order:    order,
			Query:    q,
			Status:   c.Query("status"),
			Disabled: c.Query("disabled"),
		},
	)
	if err != nil {
		h.logger.WithError(err).Error("api: Unable to get jobs, store not reachable.")
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
		e = len(jobs)
	} else {
		e, _ = strconv.Atoi(end)
		if e > len(jobs) {
			e = len(jobs)
		}
	}

	c.Header("X-Total-Count", strconv.Itoa(len(jobs)))
	renderJSON(c, http.StatusOK, jobs[s:e])
}

func renderJSON(c *gin.Context, status int, v interface{}) {
	if _, ok := c.GetQuery(pretty); ok {
		c.IndentedJSON(status, v)
	} else {
		c.JSON(status, v)
	}
}
