package main

import (
	"io"
	"os/exec"
)

type LaunchedProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
}

func launchCmd(commandName string, commandArgs []string, env []string) (*LaunchedProcess, error) {
	cmd := exec.Command(commandName, commandArgs...)
	cmd.Env = env

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	return &LaunchedProcess{cmd, stdin, stdout, stderr}, err
}
