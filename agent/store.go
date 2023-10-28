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

	"github.com/THPTUHA/kairos/pkg/workflow"
	kproto "github.com/THPTUHA/kairos/server/plugin/proto"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/buntdb"
	"google.golang.org/protobuf/proto"
)

var (
	ErrNotFound = "not found"
)

const (
	// MaxExecutions to maintain in the storage
	MaxExecutions     = 100
	collectionsPrefix = "collections"
	workflowsPrefix   = "workflows"
	tasksPrefix       = "tasks"
	executionsPrefix  = "executions"
)

var (
	// ErrExecutionDoneForDeletedTask is returned when an execution done
	// is received for a non existent task.
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

	_ = db.CreateIndex("task_id", tasksPrefix+":*", buntdb.IndexJSON("id"))
	_ = db.CreateIndex("task_key", tasksPrefix+":*", buntdb.IndexJSON("key"))
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

// for local
func (s *Store) getWorkflowLastID() int64 {
	return 0
}

// for local
func (s *Store) getTaskLastID() int64 {
	return 0
}

func (s *Store) gsStoreIdx(idx *int) func(tx *buntdb.Tx) error {
	return func(tx *buntdb.Tx) error {
		s.lock.Lock()
		id, err := tx.Get(fmt.Sprintf("idx"))
		var num int

		if err != nil {
			if err.Error() == ErrNotFound {
				num = -1
				idx = &num
			} else {
				s.lock.Unlock()
				return err
			}
		}

		if id != "" {
			num, err = strconv.Atoi(id)
			if err != nil {
				s.lock.Unlock()
				return err
			}
		} else {
			num = -1
		}
		num++
		tx.Set("idx", strconv.Itoa(num), nil)
		idx = &num
		s.lock.Unlock()

		return nil
	}
}

type CollectionOptions struct {
	ID    int64
	Key   string
	Order string
	Sort  string
}

func (s *Store) GetCollections(options *CollectionOptions) ([]*workflow.Collection, error) {
	collections := make([]*workflow.Collection, 0)
	collectionsFn := func(key, item string) bool {
		var c workflow.Collection
		if err := json.Unmarshal([]byte(item), &c); err != nil {
			return false
		}

		if options == nil ||
			(options.ID != 0 || options.ID == c.ID) &&
				(options.Key != "" || strings.Contains(c.Namespace, options.Key)) {

			collections = append(collections, &c)
		}
		return true
	}

	err := s.db.View(func(tx *buntdb.Tx) error {
		var err error
		if options.Order == "DESC" {
			err = tx.Descend(options.Sort, collectionsFn)
		} else {
			err = tx.Ascend(options.Sort, collectionsFn)
		}
		return err
	})
	return collections, err
}

func (s *Store) setCollectionTxFunc(c *workflow.Collection) func(tx *buntdb.Tx) error {
	return func(tx *buntdb.Tx) error {
		var lastIdx int
		err := s.gsStoreIdx(&lastIdx)(tx)
		if err != nil {
			return err
		}
		cb, err := json.Marshal(c)
		if err != nil {
			return err
		}
		fmt.Println("Set collection ", c.Namespace)
		if _, _, err := tx.Set(fmt.Sprintf("%s:%d", collectionsPrefix, c.ID), string(cb), nil); err != nil {
			return err
		}

		return nil
	}
}

func (s *Store) SetCollection(c *workflow.Collection) error {
	var options CollectionOptions
	options.Sort = "id"
	cs, err := s.GetCollections(&options)
	if err != nil && err.Error() != ErrNotFound {
		return err
	}
	if len(cs) > 0 && cs[0].Status == workflow.Running {
		return errors.New("Collection is running")
	}

	err = s.db.Update(func(tx *buntdb.Tx) error {
		if err := s.setCollectionTxFunc(c)(tx); err != nil {
			return err
		}
		return nil
	})

	return err
}

type WorkflowOptions struct {
	Sort     string
	Order    string
	Query    string
	Status   string
	Disabled string
}

// func (s *Store) getWorkflowTxFunc(id int64, key string, wf *workflow.WfModel) func(tx *buntdb.Tx) error {
// 	return func(tx *buntdb.Tx) error {
// 		item, err := tx.Get(fmt.Sprintf("%s:%d:%s", workflowsPrefix, id, key))
// 		if err != nil {
// 			return err
// 		}

// 		if err := json.Unmarshal([]byte(item), wf); err != nil {
// 			return err
// 		}

// 		s.logger.WithFields(logrus.Fields{
// 			"key": wf.Key,
// 			"id":  wf.ID,
// 		}).Debug("store: Retrieved workflow from datastore")

// 		return nil
// 	}
// }

// func (s *Store) GetWorkflow(workflowID int64, workflowKey string) (*workflow.WfModel, error) {
// 	var wf workflow.WfModel

// 	err := s.db.View(s.getWorkflowTxFunc(workflowID, workflowKey, &wf))
// 	if err != nil {
// 		return nil, err
// 	}
// 	return &wf, nil
// }

// func (s *Store) setWorkflowTxFunc(wf *workflow.WfModel) func(tx *buntdb.Tx) error {
// 	return func(tx *buntdb.Tx) error {
// 		workflowID := fmt.Sprintf("%s:%d", workflowsPrefix, wf.ID)

// 		wb, err := json.Marshal(wf)
// 		if err != nil {
// 			return err
// 		}
// 		s.logger.WithField("workflow", wf.ID).Debug("store: Setting workflow")

// 		if _, _, err := tx.Set(workflowID, string(wb), nil); err != nil {
// 			return err
// 		}

// 		return nil
// 	}
// }

// func (s *Store) SetWorkflow(wf *workflow.Workflow, rawData string) error {
// 	wfe, err := s.GetWorkflow(wf.ID, wf.Key)
// 	if err != nil {
// 		return err
// 	}

// 	if wfe.RawData == rawData {
// 		return nil
// 	}

// 	wfe.ID = wf.ID
// 	wfe.RawData = rawData
// 	wfe.UpdatedAt = int(time.Now().UnixMilli())
// 	wfe.Status = workflow.ReSet
// 	err = s.db.Update(func(tx *buntdb.Tx) error {
// 		if err := s.setWorkflowTxFunc(wfe)(tx); err != nil {
// 			return err
// 		}
// 		return nil
// 	})

// 	return err
// }

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
			Sort: "key",
		}
	}

	tasks := make([]*Task, 0)

	tasksFn := func(key, item string) bool {
		var pbj kproto.Task
		if err := proto.Unmarshal([]byte(item), &pbj); err != nil {
			if err := json.Unmarshal([]byte(item), &pbj); err != nil {
				return false
			}
		}
		task := NewTaskFromProto(&pbj, s.logger)
		task.logger = s.logger

		if options == nil ||
			(options.WorkflowID != 0 || options.WorkflowID == task.WorkflowID) &&
				(options.Metadata == nil || len(options.Metadata) == 0 || s.taskHasMetadata(task, options.Metadata)) &&
				(options.Query == "" || strings.Contains(task.Key, options.Query) || strings.Contains(task.DisplayName, options.Query)) &&
				(options.Disabled == "" || strconv.FormatBool(task.Disabled) == options.Disabled) &&
				((options.Status == "untriggered" && task.Status == "") || (options.Status == "" || task.Status == options.Status)) {

			tasks = append(tasks, task)
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
func (s *Store) GetTask(id int64, options *TaskOptions) (*Task, error) {
	var pbj kproto.Task

	err := s.db.View(s.getTaskTxFunc(id, &pbj))
	if err != nil {
		return nil, err
	}

	task := NewTaskFromProto(&pbj, s.logger)
	task.logger = s.logger

	return task, nil
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

func (s *Store) getTaskTxFunc(id int64, pbj *kproto.Task) func(tx *buntdb.Tx) error {
	return func(tx *buntdb.Tx) error {
		item, err := tx.Get(fmt.Sprintf("%s:%d", tasksPrefix, id))
		if err != nil {
			return err
		}

		if err := proto.Unmarshal([]byte(item), pbj); err != nil {
			if err := json.Unmarshal([]byte(item), pbj); err != nil {
				return err
			}
		}

		s.logger.WithFields(logrus.Fields{
			"task": pbj.Key,
		}).Debug("store: Retrieved task from datastore")

		return nil
	}
}

func (s *Store) setTaskTxFunc(pbj *kproto.Task) func(tx *buntdb.Tx) error {
	return func(tx *buntdb.Tx) error {
		taskID := fmt.Sprintf("%s:%d", tasksPrefix, pbj.Id)

		jb, err := json.Marshal(pbj)
		if err != nil {
			return err
		}
		s.logger.WithField("task", pbj.Id).Debug("store: Setting task")

		if _, _, err := tx.Set(taskID, string(jb), nil); err != nil {
			return err
		}

		return nil
	}
}

// SetTask stores a task in the storage
func (s *Store) SetTask(task *Task) error {
	var pbej kproto.Task
	var ej *Task

	if err := task.Validate(); err != nil {
		return err
	}

	err := s.db.Update(func(tx *buntdb.Tx) error {
		// Get if the requested task already exist
		err := s.getTaskTxFunc(task.ID, &pbej)(tx)
		if err != nil && err != buntdb.ErrNotFound {
			return err
		}
		ej = NewTaskFromProto(&pbej, s.logger)

		if ej.Key != "" {
			// When the task runs, these status vars are updated
			// otherwise use the ones that are stored
			if ej.LastError.After(task.LastError) {
				task.LastError = ej.LastError
			}
			if ej.LastSuccess.After(task.LastSuccess) {
				task.LastSuccess = ej.LastSuccess
			}
			if ej.SuccessCount > task.SuccessCount {
				task.SuccessCount = ej.SuccessCount
			}
			if ej.ErrorCount > task.ErrorCount {
				task.ErrorCount = ej.ErrorCount
			}
			if ej.Status != "" {
				task.Status = ej.Status
			}
		}

		if task.Schedule != ej.Schedule {
			task.Next, err = task.GetNext()
			if err != nil {
				return err
			}
		} else {
			// If coming from a backup us the previous value, don't allow overwriting this
			if task.Next.Before(ej.Next) {
				task.Next = ej.Next
			}
		}

		pbj := task.ToProto()
		if err := s.setTaskTxFunc(pbj)(tx); err != nil {
			return err
		}
		return nil
	})

	return err
}

func (s *Store) deleteExecutionsTxFunc(taskID int64) func(tx *buntdb.Tx) error {
	return func(tx *buntdb.Tx) error {
		var delkeys []string
		prefix := fmt.Sprintf("%s:%d", executionsPrefix, taskID)
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
func (s *Store) DeleteTask(id int64) (*Task, error) {
	var task *Task
	err := s.db.Update(func(tx *buntdb.Tx) error {
		// Get the task
		var pbj kproto.Task
		if err := s.getTaskTxFunc(id, &pbj)(tx); err != nil {
			return err
		}

		task = NewTaskFromProto(&pbj, s.logger)

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
		var pbe kproto.Execution

		// [TODO] This condition is temporary while we migrate to JSON marshalling for tasks
		// so we can use BuntDb indexes. To be removed in future versions.
		if err := proto.Unmarshal([]byte(item.Value), &pbe); err != nil {
			if err := json.Unmarshal(item.Value, &pbe); err != nil {
				s.logger.WithError(err).WithField("key", item.Key).Debug("error unmarshaling JSON")
				return nil, err
			}
		}
		execution := NewExecutionFromProto(&pbe)
		if timezone != nil {
			execution.FinishedAt = execution.FinishedAt.In(timezone)
			execution.StartedAt = execution.StartedAt.In(timezone)
		}
		executions = append(executions, execution)
	}
	return executions, nil
}

func (*Store) listTxFunc(prefix string, kvs *[]kv, found *bool, opts *ExecutionOptions) func(tx *buntdb.Tx) error {
	fnc := func(key, value string) bool {
		if strings.HasPrefix(key, prefix) {
			*found = true
			// ignore self in listing
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
	// compute task status based on execution group
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

// SetExecutionDone saves the execution and updates the task with the corresponding
// results
func (s *Store) SetExecutionDone(execution *Execution) (bool, error) {
	err := s.db.Update(func(tx *buntdb.Tx) error {
		// Load the task from the store
		var pbj kproto.Task
		if err := s.getTaskTxFunc(execution.TaskID, &pbj)(tx); err != nil {
			if err == buntdb.ErrNotFound {
				s.logger.Warn(ErrExecutionDoneForDeletedTask)
				return ErrExecutionDoneForDeletedTask
			}
			s.logger.WithError(err).Fatal(err)
			return err
		}

		key := fmt.Sprintf("%s:%d:%s", executionsPrefix, execution.TaskID, execution.Key())

		// Save the execution to store
		pbe := execution.ToProto()
		if err := s.setExecutionTxFunc(key, pbe)(tx); err != nil {
			return err
		}

		if pbe.Success {
			pbj.LastSuccess.HasValue = true
			pbj.LastSuccess.Time = pbe.FinishedAt
			pbj.SuccessCount++
		} else {
			pbj.LastError.HasValue = true
			pbj.LastError.Time = pbe.FinishedAt
			pbj.ErrorCount++
		}

		status, err := s.computeStatus(pbj.Key, pbe.Group, tx)
		if err != nil {
			return err
		}
		pbj.Status = status

		if err := s.setTaskTxFunc(&pbj)(tx); err != nil {
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

// GetExecutions returns the executions given a Task key.
func (s *Store) GetExecutions(taskID int64, opts *ExecutionOptions) ([]*Execution, error) {
	prefix := fmt.Sprintf("%s:%d:", executionsPrefix, taskID)

	kvs, err := s.list(prefix, true, opts)
	if err != nil {
		return nil, err
	}

	return s.unmarshalExecutions(kvs, opts.Timezone)
}

// GetExecutionGroup returns all executions in the same group of a given execution
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
	pbe := execution.ToProto()
	key := fmt.Sprintf("%s:%d:%s", executionsPrefix, execution.TaskID, execution.Key())

	s.logger.WithFields(logrus.Fields{
		"task":      execution.TaskID,
		"execution": key,
		"finished":  execution.FinishedAt.String(),
	}).Debug("store: Setting key")

	err := s.db.Update(s.setExecutionTxFunc(key, pbe))

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

	// Delete all execution results over the limit, starting from olders
	if len(execs) > MaxExecutions {
		//sort the array of all execution groups by StartedAt time
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

func (*Store) setExecutionTxFunc(key string, pbe *kproto.Execution) func(tx *buntdb.Tx) error {
	return func(tx *buntdb.Tx) error {
		// Get previous execution
		i, err := tx.Get(key)
		if err != nil && err != buntdb.ErrNotFound {
			return err
		}
		// Do nothing if a previous execution exists and is
		// more recent, avoiding non ordered execution set
		if i != "" {
			var p kproto.Execution
			// [TODO] This condition is temporary while we migrate to JSON marshalling for executions
			// so we can use BuntDb indexes. To be removed in future versions.
			if err := proto.Unmarshal([]byte(i), &p); err != nil {
				if err := json.Unmarshal([]byte(i), &p); err != nil {
					return err
				}
			}
			// Compare existing execution
			if p.GetFinishedAt().Seconds > pbe.GetFinishedAt().Seconds {
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
