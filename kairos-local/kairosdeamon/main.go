package main

import (
	"fmt"
	"os"

	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/config"
	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/server"
	"github.com/rs/zerolog/log"
)

func main() {
	args := os.Args

	if len(args) == 0 {
		fmt.Println("No command-line arguments provided.")
		return
	}
	firstArg := args[1]
	fmt.Println("First command-line argument:", firstArg)
	config, err := config.Set(firstArg)
	if err != nil {
		log.Error().Msg(err.Error())
		return
	}
	fmt.Printf("------Config :%+v\n", config)
	server.Start(config)
}
