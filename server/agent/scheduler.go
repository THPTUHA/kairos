package agent

import (
	"expvar"
	"sync"

	"github.com/THPTUHA/kairos/pkg/extcron"
	"github.com/robfig/cron/v3"
)

var (
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
