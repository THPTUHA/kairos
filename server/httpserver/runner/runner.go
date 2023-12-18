package runner

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/THPTUHA/kairos/pkg/logger"
	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/THPTUHA/kairos/server/httpserver/events"
	"github.com/THPTUHA/kairos/server/storage"
	"github.com/THPTUHA/kairos/server/storage/models"
	"github.com/go-redis/redis"
	"github.com/panjf2000/ants"
	"github.com/rs/zerolog/log"
	"github.com/sirupsen/logrus"
)

type Configs struct {
	MaxWorkflowConcurrent int
	Logger                *logrus.Entry
}

var redisClient *redis.Client

type Runner struct {
	mu     sync.RWMutex
	wfMap  map[int64]*Worker
	sched  *Scheduler
	config Configs
}

func NewRunner(config Configs) *Runner {
	return &Runner{
		config: config,
		wfMap:  make(map[int64]*Worker),
		sched:  NewScheduler(config.Logger),
	}
}

func (r *Runner) Start(signals chan os.Signal) {
	var wg sync.WaitGroup
	// connectRedis()
	p, _ := ants.NewPoolWithFunc(r.config.MaxWorkflowConcurrent, func(wf interface{}) {
		wfe := wf.(*events.WfEvent)

		switch wfe.Cmd {
		case events.WfCmdCreate:
			log.Debug().Msg(fmt.Sprintf("Add workflow id = %d to worker", wfe.Workflow.ID))
			r.mu.Lock()
			worker := NewWorker(wfe.Workflow.ID, wfe.Workflow, &WorkerConfig{
				DeliverDelay:         1 * time.Second,
				MaxAttempDeliverTask: 10,
				TimeoutRetryDeliver:  2 * time.Second,
				DeliverTimeout:       5 * time.Second,
				Logger:               logger.InitLogger("debug", "worker"),
			}, r.sched)
			worker.status = workflow.Pending
			worker.workflow.Status = workflow.Pending
			r.wfMap[wfe.Workflow.ID] = worker
			r.mu.Unlock()

			err := worker.Run()
			if err != nil {
				fmt.Println("Worker run err", err)
				r.mu.Lock()
				delete(r.wfMap, wfe.Workflow.ID)
				r.mu.Unlock()
				return
			}

			r.mu.Lock()
			delete(r.wfMap, wfe.Workflow.ID)
			r.mu.Unlock()
			break
		case events.WfCmdDelete:
			go r.DestroyWorker(wfe)
		case events.WfCmdRecover:
			go r.RecoverWorker(wfe)
		}
		wg.Done()
	}, ants.WithPreAlloc(true))

	defer p.Release()

	go func() {
		for wf := range events.Get() {
			log.Debug().Msg(fmt.Sprintf("Start workflow name=%s  namespace=%s", wf.Workflow.Name, wf.Workflow.Namespace))
			wg.Add(1)
			err := p.Invoke(wf)
			if err != nil {
				log.Error().Stack().Err(err).Msg("workflow run error")
			}
		}
	}()

	err := r.startInitWorkflow(true)
	if err != nil {
		fmt.Printf("[INIT WORKFLOW ERR] ERR = %s \n", err.Error())
		return
	}

	wg.Wait()
	<-signals
	log.Info().Msg("exist runner")
}

func (r *Runner) DestroyWorker(wfe *events.WfEvent) {
	log.Debug().Msg(fmt.Sprintf("Delete workflow id=%d", wfe.Workflow.ID))
	r.mu.Lock()
	worker := r.wfMap[wfe.Workflow.ID]
	r.mu.Unlock()
	var success bool
	if worker != nil {
		success = worker.Destroy()
	}

	if success {
		r.mu.Lock()
		delete(r.wfMap, wfe.Workflow.ID)
		r.mu.Unlock()
	}
}

func (r *Runner) RecoverWorker(wfe *events.WfEvent) {
	log.Debug().Msg(fmt.Sprintf("Recover workflow id=%d", wfe.Workflow.ID))
	r.mu.Lock()
	worker := r.wfMap[wfe.Workflow.ID]
	r.mu.Unlock()
	if worker != nil {
		worker.Recover()
	}
}

func (r *Runner) startInitWorkflow(isFirst bool) error {
	if isFirst {
		ids, err := storage.GetWorkflows()
		if err != nil {
			return err
		}

		r.config.Logger.Info("")

		for _, id := range ids {
			fmt.Printf("[INIT WORKFLOW ] %d \n", id)
			wf, err := storage.DetailWorkflow(id, "")
			if err != nil {
				return err
			}

			if err = wf.Compile(nil); err != nil {
				return err
			}

			events.Get() <- &events.WfEvent{
				Cmd:      events.WfCmdCreate,
				Workflow: wf,
			}
		}
	}
	return nil
}

func (r *Runner) SetWfStatus(wfs []*models.Workflow) {
	for _, wf := range wfs {
		we := r.wfMap[wf.ID]
		if we != nil {
			fmt.Printf("Workflow id = %d status = %d\n", we.workflow.ID, we.workflow.Status)
			wf.Status = we.workflow.Status
		}
	}
}

// func connectRedis() error {
// 	client := redis.NewClient(&redis.Options{
// 		Addr:     "localhost:6379",
// 		Password: "",
// 		DB:       0,
// 	})
// 	_, err := client.Ping().Result()
// 	if err != nil {
// 		fmt.Printf("[RUNNER CONNECT REDIS ERR] %+v\n", err)
// 		return err
// 	}
// 	redisClient = client
// 	return nil
// }
