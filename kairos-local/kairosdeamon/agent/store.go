package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tidwall/buntdb"
)

var (
	ErrNotFound = "not found"
)

const (
	MaxExecutions    = 100
	workflowsPrefix  = "workflows"
	tasksPrefix      = "tasks"
	executionsPrefix = "executions"
)

var (
	ErrExecutionDoneForDeletedTask = errors.New("grpc: Received execution done for a deleted task")
)

type kv struct {
	Key   string
	Value []byte
}

type Store struct {
	db   *buntdb.DB
	lock *sync.Mutex
	// for kairos local
	local  bool
	logger *logrus.Entry
}

type ExecutionOptions struct {
	Sort     string
	Order    string
	Timezone *time.Location
}

// NewStore creates a new Storage instance.
func NewStore(logger *logrus.Entry, local bool) (*Store, error) {
	db, err := buntdb.Open(":memory:")
	if err != nil {
		return nil, err
	}
	_ = db.CreateIndex("workflow_id", tasksPrefix+":*", buntdb.IndexJSON("workflow_id"))
	_ = db.CreateIndex("name", tasksPrefix+":*", buntdb.IndexJSON("name"))
	_ = db.CreateIndex("started_at", executionsPrefix+":*", buntdb.IndexJSON("started_at"))
	_ = db.CreateIndex("finished_at", executionsPrefix+":*", buntdb.IndexJSON("finished_at"))
	_ = db.CreateIndex("attempt", executionsPrefix+":*", buntdb.IndexJSON("attempt"))
	_ = db.CreateIndex("displayname", tasksPrefix+":*", buntdb.IndexJSON("displayname"))
	_ = db.CreateIndex("schedule", tasksPrefix+":*", buntdb.IndexJSON("schedule"))
	_ = db.CreateIndex("success_count", tasksPrefix+":*", buntdb.IndexJSON("success_count"))
	_ = db.CreateIndex("error_count", tasksPrefix+":*", buntdb.IndexJSON("error_count"))
	_ = db.CreateIndex("last_success", tasksPrefix+":*", buntdb.IndexJSON("last_success"))
	_ = db.CreateIndex("last_error", tasksPrefix+":*", buntdb.IndexJSON("last_error"))
	_ = db.CreateIndex("next", tasksPrefix+":*", buntdb.IndexJSON("next"))

	store := &Store{
		db:     db,
		lock:   &sync.Mutex{},
		local:  local,
		logger: logger,
	}

	return store, nil
}

func (s *Store) gsStoreIdx() func(tx *buntdb.Tx) (int, error) {
	return func(tx *buntdb.Tx) (int, error) {
		s.lock.Lock()
		id, err := tx.Get("idx")
		var num int

		if err != nil {
			if err.Error() == ErrNotFound {
				num = -1
			} else {
				s.lock.Unlock()
				return -1, err
			}
		} else {
			num, err = strconv.Atoi(id)
			if err != nil {
				s.lock.Unlock()
				return -1, err
			}
		}
		num--
		tx.Set("idx", strconv.Itoa(num), nil)
		s.logger.Debug(fmt.Sprintf("Set Idx db = %d", num))
		s.lock.Unlock()
		return num, nil
	}
}

// TaskOptions additional options to apply when loading a Task.
type TaskOptions struct {
	Metadata   map[string]string `json:"tags"`
	Sort       string
	WorkflowID int64
	Order      string
	Query      string
	Status     string
	Disabled   string
}

func (s *Store) taskHasMetadata(task *Task, metadata map[string]string) bool {
	if task == nil || task.Metadata == nil || len(task.Metadata) == 0 {
		return false
	}

	for k, v := range metadata {
		if val, ok := task.Metadata[k]; !ok || v != val {
			return false
		}
	}

	return true
}

// GetTasks returns all tasks
func (s *Store) GetTasks(options *TaskOptions) ([]*Task, error) {
	if options == nil {
		options = &TaskOptions{
			Sort: "id",
		}
	}

	tasks := make([]*Task, 0)

	tasksFn := func(key, item string) bool {
		var task Task
		if err := json.Unmarshal([]byte(item), &task); err != nil {
			return false
		}
		task.logger = s.logger

		if options == nil ||
			(options.WorkflowID != 0 || options.WorkflowID == task.WorkflowID) &&
				(options.Metadata == nil || len(options.Metadata) == 0 || s.taskHasMetadata(&task, options.Metadata)) &&
				(options.Query == "" || strings.Contains(task.Name, options.Query)) &&
				(options.Disabled == "" || strconv.FormatBool(task.Disabled) == options.Disabled) &&
				((options.Status == "untriggered" && task.Status == "") || (options.Status == "" || task.Status == options.Status)) {

			tasks = append(tasks, &task)
		}
		return true
	}

	err := s.db.View(func(tx *buntdb.Tx) error {
		var err error
		if options.Order == "DESC" {
			err = tx.Descend(options.Sort, tasksFn)
		} else {
			err = tx.Ascend(options.Sort, tasksFn)
		}
		return err
	})

	return tasks, err
}

// GetTask finds and return a Task from the store
func (s *Store) GetTask(id string, options *TaskOptions) (*Task, error) {
	var task Task

	err := s.db.View(s.getTaskTxFunc(id, &task))
	if err != nil {
		return nil, err
	}

	task.logger = s.logger

	return &task, nil
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

func (s *Store) getTaskTxFunc(id string, task *Task) func(tx *buntdb.Tx) error {
	return func(tx *buntdb.Tx) error {
		item, err := tx.Get(fmt.Sprintf("%s:%s", tasksPrefix, id))
		if err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(item), task); err != nil {
			return err
		}

		s.logger.WithFields(logrus.Fields{
			"task": task.Name,
		}).Debug("store: Retrieved task from datastore")

		return nil
	}
}

func (s *Store) setTaskTxFunc(task *Task) func(tx *buntdb.Tx) error {
	return func(tx *buntdb.Tx) error {
		taskID := fmt.Sprintf("%s:%s", tasksPrefix, task.ID)

		tj, err := json.Marshal(task)
		if err != nil {
			return err
		}
		s.logger.WithField("task", task.ID).Debug("store: Setting task")

		if _, _, err := tx.Set(taskID, string(tj), nil); err != nil {
			return err
		}

		return nil
	}
}

// SetTask stores a task in the storage
func (s *Store) SetTask(task *Task) error {
	var et Task

	if err := task.Validate(); err != nil {
		return err
	}

	err := s.db.Update(func(tx *buntdb.Tx) error {
		// Get if the requested task already exist
		err := s.getTaskTxFunc(task.ID, &et)(tx)
		if err != nil && err != buntdb.ErrNotFound {
			return err
		}

		if et.ID != "" {
			// When the task runs, these status vars are updated
			// otherwise use the ones that are stored
			if et.LastError.After(task.LastError) {
				task.LastError = et.LastError
			}
			if et.LastSuccess.After(task.LastSuccess) {
				task.LastSuccess = et.LastSuccess
			}
			if et.SuccessCount > task.SuccessCount {
				task.SuccessCount = et.SuccessCount
			}
			if et.ErrorCount > task.ErrorCount {
				task.ErrorCount = et.ErrorCount
			}
			if et.Status != "" {
				task.Status = et.Status
			}
		}

		if task.Schedule != et.Schedule {
			task.Next, err = task.GetNext()
			if err != nil {
				return err
			}
		} else {
			// If coming from a backup us the previous value, don't allow overwriting this
			if task.Next.Before(et.Next) {
				task.Next = et.Next
			}
		}

		if err := s.setTaskTxFunc(task)(tx); err != nil {
			return err
		}
		return nil
	})

	return err
}

func (s *Store) deleteExecutionsTxFunc(taskID string) func(tx *buntdb.Tx) error {
	return func(tx *buntdb.Tx) error {
		var delkeys []string
		prefix := fmt.Sprintf("%s:%s", executionsPrefix, taskID)
		if err := tx.Ascend("", func(key, value string) bool {
			if strings.HasPrefix(key, prefix) {
				delkeys = append(delkeys, key)
			}
			return true
		}); err != nil {
			return err
		}

		for _, k := range delkeys {
			_, _ = tx.Delete(k)
		}

		return nil
	}
}

// DeleteTask deletes the given task from the store, along with
// all its executions and references to it.
func (s *Store) DeleteTask(id string) (*Task, error) {
	var task *Task
	err := s.db.Update(func(tx *buntdb.Tx) error {
		// Get the task
		if err := s.getTaskTxFunc(id, task)(tx); err != nil {
			return err
		}

		if err := s.deleteExecutionsTxFunc(id)(tx); err != nil {
			return err
		}

		_, err := tx.Delete(fmt.Sprintf("%s:%d", tasksPrefix, id))
		return err
	})
	if err != nil {
		return nil, err
	}

	return task, nil
}

func (s *Store) unmarshalExecutions(items []kv, timezone *time.Location) ([]*Execution, error) {
	var executions []*Execution
	for _, item := range items {
		var execution Execution

		if err := json.Unmarshal(item.Value, &execution); err != nil {
			s.logger.WithError(err).WithField("id", item.Key).Debug("error unmarshaling JSON")
			return nil, err
		}
		if timezone != nil {
			execution.FinishedAt = execution.FinishedAt.In(timezone)
			execution.StartedAt = execution.StartedAt.In(timezone)
		}
		executions = append(executions, &execution)
	}
	return executions, nil
}

func (*Store) listTxFunc(prefix string, kvs *[]kv, found *bool, opts *ExecutionOptions) func(tx *buntdb.Tx) error {
	fnc := func(key, value string) bool {
		if strings.HasPrefix(key, prefix) {
			*found = true
			if !bytes.Equal(trimDirectoryKey([]byte(key)), []byte(prefix)) {
				kv := kv{Key: key, Value: []byte(value)}
				*kvs = append(*kvs, kv)
			}
		}
		return true
	}

	return func(tx *buntdb.Tx) (err error) {
		if opts.Order == "DESC" {
			err = tx.Descend(opts.Sort, fnc)
		} else {
			err = tx.Ascend(opts.Sort, fnc)
		}
		return err
	}
}

func (s *Store) computeStatus(taskID string, exGroup int64, tx *buntdb.Tx) (string, error) {
	kvs := []kv{}
	found := false
	prefix := fmt.Sprintf("%s:%s:", executionsPrefix, taskID)

	if err := s.listTxFunc(prefix, &kvs, &found, &ExecutionOptions{})(tx); err != nil {
		return "", err
	}

	execs, err := s.unmarshalExecutions(kvs, nil)
	if err != nil {
		return "", err
	}

	var executions []*Execution
	for _, ex := range execs {
		if ex.Group == exGroup {
			executions = append(executions, ex)
		}
	}

	success := 0
	failed := 0

	var status string
	for _, ex := range executions {
		if ex.Success {
			success = success + 1
		} else {
			failed = failed + 1
		}
	}

	if failed == 0 {
		status = StatusSuccess
	} else if failed > 0 && success == 0 {
		status = StatusFailed
	} else if failed > 0 && success > 0 {
		status = StatusPartiallyFailed
	}

	return status, nil
}

func (s *Store) SetExecutionDone(execution *Execution) (bool, error) {
	err := s.db.Update(func(tx *buntdb.Tx) error {
		var task Task
		if err := s.getTaskTxFunc(execution.TaskID, &task)(tx); err != nil {
			if err == buntdb.ErrNotFound {
				s.logger.Warn(ErrExecutionDoneForDeletedTask)
				return ErrExecutionDoneForDeletedTask
			}
			s.logger.WithError(err).Fatal(err)
			return err
		}

		key := fmt.Sprintf("%s:%d:%s", executionsPrefix, execution.TaskID, execution.Key())

		if err := s.setExecutionTxFunc(key, execution)(tx); err != nil {
			return err
		}

		if execution.Success {
			task.LastSuccess.Set(execution.FinishedAt)
			task.SuccessCount++
		} else {
			task.LastError.Set(execution.FinishedAt)
			task.ErrorCount++
		}

		status, err := s.computeStatus(task.ID, execution.Group, tx)
		if err != nil {
			return err
		}
		task.Status = status

		if err := s.setTaskTxFunc(&task)(tx); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		s.logger.WithError(err).Error("store: Error in SetExecutionDone")
		return false, err
	}

	return true, nil
}

func (s *Store) list(prefix string, checkRoot bool, opts *ExecutionOptions) ([]kv, error) {
	var found bool
	kvs := []kv{}

	err := s.db.View(s.listTxFunc(prefix, &kvs, &found, opts))
	if err == nil && !found && checkRoot {
		return nil, buntdb.ErrNotFound
	}

	return kvs, err
}

func (s *Store) GetExecutions(taskID string, opts *ExecutionOptions) ([]*Execution, error) {
	prefix := fmt.Sprintf("%s:%s:", executionsPrefix, taskID)

	kvs, err := s.list(prefix, true, opts)
	if err != nil {
		return nil, err
	}

	return s.unmarshalExecutions(kvs, opts.Timezone)
}

func (s *Store) GetExecutionGroup(execution *Execution, opts *ExecutionOptions) ([]*Execution, error) {
	res, err := s.GetExecutions(execution.TaskID, opts)
	if err != nil {
		return nil, err
	}

	var executions []*Execution
	for _, ex := range res {
		if ex.Group == execution.Group {
			executions = append(executions, ex)
		}
	}
	return executions, nil
}

func (s *Store) SetExecution(execution *Execution) (string, error) {
	key := fmt.Sprintf("%s:%d:%s", executionsPrefix, execution.TaskID, execution.Key())

	s.logger.WithFields(logrus.Fields{
		"task":      execution.TaskID,
		"execution": key,
		"finished":  execution.FinishedAt.String(),
	}).Debug("store: Setting key")

	err := s.db.Update(s.setExecutionTxFunc(key, execution))

	if err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"task":      execution.TaskID,
			"execution": key,
		}).Debug("store: Failed to set key")
		return "", err
	}

	execs, err := s.GetExecutions(execution.TaskID, &ExecutionOptions{})
	if err != nil && err != buntdb.ErrNotFound {
		s.logger.WithError(err).
			WithField("task", execution.TaskID).
			Error("store: Error getting executions for task")
	}

	if len(execs) > MaxExecutions {
		sort.Slice(execs, func(i, j int) bool {
			return execs[i].StartedAt.Before(execs[j].StartedAt)
		})

		for i := 0; i < len(execs)-MaxExecutions; i++ {
			s.logger.WithFields(logrus.Fields{
				"task":      execs[i].TaskID,
				"execution": execs[i].Key(),
			}).Debug("store: to delete key")
			err = s.db.Update(func(tx *buntdb.Tx) error {
				k := fmt.Sprintf("%s:%d:%s", executionsPrefix, execs[i].TaskID, execs[i].Key())
				_, err := tx.Delete(k)
				return err
			})
			if err != nil {
				s.logger.WithError(err).
					WithField("execution", execs[i].Key()).
					Error("store: Error trying to delete overflowed execution")
			}
		}
	}

	return key, nil
}

func (*Store) setExecutionTxFunc(key string, pbe *Execution) func(tx *buntdb.Tx) error {
	return func(tx *buntdb.Tx) error {
		i, err := tx.Get(key)
		if err != nil && err != buntdb.ErrNotFound {
			return err
		}
		if i != "" {
			var p Execution
			if err := json.Unmarshal([]byte(i), &p); err != nil {
				return err
			}
			if p.FinishedAt.UnixMilli() > pbe.FinishedAt.UnixMilli() {
				return nil
			}
		}

		eb, err := json.Marshal(pbe)
		if err != nil {
			return err
		}

		_, _, err = tx.Set(key, string(eb), nil)
		return err
	}
}

func trimDirectoryKey(key []byte) []byte {
	if isDirectoryKey(key) {
		return key[:len(key)-1]
	}

	return key
}

func isDirectoryKey(key []byte) bool {
	return len(key) > 0 && key[len(key)-1] == ':'
}
