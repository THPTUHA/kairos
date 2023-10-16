package cmd

import (
	"fmt"
	"os"

	"github.com/THPTUHA/kairos/server/agent"
	"github.com/spf13/cobra"
)

var config = agent.DefaultConfig()

var kairosCmd = &cobra.Command{
	Use:   "kairos",
	Short: "Job scheduling system",
}

func Execute() {
	if err := kairosCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// TODO
func initConfig() {

}

// add config here
// TODO
func init() {
	cobra.OnInitialize(initConfig)
}
