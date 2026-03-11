package scheduler

import (
  "orchestrator/task"
  "orchestrator/node"
)

type Scheduler interface {
    SelectCandidateNodes(t *task.Task, nodes []*node.Node) []*node.Node
    BaseScore(t *task.Task, n *node.Node) float64
    Pick(scores map[string]float64,candidates []*node.Node) *node.Node
}

type RoundRobin struct {
    Name string
    last int
}

func (s *RoundRobin) SelectCandidateNodes(t *task.Task,nodes []*node.Node,) []*node.Node {
	candidates := []*node.Node{}
	for _, n := range nodes {
		stats := n.Stats
		if stats==nil {
			continue
		}
		// memory check
		memAvailable := stats.MemAvailableKb()
		if int64(memAvailable) < t.Memory {
			continue
		}
		// disk check
		diskAvailable := stats.DiskStats.Free
		if int64(diskAvailable) < t.Disk {
			continue
		}
		// cpu check (using load average)
		if float64(t.CPU) > stats.CpuAvailable() {
			continue
		}
		candidates = append(candidates, n)
	}
	return candidates
}

func (r *RoundRobin) BaseScore(t *task.Task, n *node.Node) float64 {
	stats := n.Stats
	if stats == nil {
		return 0
	}
	//MEMORY SCORE
	memTotal := float64(stats.MemStats.MemTotal)
	memAvail := float64(stats.MemStats.MemAvailable)

	memScore := memAvail / memTotal
	if memScore > 1 {
		memScore = 1
	}

	//DISK SCORE
	diskScore := stats.DiskFree() / stats.DiskTotal()
	if diskScore > 1 {
		diskScore = 1
	}

	//CPU SCORE
	cpuScore := 1 - stats.CpuUsage()

	if cpuScore < 0 {
		cpuScore = 0
	}
	if cpuScore > 1 {
		cpuScore = 1
	}

	//WEIGHTED BASE SCORE 
	cpuWeight := 0.5
	memWeight := 0.3
	diskWeight := 0.2

	baseScore := cpuWeight*cpuScore+memWeight*memScore +diskWeight*diskScore

	return baseScore
}

func(r *RoundRobin) Pick(scores map[string]float64,candidates []*node.Node,) *node.Node {
	var best *node.Node
	bestScore := -1.0
	for _, n := range candidates {
		score, ok := scores[n.ID]
		if !ok {
			continue
		}
		if score > bestScore {
			bestScore = score
			best = n
		}
	}
	return best
}
