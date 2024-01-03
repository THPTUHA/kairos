package runner

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/THPTUHA/kairos/pkg/logger"
	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/THPTUHA/kairos/server/httpserver/events"
	"github.com/THPTUHA/kairos/server/messaging"
	"github.com/THPTUHA/kairos/server/storage"
	"github.com/go-redis/redis"
	"github.com/nats-io/nats.go"
	"github.com/panjf2000/ants"
	"github.com/rs/zerolog/log"
	"github.com/sirupsen/logrus"
)

var redisClient *redis.Client

type Runner struct {
	mu      sync.RWMutex
	wfMap   map[int64]*Worker
	sched   *Scheduler
	config  *Configs
	sub     *natsSub
	wfEvent chan *events.WfEvent
	log     *logrus.Entry
}

type natsSubConfig struct {
	url           string
	name          string
	reconnectWait time.Duration
	maxReconnects int
}

type natsSub struct {
	config *natsSubConfig
	Con    *nats.Conn
}

func NewRunner(file string) (*Runner, error) {
	log := logger.InitLogger(logrus.DebugLevel.String(), "runner")

	config, err := SetConfig(file)
	if err != nil {
		return nil, err
	}
	fmt.Printf("CONFIG %+v", config)
	var r = Runner{
		config:  config,
		wfMap:   make(map[int64]*Worker),
		sched:   NewScheduler(log),
		wfEvent: make(chan *events.WfEvent),
		log:     log,
	}

	sub, err := r.NewNatsSub(&natsSubConfig{
		url:           config.Nats.URL,
		name:          "runner",
		reconnectWait: 2 * time.Second,
		maxReconnects: 10,
	})

	r.sub = sub
	return &r, err
}

func (r *Runner) Start() error {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals,
		os.Interrupt,
	)
	go func() {
		<-signals
		os.Exit(0)
	}()
	err := storage.Connect(fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		r.config.DB.Postgres.URI,
		r.config.DB.Postgres.Port,
		r.config.DB.Postgres.Username,
		r.config.DB.Postgres.Password,
		r.config.DB.Postgres.DatabaseName,
	))
	if err != nil {
		return err
	}

	go r.listenMB()
	go r.execute(signals)

	r.log.Info(fmt.Printf("Runner running on %d \n", r.config.Runner.Port))
	if err := http.ListenAndServe(fmt.Sprintf(":%d", r.config.Runner.Port), nil); err != nil {
		return err
	}
	return nil
}

func (r *Runner) execute(signals chan os.Signal) {
	// connectRedis()
	p, err := ants.NewPoolWithFunc(r.config.Runner.MaxWorkflowConcurrent, func(wf interface{}) {
		wfe := wf.(*events.WfEvent)

		switch wfe.Cmd {
		case events.WfCmdCreate:
			r.log.Debugf("Add workflow id = %d to worker", wfe.Workflow.ID)
			r.mu.Lock()
			worker := NewWorker(wfe.Workflow.ID, wfe.Workflow, &WorkerConfig{
				DeliverDelay:         time.Duration(r.config.Runner.DeliverDelay) * time.Second,
				MaxAttempDeliverTask: r.config.Runner.MaxAttempDeliverTask,
				TimeoutRetryDeliver:  time.Duration(r.config.Runner.TimeoutRetryDeliver) * time.Second,
				DeliverTimeout:       time.Duration(r.config.Runner.DeliverTimeout) * time.Second,
				Logger:               logger.InitLogger("debug", "worker"),
				NatURL:               r.config.Nats.URL,
			}, r.sched)
			worker.status = workflow.Pending
			worker.workflow.Status = workflow.Pending
			r.wfMap[wfe.Workflow.ID] = worker
			r.mu.Unlock()

			err := worker.Run()
			if err != nil {
				r.log.Error(err)
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
	}, ants.WithPreAlloc(true))

	if err != nil {
		r.log.Error(err)
	}

	defer p.Release()

	go func() {
		for wf := range r.wfEvent {
			r.log.Infof("Start workflow name=%s  namespace=%s", wf.Workflow.Name, wf.Workflow.Namespace)
			err := p.Invoke(wf)
			if err != nil {
				r.log.Error(err)
			}
		}
	}()

	err = r.startInitWorkflow(true)
	if err != nil {
		r.log.Error(err)
	}

	<-signals
	r.log.Warning("runner stopped")
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

		for _, id := range ids {
			r.log.Debugf("[INIT WORKFLOW] %d \n", id)
			wf, err := storage.DetailWorkflow(id, "")
			if err != nil {
				return err
			}

			if err = wf.Compile(nil); err != nil {
				return err
			}

			r.wfEvent <- &events.WfEvent{
				Cmd:      events.WfCmdCreate,
				Workflow: wf,
			}
		}
	}
	return nil
}

func (r *Runner) GetWfInfo(wfIDs []int64) []*workflow.Workflow {
	wfs := make([]*workflow.Workflow, 0)
	for _, id := range wfIDs {
		we := r.wfMap[id]
		if we != nil {
			r.log.Debugf("Workflow id = %d status = %d\n", we.workflow.ID, we.workflow.Status)
			wfs = append(wfs, we.workflow)
		}
	}
	return wfs
}

func (r *Runner) listenMB() {
	_, err := r.sub.Con.Subscribe(messaging.RUNNER_HTTP, func(msg *nats.Msg) {
		var e events.WfEvent
		err := json.Unmarshal(msg.Data, &e)
		if err != nil {
			r.log.Error(err)
			return
		}
		r.log.Debugf("Receiver event wf %+v", e)
		if e.Cmd == events.WfCmdInfo {
			wfs := r.GetWfInfo(e.WfIDs)
			data, err := json.Marshal(wfs)
			if err != nil {
				r.log.Error(err)
				return
			}
			msg.Respond(data)
			return
		} else {
			r.wfEvent <- &e
		}
	})
	if err != nil {
		r.log.Error(err)
	}
}

func (r *Runner) NewNatsSub(config *natsSubConfig) (*natsSub, error) {
	nc, err := nats.Connect(config.url, optNats(config)...)
	if err != nil {
		r.log.Error(err)
		return nil, err
	}
	return &natsSub{
		config: config,
		Con:    nc,
	}, nil
}

func optNats(o *natsSubConfig) []nats.Option {
	opts := make([]nats.Option, 0)
	opts = append(opts, nats.Name(o.name))
	opts = append(opts, nats.MaxReconnects(o.maxReconnects))
	opts = append(opts, nats.ReconnectWait(o.reconnectWait))
	return opts
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
