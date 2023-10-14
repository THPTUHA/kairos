package config

type Configs struct {
	Auth struct {
		HmacSecret string `yaml:"hmacsecret"`
		HmrfSecret string `yaml:"hmrfsecret"`
	}
	HTTPServer struct {
		Port int `yaml:"port"`
	}
	Redis struct {
		Host     string `yaml:"host"`
		Port     string `yaml:"port"`
		Password string `yaml:"pwd"`
	}
}
