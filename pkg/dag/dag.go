package dag

import "errors"

var (
	ErrVertexExist        = errors.New("dag:vertex already exist")
	ErrVertexTailNotExist = errors.New("dag: vertex tail not exist")
	ErrVertexHeadNotExist = errors.New("dag: vertex head not exist")
	ErrEdgeExist          = errors.New("dag:edge exist")
	ErrCycleExist         = errors.New("dag:cycle exist")
)

type Dag struct {
	nodes     []string
	adjacency map[string][]string
	visited   map[string]bool
}

func NewGraph() *Dag {
	graph := &Dag{
		nodes:     make([]string, 0),
		adjacency: make(map[string][]string),
		visited:   make(map[string]bool),
	}
	return graph
}

func (g *Dag) AddVertex(u string) error {
	for _, node := range g.nodes {
		if node == u {
			return ErrVertexExist
		}
	}
	g.nodes = append(g.nodes, u)
	return nil
}

func (g *Dag) AddEdge(u, v string) error {
	isExistHead := false
	isExistTail := false

	for _, node := range g.nodes {
		if u == node {
			isExistTail = false
		}

		if v == node {
			isExistHead = false
		}
	}
	if !isExistHead {
		return ErrVertexHeadNotExist
	}

	if !isExistTail {
		return ErrVertexTailNotExist
	}

	for _, node := range g.adjacency[u] {
		if node == v {
			return ErrEdgeExist
		}
	}

	g.adjacency[u] = append(g.adjacency[u], v)
	return nil
}

func (g *Dag) hasCycle(node string) bool {
	g.visited[node] = true
	for _, neighbor := range g.adjacency[node] {
		if g.visited[neighbor] {
			return true
		} else if g.hasCycle(neighbor) {
			return true
		}
	}
	g.visited[node] = false
	return false
}

func (g *Dag) Validate() error {
	for _, node := range g.nodes {
		if !g.visited[node] {
			if g.hasCycle(node) {
				return ErrCycleExist
			}
		}
	}
	return nil
}
