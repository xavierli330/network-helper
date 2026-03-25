package engine

import (
	"regexp"
	"strings"
)

// ColumnDef describes a single column in a table output.
type ColumnDef struct {
	Name     string // field name in result row
	Index    int    // 0-based position in whitespace-split fields
	Type     string // "string" | "int" | "ip" | "duration" | "bytes" (future coercion)
	Optional bool
}

// TableSchema describes how to parse a fixed-column CLI table output.
type TableSchema struct {
	HeaderPattern string      // regex matching the header line
	SkipLines     int         // lines to skip after header (e.g. separator row)
	Columns       []ColumnDef
}

// TableResult holds the parsed rows.
type TableResult struct {
	// Rows is a slice of maps from column name to string value.
	Rows []map[string]string
}

// ParseTable scans raw for the header line matching HeaderPattern, then parses
// subsequent data lines into Rows using whitespace splitting.
// Returns an empty TableResult (not an error) if the header is not found.
func ParseTable(schema TableSchema, raw string) (TableResult, error) {
	headerRe, err := regexp.Compile(schema.HeaderPattern)
	if err != nil {
		return TableResult{}, err
	}

	lines := strings.Split(raw, "\n")
	headerIdx := -1
	for i, line := range lines {
		if headerRe.MatchString(line) {
			headerIdx = i
			break
		}
	}
	if headerIdx == -1 {
		return TableResult{}, nil
	}

	dataStart := headerIdx + 1 + schema.SkipLines
	var rows []map[string]string
	for _, line := range lines[dataStart:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		row := make(map[string]string, len(schema.Columns))
		for _, col := range schema.Columns {
			if col.Index < len(fields) {
				row[col.Name] = fields[col.Index]
			} else if !col.Optional {
				row[col.Name] = ""
			}
		}
		rows = append(rows, row)
	}
	return TableResult{Rows: rows}, nil
}
