package diff

import (
	"strings"
	"testing"
)

func TestUnifiedDiff(t *testing.T) {
	old := `interface GE0/0/1
 ip address 10.0.0.1 24
 ospf cost 100
#
interface GE0/0/2
 ip address 10.0.0.2 24
#`

	new_ := `interface GE0/0/1
 ip address 10.0.0.1 24
 ospf cost 200
#
interface GE0/0/2
 ip address 10.0.0.2 24
 description To-PE-01
#`

	result := Unified(old, new_, "before", "after")
	if result == "" {
		t.Fatal("expected non-empty diff")
	}
	if !strings.Contains(result, "-") && !strings.Contains(result, "+") {
		t.Error("diff should contain - and + lines")
	}
	// The cost change should appear
	if !strings.Contains(result, "cost") {
		t.Error("diff should mention cost change")
	}
}

func TestUnifiedDiffIdentical(t *testing.T) {
	text := "line1\nline2\nline3"
	result := Unified(text, text, "a", "b")
	if result != "" {
		t.Errorf("expected empty diff for identical, got: %s", result)
	}
}

func TestUnifiedDiffEmpty(t *testing.T) {
	result := Unified("", "new content", "a", "b")
	if result == "" {
		t.Error("expected diff for empty→content")
	}
}
