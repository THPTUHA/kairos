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
	"github.com/dop251/goja"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

const (
	Pending = iota
	Delivering
	Running
	Pause
	Destroying
	Destroyed
	Recovering
)

const (
	PubSubTask   = "pubsub"
	HttpHookTask = "httphook"
	HttpTask     = "http"
	FileTask     = "file"
	SqlTask      = "sql"
	ScriptTask   = "script"
)

const (
	KairosUser    = "kairosuser"
	KairosChannel = "kairoschannel"
	KairosDaemon  = "kairosdeamon"
)

// wait = input or wait = 3s, 3h..
const TaskWaitInput = "input"

type Task struct {
	ID   int64  `json:"id,omitempty"`
	Name string `yaml:"name" json:"name,omitempty"`
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
	Execute        func(ct *CmdTask) `json:"-"`
	Wait           string            `json:"wait"`
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
	ID       int64  `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Schedule string `json:"schedule,omitempty"`
	Input    string `json:"input,omitempty"`
	// Nơi mà Broker sẽ lắng nghe nhận dữ liệu đầu vào
	Listens []string `yaml:"listens" json:"listens"`
	// Các điều kiện điều hướng task
	Clients     []string                 `yaml:"clients" json:"clients"`
	Queue       bool                     `yaml:"queue" json:"queue"`
	Flows       BrokerFlows              `yaml:"flows" json:"flows,omitempty"`
	Template    *Template                `json:"template,omitempty"`
	DynamicVars map[string]*CmdReplyTask `json:"dynamic_vars,omitempty"`
	WorkflowID  int64                    `json:"workflow_id"`
	Trigger     *Trigger                 `json:"-"`

	Log       *logrus.Entry    `json:"-"`
	TriggerCh chan *Trigger    `json:"-"`
	Output    chan *ExecOutput `json:"-"`
}

func (b *Broker) IsListen(name string) bool {
	for _, l := range b.Listens {
		if l == GetTaskName(name) || l == GetChannelName(name) || l == GetClientName(name) || l == name {
			return true
		}
	}
	return false
}

func (c *Channel) Run() {
	fmt.Println("CHANNEL START TRIGGER")
	c.TriggerCh <- c.Trigger
}

func (b *Broker) Run() {
	fmt.Println("BROKER START TRIGGER")
	b.TriggerCh <- b.Trigger
}

func validateVarListen(dv string, clients []*Client, channels []*Channel, tasks *Tasks) bool {
	satisfied := false
	d := strings.TrimPrefix(GetRootDefaultVar(dv), ".")

	for _, cl := range clients {
		if GetClientName(cl.Name) == d {
			satisfied = true
		}
	}
	for _, ch := range channels {
		if GetChannelName(ch.Name) == d {
			satisfied = true
		}
	}
	tasks.Range(func(key string, value *Task) error {
		if GetTaskName(key) == d {
			satisfied = true
		}
		return nil
	})
	if !satisfied {
		return false
	}
	return true
}

func (b *Broker) Compile(userVars *Vars, clients []*Client, channels []*Channel, tasks *Tasks, funcs *goja.Runtime) error {
	b.DynamicVars = make(map[string]*CmdReplyTask)
	if b.Flows.Condition != "" {
		restrict := true
		if len(b.Clients) > 0 {
			restrict = false
		}

		t := NewTemplate(funcs)
		if err := t.Build(b.Listens, b.Flows.Condition, userVars, restrict); err != nil {
			return err
		}
		b.Template = t
		for k := range t.ListenVars {
			if !validateVarListen(k, clients, channels, tasks) {
				return errors.New(fmt.Sprintf("broker %s: var %s not exist!", b.Name, k))
			}
		}
	} else {
		if len(b.Flows.Endpoints) == 0 {
			return errors.New(fmt.Sprintf("broker %s: empty flows", b.Name))
		}
		for _, e := range b.Flows.Endpoints {
			if !validateVarListen(e, clients, channels, tasks) {
				return errors.New(fmt.Sprintf("broker %s: var %s not exist!", b.Name, e))
			}
		}
	}
	return nil
}

type Brokers struct {
	orderedmap.OrderedMap[string, *Broker]
}

type Channel struct {
	ID        int64         `json:"id"`
	Name      string        `json:"name"`
	Schedule  string        `json:"schedule"`
	Trigger   *Trigger      `json:"-"`
	TriggerCh chan *Trigger `json:"-"`
}

type Client struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Workflow struct {
	Version   *semver.Version `yaml:"version" json:"version"`
	ID        int64           `json:"id" json:"id"`
	Name      string          `yaml:"name" json:"name"`
	Vars      *Vars           `yaml:"vars" json:"vars,omitempty"`
	Namespace string          `yaml:"namespace" json:"namespace"`
	Tasks     Tasks           `yaml:"tasks" json:"tasks"`
	Brokers   Brokers         `yaml:"brokers" json:"brokers"`
	Channels  []*Channel      `json:"channels" json:"channels,omitempty"`
	Clients   []*Client       `json:"clients" json:"clients,omitempty"`
	Status    int             `json:"status" json:"status"`
	CreatedAt int             `json:"created_at" json:"created_at"`
	UpdatedAt int             `json:"updated_at" json:"updated_at"`
	UserID    int64           `json:"user_id"`
}

type Var struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Value string `json:"value"`
}
type Vars struct {
	orderedmap.OrderedMap[string, *Var]
}

type WorkflowFile struct {
	Version   *semver.Version `yaml:"version" json:"version"`
	Name      string          `yaml:"name" json:"name"`
	Vars      *Vars           `yaml:"vars"  json:"vars"`
	Namespace string          `yaml:"namespace"  json:"namespace"`
	Tasks     Tasks           `yaml:"tasks" json:"tasks"`
	Brokers   Brokers         `yaml:"brokers" json:"brokers"`
	Channels  []*Channel      `json:"channels"`
	Clients   []*Client       `json:"clients"`
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

func (t *Task) Compile(userVars map[string]string, assign bool) error {
	if t.Payload != "" {
		localVar, err := extractVariableNames(t.Payload)
		if err != nil {
			return err
		}
		for _, v := range localVar {
			if !strings.HasPrefix(v, ".") {
				return errors.New(fmt.Sprintf("task %s: %s not is variable", t.Name, v))
			}
			if _, ok := userVars[strings.TrimPrefix(v, ".")]; !ok {
				if !isDefaultVar(v) {
					return errors.New(fmt.Sprintf("task %s: %s is not define", t.Name, v))
				} else {
					s := GetRootDefaultVar(v)
					if s == "" {
						return errors.New(fmt.Sprintf("task %s: %s is not define", t.Name, v))
					}
				}
			}
		}
		tmpl, err := template.New("compile").Parse(t.Payload)
		if err != nil {
			return err
		}

		var buf bytes.Buffer
		if err = tmpl.Execute(&buf, userVars); err != nil {
			return err
		}

		if assign {
			t.Payload = buf.String()
		}

		var payload interface{}
		err = json.Unmarshal([]byte(buf.String()), &payload)
		if err != nil {
			return fmt.Errorf("task %s complie payload err: %s", t.Name, err)
		}
		if t.Executor == PubSubTask {
			arrs := make([]map[string]interface{}, 0)
			maps := make(map[string]interface{})
			if err := json.Unmarshal([]byte(buf.String()), &arrs); err != nil {
				err = json.Unmarshal([]byte(buf.String()), &maps)
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
		t.Execute(nil)
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
	v.Value = string(data)
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
	if b.Endpoints != nil {
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
	if wv != nil {
		wv.Range(func(key string, value *Var) error {
			vars[key] = value.Value
			return nil
		})
	}
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

	RequestTaskRunSyncCmd
	SetBrokerCmd
	RequestDestroyWf
	TriggerCmd
)

type ResponseActionTask int

// response
const (
	ReplySetTaskCmd ResponseActionTask = iota
	ReplyStartTaskCmd
	ReplyOutputTaskCmd
	ReplyInputTaskCmd

	ReplyMessageCmd
	ReplyRequestTaskSyncCmd
	ReplySetBrokerCmd
	ReplyDestroyWf
	ReplyLogWfCmd
	ReplyTriggerCmd
)

const (
	PendingDeliver = iota
	SuccessSetTask
	FaultSetTask
	SuccessTriggerTask
	FaultTriggerTask
	SuccessReceiveInputTaskCmd
	SuccessReceiveOutputTaskCmd
	FaultInputTask
	SuccessReceiveRequestRunSyncTaskCmd
	SuccessSetBroker
	FaultSetBroker
	SuccessDestroyWorkflow
	FaultDestroyWorkflow
	SuccessTrigger
	FaultTrigger
)

type Result struct {
	Success    bool   `json:"success"`
	Output     string `json:"output"`
	Attempt    uint   `json:"attempt"`
	StartedAt  int64  `json:"started_at"`
	FinishedAt int64  `json:"finished_at"`
	Offset     int    `json:"offset"`
	RunCount   int64  `json:"run_coun,omitemptyt"`
}

type ResultDebug struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Finish  bool   `json:"finish"`
}

type Content interface{}
type CmdReplyTask struct {
	Cmd        ResponseActionTask `json:"cmd"`
	TaskID     int64              `json:"task_id,omitempty"`
	TaskName   string             `json:"task_name,omitempty"`
	BrokerID   int64              `json:"broker_id,omitempty"`
	BrokerName string             `json:"broker_name,omitempty"`
	DeliverID  int64              `json:"deliver_id,omitempty"`
	RunOn      string             `json:"run_on,omitempty"`
	Status     int                `json:"status"`
	WorkflowID int64              `json:"workflow_id,omitempty"`
	Channel    string             `json:"channel,omitempty"`
	Message    string             `json:"message,omitempty"`
	Content    *Content           `json:"content,omitempty"`
	Result     *Result            `json:"result,omitempty"`
	Trigger    *Trigger           `json:"trigger,omitempty"`
	Input      string             `json:"input,omitempty"`
	SendAt     int64              `json:"send_at"`
	RunCount   int64              `json:"run_coun,omitempty"`
	Group      string             `json:"group,omitempty"`
	StartInput string             `json:"start_input,omitempty"`
	Start      bool               `json:"start,omitempty"`
	Parent     string             `json:"parent,omitempty"`
	Part       string             `json:"part,omitempty"`
	BeginPart  bool               `json:"begin_part,omitempty"`
	FinishPart bool               `json:"finish_part,omitempty"`
	TaskInput  string             `json:"task_input,omitempty"`
}

type Trigger struct {
	ID         int64  `json:"id"`
	WorkflowID int64  `json:"workflow_id"`
	ObjectID   int64  `json:"object_id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Schedule   string `json:"schedule"`
	Input      string `json:"input"`
	Status     int    `json:"status"`
	TriggerAt  int64  `json:"trigger_at"`
	Client     string `json:"client"`
	Action     string `json:"action"`
}

type Retry struct {
	ID      int64 `json:"cmd"`
	Attempt int   `json:"attempt"`
}
type CmdTask struct {
	Cmd        RequestActionTask `json:"cmd"`
	Task       *Task             `json:"task,omitempty"`
	Message    interface{}       `json:"message,omitempty"`
	DeliverID  int64             `json:"deliver_id"`
	WorkflowID int64             `json:"workflow_id,omitempty"`
	Channel    string            `json:"channel"`
	Status     int               `json:"status"`
	From       string            `json:"from,omitempty"`
	SendAt     int64             `json:"send_at"`
	Offset     int               `json:"offset,omitempty"`
	RunCount   int64             `json:"run_coun,omitemptyt"`
	Broker     *Broker           `json:"broker,omitempty"`
	Trigger    *Trigger          `json:"trigger,omitempty"`
	Group      string            `json:"group,omitempty"`
	Parent     string            `json:"parent,omitempty"`
	Part       string            `json:"part,omitempty"`
	Start      bool              `json:"start,omitempty"`
	StartInput string            `json:"start_input,omitempty"`
	Retry      *Retry            `json:"retry,omitempty"`
	Attempt    int               `json:"attempt,omitempty"`
}

type LogDaemon struct {
	Cmd         ResponseActionTask `json:"cmd"`
	Reply       *CmdReplyTask      `json:"reply,omitempty"`
	Request     *CmdTask           `json:"request,omitempty"`
	RunOn       string             `json:"run_on"`
	SendAt      int64              `json:"send_at"`
	WorkflowID  int64              `json:"workflow_id,omitempty"`
	Tracking    string             `json:"tracking,omitempty"`
	BrokerName  string             `json:"broker_name,omitempty"`
	BrokerGroup string             `json:"broker_group,omitempty"`
}

const (
	SetStatusWorkflow = iota
	LogMessageFlow
	DestroyWorkflow
	RecoverWorkflow
	ObjectStatusWorkflow
	TriggerStatusWorkflow
)

const (
	BrokerExecuteFault = iota
	BrokerExecuteSuccess
)

type MonitorWorkflow struct {
	Cmd          int    `json:"cmd"`
	UserID       int64  `json:"user_id"`
	WorkflowID   int64  `json:"workflow_id"`
	WorkflowName string `json:"workflow_name"`
	Data         string `json:"data"`
}
