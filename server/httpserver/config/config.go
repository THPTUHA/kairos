package config

type Configs struct {
	Auth struct {
		HmacSecret string `yaml:"hmacsecret"`
		HmrfSecret string `yaml:"hmrfsecret"`
	}
	HTTPServer struct {
		Port int `yaml:"port"`
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
