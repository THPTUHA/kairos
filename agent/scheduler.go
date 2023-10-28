package agent

import (
	"context"
	"errors"
	"expvar"
	"strings"
	"sync"

	"github.com/THPTUHA/kairos/pkg/extcron"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	"github.com/sirupsen/logrus"
)

var (
	cronInspect      = expvar.NewMap("cron_entries")
	schedulerStarted = expvar.NewInt("scheduler_started")

	ErrScheduleParse = errors.New("can't parse task schedule")
)

type EntryTask struct {
	entry *cron.Entry
	task  *Task
}

// Scheduler represents a dkron scheduler instance, it stores the cron engine
// and the related parameters.
type Scheduler struct {
	// mu is to prevent concurrent edits to Cron and Started
	mu      sync.RWMutex
	Cron    *cron.Cron
	started bool
	logger  *logrus.Entry
}

// NewScheduler creates a new Scheduler instance
func NewScheduler(logger *logrus.Entry) *Scheduler {
	schedulerStarted.Set(0)
	return &Scheduler{
		Cron:    cron.New(cron.WithParser(extcron.NewParser())),
		started: false,
		logger:  logger,
	}
}

// ClearCron clears the cron scheduler
func (s *Scheduler) ClearCron() {
	for _, e := range s.Cron.Entries() {
		if j, ok := e.Job.(*Task); !ok {
			s.logger.Errorf("scheduler: Failed to cast task to *Task found type %T and removing it", e.Job)
			s.Cron.Remove(e.ID)
		} else {
			s.RemoveTask(j.Key)
		}
	}
}

// AddTask Adds a task to the cron scheduler
func (s *Scheduler) AddTask(task *Task) error {
	// Check if the task is already set and remove it if exists
	if _, ok := s.GetEntryTask(task.Key); ok {
		s.RemoveTask(task.Key)
	}

	if task.Disabled {
		return nil
	}

	s.logger.WithFields(logrus.Fields{
		"task": task.Key,
	}).Debug("scheduler: Adding task to cron")

	// If Timezone is set on the task, and not explicitly in its schedule,
	// AND its not a descriptor (that don't support timezones), add the
	// timezone to the schedule so robfig/cron knows about it.
	schedule := task.Schedule
	if task.Timezone != "" &&
		!strings.HasPrefix(schedule, "@") &&
		!strings.HasPrefix(schedule, "TZ=") &&
		!strings.HasPrefix(schedule, "CRON_TZ=") {
		schedule = "CRON_TZ=" + task.Timezone + " " + schedule
	}

	_, err := s.Cron.AddJob(schedule, task)
	if err != nil {
		return err
	}

	cronInspect.Set(task.Key, task)

	return nil
}

// Start the cron scheduler, adding its corresponding tasks and
// executing them on time.
func (s *Scheduler) Start(tasks []*Task, agent *Agent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return errors.New("scheduler: cron already started, should be stopped first")
	}
	s.ClearCron()

	for _, task := range tasks {
		task.Agent = agent
		if err := s.AddTask(task); err != nil {
			return err
		}
	}
	s.Cron.Start()
	s.started = true
	schedulerStarted.Set(1)

	return nil
}

// Stop stops the cron scheduler if it is running; otherwise it does nothing.
// A context is returned so the caller can wait for running tasks to complete.
func (s *Scheduler) Stop() context.Context {
	ctx := s.Cron.Stop()
	if s.started {
		log.Info().Msg("scheduler: Stopping scheduler")
		s.started = false

		// expvars
		cronInspect.Do(func(kv expvar.KeyValue) {
			kv.Value = nil
		})
	}
	schedulerStarted.Set(0)
	return ctx
}

// Started will safely return if the scheduler is started or not
func (s *Scheduler) Started() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.started
}

// GetEntryTask returns a EntryTask object from a snapshot in
// the current time, and whether or not the entry was found.
func (s *Scheduler) GetEntryTask(taskID string) (EntryTask, bool) {
	for _, e := range s.Cron.Entries() {
		if j, ok := e.Job.(*Task); !ok {
			s.logger.Errorf("scheduler: Failed to cast task to *Task found type %T", e.Job)
		} else {
			j.logger = s.logger
			if j.Key == taskID {
				return EntryTask{
					entry: &e,
					task:  j,
				}, true
			}
		}
	}
	return EntryTask{}, false
}

// RemoveTask removes a task from the cron scheduler if it exists.
func (s *Scheduler) RemoveTask(taskID string) {
	s.logger.WithFields(logrus.Fields{
		"task": taskID,
	}).Debug("scheduler: Removing task from cron")

	if ej, ok := s.GetEntryTask(taskID); ok {
		s.Cron.Remove(ej.entry.ID)
		cronInspect.Delete(taskID)
	}
}
