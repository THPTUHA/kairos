package main

import (
	"github.com/THPTUHA/kairos/agent/kairos-local/kairosdeamon/server"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
)

func main() {
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack
	server.Start()
}
