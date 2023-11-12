package runner

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/THPTUHA/kairos/server/httpserver/events"
	"github.com/panjf2000/ants"
	"github.com/rs/zerolog/log"
	"github.com/sirupsen/logrus"
)

type Configs struct {
	MaxWorkflowConcurrent int
	Logger                *logrus.Entry
}

type Runner struct {
	mu     sync.RWMutex
	wfMap  map[int64]*Worker
	config Configs
}

func NewRunner(config Configs) *Runner {
	return &Runner{
		config: config,
		wfMap:  make(map[int64]*Worker),
	}
}

func (r *Runner) Start(signals chan os.Signal) {
	var wg sync.WaitGroup
	p, _ := ants.NewPoolWithFunc(r.config.MaxWorkflowConcurrent, func(wf interface{}) {
		wfe := wf.(*events.WfEvent)

		switch wfe.Cmd {
		case events.WfCmdCreate:
			log.Debug().Msg(fmt.Sprintf("Add workflow id = %d to worker", wfe.Workflow.ID))
			r.mu.Lock()
			worker := NewWorker(wfe.Workflow.ID, wfe.Workflow, &WorkerConfig{
				MaxAttempDeliverTask: 10,
				DeliverTaskChan:      "deliver",
				TimeoutRetryDeliver:  2 * time.Second,
				Logger:               r.config.Logger,
			})
			r.wfMap[wfe.Workflow.ID] = worker
			r.mu.Unlock()

			worker.Run()

			r.mu.Lock()
			delete(r.wfMap, wfe.Workflow.ID)
			r.mu.Unlock()
			break
		case events.WfCmdDelete:
			log.Debug().Msg(fmt.Sprintf("Delete workflow id=%d", wfe.Workflow.ID))
			r.mu.Lock()
			worker := r.wfMap[wfe.Workflow.ID]
			r.mu.Unlock()
			if worker != nil {
				worker.Destroy()
			}

			r.mu.Lock()
			delete(r.wfMap, wfe.Workflow.ID)
			r.mu.Unlock()
		}

		wg.Done()
	}, ants.WithPreAlloc(true))

	defer p.Release()

	go func() {
		for wf := range events.WfChan {
			log.Debug().Msg(fmt.Sprintf("Start workflow name=%s  namespace=%s", wf.Workflow.Name, wf.Workflow.Namespace))
			wg.Add(1)
			err := p.Invoke(wf)
			if err != nil {
				log.Error().Stack().Err(err).Msg("workflow run error")
			}
		}
	}()

	wg.Wait()
	<-signals
	log.Info().Msg("exist runner")
}
