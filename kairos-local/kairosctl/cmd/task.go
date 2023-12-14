package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

type TaskConfig struct {
	List     bool
	Query    string
	Endpoint string
	Create   bool
	Name     *string
	Input    *string
	Body     string
}

var taskConfig TaskConfig

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Create, delete, execute single task",
	RunE: func(cmd *cobra.Command, args []string) error {
		return taskRun()
	},
}

func init() {
	kairosctlCmd.AddCommand(taskCmd)
	taskCmd.PersistentFlags().BoolVar(&taskConfig.List, "list", false, "List task")
	taskCmd.PersistentFlags().StringVar(&taskConfig.Query, "query", "", "Query task")
	taskCmd.PersistentFlags().BoolVar(&taskConfig.Create, "create", false, "Create task")
	taskCmd.PersistentFlags().StringVar(&taskConfig.Body, "body", "", "Body task")
	taskCmd.PersistentFlags().StringVar(&taskConfig.Endpoint, "endpoint", "http://localhost:8080", "Endpoint")
	taskConfig.Name = taskCmd.PersistentFlags().StringP("name", "n", "", "Name task")
	taskConfig.Input = taskCmd.PersistentFlags().StringP("input", "i", "", "input task")
}

type TaskInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func taskRun() error {
	if taskConfig.List {
		var body = "{}"
		if taskConfig.Query != "" {
			body = taskConfig.Query
		}
		req, err := http.NewRequest("POST", fmt.Sprintf("%s/v1/tasks/list", pluginConfig.Endpoint), bytes.NewBuffer([]byte(body)))
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
		var taskInfos []TaskInfo
		err = json.Unmarshal(respBody, &taskInfos)
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.Debug)

		fmt.Fprintln(w, "ID \t Name \t Status \t")

		data := [][]interface{}{}
		for _, t := range taskInfos {
			its := make([]interface{}, 0)
			its = append(its, t.ID, t.Name, t.Status)
			data = append(data, its)
		}

		for _, row := range data {
			for _, col := range row {
				fmt.Fprintf(w, "%v\t", col)
			}
			fmt.Fprintln(w, "")
		}

		w.Flush()
	}

	if taskConfig.Create {
		if taskConfig.Body == "" {
			return fmt.Errorf("empty body")
		}
		req, err := http.NewRequest("POST", fmt.Sprintf("%s/v1/tasks", pluginConfig.Endpoint), bytes.NewBuffer([]byte(taskConfig.Body)))
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
		fmt.Println(string(respBody))
	}
	return nil
}
