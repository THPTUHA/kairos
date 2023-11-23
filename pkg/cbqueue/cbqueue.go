package cbqueue

import (
	"sync"
	"time"
)

type CBQueue struct {
	Mu   sync.Mutex
	Cond *sync.Cond
	head *AsyncCB
	tail *AsyncCB
}

type AsyncCB struct {
	fn   func(delay time.Duration)
	tm   time.Time
	next *AsyncCB
}

func (q *CBQueue) Dispatch() {
	for {
		q.Mu.Lock()
		for q.head == nil {
			q.Cond.Wait()
		}
		curr := q.head
		q.head = curr.next
		if curr == q.tail {
			q.tail = nil
		}
		q.Mu.Unlock()

		if curr.fn == nil {
			return
		}
		curr.fn(time.Since(curr.tm))
	}
}

func (q *CBQueue) Push(f func(duration time.Duration)) {
	q.PushOrClose(f, false)
}

func (q *CBQueue) PushOrClose(f func(time.Duration), close bool) {
	q.Mu.Lock()
	defer q.Mu.Unlock()
	if !close && f == nil {
		panic("pushing a nil callback with false close")
	}
	cb := &AsyncCB{fn: f, tm: time.Now()}
	if q.tail != nil {
		q.tail.next = cb
	} else {
		q.head = cb
	}
	q.tail = cb
	if close {
		q.Cond.Broadcast()
	} else {
		q.Cond.Signal()
	}
}

func (q *CBQueue) Close() {
	q.PushOrClose(nil, true)
}
