package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

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

}

// add config here
// TODO
func init() {
	cobra.OnInitialize(initConfig)
}
