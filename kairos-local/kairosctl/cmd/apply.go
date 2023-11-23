package cmd

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/spf13/cobra"
)

type ApplyConfig struct {
	File string `mapstructure:"file"`
}

var applyConfig ApplyConfig

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

type Message struct {
	Cmd     string `json:"cmd"`
	Channel string `json:"channel"`
	Content string `json:"content"`
}

func applyRun() error {
	if applyConfig.File == "" {
		return errors.New("Kairos: empty file config")
	}

	wf, _ := workflow.ComplieFile(applyConfig.File)

	// _, err := json.Marshal(wf)
	// if err != nil {
	// 	fmt.Println("fuck 1", err)
	// 	return err
	// }
	// var newWf workflow.WorkflowFile
	// err = json.Unmarshal(a, &newWf)
	// if err != nil {
	// 	fmt.Println("fuck 2", err)
	// 	return err
	// }

	wf.Brokers.Range(func(key string, value *workflow.Broker) error {
		d, err := json.Marshal(value.Flows)
		fmt.Println(string(d), err)
		var e workflow.BrokerFlows
		err = json.Unmarshal(d, &e)
		fmt.Println("dd", e, err)
		return nil
	})
	// var collection workflow.Collection
	// collection.Namespace = wfyml.Collection.Namespace
	// collection.RawData = string(file)
	// collection.Workflows = wfyml.Collection.Workflows

	// url := "http://localhost:3111/apis/apply-collection"
	// body, err := json.Marshal(collection)
	// if err != nil {
	// 	return err
	// }

	// req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	// if err != nil {
	// 	return err
	// }
	// req.Header.Set("Content-Type", "application/json")
	// client := &http.Client{}
	// resp, err := client.Do(req)
	// if err != nil {
	// 	return err
	// }

	// defer resp.Body.Close()
	// c := &bytes.Buffer{}
	// _, err = c.ReadFrom(resp.Body)
	// fmt.Println(c.String())
	// // save workflow
	// check workflow thoa man local

	return nil
}
