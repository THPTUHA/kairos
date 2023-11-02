package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

type ApplyConfig struct {
	File string `mapstructure:"file"`
}

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

func applyRun() error {
	if applyConfig.File == "" {
		return errors.New("Empty file config")
	}
	file, err := os.ReadFile(applyConfig.File)
	if err != nil {
		return err
	}
	var wfyml workflow.Yaml
	err = yaml.Unmarshal(file, &wfyml)
	if err != nil {
		return err
	}

	var collection workflow.Collection
	collection.Namespace = wfyml.Collection.Namespace
	collection.RawData = string(file)
	collection.Workflows = wfyml.Collection.Workflows

	url := "http://localhost:3111/apis/apply-collection"
	body, err := json.Marshal(collection)
	if err != nil {
		return err
	}

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
	fmt.Println(c.String())
	// save workflow
	// check workflow thoa man local

	return err
}
