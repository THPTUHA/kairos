package priority

import "container/heap"

type Item struct {
	Value    string
	Priority int64
	index    int
}

type Queue []*Item

func (pq Queue) Len() int {
	return len(pq)
}

func (pq Queue) Less(i, j int) bool {
	return pq[i].Priority < pq[j].Priority
}

func (pq Queue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *Queue) Push(x any) {
	n := len(*pq)
	item := x.(*Item)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *Queue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}

func MakeQueue() Queue {
	pq := make(Queue, 0)
	heap.Init(&pq)
	return pq
}
