package workflow

import "github.com/THPTUHA/kairos/pkg/dag"

const (
	ReSet = iota
	Pending
	Running
)

type Task struct {
	ID  int64  `json:"id"`
	Key string `yaml:"key" json:"key"`
	// Để chạy đưọc task này thì các task deps phải chạy success, hoặc pass qua nếu lỗi.
	Dependencies []string `yaml:"dependencies" json:"dependencies"`
	// Thời điểm task thực thi, nếu đến thời điểm thực thi mà các deps chưa hoàn thành,
	// sẽ dựa vào ontime để có chiến lược thực thi
	// Nếu để trống thì mặc định sẽ là thực hiện sau deps
	Schedule string `yaml:"schedule" json:"schedule"`
	Timezone string `yaml:"timezone" json:"timezone"`
	// Client sẽ tiếp nhận task
	Executor string `yaml:"executor" json:"executor"`
	// Thời gian thực thi task ở phía client
	Timeout string `yaml:"timeout" json:"timeout"`
	// Số lần thử gửi task,
	Retries int `yaml:"retries" json:"retries"`
	//
	Run string `yaml:"run" json:"run"`
	// Dữ liệu trả về để thực thi task (message, script)
	Payload string `yaml:"payload" json:"payload"`
}

type Workflow struct {
	ID           int64   `json:"id"`
	Key          string  `yaml:"key"`
	Tasks        []*Task `yaml:"tasks" json:"tasks"`
	Status       int     `json:"status" json:"status"`
	CollectionID int64   `json:"collection_id" json:"collection_id"`
	CreatedAt    int     `json:"created_at" json:"created_at"`
	UpdatedAt    int     `json:"updated_at" json:"updated_at"`
}
type Collection struct {
	ID        int64       `json:"id"`
	Path      string      `json:"path"`
	Username  string      `json:"username"`
	Namespace string      `json:"namespace" yaml:"namespace"`
	RawData   string      `json:"string"`
	Status    int         `json:"status"`
	Workflows []*Workflow `json:"workflows" yaml:"workflows"`
}

type Yaml struct {
	Collection Collection `yaml:"collection"`
}

func isDag(tasks []*Task) error {
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
