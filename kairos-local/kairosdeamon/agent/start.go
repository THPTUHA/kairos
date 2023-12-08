package agent

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/config"
	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/events"
	"github.com/rs/zerolog/log"
)

var ShutdownCh chan (struct{})
var agentServer *Agent

const (
	gracefulTimeout = 3 * time.Hour
)

func AgentStart(eventCh chan *events.Event, conf *config.Configs) error {
	config := DefaultConfig(conf)
	p := &Plugins{
		LogLevel: config.LogLevel,
		NodeName: config.NodeName,
	}

	if err := p.DiscoverPlugins(); err != nil {
		log.Error().Msg(err.Error())
	}

	log.Info().Msg(fmt.Sprintf("Executor %+v:", p.Executors))

	plugins := Plugins{
		Processors: p.Processors,
		Executors:  p.Executors,
	}
	hub := NewHub(eventCh, conf)
	agentServer = NewAgent(config, WithPlugins(plugins), WithEventCh(eventCh))
	agentServer.AddHub(hub)
	if err := agentServer.Start(); err != nil {
		return err
	}
	exit := handleSignals()
	if exit != 0 {
		return fmt.Errorf("exit status: %d", exit)
	}

	return nil
}

func handleSignals() int {
	signalCh := make(chan os.Signal, 4)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	var sig os.Signal
WAIT_EXP:
	select {
	case s := <-signalCh:
		sig = s
	case <-ShutdownCh:
		sig = os.Interrupt
	}

	if sig == syscall.SIGHUP {
		handleReload()
		goto WAIT_EXP
	}

	if sig != syscall.SIGTERM && sig != os.Interrupt {
		return 1
	}

	go func() {
		if err := agentServer.Stop(); err != nil {
			fmt.Printf("Error: %s", err)
			return
		}
	}()

	gracefulCh := make(chan struct{})

	for {
		log.Info().Msg("Waiting for tasks to finish...")
		if agentServer.GetRunningTasks() < 1 {
			log.Info().Msg("No tasks left. Exiting.")
			break
		}
		time.Sleep(1 * time.Second)
	}

	close(gracefulCh)

	select {
	case <-signalCh:
		return 1
	case <-time.After(gracefulTimeout):
		return 1
	case <-gracefulCh:
		return 0
	}

}

func handleReload() {
	fmt.Println("Reloading configuration...")
}
