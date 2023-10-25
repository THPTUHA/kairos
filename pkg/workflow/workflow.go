package workflow

type Task struct {
	Key          string            `yaml:"key"`
	Dependencies string            `yaml:"dependencies"`
	TriggerTime  string            `yaml:"trigger_time"`
	Executor     string            `yaml:"executor"`
	Timeout      string            `yaml:"timeout"`
	Retries      int               `yaml:"retries"`
	Inputs       map[string]string `yaml:"inputs"`
	Run          string            `yaml:"run"`
}

type Workflow struct {
	Key         string `yaml:"key"`
	TriggerTime string `yaml:"trigger_time"`
	Retries     int    `yaml:"retries"`
	Tasks       []Task `yaml:"tasks"`
}

type Test struct {
	Workflows struct {
		Key         string `yaml:"key"`
		TriggerTime string `yaml:"trigger_time"`
		Retries     int    `yaml:"retries"`
		Tasks       []Task `yaml:"tasks"`
	}
}
