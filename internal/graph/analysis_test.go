// internal/graph/analysis_test.go
package graph

import "testing"

func TestDetectLoopsNone(t *testing.T) {
	g := New()
	g.AddNode("a", NodeTypeDevice, nil)
	g.AddNode("b", NodeTypeDevice, nil)
	g.AddEdge("a", "b", EdgePeer, nil)

	loops := DetectLoops(g)
	if len(loops) != 0 {
		t.Errorf("expected no loops, got %d", len(loops))
	}
}

func TestDetectLoopsFound(t *testing.T) {
	g := New()
	g.AddNode("a", NodeTypeDevice, nil)
	g.AddNode("b", NodeTypeDevice, nil)
	g.AddNode("c", NodeTypeDevice, nil)
	g.AddEdge("a", "b", EdgePeer, nil)
	g.AddEdge("b", "c", EdgePeer, nil)
	g.AddEdge("c", "a", EdgePeer, nil) // cycle

	loops := DetectLoops(g)
	if len(loops) == 0 {
		t.Error("expected to find a loop")
	}
}

func TestFindSPOF(t *testing.T) {
	// Linear: A -- B -- C
	// B is a single point of failure
	g := New()
	g.AddNode("a", NodeTypeDevice, nil)
	g.AddNode("b", NodeTypeDevice, nil)
	g.AddNode("c", NodeTypeDevice, nil)
	g.AddBidirectionalEdge("a", "b", EdgePeer, nil)
	g.AddBidirectionalEdge("b", "c", EdgePeer, nil)

	spofs := FindSPOF(g, NodeTypeDevice)
	found := false
	for _, s := range spofs {
		if s == "b" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'b' as SPOF, got %v", spofs)
	}
}

func TestFindSPOFNone(t *testing.T) {
	// Triangle: A -- B -- C -- A (fully connected, no SPOF)
	g := New()
	g.AddNode("a", NodeTypeDevice, nil)
	g.AddNode("b", NodeTypeDevice, nil)
	g.AddNode("c", NodeTypeDevice, nil)
	g.AddBidirectionalEdge("a", "b", EdgePeer, nil)
	g.AddBidirectionalEdge("b", "c", EdgePeer, nil)
	g.AddBidirectionalEdge("c", "a", EdgePeer, nil)

	spofs := FindSPOF(g, NodeTypeDevice)
	if len(spofs) != 0 {
		t.Errorf("expected no SPOFs in triangle, got %v", spofs)
	}
}

func TestImpactAnalysis(t *testing.T) {
	// A -- B -- C, A -- B -- D
	// Removing B isolates C and D from A
	g := New()
	g.AddNode("a", NodeTypeDevice, nil)
	g.AddNode("b", NodeTypeDevice, nil)
	g.AddNode("c", NodeTypeDevice, nil)
	g.AddNode("d", NodeTypeDevice, nil)
	g.AddBidirectionalEdge("a", "b", EdgePeer, nil)
	g.AddBidirectionalEdge("b", "c", EdgePeer, nil)
	g.AddBidirectionalEdge("b", "d", EdgePeer, nil)

	affected := ImpactAnalysis(g, "b", NodeTypeDevice)
	// Removing b should affect c and d (they lose connectivity)
	if len(affected) < 2 {
		t.Errorf("expected >=2 affected, got %v", affected)
	}
}
