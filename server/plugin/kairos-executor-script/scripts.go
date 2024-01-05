package main

import (
	"encoding/json"
	"fmt"

	"github.com/THPTUHA/kairos/server/plugin"
	"github.com/THPTUHA/kairos/server/plugin/proto"
)

type Script struct {
}

func (s *Script) Execute(args *proto.ExecuteRequest, cb plugin.StatusHelper) (*proto.ExecuteResponse, error) {

	out, err := s.ExecuteImpl(args, cb)
	resp := &proto.ExecuteResponse{
		Output: out,
	}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, err
}

func (s *Script) ExecuteImpl(args *proto.ExecuteRequest, cb plugin.StatusHelper) ([]byte, error) {
	command := args.Config["command"]
	agrsStr := args.Config["commandArgs"]
	inputStr := args.Config["inputs"]
	commandArgs := make([]string, 0)
	inputs := make([]string, 0)

	fmt.Println("SCRIPT---", command, agrsStr, inputStr)
	if agrsStr != "" {
		err := json.Unmarshal([]byte(agrsStr), &commandArgs)
		if err != nil {
			return nil, err
		}
	}

	if inputStr != "" {
		err := json.Unmarshal([]byte(inputStr), &inputs)
		if err != nil {
			return nil, err
		}
	}

	envStr := args.Config["env"]
	envs := make([]string, 0)
	if envStr != "" {
		err := json.Unmarshal([]byte(envStr), &envs)
		if err != nil {
			return nil, err
		}
	}

	launched, err := launchCmd(command, commandArgs, envs)
	if err != nil {
		return nil, err
	}
	process := NewProcessEndpoint(launched)
	defer process.Terminate()

	// inputCh := make(chan []byte)
	process.StartReading()

	// go func() {
	// 	if len(inputs) > 0 {
	// 		for _, i := range inputs {
	// 			inputCh <- []byte(i)
	// 		}
	// 	}
	// 	for {
	// 		inputCh <- cb.Input()
	// 	}
	// }()

	for {
		select {
		case msgOne, ok := <-process.Output():
			if !ok || len(msgOne.data) == 0 {
				return []byte("finish"), nil
			}
			if msgOne.data != nil {
				_, err := cb.Update(msgOne.data, true)
				if err != nil {
					return nil, err
				}
			}
		case errOne, ok := <-process.Error():
			if !ok || len(errOne.data) == 0 {
				return []byte("finish"), nil
			}
			if errOne.data != nil && ok {
				_, err := cb.Update(errOne.data, false)
				if err != nil {
					return nil, err
				}
			}
			// return []byte("fuck"), nil

			// case msgTwo, ok := <-inputCh:
			// 	if !ok || !process.Send(msgTwo) {
			// 		return nil, nil
			// 	}
		}
	}
}
