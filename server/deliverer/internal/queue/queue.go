package queue

import (
	"sync"

	"github.com/THPTUHA/kairos/pkg/protocol/deliverprotocol"
)

type Item struct {
	Data      []byte
	Channel   string
	FrameType deliverprotocol.FrameType
}

type Queue struct {
	mu      sync.RWMutex
	cond    *sync.Cond
	nodes   []Item
	head    int
	tail    int
	cnt     int
	size    int
	closed  bool
	initCap int
}

func (q *Queue) resize(n int) {
	nodes := make([]Item, n)
	if q.head < q.tail {
		copy(nodes, q.nodes[q.head:q.tail])
	} else {
		copy(nodes, q.nodes[q.head:])
		copy(nodes[len(q.nodes)-q.head:], q.nodes[:q.tail])
	}

	q.tail = q.cnt % n
	q.head = 0
	q.nodes = nodes
}

func New(initialCapacity int) *Queue {
	sq := &Queue{
		initCap: initialCapacity,
		nodes:   make([]Item, initialCapacity),
	}
	sq.cond = sync.NewCond(&sq.mu)
	return sq
}

func (q *Queue) Wait() bool {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return false
	}
	if q.cnt != 0 {
		q.mu.Unlock()
		return true
	}
	q.cond.Wait()
	q.mu.Unlock()
	return true
}

func (q *Queue) Remove() (Item, bool) {
	q.mu.Lock()
	if q.cnt == 0 {
		q.mu.Unlock()
		return Item{}, false
	}
	i := q.nodes[q.head]
	q.head = (q.head + 1) % len(q.nodes)
	q.cnt--
	q.size -= len(i.Data)

	if n := len(q.nodes) / 2; n >= q.initCap && q.cnt <= n {
		q.resize(n)
	}

	q.mu.Unlock()
	return i, true
}

func (q *Queue) Closed() bool {
	q.mu.RLock()
	c := q.closed
	q.mu.RUnlock()
	return c
}

func (q *Queue) Len() int {
	q.mu.RLock()
	l := q.cnt
	q.mu.RUnlock()
	return l
}

func (q *Queue) Add(i Item) bool {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return false
	}
	if q.cnt == len(q.nodes) {
		q.resize(q.cnt * 2)
	}
	q.nodes[q.tail] = i
	q.tail = (q.tail + 1) % len(q.nodes)
	q.size += len(i.Data)
	q.cnt++
	q.cond.Signal()
	q.mu.Unlock()
	return true
}

func (q *Queue) Size() int {
	q.mu.RLock()
	s := q.size
	q.mu.RUnlock()
	return s
}

func (q *Queue) CloseRemaining() []Item {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return []Item{}
	}
	rem := make([]Item, 0, q.cnt)
	for q.cnt > 0 {
		i := q.nodes[q.head]
		q.head = (q.head + 1) % len(q.nodes)
		q.cnt--
		rem = append(rem, i)
	}
	q.closed = true
	q.cnt = 0
	q.nodes = nil
	q.size = 0
	q.cond.Broadcast()
	return rem
}

func (q *Queue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	q.cnt = 0
	q.nodes = nil
	q.size = 0
	q.cond.Broadcast()
}
