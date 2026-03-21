// internal/graph/graph_test.go
package graph

import "testing"

func TestAddAndGetNode(t *testing.T) {
	g := New()
	g.AddNode("dev1", NodeTypeDevice, map[string]string{"hostname": "Core-01"})

	n, ok := g.GetNode("dev1")
	if !ok { t.Fatal("node not found") }
	if n.Type != NodeTypeDevice { t.Errorf("type: %s", n.Type) }
	if n.Props["hostname"] != "Core-01" { t.Errorf("hostname: %s", n.Props["hostname"]) }
}

func TestAddEdgeAndNeighbors(t *testing.T) {
	g := New()
	g.AddNode("dev1", NodeTypeDevice, nil)
	g.AddNode("iface1", NodeTypeInterface, nil)
	g.AddEdge("dev1", "iface1", EdgeHasInterface, nil)

	neighbors := g.Neighbors("dev1")
	if len(neighbors) != 1 { t.Fatalf("expected 1, got %d", len(neighbors)) }
	if neighbors[0].To != "iface1" { t.Errorf("to: %s", neighbors[0].To) }
	if neighbors[0].Type != EdgeHasInterface { t.Errorf("type: %s", neighbors[0].Type) }
}

func TestBidirectionalEdge(t *testing.T) {
	g := New()
	g.AddNode("a", NodeTypeInterface, nil)
	g.AddNode("b", NodeTypeInterface, nil)
	g.AddBidirectionalEdge("a", "b", EdgeConnectsTo, nil)

	na := g.Neighbors("a")
	nb := g.Neighbors("b")
	if len(na) != 1 || na[0].To != "b" { t.Error("a should connect to b") }
	if len(nb) != 1 || nb[0].To != "a" { t.Error("b should connect to a") }
}

func TestNodesByType(t *testing.T) {
	g := New()
	g.AddNode("dev1", NodeTypeDevice, nil)
	g.AddNode("dev2", NodeTypeDevice, nil)
	g.AddNode("iface1", NodeTypeInterface, nil)

	devices := g.NodesByType(NodeTypeDevice)
	if len(devices) != 2 { t.Errorf("expected 2 devices, got %d", len(devices)) }
}

func TestEdgesBetween(t *testing.T) {
	g := New()
	g.AddNode("a", NodeTypeDevice, nil)
	g.AddNode("b", NodeTypeDevice, nil)
	g.AddEdge("a", "b", EdgePeer, map[string]string{"protocol": "ospf"})
	g.AddEdge("a", "b", EdgePeer, map[string]string{"protocol": "ldp"})

	edges := g.EdgesBetween("a", "b")
	if len(edges) != 2 { t.Errorf("expected 2 edges, got %d", len(edges)) }
}

func TestNodeCount(t *testing.T) {
	g := New()
	g.AddNode("a", NodeTypeDevice, nil)
	g.AddNode("b", NodeTypeDevice, nil)
	if g.NodeCount() != 2 { t.Errorf("expected 2, got %d", g.NodeCount()) }
	if g.EdgeCount() != 0 { t.Errorf("expected 0, got %d", g.EdgeCount()) }
}
