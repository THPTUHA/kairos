package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/THPTUHA/kairos/agent"
	"github.com/hashicorp/go-plugin"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var ShutdownCh chan (struct{})
var agentServer *agent.Agent

const (
	// gracefulTimeout controls how long we wait before forcefully terminating
	gracefulTimeout = 3 * time.Hour
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Start a kairos agent",
	Long:  `Start a kairos agent that schedules tasks, listens for executions and runs executors.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return agentRun()
	},
}

func init() {
	kairosCmd.AddCommand(agentCmd)

	agentCmd.Flags().AddFlagSet(agent.ConfigAgentFlagSet())
	viper.BindPFlags(agentCmd.Flags())
}

func agentRun() error {
	p := &Plugins{
		LogLevel: config.LogLevel,
		NodeName: config.NodeName,
	}

	if err := p.DiscoverPlugins(); err != nil {
		log.Error().Msg(err.Error())
	}

	log.Info().Msg(fmt.Sprintf("Executor %+v:", p.Executors))

	plugins := agent.Plugins{
		Processors: p.Processors,
		Executors:  p.Executors,
	}
	agentServer = agent.NewAgent(config, agent.WithPlugins(plugins))

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
	// Wait for a signal
	var sig os.Signal
WAIT:
	select {
	case s := <-signalCh:
		sig = s
	case err := <-agentServer.RetryJoinCh():
		fmt.Println("[ERR] agent: Retry join failed: ", err)
		return 1
	case <-ShutdownCh:
		sig = os.Interrupt
	}

	// Check if this is a SIGHUP
	if sig == syscall.SIGHUP {
		handleReload()
		goto WAIT
	}

	// Fail fast if not doing a graceful leave
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

	plugin.CleanupClients()
	close(gracefulCh)

	// Wait for leave or another signal
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
	initConfig()
	//Config reloading will also reload Notification settings
	agentServer.UpdateTags(config.Tags)

}
