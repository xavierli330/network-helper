package parser

import (
	"regexp"
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

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
	lineIndex int
	hostname  string
	vendor    string
	command   string
}

func Split(raw string, registry *Registry) []CommandBlock {
	if strings.TrimSpace(raw) == "" { return nil }

	lines := strings.Split(raw, "\n")
	parsers := registry.Parsers()
	var matches []promptMatch

	for i, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" { continue }
		for _, p := range parsers {
			hostname, ok := p.DetectPrompt(trimmed)
			if !ok { continue }
			cmd := extractCommand(trimmed, p)
			if cmd == "" { continue }
			matches = append(matches, promptMatch{lineIndex: i, hostname: hostname, vendor: p.Vendor(), command: cmd})
			break
		}
	}

	var blocks []CommandBlock
	for i, m := range matches {
		outputStart := m.lineIndex + 1
		var outputEnd int
		if i+1 < len(matches) { outputEnd = matches[i+1].lineIndex } else { outputEnd = len(lines) }
		var outputLines []string
		if outputStart < outputEnd { outputLines = lines[outputStart:outputEnd] }
		output := strings.TrimRight(strings.Join(outputLines, "\n"), "\n\r \t")
		blocks = append(blocks, CommandBlock{Hostname: m.hostname, Vendor: m.vendor, Command: m.command, Output: output})
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
