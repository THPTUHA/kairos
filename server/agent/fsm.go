package agent

import (
	"io"

	"github.com/hashicorp/raft"
)

// MessageType is the type to encode FSM commands.
type MessageType uint8

// LogApplier is the definition of a function that can apply a Raft log
type LogApplier func(buf []byte, index uint64) interface{}

// LogAppliers is a mapping of the Raft MessageType to the appropriate log
// applier
type LogAppliers map[MessageType]LogApplier

type clusteragentFSM struct {
	store Storage

	// proAppliers holds the set of pro only LogAppliers
	proAppliers LogAppliers
}

// NewFSM is used to construct a new FSM with a blank state
func newFSM(store Storage, logAppliers LogAppliers) *clusteragentFSM {
	return &clusteragentFSM{
		store:       store,
		proAppliers: logAppliers,
	}
}

// TODO
// Apply applies a Raft log entry to the key-value store.
func (d *clusteragentFSM) Apply(l *raft.Log) interface{} {
	return nil
}

// Snapshot returns a snapshot of the key-value store. We wrap
// the things we need in dkronSnapshot and then send that over to Persist.
// Persist encodes the needed data from dkronSnapshot and transport it to
// Restore where the necessary data is replicated into the finite state machine.
// This allows the consensus algorithm to truncate the replicated log.
func (d *clusteragentFSM) Snapshot() (raft.FSMSnapshot, error) {
	return &clusteragentSnapshot{store: d.store}, nil
}

// Restore stores the key-value store to a previous state.
func (d *clusteragentFSM) Restore(r io.ReadCloser) error {
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
