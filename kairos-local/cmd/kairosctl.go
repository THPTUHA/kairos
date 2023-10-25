package cmd

import (
	"fmt"
	"os"

	"github.com/THPTUHA/kairos/kairos-local/apply"
	"github.com/spf13/cobra"
)

var applyConfig apply.ApplyConfig

var kairosctlCmd = &cobra.Command{
	Use:   "kairosctl",
	Short: "Job scheduling system local",
}

func Execute() {
	if err := kairosctlCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
