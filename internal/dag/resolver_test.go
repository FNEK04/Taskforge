package dag_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/taskforge/internal"
	"github.com/taskforge/internal/dag"
)

func TestValidate_ValidDAG(t *testing.T) {
	r := dag.New()
	d := &internal.DAGDefinition{
		Nodes: []internal.DAGNode{
			{ID: "a", Type: "default"}, {ID: "b", Type: "default"}, {ID: "c", Type: "default"},
		},
		Edges: []internal.DAGEdge{
			{From: "a", To: "b"}, {From: "b", To: "c"},
		},
	}
	assert.NoError(t, r.Validate(d))
}

func TestValidate_EmptyNodes(t *testing.T) {
	r := dag.New()
	err := r.Validate(&internal.DAGDefinition{})
	assert.ErrorContains(t, err, "at least one node")
}

func TestValidate_EmptyNodeID(t *testing.T) {
	r := dag.New()
	err := r.Validate(&internal.DAGDefinition{
		Nodes: []internal.DAGNode{{ID: ""}},
	})
	assert.ErrorContains(t, err, "empty id")
}

func TestValidate_UnknownNodeInEdge(t *testing.T) {
	r := dag.New()
	err := r.Validate(&internal.DAGDefinition{
		Nodes: []internal.DAGNode{{ID: "a", Type: "default"}},
		Edges: []internal.DAGEdge{{From: "a", To: "missing"}},
	})
	assert.ErrorContains(t, err, "unknown node")
}

func TestValidate_Cycle(t *testing.T) {
	r := dag.New()
	err := r.Validate(&internal.DAGDefinition{
		Nodes: []internal.DAGNode{
			{ID: "a"}, {ID: "b"}, {ID: "c"},
		},
		Edges: []internal.DAGEdge{
			{From: "a", To: "b"}, {From: "b", To: "c"}, {From: "c", To: "a"},
		},
	})
	assert.ErrorContains(t, err, "cycle")
}

func TestValidate_SelfLoop(t *testing.T) {
	r := dag.New()
	err := r.Validate(&internal.DAGDefinition{
		Nodes: []internal.DAGNode{{ID: "a"}},
		Edges: []internal.DAGEdge{{From: "a", To: "a"}},
	})
	assert.ErrorContains(t, err, "cycle")
}

func TestDetectCycle_NoCycle(t *testing.T) {
	r := dag.New()
	assert.False(t, r.DetectCycle(&internal.DAGDefinition{
		Nodes: []internal.DAGNode{{ID: "a"}, {ID: "b"}},
		Edges: []internal.DAGEdge{{From: "a", To: "b"}},
	}))
}

func TestDetectCycle_HasCycle(t *testing.T) {
	r := dag.New()
	assert.True(t, r.DetectCycle(&internal.DAGDefinition{
		Nodes: []internal.DAGNode{{ID: "a"}, {ID: "b"}, {ID: "c"}},
		Edges: []internal.DAGEdge{{From: "a", To: "b"}, {From: "b", To: "c"}, {From: "c", To: "a"}},
	}))
}

func TestDetectCycle_Disconnected(t *testing.T) {
	r := dag.New()
	assert.False(t, r.DetectCycle(&internal.DAGDefinition{
		Nodes: []internal.DAGNode{{ID: "a"}, {ID: "b"}, {ID: "c"}},
	}))
}

func TestTopologicalSort_Linear(t *testing.T) {
	r := dag.New()
	order, err := r.TopologicalSort(&internal.DAGDefinition{
		Nodes: []internal.DAGNode{{ID: "a"}, {ID: "b"}, {ID: "c"}},
		Edges: []internal.DAGEdge{{From: "a", To: "b"}, {From: "b", To: "c"}},
	})
	assert.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, order)
}

func TestTopologicalSort_Diamond(t *testing.T) {
	r := dag.New()
	d := &internal.DAGDefinition{
		Nodes: []internal.DAGNode{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}},
		Edges: []internal.DAGEdge{
			{From: "a", To: "b"}, {From: "a", To: "c"}, {From: "b", To: "d"}, {From: "c", To: "d"},
		},
	}
	order, err := r.TopologicalSort(d)
	assert.NoError(t, err)
	assert.Len(t, order, 4)
	assert.Equal(t, "a", order[0])
	assert.Equal(t, "d", order[3])
}

func TestTopologicalSort_Cycle(t *testing.T) {
	r := dag.New()
	_, err := r.TopologicalSort(&internal.DAGDefinition{
		Nodes: []internal.DAGNode{{ID: "a"}, {ID: "b"}},
		Edges: []internal.DAGEdge{{From: "a", To: "b"}, {From: "b", To: "a"}},
	})
	assert.Error(t, err)
}

func TestReadyNodes_AllDepsMet(t *testing.T) {
	r := dag.New()
	nodes := []internal.DAGNode{{ID: "a"}, {ID: "b"}}
	ready := r.ReadyNodes(&internal.DAGDefinition{Nodes: nodes}, &internal.DAGRun{})
	assert.Len(t, ready, 2)
}

func TestReadyNodes_DepCompleted(t *testing.T) {
	r := dag.New()
	nodes := []internal.DAGNode{{ID: "a"}, {ID: "b"}}
	ready := r.ReadyNodes(&internal.DAGDefinition{
		Nodes: nodes,
		Edges: []internal.DAGEdge{{From: "a", To: "b"}},
	}, &internal.DAGRun{
		JobStatuses: map[string]internal.JobStatus{"a": internal.JobStatusCompleted},
	})
	assert.Len(t, ready, 1)
	assert.Equal(t, "b", ready[0].ID)
}

func TestReadyNodes_DepNotMet(t *testing.T) {
	r := dag.New()
	ready := r.ReadyNodes(&internal.DAGDefinition{
		Nodes: []internal.DAGNode{{ID: "a"}, {ID: "b"}},
		Edges: []internal.DAGEdge{{From: "a", To: "b"}},
	}, &internal.DAGRun{
		JobStatuses: map[string]internal.JobStatus{"a": internal.JobStatusRunning},
	})
	assert.Len(t, ready, 0)
}

func TestReadyNodes_NodeAlreadyCompleted(t *testing.T) {
	r := dag.New()
	ready := r.ReadyNodes(&internal.DAGDefinition{
		Nodes: []internal.DAGNode{{ID: "a"}},
	}, &internal.DAGRun{
		JobStatuses: map[string]internal.JobStatus{"a": internal.JobStatusCompleted},
	})
	assert.Len(t, ready, 0)
}

func TestIsDAGComplete_AllDone(t *testing.T) {
	r := dag.New()
	complete := r.IsDAGComplete(&internal.DAGDefinition{
		Nodes: []internal.DAGNode{{ID: "a"}, {ID: "b"}},
	}, &internal.DAGRun{
		JobStatuses: map[string]internal.JobStatus{
			"a": internal.JobStatusCompleted,
			"b": internal.JobStatusCompleted,
		},
	})
	assert.True(t, complete)
}

func TestIsDAGComplete_SomeRunning(t *testing.T) {
	r := dag.New()
	complete := r.IsDAGComplete(&internal.DAGDefinition{
		Nodes: []internal.DAGNode{{ID: "a"}, {ID: "b"}},
	}, &internal.DAGRun{
		JobStatuses: map[string]internal.JobStatus{
			"a": internal.JobStatusCompleted,
		},
	})
	assert.False(t, complete)
}

func TestIsDAGComplete_SomeFailed(t *testing.T) {
	r := dag.New()
	complete := r.IsDAGComplete(&internal.DAGDefinition{
		Nodes: []internal.DAGNode{{ID: "a"}, {ID: "b"}},
	}, &internal.DAGRun{
		JobStatuses: map[string]internal.JobStatus{
			"a": internal.JobStatusFailed,
			"b": internal.JobStatusCompleted,
		},
	})
	assert.True(t, complete)
}

func TestIsDAGSuccessful_AllSuccess(t *testing.T) {
	r := dag.New()
	ok := r.IsDAGSuccessful(&internal.DAGDefinition{
		Nodes: []internal.DAGNode{{ID: "a"}},
	}, &internal.DAGRun{
		JobStatuses: map[string]internal.JobStatus{"a": internal.JobStatusCompleted},
	})
	assert.True(t, ok)
}

func TestIsDAGSuccessful_OneFailed(t *testing.T) {
	r := dag.New()
	ok := r.IsDAGSuccessful(&internal.DAGDefinition{
		Nodes: []internal.DAGNode{{ID: "a"}},
	}, &internal.DAGRun{
		JobStatuses: map[string]internal.JobStatus{"a": internal.JobStatusFailed},
	})
	assert.False(t, ok)
}
