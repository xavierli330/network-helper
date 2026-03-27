# Nethelper Plan 4: In-Memory Graph Engine + Path Analysis

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the in-memory graph engine that loads topology from SQLite on startup, and implement graph analysis commands: `show topology`, `trace path`, `trace impact`, `check loop`, and `check spof`.

**Architecture:** The graph package defines a generic directed graph with typed nodes and edges. On startup (or after ingest), the graph is populated from SQLite data (devices, interfaces, neighbors, routes). The graph supports BFS/DFS traversal, shortest path, and connectivity analysis. CLI commands in `trace.go` and `check.go` query the graph. The graph is rebuilt from DB on each CLI invocation (stateless per command) — no long-lived in-memory state to synchronize.

**Tech Stack:** Go 1.24+, pure Go graph implementation (no external libs), existing internal/store and internal/model packages

**Spec:** `docs/superpowers/specs/2026-03-21-network-helper-design.md` (Section 3.6)

**Depends on:** Plan 1 (store), Plan 2 (parser pipeline), Plan 3 (multi-vendor parsing)

---

## File Structure

```
internal/
├── graph/
│   ├── graph.go                   # Core graph: Node, Edge, Graph struct, Add/Get/Neighbors
│   ├── graph_test.go              # Unit tests for core graph operations
│   ├── builder.go                 # BuildFromDB: loads SQLite data into graph
│   ├── builder_test.go            # Tests for graph building
│   ├── path.go                    # ShortestPath (BFS), AllPaths
│   ├── path_test.go               # Path algorithm tests
│   ├── analysis.go                # DetectLoops, FindSPOF (single points of failure)
│   └── analysis_test.go           # Analysis tests
├── cli/
│   ├── show.go                    # Modify: add "show topology" subcommand
│   ├── trace.go                   # New: trace path/impact commands
│   └── check.go                   # New: check loop/spof commands
```

---

### Task 1: Core Graph Data Structure

**Files:**
- Create: `internal/graph/graph.go`
- Create: `internal/graph/graph_test.go`

- [ ] **Step 1: Write test**

```go
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
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/graph/ -v`
Expected: FAIL

- [ ] **Step 3: Implement graph.go**

```go
// internal/graph/graph.go
package graph

// Node types matching the spec
type NodeType string

const (
	NodeTypeDevice          NodeType = "device"
	NodeTypeInterface       NodeType = "interface"
	NodeTypeSubnet          NodeType = "subnet"
	NodeTypeVLAN            NodeType = "vlan"
	NodeTypeVRF             NodeType = "vrf"
	NodeTypeRoutingInstance NodeType = "routing_instance"
	NodeTypeLSP             NodeType = "lsp"
	NodeTypeSRPolicy        NodeType = "sr_policy"
)

// Edge types matching the spec
type EdgeType string

const (
	EdgeHasInterface  EdgeType = "HAS_INTERFACE"
	EdgeMemberOf      EdgeType = "MEMBER_OF"
	EdgeConnectsTo    EdgeType = "CONNECTS_TO"
	EdgeInSubnet      EdgeType = "IN_SUBNET"
	EdgeInVRF         EdgeType = "IN_VRF"
	EdgePeer          EdgeType = "PEER"
	EdgeRoutesVia     EdgeType = "ROUTES_VIA"
	EdgeLSPHop        EdgeType = "LSP_HOP"
	EdgeLabelBind     EdgeType = "LABEL_BIND"
	EdgeTunnelsThrough EdgeType = "TUNNELS_THROUGH"
)

type Node struct {
	ID    string
	Type  NodeType
	Props map[string]string
}

type Edge struct {
	From  string
	To    string
	Type  EdgeType
	Props map[string]string
}

// Graph is a directed multigraph with typed nodes and edges.
type Graph struct {
	nodes map[string]*Node
	edges map[string][]Edge // adjacency list: from → []Edge
}

func New() *Graph {
	return &Graph{
		nodes: make(map[string]*Node),
		edges: make(map[string][]Edge),
	}
}

func (g *Graph) AddNode(id string, nodeType NodeType, props map[string]string) {
	if props == nil { props = make(map[string]string) }
	g.nodes[id] = &Node{ID: id, Type: nodeType, Props: props}
}

func (g *Graph) GetNode(id string) (*Node, bool) {
	n, ok := g.nodes[id]
	return n, ok
}

func (g *Graph) AddEdge(from, to string, edgeType EdgeType, props map[string]string) {
	if props == nil { props = make(map[string]string) }
	g.edges[from] = append(g.edges[from], Edge{From: from, To: to, Type: edgeType, Props: props})
}

func (g *Graph) AddBidirectionalEdge(a, b string, edgeType EdgeType, props map[string]string) {
	g.AddEdge(a, b, edgeType, props)
	g.AddEdge(b, a, edgeType, props)
}

func (g *Graph) Neighbors(id string) []Edge {
	return g.edges[id]
}

// NeighborsByType returns edges of a specific type from a node.
func (g *Graph) NeighborsByType(id string, edgeType EdgeType) []Edge {
	var result []Edge
	for _, e := range g.edges[id] {
		if e.Type == edgeType { result = append(result, e) }
	}
	return result
}

func (g *Graph) EdgesBetween(from, to string) []Edge {
	var result []Edge
	for _, e := range g.edges[from] {
		if e.To == to { result = append(result, e) }
	}
	return result
}

func (g *Graph) NodesByType(nodeType NodeType) []*Node {
	var result []*Node
	for _, n := range g.nodes {
		if n.Type == nodeType { result = append(result, n) }
	}
	return result
}

func (g *Graph) NodeCount() int { return len(g.nodes) }

func (g *Graph) EdgeCount() int {
	count := 0
	for _, edges := range g.edges { count += len(edges) }
	return count
}

// AllNodeIDs returns all node IDs in the graph.
func (g *Graph) AllNodeIDs() []string {
	ids := make([]string, 0, len(g.nodes))
	for id := range g.nodes { ids = append(ids, id) }
	return ids
}
```

- [ ] **Step 4: Run test — verify PASS**

Run: `go test ./internal/graph/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/graph/graph.go internal/graph/graph_test.go
git commit -m "feat: add core graph data structure with typed nodes and edges"
```

---

### Task 2: Graph Builder — Load from SQLite

**Files:**
- Create: `internal/graph/builder.go`
- Create: `internal/graph/builder_test.go`

- [ ] **Step 1: Write test**

```go
// internal/graph/builder_test.go
package graph

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/store"
)

func seedTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil { t.Fatalf("open: %v", err) }
	t.Cleanup(func() { db.Close() })

	// Two devices
	db.UpsertDevice(model.Device{ID: "core-01", Hostname: "Core-01", Vendor: "huawei", RouterID: "1.1.1.1", LastSeen: time.Now()})
	db.UpsertDevice(model.Device{ID: "pe-01", Hostname: "PE-01", Vendor: "cisco", RouterID: "2.2.2.2", LastSeen: time.Now()})

	// Interfaces
	db.UpsertInterface(model.Interface{ID: "core-01:GE0/0/1", DeviceID: "core-01", Name: "GE0/0/1", Type: model.IfTypePhysical, Status: "up", IPAddress: "10.0.0.1", Mask: "30", LastUpdated: time.Now()})
	db.UpsertInterface(model.Interface{ID: "core-01:Lo0", DeviceID: "core-01", Name: "Lo0", Type: model.IfTypeLoopback, Status: "up", IPAddress: "1.1.1.1", LastUpdated: time.Now()})
	db.UpsertInterface(model.Interface{ID: "pe-01:Gi0/0", DeviceID: "pe-01", Name: "Gi0/0", Type: model.IfTypePhysical, Status: "up", IPAddress: "10.0.0.2", Mask: "30", LastUpdated: time.Now()})

	// Snapshot + Neighbors (OSPF peer)
	snapID1, _ := db.CreateSnapshot(model.Snapshot{DeviceID: "core-01", SourceFile: "test"})
	db.InsertNeighbors([]model.NeighborInfo{
		{DeviceID: "core-01", Protocol: "ospf", RemoteID: "2.2.2.2", LocalInterface: "GE0/0/1", State: "full", SnapshotID: snapID1},
	})

	// RIB
	db.InsertRIBEntries([]model.RIBEntry{
		{DeviceID: "core-01", Prefix: "10.0.0.0", MaskLen: 30, Protocol: "direct", NextHop: "", OutgoingInterface: "GE0/0/1", SnapshotID: snapID1},
		{DeviceID: "core-01", Prefix: "2.2.2.2", MaskLen: 32, Protocol: "ospf", NextHop: "10.0.0.2", OutgoingInterface: "GE0/0/1", SnapshotID: snapID1},
	})

	return db
}

func TestBuildFromDB(t *testing.T) {
	db := seedTestDB(t)
	g, err := BuildFromDB(db)
	if err != nil { t.Fatalf("build: %v", err) }

	// Should have device nodes
	if _, ok := g.GetNode("core-01"); !ok { t.Error("core-01 not found") }
	if _, ok := g.GetNode("pe-01"); !ok { t.Error("pe-01 not found") }

	// Should have interface nodes
	if _, ok := g.GetNode("core-01:GE0/0/1"); !ok { t.Error("interface not found") }

	// Should have HAS_INTERFACE edges
	edges := g.NeighborsByType("core-01", EdgeHasInterface)
	if len(edges) < 1 { t.Error("expected HAS_INTERFACE edges from core-01") }

	// Should have PEER edges from neighbor data
	peers := g.NeighborsByType("core-01", EdgePeer)
	if len(peers) < 1 { t.Error("expected PEER edge from core-01") }

	// Should have device count >= 2
	devices := g.NodesByType(NodeTypeDevice)
	if len(devices) < 2 { t.Errorf("expected >=2 devices, got %d", len(devices)) }
}

func TestBuildSubnets(t *testing.T) {
	// Create a fresh DB with two interfaces on the same /30
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil { t.Fatalf("open: %v", err) }
	defer db.Close()

	db.UpsertDevice(model.Device{ID: "r1", Hostname: "R1", Vendor: "huawei", LastSeen: time.Now()})
	db.UpsertDevice(model.Device{ID: "r2", Hostname: "R2", Vendor: "cisco", LastSeen: time.Now()})
	// 10.0.0.1/30 and 10.0.0.2/30 → both in subnet 10.0.0.0/30
	db.UpsertInterface(model.Interface{ID: "r1:ge1", DeviceID: "r1", Name: "GE0/0/1", Type: model.IfTypePhysical, Status: "up", IPAddress: "10.0.0.1", Mask: "30", LastUpdated: time.Now()})
	db.UpsertInterface(model.Interface{ID: "r2:gi0", DeviceID: "r2", Name: "Gi0/0", Type: model.IfTypePhysical, Status: "up", IPAddress: "10.0.0.2", Mask: "30", LastUpdated: time.Now()})

	g, _ := BuildFromDB(db)

	// Both interfaces should point to the same subnet node "10.0.0.0/30"
	subnets := g.NodesByType(NodeTypeSubnet)
	if len(subnets) != 1 { t.Fatalf("expected 1 subnet, got %d", len(subnets)) }
	if subnets[0].ID != "10.0.0.0/30" { t.Errorf("expected 10.0.0.0/30, got %s", subnets[0].ID) }

	// Should have CONNECTS_TO edge between the two interfaces
	edges := g.EdgesBetween("r1:ge1", "r2:gi0")
	connectsFound := false
	for _, e := range edges {
		if e.Type == EdgeConnectsTo { connectsFound = true }
	}
	if !connectsFound { t.Error("expected CONNECTS_TO edge between interfaces on same /30 subnet") }
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/graph/ -v -run TestBuild`
Expected: FAIL

- [ ] **Step 3: Implement builder.go**

```go
// internal/graph/builder.go
package graph

import (
	"fmt"
	"strings"

	"github.com/xavierli/nethelper/internal/store"
)

// BuildFromDB loads all data from the store and constructs the in-memory graph.
func BuildFromDB(db *store.DB) (*Graph, error) {
	g := New()

	// 1. Load devices as nodes
	devices, err := db.ListDevices()
	if err != nil { return nil, fmt.Errorf("list devices: %w", err) }

	for _, d := range devices {
		g.AddNode(d.ID, NodeTypeDevice, map[string]string{
			"hostname":  d.Hostname,
			"vendor":    d.Vendor,
			"router_id": d.RouterID,
			"mgmt_ip":   d.MgmtIP,
		})
	}

	// 2. Load interfaces and create HAS_INTERFACE + MEMBER_OF edges
	for _, d := range devices {
		ifaces, err := db.GetInterfaces(d.ID)
		if err != nil { continue }

		for _, iface := range ifaces {
			nodeID := iface.ID
			if nodeID == "" { nodeID = d.ID + ":" + iface.Name }
			g.AddNode(nodeID, NodeTypeInterface, map[string]string{
				"name":      iface.Name,
				"device_id": d.ID,
				"type":      string(iface.Type),
				"status":    iface.Status,
				"ip":        iface.IPAddress,
				"mask":      iface.Mask,
			})
			g.AddEdge(d.ID, nodeID, EdgeHasInterface, nil)

			// MEMBER_OF for trunk members
			if iface.ParentID != "" {
				g.AddEdge(nodeID, iface.ParentID, EdgeMemberOf, nil)
			}

			// Create subnet nodes from IP/mask
			if iface.IPAddress != "" && iface.Mask != "" {
				subnetID := computeSubnetID(iface.IPAddress, iface.Mask)
				if subnetID != "" {
					if _, ok := g.GetNode(subnetID); !ok {
						g.AddNode(subnetID, NodeTypeSubnet, map[string]string{"prefix": subnetID})
					}
					g.AddEdge(nodeID, subnetID, EdgeInSubnet, nil)
				}
			}
		}
	}

	// 3. Build CONNECTS_TO edges — interfaces in the same subnet are connected
	subnets := g.NodesByType(NodeTypeSubnet)
	for _, subnet := range subnets {
		// Find all interfaces pointing to this subnet
		var ifaceIDs []string
		for _, d := range devices {
			for _, e := range g.NeighborsByType(d.ID, EdgeHasInterface) {
				for _, se := range g.NeighborsByType(e.To, EdgeInSubnet) {
					if se.To == subnet.ID { ifaceIDs = append(ifaceIDs, e.To) }
				}
			}
		}
		// Connect each pair
		for i := 0; i < len(ifaceIDs); i++ {
			for j := i + 1; j < len(ifaceIDs); j++ {
				g.AddBidirectionalEdge(ifaceIDs[i], ifaceIDs[j], EdgeConnectsTo, nil)
			}
		}
	}

	// 4. Build PEER edges from protocol neighbors (latest snapshot per device)
	for _, d := range devices {
		snapID, err := db.LatestSnapshotID(d.ID)
		if err != nil { continue }

		neighbors, err := db.GetNeighbors(d.ID, snapID)
		if err != nil { continue }

		for _, n := range neighbors {
			// Try to find the remote device by router-ID or remote-ID
			remoteDevID := findDeviceByRouterID(g, n.RemoteID)
			if remoteDevID == "" { remoteDevID = findDeviceByRouterID(g, n.RemoteAddress) }
			if remoteDevID == "" { continue }

			g.AddEdge(d.ID, remoteDevID, EdgePeer, map[string]string{
				"protocol":  n.Protocol,
				"state":     n.State,
				"local_if":  n.LocalInterface,
				"remote_id": n.RemoteID,
			})
		}
	}

	return g, nil
}

// findDeviceByRouterID searches for a device node whose router_id matches.
func findDeviceByRouterID(g *Graph, routerID string) string {
	if routerID == "" { return "" }
	for _, n := range g.NodesByType(NodeTypeDevice) {
		if n.Props["router_id"] == routerID { return n.ID }
	}
	// Also try matching by device ID (lowercase hostname)
	if _, ok := g.GetNode(strings.ToLower(routerID)); ok {
		return strings.ToLower(routerID)
	}
	return ""
}

// computeSubnetID computes the network address from IP and mask length.
// E.g., "10.0.0.1" + "30" → "252.227.81.4/30"
func computeSubnetID(ip, mask string) string {
	if ip == "" || mask == "" { return "" }
	parts := strings.Split(ip, ".")
	if len(parts) != 4 { return "" }

	var ipBytes [4]byte
	for i, p := range parts {
		n := 0
		for _, ch := range p {
			n = n*10 + int(ch-'0')
		}
		ipBytes[i] = byte(n)
	}

	maskLen := 0
	for _, ch := range mask {
		maskLen = maskLen*10 + int(ch-'0')
	}
	if maskLen < 0 || maskLen > 32 { return "" }

	// Compute network address by masking
	var maskBytes [4]byte
	for i := 0; i < 4; i++ {
		if maskLen >= 8 {
			maskBytes[i] = 255
			maskLen -= 8
		} else {
			maskBytes[i] = byte(0xFF << (8 - maskLen))
			maskLen = 0
		}
	}

	netIP := fmt.Sprintf("%d.%d.%d.%d",
		ipBytes[0]&maskBytes[0], ipBytes[1]&maskBytes[1],
		ipBytes[2]&maskBytes[2], ipBytes[3]&maskBytes[3])
	return netIP + "/" + mask
}
```

- [ ] **Step 4: Run test — verify PASS**

Run: `go test ./internal/graph/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/graph/builder.go internal/graph/builder_test.go
git commit -m "feat: add graph builder that loads topology from SQLite"
```

---

### Task 3: Path Algorithms — BFS Shortest Path

**Files:**
- Create: `internal/graph/path.go`
- Create: `internal/graph/path_test.go`

- [ ] **Step 1: Write test**

```go
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
	if err != nil { t.Fatalf("error: %v", err) }
	// A→B→C→D or A→B→E→D (both length 3)
	if len(path) != 4 { t.Fatalf("expected 4 hops, got %d: %v", len(path), path) }
	if path[0] != "a" || path[len(path)-1] != "d" { t.Errorf("path: %v", path) }
}

func TestShortestPathSameNode(t *testing.T) {
	g := buildTestGraph()
	path, err := ShortestPath(g, "a", "a")
	if err != nil { t.Fatalf("error: %v", err) }
	if len(path) != 1 || path[0] != "a" { t.Errorf("expected [a], got %v", path) }
}

func TestShortestPathUnreachable(t *testing.T) {
	g := buildTestGraph()
	g.AddNode("isolated", NodeTypeDevice, nil)
	_, err := ShortestPath(g, "a", "isolated")
	if err == nil { t.Error("expected error for unreachable node") }
}

func TestAllPaths(t *testing.T) {
	g := buildTestGraph()
	paths := AllPaths(g, "a", "d", 5)
	// Should find at least 2 paths: A→B→C→D and A→B→E→D
	if len(paths) < 2 { t.Errorf("expected >=2 paths, got %d", len(paths)) }
}

func TestShortestPathDeviceLevel(t *testing.T) {
	g := buildTestGraph()
	path, err := ShortestPathByNodeType(g, "a", "d", NodeTypeDevice)
	if err != nil { t.Fatalf("error: %v", err) }
	if len(path) < 2 { t.Error("path too short") }
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/graph/ -v -run TestShortest`
Expected: FAIL

- [ ] **Step 3: Implement path.go**

```go
// internal/graph/path.go
package graph

import "fmt"

// ShortestPath finds the shortest path between two nodes using BFS.
// Returns the ordered list of node IDs from src to dst.
func ShortestPath(g *Graph, src, dst string) ([]string, error) {
	if src == dst { return []string{src}, nil }
	if _, ok := g.GetNode(src); !ok { return nil, fmt.Errorf("source node %q not found", src) }
	if _, ok := g.GetNode(dst); !ok { return nil, fmt.Errorf("destination node %q not found", dst) }

	visited := map[string]bool{src: true}
	parent := map[string]string{}
	queue := []string{src}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, edge := range g.Neighbors(current) {
			if visited[edge.To] { continue }
			visited[edge.To] = true
			parent[edge.To] = current

			if edge.To == dst {
				return reconstructPath(parent, src, dst), nil
			}
			queue = append(queue, edge.To)
		}
	}

	return nil, fmt.Errorf("no path from %q to %q", src, dst)
}

// ShortestPathByNodeType finds shortest path only traversing nodes of a given type.
func ShortestPathByNodeType(g *Graph, src, dst string, nodeType NodeType) ([]string, error) {
	if src == dst { return []string{src}, nil }
	if _, ok := g.GetNode(src); !ok { return nil, fmt.Errorf("source node %q not found", src) }
	if _, ok := g.GetNode(dst); !ok { return nil, fmt.Errorf("destination node %q not found", dst) }

	visited := map[string]bool{src: true}
	parent := map[string]string{}
	queue := []string{src}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, edge := range g.Neighbors(current) {
			if visited[edge.To] { continue }
			n, ok := g.GetNode(edge.To)
			if !ok { continue }
			if n.Type != nodeType { continue }
			visited[edge.To] = true
			parent[edge.To] = current

			if edge.To == dst {
				return reconstructPath(parent, src, dst), nil
			}
			queue = append(queue, edge.To)
		}
	}

	return nil, fmt.Errorf("no path from %q to %q (type=%s)", src, dst, nodeType)
}

// AllPaths finds all paths from src to dst up to maxDepth using DFS.
func AllPaths(g *Graph, src, dst string, maxDepth int) [][]string {
	var results [][]string
	visited := map[string]bool{}
	var dfs func(current string, path []string)

	dfs = func(current string, path []string) {
		if len(path) > maxDepth { return }
		if current == dst {
			cp := make([]string, len(path))
			copy(cp, path)
			results = append(results, cp)
			return
		}

		visited[current] = true
		for _, edge := range g.Neighbors(current) {
			if !visited[edge.To] {
				dfs(edge.To, append(path, edge.To))
			}
		}
		visited[current] = false
	}

	dfs(src, []string{src})
	return results
}

func reconstructPath(parent map[string]string, src, dst string) []string {
	var path []string
	for current := dst; current != src; current = parent[current] {
		path = append([]string{current}, path...)
	}
	return append([]string{src}, path...)
}
```

- [ ] **Step 4: Run test — verify PASS**

Run: `go test ./internal/graph/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/graph/path.go internal/graph/path_test.go
git commit -m "feat: add BFS shortest path and DFS all-paths algorithms"
```

---

### Task 4: Graph Analysis — Loop Detection + SPOF

**Files:**
- Create: `internal/graph/analysis.go`
- Create: `internal/graph/analysis_test.go`

- [ ] **Step 1: Write test**

```go
// internal/graph/analysis_test.go
package graph

import "testing"

func TestDetectLoopsNone(t *testing.T) {
	g := New()
	g.AddNode("a", NodeTypeDevice, nil)
	g.AddNode("b", NodeTypeDevice, nil)
	g.AddEdge("a", "b", EdgePeer, nil)

	loops := DetectLoops(g)
	if len(loops) != 0 { t.Errorf("expected no loops, got %d", len(loops)) }
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
	if len(loops) == 0 { t.Error("expected to find a loop") }
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
		if s == "b" { found = true }
	}
	if !found { t.Errorf("expected 'b' as SPOF, got %v", spofs) }
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
	if len(spofs) != 0 { t.Errorf("expected no SPOFs in triangle, got %v", spofs) }
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
	if len(affected) < 2 { t.Errorf("expected >=2 affected, got %v", affected) }
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/graph/ -v -run "TestDetect|TestFindSPOF|TestImpact"`
Expected: FAIL

- [ ] **Step 3: Implement analysis.go**

```go
// internal/graph/analysis.go
package graph

// DetectLoops finds directed cycles in the graph using DFS coloring.
// Returns a list of cycles, each represented as a slice of node IDs.
func DetectLoops(g *Graph) [][]string {
	var loops [][]string
	white := 0 // unvisited
	gray := 1  // in current DFS path
	black := 2 // fully processed

	color := make(map[string]int)
	parent := make(map[string]string)
	path := make(map[string][]string) // track path to each node

	for _, id := range g.AllNodeIDs() { color[id] = white }

	var dfs func(u string)
	dfs = func(u string) {
		color[u] = gray
		for _, edge := range g.Neighbors(u) {
			v := edge.To
			if color[v] == gray {
				// Back edge found — extract cycle
				cycle := []string{v, u}
				for cur := u; cur != v; {
					cur = parent[cur]
					if cur == "" { break }
					if cur == v { break }
					cycle = append(cycle, cur)
				}
				loops = append(loops, cycle)
			} else if color[v] == white {
				parent[v] = u
				path[v] = append(append([]string{}, path[u]...), v)
				dfs(v)
			}
		}
		color[u] = black
	}

	for _, id := range g.AllNodeIDs() {
		if color[id] == white {
			path[id] = []string{id}
			dfs(id)
		}
	}
	return loops
}

// FindSPOF finds single points of failure — nodes whose removal disconnects the graph.
// Only considers nodes of the specified type. Uses brute-force removal + connectivity check.
func FindSPOF(g *Graph, nodeType NodeType) []string {
	candidates := g.NodesByType(nodeType)
	if len(candidates) <= 2 { return nil }

	var spofs []string
	for _, candidate := range candidates {
		if isArticulationPoint(g, candidate.ID, nodeType) {
			spofs = append(spofs, candidate.ID)
		}
	}
	return spofs
}

// isArticulationPoint checks if removing a node disconnects the subgraph of a given type.
func isArticulationPoint(g *Graph, removeID string, nodeType NodeType) bool {
	// Get all nodes of this type except the removed one
	var remaining []string
	for _, n := range g.NodesByType(nodeType) {
		if n.ID != removeID { remaining = append(remaining, n.ID) }
	}
	if len(remaining) <= 1 { return false }

	// BFS from first remaining node, only traversing nodes of the same type
	start := remaining[0]
	visited := map[string]bool{start: true}
	queue := []string{start}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, edge := range g.Neighbors(current) {
			if edge.To == removeID { continue }
			if visited[edge.To] { continue }
			n, ok := g.GetNode(edge.To)
			if !ok { continue }
			if n.Type != nodeType { continue }
			visited[edge.To] = true
			queue = append(queue, edge.To)
		}
	}

	return len(visited) < len(remaining)
}

// ImpactAnalysis determines which nodes become unreachable if a given node is removed.
// Returns nodes of the specified type that lose all paths to remaining nodes.
func ImpactAnalysis(g *Graph, removeID string, nodeType NodeType) []string {
	// Get all nodes of this type except the removed one
	var remaining []string
	for _, n := range g.NodesByType(nodeType) {
		if n.ID != removeID { remaining = append(remaining, n.ID) }
	}
	if len(remaining) <= 1 { return nil }

	// BFS from the first remaining node
	start := remaining[0]
	reachable := map[string]bool{start: true}
	queue := []string{start}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, edge := range g.Neighbors(current) {
			if edge.To == removeID { continue }
			if reachable[edge.To] { continue }
			n, ok := g.GetNode(edge.To)
			if !ok || n.Type != nodeType { continue }
			reachable[edge.To] = true
			queue = append(queue, edge.To)
		}
	}

	// Nodes not reachable are affected
	var affected []string
	for _, id := range remaining {
		if !reachable[id] { affected = append(affected, id) }
	}
	return affected
}
```

- [ ] **Step 4: Run test — verify PASS**

Run: `go test ./internal/graph/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/graph/analysis.go internal/graph/analysis_test.go
git commit -m "feat: add loop detection, SPOF finding, and impact analysis"
```

---

### Task 5: CLI — show topology + trace + check commands

**Files:**
- Modify: `internal/cli/show.go`
- Create: `internal/cli/trace.go`
- Create: `internal/cli/check.go`
- Modify: `internal/cli/root.go`

- [ ] **Step 1: Create trace.go**

```go
// internal/cli/trace.go
package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/graph"
)

func newTraceCmd() *cobra.Command {
	trace := &cobra.Command{
		Use:   "trace",
		Short: "Path analysis and impact assessment",
	}
	trace.AddCommand(newTracePathCmd())
	trace.AddCommand(newTraceImpactCmd())
	return trace
}

func newTracePathCmd() *cobra.Command {
	var from, to string
	var all bool
	cmd := &cobra.Command{
		Use:   "path",
		Short: "Trace path between two devices",
		RunE: func(cmd *cobra.Command, args []string) error {
			if from == "" || to == "" { return fmt.Errorf("--from and --to are required") }

			g, err := graph.BuildFromDB(db)
			if err != nil { return fmt.Errorf("build graph: %w", err) }

			from = strings.ToLower(from)
			to = strings.ToLower(to)

			if all {
				paths := graph.AllPaths(g, from, to, 10)
				if len(paths) == 0 {
					fmt.Printf("No path found from %s to %s\n", from, to)
					return nil
				}
				fmt.Printf("Found %d path(s) from %s to %s:\n\n", len(paths), from, to)
				for i, path := range paths {
					fmt.Printf("  Path %d (%d hops): %s\n", i+1, len(path)-1, strings.Join(path, " → "))
				}
				return nil
			}

			path, err := graph.ShortestPath(g, from, to)
			if err != nil { return err }

			fmt.Printf("Shortest path from %s to %s (%d hops):\n\n", from, to, len(path)-1)
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "HOP\tNODE\tTYPE\tDETAILS\n")
			for i, nodeID := range path {
				n, ok := g.GetNode(nodeID)
				if !ok { continue }
				details := ""
				if n.Props["hostname"] != "" { details = n.Props["hostname"] }
				if n.Props["ip"] != "" { details = n.Props["ip"] }
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", i, nodeID, n.Type, details)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "source device ID")
	cmd.Flags().StringVar(&to, "to", "", "destination device ID")
	cmd.Flags().BoolVar(&all, "all", false, "show all paths (not just shortest)")
	return cmd
}

func newTraceImpactCmd() *cobra.Command {
	var node string
	cmd := &cobra.Command{
		Use:   "impact",
		Short: "Analyze impact of a node/link failure",
		RunE: func(cmd *cobra.Command, args []string) error {
			if node == "" { return fmt.Errorf("--node is required") }

			g, err := graph.BuildFromDB(db)
			if err != nil { return fmt.Errorf("build graph: %w", err) }

			node = strings.ToLower(node)
			affected := graph.ImpactAnalysis(g, node, graph.NodeTypeDevice)

			if len(affected) == 0 {
				fmt.Printf("Removing %s has no impact — all remaining devices stay connected.\n", node)
				return nil
			}

			fmt.Printf("Removing %s would isolate %d device(s):\n\n", node, len(affected))
			for _, id := range affected {
				n, ok := g.GetNode(id)
				hostname := id
				if ok && n.Props["hostname"] != "" { hostname = n.Props["hostname"] }
				fmt.Printf("  - %s (%s)\n", id, hostname)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&node, "node", "", "node to simulate removing")
	return cmd
}
```

- [ ] **Step 2: Create check.go**

```go
// internal/cli/check.go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/graph"
)

func newCheckCmd() *cobra.Command {
	check := &cobra.Command{
		Use:   "check",
		Short: "Health checks and consistency verification",
	}
	check.AddCommand(newCheckLoopCmd())
	check.AddCommand(newCheckSPOFCmd())
	return check
}

func newCheckLoopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "loop",
		Short: "Detect routing loops",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := graph.BuildFromDB(db)
			if err != nil { return fmt.Errorf("build graph: %w", err) }

			loops := graph.DetectLoops(g)
			if len(loops) == 0 {
				fmt.Println("No loops detected.")
				return nil
			}

			fmt.Printf("Found %d potential loop(s):\n\n", len(loops))
			for i, loop := range loops {
				fmt.Printf("  Loop %d: %v\n", i+1, loop)
			}
			return nil
		},
	}
}

func newCheckSPOFCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "spof",
		Short: "Find single points of failure",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := graph.BuildFromDB(db)
			if err != nil { return fmt.Errorf("build graph: %w", err) }

			spofs := graph.FindSPOF(g, graph.NodeTypeDevice)
			if len(spofs) == 0 {
				fmt.Println("No single points of failure found.")
				return nil
			}

			fmt.Printf("Found %d single point(s) of failure:\n\n", len(spofs))
			for _, id := range spofs {
				n, ok := g.GetNode(id)
				hostname := id
				if ok && n.Props["hostname"] != "" { hostname = n.Props["hostname"] }
				fmt.Printf("  - %s (%s)\n", id, hostname)
			}
			return nil
		},
	}
}
```

- [ ] **Step 3: Add show topology to show.go**

Add this function to `internal/cli/show.go` and register it in `newShowCmd`:

```go
func newShowTopologyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "topology",
		Short: "Show network topology overview",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := graph.BuildFromDB(db)
			if err != nil { return fmt.Errorf("build graph: %w", err) }

			fmt.Printf("Network Topology:\n")
			fmt.Printf("  Devices:    %d\n", len(g.NodesByType(graph.NodeTypeDevice)))
			fmt.Printf("  Interfaces: %d\n", len(g.NodesByType(graph.NodeTypeInterface)))
			fmt.Printf("  Subnets:    %d\n", len(g.NodesByType(graph.NodeTypeSubnet)))
			fmt.Printf("  Total nodes: %d\n", g.NodeCount())
			fmt.Printf("  Total edges: %d\n\n", g.EdgeCount())

			// List devices with their peer count
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "DEVICE\tHOSTNAME\tPEERS\tINTERFACES\n")
			for _, dev := range g.NodesByType(graph.NodeTypeDevice) {
				peers := g.NeighborsByType(dev.ID, graph.EdgePeer)
				ifaces := g.NeighborsByType(dev.ID, graph.EdgeHasInterface)
				fmt.Fprintf(w, "%s\t%s\t%d\t%d\n", dev.ID, dev.Props["hostname"], len(peers), len(ifaces))
			}
			return w.Flush()
		},
	}
}
```

Add `"github.com/xavierli/nethelper/internal/graph"` import to show.go.

In `newShowCmd`, add: `show.AddCommand(newShowTopologyCmd())`

- [ ] **Step 4: Register trace and check in root.go**

In `internal/cli/root.go`, add to `NewRootCmd`:

```go
	root.AddCommand(newTraceCmd())
	root.AddCommand(newCheckCmd())
```

- [ ] **Step 5: Build and verify**

Run: `go build ./cmd/nethelper`
Expected: compiles

Run: `./nethelper trace --help`
Expected: shows path and impact subcommands

Run: `./nethelper check --help`
Expected: shows loop and spof subcommands

Run: `./nethelper show topology --help`
Expected: shows topology command

- [ ] **Step 6: Run full test suite**

Run: `go test ./... -timeout 60s`
Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add internal/cli/trace.go internal/cli/check.go internal/cli/show.go internal/cli/root.go
git commit -m "feat: add trace path/impact, check loop/spof, and show topology CLI commands"
```

- [ ] **Step 8: Clean up**

Run: `rm -f nethelper`
