package pubsub

import (
	"sync"
	"time"
)

type cbQueue struct {
	mu   sync.Mutex
	cond *sync.Cond
	head *asyncCB
	tail *asyncCB
}

type asyncCB struct {
	fn   func(delay time.Duration)
	tm   time.Time
	next *asyncCB
}

// dispatch is responsible for calling async callbacks. Should be run
// in separate goroutine.
func (q *cbQueue) dispatch() {
	for {
		q.mu.Lock()
		// Protect for spurious wake-ups. We should get out of the
		// wait only if there is an element to pop from the list.
		for q.head == nil {
			q.cond.Wait()
		}
		curr := q.head
		q.head = curr.next
		if curr == q.tail {
			q.tail = nil
		}
		q.mu.Unlock()

		// This signals that the dispatcher has been closed and all
		// previous callbacks have been dispatched.
		if curr.fn == nil {
			return
		}
		curr.fn(time.Since(curr.tm))
	}
}

// Push adds the given function to the tail of the list and
// signals the dispatcher.
func (q *cbQueue) push(f func(duration time.Duration)) {
	q.pushOrClose(f, false)
}

func (q *cbQueue) pushOrClose(f func(time.Duration), close bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	// Make sure that library is not calling push with nil function,
	// since this is used to notify the dispatcher that it must stop.
	if !close && f == nil {
		panic("pushing a nil callback with false close")
	}
	cb := &asyncCB{fn: f, tm: time.Now()}
	if q.tail != nil {
		q.tail.next = cb
	} else {
		q.head = cb
	}
	q.tail = cb
	if close {
		q.cond.Broadcast()
	} else {
		q.cond.Signal()
	}
}
