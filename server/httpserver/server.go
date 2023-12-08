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
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/sirupsen/logrus"
)

type HttpServer struct {
	Router     *routes.Route
	Token      auth.TokenInterface
	Config     *config.Configs
	PubsubChan chan *pubsub.PubSubPayload
	NatConn    *nats.Conn

	Logger *logrus.Entry
}

func (server *HttpServer) initialize(config *config.Configs) {
	server.Config = config
	server.Logger = logger.InitLogger("debug", "httpserver")
}

func (server *HttpServer) start() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals,
		os.Interrupt,
	)
	worklowRunner := runner.NewRunner(runner.Configs{
		MaxWorkflowConcurrent: server.Config.HTTPServer.MaxWorkflowConcurrent,
		Logger:                logger.InitLogger("debug", "runner"),
	})

	server.Router = routes.New(&routes.RouteConfig{
		Pubsub:   server.PubsubChan,
		WfRunner: worklowRunner,
		Nats:     server.NatConn,
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", server.Config.HTTPServer.Port),
		Handler: server.Router.Build(),
	}

	go server.serviceNats()
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
		log.Error().Msg(err.Error())
		return
	}
	httpserver := HttpServer{}

	pubsubChan := make(chan *pubsub.PubSubPayload)
	httpserver.PubsubChan = pubsubChan

	httpserver.NatConn, err = nats.Connect(config.Nats.URL, nats.Name(config.Nats.Name))
	if err != nil {
		log.Error().Msg(err.Error())
		return
	}
	go pubsub.Start(config, pubsubChan)
	httpserver.initialize(config)
	httpserver.start()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	<-signalChan
}
