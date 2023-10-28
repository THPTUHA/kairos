package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var applyConfig ApplyConfig

var kairosctlCmd = &cobra.Command{
	Use:   "kairosctl",
	Short: "Task scheduling system local",
}

func Execute() {
	if err := kairosctlCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
