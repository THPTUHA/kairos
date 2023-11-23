package agent

import (
	"context"
	"errors"
	"expvar"
	"fmt"
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

type Scheduler struct {
	mu      sync.RWMutex
	Cron    *cron.Cron
	started bool
	logger  *logrus.Entry
}

func NewScheduler(logger *logrus.Entry) *Scheduler {
	schedulerStarted.Set(0)
	return &Scheduler{
		Cron:    cron.New(cron.WithParser(extcron.NewParser())),
		started: false,
		logger:  logger,
	}
}

func (s *Scheduler) ClearCron() {
	for _, e := range s.Cron.Entries() {
		if t, ok := e.Job.(*Task); !ok {
			s.logger.Errorf("scheduler: Failed to cast task to *Task found type %T and removing it", e.Job)
			s.Cron.Remove(e.ID)
		} else {
			s.RemoveTask(t.ID)
		}
	}
}

func (s *Scheduler) AddTask(task *Task) error {
	s.logger.Debug(fmt.Sprintf("schedule task name = %s, id =%s", task.Name, task.ID))
	if _, ok := s.GetEntryTask(task.ID); ok {
		s.RemoveTask(task.ID)
	}

	if task.Disabled {
		return nil
	}

	schedule := task.Schedule
	if task.Timezone != "" &&
		!strings.HasPrefix(schedule, "@") &&
		!strings.HasPrefix(schedule, "TZ=") &&
		!strings.HasPrefix(schedule, "CRON_TZ=") {
		schedule = "CRON_TZ=" + task.Timezone + " " + schedule
	}
	s.logger.WithFields(logrus.Fields{
		"task":     task.ID,
		"schedule": task.Schedule,
	}).Debug("scheduler: Adding task to cron")
	task.logger = s.logger
	_, err := s.Cron.AddJob(schedule, task)
	if err != nil {
		return err
	}
	s.Cron.Start()
	cronInspect.Set(task.ID, task)

	return nil
}

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

func (s *Scheduler) Started() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.started
}

func (s *Scheduler) GetEntryTask(taskID string) (EntryTask, bool) {
	for _, e := range s.Cron.Entries() {
		if j, ok := e.Job.(*Task); !ok {
			s.logger.Errorf("scheduler: Failed to cast task to *Task found type %T", e.Job)
		} else {
			j.logger = s.logger
			if j.ID == taskID {
				return EntryTask{
					entry: &e,
					task:  j,
				}, true
			}
		}
	}
	return EntryTask{}, false
}

func (s *Scheduler) RemoveTask(taskID string) {
	s.logger.WithFields(logrus.Fields{
		"task": taskID,
	}).Info("scheduler: Removing task from cron")

	if ej, ok := s.GetEntryTask(taskID); ok {
		s.Cron.Remove(ej.entry.ID)
		cronInspect.Delete(taskID)
	}
}
