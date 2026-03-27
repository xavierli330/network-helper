package engine

import (
	"regexp"
	"strings"
	"unicode"
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
	Columns       []ColumnDef // if empty/nil, auto-detect columns from header line
}

// TableResult holds the parsed rows.
type TableResult struct {
	// Rows is a slice of maps from column name to string value.
	Rows []map[string]string
	// AutoColumns is populated when columns were auto-detected from the header line.
	// Nil when explicit Columns were provided in the schema.
	AutoColumns []ColumnDef `json:"auto_columns,omitempty"`
}

// ParseTable scans raw for the header line matching HeaderPattern, then parses
// subsequent data lines into Rows using whitespace splitting.
//
// If schema.Columns is empty, columns are auto-detected from the header line:
// each whitespace-separated token in the header becomes a column name (lowercased,
// with special chars replaced by underscore). The auto-detected columns are
// returned in TableResult.AutoColumns so callers can display them.
//
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

	// Auto-detect columns from header line when Columns is empty
	columns := schema.Columns
	var autoDetected []ColumnDef
	if len(columns) == 0 {
		headerTokens := strings.Fields(lines[headerIdx])
		columns = make([]ColumnDef, len(headerTokens))
		for i, tok := range headerTokens {
			columns[i] = ColumnDef{
				Name:  normaliseColumnName(tok),
				Index: i,
				Type:  "string",
			}
		}
		autoDetected = columns
	}

	dataStart := headerIdx + 1 + schema.SkipLines
	if dataStart > len(lines) {
		dataStart = len(lines)
	}
	var rows []map[string]string
	for _, line := range lines[dataStart:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		row := make(map[string]string, len(columns))
		for _, col := range columns {
			if col.Index < len(fields) {
				row[col.Name] = fields[col.Index]
			} else if !col.Optional {
				row[col.Name] = ""
			}
		}
		rows = append(rows, row)
	}
	return TableResult{Rows: rows, AutoColumns: autoDetected}, nil
}

// normaliseColumnName converts a header token to a snake_case column name.
// "Interface" → "interface", "IP Address" → "ip_address", "PHY-Status" → "phy_status"
func normaliseColumnName(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	prevUnderscore := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
			prevUnderscore = false
		} else if !prevUnderscore && b.Len() > 0 {
			b.WriteByte('_')
			prevUnderscore = true
		}
	}
	result := b.String()
	return strings.TrimRight(result, "_")
}
