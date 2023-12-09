package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/THPTUHA/kairos/pkg/logger"
	"github.com/dop251/goja"
	"github.com/sirupsen/logrus"
)

var log *logrus.Entry

func init() {
	log = logger.InitLogger("debug", "expression")
}

var (
	compareExp = []string{"eq", "gt", "lt"}
	errStack   = errors.New("stack err")
)

type FuncCall struct {
	call  map[string]goja.Callable
	funcs *goja.Runtime
}
type Template struct {
	// Kiếm tra biến
	ListenVars   map[string]bool
	Vars         *Vars
	IndexRun     map[int]int
	Input        []string
	Exps         [][]*Expression
	FuncCalls    *FuncCall
	FuncNotFound []string
}

func includes[T comparable](source []T, e T) bool {
	for _, s := range source {
		if s == e {
			return true
		}
	}
	return false
}

func NewTemplate(funcs *goja.Runtime) *Template {
	return &Template{
		ListenVars: make(map[string]bool),
		IndexRun:   make(map[int]int),
		Exps:       make([][]*Expression, 0),
		FuncCalls: &FuncCall{
			call:  make(map[string]goja.Callable),
			funcs: funcs,
		},
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
	SubClientTaskName   = "_client_task_name"
	SubClientTaskResult = "_client_task_result"
	SubClientTaskOutPut = "_client_task_output"
	SubClients          = []string{
		SubClientMSG,
		SubClientTaskResult,
		SubClientTaskOutPut,
		SubClientTaskName,
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
	isExist := false
	if strings.HasSuffix(v, SubTask) || strings.HasSuffix(v, SubChannel) || strings.HasSuffix(v, SubClient) {
		isExist = true
		t.ListenVars[v] = true
	}

	for _, e := range SubTasks {
		if strings.HasSuffix(v, e) {
			_v := getVarName(strings.TrimSuffix(v, strings.TrimPrefix(e, SubTask)))
			if !includes(t.Input, strings.ToLower(_v)) {
				return errors.New(fmt.Sprintf("task: %s not listened by broker (variable %s)", _v, v))
			}
			isExist = true
			t.ListenVars[v] = true
		}
	}

	for _, e := range SubClients {
		if strings.HasSuffix(v, e) {
			_v := getVarName(strings.TrimSuffix(v, strings.TrimPrefix(e, SubTask)))
			if !includes(t.Input, strings.ToLower(_v)) {
				return errors.New(fmt.Sprintf("client: %s not listened by broker (variable %s)", _v, v))
			}
			isExist = true
			t.ListenVars[v] = true
		}
	}

	for _, e := range SubChannels {
		if strings.HasSuffix(v, e) {
			_v := getVarName(strings.TrimSuffix(v, strings.TrimPrefix(e, SubChannel)))
			if !includes(t.Input, strings.ToLower(_v)) {
				return errors.New(fmt.Sprintf("channel: %s not listened by broker (variable %s)", _v, v))
			}
			isExist = true
			t.ListenVars[v] = true
		}
	}

	if !isExist {
		return fmt.Errorf("exp %s: %s is not define", exp, v)
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

func isContants(s string) error {
	if !strings.HasPrefix(s, "(") || !strings.HasSuffix(s, ")") {
		return fmt.Errorf("params %s: is not const", s)
	}
	return nil
}

func (t *Template) checkValidParam(items []string, varTemps *varTemp) error {
	for _, e := range items {

		if !strings.HasPrefix(e, ".") {
			if err := isContants(e); err != nil {
				return err
			}
			return nil
		}
		if t.Vars != nil && t.Vars.Exists(strings.TrimPrefix(e, ".")) {
			return nil
		}
		if includes(t.Input, strings.TrimPrefix(e, ".")) {
			return nil
		}

		if !existIn(varTemps, e) {
			return errors.New(fmt.Sprintf("%s: stack err, variable = %s has not define", items[0], e))
		}
	}
	return nil
}

const (
	IF_EXP     = "if"
	ELSE_EXP   = "else"
	ELSEIF_EXP = "elseif"

	SET_EXP   = "set"
	SUB_EXP   = "sub"
	MERGE_EXP = "merge"
	RANGE_EXP = "range"
	GET_EXP   = "get"
	GETP_EXP  = "getp"

	PUT_EXP  = "put"
	PUSH_EXP = "push"

	DELETE_EXP = "delete"
	POP_EXP    = "pop"

	PARSE_EXP = "parse"

	SEND     = "send"
	SENDU    = "sendu"
	SENDSYNC = "sendsync"
	WAIT_EXP = "wait"

	EQ_EXP = "eq"
	NE_EXP = "ne"
	LT_EXP = "lt"
	LE_EXP = "le"
	GT_EXP = "gt"
	GE_EXP = "ge"

	END_EXP    = "end"
	BREAK_EXP  = "break"
	RETURN_EXP = "return"
	DEFINE     = "define"

	PRINTF_EXP = "printf"
)

var OPEN_EXP = []string{
	MERGE_EXP, RANGE_EXP, PUT_EXP, DELETE_EXP, POP_EXP, IF_EXP, GET_EXP, PUSH_EXP,
}

var COMPARE_EXP = []string{
	EQ_EXP, NE_EXP, LT_EXP, LE_EXP, GT_EXP, GE_EXP,
}

var FUNC_BUILDIN = []string{
	SUB_EXP, MERGE_EXP,
}

var FUNC_CAN_EMPTY_INPUT = []string{
	ELSE_EXP, END_EXP, RETURN_EXP, PARSE_EXP,
}

func (t *Template) complieChainExps(items []string, varTemps *varTemp, restrict bool) ([]*Expression, int, error) {
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
	var open int
	for idx, c := range chain {
		if len(c) == 1 {
			if strings.HasPrefix(c[0], ".") {
				if idx == 0 {
					if err := t.checkValidParam(c, varTemps); err != nil {
						return nil, -1, fmt.Errorf("%s exp_num = %d", err, idx)
					}
				}
				varTemps.Push(&kv{
					key: strings.ToLower(c[0]),
				})
				exps = append(exps, &Expression{
					Func:   "set",
					Params: c,
				})
				continue
			} else if includes(FUNC_CAN_EMPTY_INPUT, c[0]) {

			} else if t.FuncCalls.funcs != nil {
				var funcC goja.Callable
				f := t.FuncCalls.funcs.Get(c[0])
				if f == nil {
					if restrict {
						return nil, -1, fmt.Errorf("exp %s not define, can't find", c[0])
					}

				}
				err := t.FuncCalls.funcs.ExportTo(f, &funcC)
				if err != nil {
					return nil, -1, fmt.Errorf("exp %s can't export %+v", c[0], err)
				}
				t.FuncCalls.call[c[0]] = funcC
				exps = append(exps, &Expression{
					Func:   c[0],
					Params: make([]string, 0),
				})
				continue
			}
		}

		n := len(c)
		switch c[0] {
		case IF_EXP, ELSEIF_EXP:
			if idx > 0 {
				return nil, -1, fmt.Errorf("exp: %s cannot use as pipeline", c[0])
			}
			if n == 1 {
				return nil, -1, fmt.Errorf("exp: %s empty", c[0])
			}

			if !includes(COMPARE_EXP, c[1]) && !includes(FUNC_BUILDIN, c[1]) {
				var f goja.Callable
				v := t.FuncCalls.funcs.Get(c[1])
				if v == nil {
					if restrict {
						return nil, -1, fmt.Errorf("exp %s: not exist", c[1])
					}
					t.FuncNotFound = append(t.FuncNotFound, c[0])
					return nil, 0, nil
				}
				err := t.FuncCalls.funcs.ExportTo(v, &f)
				if err != nil {
					return nil, -1, err
				}
			}

			if err := t.checkValidParam(c[2:], varTemps); err != nil {
				return nil, -1, err
			}

			if c[0] == IF_EXP {
				open = 1
			} else if err := varTemps.ClearTop(); err != nil {
				return nil, -1, err
			}

		case ELSE_EXP:
			if idx != 0 || len(c) != 1 {
				return nil, -1, fmt.Errorf("exp: %s error", c[0])
			}
			if err := varTemps.ClearTop(); err != nil {
				return nil, -1, err
			}
		case END_EXP:
			if len(c) > 1 {
				return nil, -1, fmt.Errorf("exp %s: no params", c[0])
			}
			if err := varTemps.Pop(); err != nil {
				return nil, -1, err
			}
			open--
		case SET_EXP, DEFINE:
			if len(c) != 3 {
				return nil, -1, fmt.Errorf("exp %s: invalid params", c[0])
			}
			if c[0] == DEFINE {
				if c[2] != "map" && c[2] != "array" && c[2] != "number" && c[2] != "string" {
					return nil, -1, fmt.Errorf("exp %s: not support type %s", c[0], c[2])
				}
			}
			varTemps.Push(&kv{
				key: strings.ToLower(c[1]),
			})
		case RANGE_EXP:
			if idx > 0 {
				return nil, -1, fmt.Errorf("exp: %s cannot use as pipeline", c[0])
			}

			if err := t.checkValidParam(c[1:2], varTemps); err != nil {
				return nil, -1, err
			}
			varTemps.Push(&kv{
				key: c[2],
			})
			varTemps.Push(&kv{
				key: c[3],
			})
			open++
		case SUB_EXP, MERGE_EXP:
			if idx == 0 && len(c) == 2 {
				return nil, -1, fmt.Errorf("exp: %s invalid params", c[0])
			}
			if err := t.checkValidParam(c[1:], varTemps); err != nil {
				return nil, -1, err
			}
			open++
		case PARSE_EXP:
			if idx == 0 && len(c) == 1 {
				return nil, -1, fmt.Errorf("exp: %s invalid params", c[0])
			}

		case BREAK_EXP:
			if idx > 0 {
				return nil, -1, fmt.Errorf("exp: %s cannot use as pipeline", c[0])
			}
			if err := varTemps.Pop(); err != nil {
				return nil, -1, err
			}
		case WAIT_EXP:
			// for _, e := range c[1:] {
			// 	if !t.Vars.Exists(strings.TrimPrefix(c[1], ".")) {
			// 		return nil, -1, fmt.Errorf("exp: %s invalid param %s", c[0], e)
			// 	}
			// }
		case PRINTF_EXP:
			if err := isContants(c[1]); err != nil {
				return nil, -1, err
			}
		case SEND, SENDU, SENDSYNC:
			if len(c) == 1 || len(c) > 3 {
				return nil, -1, fmt.Errorf("exp: %s invalid params", c[0])
			}
			send := c[1]
			if len(c) == 3 {
				if err := t.checkValidParam(c[1:1], varTemps); err != nil {
					return nil, -1, err
				}
				send = c[2]
			}

			for _, e := range strings.Split(send, ",") {
				if !(strings.HasSuffix(e, SubTask) &&
					strings.HasSuffix(e, SubClient)) {
					if strings.HasSuffix(e, SubChannel) {
						if strings.HasSuffix(strings.Split(e, ":")[0], SubTask) {
							return nil, -1, fmt.Errorf("exp: %s invalid param %s", c[0], e)
						}
					}
				}
			}
		case GET_EXP, PUT_EXP, DELETE_EXP, POP_EXP, GETP_EXP:
			if len(c) == 1 {
				return nil, -1, fmt.Errorf("exp: %s missing param", c[0])
			}
			if err := t.checkValidParam(c[1:], varTemps); err != nil {
				return nil, -1, err
			}
			open++
		case RETURN_EXP:
		case PUSH_EXP:
			open++
		default:
			if t.FuncCalls != nil && t.FuncCalls.funcs != nil {
				var f goja.Callable
				v := t.FuncCalls.funcs.Get(c[0])
				if v == nil {
					if restrict {
						return nil, -1, fmt.Errorf("exp %s: not exist", c[0])
					}
					t.FuncNotFound = append(t.FuncNotFound, c[0])
					return nil, 0, nil
				}
				err := t.FuncCalls.funcs.ExportTo(v, &f)
				if err != nil {
					return nil, -1, err
				}
				t.FuncCalls.call[c[0]] = f
			} else {
				return nil, -1, fmt.Errorf("exp %s is not define", c[0])
			}
		}
		exps = append(exps, &Expression{
			Func:   c[0],
			Params: c[1:],
		})
	}
	fmt.Printf("open=%d chain=%+v\n", open, chain)
	if open > 0 {
		open = 1
		varTemps.Append()
	}

	return exps, open, nil
}

type kv struct {
	key   string
	value interface{}
}

type varTemp struct {
	items [][]*kv
}

func (vt *varTemp) String() {
	fmt.Println("log varTemp start")
	for _, e := range vt.items {
		fmt.Println("next stack---")
		for _, k := range e {
			fmt.Println(k.key, k.value)
		}
	}
	fmt.Println("log varTemp end")
}

func (vt *varTemp) Push(kv *kv) {
	fmt.Println("push...", kv.key, kv.value)
	vt.items[len(vt.items)-1] = append(vt.items[len(vt.items)-1], kv)
}

func (vt *varTemp) Set(kv *kv) {
	fmt.Println("set...", kv.key, kv.value)
	exist := false
	for idx := len(vt.items) - 1; idx >= 0; idx-- {
		v := vt.items[idx]
		for edx := len(v) - 1; edx >= 0; edx-- {
			t := v[edx]
			if t.key == kv.key {
				t.value = kv.value
				vt.items[idx][edx] = kv
				exist = true
				break
			}
		}
	}
	if !exist {
		vt.items[len(vt.items)-1] = append(vt.items[len(vt.items)-1], kv)
	}
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
	if vt == nil {
		return nil
	}
	for idx := len(vt.items) - 1; idx >= 0; idx-- {
		v := vt.items[idx]
		for edx := len(v) - 1; edx >= 0; edx-- {
			t := v[edx]
			if t.key == key {
				return t
			}
		}
	}
	return nil
}

func (t *Template) complieExp(exp string, input []string, varTemps *varTemp, restrict bool) ([]*Expression, int, error) {
	exp = strings.Trim(exp, " ")
	if exp == "" {
		return nil, -1, errors.New(fmt.Sprintf("empty expression "))
	}

	re := regexp.MustCompile(`\([^)]*\)|\S+`)

	items := re.FindAllString(exp, -1)
	for idx, e := range items {
		if strings.HasPrefix(e, ".") || strings.HasPrefix(e, "(") {
			items[idx] = strings.ToLower(e)
		}
	}
	exps, open, err := t.complieChainExps(items, varTemps, restrict)
	if err != nil {
		return nil, -1, err
	}
	return exps, open, nil
}

func (t *Template) Parse(str string, input []string, globalVar *Vars, restrict bool) error {
	t.Vars = globalVar
	open := 1
	close := 1
	expStr := ""
	var varTemps varTemp
	buf := make([]*kv, 0)
	varTemps.items = append(varTemps.items, buf)
	openExp := 0
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
				exp, ope, err := t.complieExp(expStr, input, &varTemps, restrict)
				if err != nil {
					return err
				}
				t.Exps = append(t.Exps, exp)
				expStr = ""
				openExp += ope
				if openExp < 0 {
					return fmt.Errorf("redundancy end")
				}
			}
		default:
			expStr += string(c)
		}
	}
	if openExp != 0 {
		return fmt.Errorf("missing end")
	}

	return nil
}

func (t *Template) Build(input []string, s string, vars *Vars, restrict bool) error {
	t.Input = input
	err := t.Parse(s, input, vars, restrict)
	if err != nil {
		return err
	}
	stack := Stack{}
	for idx, exp := range t.Exps {
		for _, e := range exp {
			if includes(OPEN_EXP, e.Func) {
				fmt.Println("open ccc")
				stack.Push(idx)
				break
			} else if e.Func == ELSEIF_EXP || e.Func == ELSE_EXP || e.Func == BREAK_EXP {
				fmt.Println("pop ccc")
				jum, err := stack.Pop()
				if err != nil {
					log.Error(err)
					return err
				}
				t.IndexRun[jum] = idx
				stack.Push(idx)
				break
			} else if e.Func == END_EXP {
				fmt.Printf("end ccc %+v \n", e)
				jum, err := stack.Pop()
				if err != nil {
					log.Error(err)
					return err
				}
				t.IndexRun[jum] = idx
				break
			}
		}
	}

	// TODO build
	for k, v := range t.IndexRun {
		fmt.Println(k, v)
	}
	return nil
}

type DeliverFlow struct {
	Reciever string      `json:"reciever"`
	Send     string      `json:"send"`
	Msg      interface{} `json:"msg"`
}

type ReplyData map[string]interface{}

type RangeValue struct {
	Value      []*kv
	StartIndex int
	Index      int
	EndIndex   int
	KeyVar     string
	ValueVar   string
}
type TemplateRuntime struct {
	expIndex   string
	outputVars map[string]ReplyData
	varTemp    *varTemp
	golbalVar  *Vars
	exps       [][]*Expression
	indexRun   map[int]int
	ExpInput   []interface{}
	FuncCall   *FuncCall
	RangeValue *RangeValue
}

func (t *Template) NewRutime(outputVars map[string]ReplyData) *TemplateRuntime {
	vt := varTemp{}
	vt.Append()
	return &TemplateRuntime{
		golbalVar:  t.Vars,
		varTemp:    &vt,
		outputVars: outputVars,
		exps:       t.Exps,
		indexRun:   t.IndexRun,
		expIndex:   "0-0",
		ExpInput:   make([]interface{}, 0),
		FuncCall:   t.FuncCalls,
	}
}

type TrackLog struct {
	Err string `json:"err"`
	Exp string `json:"exp"`
}
type Tracking struct {
	Err  string      `json:"err"`
	Logs []*TrackLog `json:"tracks"`
}

type ExecOutput struct {
	DeliverFlows []*DeliverFlow `json:"deliver_flows,omitempty"`
	Tracking     *Tracking      `json:"tracking,omitempty"`
	Result       []byte         `json:"result,omitempty"`
}

func (t *TemplateRuntime) Execute() *ExecOutput {
	dfs := make([]*DeliverFlow, 0)
	index := strings.Split(t.expIndex, "-")
	rowIdx, _ := strconv.Atoi(index[0])
	expIdx, _ := strconv.Atoi(index[1])
	rangeStack := make([]*RangeValue, 0)
	var exeOutput = ExecOutput{
		DeliverFlows: make([]*DeliverFlow, 0),
		Tracking:     &Tracking{},
	}

	for idx := rowIdx; idx < len(t.exps); idx++ {
		exp := t.exps[idx]
		for _, e := range exp {
			if includes(OPEN_EXP, e.Func) {
				t.varTemp.Append()
				break
			}
		}

		for edx := expIdx; edx < len(exp); edx++ {
			e := exp[edx]
			e.Input = t.ExpInput
			out, err := e.Execute(t.golbalVar, t.outputVars, t.varTemp, t.FuncCall)

			fmt.Println(e.Func, out, err)

			if err != nil {
				fmt.Println("ERR R-----", err)
				exeOutput.Tracking.Logs = append(exeOutput.Tracking.Logs, &TrackLog{
					Err: err.Error(),
					Exp: fmt.Sprintf("expression number %+v: %s", idx+1, fmt.Sprintf("%s %s", e.Func, strings.Join(e.Params, " "))),
				})
			}
			if err != nil && e.Func == WAIT_EXP {
				return &exeOutput
			}

			if e.Func == SENDSYNC || e.Func == SEND || e.Func == SENDU {
				if err != nil {
					return &exeOutput
				}
				if len(out) == 0 {
					return &exeOutput
				}
				var df DeliverFlow
				if len(e.Params) == 2 {
					df.Reciever = strings.Trim(e.Params[1], ".")
				} else {
					df.Reciever = strings.Trim(e.Params[0], ".")
				}
				df.Send = e.Func
				df.Msg = out[0]
				t.expIndex = fmt.Sprintf("%d-%d", idx, (edx + 1))
				dfs = append(dfs, &df)

				if e.Func == SENDSYNC {
					exeOutput.DeliverFlows = dfs
					return &exeOutput
				}
				continue
			}

			if e.Func == RANGE_EXP && err == nil {
				rangeV := RangeValue{
					Index:      0,
					EndIndex:   t.indexRun[idx],
					KeyVar:     e.Params[1],
					ValueVar:   e.Params[2],
					StartIndex: idx,
				}
				t.RangeValue = &rangeV
				value := out[0]
				fmt.Printf("OUTPUT RANGE TASK %+v \n", value)
				if maps, ok := value.(map[string]interface{}); !ok {
					if maps, ok := value.(*ReplyData); !ok {
						if arrs, ok := value.([]interface{}); !ok {
						} else {
							value := make([]*kv, 0)
							for k, v := range arrs {
								fmt.Printf("RANGE ARRAY KEY = %d VALUE= %+v\n", k, v)
								value = append(value, &kv{key: fmt.Sprint(k), value: v})
							}
							t.RangeValue.Value = value
						}
					} else {
						value := make([]*kv, 0)
						for k, v := range *maps {
							fmt.Printf("RANGE RELPY KEY = %s VALUE= %+v\n", k, v)
							value = append(value, &kv{key: k, value: v})
						}
						t.RangeValue.Value = value
					}

				} else {
					value := make([]*kv, 0)
					for k, v := range maps {
						fmt.Printf(" RANGE MAP KEY = %s VALUE= %+v\n", k, v)
						value = append(value, &kv{key: k, value: v})
					}
					t.RangeValue.Value = value
				}
				if len(rangeV.Value) == 0 {
					idx = t.indexRun[idx] - 1
				} else {
					t.varTemp.Push(&kv{
						key:   rangeV.KeyVar,
						value: rangeV.Value[0].key,
					})
					t.varTemp.Push(&kv{
						key:   rangeV.ValueVar,
						value: rangeV.Value[0].value,
					})
					rangeV.Index++
					rangeStack = append(rangeStack, &rangeV)
				}

			}

			if e.Func == RETURN_EXP {
				v, _ := json.Marshal(out)
				exeOutput.Result = v
				return &exeOutput
			}

			if e.Func == END_EXP {
				// fmt.Println("BEGIN.............")
				// t.varTemp.String()
				t.varTemp.Pop()
				fmt.Println("END--------------------", idx)
				// fmt.Println("END.............")
				// t.varTemp.String()
				if t.RangeValue != nil && t.RangeValue.EndIndex == idx {
					if len(t.RangeValue.Value)-1 >= t.RangeValue.Index {
						fmt.Printf("GO HERE RANGE %+v\n", t.RangeValue)
						idx = t.RangeValue.StartIndex
						rangeV := t.RangeValue
						t.varTemp.Append()
						t.varTemp.Push(&kv{
							key:   rangeV.KeyVar,
							value: rangeV.Value[t.RangeValue.Index].key,
						})
						t.varTemp.Push(&kv{
							key:   rangeV.ValueVar,
							value: rangeV.Value[t.RangeValue.Index].value,
						})
						t.RangeValue.Index++
					} else {
						if len(rangeStack) > 0 {
							rangeStack = rangeStack[:len(rangeStack)-1]
							rsl := len(rangeStack)
							if rsl == 0 {
								t.RangeValue = nil
							} else {
								t.RangeValue = rangeStack[rsl-1]
								if len(t.RangeValue.Value)-1 >= t.RangeValue.Index {
									t.varTemp.Pop()
									idx = t.RangeValue.StartIndex
									rangeV := t.RangeValue
									t.varTemp.Append()
									t.varTemp.Push(&kv{
										key:   rangeV.KeyVar,
										value: rangeV.Value[t.RangeValue.Index].key,
									})
									t.varTemp.Push(&kv{
										key:   rangeV.ValueVar,
										value: rangeV.Value[t.RangeValue.Index].value,
									})
									t.RangeValue.Index++
								}
							}
						} else {
							fmt.Printf("END RANGE %+v\n", t.RangeValue)
						}

					}
				}
			}

			if e.Func == BREAK_EXP {
				t.RangeValue = nil
			}

			if err != nil && t.indexRun[idx] == 0 {
				fmt.Printf("ERR EXP %+v func=%s \n", err, e.Func)
				exeOutput.Tracking.Err = err.Error()
				return &exeOutput
			}

			if e.Func == ELSE_EXP {
				idx = t.indexRun[idx] - 1
				break
			}

			if (len(out) == 1 && (out[0] == "" || out[0] == false) || err != nil) &&
				t.indexRun[idx] > 0 {
				fmt.Println("RUN FORWARD ERR", err)
				fmt.Printf("EXP ERR %+v\n", e)
				idx = t.indexRun[idx]
				break
			}

			t.ExpInput = out

			if err != nil {
				log.Error(err)
				return &exeOutput
			}
		}

		t.ExpInput = []interface{}{}
	}
	exeOutput.DeliverFlows = dfs
	return &exeOutput
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
