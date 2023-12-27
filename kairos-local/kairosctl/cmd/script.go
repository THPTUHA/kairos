package cmd

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

type ScriptConfig struct {
	Path     string
	Endpoint string
	Delete   bool
	Name     string
	Show     bool
	List     bool
}

var scriptConfig ScriptConfig

var scriptCmd = &cobra.Command{
	Use:   "script",
	Short: "Script kairosdeamon to connect server",
	RunE: func(cmd *cobra.Command, args []string) error {
		return scriptRun()
	},
}

func init() {
	kairosctlCmd.AddCommand(scriptCmd)
	scriptCmd.PersistentFlags().StringVar(&scriptConfig.Path, "path", "", "Path file script")
	scriptCmd.PersistentFlags().StringVar(&scriptConfig.Name, "name", "", "Name script")
	scriptCmd.PersistentFlags().BoolVar(&scriptConfig.List, "list", false, "List script")
	scriptCmd.PersistentFlags().BoolVar(&scriptConfig.Delete, "delete", false, "Delete script")
	scriptCmd.PersistentFlags().BoolVar(&scriptConfig.Show, "show", false, "Show file script")
	scriptCmd.PersistentFlags().StringVar(&scriptConfig.Endpoint, "endpoint", "http://localhost:8080", "Endpoint")
}

func scriptRun() error {
	if scriptConfig.Path != "" {
		if scriptConfig.Name == "" {
			return fmt.Errorf("must script name")
		}
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		filePath := filepath.Join(dir, scriptConfig.Path)

		file, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer file.Close()
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("file", filePath)
		if err != nil {
			return err
		}

		_, err = io.Copy(part, file)
		if err != nil {
			return err
		}

		writer.Close()

		req, err := http.NewRequest("POST", fmt.Sprintf("%s/v1/script/%s/upload", scriptConfig.Endpoint, scriptConfig.Name), body)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())

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

		fmt.Println("Server Response:", string(respBody))
	}

	if scriptConfig.Show {
		if scriptConfig.Name == "" {
			return fmt.Errorf("must script name")
		}
		req, err := http.NewRequest("GET", fmt.Sprintf("%s/v1/script/%s/show", scriptConfig.Endpoint, scriptConfig.Name), nil)
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
		fmt.Println("Script:", string(respBody))
	}

	if scriptConfig.Delete {
		if scriptConfig.Name == "" {
			return fmt.Errorf("must script name")
		}
		req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/v1/script/%s/drop", scriptConfig.Endpoint, scriptConfig.Name), nil)
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

	if scriptConfig.List {
		req, err := http.NewRequest("GET", fmt.Sprintf("%s/v1/script/list", scriptConfig.Endpoint), nil)
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
