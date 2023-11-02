package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/THPTUHA/kairos/agent/kairos-local/kairosdeamon/config"
	"github.com/THPTUHA/kairos/agent/kairos-local/kairosdeamon/routers"
	"github.com/THPTUHA/kairos/agent/kairos-local/kairosdeamon/storage"
	"github.com/THPTUHA/kairos/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/sirupsen/logrus"
)

type DeamonServer struct {
	Router *gin.Engine
	Logger *logrus.Entry
}

func (server *DeamonServer) initialize() error {
	server.Router = routers.Setup()
	server.Logger = logger.InitLogger(logrus.DebugLevel.String(), "local")

	return nil
}

func (server *DeamonServer) start() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals,
		os.Interrupt,
	)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.ServerPort),
		Handler: server.Router,
	}

	go func() {
		<-signals
		log.Warn().Msg("Shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := srv.Shutdown(ctx)
		if err != nil {
			log.Error().Err(err).Send()
		}
		os.Exit(0)
	}()

	log.Info().Msg("Starting the deamon...")
	if err := server.Router.Run(srv.Addr); err != nil {
		log.Error().Err(err).Msg("Deamon is not running!")
	}
}

var Deamon DeamonServer

func Start() {
	Deamon = DeamonServer{}
	err := Deamon.initialize()
	storage.Init(Deamon.Logger)

	if err != nil {
		Deamon.Logger.Error(err.Error())
	}
	Deamon.start()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	<-signalChan
}
