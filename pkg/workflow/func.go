package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/dop251/goja"
)

type Expression struct {
	Func   string
	Params []string
	Input  []interface{}
}

var (
	ErrorVariableNotReady = errors.New("variable not ready")
	ErrorNotExist         = errors.New("not exist")
)

func extractValue(outputVars map[string]ReplyData, p string) (interface{}, error) {
	r := GetRootDefaultVar(strings.TrimPrefix(p, "."))
	fmt.Println(" outputVars --------- ")
	for k, v := range outputVars {
		fmt.Println(k, v)
	}
	fmt.Println(" outputVars --------- end")
	value, ok := outputVars[r]
	if !ok {
		return "", ErrorNotExist
	}

	if strings.HasSuffix(p, SubTask) || strings.HasSuffix(p, SubChannel) || strings.HasSuffix(p, SubClient) {
		return value, nil
	}

	return "", ErrorNotExist
}

type ValueMap struct {
	params      []string
	globalVar   *Vars
	outputVars  map[string]ReplyData
	tempvarTemp *varTemp
}

func getValueSymbolSpecial(symbol string) (interface{}, error) {
	switch symbol {
	case "_ping":
		return make(map[string]interface{}), nil
	default:
		return nil, fmt.Errorf("symbol %s not exist", symbol)
	}
}

func setValue(vm *ValueMap) (map[string]interface{}, error) {
	mp := make(map[string]interface{})
	for _, p := range vm.params {
		if v, err := getValueSymbolSpecial(p); err == nil {
			mp[p] = v
			continue
		}
		if isContants(p) == nil {
			v := getConstVar(p)
			if strings.HasPrefix(v, `"`) {
				mp[p] = strings.Trim(v, `()"`)
			} else {
				mp[p], _ = strconv.ParseFloat(v, 64)
				fmt.Println("Pare value", v)
			}
			continue
		}

		v := vm.tempvarTemp.GetValue(p)
		if v == nil {
			v, err := extractValue(vm.outputVars, p)
			if err != ErrorNotExist {
				mp[p] = v
				continue
			} else if vm.globalVar.Exists(strings.TrimPrefix(p, ".")) {
				mp[p] = vm.globalVar.Get(strings.TrimPrefix(p, ".")).Value
				continue
			} else if v, ok := GlobalVars[p]; ok {
				mp[p] = v
				continue
			}
		} else {
			mp[p] = v.value
			continue
		}
		fmt.Printf("[ FUNC ] Var = %s not exist\n", p)
		return nil, fmt.Errorf("var %s not ready", p)
	}
	return mp, nil
}

func getConstVar(s string) string {
	re := regexp.MustCompile(`[()]`)
	return re.ReplaceAllString(s, "")
}

func convertInterfaceToReply(s interface{}) (*ReplyData, error) {
	var input ReplyData
	data, _ := json.Marshal(s)
	err := json.Unmarshal(data, &input)
	if err != nil {
		return nil, fmt.Errorf("err: %s , value: %+v", err.Error(), s)
	}
	return &input, err
}

func convertInterfaceToArray(s interface{}) ([]interface{}, error) {
	var input []interface{}
	var data []byte
	_, ok := s.(string)
	if !ok {
		json, _ := json.Marshal(s)
		data = json
	} else {
		data = []byte(s.(string))
	}

	err := json.Unmarshal(data, &input)
	return input, err
}

func (exp *Expression) Execute(globalVar *Vars, outputVars map[string]ReplyData, tempvarTemp *varTemp, funcCall *FuncCall) ([]interface{}, error) {
	vm := ValueMap{
		globalVar:   globalVar,
		outputVars:  outputVars,
		tempvarTemp: tempvarTemp,
	}
	switch exp.Func {
	case IF_EXP, ELSEIF_EXP:
		if len(exp.Params) == 3 {
			vm.params = exp.Params[1:]
			mp, e := setValue(&vm)
			if e != nil {
				return nil, e
			}
			r, err := CompareStringsAndNumbers(mp[exp.Params[1]], mp[exp.Params[2]], EQ_EXP)
			if err != nil {
				return nil, err
			}

			if r {
				return []interface{}{true}, nil
			} else {
				return []interface{}{false}, nil
			}
		} else if len(exp.Params) == 1 {
			vm.params = exp.Params
			mp, e := setValue(&vm)
			if e != nil {
				return nil, e
			}
			if mp[exp.Params[0]] != "" && mp[exp.Params[0]] != false && mp[exp.Params[0]] != "false" {
				return []interface{}{true}, nil
			} else {
				return []interface{}{false}, nil
			}
		} else if len(exp.Params) == 2 {
			vm.params = exp.Params[1:]
			mp, e := setValue(&vm)
			if e != nil {
				return nil, e
			}
			if exp.Params[0] == "ne" {
				if mp[exp.Params[1]] != "" && mp[exp.Params[1]] != false && mp[exp.Params[1]] != "false" {
					return []interface{}{false}, nil
				} else {
					return []interface{}{true}, nil
				}
			}
		}

		if exp.Func == IF_EXP {
			tempvarTemp.Append()
		} else {
			tempvarTemp.ClearTop()
		}
	case GET_EXP:
		var source ReplyData
		vm.params = exp.Params
		vm.tempvarTemp.String()
		mp, err := setValue(&vm)

		get := exp.Params[0]
		if err != nil {
			return nil, err
		}

		if len(exp.Params) == 1 {
			if len(exp.Input) == 0 {
				return nil, fmt.Errorf("%s has no value", exp.Func)
			}
			input, ok := exp.Input[0].(ReplyData)

			if !ok {
				v, err := convertInterfaceToReply(exp.Input[0])
				fmt.Printf("source %+v\n", v)
				if err != nil {
					return nil, fmt.Errorf("input exp %s is not map", exp.Func)
				}
				input = *v
			}
			source = input
		} else {
			input, ok := mp[exp.Params[0]].(ReplyData)
			if !ok {
				v, err := convertInterfaceToReply(mp[exp.Params[0]])
				if err != nil {
					return nil, fmt.Errorf("expression %s param invalid type", exp.Func)
				}
				input = *v
			}
			source = input
			get = exp.Params[1]
		}
		if v, ok := mp[get].(string); !ok {
			return nil, fmt.Errorf("var %s has no value", exp.Params[0])
		} else {
			return []interface{}{source[v]}, nil
		}
	case GETP_EXP:
		vm.params = exp.Params
		var source []interface{}
		mp, err := setValue(&vm)
		if err != nil {
			return nil, err
		}
		get := exp.Params[0]
		if len(exp.Params) == 1 {
			if len(exp.Input) == 0 {
				return nil, fmt.Errorf("%s has no value", exp.Func)
			}
			input, ok := exp.Input[0].([]interface{})

			if !ok {
				v, err := convertInterfaceToArray(exp.Input[0])
				fmt.Printf("array %+v\n", v)
				if err != nil {
					return nil, fmt.Errorf("input exp %s is not array", exp.Func)
				}
				input = v
			}
			source = input
		} else {
			input, ok := exp.Input[0].([]interface{})
			if !ok {
				v, err := convertInterfaceToArray(exp.Input[0])
				fmt.Printf("array %+v\n", source)
				if err != nil {
					return nil, fmt.Errorf("input exp %s is not array", exp.Func)
				}
				input = v
			}
			source = input
			get = exp.Params[1]
		}

		if v, ok := mp[get].(int); !ok {
			return nil, fmt.Errorf("var %s has no value", exp.Params[0])
		} else {
			return []interface{}{source[v]}, nil
		}
	case SET_EXP:
		vm.params = exp.Params
		if len(exp.Params) == 1 {
			if len(exp.Input) == 0 {
				return nil, fmt.Errorf("set has no value")
			}

			tempvarTemp.Set(&kv{
				key:   exp.Params[0],
				value: exp.Input[0],
			})
			return []interface{}{exp.Input[0]}, nil
		} else {
			vm.params = exp.Params[1:]
			mp, e := setValue(&vm)
			if e != nil {
				return nil, e
			}
			tempvarTemp.Set(&kv{
				key:   exp.Params[0],
				value: mp[exp.Params[1]],
			})

			return []interface{}{mp[exp.Params[1]]}, nil
		}
	case SEND, SENDU:
		var v interface{}
		if len(exp.Params) == 1 {
			if len(exp.Input) == 0 {
				return nil, fmt.Errorf("expression %s has no value to send", exp.Func)
			}
			v = exp.Input[0]
		} else {
			vm.params = exp.Params[:1]
			mp, err := setValue(&vm)
			if err != nil {
				return nil, err
			}
			v = mp[exp.Params[0]]
		}

		if v == nil {
			return nil, fmt.Errorf("expression %s: can't send nil value, var %s", exp.Func, exp.Params[0])
		}
		ac, err := convertInterfaceToReply(v)
		if err != nil {
			return nil, errors.New(fmt.Sprintf("Can't send %s, it must be default alias err =%+v", exp.Params[0], err))
		}
		return []interface{}{ac}, nil
	case WAIT_EXP:
		vm.params = exp.Params
		_, e := setValue(&vm)
		if e != nil {
			return nil, e
		}
	case WAITE_EXP:
		vm.params = exp.Params
		mp, e := setValue(&vm)
		if e != nil {
			return nil, e
		}
		if v, ok := mp[exp.Params[0]].(ReplyData); ok {
			result := v["result"]
			if result == nil {
				return nil, fmt.Errorf("invalid reply data, nil result")
			}
			if result, ok := result.(*Result); ok {
				if result.FinishedAt == 0 {
					return nil, fmt.Errorf("unfinished")
				}
				return nil, nil
			}

			if result, ok := result.(map[string]interface{}); ok {
				if f, ok := result["finish"].(bool); !ok {
					if fs, ok := result["finish"].(string); !ok {
						if fs != "true" {
							return nil, fmt.Errorf("unfinished")
						}
					}
				} else if !f {
					return nil, fmt.Errorf("unfinished")
				}
				return nil, nil
			} else {
				return nil, fmt.Errorf("invalid result")
			}

		} else {
			return nil, fmt.Errorf("can't convert to reply")
		}

	case PARSE_EXP:
		vm.params = exp.Params
		mp, err := setValue(&vm)
		if err != nil {
			return nil, err
		}
		items := make([]interface{}, 0)
		for _, e := range exp.Input {
			items = append(items, e)
		}
		for _, e := range exp.Params {
			items = append(items, mp[e])
		}
		return parse(items), nil
	case MERGE_EXP:
		vm.params = exp.Params
		mp, err := setValue(&vm)
		if err != nil {
			return nil, err
		}
		var paramsInterface []interface{}

		for _, e := range exp.Input {
			paramsInterface = append(paramsInterface, e)
		}
		for _, param := range exp.Params {
			paramsInterface = append(paramsInterface, mp[param])
		}
		v, err := merge(paramsInterface)
		if err != nil {
			return nil, err
		}
		return []interface{}{v}, nil
	case DELETE_EXP:
		vm.params = exp.Params
		var rd ReplyData
		var key string
		mp, err := setValue(&vm)
		if err != nil {
			return nil, err
		}
		if len(exp.Params) == 2 {
			src, err := convertInterfaceToReply(mp[exp.Params[0]])
			if err != nil {
				return nil, err
			}
			rd = *src
			k, ok := mp[exp.Params[1]].(string)
			if !ok {
				return nil, fmt.Errorf("exp put: %+v is not string %+v", exp.Params[1], mp[exp.Params[1]])
			}
			key = k
		} else {
			src, err := convertInterfaceToReply(exp.Input[0])
			if err != nil {
				return nil, err
			}
			rd = *src
			k, ok := mp[exp.Params[0]].(string)
			if !ok {
				return nil, fmt.Errorf("exp put: %+v is not string %+v", exp.Params[0], mp[exp.Params[0]])
			}
			key = k
		}
		delete(rd, key)
		return []interface{}{rd}, nil
	case POP_EXP:
		vm.params = exp.Params
		if len(exp.Input) == 0 {
			return nil, fmt.Errorf("can't pop empty")
		}
		input, ok := exp.Input[0].([]interface{})
		if !ok {
			v, err := convertInterfaceToArray(exp.Input[0])
			fmt.Printf("array %+v\n", v)
			if err != nil {
				return nil, fmt.Errorf("input exp %s is not array", exp.Func)
			}
			input = v
		}
		if len(input) == 0 {
			return nil, fmt.Errorf("can't pop array empty")
		}
		input = input[:len(input)-1]
		return []interface{}{input}, nil
	case PUSH_EXP:
		vm.params = exp.Params
		return pushExp(exp, &vm)
	case PRINTF_EXP:
		vm.params = exp.Params
		mp, err := setValue(&vm)
		if err != nil {
			return nil, err
		}
		vals := make([]interface{}, 0)
		for _, p := range exp.Params[1:] {
			vals = append(vals, mp[p])
		}
		return []interface{}{fmt.Sprintf(mp[exp.Params[0]].(string), vals...)}, nil
	case SENDSYNC:
		vm.params = exp.Params
		if len(exp.Params) == 1 {
			return exp.Input, nil
		}
		// tempvarTemp.String()
		vm.params = exp.Params[0:1]
		mp, err := setValue(&vm)
		if err != nil {
			return nil, err
		}
		return []interface{}{mp[exp.Params[0]]}, nil
	case DEFINE:
		vm.params = exp.Params
		switch exp.Params[1] {
		case "map":
			v := make(map[string]interface{})
			tempvarTemp.Push(&kv{
				key:   exp.Params[0],
				value: v,
			})
			return []interface{}{&v}, nil
		case "array":
			v := make([]interface{}, 0)
			tempvarTemp.Push(&kv{
				key:   exp.Params[0],
				value: v,
			})
			return []interface{}{v}, nil
		case "string":
			v := ""
			tempvarTemp.Push(&kv{
				key:   exp.Params[0],
				value: v,
			})
			return []interface{}{v}, nil
		case "number":
			v := 0.0
			tempvarTemp.Push(&kv{
				key:   exp.Params[0],
				value: v,
			})
		}
	case PUT_EXP:
		vm.params = exp.Params
		var rd ReplyData
		var key string
		var value interface{}
		mp, err := setValue(&vm)
		if err != nil {
			return nil, err
		}
		if len(exp.Params) == 3 {
			src, err := convertInterfaceToReply(mp[exp.Params[0]])
			if err != nil {
				return nil, err
			}
			rd = *src
			k, ok := mp[exp.Params[1]].(string)
			if !ok {
				return nil, fmt.Errorf("exp put: %+v is not string value= %+v", exp.Params[1], mp[exp.Params[1]])
			}
			key = k
			value = mp[exp.Params[2]]
		} else if len(exp.Params) == 2 {
			if len(exp.Input) == 0 {
				return nil, fmt.Errorf("exp %s: can't put nil value", exp.Func)
			}
			src, err := convertInterfaceToReply(exp.Input[0])
			if err != nil {
				return nil, err
			}
			rd = *src
			k, ok := mp[exp.Params[0]].(string)
			if !ok {
				return nil, fmt.Errorf("exp put: %+v is not string value= %+v", exp.Params[0], mp[exp.Params[0]])
			}
			key = k
			value = mp[exp.Params[1]]
		} else {
			return nil, fmt.Errorf("exp put: %+v empty queue or value", exp.Params[0])
		}
		rd[key] = value
		return []interface{}{rd}, nil
	case RETURN_EXP:
		vm.params = exp.Params
		if len(exp.Params) == 0 {
			return []interface{}{exp.Input}, nil
		}
		mp, err := setValue(&vm)
		if err != nil {
			return nil, err
		}
		var result []interface{}
		for _, k := range exp.Params {
			result = append(result, mp[k])
		}
		return result, nil
	case ELSE_EXP, END_EXP:
		return nil, nil
	case SUB_EXP:
		vm.params = exp.Params
		var result []interface{}
		var err error
		var r interface{}
		if len(exp.Params) == 2 {
			r, err = subtract(exp.Params[0], exp.Params[1])
		} else if len(exp.Params) == 1 {
			r, err = subtract(exp.Input[0], exp.Params[0])
		} else if len(exp.Input) >= 2 {
			r, err = subtract(exp.Input[0], exp.Input[1])
		} else {
			return nil, fmt.Errorf("exp %s: invalid params", exp.Func)
		}
		if err != nil {
			return nil, err
		}
		result = append(result, r)
		return result, nil
	case RANGE_EXP:
		vm.params = exp.Params[:1]
		var result []interface{}
		mp, err := setValue(&vm)
		if err != nil {
			return nil, err
		}
		iter := mp[exp.Params[0]]
		fmt.Printf("ITER___, %+v param = %s , mapValue= %+v\n", iter, exp.Params[0], mp)
		if r, err := convertInterfaceToReply(iter); err != nil {
			if r, err := convertInterfaceToArray(iter); err != nil {
				log.Error(err)
				return nil, fmt.Errorf("exp %s invalid param %+v", exp.Func, iter)
			} else {
				fmt.Printf("RANGE ARRAY %+v\n", r)
				vm.tempvarTemp.String()
				result = append(result, r)
			}
		} else {
			fmt.Printf("RANGE MAP %+v\n", r)
			result = append(result, r)
		}
		return result, nil
	default:
		if funcCall != nil && funcCall.Funcs != nil {
			vm.params = exp.Params
			f := funcCall.GetFunction(exp.Func)
			if f == nil {
				return nil, fmt.Errorf("expression '%s' not define", exp.Func)
			}
			mp, err := setValue(&vm)
			if err != nil {
				return nil, err
			}
			tempvarTemp.String()
			items := make([]goja.Value, 0)
			params := make([]interface{}, 0)
			for _, e := range exp.Input {
				params = append(params, e)
				items = append(items, funcCall.Funcs.ToValue(e))
			}
			for _, e := range exp.Params {
				params = append(params, mp[e])
				items = append(items, funcCall.Funcs.ToValue(mp[e]))
			}
			fmt.Printf("RUN HERE EXP %+v \n", params)
			result, err := f(goja.Undefined(), items...)
			fmt.Printf("RESULT --- %+v\n", result)
			if err != nil {
				fmt.Printf("RESULT 222--- %+v\n", err)
				return nil, err
			}
			var rt interface{}
			switch result.ExportType().Kind() {
			case reflect.Map, reflect.Array:
				obj := result.ToObject(funcCall.Funcs)
				mp := make(map[string]interface{})
				for _, k := range obj.Keys() {
					mp[k] = obj.Get(k)
				}
				rt = mp
			case reflect.Int64, reflect.Float64:
				rt = result.ToNumber()
			case reflect.String:
				rt = result.ToString()
			default:
				return nil, fmt.Errorf("exp %s: can't support return type %s", exp.Func, result.ExportType().Kind())
			}

			fmt.Printf("RESULT --- %+v ERR= %+v  type=%+v\n", rt, err, result.ExportType())
			return []interface{}{rt}, nil
		} else {
			return nil, fmt.Errorf("expression '%s' not found", exp.Func)
		}
	}
	return nil, nil
}

func parse(input []interface{}) []interface{} {
	result := make([]interface{}, len(input))

	for i, item := range input {
		switch v := item.(type) {
		case string:
			var parsedMap ReplyData
			err := json.Unmarshal([]byte(v), &parsedMap)
			if err == nil {
				result[i] = parsedMap
				continue
			}

			var parsedArray []interface{}
			err = json.Unmarshal([]byte(v), &parsedArray)
			if err == nil {
				result[i] = parsedArray
				continue
			}

			if num, err := strconv.Atoi(v); err == nil {
				result[i] = num
				continue
			}

			result[i] = item

		default:
			result[i] = item
		}
	}

	return result
}

func CompareStringsAndNumbers(a, b interface{}, operator string) (result bool, err error) {
	switch operator {
	case "eq":
		result = reflect.DeepEqual(a, b)
	case "ne":
		result = !reflect.DeepEqual(a, b)
	case "lt", "le", "gt", "ge":
		switch a.(type) {
		case string:
			if reflect.TypeOf(b) != reflect.TypeOf(a) {
				err = fmt.Errorf("Type mismatch: %s and %s", reflect.TypeOf(a), reflect.TypeOf(b))
				return
			}
			cmp := reflect.ValueOf(a).Interface().(string) < reflect.ValueOf(b).Interface().(string)
			switch operator {
			case "lt":
				result = cmp
			case "le":
				result = cmp || reflect.ValueOf(a).Interface().(string) == reflect.ValueOf(b).Interface().(string)
			case "gt":
				result = !cmp
			case "ge":
				result = !cmp || reflect.ValueOf(a).Interface().(string) == reflect.ValueOf(b).Interface().(string)
			}
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
			if reflect.TypeOf(b) != reflect.TypeOf(a) {
				err = fmt.Errorf("Type mismatch: %s and %s", reflect.TypeOf(a), reflect.TypeOf(b))
				return
			}
			switch operator {
			case "lt":
				result = reflect.ValueOf(a).Interface().(float64) < reflect.ValueOf(b).Interface().(float64)
			case "le":
				result = reflect.ValueOf(a).Interface().(float64) <= reflect.ValueOf(b).Interface().(float64)
			case "gt":
				result = reflect.ValueOf(a).Interface().(float64) > reflect.ValueOf(b).Interface().(float64)
			case "ge":
				result = reflect.ValueOf(a).Interface().(float64) >= reflect.ValueOf(b).Interface().(float64)
			}
		default:
			err = fmt.Errorf("Unsupported type: %s", reflect.TypeOf(a))
		}
	default:
		err = fmt.Errorf("Unsupported operator: %s", operator)
	}
	return
}

func subtract(a, b interface{}) (result interface{}, err error) {
	switch a.(type) {
	case int:
		if bInt, ok := b.(int); ok {
			result = a.(int) - bInt
		} else {
			err = fmt.Errorf("Type mismatch: %s and %s", reflect.TypeOf(a), reflect.TypeOf(b))
		}
	case float64:
		if bFloat, ok := b.(float64); ok {
			result = a.(float64) - bFloat
		} else {
			err = fmt.Errorf("Type mismatch: %s and %s", reflect.TypeOf(a), reflect.TypeOf(b))
		}
	default:
		err = fmt.Errorf("Unsupported type: %s", reflect.TypeOf(a))
	}
	return
}

func pushExp(exp *Expression, vm *ValueMap) ([]interface{}, error) {
	var source []interface{}
	mp, err := setValue(vm)
	if err != nil {
		return nil, err
	}
	get := mp[exp.Params[0]]
	if len(exp.Params) == 1 {
		if len(exp.Input) == 0 {
			return nil, fmt.Errorf("%s has no value", exp.Func)
		}
		input, ok := exp.Input[0].([]interface{})

		if !ok {
			v, err := convertInterfaceToArray(exp.Input[0])
			if err != nil {
				return nil, fmt.Errorf("input exp %s is not array", exp.Func)
			}
			input = v
		}
		source = input
	} else {
		input, ok := mp[exp.Params[0]].([]interface{})
		if !ok {
			v, err := convertInterfaceToArray(exp.Params[0])
			if err != nil {
				return nil, fmt.Errorf("input exp %s is not array", exp.Func)
			}
			input = v
		}
		source = input
		get = mp[exp.Params[1]]
	}

	source = append(source, get)
	return []interface{}{source}, nil
}

func merge(input []interface{}) (interface{}, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("merge: empty input")
	}

	firstElementType := reflect.TypeOf(input[0])

	switch firstElementType.Kind() {
	case reflect.Float64:
		sum := 0.0
		for _, v := range input {
			k := reflect.TypeOf(v).Kind()
			if k != reflect.Float64 {
				return nil, fmt.Errorf("can't merge type %v and %v value=%v", firstElementType.Kind(), k, v)
			}
			sum += reflect.ValueOf(v).Convert(reflect.TypeOf(sum)).Float()
		}
		return sum, nil
	case reflect.Int, reflect.Int64:
		var sum int64
		for _, v := range input {
			k := reflect.TypeOf(v).Kind()
			if k != reflect.Int && k != reflect.Int64 {
				return nil, fmt.Errorf("can't merge type %v and %v", firstElementType.Kind(), k)
			}
			sum += reflect.ValueOf(v).Convert(reflect.TypeOf(sum)).Int()
		}
		return sum, nil
	case reflect.String:
		concatenated := ""
		for _, v := range input {
			k := reflect.TypeOf(v).Kind()
			if k != reflect.String {
				return nil, fmt.Errorf("can't merge type %v and %v", firstElementType.Kind(), k)
			}
			concatenated += reflect.ValueOf(v).String()
		}
		return concatenated, nil
	case reflect.Array, reflect.Slice:
		result := make([]interface{}, 0)
		for _, v := range input {
			k := reflect.TypeOf(v).Kind()
			if k != reflect.Slice {
				return nil, fmt.Errorf("can't merge type %v and %v", firstElementType.Kind(), k)
			}
			result = append(result, v)
		}
		return result, nil
	case reflect.Map:
		result := make(map[interface{}]interface{})
		for _, v := range input {
			k := reflect.TypeOf(v).Kind()
			if k != reflect.Map {
				return nil, fmt.Errorf("can't merge type %v and %v", firstElementType.Kind(), k)
			}
			for _, key := range reflect.ValueOf(v).MapKeys() {
				result[key.Interface()] = reflect.ValueOf(v).MapIndex(key).Interface()
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("merge not support type %+v", firstElementType.Kind())
	}
}
