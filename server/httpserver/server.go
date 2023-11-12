package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/THPTUHA/kairos/pkg/logger"
	"github.com/THPTUHA/kairos/server/httpserver/auth"
	"github.com/THPTUHA/kairos/server/httpserver/config"
	"github.com/THPTUHA/kairos/server/httpserver/events"
	"github.com/THPTUHA/kairos/server/httpserver/pubsub"
	"github.com/THPTUHA/kairos/server/httpserver/routes"
	"github.com/THPTUHA/kairos/server/httpserver/runner"
	"github.com/THPTUHA/kairos/server/storage"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/sirupsen/logrus"
)

type HttpServer struct {
	Router     *gin.Engine
	Token      auth.TokenInterface
	Config     *config.Configs
	PubsubChan chan *pubsub.PubSubPayload

	Logger *logrus.Entry
}

func (server *HttpServer) initialize(config *config.Configs) {
	server.Config = config
	server.Router = routes.Build(server.PubsubChan)
	server.Logger = logger.InitLogger("debug", "httpserver")
}

func (server *HttpServer) start() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals,
		os.Interrupt,
	)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", server.Config.HTTPServer.Port),
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

	worklowRunner := runner.NewRunner(runner.Configs{
		MaxWorkflowConcurrent: server.Config.HTTPServer.MaxWorkflowConcurrent,
		Logger:                server.Logger,
	})

	go func() {
		worklowRunner.Start(signals)
	}()

	log.Info().Msg("Starting the server...")
	if err := server.Router.Run(fmt.Sprintf(":%d", server.Config.HTTPServer.Port)); err != nil {
		log.Error().Err(err).Msg("Server is not running!")
	}
}

func main() {
	config, err := config.Set("httpserver.yaml")
	if err != nil {
		log.Error().Msg(err.Error())
		return
	}
	events.Init()
	storage.Connect(fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		config.DB.Postgres.URI,
		config.DB.Postgres.Port,
		config.DB.Postgres.Username,
		config.DB.Postgres.Password,
		config.DB.Postgres.DatabaseName,
	))

	auth.Init(config.Auth.HmacSecret, config.Auth.HmrfSecret)
	if err != nil {
		return
	}
	httpserver := HttpServer{}

	pubsubChan := make(chan *pubsub.PubSubPayload)
	httpserver.PubsubChan = pubsubChan

	go pubsub.Start(config, pubsubChan)
	httpserver.initialize(config)
	httpserver.start()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	<-signalChan
}
