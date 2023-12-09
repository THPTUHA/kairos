package main

import (
	"errors"
	"os"

	"gopkg.in/yaml.v2"
)

const (
	Port      = 8003
	RedisHost = "127.0.0.1"
	RedisPort = "6379"
)

type Configs struct {
	Auth struct {
		HmacSecret string `yaml:"hmacsecret"`
		HmrfSecret string `yaml:"hmrfsecret"`
	}
	Deliverer struct {
		Port int `yaml:"port"`
	}

	Nats struct {
		URL  string `yaml:"url"`
		Name string `yaml:"name"`
	}
}

var config *Configs

func GetConfig() (*Configs, error) {
	if config == nil {
		return nil, errors.New("empty config")
	}
	return config, nil
}

func SetConfig(f string) (*Configs, error) {
	config = &Configs{}
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
