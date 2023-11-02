package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/THPTUHA/kairos/server/httpserver/auth"
	"github.com/THPTUHA/kairos/server/httpserver/config"
	"github.com/THPTUHA/kairos/server/httpserver/pubsub"
	"github.com/THPTUHA/kairos/server/httpserver/routes"
	"github.com/THPTUHA/kairos/server/storage"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type HttpServer struct {
	Router     *gin.Engine
	Token      auth.TokenInterface
	Config     *config.Configs
	PubsubChan chan *pubsub.PubSubPayload
}

func (server *HttpServer) initialize(config *config.Configs) {
	server.Config = config
	server.Router = routes.Build(server.PubsubChan)
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

	log.Info().Msg("Starting the server...")
	if err := server.Router.Run(fmt.Sprintf(":%d", server.Config.HTTPServer.Port)); err != nil {
		log.Error().Err(err).Msg("Server is not running!")
	}
}

func main() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	config, err := config.Set("httpserver.yaml")
	if err != nil {
		log.Error().Msg(err.Error())
		return
	}

	storage.Connect(fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		config.DB.Postgres.URI,
		config.DB.Postgres.Port,
		config.DB.Postgres.Username,
		config.DB.Postgres.Password,
		config.DB.Postgres.DatabaseName,
	))

	auth.Init(config)
	if err != nil {
		log.Error().Msg(err.Error())
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
