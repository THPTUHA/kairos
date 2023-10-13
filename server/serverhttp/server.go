package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/THPTUHA/kairos/server/serverhttp/middlewares"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func Build() *gin.Engine {
	ginApp := gin.New()
	ginApp.Use(gin.Recovery())

	ginApp.Use(middlewares.CORSMiddleware())

	return ginApp
}

func Start(app *gin.Engine, port int) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals,
		os.Interrupt, // this catch ctrl + c
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
