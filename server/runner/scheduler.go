package runner

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"strings"
	"sync"

	"github.com/THPTUHA/kairos/pkg/extcron"
	"github.com/THPTUHA/kairos/pkg/workflow"
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
	task  *workflow.Task
}

type EntryBroker struct {
	entry  *cron.Entry
	broker *workflow.Broker
}

type EntryChannl struct {
	entry   *cron.Entry
	channel *workflow.Channel
}
type Scheduler struct {
	mu          sync.RWMutex
	Cron        *cron.Cron
	BrokerCron  *cron.Cron
	ChannelCron *cron.Cron
	started     bool
	logger      *logrus.Entry
}

func NewScheduler(logger *logrus.Entry) *Scheduler {
	schedulerStarted.Set(0)
	return &Scheduler{
		Cron:        cron.New(cron.WithParser(extcron.NewParser())),
		BrokerCron:  cron.New(cron.WithParser(extcron.NewParser())),
		ChannelCron: cron.New(cron.WithParser(extcron.NewParser())),
		started:     false,
		logger:      logger,
	}
}

func (s *Scheduler) ClearCron() {
	for _, e := range s.Cron.Entries() {
		if t, ok := e.Job.(*workflow.Task); !ok {
			s.logger.Errorf("scheduler: Failed to cast task to *Task found type %T and removing it", e.Job)
			s.Cron.Remove(e.ID)
		} else {
			s.RemoveTask(t.ID)
		}
	}
}

func (s *Scheduler) AddTask(worker *Worker, task *workflow.Task, input string) (cron.EntryID, error) {
	s.logger.Debug(fmt.Sprintf("schedule task name = %s, id =%d", task.Name, task.ID))
	if _, ok := s.GetEntryTask(task.ID); ok {
		s.RemoveTask(task.ID)
	}

	schedule := task.Schedule
	if schedule == "" {
		worker.RunTask(task, input)
		return -1, nil
	}
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
	tjid, err := s.Cron.AddJob(schedule, task)
	if err != nil {
		return -1, err
	}
	s.Cron.Start()
	return tjid, nil
}

func (s *Scheduler) AddBroker(worker *Worker, broker *workflow.Broker, trigger *workflow.Trigger) (cron.EntryID, error) {
	s.logger.Debug(fmt.Sprintf("schedule broker name = %s, id =%d", broker.Name, broker.ID))
	if _, ok := s.GetEntryBroker(broker.ID); ok {
		s.RemoveTask(broker.ID)
	}

	schedule := broker.Schedule
	if schedule == "" {
		worker.RunBroker(broker, trigger.Input, trigger.ID)
		return -1, nil
	}

	s.logger.WithFields(logrus.Fields{
		"broker":   broker.ID,
		"schedule": broker.Schedule,
	}).Debug("scheduler: Adding broker to cron")
	tjid, err := s.BrokerCron.AddJob(schedule, broker)
	if err != nil {
		return -1, err
	}
	s.BrokerCron.Start()
	return tjid, nil
}

func (s *Scheduler) AddChannel(worker *Worker, channel *workflow.Channel, trigger *workflow.Trigger) (cron.EntryID, error) {
	s.logger.Debug(fmt.Sprintf("schedule channel name = %s, id =%d", channel.Name, channel.ID))
	if _, ok := s.GetEntryBroker(channel.ID); ok {
		s.RemoveTask(channel.ID)
	}

	schedule := channel.Schedule
	if schedule == "" {
		worker.RunChannel(channel, trigger.Input, trigger.ID)
		return -1, nil
	}

	s.logger.WithFields(logrus.Fields{
		"channel":  channel.ID,
		"schedule": channel.Schedule,
	}).Debug("scheduler: Adding broker to cron")
	tjid, err := s.ChannelCron.AddJob(schedule, channel)
	if err != nil {
		return -1, err
	}
	s.ChannelCron.Start()
	return tjid, nil
}

func (s *Scheduler) GetEntryChannel(channelID int64) (EntryChannl, bool) {
	for _, e := range s.Cron.Entries() {
		if c, ok := e.Job.(*workflow.Channel); !ok {
			s.logger.Errorf("scheduler: Failed to cast broker to *Broker found type %T", e.Job)
		} else {
			if c.ID == channelID {
				return EntryChannl{
					entry:   &e,
					channel: c,
				}, true
			}
		}
	}
	return EntryChannl{}, false
}

// func (s *Scheduler) Start(tasks []*workflow.Task) error {
// 	s.mu.Lock()
// 	defer s.mu.Unlock()

// 	if s.started {
// 		return errors.New("scheduler: cron already started, should be stopped first")
// 	}
// 	s.ClearCron()

// 	for _, task := range tasks {
// 		if _, err := s.AddTask(task); err != nil {
// 			return err
// 		}
// 	}
// 	s.Cron.Start()
// 	s.started = true
// 	schedulerStarted.Set(1)
// 	return nil
// }

func (s *Scheduler) Stop() context.Context {
	ctx := s.Cron.Stop()
	if s.started {
		log.Info().Msg("scheduler: Stopping scheduler")
		s.started = false

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

func (s *Scheduler) GetEntryTask(taskID int64) (EntryTask, bool) {
	for _, e := range s.Cron.Entries() {
		if j, ok := e.Job.(*workflow.Task); !ok {
			s.logger.Errorf("scheduler: Failed to cast task to *Task found type %T", e.Job)
		} else {
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

func (s *Scheduler) GetEntryBroker(brokerID int64) (EntryBroker, bool) {
	for _, e := range s.Cron.Entries() {
		if j, ok := e.Job.(*workflow.Broker); !ok {
			s.logger.Errorf("scheduler: Failed to cast broker to *Broker found type %T", e.Job)
		} else {
			if j.ID == brokerID {
				return EntryBroker{
					entry:  &e,
					broker: j,
				}, true
			}
		}
	}
	return EntryBroker{}, false
}

func (s *Scheduler) RemoveTask(taskID int64) {
	s.logger.WithFields(logrus.Fields{
		"task": taskID,
	}).Info("scheduler: Removing task from cron")

	if ej, ok := s.GetEntryTask(taskID); ok {
		s.Cron.Remove(ej.entry.ID)
		cronInspect.Delete(fmt.Sprint(taskID))
	}
}

func (s *Scheduler) RemoveBroker(brokerID int64) {
	s.logger.WithFields(logrus.Fields{
		"broker": brokerID,
	}).Info("scheduler: Removing broker from cron")

	if ej, ok := s.GetEntryBroker(brokerID); ok {
		s.BrokerCron.Remove(ej.entry.ID)
		cronInspect.Delete(fmt.Sprint(brokerID))
	}
}
