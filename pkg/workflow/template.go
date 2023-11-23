package workflow

import (
	"errors"
	"fmt"
	"strings"
)

var (
	compareExp = []string{"eq", "gt", "lt"}
	errStack   = errors.New("stack err")
)

type Template struct {
	// Kiếm tra biến
	ListenVars map[string]bool
	Vars       *Vars
	IndexRun   map[int]int
	Input      []string
	Exps       [][]*Expression
}

func includes[T comparable](source []T, e T) bool {
	for _, s := range source {
		if s == e {
			return true
		}
	}
	return false
}

func NewTemplate() *Template {
	return &Template{
		ListenVars: make(map[string]bool),
		IndexRun:   make(map[int]int),
		Exps:       make([][]*Expression, 0),
	}
}
func getVarName(v string) string {
	return strings.TrimPrefix(v, ".")
}

// TODO allow set _TASK_OUTPUT, _CHANNEL_MSG
var (
	SubTask        = "_task"
	SubTaskOutPut  = "_task_output"
	SubTaskSuccess = "_task_success"
	SubTaskResult  = "_task_result"
	SubTaskQueue   = "_task_queue"
	SubTasks       = []string{
		SubTaskOutPut,
		SubTaskSuccess,
		SubTaskResult,
	}
	SubChannel    = "_channel"
	SubChannelMSG = "_channel_msg"
	SubChannelCMD = "_channel_cmd"
	SubChannels   = []string{
		SubChannelMSG,
		SubChannelCMD,
	}
	SubClient           = "_client"
	SubClientMSG        = "_client_msg"
	SubClientTaskResult = "_client_task_result"
	SubClientTaskOutPut = "_client_task_output"
	SubClients          = []string{
		SubClientMSG,
		SubClientTaskResult,
		SubClientTaskOutPut,
	}
	SubBroker = "_broker"
)

var (
	GlobalVars = map[string]string{
		".TASK_STATUS_SUCCESS": "1",
	}
)

func (t *Template) checkVariableDefault(exp string, v string) error {
	if !strings.HasPrefix(v, ".") {
		return errors.New(fmt.Sprintf("exp %s: %s is not variable", exp, v))
	}
	// if !strings.HasSuffix(v, SubTask) && !strings.HasSuffix(v, SubChannel) && !strings.HasSuffix(v, SubClient) &&
	// 	t.ListenVars[v] == false &&
	// 	!strings.HasSuffix(v, "_TASK_SUCCESS") &&
	// 	!strings.HasSuffix(v, "_TASK_OUTPUT") &&
	// 	!strings.HasSuffix(v, "_CHANNEL_MSG") &&
	// 	!strings.HasSuffix(v, "_CHANNEL_CMD") {
	// 	if _, ok := GlobalVars[v]; !ok {
	// 		if t.Vars != nil && !t.Vars.Exists(v) {
	// 			return errors.New(fmt.Sprintf("exp %s: %s must be task, channel or globle variable", exp, v))
	// 		}
	// 	}
	// }

	if strings.HasSuffix(v, SubTask) {
		t.ListenVars[v] = true
	}

	if strings.HasSuffix(v, SubChannel) {
		t.ListenVars[v] = true
	}

	if strings.HasSuffix(v, SubClient) {
		t.ListenVars[v] = true
	}

	for _, e := range SubTasks {
		if strings.HasSuffix(v, e) {
			_v := getVarName(strings.TrimSuffix(v, strings.TrimPrefix(e, SubTask)))
			if !includes(t.Input, strings.ToLower(_v)) {
				return errors.New(fmt.Sprintf("task: %s not listened by broker (variable %s)", _v, v))
			}
			t.ListenVars[v] = true
		}
	}

	for _, e := range SubClients {
		if strings.HasSuffix(v, e) {
			_v := getVarName(strings.TrimSuffix(v, strings.TrimPrefix(e, SubTask)))
			if !includes(t.Input, strings.ToLower(_v)) {
				return errors.New(fmt.Sprintf("client: %s not listened by broker (variable %s)", _v, v))
			}
			t.ListenVars[v] = true
		}
	}

	for _, e := range SubChannels {
		if strings.HasSuffix(v, e) {
			_v := getVarName(strings.TrimSuffix(v, strings.TrimPrefix(e, SubChannel)))
			if !includes(t.Input, strings.ToLower(_v)) {
				return errors.New(fmt.Sprintf("channel: %s not listened by broker (variable %s)", _v, v))
			}
			t.ListenVars[v] = true
		}
	}

	return nil
}

func (t *Template) checkParams(items []string, l int) error {
	if len(items) != l {
		return errors.New(fmt.Sprintf("if expression requires %d parameters", l))
	}
	return nil
}

func existIn(varTemps *varTemp, s string) bool {
	for _, buf := range varTemps.items {
		for _, e := range buf {
			if e.key == s {
				return true
			}
		}
	}
	return false
}

func (t *Template) checkValidVar(items []string, start int, varTemps *varTemp) error {
	for _, e := range items[start:] {
		if t.Vars.Exists(strings.TrimPrefix(e, ".")) {
			return nil
		}
		errVarDefault := t.checkVariableDefault(items[0], e)
		if errVarDefault != nil {
			if !existIn(varTemps, e) {
				return errors.New(fmt.Sprintf("%s: stack err, variable = %s has not define", items[0], e))
			}
		}
	}
	return nil
}

func (t *Template) complieChainExps(items []string, varTemps *varTemp) ([]*Expression, error) {
	chain := make([][]string, 0)
	buf := make([]string, 0)
	var exps []*Expression
	for _, e := range items {
		if e == "|" {
			chain = append(chain, buf)
			buf = make([]string, 0)
		} else {
			buf = append(buf, e)
		}
	}
	if len(buf) > 0 {
		chain = append(chain, buf)
	}
	for _, c := range chain {
		if len(c) == 0 && strings.HasPrefix(c[0], ".") {
			if err := t.checkValidVar(c, 0, varTemps); err != nil {
				return nil, err
			}
			varTemps.items[len(varTemps.items)-1] = append(varTemps.items[len(varTemps.items)-1], &kv{
				key: c[0],
			})
			exps = append(exps, &Expression{
				Func:   "set",
				Params: c,
			})
		}
		switch c[0] {
		case "sum":
			if err := t.checkValidVar(c, 1, varTemps); err != nil {
				return nil, err
			}
			exps = append(exps, &Expression{
				Func:   "sum",
				Params: c[1:],
			})
		case "sub":
			if err := t.checkValidVar(c, 1, varTemps); err != nil {
				return nil, err
			}
			exps = append(exps, &Expression{
				Func:   "sub",
				Params: c[1:],
			})
		}
	}
	return exps, nil
}

type kv struct {
	key   string
	value interface{}
}

type varTemp struct {
	items [][]*kv
}

func (vt *varTemp) Push(kv *kv) {
	fmt.Println("push...")
	vt.items[len(vt.items)-1] = append(vt.items[len(vt.items)-1], kv)
}

func (vt *varTemp) Append() {
	fmt.Println("append...")
	buf := make([]*kv, 0)
	vt.items = append(vt.items, buf)
}

func (vt *varTemp) ClearTop() error {
	fmt.Println("clear...")
	if len(vt.items) == 0 {
		return errors.New("stack empty")
	}
	vt.items[len(vt.items)-1] = make([]*kv, 0)
	return nil
}

func (vt *varTemp) Pop() error {
	fmt.Println("pop...")
	if len(vt.items) == 0 {
		return errors.New("stack empty")
	}
	vt.items = vt.items[:len(vt.items)-1]
	return nil
}

func (vt *varTemp) Top() []*kv {
	if len(vt.items) == 0 {
		return nil
	}
	return vt.items[len(vt.items)-1]
}

func (vt *varTemp) GetValue(key string) *kv {
	t := vt.Top()
	if t == nil {
		return nil
	}
	for _, v := range t {
		if v.key == key {
			return v
		}
	}
	return nil
}

func (t *Template) complieExp(exp string, input []string, varTemps *varTemp) ([]*Expression, error) {
	exp = strings.Trim(exp, " ")
	if exp == "" {
		return nil, errors.New(fmt.Sprintf("empty expression "))
	}

	items := strings.Fields(exp)
	var exps []*Expression
	switch items[0] {
	case "if":
		varTemps.Append()
		if !(len(items) == 2 && strings.HasSuffix(items[1], SubTaskSuccess)) {
			if err := t.checkParams(items, 4); err != nil {
				return nil, err
			}
			if err := t.checkValidVar(items, 2, varTemps); err != nil {
				return nil, err
			}
		}
		exps = append(exps, &Expression{
			Func:   "if",
			Params: items[1:],
		})
	case "elseif":
		if len(varTemps.items) == 0 {
			fmt.Println(exp, "...")
			return nil, errors.New("elseif: stack error")
		}
		varTemps.ClearTop()
		if err := t.checkParams(items, 4); err != nil {
			return nil, err
		}
		if err := t.checkValidVar(items, 2, varTemps); err != nil {
			return nil, err
		}
		exps = append(exps, &Expression{
			Func:   "elseif",
			Params: items[1:],
		})
	case "send":
		if err := t.checkParams(items, 3); err != nil {
			return nil, err
		}
		err := t.checkVariableDefault(items[0], items[1])
		if err != nil {
			return nil, err
		}

		if !IsRootDefaultVar(items[2]) {
			return nil, errors.New(fmt.Sprintf("send: destination must have root default var, var %s not is root var", items[2]))
		}
		exps = append(exps, &Expression{
			Func:   "send",
			Params: items[1:],
		})
	case "range":
		// TODO range
		if err := t.checkParams(items, 2); err != nil {
			return nil, err
		}
		varTemps.Append()
		exps = append(exps, &Expression{
			Func:   "range",
			Params: items[1:],
		})
	case "set":
		if err := t.checkParams(items, 3); err != nil {
			return nil, err
		}
		varTemps.Push(&kv{
			key: items[1],
		})
		if err := t.checkValidVar(items, 1, varTemps); err != nil {
			return nil, err
		}
		exps = append(exps, &Expression{
			Func:   "set",
			Params: items[1:],
		})
	case "endif":
		if len(varTemps.items) == 0 {
			return nil, errors.New("endif: stack error")
		}
		varTemps.Pop()
		exps = append(exps, &Expression{
			Func:   "endif",
			Params: items[1:],
		})
	case "endrange":
		if len(varTemps.items) == 0 {
			return nil, errors.New("endrange: stack error")
		}
		varTemps.Pop()
		exps = append(exps, &Expression{
			Func:   "endrange",
			Params: items[1:],
		})
	case "sum":
		var err error
		exps, err = t.complieChainExps(items, varTemps)
		if err != nil {
			return nil, err
		}
	case "wait":
		if len(items) == 0 {
			return nil, errors.New("wait: must have one param")
		}
		for i := 1; i < len(items); i++ {
			if !IsRootDefaultVar(items[i]) {
				return nil, errors.New(fmt.Sprintf("wait: must have root default var, var %s not is root var", items[i]))
			}
			t.ListenVars[items[i]] = true
		}
		exps = append(exps, &Expression{
			Func:   "wait",
			Params: items[1:],
		})
	default:
		return nil, errors.New(fmt.Sprintf("exp %s: not exist", items[0]))
	}

	return exps, nil
}

func (t *Template) Parse(str string, input []string, globalVar *Vars) error {
	t.Vars = globalVar
	open := 1
	close := 1
	expStr := ""
	var varTemps varTemp
	buf := make([]*kv, 0)
	varTemps.items = append(varTemps.items, buf)
	for _, c := range str {
		switch c {
		case '{':
			expStr = ""
			open++
			if open-close > 2 {
				return errors.New(fmt.Sprintf("redundancy { "))
			}
			if open-close < 0 {
				return errors.New(fmt.Sprintf("missing sign { "))
			}
		case '}':
			close++
			if open-close < 0 {
				return errors.New(fmt.Sprintf("missing sign { "))
			}
			if open == close {
				exp, err := t.complieExp(strings.ToLower(expStr), input, &varTemps)
				if err != nil {
					return err
				}
				t.Exps = append(t.Exps, exp)
				expStr = ""
			}
		default:
			expStr += string(c)
		}
	}
	return nil
}

func (t *Template) Build(input []string, s string, vars *Vars) error {
	t.Input = input
	err := t.Parse(s, input, vars)
	if err != nil {
		return err
	}
	stack := Stack{}
	hasSendFunc := false
	for idx, exp := range t.Exps {
		for _, e := range exp {
			if e.Func == "if" {
				stack.Push(idx)
			} else if e.Func == "elseif" {
				jum, _ := stack.Pop()
				t.IndexRun[jum] = idx
				stack.Push(idx)
			} else if e.Func == "endif" {
				jum, _ := stack.Pop()
				t.IndexRun[jum] = idx
			} else if e.Func == "send" {
				hasSendFunc = true
			}
		}
	}

	// TODO build
	for k, v := range t.IndexRun {
		fmt.Println(k, v)
	}
	if !hasSendFunc {
		return errors.New("must have send function")
	}
	return nil
}

type DeliverFlow struct {
	From     string
	Reciever string
	Msg      *CmdReplyTask
}

// Đảm bảo đủ hết các tham số thì mới chạy
func (t *Template) Execute(outputVars map[string]*CmdReplyTask) ([]*DeliverFlow, error) {
	// for _, exp := range t.Exps {
	// 	for _, e := range exp {
	// 		fmt.Printf("%+v\n", e)

	// 	}
	// }
	delivers := make([]*DeliverFlow, 0)
	var varTemp varTemp
	var tempOut interface{}
	for idx := 0; idx < len(t.Exps); idx++ {
		exp := t.Exps[idx]
		for _, e := range exp {
			e.Input = tempOut
			out, err := e.Execute(t.Vars, outputVars, &varTemp)
			if e, ok := out.(*DeliverFlow); ok {
				delivers = append(delivers, e)
			}
			fmt.Println(e.Func, out, err)
			if err != nil {
				return nil, err
			}
			if e.Func == "if" || e.Func == "elseif" {
				if out == false {
					idx = t.IndexRun[idx] - 1
				}
			}
			tempOut = out
		}
	}
	return delivers, nil
}

type Stack struct {
	items []int
}

func (s *Stack) Push(item int) {
	s.items = append(s.items, item)
}

func (s *Stack) Pop() (int, error) {
	if len(s.items) == 0 {
		return -1, errors.New("stack is empty")
	}
	item := s.items[len(s.items)-1]
	s.items = s.items[:len(s.items)-1]
	return item, nil
}

func (s *Stack) Peek() (int, error) {
	if len(s.items) == 0 {
		return -1, errors.New("stack is empty")
	}
	return s.items[len(s.items)-1], nil
}

func (s *Stack) IsEmpty() bool {
	return len(s.items) == 0
}

func (s *Stack) Size() int {
	return len(s.items)
}
