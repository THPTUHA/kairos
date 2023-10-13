package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/THPTUHA/kairos/server/pkg/auth"
	"github.com/THPTUHA/kairos/server/serverhttp/config"
	"github.com/THPTUHA/kairos/server/serverhttp/middlewares"
	"github.com/THPTUHA/kairos/server/serverhttp/routes"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func build() *gin.Engine {
	ginApp := gin.New()
	ginApp.Use(gin.Recovery())
	ginApp.Use(middlewares.CORSMiddleware())
	routes.Init(ginApp)

	return ginApp
}

func start(app *gin.Engine, port int) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals,
		os.Interrupt,
	)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: app,
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
	if err := app.Run(fmt.Sprintf(":%d", port)); err != nil {
		log.Error().Err(err).Msg("Server is not running!")
	}
}

func main() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	config, err := config.Get("serverhttp.yaml")
	if err != nil {
		log.Error().Msg(err.Error())
		return
	}

	auth.Init(config)
	if err != nil {
		log.Error().Msg(err.Error())
		return
	}
	ginApp := build()
	start(ginApp, config.ServerHTTP.Port)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	<-signalChan
}
