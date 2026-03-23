package channel

import "strings"

type PermissionGroup struct {
	Name  string
	Users []string
	Tools []string
}

type PermissionConfig struct {
	Groups []PermissionGroup
}

func (pc *PermissionConfig) Resolve(userKey string) *PermissionGroup {
	var fallback *PermissionGroup
	for i := range pc.Groups {
		g := &pc.Groups[i]
		for _, u := range g.Users {
			if u == userKey {
				return g
			}
			if u == "*" {
				fallback = g
			}
		}
	}
	return fallback
}

func (g *PermissionGroup) ToolAllowed(toolName string) bool {
	for _, pattern := range g.Tools {
		if pattern == "*" || pattern == toolName {
			return true
		}
		if strings.HasSuffix(pattern, "*") {
			if strings.HasPrefix(toolName, strings.TrimSuffix(pattern, "*")) {
				return true
			}
		}
	}
	return false
}
