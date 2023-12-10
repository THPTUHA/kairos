package cmd

import (
	"github.com/THPTUHA/kairos/server/deliverer"
	"github.com/spf13/cobra"
)

var delivererConfigFile string
var delievererCmd = &cobra.Command{
	Use:   "deliverer",
	Short: "Deliver message on kairos",
	RunE: func(cmd *cobra.Command, args []string) error {
		return delivererRun(args...)
	},
}

func init() {
	kairosCmd.AddCommand(delievererCmd)
	delievererCmd.PersistentFlags().StringVar(&delivererConfigFile, "file", "deliverer.yaml", "File apply")
}

func delivererRun(args ...string) error {
	server, err := deliverer.NewDelivererServer(delivererConfigFile)
	if err != nil {
		return err
	}
	err = server.Start()
	return err
}
