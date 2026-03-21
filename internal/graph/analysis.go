// internal/graph/analysis.go
package graph

// DetectLoops finds directed cycles in the graph using DFS coloring.
// Returns a list of cycles, each represented as a slice of node IDs.
func DetectLoops(g *Graph) [][]string {
	var loops [][]string

	const (
		white = 0 // unvisited
		gray  = 1 // in current DFS path
		black = 2 // fully processed
	)

	color := make(map[string]int)
	parent := make(map[string]string)

	for _, id := range g.AllNodeIDs() {
		color[id] = white
	}

	var dfs func(u string)
	dfs = func(u string) {
		color[u] = gray
		for _, edge := range g.Neighbors(u) {
			v := edge.To
			if color[v] == gray {
				// Back edge found — extract cycle by walking up parent chain to v
				cycle := []string{v}
				for cur := u; cur != v; {
					cycle = append(cycle, cur)
					p, ok := parent[cur]
					if !ok || p == "" {
						break
					}
					cur = p
				}
				loops = append(loops, cycle)
			} else if color[v] == white {
				parent[v] = u
				dfs(v)
			}
		}
		color[u] = black
	}

	for _, id := range g.AllNodeIDs() {
		if color[id] == white {
			dfs(id)
		}
	}
	return loops
}

// FindSPOF finds single points of failure — nodes whose removal disconnects the graph.
// Only considers nodes of the specified type. Uses brute-force removal + connectivity check.
func FindSPOF(g *Graph, nodeType NodeType) []string {
	candidates := g.NodesByType(nodeType)
	if len(candidates) <= 2 {
		return nil
	}

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
	// Collect all nodes of this type except the removed one
	var remaining []string
	for _, n := range g.NodesByType(nodeType) {
		if n.ID != removeID {
			remaining = append(remaining, n.ID)
		}
	}
	if len(remaining) <= 1 {
		return false
	}

	// BFS from the first remaining node, only traversing nodes of the same type
	start := remaining[0]
	visited := map[string]bool{start: true}
	queue := []string{start}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, edge := range g.Neighbors(current) {
			if edge.To == removeID {
				continue
			}
			if visited[edge.To] {
				continue
			}
			n, ok := g.GetNode(edge.To)
			if !ok {
				continue
			}
			if n.Type != nodeType {
				continue
			}
			visited[edge.To] = true
			queue = append(queue, edge.To)
		}
	}

	return len(visited) < len(remaining)
}

// ImpactAnalysis determines which nodes become unreachable if a given node is removed.
// Returns nodes of the specified type that lose all paths to at least one remaining node.
func ImpactAnalysis(g *Graph, removeID string, nodeType NodeType) []string {
	// Collect all nodes of this type except the removed one
	var remaining []string
	for _, n := range g.NodesByType(nodeType) {
		if n.ID != removeID {
			remaining = append(remaining, n.ID)
		}
	}
	if len(remaining) <= 1 {
		return nil
	}

	// BFS from the first remaining node, skipping removeID
	start := remaining[0]
	reachable := map[string]bool{start: true}
	queue := []string{start}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, edge := range g.Neighbors(current) {
			if edge.To == removeID {
				continue
			}
			if reachable[edge.To] {
				continue
			}
			n, ok := g.GetNode(edge.To)
			if !ok || n.Type != nodeType {
				continue
			}
			reachable[edge.To] = true
			queue = append(queue, edge.To)
		}
	}

	// Nodes not reachable from start are affected
	var affected []string
	for _, id := range remaining {
		if !reachable[id] {
			affected = append(affected, id)
		}
	}
	return affected
}
