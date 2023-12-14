package logger

import (
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

var ginOnce sync.Once

func InitLogger(logLevel string, node string) *logrus.Entry {
	formattedLogger := logrus.New()
	formattedLogger.Formatter = &logrus.TextFormatter{FullTimestamp: true}

	level, err := logrus.ParseLevel(logLevel)
	if err != nil {
		logrus.WithError(err).Error("Error parsing log level, using: info")
		level = logrus.InfoLevel
	}

	formattedLogger.Level = level
	formattedLogger.SetReportCaller(true)
	formattedLogger.Formatter = &logrus.TextFormatter{
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			repopath := fmt.Sprintf("%s/src/github.com/bob", os.Getenv("GOPATH"))
			filename := strings.Replace(f.File, repopath, "", -1)
			return fmt.Sprintf("%s()", f.Function), fmt.Sprintf("%s:%d", filename, f.Line)
		},
	}
	log := logrus.NewEntry(formattedLogger).WithField("node", node)
	ginOnce.Do(func() {
		if level == logrus.DebugLevel {
			gin.DefaultWriter = log.Writer()
			gin.SetMode(gin.DebugMode)
		} else {
			gin.DefaultWriter = ioutil.Discard
			gin.SetMode(gin.ReleaseMode)
		}
	})

	return log
}
