package agent

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
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

}
