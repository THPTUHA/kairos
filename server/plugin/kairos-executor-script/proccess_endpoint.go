package main

import (
	"bufio"
	"fmt"
	"io"
	"syscall"
	"time"
)

type Output struct {
	data []byte
}

type Error struct {
	data []byte
}
type ProcessEndpoint struct {
	process   *LaunchedProcess
	closetime time.Duration
	output    chan *Output
	err       chan *Error
}

func NewProcessEndpoint(process *LaunchedProcess) *ProcessEndpoint {
	return &ProcessEndpoint{
		process: process,
		output:  make(chan *Output),
		err:     make(chan *Error),
	}
}

func (pe *ProcessEndpoint) Terminate() {
	terminated := make(chan struct{})
	go func() { pe.process.cmd.Wait(); terminated <- struct{}{} }()

	pe.process.stdin.Close()

	select {
	case <-terminated:
		return
	case <-time.After(100*time.Millisecond + pe.closetime):
	}

	err := pe.process.cmd.Process.Signal(syscall.SIGINT)
	if err != nil {
		fmt.Printf("process: SIGINT unsuccessful to %v: %s", pe.process.cmd.Process.Pid, err)
	}

	select {
	case <-terminated:
		fmt.Printf("process: Process %v terminated after SIGINT", pe.process.cmd.Process.Pid)
		return
	case <-time.After(250*time.Millisecond + pe.closetime):
	}

	err = pe.process.cmd.Process.Signal(syscall.SIGTERM)
	if err != nil {
		fmt.Printf("process: SIGTERM unsuccessful to %v: %s", pe.process.cmd.Process.Pid, err)
	}

	select {
	case <-terminated:
		fmt.Printf("process: Process %v terminated after SIGTERM", pe.process.cmd.Process.Pid)
		return
	case <-time.After(500*time.Millisecond + pe.closetime):
	}

	err = pe.process.cmd.Process.Kill()
	if err != nil {
		fmt.Printf("process: SIGKILL unsuccessful to %v: %s", pe.process.cmd.Process.Pid, err)
		return
	}

	select {
	case <-terminated:
		fmt.Printf("process: Process %v terminated after SIGKILL", pe.process.cmd.Process.Pid)
		return
	case <-time.After(1000 * time.Millisecond):
	}

	fmt.Printf("process: SIGKILL did not terminate %v!", pe.process.cmd.Process.Pid)
}

func (pe *ProcessEndpoint) Output() chan *Output {
	return pe.output
}

func (pe *ProcessEndpoint) Error() chan *Error {
	return pe.err
}

func (pe *ProcessEndpoint) Send(msg []byte) bool {
	pe.process.stdin.Write(msg)
	return true
}

func (pe *ProcessEndpoint) StartReading() {
	go pe.log_stderr()
	go pe.process_txtout()
}

func (pe *ProcessEndpoint) process_txtout() {
	bufin := bufio.NewReader(pe.process.stdout)
	for {
		buf, err := bufin.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				fmt.Printf("process: Unexpected error while reading STDOUT from process: %s", err)
			} else {
				fmt.Printf("process: Process STDOUT closed")
			}
			pe.output <- &Output{
				data: trimEOL(buf),
			}
			pe.output <- &Output{
				data: []byte{},
			}
			return
		}
		pe.output <- &Output{
			data: trimEOL(buf),
		}
	}
}

func (pe *ProcessEndpoint) process_binout() {
	buf := make([]byte, 10*1024*1024)
	for {
		n, err := pe.process.stdout.Read(buf)
		if err != nil {
			if err != io.EOF {
				fmt.Printf("process: Unexpected error while reading STDOUT from process: %s", err)
			} else {
				fmt.Printf("process: Process STDOUT closed")
			}
			pe.output <- &Output{
				data: trimEOL(buf),
			}
			defer close(pe.output)
			return
		}
		pe.output <- &Output{
			data: append(make([]byte, 0, n), buf[:n]...),
		}
	}
}

func (pe *ProcessEndpoint) log_stderr() {
	bufstderr := bufio.NewReader(pe.process.stderr)
	for {
		buf, err := bufstderr.ReadSlice('\n')
		if err != nil {
			if err != io.EOF {
				fmt.Printf("process: Unexpected error while reading STDERR from process: %s", err)
			} else {
				fmt.Printf("process: Process STDERR closed")
			}
			break
		}
		pe.err <- &Error{
			data: buf,
		}
		pe.err <- &Error{
			data: []byte{},
		}
		// fmt.Printf("stderr: %s", string(trimEOL(buf)))
	}

}

func trimEOL(b []byte) []byte {
	lns := len(b)
	if lns > 0 && b[lns-1] == '\n' {
		lns--
		if lns > 0 && b[lns-1] == '\r' {
			lns--
		}
	}
	return b[:lns]
}
