package runner

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/THPTUHA/kairos/server/httpserver/events"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/sirupsen/logrus"
)

type WorkerConfig struct {
	DeliverTaskChan      string
	MaxAttempDeliverTask int
	TimeoutRetryDeliver  time.Duration

	Logger *logrus.Entry
}

type natsOptions struct {
	url           string
	name          string
	reconnectWait time.Duration
	maxReconnects int
}

func defaultNatsOptions() *natsOptions {
	optsDef := natsOptions{
		name:          "worker",
		reconnectWait: 2 * time.Second,
		maxReconnects: 10,
	}
	return &optsDef

}

func optNats(o *natsOptions) []nats.Option {
	opts := make([]nats.Option, 0)
	opts = append(opts, nats.Name(o.name))
	opts = append(opts, nats.MaxReconnects(o.maxReconnects))
	opts = append(opts, nats.ReconnectWait(o.reconnectWait))
	return opts
}

type Worker struct {
	ID        int64
	eventChan chan int
	status    int
	conf      *WorkerConfig
	workflow  *workflow.Workflow

	natsOptions *natsOptions
	natsConn    *nats.Conn
}

func (w *Worker) run() {
	w.setupTask()
	log.Debug().Msg(fmt.Sprintf("start worker workflow running name = %s, namespace = %s", w.workflow.Name, w.workflow.Namespace))
	time.Sleep(5 * time.Second)
	log.Debug().Msg(fmt.Sprintf("finish worker workflow running name = %s, namespace = %s", w.workflow.Name, w.workflow.Namespace))
}

func (w *Worker) Run() error {
	w.conf.Logger.Debug(fmt.Sprintf("worker %d run ", w.ID))
	natConn, err := nats.Connect(nats.DefaultURL, optNats(defaultNatsOptions())...)
	if err != nil {
		return err
	}
	w.natsConn = natConn

	for {
		select {
		case cmd := <-w.eventChan:
			if cmd == events.DeleteWorker {
				log.Debug().Msg(fmt.Sprintf("delete worker id =%d, workflow running name = %s, namespace = %s", w.ID, w.workflow.Name, w.workflow.Namespace))
				return nil
			}
		default:
			w.run()
			return nil
		}
	}
}

func NewWorker(workerID int64, workflow *workflow.Workflow, conf *WorkerConfig) *Worker {
	return &Worker{
		ID:          workerID,
		eventChan:   make(chan int),
		status:      events.WorkerCreating,
		workflow:    workflow,
		natsOptions: defaultNatsOptions(),
		conf:        conf,
	}
}

func (w *Worker) IsRunning() bool {
	return w.status == events.WorkerRunning
}

func (w *Worker) Destroy() {
	w.eventChan <- events.DeleteWorker
}

func (w *Worker) deliverTask(t *workflow.Task) error {
	tj, err := json.Marshal(t)
	if err != nil {
		return err
	}
	for i := 0; i < w.conf.MaxAttempDeliverTask; i++ {
		err = w.natsConn.Publish(w.conf.DeliverTaskChan, tj)
		if err == nil {
			return nil
		}
		time.Sleep(w.conf.TimeoutRetryDeliver)
		w.conf.Logger.WithField("attemp deliver task", i+1).Error(err)
	}
	return err
}

func (w *Worker) setupTask() error {
	err := w.workflow.Tasks.Range(func(key string, value *workflow.Task) error {
		w.conf.Logger.Debug(fmt.Sprintf("task name = %s, executor: %s", key, value.Executor))
		switch value.Executor {
		case workflow.PubSubTask:
			// TODO
		case workflow.HttpTask:
			w.conf.Logger.Debug("deliver task")
			err := w.deliverTask(value)
			if err != nil {
				return err
			}
		}
		return nil
	})

	return err
}
