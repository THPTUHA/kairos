package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/boltdb/bolt"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/buntdb"
)

var (
	ErrNotFound = errors.New("not found")
)

const (
	MaxExecutions    = 100
	workflowsPrefix  = "workflows"
	tasksPrefix      = "tasks"
	executionsPrefix = "executions"
	brokerPrefix     = "brokers"
)

var (
	workflowBucket   = []byte("workflows")
	tasksBucket      = []byte("tasks")
	executionsBucket = []byte("executions")
	metaBucket       = []byte("meta")
	brokersBucket    = []byte("brokers")
	queueBucket      = []byte("queue")
)

var (
	ErrExecutionDoneForDeletedTask = errors.New("grpc: Received execution done for a deleted task")
)

type kv struct {
	Key   string
	Value []byte
}

type Store struct {
	db   *bolt.DB
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

func NewStore(database string, logger *logrus.Entry, local bool) (*Store, error) {
	db, err := bolt.Open(database, 0600, nil)
	if err != nil {
		return nil, err
	}

	store := &Store{
		db:     db,
		lock:   &sync.Mutex{},
		local:  local,
		logger: logger,
	}

	return store, nil
}

type TaskOptions struct {
	Metadata    map[string]string `json:"tags"`
	Sort        string
	WorkflowID  int64
	Order       string
	Query       string
	Status      string
	Disabled    string
	NoScheduler bool
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

func (s *Store) GetMeta(key string) (string, error) {
	var v string
	tx, err := s.db.Begin(true)
	if err != nil {
		return v, err
	}
	defer tx.Rollback()
	mBkt, err := tx.CreateBucketIfNotExists(metaBucket)
	if err != nil {
		return v, err
	}
	e := string(mBkt.Get([]byte(key)))
	return e, tx.Commit()
}

func (s *Store) getMetaTxFunc(key string, value *string) func(tx *bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		mBkt, err := tx.CreateBucketIfNotExists(metaBucket)
		if err != nil {
			return err
		}
		e := string(mBkt.Get([]byte(key)))
		value = &e
		return nil
	}
}

func (s *Store) SetMeta(key, value string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return s.setMetaTxFunc(key, value)(tx)
	})
}

func (s *Store) setMetaTxFunc(key, value string) func(tx *bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		mBkt, err := tx.CreateBucketIfNotExists(metaBucket)
		if err != nil {
			return err
		}
		mBkt.Put([]byte(key), []byte(value))
		return nil
	}
}

func (s *Store) GetTasks(options *TaskOptions) ([]*Task, error) {
	s.logger.Debug(" GetTasks-----")
	tasks := make([]*Task, 0)
	tx, err := s.db.Begin(true)
	if err != nil {
		return tasks, err
	}
	defer tx.Rollback()

	taskBkt, err := tx.CreateBucketIfNotExists(tasksBucket)
	if err != nil {
		s.logger.Debug("err 1")
		return tasks, err
	}

	if options == nil {
		options = &TaskOptions{
			Sort: "id",
		}
	}

	tasksFn := func(task *Task) bool {
		task.logger = s.logger
		if options == nil ||
			(options.WorkflowID != 0 || options.WorkflowID == task.WorkflowID) &&
				(options.Metadata == nil || len(options.Metadata) == 0 || s.taskHasMetadata(task, options.Metadata)) &&
				(options.Query == "" || strings.Contains(task.Name, options.Query)) &&
				(options.Disabled == "" || strconv.FormatBool(task.Disabled) == options.Disabled) &&
				(task.Status == "" || (options.Status == "" || task.Status == options.Status)) &&
				(task.Schedule != "" || options.NoScheduler) {
			return true
		}
		return false
	}

	if options.WorkflowID == 0 {
		c := taskBkt.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			s.logger.Debug("taskID = ", string(k))
			var task Task
			err := json.Unmarshal(v, &task)
			if err != nil {
				return tasks, err
			}
			if tasksFn(&task) {
				s.logger.Debug("taskID Pass = ", string(k))
				task.logger = s.logger
				tasks = append(tasks, &task)
			}
		}
	} else {
		workflowBkt, err := tx.CreateBucketIfNotExists(workflowBucket)
		if err != nil {
			return tasks, err
		}

		r := workflowBkt.Get([]byte(fmt.Sprint(options.WorkflowID)))
		ids := strings.Split(string(r), ",")
		for _, id := range ids {
			v := taskBkt.Get([]byte(id))
			if v != nil {
				var task Task
				err := json.Unmarshal(v, &task)
				if err != nil {
					return tasks, err
				}
				if tasksFn(&task) {
					task.logger = s.logger
					tasks = append(tasks, &task)
				}
			}
		}

	}
	return tasks, tx.Commit()
}

func (s *Store) GetTask(id string, options *TaskOptions) (*Task, error) {
	var task Task

	s.db.Update(func(tx *bolt.Tx) error {
		err := s.getTaskTxFunc(id, &task)(tx)
		return err
	})

	return &task, nil
}

func (s *Store) Shutdown() error {
	return s.db.Close()
}

func (s *Store) SetTask(task *Task) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return s.setTaskTxFunc(task)(tx)
	})
}

func (s *Store) setBrokerTxFunc(broker *workflow.Broker) func(tx *bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		brokerBkt, err := tx.CreateBucketIfNotExists(brokersBucket)
		if err != nil {
			return err
		}
		bt, err := json.Marshal(brokerBkt)
		if err != nil {
			return err
		}
		brokerBkt.Put([]byte(fmt.Sprint(broker.ID)), bt)
		return nil
	}
}

func (s *Store) SetBroker(broker *workflow.Broker) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return s.setBrokerTxFunc(broker)(tx)
	})
}

func (s *Store) getBrokerTxFunc(brokerID string, broker *workflow.Broker) func(tx *bolt.Tx) error {
	return func(tx *bolt.Tx) error {

		brokerBkt, err := tx.CreateBucketIfNotExists(tasksBucket)
		if err != nil {
			return err
		}
		v := brokerBkt.Get([]byte(brokerID))
		if v == nil {
			return ErrNotFound
		}
		err = json.Unmarshal(v, broker)
		if err != nil {
			return err
		}
		return nil
	}
}

func (s *Store) GetBroker(id string) (*workflow.Broker, error) {
	var broker workflow.Broker
	s.db.Update(func(tx *bolt.Tx) error {
		err := s.getBrokerTxFunc(id, &broker)(tx)
		return err
	})
	return &broker, nil
}

func (s *Store) GetBrokers() ([]*workflow.Broker, error) {
	brokers := make([]*workflow.Broker, 0)
	bx, err := s.db.Begin(true)
	if err != nil {
		return brokers, err
	}
	defer bx.Rollback()

	brokerBkt, err := bx.CreateBucketIfNotExists(tasksBucket)
	if err != nil {
		return brokers, err
	}

	c := brokerBkt.Cursor()
	for k, v := c.First(); k != nil; k, v = c.Next() {
		var broker workflow.Broker
		err := json.Unmarshal(v, &broker)
		if err != nil {
			return brokers, err
		}
	}

	return brokers, bx.Commit()
}

func (*Store) setQueueTxFunc(workflowID, k, v string) func(tx *bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		qbkt, err := tx.CreateBucketIfNotExists(queueBucket)

		bk, err := qbkt.CreateBucketIfNotExists([]byte(workflowID))

		if err != nil {
			return err
		}
		q := bk.Get([]byte(k))
		qs := make([]string, 0)
		if q != nil {
			err := json.Unmarshal(q, &qs)
			if err != nil {
				return err
			}
		}
		qs = append(qs, v)
		q, err = json.Marshal(qs)
		if err != nil {
			return err
		}
		err = bk.Put([]byte(k), q)
		if err != nil {
			return err
		}
		return nil
	}
}

func (*Store) getQueueTxFunc(workflowID string, value *map[string]string) func(tx *bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		qbkt, err := tx.CreateBucketIfNotExists(queueBucket)

		bk, err := qbkt.CreateBucketIfNotExists([]byte(workflowID))

		if err != nil {
			return err
		}
		qs := make([]string, 0)
		c := bk.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			err := json.Unmarshal(v, &qs)
			if err != nil {
				return err
			}
			f := qs[0]
			qs = qs[1:]
			if len(qs) == 0 {
				bk.Delete([]byte(k))
			} else {
				q, err := json.Marshal(qs)
				if err != nil {
					return err
				}
				bk.Put([]byte(k), q)
			}
			(*value)[string(k)] = string(f)
		}
		return nil
	}
}

func (s *Store) SetQueue(workflowID, k, v string) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.db.Update(func(tx *bolt.Tx) error {
		return s.setQueueTxFunc(workflowID, k, v)(tx)
	})
}

func (s *Store) GetQueue(workflowID string, value *map[string]string) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.db.Update(func(tx *bolt.Tx) error {
		return s.getQueueTxFunc(workflowID, value)(tx)
	})
}

func (s *Store) getTaskTxFunc(taskID string, task *Task) func(tx *bolt.Tx) error {
	return func(tx *bolt.Tx) error {

		taskBkt, err := tx.CreateBucketIfNotExists(tasksBucket)
		if err != nil {
			return err
		}
		v := taskBkt.Get([]byte(taskID))
		if v == nil {
			return ErrNotFound
		}
		err = json.Unmarshal(v, task)
		if err != nil {
			return err
		}
		task.logger = s.logger
		return nil
	}
}

func (s *Store) setTaskTxFunc(task *Task) func(tx *bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		taskBkt, err := tx.CreateBucketIfNotExists(tasksBucket)
		te := taskBkt.Get([]byte(task.ID))
		if te != nil {
			var et Task
			err := json.Unmarshal(te, &et)
			if err != nil {
				return err
			}

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

			if task.Schedule != et.Schedule {
				task.Next, err = task.GetNext()
				if err != nil {
					return err
				}
			} else {
				if task.Next.Before(et.Next) {
					task.Next = et.Next
				}
			}

		}
		bt, err := task.ToBytes()
		if err != nil {
			return err
		}
		taskBkt.Put([]byte(task.ID), bt)
		return nil
	}
}

func (s *Store) deleteTaskTxFunc(taskID string) func(tx *bolt.Tx) error {
	return func(tx *bolt.Tx) error {

		taskBkt, err := tx.CreateBucketIfNotExists(tasksBucket)
		if err != nil {
			return err
		}
		te := taskBkt.Get([]byte(taskID))
		if te == nil {
			return ErrNotFound
		}
		if err = taskBkt.Delete([]byte(taskID)); err != nil {
			return err
		}
		return nil
	}
}

func (s *Store) DeleteTask(id string) error {
	s.db.Update(func(tx *bolt.Tx) error {
		err := s.deleteTaskTxFunc(id)(tx)
		if err != nil {
			return err

		}
		return s.deleteExecutionsTxFunc(id)(tx)
	})
	return nil
}

func (s *Store) deleteExecutionsTxFunc(taskID string) func(tx *bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		eBkt, err := tx.CreateBucketIfNotExists(executionsBucket)
		if err != nil {
			return err
		}
		err = eBkt.DeleteBucket([]byte(taskID))
		return err
	}
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

func (s *Store) SetExecutionDone(taskID string, execution *Execution) (bool, error) {
	err := s.db.Update(func(tx *bolt.Tx) error {
		err := s.setExecutionTxFunc(taskID, execution)(tx)
		if err != nil {
			return err
		}
		var task Task
		err = s.getTaskTxFunc(taskID, &task)(tx)
		if err != nil {
			return err
		}

		if execution.Success {
			task.LastSuccess.Set(execution.FinishedAt)
			task.SuccessCount++
		} else {
			task.LastError.Set(execution.FinishedAt)
			task.ErrorCount++
		}
		return s.setTaskTxFunc(&task)(tx)
	})
	if err != nil {
		return false, err
	}

	return true, nil
}

func (s *Store) GetExecutions(taskID string, opts *ExecutionOptions) ([]*Execution, error) {
	s.logger.Debug(" executor run here 1")
	tx, err := s.db.Begin(true)

	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	s.logger.Debug(" executor run here 2")
	ebkt, err := tx.CreateBucketIfNotExists(executionsBucket)
	if err != nil {
		return nil, err
	}

	bk := ebkt.Bucket([]byte(taskID))
	if bk == nil {
		return nil, ErrNotFound
	}
	s.logger.Debug(" executor run here")
	kvs := make([]kv, 0)
	c := bk.Cursor()
	for k, v := c.First(); k != nil; k, v = c.Next() {
		kvs = append(kvs, kv{
			Value: v,
			Key:   string(k),
		})
	}
	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	return s.unmarshalExecutions(kvs, opts.Timezone)
}

func (s *Store) SetExecution(taskID string, execution *Execution) error {

	s.logger.WithFields(logrus.Fields{
		"task":      execution.TaskID,
		"execution": execution.Id,
		"finished":  execution.FinishedAt.String(),
	}).Debug("store: Setting key")

	return s.db.Update(func(tx *bolt.Tx) error {
		return s.setExecutionTxFunc(taskID, execution)(tx)
	})
}

func (*Store) setExecutionTxFunc(taskID string, execution *Execution) func(tx *bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		ebkt, err := tx.CreateBucketIfNotExists(executionsBucket)

		bk, err := ebkt.CreateBucketIfNotExists([]byte(taskID))

		if err != nil {
			return err
		}

		eb, err := json.Marshal(execution)
		if err != nil {
			return err
		}
		err = bk.Put([]byte(execution.Id), eb)
		if err != nil {
			return err
		}
		return nil
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
