package cmd

import (
	"fmt"
	"os"

	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Create workflow task",
	Long:  `Create workflow task, schedule task`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return applyRun()
	},
}

func init() {
	kairosctlCmd.AddCommand(applyCmd)
	applyCmd.PersistentFlags().StringVar(&applyConfig.File, "file", "", "File apply")
}

type Workflows struct {
	Dag []workflow.Workflow `yaml:"workflows"`
}

func applyRun() error {
	file, err := os.ReadFile(applyConfig.File)
	if err != nil {
		return err
	}
	var workflows workflow.Test
	yaml.Unmarshal(file, workflows)
	fmt.Printf("%+v\n", workflows)
	return nil
}
