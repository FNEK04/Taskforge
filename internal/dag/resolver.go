package dag

import (
	"fmt"

	"github.com/taskforge/internal"
)

type Resolver struct{}

func New() *Resolver { return &Resolver{} }

func (r *Resolver) Validate(dag *internal.DAGDefinition) error {
	if len(dag.Nodes) == 0 {
		return fmt.Errorf("dag must have at least one node")
	}

	nodeSet := make(map[string]bool)
	for _, n := range dag.Nodes {
		if n.ID == "" {
			return fmt.Errorf("node has empty id")
		}
		nodeSet[n.ID] = true
	}

	for _, e := range dag.Edges {
		if !nodeSet[e.From] {
			return fmt.Errorf("edge references unknown node: %s", e.From)
		}
		if !nodeSet[e.To] {
			return fmt.Errorf("edge references unknown node: %s", e.To)
		}
	}

	cycle := r.DetectCycle(dag)
	if cycle {
		return fmt.Errorf("dag contains a cycle")
	}

	return nil
}

func (r *Resolver) DetectCycle(dag *internal.DAGDefinition) bool {
	adj := make(map[string][]string)
	for _, e := range dag.Edges {
		adj[e.From] = append(adj[e.From], e.To)
	}

	visited := make(map[string]int) // 0=unvisited, 1=visiting, 2=done
	var dfs func(node string) bool
	dfs = func(node string) bool {
		if visited[node] == 1 {
			return true
		}
		if visited[node] == 2 {
			return false
		}
		visited[node] = 1
		for _, next := range adj[node] {
			if dfs(next) {
				return true
			}
		}
		visited[node] = 2
		return false
	}

	for _, n := range dag.Nodes {
		if dfs(n.ID) {
			return true
		}
	}
	return false
}

func (r *Resolver) TopologicalSort(dag *internal.DAGDefinition) ([]string, error) {
	inDegree := make(map[string]int)
	adj := make(map[string][]string)
	nodeSet := make(map[string]bool)

	for _, n := range dag.Nodes {
		inDegree[n.ID] = 0
		nodeSet[n.ID] = true
	}
	for _, e := range dag.Edges {
		adj[e.From] = append(adj[e.From], e.To)
		inDegree[e.To]++
	}

	var queue []string
	for _, n := range dag.Nodes {
		if inDegree[n.ID] == 0 {
			queue = append(queue, n.ID)
		}
	}

	var order []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)
		for _, next := range adj[node] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	if len(order) != len(dag.Nodes) {
		return nil, fmt.Errorf("dag contains a cycle or disconnected nodes")
	}
	return order, nil
}

func (r *Resolver) ReadyNodes(dag *internal.DAGDefinition, run *internal.DAGRun) []internal.DAGNode {
	if run.JobStatuses == nil {
		run.JobStatuses = make(map[string]internal.JobStatus)
	}

	depMap := make(map[string][]string)
	nodeMap := make(map[string]internal.DAGNode)
	for _, e := range dag.Edges {
		depMap[e.To] = append(depMap[e.To], e.From)
	}
	for _, n := range dag.Nodes {
		nodeMap[n.ID] = n
	}

	var ready []internal.DAGNode
	for _, n := range dag.Nodes {
		status, exists := run.JobStatuses[n.ID]
		if exists && status != internal.JobStatusFailed {
			continue
		}
		if exists && status == internal.JobStatusFailed {
			continue
		}

		deps := depMap[n.ID]
		allDone := true
		for _, dep := range deps {
			depStatus, ok := run.JobStatuses[dep]
			if !ok || depStatus != internal.JobStatusCompleted {
				allDone = false
				break
			}
		}
		if allDone {
			ready = append(ready, n)
		}
	}
	return ready
}

func (r *Resolver) IsDAGComplete(dag *internal.DAGDefinition, run *internal.DAGRun) bool {
	for _, n := range dag.Nodes {
		status, ok := run.JobStatuses[n.ID]
		if !ok || (status != internal.JobStatusCompleted && status != internal.JobStatusFailed) {
			return false
		}
	}
	return true
}

func (r *Resolver) IsDAGSuccessful(dag *internal.DAGDefinition, run *internal.DAGRun) bool {
	for _, n := range dag.Nodes {
		status, ok := run.JobStatuses[n.ID]
		if !ok || status != internal.JobStatusCompleted {
			return false
		}
	}
	return true
}
