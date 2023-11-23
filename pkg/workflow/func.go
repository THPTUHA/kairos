package workflow

import (
	"errors"
	"fmt"
	"strings"
)

type Expression struct {
	Func   string
	Params []string
	Input  interface{}
}

var (
	ErrorVariableNotReady = errors.New("variable not ready")
	ErrorNotExist         = errors.New("not exist")
)

func setDeafultValue(outputVars map[string]*CmdReplyTask, p string, v string) (string, error) {
	r := GetRootDefaultVar(strings.TrimPrefix(p, "."))
	// todo change default var
	value, ok := outputVars[r]
	if !ok {
		return "", ErrorNotExist
	}

	if strings.HasSuffix(p, SubTaskOutPut) {
		fmt.Printf("[DEBUG SUB TASK OUTPUT]\n")
		value.Result.Output = v
		outputVars[p] = value
		return value.Result.Output, nil
	}

	if strings.HasSuffix(p, SubChannelMSG) {
		value.Content.Message = v
		outputVars[p] = value
		return value.Content.Message, nil
	}

	if strings.HasSuffix(p, SubChannelCMD) {
		value.Content.Cmd = v
		outputVars[p] = value
		return value.Content.Cmd, nil
	}

	return "", ErrorNotExist
}

func extractValue(outputVars map[string]*CmdReplyTask, p string) (interface{}, error) {
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

	if strings.HasSuffix(p, SubTaskOutPut) {
		return value.Result.Output, nil
	}

	if strings.HasSuffix(p, SubTaskSuccess) {
		fmt.Printf("[DEBUG OUTPUT ] result = %+v, r= %s\n", value.Result, r)
		if value.Result.Success {
			return "success", nil
		}
		return "", nil
	}

	if strings.HasSuffix(p, SubChannelMSG) {
		return value.Content.Message, nil
	}

	if strings.HasSuffix(p, SubChannelCMD) {
		return value.Content.Cmd, nil
	}

	return "", ErrorNotExist
}

func setValue(params []string, globalVar *Vars, outputVars map[string]*CmdReplyTask, tempvarTemp *varTemp) (map[string]interface{}, error) {
	mp := make(map[string]interface{})
	for _, p := range params {
		var exist bool
		v := tempvarTemp.GetValue(p)
		if v == nil {
			v, err := extractValue(outputVars, p)
			if err != ErrorNotExist {
				mp[p] = v
				exist = true
			} else if globalVar.Exists(strings.TrimPrefix(p, ".")) {
				mp[p] = globalVar.Get(strings.TrimPrefix(p, ".")).Value
				exist = true
			} else if v, ok := GlobalVars[p]; ok {
				mp[p] = v
				exist = true
			}
		} else {
			exist = true
		}
		if !exist {
			fmt.Printf("[ FUNC ] Var = %s not exist\n", p)
			return nil, ErrorVariableNotReady
		}
	}
	return mp, nil
}

func (exp *Expression) Execute(globalVar *Vars, outputVars map[string]*CmdReplyTask, tempvarTemp *varTemp) (interface{}, error) {
	fmt.Printf("EXPRESS  %+v\n", exp)
	switch exp.Func {
	case "if", "elseif":
		if exp.Params[0] == "eq" {
			mp, e := setValue(exp.Params[1:], globalVar, outputVars, tempvarTemp)
			if e != nil {
				return nil, e
			}
			if mp[exp.Params[1]] == mp[exp.Params[2]] {
				return true, nil
			} else {
				return false, nil
			}
		} else if len(exp.Params) == 1 {
			mp, e := setValue(exp.Params, globalVar, outputVars, tempvarTemp)
			if e != nil {
				return nil, e
			}
			// fmt.Println("DEBUG FUCKING SUCESS ", mp[exp.Params[0]], exp.Params[0])
			if mp[exp.Params[0]] != "" {
				return true, nil
			} else {
				return false, nil
			}
		}

		if exp.Func == "if" {
			tempvarTemp.Append()
		} else {
			tempvarTemp.ClearTop()
		}
	case "set":
		mp, e := setValue(exp.Params[1:], globalVar, outputVars, tempvarTemp)
		if e != nil {
			return nil, e
		}
		v, ok := mp[exp.Params[1]].(string)
		if !ok {
			return nil, errors.New(fmt.Sprintf("Can't assign value %+v to string %s", exp.Params[1], exp.Params[0]))
		}
		_, e = setDeafultValue(outputVars, exp.Params[0], v)
		if e != nil {
			tempvarTemp.Push(&kv{
				key:   exp.Params[0],
				value: mp[exp.Params[1]],
			})
		}
	case "send":
		v, err := extractValue(outputVars, exp.Params[0])
		if err != nil {
			return nil, err
		}
		ac, ok := v.(*CmdReplyTask)
		if !ok {
			return nil, errors.New(fmt.Sprintf("Can't send %s, it must be default alias", exp.Params[0]))
		}
		return &DeliverFlow{
			From:     strings.ToLower(strings.TrimPrefix(exp.Params[0], ".")),
			Reciever: strings.ToLower(strings.TrimPrefix(exp.Params[1], ".")),
			Msg:      ac,
		}, nil
	case "wait":
		// TODO check params
		_, e := setValue(exp.Params, globalVar, outputVars, tempvarTemp)
		if e != nil {
			return nil, e
		}

	case "sum":
		if exp.Input != nil {
			// mp, e := setValue(exp.Params, globalVar, outputVars, tempvarTemp)
			// if e != nil {
			// 	return nil, e
			// }
			// input, ok := exp.Input.(int)

		}
	}
	return nil, nil
}
