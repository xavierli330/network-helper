// internal/graph/graph.go
package graph

// NodeType represents the type of a graph node.
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

// EdgeType represents the type of a graph edge.
type EdgeType string

const (
	EdgeHasInterface   EdgeType = "HAS_INTERFACE"
	EdgeMemberOf       EdgeType = "MEMBER_OF"
	EdgeConnectsTo     EdgeType = "CONNECTS_TO"
	EdgeInSubnet       EdgeType = "IN_SUBNET"
	EdgeInVRF          EdgeType = "IN_VRF"
	EdgePeer           EdgeType = "PEER"
	EdgeRoutesVia      EdgeType = "ROUTES_VIA"
	EdgeLSPHop         EdgeType = "LSP_HOP"
	EdgeLabelBind      EdgeType = "LABEL_BIND"
	EdgeTunnelsThrough EdgeType = "TUNNELS_THROUGH"
)

// Node is a vertex in the graph with a type and properties.
type Node struct {
	ID    string
	Type  NodeType
	Props map[string]string
}

// Edge is a directed edge in the graph with a type and properties.
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

// New creates and returns an empty Graph.
func New() *Graph {
	return &Graph{
		nodes: make(map[string]*Node),
		edges: make(map[string][]Edge),
	}
}

// AddNode adds or replaces a node in the graph.
func (g *Graph) AddNode(id string, nodeType NodeType, props map[string]string) {
	if props == nil {
		props = make(map[string]string)
	}
	g.nodes[id] = &Node{ID: id, Type: nodeType, Props: props}
}

// GetNode retrieves a node by ID.
func (g *Graph) GetNode(id string) (*Node, bool) {
	n, ok := g.nodes[id]
	return n, ok
}

// AddEdge adds a directed edge from → to.
func (g *Graph) AddEdge(from, to string, edgeType EdgeType, props map[string]string) {
	if props == nil {
		props = make(map[string]string)
	}
	g.edges[from] = append(g.edges[from], Edge{From: from, To: to, Type: edgeType, Props: props})
}

// AddBidirectionalEdge adds edges in both directions.
func (g *Graph) AddBidirectionalEdge(a, b string, edgeType EdgeType, props map[string]string) {
	g.AddEdge(a, b, edgeType, props)
	g.AddEdge(b, a, edgeType, props)
}

// Neighbors returns all outgoing edges from the given node.
func (g *Graph) Neighbors(id string) []Edge {
	return g.edges[id]
}

// NeighborsByType returns outgoing edges of a specific type from a node.
func (g *Graph) NeighborsByType(id string, edgeType EdgeType) []Edge {
	var result []Edge
	for _, e := range g.edges[id] {
		if e.Type == edgeType {
			result = append(result, e)
		}
	}
	return result
}

// EdgesBetween returns all edges directed from → to.
func (g *Graph) EdgesBetween(from, to string) []Edge {
	var result []Edge
	for _, e := range g.edges[from] {
		if e.To == to {
			result = append(result, e)
		}
	}
	return result
}

// NodesByType returns all nodes of a given type.
func (g *Graph) NodesByType(nodeType NodeType) []*Node {
	var result []*Node
	for _, n := range g.nodes {
		if n.Type == nodeType {
			result = append(result, n)
		}
	}
	return result
}

// NodeCount returns the total number of nodes.
func (g *Graph) NodeCount() int { return len(g.nodes) }

// EdgeCount returns the total number of directed edges.
func (g *Graph) EdgeCount() int {
	count := 0
	for _, edges := range g.edges {
		count += len(edges)
	}
	return count
}

// AllNodeIDs returns all node IDs in the graph.
func (g *Graph) AllNodeIDs() []string {
	ids := make([]string, 0, len(g.nodes))
	for id := range g.nodes {
		ids = append(ids, id)
	}
	return ids
}
