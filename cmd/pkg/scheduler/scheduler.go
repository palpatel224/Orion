package scheduler

import (
"orchestrator/task"
"orchestrator/node"
)

type Scheduler interface {
SelectCandidateNodes(t *task.Task, nodes []*node.Node) []*node.Node
Score(t *task.Task, nodes []*node.Node) map[string]float64
Pick(scores map[string]float64, candidates []*node.Node) *node.Node
}

type RoundRobin struct {
Name string
last int
}

func (r *RoundRobin) SelectCandidateNodes(t *task.Task, nodes []*node.Node) []*node.Node {
// For round robin, all nodes are candidates
return nodes
}

func (r *RoundRobin) Score(t *task.Task, nodes []*node.Node) map[string]float64 {
// For round robin, all nodes get equal score
scores := make(map[string]float64)
for _, n := range nodes {
scores[n.ID] = 1.0
}
return scores
}

func (r *RoundRobin) Pick(scores map[string]float64, candidates []*node.Node) *node.Node {
if len(candidates) == 0 {
return nil
}

// Simple round robin selection
r.last = (r.last + 1) % len(candidates)
return candidates[r.last]
}
