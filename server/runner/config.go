package runner

import (
	"os"

	"gopkg.in/yaml.v2"
)

type Configs struct {
	Runner struct {
		MaxWorkflowConcurrent int `yaml:"max_workflow_concurrent"`
		DeliverDelay          int `yaml:"deliver_delay"`
		MaxAttempDeliverTask  int `yaml:"max_attemp_deliver"`
		TimeoutRetryDeliver   int `yaml:"timeout_retry_deliver"`
		DeliverTimeout        int `yaml:"deliver_timeout"`
		Port                  int `yaml:"port"`
	}
	Nats struct {
		URL           string `yaml:"url"`
		Name          string `yaml:"name"`
		ReconnectWait int    `yaml:"reconnect_wait"`
		MaxReconnects int    `yaml:"max_reconnect"`
	}
	DB struct {
		Postgres struct {
			Username     string `yaml:"username"`
			Password     string `yaml:"password"`
			Port         int    `yaml:"port"`
			URI          string `yaml:"uri"`
			DatabaseName string `yaml:"databaseName"`
			Protocol     string `yaml:"protocol"`
		}
	}
}

func SetConfig(f string) (*Configs, error) {
	config := &Configs{}
	file, err := os.ReadFile(f)
	if err != nil {
		return nil, err
	}
	err = GetYaml(file, config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func GetYaml(f []byte, s interface{}) error {
	y := yaml.Unmarshal(f, s)
	return y
}
