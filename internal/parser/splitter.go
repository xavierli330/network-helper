package parser

import (
	"regexp"
	"strings"
	"time"

	"github.com/xavierli/nethelper/internal/model"
)

// timestampRe matches common log timestamp prefixes:
// "2026-03-21-13-11-26: " or "2026-03-21 13:11:26 " or "Mar 21 13:11:26 "
var timestampRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[-T ]\d{2}[-:]\d{2}[-:]\d{2}[:.]\s*`)

// timestampLayouts lists the time.Parse formats tried in order when extracting a timestamp.
var timestampLayouts = []string{
	"2006-01-02-15-04-05",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
}

// stripTimestamp removes a leading timestamp prefix from a line if present.
func stripTimestamp(line string) string {
	return timestampRe.ReplaceAllString(line, "")
}

// extractTimestamp parses the timestamp value from a line's prefix.
// It returns the parsed time and true on success, or zero/false if no prefix is found.
func extractTimestamp(line string) (time.Time, bool) {
	loc := timestampRe.FindStringIndex(line)
	if loc == nil {
		return time.Time{}, false
	}
	// The matched prefix ends at loc[1]; trim the trailing separator/space to get the raw value.
	raw := strings.TrimRight(line[:loc[1]], ": \t")
	for _, layout := range timestampLayouts {
		if t, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// promptOnlyParser is a minimal VendorParser used only for prompt detection during splitting.
type promptOnlyParser struct {
	vendor   string
	promptRe *regexp.Regexp
}

func newPromptOnlyParser(vendor, pattern string) *promptOnlyParser {
	return &promptOnlyParser{vendor: vendor, promptRe: regexp.MustCompile(pattern)}
}

func (p *promptOnlyParser) Vendor() string { return p.vendor }
func (p *promptOnlyParser) DetectPrompt(line string) (string, bool) {
	m := p.promptRe.FindStringSubmatch(line)
	if m == nil { return "", false }
	return m[1], true
}
func (p *promptOnlyParser) ClassifyCommand(cmd string) model.CommandType { return model.CmdUnknown }
func (p *promptOnlyParser) ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error) {
	return model.ParseResult{RawText: raw}, nil
}

type promptMatch struct {
	lineIndex  int
	hostname   string
	vendor     string
	command    string
	capturedAt time.Time // zero if the line had no timestamp prefix
}

func Split(raw string, registry *Registry) []CommandBlock {
	if strings.TrimSpace(raw) == "" { return nil }

	lines := strings.Split(raw, "\n")
	parsers := registry.Parsers()
	var matches []promptMatch

	for i, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" { continue }
		// Extract timestamp value before stripping the prefix.
		ts, _ := extractTimestamp(trimmed)
		// Strip timestamp prefix before prompt detection.
		stripped := stripTimestamp(trimmed)
		for _, p := range parsers {
			hostname, ok := p.DetectPrompt(stripped)
			if !ok { continue }
			cmd := extractCommand(stripped, p)
			if cmd == "" { continue }
			matches = append(matches, promptMatch{lineIndex: i, hostname: hostname, vendor: p.Vendor(), command: cmd, capturedAt: ts})
			break
		}
	}

	var blocks []CommandBlock
	for i, m := range matches {
		outputStart := m.lineIndex + 1
		var outputEnd int
		if i+1 < len(matches) { outputEnd = matches[i+1].lineIndex } else { outputEnd = len(lines) }
		// Collect output lines, stripping timestamps from each
		var outputLines []string
		if outputStart < outputEnd {
			for _, ol := range lines[outputStart:outputEnd] {
				stripped := stripTimestamp(strings.TrimRight(ol, "\r"))
				outputLines = append(outputLines, stripped)
			}
		}
		output := strings.TrimRight(strings.Join(outputLines, "\n"), "\n\r \t")
		blocks = append(blocks, CommandBlock{Hostname: m.hostname, Vendor: m.vendor, Command: m.command, Output: output, CapturedAt: m.capturedAt})
	}
	return blocks
}

// extractCommand gets the command text after the prompt pattern on a line.
// For promptOnlyParser, uses regex loc. For full VendorParsers, uses heuristic with common delimiters.
func extractCommand(line string, p VendorParser) string {
	switch pp := p.(type) {
	case *promptOnlyParser:
		loc := pp.promptRe.FindStringIndex(line)
		if loc == nil { return "" }
		return strings.TrimSpace(line[loc[1]:])
	default:
		hostname, ok := p.DetectPrompt(line)
		if !ok { return "" }
		for _, delim := range []string{">", "#", "]"} {
			idx := strings.Index(line, hostname)
			if idx < 0 { continue }
			afterHostname := line[idx+len(hostname):]
			delimIdx := strings.Index(afterHostname, delim)
			if delimIdx >= 0 {
				return strings.TrimSpace(afterHostname[delimIdx+len(delim):])
			}
		}
		return ""
	}
}
