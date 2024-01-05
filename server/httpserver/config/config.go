package config

import "time"

var (
	KairosDeamon = "kairosdeamon"
	KairosWeb    = "kairosweb"
	AuthTimeout  = 5 * time.Second
	KairosWebURL = "http://kairosweb.badaosuotdoi.com"
)

type Configs struct {
	Auth struct {
		HmacSecret   string `yaml:"hmacsecret"`
		HmrfSecret   string `yaml:"hmrfsecret"`
		ClientID     string `yaml:"clientid"`
		ClientSecret string `yaml:"client_secret"`
	}
	HTTPServer struct {
		Port                  int    `yaml:"port"`
		MaxWorkflowConcurrent int    `yaml:"max_worklow_concurrent"`
		Domain                string `yaml:"domain"`
	}
	PubSub struct {
		Port int `yaml:"port"`
	}
	Nats struct {
		URL  string `yaml:"url"`
		Name string `yaml:"name"`
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
