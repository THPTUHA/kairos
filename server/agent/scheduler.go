package agent

import (
	"context"
	"expvar"
	"sync"

	"github.com/THPTUHA/kairos/pkg/extcron"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
)

var (
	cronInspect      = expvar.NewMap("cron_entries")
	schedulerStarted = expvar.NewInt("scheduler_started")
)

// Scheduler represents a agent scheduler instance, it stores the cron engine
// and the related parameters.
type Scheduler struct {
	// mu is to prevent concurrent edits to Cron and Started
	mu      sync.RWMutex
	Cron    *cron.Cron
	started bool
}

// NewScheduler creates a new Scheduler instance
func NewScheduler() *Scheduler {
	schedulerStarted.Set(0)
	return &Scheduler{
		Cron:    cron.New(cron.WithParser(extcron.NewParser())),
		started: false,
	}
}

// TODO
// Start the cron scheduler, adding its corresponding jobs and
// executing them on time.
func (s *Scheduler) Start(jobs []*Job, agent *Agent) error {
	return nil
}

// Stop stops the cron scheduler if it is running; otherwise it does nothing.
// A context is returned so the caller can wait for running jobs to complete.
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
