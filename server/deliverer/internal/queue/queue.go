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

// Queue is an unbounded queue of Item.
// The queue is goroutine safe.
// Inspired by http://blog.dubbelboer.com/2015/04/25/go-faster-queue.html (MIT)
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

// WriteMany mutex must be held when calling
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

// New Queue returns a new Item queue with initial capacity.
func New(initialCapacity int) *Queue {
	sq := &Queue{
		initCap: initialCapacity,
		nodes:   make([]Item, initialCapacity),
	}
	sq.cond = sync.NewCond(&sq.mu)
	return sq
}

// Wait for a message to be added.
// If there are items on the queue will return immediately.
// Will return false if the queue is closed.
// Otherwise, returns true.
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

// Remove will remove an Item from the queue.
// If false is returned, it either means 1) there were no items on the queue
// or 2) the queue is closed.
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

// Closed returns true if the queue has been closed
// The call cannot guarantee that the queue hasn't been
// closed while the function returns, so only "true" has a definite meaning.
func (q *Queue) Closed() bool {
	q.mu.RLock()
	c := q.closed
	q.mu.RUnlock()
	return c
}

// Len returns the current length of the queue.
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
		// Also tested a growth rate of 1.5, see: http://stackoverflow.com/questions/2269063/buffer-growth-strategy
		// In Go this resulted in a higher memory usage.
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

// Size returns the current size of the queue.
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
