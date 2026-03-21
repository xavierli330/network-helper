package diff

import (
	"fmt"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// Unified produces a unified diff between two texts.
// Returns empty string if texts are identical.
func Unified(oldText, newText, oldName, newName string) string {
	if oldText == newText {
		return ""
	}

	dmp := diffmatchpatch.New()
	a, b, lineArray := dmp.DiffLinesToChars(oldText, newText)
	diffs := dmp.DiffMain(a, b, false)
	diffs = dmp.DiffCharsToLines(diffs, lineArray)
	diffs = dmp.DiffCleanupSemantic(diffs)

	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n", oldName)
	fmt.Fprintf(&sb, "+++ %s\n", newName)

	for _, d := range diffs {
		lines := strings.Split(strings.TrimRight(d.Text, "\n"), "\n")
		for _, line := range lines {
			switch d.Type {
			case diffmatchpatch.DiffDelete:
				fmt.Fprintf(&sb, "-%s\n", line)
			case diffmatchpatch.DiffInsert:
				fmt.Fprintf(&sb, "+%s\n", line)
			case diffmatchpatch.DiffEqual:
				fmt.Fprintf(&sb, " %s\n", line)
			}
		}
	}

	return sb.String()
}
