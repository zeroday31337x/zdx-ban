package ban

import (
	"errors"
	"sort"
	"sync"
)

type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}
type Graph struct {
	mu                 sync.RWMutex
	nodes              map[string]*State
	fingerprints       map[string]string
	edges              []Edge
	maxDepth, maxNodes int
}

func NewGraph(depth, nodes int) *Graph {
	return &Graph{nodes: map[string]*State{}, fingerprints: map[string]string{}, maxDepth: depth, maxNodes: nodes}
}
func (g *Graph) AddNode(s *State) (*State, bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if s.Depth > g.maxDepth {
		return nil, false, errors.New("maximum depth exceeded")
	}
	if id, ok := g.fingerprints[Fingerprint(s)]; ok {
		return g.nodes[id], true, nil
	}
	if len(g.nodes) >= g.maxNodes {
		return nil, false, errors.New("maximum nodes exceeded")
	}
	if _, ok := g.nodes[s.ID]; ok {
		return nil, false, errors.New("duplicate id")
	}
	g.nodes[s.ID] = s
	g.fingerprints[Fingerprint(s)] = s.ID
	return s, false, nil
}
func (g *Graph) AddEdge(from, to string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if from == to || g.reaches(to, from, map[string]bool{}) {
		return errors.New("cycle rejected")
	}
	a, aok := g.nodes[from]
	b, bok := g.nodes[to]
	if !aok || !bok {
		return errors.New("unknown node")
	}
	for _, e := range g.edges {
		if e.From == from && e.To == to {
			return nil
		}
	}
	g.edges = append(g.edges, Edge{from, to})
	a.ChildIDs = append(a.ChildIDs, to)
	b.ParentIDs = append(b.ParentIDs, from)
	return nil
}
func (g *Graph) reaches(a, b string, seen map[string]bool) bool {
	if a == b {
		return true
	}
	if seen[a] {
		return false
	}
	seen[a] = true
	for _, e := range g.edges {
		if e.From == a && g.reaches(e.To, b, seen) {
			return true
		}
	}
	return false
}
func (g *Graph) Get(id string) (*State, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	s, ok := g.nodes[id]
	return s, ok
}
func (g *Graph) Nodes() []*State {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]*State, 0, len(g.nodes))
	for _, s := range g.nodes {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func (g *Graph) Edges() []Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return append([]Edge(nil), g.edges...)
}
func (g *Graph) Prune(id string) error {
	s, ok := g.Get(id)
	if !ok {
		return errors.New("unknown node")
	}
	s.Status = Pruned
	return nil
}
