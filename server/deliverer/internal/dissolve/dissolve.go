package dissolve

import (
	"errors"
	"runtime"
)

type Dissolver struct {
	queue      queue
	numWorkers int
}

func New(numWorkers int) *Dissolver {
	return &Dissolver{
		queue:      newQueue(),
		numWorkers: numWorkers,
	}
}

func (d *Dissolver) Run() error {
	for i := 0; i < d.numWorkers; i++ {
		go d.runWorker()
	}
	return nil
}

func (d *Dissolver) runWorker() {
	for {
		job, ok := d.queue.Wait()
		if !ok {
			if d.queue.Closed() {
				break
			}
			continue
		}
		err := job()
		if err != nil {
			runtime.Gosched()
			d.queue.Add(job)
		}
	}
}

func (d *Dissolver) Submit(job Job) error {
	if !d.queue.Add(job) {
		return errors.New("can not submit job to closed dissolver")
	}
	return nil
}

func (d *Dissolver) Close() error {
	d.queue.Close()
	return nil
}
