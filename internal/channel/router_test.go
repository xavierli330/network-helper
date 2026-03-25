package channel

import (
	"testing"
)

func TestPermissionGroup_ToolAllowed(t *testing.T) {
	tests := []struct {
		name     string
		group    PermissionGroup
		tool     string
		expected bool
	}{
		{
			name:     "wildcard allows all",
			group:    PermissionGroup{Name: "admin", Tools: []string{"*"}},
			tool:     "any_tool",
			expected: true,
		},
		{
			name:     "exact match",
			group:    PermissionGroup{Name: "readonly", Tools: []string{"show_devices", "show_interfaces"}},
			tool:     "show_devices",
			expected: true,
		},
		{
			name:     "prefix match",
			group:    PermissionGroup{Name: "readonly", Tools: []string{"show_*"}},
			tool:     "show_interfaces",
			expected: true,
		},
		{
			name:     "denied tool",
			group:    PermissionGroup{Name: "readonly", Tools: []string{"show_*"}},
			tool:     "plan_isolate",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.group.ToolAllowed(tt.tool)
			if got != tt.expected {
				t.Errorf("ToolAllowed(%q) = %v, want %v", tt.tool, got, tt.expected)
			}
		})
	}
}

func TestPermissionConfig_Resolve(t *testing.T) {
	config := PermissionConfig{
		Groups: []PermissionGroup{
			{Name: "admin", Users: []string{"user1"}, Tools: []string{"*"}},
			{Name: "readonly", Users: []string{"*"}, Tools: []string{"show_*"}},
		},
	}

	t.Run("find specific user", func(t *testing.T) {
		group := config.Resolve("user1")
		if group == nil {
			t.Fatal("expected to find group for user1")
		}
		if group.Name != "admin" {
			t.Errorf("expected admin group, got %s", group.Name)
		}
	})

	t.Run("fallback to wildcard", func(t *testing.T) {
		group := config.Resolve("unknown_user")
		if group == nil {
			t.Fatal("expected to find fallback group")
		}
		if group.Name != "readonly" {
			t.Errorf("expected readonly group, got %s", group.Name)
		}
	})
}
