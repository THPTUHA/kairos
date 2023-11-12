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

var WfChan chan *WfEvent

func Init() {
	WfChan = make(chan *WfEvent)
}
