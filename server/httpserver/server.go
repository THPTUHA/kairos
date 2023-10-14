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
	"github.com/THPTUHA/kairos/server/httpserver/routes"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type HttpServer struct {
	Router *gin.Engine
	Token  auth.TokenInterface
	Config *config.Configs
}

func (server *HttpServer) initialize(config *config.Configs) {
	server.Config = config
	server.Router = routes.Build()
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
	config, err := config.Get("httpserver.yaml")
	if err != nil {
		log.Error().Msg(err.Error())
		return
	}

	auth.Init(config)
	if err != nil {
		log.Error().Msg(err.Error())
		return
	}

	httpserver := HttpServer{}
	httpserver.initialize(config)
	httpserver.start()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	<-signalChan
}
