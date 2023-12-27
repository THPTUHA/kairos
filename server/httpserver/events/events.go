package events

import (
	"github.com/THPTUHA/kairos/pkg/workflow"
)

const (
	WfCmdCreate = iota
	WfCmdUpdate
	WfCmdDelete
	WfCmdStart
	WfCmdRecover
	WfCmdInfo
)

type WfEvent struct {
	Cmd      int
	Workflow *workflow.Workflow
	WfIDs    []int64
}

const (
	WorkerCreating = iota
	WorkerRunning
)

const (
	DestroyWorker = iota
)
