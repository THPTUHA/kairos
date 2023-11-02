package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/THPTUHA/kairos/agent"
	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/spf13/cobra"
)

type CollectionConfig struct {
	Path         string
	Namespace    string
	ShowWorkflow string
	Query        string
}

var collectionCmd = &cobra.Command{
	Use: "collection",
	RunE: func(cmd *cobra.Command, args []string) error {
		return collectionRun()
	},
}

func init() {
	kairosctlCmd.AddCommand(collectionCmd)
	collectionCmd.PersistentFlags().StringVar(&collectionConfig.Path, "path", "", "Find collection by path")
	collectionCmd.PersistentFlags().StringVar(&collectionConfig.Query, "query", "", "Find collection by namesapce like query")
	collectionCmd.PersistentFlags().StringVar(&collectionConfig.Namespace, "namespace", "", "Find collection by namesapce")
	collectionCmd.PersistentFlags().StringVar(&collectionConfig.ShowWorkflow, "show-workflow", "", "Show workflows")
}

type collectionResult struct {
	Collections []*workflow.Collection `json:"collections"`
	Message     string                 `json:"message"`
	Err         string                 `json:"error"`
}

func collectionRun() error {
	options := agent.CollectionOptions{
		Namespace: collectionConfig.Namespace,
		Query:     collectionConfig.Query,
		Path:      collectionConfig.Path,
	}
	body, err := json.Marshal(options)
	if err != nil {
		return err
	}

	url := "http://localhost:3111/apis/collection/list"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	c := &bytes.Buffer{}
	_, err = c.ReadFrom(resp.Body)
	if err != nil {
		return err
	}
	var result collectionResult

	err = json.Unmarshal(c.Bytes(), &result)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	if collectionConfig.ShowWorkflow != "" {
		for _, c := range result.Collections {
			wg.Add(1)
			go getWorkflows(c, &wg)
		}
	}

	wg.Wait()

	for _, c := range result.Collections {
		fmt.Printf("%+v\n", c)
		fmt.Println("*************************")
	}
	return nil
}

type workflowResult struct {
	Workflows []*workflow.Workflow `json:"workflows"`
	Message   string               `json:"message"`
	Err       string               `json:"error"`
}

func getWorkflows(c *workflow.Collection, wg *sync.WaitGroup) error {
	q := ""
	if collectionConfig.ShowWorkflow != "*" {
		q = collectionConfig.ShowWorkflow
	}

	options := agent.WorkflowOptions{
		Query:        q,
		CollectionID: c.ID,
	}

	body, err := json.Marshal(options)
	if err != nil {
		wg.Done()
		return err
	}

	url := "http://localhost:3111/apis/collection/workflow/list"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		wg.Done()
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		wg.Done()
		return err
	}
	defer resp.Body.Close()

	wf := &bytes.Buffer{}
	_, err = wf.ReadFrom(resp.Body)
	if err != nil {
		wg.Done()
		return err
	}

	var result workflowResult

	err = json.Unmarshal(wf.Bytes(), &result)
	if err != nil {
		wg.Done()
		return err
	}

	c.Workflows = result.Workflows
	wg.Done()
	return nil
}
