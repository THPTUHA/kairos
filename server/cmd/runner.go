package cmd

import (
	"github.com/THPTUHA/kairos/server/runner"
	"github.com/spf13/cobra"
)

var runnerConfigFile string
var runnerCmd = &cobra.Command{
	Use:   "runner",
	Short: "Runner execute, distribute, monitor task",
	RunE: func(cmd *cobra.Command, args []string) error {
		return RunnerRun(args...)
	},
}

func init() {
	kairosCmd.AddCommand(runnerCmd)
	runnerCmd.PersistentFlags().StringVar(&runnerConfigFile, "file", "runner.yaml", "File apply")
}

func RunnerRun(args ...string) error {
	runner, err := runner.NewRunner(runnerConfigFile)
	if err != nil {
		return err
	}
	err = runner.Start()
	return err
}
