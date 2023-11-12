package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"text/template"

	"github.com/Masterminds/semver/v3"
	"github.com/THPTUHA/kairos/pkg/orderedmap"
	"gopkg.in/yaml.v3"
)

const (
	ReSet = iota
	Pending
	Running
)

const (
	PubSubTask = "pubsub"
	HttpTask   = "http"
)

type Task struct {
	ID   int64  `json:"id"`
	Name string `yaml:"name" json:"name"`
	// Để chạy đưọc task này thì các task deps phải chạy success, hoặc pass qua nếu lỗi.
	Deps []string `yaml:"deps" json:"deps"`
	// Thời điểm task thực thi, nếu đến thời điểm thực thi mà các deps chưa hoàn thành,
	// sẽ dựa vào ontime để có chiến lược thực thi
	// Nếu để trống thì mặc định sẽ là thực hiện sau deps
	Schedule string `yaml:"schedule" json:"schedule"`
	Timezone string `yaml:"timezone" json:"timezone"`
	// Client sẽ tiếp nhận task
	Clients []string `yaml:"clients" json:"clients"`
	// Thời gian hiệu lực của task, khi task đã chạy done thì kết quả sẽ được dùng trong thời gian này
	// Khi hết hạn sẽ chạy mới
	// Task khác mà gọi task ngoài thời gian này sẽ phải chờ
	// Nếu không set thì task sẽ chạy khi bị gọi
	Duration string `yaml:"duration" json:"duraion"`
	// Số lần thử gửi task.
	Retries int `yaml:"retries" json:"retries"`
	// Executor sẽ thực hiện task
	Executor string `json:"executor"`
	// Dữ liệu để thực thi task
	Payload string `yaml:"payload" json:"payload"`
	// Hạn thực thi task, khi quá thời gian sẽ dừng task
	ExpiresAt string `json:"expires_at"`
	// Dữ liệu đầu vào task
	Input string `json:"input"`
	// Kết quả của task
	Output string `json:"output"`

	// map giữa tên client và kairos
	Domains map[string]string
}

type Tasks struct {
	orderedmap.OrderedMap[string, *Task]
}

type BrokerFlows struct {
	Endpoints []string
	Condition string
}
type Broker struct {
	ID   int64
	Name string
	// Nơi mà Broker sẽ lắng nghe nhận dữ liệu đầu vào
	Listens []string `yaml:"listens" json:"listens"`
	// Các điều kiện điều hướng task
	Flows BrokerFlows `yaml:"flows" json:"flows"`
}
type Brokers struct {
	orderedmap.OrderedMap[string, *Broker]
}
type Workflow struct {
	Version   *semver.Version `yaml:"version"`
	ID        int64           `json:"id"`
	Name      string          `yaml:"name"`
	Vars      *Vars           `yaml:"vars"`
	Namespace string          `yaml:"namespace"`
	Tasks     Tasks           `yaml:"tasks" json:"tasks"`
	Brokers   Brokers         `yaml:"brokers"`
	Status    int             `json:"status" json:"status"`
	CreatedAt int             `json:"created_at" json:"created_at"`
	UpdatedAt int             `json:"updated_at" json:"updated_at"`
}

type Var struct {
	ID    int64
	Value string
}
type Vars struct {
	orderedmap.OrderedMap[string, *Var]
}

type WorkflowFile struct {
	Version   *semver.Version `yaml:"version"`
	Name      string          `yaml:"name"`
	Vars      *Vars           `yaml:"vars"`
	Namespace string          `yaml:"namespace"`
	Tasks     Tasks           `yaml:"tasks"`
	Brokers   Brokers         `yaml:"brokers"`
}

func (wf *WorkflowFile) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		var workflowfile struct {
			Version   *semver.Version
			Name      string
			Vars      *Vars
			Namespace string
			Tasks     Tasks
			Brokers   Brokers
		}

		if err := node.Decode(&workflowfile); err != nil {
			return err
		}

		if workflowfile.Version == nil {
			return errors.New("worflowfile: 'version' is required")
		}
		if workflowfile.Tasks.Len() == 0 {
			return errors.New("worflowfile: 'tasks' must not empty")
		}

		if workflowfile.Namespace == "" {
			workflowfile.Namespace = "default"
		}
		wf.Version = workflowfile.Version
		wf.Name = workflowfile.Name
		wf.Vars = workflowfile.Vars
		wf.Namespace = workflowfile.Namespace
		wf.Tasks = workflowfile.Tasks
		wf.Brokers = workflowfile.Brokers

		return nil
	}
	return fmt.Errorf("yaml: line %d: cannot unmarshal %s into workflowfile", node.Line, node.ShortTag())
}

func (t *Task) Complie(vars map[string]string) error {
	if t.Payload != "" {
		ptmp, err := template.New("payload").Parse(t.Payload)
		// TODO check  số lượng biến tồn tại trong template
		if err != nil {
			return err
		}
		var b bytes.Buffer

		err = ptmp.Execute(&b, vars)
		t.Payload = b.String()
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *Tasks) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		tasks := orderedmap.New[string, *Task]()
		if err := node.Decode(&tasks); err != nil {
			return err
		}

		tasks.Range(func(name string, task *Task) error {
			if task == nil {
				task = &Task{
					Name: name,
				}
			}
			task.Name = name

			tasks.Set(name, task)
			return nil
		})

		*t = Tasks{
			OrderedMap: tasks,
		}
		return nil
	}
	return fmt.Errorf("yaml: line %d: cannot unmarshal %s into tasks", node.Line, node.ShortTag())
}

func (v *Var) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if err := node.Decode(&v.Value); err != nil {
			return err
		}

		return nil
	}
	return fmt.Errorf("yaml: line %d: cannot unmarshal %s into vars", node.Line, node.ShortTag())
}

func (v *Var) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &v.Value); err != nil {
		return err
	}
	return nil
}

func (v *Vars) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		vars := orderedmap.New[string, *Var]()
		if err := node.Decode(&vars); err != nil {
			return err
		}

		*v = Vars{
			OrderedMap: vars,
		}
		return nil
	}
	return fmt.Errorf("yaml: line %d: cannot unmarshal %s into vars", node.Line, node.ShortTag())
}

func (b *Brokers) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		brokers := orderedmap.New[string, *Broker]()
		if err := node.Decode(&brokers); err != nil {
			return err
		}

		brokers.Range(func(name string, broker *Broker) error {
			if broker == nil {
				broker = &Broker{
					Name: name,
				}
			}
			broker.Name = name

			brokers.Set(name, broker)
			return nil
		})

		*b = Brokers{
			OrderedMap: brokers,
		}
		return nil
	}
	return fmt.Errorf("yaml: line %d: cannot unmarshal %s into brokers", node.Line, node.ShortTag())
}

func (bf *BrokerFlows) UnmarshalYAML(node *yaml.Node) error {

	switch node.Kind {
	case yaml.SequenceNode:
		if err := node.Decode(&bf.Endpoints); err != nil {
			return err
		}
		return nil
	case yaml.ScalarNode:
		if err := node.Decode(&bf.Condition); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("yaml: line %d: cannot unmarshal %s into brokerflows", node.Line, node.ShortTag())
}

func (b *BrokerFlows) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &b.Endpoints); err != nil {
		if err := json.Unmarshal(data, &b.Condition); err != nil {
			return err
		}
	}
	return nil
}

func (b *BrokerFlows) MarshalJSON() ([]byte, error) {
	if b.Condition != "" {
		return json.Marshal(b.Condition)
	}
	if len(b.Endpoints) != 0 {
		return json.Marshal(b.Endpoints)
	}

	return nil, fmt.Errorf("json: cannot marshal brokerflows")
}

func (w *WorkflowFile) String() string {

	metaStr := fmt.Sprintf(`
		Version : %s
		Name : %s
		Namespace : %s 
	`, w.Version, w.Name, w.Namespace)

	varStr := ""
	if w.Vars != nil {
		w.Vars.Range(func(name string, value *Var) error {
			varStr += fmt.Sprintf("\t%s:%s\n", name, value.Value)
			return nil
		})
	}

	taskStr := ""
	w.Tasks.Range(func(name string, task *Task) error {
		taskStr += fmt.Sprintf("\t%s:\n", name)
		taskStr += fmt.Sprintf("\t\t Name: %s\n", task.Name)
		taskStr += fmt.Sprintf("\t\t Deps: %s\n", task.Deps)
		taskStr += fmt.Sprintf("\t\t Schedule: %s\n", task.Schedule)
		taskStr += fmt.Sprintf("\t\t Timezone: %s\n", task.Timezone)
		taskStr += fmt.Sprintf("\t\t Clients: %s\n", task.Clients)
		taskStr += fmt.Sprintf("\t\t Duration: %s\n", task.Duration)
		taskStr += fmt.Sprintf("\t\t Retries: %d\n", task.Retries)
		taskStr += fmt.Sprintf("\t\t Executor: %s\n", task.Executor)
		taskStr += fmt.Sprintf("\t\t ExpiresAt: %s\n", task.ExpiresAt)
		taskStr += fmt.Sprintf("\t\t Payload: %s\n", task.Payload)
		return nil
	})

	brokerStr := ""
	w.Brokers.Range(func(name string, broker *Broker) error {
		brokerStr += fmt.Sprintf("\t%s:\n", name)
		brokerStr += fmt.Sprintf("\t\t Name: %s\n", broker.Name)
		brokerStr += fmt.Sprintf("\t\t Listens: %s\n", broker.Listens)
		brokerStr += fmt.Sprintf("\t\t Flows: %s\n", broker.Flows)
		return nil
	})

	return fmt.Sprintf("Meta:\n%s\nVars:\n%sTasks:\n%sBrokers:\n%s",
		metaStr, varStr, taskStr, brokerStr)
}

func GetKeyValueVars(wv *Vars) map[string]string {
	vars := make(map[string]string)
	wv.Range(func(key string, value *Var) error {
		vars[key] = value.Value
		return nil
	})
	return vars
}
