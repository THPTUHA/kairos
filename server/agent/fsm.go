package agent

import (
	"io"

	"github.com/hashicorp/raft"
	"github.com/sirupsen/logrus"
)

// MessageType is the type to encode FSM commands.
type MessageType uint8

const (
	// SetJobType is the command used to store a job in the store.
	SetJobType MessageType = iota
	// DeleteJobType is the command used to delete a Job from the store.
	DeleteJobType
	// SetExecutionType is the command used to store an Execution to the store.
	SetExecutionType
	// DeleteExecutionsType is the command used to delete executions from the store.
	DeleteExecutionsType
	// ExecutionDoneType is the command to perform the logic needed once an execution
	// is done.
	ExecutionDoneType
)

// LogApplier is the definition of a function that can apply a Raft log
type LogApplier func(buf []byte, index uint64) interface{}

// LogAppliers is a mapping of the Raft MessageType to the appropriate log
// applier
type LogAppliers map[MessageType]LogApplier

type kairosFSM struct {
	store Storage

	// proAppliers holds the set of pro only LogAppliers
	proAppliers LogAppliers
	logger      *logrus.Entry
}

// NewFSM is used to construct a new FSM with a blank state
func newFSM(store Storage, logAppliers LogAppliers, logger *logrus.Entry) *kairosFSM {
	return &kairosFSM{
		store:       store,
		proAppliers: logAppliers,
		logger:      logger,
	}
}

// TODO
// Apply applies a Raft log entry to the key-value store.
func (d *kairosFSM) Apply(l *raft.Log) interface{} {
	return nil
}

// Snapshot returns a snapshot of the key-value store. We wrap
// the things we need in dkronSnapshot and then send that over to Persist.
// Persist encodes the needed data from dkronSnapshot and transport it to
// Restore where the necessary data is replicated into the finite state machine.
// This allows the consensus algorithm to truncate the replicated log.
func (d *kairosFSM) Snapshot() (raft.FSMSnapshot, error) {
	return &clusteragentSnapshot{store: d.store}, nil
}

// Restore stores the key-value store to a previous state.
func (d *kairosFSM) Restore(r io.ReadCloser) error {
	defer r.Close()
	return d.store.Restore(r)
}

type clusteragentSnapshot struct {
	store Storage
}

func (d *clusteragentSnapshot) Persist(sink raft.SnapshotSink) error {
	if err := d.store.Snapshot(sink); err != nil {
		_ = sink.Cancel()
		return err
	}

	// Close the sink.
	if err := sink.Close(); err != nil {
		return err
	}

	return nil
}

func (d *clusteragentSnapshot) Release() {}
