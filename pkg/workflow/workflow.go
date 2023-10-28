package workflow

import "github.com/THPTUHA/kairos/pkg/dag"

const (
	ReSet = iota
	Running
)

type Task struct {
	ID           int64    `json:"id"`
	Key          string   `yaml:"key" json:"key"`
	Dependencies []string `yaml:"dependencies" json:"dependencies"`
	Schedule     string   `yaml:"schedule" json:"schedule"`
	Timezone     string   `yaml:"timezone" json:"timezone"`
	Executor     string   `yaml:"executor" json:"executor"`
	Timeout      string   `yaml:"timeout" json:"timeout"`
	Retries      int      `yaml:"retries" json:"retries"`
	Inputs       []string `yaml:"inputs" json:"inputs"`
	Run          string   `yaml:"run" json:"run"`
	Strict       bool     `yaml:"strict" json:"strict"`
}

type Workflow struct {
	ID           int64  `json:"id"`
	Key          string `yaml:"key"`
	Tasks        []Task `yaml:"tasks" json:"tasks"`
	Status       int    `json:"status" json:"status"`
	CollectionID int64  `json:"collection_id" json:"collection_id"`
	CreatedAt    int    `json:"created_at" json:"created_at"`
	UpdatedAt    int    `json:"updated_at" json:"updated_at"`
}
type Collection struct {
	ID        int64      `json:"id"`
	Namespace string     `json:"namespace" yaml:"namespace"`
	RawData   string     `json:"string"`
	Status    int        `json:"status"`
	Workflows []Workflow `json:"workflows" yaml:"workflows"`
}

type Yaml struct {
	Collection Collection `yaml:"collection"`
}

func isDag(tasks []Task) error {
	graph := dag.NewGraph()
	for _, task := range tasks {
		if err := graph.AddVertex(task.Key); err != nil {
			return err
		}
	}

	for _, task := range tasks {
		for _, task_c := range task.Dependencies {
			if err := graph.AddEdge(task.Key, task_c); err != nil {
				return err
			}
		}
	}

	return graph.Validate()
}

func (workflow *Workflow) Validate() error {
	err := isDag(workflow.Tasks)
	if err != nil {
		return err
	}

	return nil
}
