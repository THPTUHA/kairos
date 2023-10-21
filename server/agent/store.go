package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	kproto "github.com/THPTUHA/kairos/server/plugin/proto"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/buntdb"
	"google.golang.org/protobuf/proto"
)

const (
	jobsPrefix = "jobs"
)

type Store struct {
	db     *buntdb.DB
	lock   *sync.Mutex // for
	logger *logrus.Entry
}

// TODO
// NewStore creates a new Storage instance.
func NewStore(logger *logrus.Entry) (*Store, error) {
	db, err := buntdb.Open(":memory:")
	if err != nil {
		return nil, err
	}

	store := &Store{
		db:     db,
		lock:   &sync.Mutex{},
		logger: logger,
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

func (s *Store) jobHasMetadata(job *Job, metadata map[string]string) bool {
	if job == nil || job.Metadata == nil || len(job.Metadata) == 0 {
		return false
	}

	for k, v := range metadata {
		if val, ok := job.Metadata[k]; !ok || v != val {
			return false
		}
	}

	return true
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

	jobsFn := func(key, item string) bool {
		var pbj kproto.Job
		// [TODO] This condition is temporary while we migrate to JSON marshalling for jobs
		// so we can use BuntDb indexes. To be removed in future versions.
		if err := proto.Unmarshal([]byte(item), &pbj); err != nil {
			if err := json.Unmarshal([]byte(item), &pbj); err != nil {
				return false
			}
		}
		job := NewJobFromProto(&pbj, s.logger)
		job.logger = s.logger

		if options == nil ||
			(options.Metadata == nil || len(options.Metadata) == 0 || s.jobHasMetadata(job, options.Metadata)) &&
				(options.Query == "" || strings.Contains(job.Name, options.Query) || strings.Contains(job.DisplayName, options.Query)) &&
				(options.Disabled == "" || strconv.FormatBool(job.Disabled) == options.Disabled) &&
				((options.Status == "untriggered" && job.Status == "") || (options.Status == "" || job.Status == options.Status)) {

			jobs = append(jobs, job)
		}
		return true
	}

	err := s.db.View(func(tx *buntdb.Tx) error {
		var err error
		if options.Order == "DESC" {
			err = tx.Descend(options.Sort, jobsFn)
		} else {
			err = tx.Ascend(options.Sort, jobsFn)
		}
		return err
	})

	return jobs, err
}

// GetJob finds and return a Job from the store
func (s *Store) GetJob(name string, options *JobOptions) (*Job, error) {
	var pbj kproto.Job

	err := s.db.View(s.getJobTxFunc(name, &pbj))
	if err != nil {
		return nil, err
	}

	job := NewJobFromProto(&pbj, s.logger)
	job.logger = s.logger

	return job, nil
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

// This will allow reuse this code to avoid nesting transactions
func (s *Store) getJobTxFunc(name string, pbj *kproto.Job) func(tx *buntdb.Tx) error {
	return func(tx *buntdb.Tx) error {
		item, err := tx.Get(fmt.Sprintf("%s:%s", jobsPrefix, name))
		if err != nil {
			return err
		}

		// [TODO] This condition is temporary while we migrate to JSON marshalling for jobs
		// so we can use BuntDb indexes. To be removed in future versions.
		if err := proto.Unmarshal([]byte(item), pbj); err != nil {
			if err := json.Unmarshal([]byte(item), pbj); err != nil {
				return err
			}
		}

		s.logger.WithFields(logrus.Fields{
			"job": pbj.Name,
		}).Debug("store: Retrieved job from datastore")

		return nil
	}
}

// Removes the given job from its parent.
// Does nothing if nil is passed as child.
func (s *Store) removeFromParent(child *Job) error {
	// Do nothing if no job was given or job has no parent
	if child == nil || child.ParentJob == "" {
		return nil
	}

	parent, err := child.GetParent(s)
	if err != nil {
		return err
	}

	// Remove all occurrences from the parent, not just one.
	// Due to an old bug (in v1), a parent can have the same child more than once.
	djs := []string{}
	for _, djn := range parent.DependentJobs {
		if djn != child.Name {
			djs = append(djs, djn)
		}
	}
	parent.DependentJobs = djs
	if err := s.SetJob(parent, false); err != nil {
		return err
	}

	return nil
}

// SetJob stores a job in the storage
func (s *Store) SetJob(job *Job, copyDependentJobs bool) error {
	var pbej kproto.Job
	var ej *Job

	if err := job.Validate(); err != nil {
		return err
	}

	// Abort if parent not found before committing job to the store
	if job.ParentJob != "" {
		if j, _ := s.GetJob(job.ParentJob, nil); j == nil {
			return ErrParentJobNotFound
		}
	}

	err := s.db.Update(func(tx *buntdb.Tx) error {
		// Get if the requested job already exist
		err := s.getJobTxFunc(job.Name, &pbej)(tx)
		if err != nil && err != buntdb.ErrNotFound {
			return err
		}
		ej = NewJobFromProto(&pbej, s.logger)

		if ej.Name != "" {
			// When the job runs, these status vars are updated
			// otherwise use the ones that are stored
			if ej.LastError.After(job.LastError) {
				job.LastError = ej.LastError
			}
			if ej.LastSuccess.After(job.LastSuccess) {
				job.LastSuccess = ej.LastSuccess
			}
			if ej.SuccessCount > job.SuccessCount {
				job.SuccessCount = ej.SuccessCount
			}
			if ej.ErrorCount > job.ErrorCount {
				job.ErrorCount = ej.ErrorCount
			}
			if len(ej.DependentJobs) != 0 && copyDependentJobs {
				job.DependentJobs = ej.DependentJobs
			}
			if ej.Status != "" {
				job.Status = ej.Status
			}
		}

		if job.Schedule != ej.Schedule {
			job.Next, err = job.GetNext()
			if err != nil {
				return err
			}
		} else {
			// If coming from a backup us the previous value, don't allow overwriting this
			if job.Next.Before(ej.Next) {
				job.Next = ej.Next
			}
		}

		pbj := job.ToProto()
		if err := s.setJobTxFunc(pbj)(tx); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return err
	}

	// If the parent job changed update the parents of the old (if any) and new jobs
	if job.ParentJob != ej.ParentJob {
		if err := s.removeFromParent(ej); err != nil {
			return err
		}
		if err := s.addToParent(job); err != nil {
			return err
		}
	}

	return nil

}
