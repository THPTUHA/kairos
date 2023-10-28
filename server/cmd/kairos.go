package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/THPTUHA/kairos/agent"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var config = agent.DefaultConfig()

var kairosCmd = &cobra.Command{
	Use:   "kairos",
	Short: "Task scheduling system",
}

func Execute() {
	if err := kairosCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// TODO
func initConfig() {
	if err := viper.Unmarshal(config); err != nil {
		log.Fatal("config: Error unmarshalling config")
	}
}

// add config here
// TODO
func init() {
	cobra.OnInitialize(initConfig)
}
