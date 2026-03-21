// internal/graph/path.go
package graph

import "fmt"

// ShortestPath finds the shortest path between two nodes using BFS.
// Returns the ordered list of node IDs from src to dst.
func ShortestPath(g *Graph, src, dst string) ([]string, error) {
	if src == dst {
		return []string{src}, nil
	}
	if _, ok := g.GetNode(src); !ok {
		return nil, fmt.Errorf("source node %q not found", src)
	}
	if _, ok := g.GetNode(dst); !ok {
		return nil, fmt.Errorf("destination node %q not found", dst)
	}

	visited := map[string]bool{src: true}
	parent := map[string]string{}
	queue := []string{src}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, edge := range g.Neighbors(current) {
			if visited[edge.To] {
				continue
			}
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

// ShortestPathByNodeType finds the shortest path only traversing nodes of a given type.
func ShortestPathByNodeType(g *Graph, src, dst string, nodeType NodeType) ([]string, error) {
	if src == dst {
		return []string{src}, nil
	}
	if _, ok := g.GetNode(src); !ok {
		return nil, fmt.Errorf("source node %q not found", src)
	}
	if _, ok := g.GetNode(dst); !ok {
		return nil, fmt.Errorf("destination node %q not found", dst)
	}

	visited := map[string]bool{src: true}
	parent := map[string]string{}
	queue := []string{src}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, edge := range g.Neighbors(current) {
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
			parent[edge.To] = current

			if edge.To == dst {
				return reconstructPath(parent, src, dst), nil
			}
			queue = append(queue, edge.To)
		}
	}

	return nil, fmt.Errorf("no path from %q to %q (type=%s)", src, dst, nodeType)
}

// AllPaths finds all simple paths from src to dst up to maxDepth using DFS.
func AllPaths(g *Graph, src, dst string, maxDepth int) [][]string {
	var results [][]string
	visited := map[string]bool{}

	var dfs func(current string, path []string)
	dfs = func(current string, path []string) {
		if len(path) > maxDepth {
			return
		}
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

// reconstructPath builds the path from src to dst using the parent map.
func reconstructPath(parent map[string]string, src, dst string) []string {
	var path []string
	for current := dst; current != src; current = parent[current] {
		path = append([]string{current}, path...)
	}
	return append([]string{src}, path...)
}
