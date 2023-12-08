package main

import (
	"encoding/json"
	"fmt"

	kplugin "github.com/THPTUHA/kairos/server/plugin"
	"github.com/THPTUHA/kairos/server/plugin/proto"
)

type Script struct {
}

func (s *Script) Execute(args *proto.ExecuteRequest, cb kplugin.StatusHelper) (*proto.ExecuteResponse, error) {

	out, err := s.ExecuteImpl(args, cb)
	resp := &proto.ExecuteResponse{Output: out}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

func (s *Script) ExecuteImpl(args *proto.ExecuteRequest, cb kplugin.StatusHelper) ([]byte, error) {
	command := args.Config["command"]
	agrsStr := args.Config["commandArgs"]
	inputStr := args.Config["inputs"]
	commandArgs := make([]string, 0)
	inputs := make([]string, 0)

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

	inputCh := make(chan []byte)
	process.StartReading()

	go func() {
		if len(inputs) > 0 {
			for _, i := range inputs {
				inputCh <- []byte(i)
			}
		}
		for {
			inputCh <- cb.Input()
		}
	}()

	for {
		select {
		case msgOne, ok := <-process.Output():
			fmt.Println(" msgOne-----", msgOne, ok)
			if !ok {
				// cb.Update(msgOne, false)
				return nil, nil
			}

			_, err := cb.Update(msgOne, true)
			if err != nil {
				return nil, err
			}
		case msgTwo, ok := <-inputCh:
			if !ok || !process.Send(msgTwo) {
				return nil, nil
			}
		}
	}
}
