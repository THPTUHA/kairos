package cmd

import (
	"github.com/THPTUHA/kairos/server/httpserver"
	"github.com/spf13/cobra"
)

var configFile string
var httpserverCmd = &cobra.Command{
	Use:   "httpserver",
	Short: "Server serive api on kairos",
	RunE: func(cmd *cobra.Command, args []string) error {
		return httpserverRun(args...)
	},
}

func init() {
	kairosCmd.AddCommand(httpserverCmd)
	httpserverCmd.PersistentFlags().StringVar(&configFile, "file", "httpserver.yaml", "File apply")
}

func httpserverRun(args ...string) error {
	server, err := httpserver.NewHTTPServer(configFile)
	if err != nil {
		return err
	}
	err = server.Start()
	return err
}
