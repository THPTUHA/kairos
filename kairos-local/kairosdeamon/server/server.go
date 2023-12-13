package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/agent"
	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/config"
	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/events"
	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/routers"
	"github.com/THPTUHA/kairos/pkg/logger"
	"github.com/THPTUHA/kairos/server/plugin"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/sirupsen/logrus"
)

type DeamonServer struct {
	router  *gin.Engine
	logger  *logrus.Entry
	eventCh chan *events.Event
	config  *config.Configs
}

func (server *DeamonServer) initialize() error {
	server.eventCh = make(chan *events.Event)
	server.router = routers.Setup(server.eventCh)
	server.logger = logger.InitLogger(server.config.LogLevel, server.config.NodeName)
	return nil
}

func (server *DeamonServer) start() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals,
		os.Interrupt,
	)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", server.config.ServerPort),
		Handler: server.router,
	}

	go func() {
		<-signals
		log.Warn().Msg("Shutting down...")
		plugin.CleanupClients()
		log.Info().Msg("Plugin clean finish...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := srv.Shutdown(ctx)
		if err != nil {
			log.Error().Err(err).Send()
		}
		os.Exit(0)
	}()

	go func() {
		agent.AgentStart(server.eventCh, server.config)
	}()

	log.Info().Msg("Starting the deamon...")
	if err := server.router.Run(srv.Addr); err != nil {
		log.Error().Err(err).Msg("Deamon is not running!")
	}
}

var deamon DeamonServer

func Start(config *config.Configs) {
	deamon = DeamonServer{
		config: config,
	}
	err := deamon.initialize()

	if err != nil {
		deamon.logger.Error(err.Error())
	}
	deamon.start()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	<-signalChan
}
