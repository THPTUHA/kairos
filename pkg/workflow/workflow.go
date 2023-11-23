package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/Masterminds/semver/v3"
	"github.com/THPTUHA/kairos/pkg/orderedmap"
	"gopkg.in/yaml.v3"
)

const (
	Pending = iota
	Delivering
	Running
	Pause
)

const (
	PubSubTask  = "pub"
	WebHookTask = "webhook"
	HttpTask    = "http"
)

type Task struct {
	ID   int64  `json:"id,omitempty"`
	Name string `yaml:"name" json:"name,omitempty"`
	// Để chạy đưọc task này thì cần phải đợi kết quả từ các deps
	Deps []string `yaml:"deps" json:"deps,omitempty"`
	// Thời điểm task thực thi, nếu đến thời điểm thực thi mà các deps chưa hoàn thành,
	// sẽ dựa vào ontime để có chiến lược thực thi
	// Nếu để trống thì mặc định sẽ là thực hiện sau deps
	Schedule string `yaml:"schedule" json:"schedule,omitempty"`
	Timezone string `yaml:"timezone" json:"timezone,omitempty"`
	// Client sẽ tiếp nhận task
	Clients []string `yaml:"clients" json:"clients,omitempty"`
	// Số lần thử gửi task.
	Retries int `yaml:"retries" json:"retries,omitempty"`
	// Executor sẽ thực hiện task
	Executor string `json:"executor,omitempty"`
	// Dữ liệu để thực thi task
	Payload string `yaml:"payload" json:"payload,omitempty"`
	// Hạn thực thi task, khi quá thời gian task sẽ không chạy được, nếu task đang chạy vẫn sẽ tiếp tục chạy
	ExpiresAt string `json:"expires_at,omitempty"`

	// Dữ liệu đầu vào task
	Input string `json:"input,omitempty"`
	// Kết quả của task
	Output     string            `json:"output,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	WorkflowID int64             `json:"workflow_id"`

	UserDefineVars map[string]string `json:"user_define_vars,omitempty"`
	DynamicVars    map[string]bool   `json:"dynamic_vars,omitempty"`
	Execute        func()            `json:"-"`
}

type Tasks struct {
	orderedmap.OrderedMap[string, *Task]
}

type BrokerFlows struct {
	Endpoints []string
	Condition string
}

type BrokerFlowsUn struct {
	Endpoints []string
	Condition string
}
type Broker struct {
	ID   int64  `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	// Nơi mà Broker sẽ lắng nghe nhận dữ liệu đầu vào
	Listens []string `yaml:"listens" json:"listens"`
	// Các điều kiện điều hướng task
	Flows       BrokerFlows              `yaml:"flows" json:"flows,omitempty"`
	Queue       bool                     `yaml:"queue" json:"queue"`
	Exps        *Template                `json:"template,omitempty"`
	DynamicVars map[string]*CmdReplyTask `json:"dynamic_vars,omitempty"`
}

func (b *Broker) IsListen(name string) bool {
	for _, l := range b.Listens {
		if l == GetTaskName(name) || l == GetChannelName(name) || l == GetClientName(name) || l == name {
			return true
		}
	}
	return false
}

func (b *Broker) Compile(userVars *Vars) error {
	b.DynamicVars = make(map[string]*CmdReplyTask)
	if b.Flows.Condition != "" {
		t := NewTemplate()
		if err := t.Build(b.Listens, b.Flows.Condition, userVars); err != nil {
			return err
		}
		b.Exps = t
	} else {
		if len(b.Flows.Endpoints) == 0 {
			return errors.New(fmt.Sprintf("broker %s: empty flows", b.Name))
		}
		// TODO check validate condition
	}
	return nil
}

type Brokers struct {
	orderedmap.OrderedMap[string, *Broker]
}

type Channel struct {
	ID   int64
	Name string
}

type Client struct {
	ID   int64
	Name string
}

type Workflow struct {
	Version   *semver.Version `yaml:"version"`
	ID        int64           `json:"id"`
	Name      string          `yaml:"name"`
	Vars      *Vars           `yaml:"vars"`
	Namespace string          `yaml:"namespace"`
	Tasks     Tasks           `yaml:"tasks" json:"tasks"`
	Brokers   Brokers         `yaml:"brokers"`
	Channels  []*Channel      `json:"channels"`
	Clients   []*Client       `json:"clients"`
	Status    int             `json:"status" json:"status"`
	CreatedAt int             `json:"created_at" json:"created_at"`
	UpdatedAt int             `json:"updated_at" json:"updated_at"`
	UserID    int64
}

type Var struct {
	ID    int64
	Name  string
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

func isDefaultVar(v string) bool {
	if strings.HasSuffix(v, SubTask) || strings.HasSuffix(v, SubChannel) {
		return true
	}
	for _, e := range SubTasks {
		if strings.HasSuffix(v, e) {
			return true
		}
	}
	for _, e := range SubChannels {
		if strings.HasSuffix(v, e) {
			return true
		}
	}
	return false
}

func IsRootDefaultVar(v string) bool {
	return strings.HasSuffix(v, SubTask) ||
		strings.HasSuffix(v, SubChannel) ||
		strings.HasSuffix(v, SubClient)
}

func GetRootDefaultVar(v string) string {
	if strings.HasSuffix(v, SubTask) || strings.HasSuffix(v, SubChannel) || strings.HasSuffix(v, SubClient) {
		return v
	}
	for _, e := range SubTasks {
		if strings.HasSuffix(v, e) {
			return GetTaskName(strings.TrimSuffix(v, e))
		}
	}

	for _, e := range SubChannels {
		if strings.HasSuffix(v, e) {
			return GetChannelName(strings.TrimSuffix(v, e))
		}
	}

	for _, e := range SubClients {
		if strings.HasSuffix(v, e) {
			return GetClientName(strings.TrimRight(v, e))
		}
	}
	return ""
}

func isPoint(v string) bool {
	return strings.HasSuffix(v, SubTask) || strings.HasSuffix(v, SubChannel) || strings.HasSuffix(v, SubClient)
}

func (v *Var) Compile() error {
	if v.Value == "" {
		return errors.New(fmt.Sprintf("var %s: empty", v.Name))
	}
	localVar, err := extractVariableNames(v.Value)
	if err != nil {
		return err
	}
	if len(localVar) > 0 {
		return errors.New(fmt.Sprintf("var %s: No dynamic variables allowed", v.Name))
	}
	return nil
}

func (t *Task) Compile(userVars map[string]string) error {
	for _, e := range t.Deps {
		if !isPoint(e) {
			return errors.New(fmt.Sprintf("dep %s must be _task, _channel or _client", e))
		}
	}
	if t.Payload != "" {
		localVar, err := extractVariableNames(t.Payload)
		if err != nil {
			return err
		}
		dynamicVars := make(map[string]bool)
		// for l, v := range userVars {
		// 	fmt.Printf("Key value ---", l, v)
		// }
		for _, v := range localVar {
			if !strings.HasPrefix(v, ".") {
				return errors.New(fmt.Sprintf("task %s: %s not is variable", t.Name, v))
			}
			if _, ok := userVars[strings.TrimPrefix(v, ".")]; !ok {
				// check default var
				if !isDefaultVar(v) {
					return errors.New(fmt.Sprintf("task %s: %s is not define", t.Name, v))
				} else {
					// check deps
					s := GetRootDefaultVar(v)
					if s == "" {
						return errors.New(fmt.Sprintf("task %s: %s is not define", t.Name, v))
					}
					if !includes(t.Deps, strings.ToLower(strings.TrimPrefix(s, "."))) {
						return errors.New(fmt.Sprintf("%s is not dependence in %+v", v, t.Deps))
					}
					dynamicVars[strings.TrimPrefix(v, ".")] = true
				}
			}
		}
		t.DynamicVars = dynamicVars
		if len(dynamicVars) == 0 {
			// no value
			tmpl, err := template.New("extract").Parse(t.Payload)
			if err != nil {
				return err
			}
			var buf bytes.Buffer
			// to upper
			parse := make(map[string]string)
			for k, v := range userVars {
				parse[strings.ToUpper(k)] = v
			}
			err = tmpl.Execute(&buf, &parse)
			if err != nil {
				return err
			}
			t.Payload = buf.String()
		}

		if t.Executor == PubSubTask {
			arrs := make([]map[string]interface{}, 0)
			maps := make(map[string]interface{})
			if err := json.Unmarshal([]byte(t.Payload), &arrs); err != nil {
				err = json.Unmarshal([]byte(t.Payload), &maps)
				if err != nil {
					return err
				}
			}

			for _, m := range arrs {
				for k, v := range m {
					switch v.(type) {
					case string:
						fmt.Println("String here", k, v)
					default:
						str, _ := json.Marshal(v)
						fmt.Println("Marshal String here", k, string(str))
					}
				}
			}
		}
	}
	return nil
}

func (t *Task) Run() {
	if t.Execute != nil {
		t.Execute()
	}
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
			// stand
			for idx, d := range task.Deps {
				task.Deps[idx] = strings.ToLower(d)
			}
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

		varsStand := orderedmap.New[string, *Var]()
		vars.Range(func(key string, value *Var) error {
			varsStand.Set(strings.ToLower(key), value)
			return nil
		})
		*v = Vars{
			OrderedMap: varsStand,
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
			for idx, l := range broker.Listens {
				broker.Listens[idx] = strings.ToLower(l)
			}
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
			var c BrokerFlowsUn
			if err := json.Unmarshal(data, &c); err != nil {
				return err
			}
			b.Condition = c.Condition
			b.Endpoints = c.Endpoints
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

func GetTaskName(taskName string) string {
	return strings.ToLower(fmt.Sprintf("%s%s", taskName, SubTask))
}

func GetChannelName(channel string) string {
	return strings.ToLower(fmt.Sprintf("%s%s", channel, SubChannel))
}

func GetBrokerName(broker string) string {
	return strings.ToLower(fmt.Sprintf("%s%s", broker, SubBroker))
}

func GetClientName(clientName string) string {
	return strings.ToLower(fmt.Sprintf("%s%s", clientName, SubClient))
}

func GetRawName(t string) string {
	t = strings.ToLower(t)
	if strings.HasSuffix(t, SubChannel) {
		return strings.TrimSuffix(t, SubChannel)
	}

	if strings.HasSuffix(t, SubTask) {
		return strings.TrimSuffix(t, SubTask)
	}

	if strings.HasSuffix(t, SubClient) {
		return strings.TrimSuffix(t, SubClient)
	}

	for _, e := range SubClients {
		if strings.HasSuffix(t, e) {
			return strings.TrimSuffix(t, e)
		}
	}

	for _, e := range SubChannels {
		if strings.HasSuffix(t, e) {
			return strings.TrimSuffix(t, e)
		}
	}

	for _, e := range SubTasks {
		if strings.HasSuffix(t, e) {
			return strings.TrimSuffix(t, e)
		}
	}
	return ""
}

type RequestActionTask int

// request
const (
	SetTaskCmd RequestActionTask = iota
	TriggerStartTaskCmd
	InputTaskCmd
)

type ResponseActionTask int

// response
const (
	ReplySetTaskCmd ResponseActionTask = iota
	ReplyStartTaskCmd
	ReplyOutputTaskCmd
	ReplyInputTaskCmd
)

const (
	PendingDeliver = iota
	SuccessSetTask
	FaultSetTask
	ScuccessTriggerTask
	FaultTriggerTask
	SuccessReceiveInputTaskCmd
	SuccessReceiveOutputTaskCmd
	FaultInputTask
)

type Result struct {
	Success    bool   `json:"success"`
	Output     string `json:"output"`
	Attempt    uint   `json:"attempt"`
	StartedAt  int64  `json:"startd_at"`
	FinishedAt int64  `json:"finished_at"`
}

type Content struct {
	Cmd     string `json:"cmd"`
	Message string `json:"message"`
}
type CmdReplyTask struct {
	Cmd        ResponseActionTask `json:"cmd"`
	UserID     int64              `json:"user_id,omitempty"`
	TaskID     int64              `json:"task_id,omitempty"`
	TaskName   string             `json:"task_name,omitempty"`
	DeliverID  int64              `json:"deliver_id,omitempty"`
	RunIn      string             `json:"run_in,omitempty"`
	Status     int                `json:"status"`
	WorkflowID int64              `json:"workflow_id,omitempty"`
	Message    string             `json:"message,omitempty"`
	Content    *Content           `json:"content,omitempty"`
	Result     *Result            `json:"result,omitempty"`
	SendAt     int64              `json:"send_at"`
}

type CmdTask struct {
	Cmd       RequestActionTask `json:"cmd"`
	Task      *Task             `json:"task,omitempty"`
	DeliverID int64             `json:"deliver_id"`
	Channel   string            `json:"channel"`
	Status    int               `json:"status"`
	From      string            `json:"from,omitempty"`
	SendAt    int64             `json:"send_at"`
}

const (
	SetStatusWorkflow = iota
	LogMessageFlow
)

type MonitorWorkflow struct {
	Cmd          int    `json:"cmd"`
	UserID       int64  `json:"user_id"`
	WorkflowID   int64  `json:"workflow_id"`
	WorkflowName string `json:"workflow_name"`
	Data         string `json:"data"`
}
