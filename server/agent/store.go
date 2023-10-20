package agent

import (
	"io"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/tidwall/buntdb"
)

type Store struct {
	db   *buntdb.DB
	lock *sync.Mutex // for

}

// TODO
// NewStore creates a new Storage instance.
func NewStore(logger *logrus.Entry) (*Store, error) {
	db, err := buntdb.Open(":memory:")
	if err != nil {
		return nil, err
	}

	store := &Store{
		db:   db,
		lock: &sync.Mutex{},
	}

	return store, nil
}

// JobOptions additional options to apply when loading a Job.
type JobOptions struct {
	Metadata map[string]string `json:"tags"`
	Sort     string
	Order    string
	Query    string
	Status   string
	Disabled string
}

// TODO
// GetJobs returns all jobs
func (s *Store) GetJobs(options *JobOptions) ([]*Job, error) {
	if options == nil {
		options = &JobOptions{
			Sort: "name",
		}
	}

	jobs := make([]*Job, 0)
	return jobs, nil
}

// Snapshot creates a backup of the data stored in BuntDB
func (s *Store) Snapshot(w io.WriteCloser) error {
	return s.db.Save(w)
}

// Restore load data created with backup in to Bunt
func (s *Store) Restore(r io.ReadCloser) error {
	return s.db.Load(r)
}

// Shutdown close the KV store
func (s *Store) Shutdown() error {
	return s.db.Close()
}
