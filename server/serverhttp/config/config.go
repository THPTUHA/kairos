package config

type Configs struct {
	Auth struct {
		HmacSecret string `yaml:"hmacsecret"`
	}
	ServerHTTP struct {
		Port int `yaml:"port"`
	}
}
