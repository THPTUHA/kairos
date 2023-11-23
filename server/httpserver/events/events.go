package events

import (
	"github.com/THPTUHA/kairos/pkg/workflow"
)

const (
	WfCmdCreate = iota
	WfCmdUpdate
	WfCmdDelete
	WfCmdStart
)

type WfEvent struct {
	Cmd      int
	Workflow *workflow.Workflow
}

const (
	WorkerCreating = iota
	WorkerRunning
)

const (
	DeleteWorker = iota
)

var wfChan chan *WfEvent

func Init() {
	wfChan = make(chan *WfEvent)
}

func Get() chan *WfEvent {
	if wfChan == nil {
		Init()
	}
	return wfChan
}
