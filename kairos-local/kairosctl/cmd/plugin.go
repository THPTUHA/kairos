package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

type PluginConfig struct {
	List     bool
	Endpoint string
	Name     string
	Delete   bool
	Add      string
}

var pluginConfig PluginConfig

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Plugin kairosdeamon executor task",
	RunE: func(cmd *cobra.Command, args []string) error {
		return pluginRun()
	},
}

func init() {
	kairosctlCmd.AddCommand(pluginCmd)
	pluginCmd.PersistentFlags().BoolVar(&pluginConfig.List, "list", false, "List plugin")
	pluginCmd.PersistentFlags().StringVar(&pluginConfig.Name, "name", "", "Name script")
	pluginCmd.PersistentFlags().BoolVar(&pluginConfig.Delete, "delete", false, "Delete plugin")
	pluginCmd.PersistentFlags().StringVar(&pluginConfig.Add, "add", "", "Add plugin [name] [cmd] [args]")
	pluginCmd.PersistentFlags().StringVar(&pluginConfig.Endpoint, "endpoint", "http://localhost:8080", "Endpoint")
}

type Plugin struct {
	Name string   `json:"name"`
	Cmd  string   `json:"cmd"`
	Args []string `json:"args"`
}

func pluginRun() error {
	if pluginConfig.List {
		req, err := http.NewRequest("GET", fmt.Sprintf("%s/v1/plugin/list", pluginConfig.Endpoint), nil)
		if err != nil {
			return err
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		fmt.Println("Plugin", string(respBody))
	}

	if pluginConfig.Delete {
		req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/v1/plugin/%s/delete", pluginConfig.Endpoint, pluginConfig.Name), nil)
		if err != nil {
			return err
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		fmt.Println("Plugin", string(respBody))
	}

	if pluginConfig.Delete {
		req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/v1/plugin/%s/delete", pluginConfig.Endpoint, pluginConfig.Name), nil)
		if err != nil {
			return err
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		fmt.Println("Plugin", string(respBody))
	}

	if len(pluginConfig.Add) > 0 {
		pa := strings.Split(pluginConfig.Add, " ")
		if len(pa) < 2 {
			return fmt.Errorf("Plugin add need cmd")
		}
		p := Plugin{
			Name: pa[0],
			Cmd:  pa[1],
			Args: pa[2:],
		}

		body, _ := json.Marshal(p)
		req, err := http.NewRequest("POST", fmt.Sprintf("%s/v1/plugin/add", pluginConfig.Endpoint), bytes.NewBuffer(body))
		if err != nil {
			return err
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		fmt.Println("Plugin", string(respBody))
	}
	return nil
}
