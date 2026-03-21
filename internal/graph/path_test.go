// internal/graph/path_test.go
package graph

import "testing"

func buildTestGraph() *Graph {
	g := New()
	// A -- B -- C -- D
	//      \       /
	//       E ----/
	g.AddNode("a", NodeTypeDevice, nil)
	g.AddNode("b", NodeTypeDevice, nil)
	g.AddNode("c", NodeTypeDevice, nil)
	g.AddNode("d", NodeTypeDevice, nil)
	g.AddNode("e", NodeTypeDevice, nil)

	g.AddBidirectionalEdge("a", "b", EdgePeer, nil)
	g.AddBidirectionalEdge("b", "c", EdgePeer, nil)
	g.AddBidirectionalEdge("c", "d", EdgePeer, nil)
	g.AddBidirectionalEdge("b", "e", EdgePeer, nil)
	g.AddBidirectionalEdge("e", "d", EdgePeer, nil)

	return g
}

func TestShortestPath(t *testing.T) {
	g := buildTestGraph()

	path, err := ShortestPath(g, "a", "d")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// A→B→C→D or A→B→E→D (both length 3)
	if len(path) != 4 {
		t.Fatalf("expected 4 hops, got %d: %v", len(path), path)
	}
	if path[0] != "a" || path[len(path)-1] != "d" {
		t.Errorf("path: %v", path)
	}
}

func TestShortestPathSameNode(t *testing.T) {
	g := buildTestGraph()
	path, err := ShortestPath(g, "a", "a")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(path) != 1 || path[0] != "a" {
		t.Errorf("expected [a], got %v", path)
	}
}

func TestShortestPathUnreachable(t *testing.T) {
	g := buildTestGraph()
	g.AddNode("isolated", NodeTypeDevice, nil)
	_, err := ShortestPath(g, "a", "isolated")
	if err == nil {
		t.Error("expected error for unreachable node")
	}
}

func TestAllPaths(t *testing.T) {
	g := buildTestGraph()
	paths := AllPaths(g, "a", "d", 5)
	// Should find at least 2 paths: A→B→C→D and A→B→E→D
	if len(paths) < 2 {
		t.Errorf("expected >=2 paths, got %d", len(paths))
	}
}

func TestShortestPathDeviceLevel(t *testing.T) {
	g := buildTestGraph()
	path, err := ShortestPathByNodeType(g, "a", "d", NodeTypeDevice)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(path) < 2 {
		t.Error("path too short")
	}
}
