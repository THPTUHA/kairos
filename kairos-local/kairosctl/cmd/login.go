package cmd

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

type LoginConfig struct {
	NodeName            string
	DeamonLoginEndpoint string
	APIKey              string
	SecretKey           string
}

var loginConfig LoginConfig

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login kairosdeamon to connect server",
	Long:  `Login kairosdeamon to connect server`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return loginRun()
	},
}

func init() {
	kairosctlCmd.AddCommand(loginCmd)
	loginCmd.PersistentFlags().StringVar(&loginConfig.NodeName, "name", "", "Node name")
	loginCmd.PersistentFlags().StringVar(&loginConfig.APIKey, "api_key", "", "API KEY")
	loginCmd.PersistentFlags().StringVar(&loginConfig.SecretKey, "secret_key", "", "Secret Key")
	loginCmd.PersistentFlags().StringVar(&loginConfig.DeamonLoginEndpoint, "endpoint", "http://localhost:3111/apis/login", "Node name")
}

func loginRun() error {
	fmt.Println("Start login ....")
	req, err := http.NewRequest("GET", fmt.Sprintf("%s?name=%s&api_key=%s&secret_key=%s",
		fmt.Sprintf("%s/", loginConfig.DeamonLoginEndpoint),
		loginConfig.NodeName,
		loginConfig.APIKey,
		loginConfig.SecretKey,
	), nil)
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
	return err
}
