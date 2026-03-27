# Nethelper Plan 2: Parser Pipeline + Huawei Parser

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the parser pipeline (splitter → detector → parser → store) and implement the first vendor parser (Huawei VRP) covering `display interface brief`, `display ip routing-table`, `display ospf peer`, `display mpls ldp session`, and `display mpls lsp`. Then wire `watch ingest` to actually parse files.

**Architecture:** The parser package defines a `VendorParser` interface. The splitter cuts raw terminal logs into individual command blocks by recognizing prompt patterns. The detector identifies vendor + command type. Each vendor has its own sub-package implementing the interface. The pipeline orchestrator ties everything together and writes parsed results into the store. Plan 2 focuses on Huawei only — other vendors follow in Plan 3.

**Tech Stack:** Go 1.24+, regexp (stdlib), existing internal/model and internal/store packages

**Spec:** `docs/superpowers/specs/2026-03-21-network-helper-design.md` (Section 4)

**Depends on:** Plan 1 (Core Foundation) — all model structs and store operations exist

---

## File Structure

```
internal/
├── parser/
│   ├── types.go                   # VendorParser interface, CommandBlock struct, registry
│   ├── splitter.go                # Split raw log into CommandBlocks by prompt detection
│   ├── detector.go                # Detect vendor from prompt style, classify command type
│   ├── pipeline.go                # Orchestrator: read file → split → detect → parse → store
│   ├── pipeline_test.go           # Integration test for full pipeline
│   ├── splitter_test.go           # Unit tests for splitter
│   ├── detector_test.go           # Unit tests for detector
│   └── huawei/
│       ├── huawei.go              # HuaweiParser implementing VendorParser
│       ├── interface_brief.go     # Parse "display interface brief"
│       ├── routing_table.go       # Parse "display ip routing-table"
│       ├── ospf_peer.go           # Parse "display ospf peer"
│       ├── ldp_session.go         # Parse "display mpls ldp session"
│       ├── mpls_lsp.go            # Parse "display mpls lsp"
│       └── huawei_test.go         # Tests for all Huawei parsers
├── cli/
│   └── watch.go                   # Modify: wire ingest to use pipeline
```

---

### Task 1: VendorParser Interface + CommandBlock + Registry

**Files:**
- Create: `internal/parser/types.go`

- [ ] **Step 1: Write test**

Create: `internal/parser/types_test.go`

```go
package parser

import (
	"testing"

	"github.com/xavierli/nethelper/internal/model"
)

type mockParser struct{}

func (m *mockParser) Vendor() string                                                    { return "mock" }
func (m *mockParser) DetectPrompt(line string) (string, bool)                           { return "", false }
func (m *mockParser) ClassifyCommand(cmd string) model.CommandType                      { return model.CmdUnknown }
func (m *mockParser) ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error) {
	return model.ParseResult{}, nil
}

func TestRegisterAndGetParser(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockParser{})

	p, ok := r.Get("mock")
	if !ok {
		t.Fatal("expected to find mock parser")
	}
	if p.Vendor() != "mock" {
		t.Errorf("expected mock, got %s", p.Vendor())
	}

	_, ok = r.Get("nonexistent")
	if ok {
		t.Error("should not find nonexistent parser")
	}
}

func TestRegistryParsers(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockParser{})

	parsers := r.Parsers()
	if len(parsers) != 1 {
		t.Errorf("expected 1 parser, got %d", len(parsers))
	}
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/parser/ -v -run TestRegister`
Expected: FAIL (package doesn't exist)

- [ ] **Step 3: Implement types.go**

```go
// internal/parser/types.go
package parser

import "github.com/xavierli/nethelper/internal/model"

// VendorParser is the interface each vendor parser must implement.
type VendorParser interface {
	// Vendor returns the vendor identifier (e.g., "huawei", "cisco").
	Vendor() string

	// DetectPrompt checks if a line is a CLI prompt for this vendor.
	// Returns the hostname and true if detected.
	DetectPrompt(line string) (hostname string, ok bool)

	// ClassifyCommand identifies what type of command was executed.
	ClassifyCommand(cmd string) model.CommandType

	// ParseOutput parses the raw output of a command into structured data.
	ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error)
}

// CommandBlock represents a single command and its output extracted from a log.
type CommandBlock struct {
	Hostname string
	Vendor   string
	Command  string
	Output   string
	CmdType  model.CommandType
}

// Registry holds registered vendor parsers.
type Registry struct {
	parsers map[string]VendorParser
}

func NewRegistry() *Registry {
	return &Registry{parsers: make(map[string]VendorParser)}
}

func (r *Registry) Register(p VendorParser) {
	r.parsers[p.Vendor()] = p
}

func (r *Registry) Get(vendor string) (VendorParser, bool) {
	p, ok := r.parsers[vendor]
	return p, ok
}

func (r *Registry) Parsers() []VendorParser {
	result := make([]VendorParser, 0, len(r.parsers))
	for _, p := range r.parsers {
		result = append(result, p)
	}
	return result
}
```

- [ ] **Step 4: Run test — verify PASS**

Run: `go test ./internal/parser/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/parser/types.go internal/parser/types_test.go
git commit -m "feat: add VendorParser interface, CommandBlock, and Registry"
```

---

### Task 2: Splitter — Split Log Into Command Blocks

**Files:**
- Create: `internal/parser/splitter.go`
- Create: `internal/parser/splitter_test.go`

- [ ] **Step 1: Write test**

```go
// internal/parser/splitter_test.go
package parser

import "testing"

func TestSplitBlocks_Huawei(t *testing.T) {
	input := `<HUAWEI-Core-01>display ip routing-table
Route Flags: R - relay, D - download to fib
Routing Tables: Public
         Destinations : 12       Routes : 14

Destination/Mask    Proto   Pre  Cost  Flags NextHop         Interface
10.1.1.0/24         OSPF    10   2           10.0.0.1        GE0/0/1
172.16.0.0/16       Static  60   0     RD    10.0.0.2        GE0/0/2
<HUAWEI-Core-01>display interface brief
Interface                   PHY   Protocol InUti OutUti   inErr  outErr
GE0/0/1                     up    up       0.5%  0.3%         0       0
GE0/0/2                     down  down     0%    0%           0       0
`

	registry := NewRegistry()
	registry.Register(newPromptOnlyParser("huawei", `^<([^>]+)>`))

	blocks := Split(input, registry)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}

	if blocks[0].Hostname != "HUAWEI-Core-01" {
		t.Errorf("block 0 hostname: expected HUAWEI-Core-01, got %s", blocks[0].Hostname)
	}
	if blocks[0].Command != "display ip routing-table" {
		t.Errorf("block 0 command: expected 'display ip routing-table', got %q", blocks[0].Command)
	}
	if blocks[1].Command != "display interface brief" {
		t.Errorf("block 1 command: expected 'display interface brief', got %q", blocks[1].Command)
	}
}

func TestSplitBlocks_Cisco(t *testing.T) {
	input := `Router-PE01#show ip route
Codes: L - local, C - connected, S - static
Gateway of last resort is 10.0.0.1 to network 0.0.0.0
     10.0.0.0/8 is variably subnetted, 4 subnets, 2 masks
C       10.1.1.0/24 is directly connected, GigabitEthernet0/0
Router-PE01#show interfaces brief
Interface              IP-Address      OK? Method Status                Protocol
GigabitEthernet0/0     10.1.1.1        YES manual up                    up
`

	registry := NewRegistry()
	registry.Register(newPromptOnlyParser("cisco", `^([A-Za-z][A-Za-z0-9._-]*)#`))

	blocks := Split(input, registry)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].Hostname != "Router-PE01" {
		t.Errorf("expected Router-PE01, got %s", blocks[0].Hostname)
	}
}

func TestSplitBlocks_Empty(t *testing.T) {
	registry := NewRegistry()
	blocks := Split("", registry)
	if len(blocks) != 0 {
		t.Errorf("expected 0 blocks, got %d", len(blocks))
	}
}

func TestSplitBlocks_Juniper(t *testing.T) {
	input := `admin@MX204-01> show route
inet.0: 5 destinations, 5 routes
+ = Active Route

10.0.0.0/24        *[OSPF/10] 00:05:12
                    >  to 10.0.0.1 via ge-0/0/0.0
admin@MX204-01> show interfaces terse
Interface               Admin Link Proto    Local
ge-0/0/0                up    up
ge-0/0/0.0              up    up   inet     10.0.0.2/24
`

	registry := NewRegistry()
	registry.Register(newPromptOnlyParser("juniper", `^[a-zA-Z][a-zA-Z0-9._-]*@([A-Za-z][A-Za-z0-9._-]*)>`))

	blocks := Split(input, registry)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].Hostname != "MX204-01" {
		t.Errorf("expected MX204-01, got %s", blocks[0].Hostname)
	}
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/parser/ -v -run TestSplitBlocks`
Expected: FAIL

- [ ] **Step 3: Implement splitter.go**

```go
// internal/parser/splitter.go
package parser

import (
	"regexp"
	"strings"

	"github.com/xavierli/nethelper/internal/model"
)

// promptOnlyParser is a minimal VendorParser used only for prompt detection during splitting.
type promptOnlyParser struct {
	vendor    string
	promptRe  *regexp.Regexp
}

func newPromptOnlyParser(vendor, pattern string) *promptOnlyParser {
	return &promptOnlyParser{vendor: vendor, promptRe: regexp.MustCompile(pattern)}
}

func (p *promptOnlyParser) Vendor() string { return p.vendor }

func (p *promptOnlyParser) DetectPrompt(line string) (string, bool) {
	m := p.promptRe.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	return m[1], true
}

func (p *promptOnlyParser) ClassifyCommand(cmd string) model.CommandType {
	return model.CmdUnknown
}

func (p *promptOnlyParser) ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error) {
	return model.ParseResult{RawText: raw}, nil
}

// promptMatch holds info about a detected prompt line.
type promptMatch struct {
	lineIndex int
	hostname  string
	vendor    string
	command   string // text after the prompt on the same line
}

// Split splits raw terminal log text into CommandBlocks by detecting prompt lines.
func Split(raw string, registry *Registry) []CommandBlock {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	lines := strings.Split(raw, "\n")
	parsers := registry.Parsers()
	var matches []promptMatch

	for i, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" {
			continue
		}
		for _, p := range parsers {
			hostname, ok := p.DetectPrompt(trimmed)
			if !ok {
				continue
			}
			// Extract command: everything after the prompt match on this line
			cmd := extractCommand(trimmed, p)
			if cmd == "" {
				continue // prompt with no command (just a prompt line, skip)
			}
			matches = append(matches, promptMatch{
				lineIndex: i,
				hostname:  hostname,
				vendor:    p.Vendor(),
				command:   cmd,
			})
			break
		}
	}

	var blocks []CommandBlock
	for i, m := range matches {
		// Output starts on next line after prompt+command
		outputStart := m.lineIndex + 1
		var outputEnd int
		if i+1 < len(matches) {
			outputEnd = matches[i+1].lineIndex
		} else {
			outputEnd = len(lines)
		}

		var outputLines []string
		if outputStart < outputEnd {
			outputLines = lines[outputStart:outputEnd]
		}
		output := strings.Join(outputLines, "\n")
		output = strings.TrimRight(output, "\n\r \t")

		blocks = append(blocks, CommandBlock{
			Hostname: m.hostname,
			Vendor:   m.vendor,
			Command:  m.command,
			Output:   output,
		})
	}

	return blocks
}

// extractCommand gets the command text after the prompt pattern on a line.
func extractCommand(line string, p VendorParser) string {
	// Find prompt prefix and return everything after it
	switch pp := p.(type) {
	case *promptOnlyParser:
		loc := pp.promptRe.FindStringIndex(line)
		if loc == nil {
			return ""
		}
		cmd := strings.TrimSpace(line[loc[1]:])
		return cmd
	default:
		// For full VendorParsers, use DetectPrompt to find the hostname,
		// then find hostname in line and take everything after the prompt delimiter
		hostname, ok := p.DetectPrompt(line)
		if !ok {
			return ""
		}
		// Try common prompt endings: > # ] to find where command starts
		for _, delim := range []string{">", "#", "]"} {
			idx := strings.Index(line, hostname)
			if idx < 0 {
				continue
			}
			afterHostname := line[idx+len(hostname):]
			delimIdx := strings.Index(afterHostname, delim)
			if delimIdx >= 0 {
				cmd := strings.TrimSpace(afterHostname[delimIdx+len(delim):])
				return cmd
			}
		}
		return ""
	}
}
```

- [ ] **Step 4: Run test — verify PASS**

Run: `go test ./internal/parser/ -v -run TestSplitBlocks`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/parser/splitter.go internal/parser/splitter_test.go
git commit -m "feat: add log splitter that detects prompts and extracts command blocks"
```

---

### Task 3: Detector — Vendor Detection + Command Classification

**Files:**
- Create: `internal/parser/detector.go`
- Create: `internal/parser/detector_test.go`

- [ ] **Step 1: Write test**

```go
// internal/parser/detector_test.go
package parser

import (
	"testing"

	"github.com/xavierli/nethelper/internal/model"
)

func TestDetectVendor(t *testing.T) {
	tests := []struct {
		line     string
		wantVendor string
		wantHost   string
	}{
		{"<HUAWEI-Core-01>display version", "huawei", "HUAWEI-Core-01"},
		{"[HUAWEI-Core-01]display version", "huawei", "HUAWEI-Core-01"},
		{"Router-PE01#show version", "cisco", "Router-PE01"},
		{"Router-PE01(config)#interface GE0/0", "cisco", "Router-PE01"},
		{"admin@MX204-01> show version", "juniper", "MX204-01"},
		{"admin@MX204-01# set interfaces", "juniper", "MX204-01"},
		{"just some random text", "", ""},
	}

	for _, tt := range tests {
		vendor, hostname := DetectVendor(tt.line)
		if vendor != tt.wantVendor {
			t.Errorf("line %q: vendor=%q, want %q", tt.line, vendor, tt.wantVendor)
		}
		if hostname != tt.wantHost {
			t.Errorf("line %q: host=%q, want %q", tt.line, hostname, tt.wantHost)
		}
	}
}

func TestClassifyHuaweiCommand(t *testing.T) {
	tests := []struct {
		cmd  string
		want model.CommandType
	}{
		{"display ip routing-table", model.CmdRIB},
		{"display ip routing-table vpn-instance VPN1", model.CmdRIB},
		{"display interface brief", model.CmdInterface},
		{"display interface GigabitEthernet0/0/1", model.CmdInterface},
		{"display ospf peer", model.CmdNeighbor},
		{"display ospf peer brief", model.CmdNeighbor},
		{"display bgp peer", model.CmdNeighbor},
		{"display mpls ldp session", model.CmdNeighbor},
		{"display mpls lsp", model.CmdLFIB},
		{"display mpls lsp verbose", model.CmdLFIB},
		{"display current-configuration", model.CmdConfig},
		{"display saved-configuration", model.CmdConfig},
		{"display version", model.CmdUnknown},
	}

	for _, tt := range tests {
		got := ClassifyHuaweiCommand(tt.cmd)
		if got != tt.want {
			t.Errorf("cmd %q: got %q, want %q", tt.cmd, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/parser/ -v -run TestDetect`
Expected: FAIL

- [ ] **Step 3: Implement detector.go**

```go
// internal/parser/detector.go
package parser

import (
	"regexp"
	"strings"

	"github.com/xavierli/nethelper/internal/model"
)

// Prompt patterns for vendor detection
var (
	// Huawei/H3C: <hostname>command or [hostname]command
	huaweiAnglePrompt  = regexp.MustCompile(`^<([^>]+)>(.*)`)
	huaweiBracketPrompt = regexp.MustCompile(`^\[([^\]]+)\](.*)`)

	// Cisco: hostname#command or hostname(config)#command
	ciscoPrompt = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9._-]*)(?:\([^)]*\))?#(.*)`)

	// Juniper: user@hostname> command or user@hostname# command
	juniperPrompt = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9._-]*@([A-Za-z][A-Za-z0-9._-]*)[>#]\s*(.*)`)
)

// DetectVendor detects the vendor and hostname from a prompt line.
// Returns ("", "") if no vendor prompt is recognized.
func DetectVendor(line string) (vendor, hostname string) {
	trimmed := strings.TrimRight(line, "\r \t")

	// Try Huawei/H3C angle brackets: <hostname>
	if m := huaweiAnglePrompt.FindStringSubmatch(trimmed); m != nil {
		return "huawei", m[1]
	}

	// Try Huawei/H3C square brackets: [hostname]
	if m := huaweiBracketPrompt.FindStringSubmatch(trimmed); m != nil {
		return "huawei", m[1]
	}

	// Try Juniper (before Cisco, because Juniper also uses # but with user@host)
	if m := juniperPrompt.FindStringSubmatch(trimmed); m != nil {
		return "juniper", m[1]
	}

	// Try Cisco
	if m := ciscoPrompt.FindStringSubmatch(trimmed); m != nil {
		return "cisco", m[1]
	}

	return "", ""
}

// ClassifyHuaweiCommand classifies a Huawei VRP command string.
func ClassifyHuaweiCommand(cmd string) model.CommandType {
	lower := strings.ToLower(strings.TrimSpace(cmd))

	switch {
	case strings.HasPrefix(lower, "display ip routing-table"):
		return model.CmdRIB
	case strings.HasPrefix(lower, "display fib"):
		return model.CmdFIB
	case strings.HasPrefix(lower, "display mpls lsp"):
		return model.CmdLFIB
	case strings.HasPrefix(lower, "display mpls forwarding"):
		return model.CmdLFIB
	case strings.HasPrefix(lower, "display interface"):
		return model.CmdInterface
	case strings.HasPrefix(lower, "display ospf peer"),
		strings.HasPrefix(lower, "display bgp peer"),
		strings.HasPrefix(lower, "display isis peer"),
		strings.HasPrefix(lower, "display mpls ldp session"),
		strings.HasPrefix(lower, "display mpls ldp peer"),
		strings.HasPrefix(lower, "display rsvp session"),
		strings.HasPrefix(lower, "display lldp neighbor"):
		return model.CmdNeighbor
	case strings.HasPrefix(lower, "display mpls te tunnel"):
		return model.CmdTunnel
	case strings.HasPrefix(lower, "display segment-routing"),
		strings.HasPrefix(lower, "display isis segment-routing"):
		return model.CmdSRMapping
	case strings.HasPrefix(lower, "display current-configuration"),
		strings.HasPrefix(lower, "display saved-configuration"):
		return model.CmdConfig
	default:
		return model.CmdUnknown
	}
}
```

- [ ] **Step 4: Run test — verify PASS**

Run: `go test ./internal/parser/ -v -run "TestDetect|TestClassify"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/parser/detector.go internal/parser/detector_test.go
git commit -m "feat: add vendor detection and Huawei command classification"
```

---

### Task 4: Huawei Parser — Interface + Scaffold

**Files:**
- Create: `internal/parser/huawei/huawei.go`
- Create: `internal/parser/huawei/interface_brief.go`
- Create: `internal/parser/huawei/huawei_test.go`

- [ ] **Step 1: Write test**

```go
// internal/parser/huawei/huawei_test.go
package huawei

import (
	"testing"

	"github.com/xavierli/nethelper/internal/model"
)

func TestHuaweiVendor(t *testing.T) {
	p := New()
	if p.Vendor() != "huawei" {
		t.Errorf("expected huawei, got %s", p.Vendor())
	}
}

func TestHuaweiDetectPrompt(t *testing.T) {
	p := New()

	tests := []struct {
		line     string
		wantHost string
		wantOK   bool
	}{
		{"<HUAWEI-Core-01>display version", "HUAWEI-Core-01", true},
		{"[HUAWEI-Core-01]interface GE0/0/1", "HUAWEI-Core-01", true},
		{"Router-PE01#show version", "", false},
		{"some random text", "", false},
	}

	for _, tt := range tests {
		host, ok := p.DetectPrompt(tt.line)
		if ok != tt.wantOK || host != tt.wantHost {
			t.Errorf("line %q: got (%q, %v), want (%q, %v)", tt.line, host, ok, tt.wantHost, tt.wantOK)
		}
	}
}

func TestParseInterfaceBrief(t *testing.T) {
	input := `Interface                   PHY   Protocol InUti OutUti   inErr  outErr
GE0/0/1                     up    up       0.5%  0.3%         0       0
GE0/0/2                     down  down     0%    0%           0       0
LoopBack0                   up    up(s)    --    --           0       0
Eth-Trunk1                  up    up       1.2%  0.8%         0       0
Vlanif100                   up    up       --    --           0       0
NULL0                       up    up(s)    --    --           0       0`

	result, err := ParseInterfaceBrief(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Interfaces) != 6 {
		t.Fatalf("expected 6 interfaces, got %d", len(result.Interfaces))
	}

	// Check GE0/0/1
	ge1 := result.Interfaces[0]
	if ge1.Name != "GE0/0/1" {
		t.Errorf("expected GE0/0/1, got %s", ge1.Name)
	}
	if ge1.Status != "up" {
		t.Errorf("expected up, got %s", ge1.Status)
	}
	if ge1.Type != model.IfTypePhysical {
		t.Errorf("expected physical, got %s", ge1.Type)
	}

	// Check GE0/0/2 is down
	ge2 := result.Interfaces[1]
	if ge2.Status != "down" {
		t.Errorf("expected down, got %s", ge2.Status)
	}

	// Check LoopBack0
	lo := result.Interfaces[2]
	if lo.Type != model.IfTypeLoopback {
		t.Errorf("expected loopback, got %s", lo.Type)
	}

	// Check Eth-Trunk1
	trunk := result.Interfaces[3]
	if trunk.Type != model.IfTypeEthTrunk {
		t.Errorf("expected eth-trunk, got %s", trunk.Type)
	}

	// Check Vlanif100
	vlan := result.Interfaces[4]
	if vlan.Type != model.IfTypeVlanif {
		t.Errorf("expected vlanif, got %s", vlan.Type)
	}

	// Check NULL0
	null0 := result.Interfaces[5]
	if null0.Type != model.IfTypeNull {
		t.Errorf("expected null, got %s", null0.Type)
	}
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/parser/huawei/ -v`
Expected: FAIL

- [ ] **Step 3: Implement huawei.go (scaffold)**

```go
// internal/parser/huawei/huawei.go
package huawei

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/xavierli/nethelper/internal/model"
)

var (
	anglePrompt  = regexp.MustCompile(`^<([^>]+)>`)
	bracketPrompt = regexp.MustCompile(`^\[([^\]]+)\]`)
)

// Parser implements parser.VendorParser for Huawei VRP devices.
type Parser struct{}

func New() *Parser {
	return &Parser{}
}

func (p *Parser) Vendor() string {
	return "huawei"
}

func (p *Parser) DetectPrompt(line string) (string, bool) {
	trimmed := strings.TrimRight(line, "\r \t")
	if m := anglePrompt.FindStringSubmatch(trimmed); m != nil {
		return m[1], true
	}
	if m := bracketPrompt.FindStringSubmatch(trimmed); m != nil {
		return m[1], true
	}
	return "", false
}

func (p *Parser) ClassifyCommand(cmd string) model.CommandType {
	lower := strings.ToLower(strings.TrimSpace(cmd))

	switch {
	case strings.HasPrefix(lower, "display ip routing-table"):
		return model.CmdRIB
	case strings.HasPrefix(lower, "display fib"):
		return model.CmdFIB
	case strings.HasPrefix(lower, "display mpls lsp"),
		strings.HasPrefix(lower, "display mpls forwarding"):
		return model.CmdLFIB
	case strings.HasPrefix(lower, "display interface"):
		return model.CmdInterface
	case strings.HasPrefix(lower, "display ospf peer"),
		strings.HasPrefix(lower, "display bgp peer"),
		strings.HasPrefix(lower, "display isis peer"),
		strings.HasPrefix(lower, "display mpls ldp session"),
		strings.HasPrefix(lower, "display mpls ldp peer"),
		strings.HasPrefix(lower, "display rsvp session"),
		strings.HasPrefix(lower, "display lldp neighbor"):
		return model.CmdNeighbor
	case strings.HasPrefix(lower, "display mpls te tunnel"):
		return model.CmdTunnel
	case strings.HasPrefix(lower, "display segment-routing"),
		strings.HasPrefix(lower, "display isis segment-routing"):
		return model.CmdSRMapping
	case strings.HasPrefix(lower, "display current-configuration"),
		strings.HasPrefix(lower, "display saved-configuration"):
		return model.CmdConfig
	default:
		return model.CmdUnknown
	}
}

func (p *Parser) ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error) {
	switch cmdType {
	case model.CmdInterface:
		return ParseInterfaceBrief(raw)
	case model.CmdRIB:
		return ParseRoutingTable(raw)
	case model.CmdNeighbor:
		return ParseNeighbor(raw)
	case model.CmdLFIB:
		return ParseMplsLsp(raw)
	default:
		// L3 fallback: store raw text
		return model.ParseResult{Type: cmdType, RawText: raw}, nil
	}
}

// inferInterfaceType guesses the interface type from its name.
func inferInterfaceType(name string) model.InterfaceType {
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(lower, "loopback") || strings.HasPrefix(lower, "lo"):
		return model.IfTypeLoopback
	case strings.HasPrefix(lower, "vlanif"):
		return model.IfTypeVlanif
	case strings.HasPrefix(lower, "eth-trunk"):
		return model.IfTypeEthTrunk
	case strings.HasPrefix(lower, "tunnel") && strings.Contains(lower, "te"):
		return model.IfTypeTunnelTE
	case strings.HasPrefix(lower, "tunnel"):
		return model.IfTypeTunnelGRE
	case strings.HasPrefix(lower, "nve"):
		return model.IfTypeNVE
	case strings.HasPrefix(lower, "null"):
		return model.IfTypeNull
	case strings.Contains(lower, "."):
		return model.IfTypeSubInterface
	default:
		return model.IfTypePhysical
	}
}

// ParseRoutingTable is a placeholder for Task 5.
func ParseRoutingTable(raw string) (model.ParseResult, error) {
	return model.ParseResult{Type: model.CmdRIB, RawText: raw}, fmt.Errorf("not implemented")
}

// ParseNeighbor is a placeholder for Task 6.
func ParseNeighbor(raw string) (model.ParseResult, error) {
	return model.ParseResult{Type: model.CmdNeighbor, RawText: raw}, fmt.Errorf("not implemented")
}

// ParseMplsLsp is a placeholder for Task 7.
func ParseMplsLsp(raw string) (model.ParseResult, error) {
	return model.ParseResult{Type: model.CmdLFIB, RawText: raw}, fmt.Errorf("not implemented")
}
```

- [ ] **Step 4: Implement interface_brief.go**

```go
// internal/parser/huawei/interface_brief.go
package huawei

import (
	"strings"

	"github.com/xavierli/nethelper/internal/model"
)

// ParseInterfaceBrief parses "display interface brief" output.
// Example format:
//
//	Interface                   PHY   Protocol InUti OutUti   inErr  outErr
//	GE0/0/1                     up    up       0.5%  0.3%         0       0
func ParseInterfaceBrief(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdInterface, RawText: raw}

	lines := strings.Split(raw, "\n")
	headerFound := false

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" {
			continue
		}

		// Detect header line
		if !headerFound {
			if strings.Contains(trimmed, "PHY") && strings.Contains(trimmed, "Protocol") {
				headerFound = true
			}
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) < 3 {
			continue
		}

		name := fields[0]
		phyStatus := strings.ToLower(fields[1])
		// Normalize status: "*down" → "admin-down"
		if strings.HasPrefix(phyStatus, "*") {
			phyStatus = "admin-down"
		}

		iface := model.Interface{
			Name:   name,
			Type:   inferInterfaceType(name),
			Status: phyStatus,
		}

		result.Interfaces = append(result.Interfaces, iface)
	}

	return result, nil
}
```

- [ ] **Step 5: Run test — verify PASS**

Run: `go test ./internal/parser/huawei/ -v`
Expected: PASS (interface tests pass; routing/neighbor/lsp placeholders not tested yet)

- [ ] **Step 6: Commit**

```bash
git add internal/parser/huawei/
git commit -m "feat: add Huawei parser scaffold with interface brief parsing"
```

---

### Task 5: Huawei Parser — Routing Table (RIB)

**Files:**
- Modify: `internal/parser/huawei/huawei.go` (remove placeholder)
- Create: `internal/parser/huawei/routing_table.go`
- Modify: `internal/parser/huawei/huawei_test.go` (add test)

- [ ] **Step 1: Add test to huawei_test.go**

Append to `internal/parser/huawei/huawei_test.go`:

```go
func TestParseRoutingTable(t *testing.T) {
	input := `Route Flags: R - relay, D - download to fib
------------------------------------------------------------------------------
Routing Tables: Public
         Destinations : 5        Routes : 6

Destination/Mask    Proto   Pre  Cost  Flags NextHop         Interface
10.1.1.0/24         OSPF    10   2           10.0.0.1        GE0/0/1
10.2.0.0/16         Static  60   0     RD    10.0.0.2        GE0/0/2
172.16.0.0/24       Direct  0    0     D     172.16.0.1      LoopBack0
0.0.0.0/0           Static  60   0     RD    10.0.0.254      GE0/0/1
192.168.1.0/24      BGP     255  0           10.0.0.3        GE0/0/3`

	result, err := ParseRoutingTable(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.RIBEntries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(result.RIBEntries))
	}

	// Check first entry
	e := result.RIBEntries[0]
	if e.Prefix != "10.1.1.0" || e.MaskLen != 24 {
		t.Errorf("entry 0: expected 10.1.1.0/24, got %s/%d", e.Prefix, e.MaskLen)
	}
	if e.Protocol != "ospf" {
		t.Errorf("entry 0: expected ospf, got %s", e.Protocol)
	}
	if e.Preference != 10 {
		t.Errorf("entry 0: expected pref 10, got %d", e.Preference)
	}
	if e.Metric != 2 {
		t.Errorf("entry 0: expected metric 2, got %d", e.Metric)
	}
	if e.NextHop != "10.0.0.1" {
		t.Errorf("entry 0: expected next-hop 10.0.0.1, got %s", e.NextHop)
	}

	// Check default route
	def := result.RIBEntries[3]
	if def.Prefix != "0.0.0.0" || def.MaskLen != 0 {
		t.Errorf("entry 3: expected 0.0.0.0/0, got %s/%d", def.Prefix, def.MaskLen)
	}
}

func TestParseRoutingTableVPN(t *testing.T) {
	input := `Route Flags: R - relay, D - download to fib
------------------------------------------------------------------------------
Routing Tables: VPN1
         Destinations : 2        Routes : 2

Destination/Mask    Proto   Pre  Cost  Flags NextHop         Interface
10.100.0.0/24       OSPF    10   3           10.100.0.1      GE0/0/5
10.200.0.0/16       BGP     255  0           10.100.0.2      GE0/0/5`

	result, err := ParseRoutingTable(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.RIBEntries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result.RIBEntries))
	}

	// Verify VRF name is extracted
	for _, e := range result.RIBEntries {
		if e.VRF != "VPN1" {
			t.Errorf("expected VRF 'VPN1', got %q", e.VRF)
		}
	}
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/parser/huawei/ -v -run TestParseRoutingTable`
Expected: FAIL (placeholder returns error)

- [ ] **Step 3: Implement routing_table.go**

```go
// internal/parser/huawei/routing_table.go
package huawei

import (
	"strconv"
	"strings"

	"github.com/xavierli/nethelper/internal/model"
)

// ParseRoutingTable parses "display ip routing-table" output.
// Example format:
//
//	Destination/Mask    Proto   Pre  Cost  Flags NextHop         Interface
//	10.1.1.0/24         OSPF    10   2           10.0.0.1        GE0/0/1
func ParseRoutingTable(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdRIB, RawText: raw}

	lines := strings.Split(raw, "\n")
	headerFound := false
	vrf := "default"

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" {
			continue
		}

		// Extract VRF from "Routing Tables: VPN1" (anything other than "Public" = VPN instance)
		if strings.HasPrefix(strings.TrimSpace(trimmed), "Routing Tables:") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				vrfName := strings.TrimSpace(parts[1])
				if vrfName != "" && !strings.EqualFold(vrfName, "Public") {
					vrf = vrfName
				}
			}
			continue
		}

		// Detect the header line
		if !headerFound {
			if strings.Contains(trimmed, "Destination/Mask") && strings.Contains(trimmed, "Proto") {
				headerFound = true
			}
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) < 6 {
			continue
		}

		// Parse destination/mask
		prefix, maskLen, ok := parsePrefixMask(fields[0])
		if !ok {
			continue
		}

		proto := strings.ToLower(fields[1])
		pref, _ := strconv.Atoi(fields[2])
		cost, _ := strconv.Atoi(fields[3])

		// Fields after cost may include Flags (optional letters) then NextHop then Interface
		// Strategy: NextHop is the first IP-like field after cost, Interface is the last field
		var nextHop, outIface string
		remaining := fields[4:]

		for i, f := range remaining {
			if isIPLike(f) {
				nextHop = f
				if i+1 < len(remaining) {
					outIface = remaining[i+1]
				}
				break
			}
		}

		// If no IP found, last two fields might be nextHop and interface
		if nextHop == "" && len(remaining) >= 2 {
			nextHop = remaining[len(remaining)-2]
			outIface = remaining[len(remaining)-1]
		} else if nextHop == "" && len(remaining) >= 1 {
			outIface = remaining[len(remaining)-1]
		}

		entry := model.RIBEntry{
			Prefix:            prefix,
			MaskLen:           maskLen,
			Protocol:          proto,
			Preference:        pref,
			Metric:            cost,
			NextHop:           nextHop,
			OutgoingInterface: outIface,
			VRF:               vrf,
		}

		result.RIBEntries = append(result.RIBEntries, entry)
	}

	return result, nil
}

// parsePrefixMask splits "10.1.1.0/24" into ("10.1.1.0", 24, true).
func parsePrefixMask(s string) (string, int, bool) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return "", 0, false
	}
	maskLen, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, false
	}
	return parts[0], maskLen, true
}

// isIPLike checks if a string looks like an IP address.
func isIPLike(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 255 {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4: Remove placeholder from huawei.go**

In `internal/parser/huawei/huawei.go`, replace the placeholder:

```go
// Remove this:
// ParseRoutingTable is a placeholder for Task 5.
func ParseRoutingTable(raw string) (model.ParseResult, error) {
	return model.ParseResult{Type: model.CmdRIB, RawText: raw}, fmt.Errorf("not implemented")
}
```

Also remove the `"fmt"` import if it becomes unused.

- [ ] **Step 5: Run test — verify PASS**

Run: `go test ./internal/parser/huawei/ -v -run TestParseRoutingTable`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/parser/huawei/
git commit -m "feat: add Huawei routing table (RIB) parser"
```

---

### Task 6: Huawei Parser — OSPF Peer + LDP Session (Neighbors)

**Files:**
- Modify: `internal/parser/huawei/huawei.go` (remove placeholder)
- Create: `internal/parser/huawei/ospf_peer.go`
- Create: `internal/parser/huawei/ldp_session.go`
- Modify: `internal/parser/huawei/huawei_test.go` (add tests)

- [ ] **Step 1: Add tests to huawei_test.go**

Append:

```go
func TestParseOspfPeer(t *testing.T) {
	input := `OSPF Process 1 with Router ID 10.0.0.1
                 Peer Statistic Information
 ----------------------------------------------------------------------------
 Area Id          Interface                        Neighbor id      State
 0.0.0.0          GE0/0/1                          10.0.0.2         Full
 0.0.0.0          GE0/0/2                          10.0.0.3         Full
 0.0.0.1          GE0/0/3                          10.0.0.4         Loading`

	result, err := parseOspfPeer(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Neighbors) != 3 {
		t.Fatalf("expected 3, got %d", len(result.Neighbors))
	}

	n := result.Neighbors[0]
	if n.Protocol != "ospf" {
		t.Errorf("expected ospf, got %s", n.Protocol)
	}
	if n.RemoteID != "10.0.0.2" {
		t.Errorf("expected 10.0.0.2, got %s", n.RemoteID)
	}
	if n.State != "full" {
		t.Errorf("expected full, got %s", n.State)
	}
	if n.AreaID != "0.0.0.0" {
		t.Errorf("expected 0.0.0.0, got %s", n.AreaID)
	}
	if n.LocalInterface != "GE0/0/1" {
		t.Errorf("expected GE0/0/1, got %s", n.LocalInterface)
	}
}

func TestParseLdpSession(t *testing.T) {
	input := ` LDP Session(s) with peer: Total 2
 ---------------------------------------------------------------------------
 Peer LDP ID         State       GR  KA(Sent/Rcvd) Up Time
 10.0.0.2:0          Operational N   30/30          5d:12h:30m
 10.0.0.3:0          Operational N   30/30          3d:06h:15m`

	result, err := parseLdpSession(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Neighbors) != 2 {
		t.Fatalf("expected 2, got %d", len(result.Neighbors))
	}

	n := result.Neighbors[0]
	if n.Protocol != "ldp" {
		t.Errorf("expected ldp, got %s", n.Protocol)
	}
	if n.RemoteID != "10.0.0.2" {
		t.Errorf("expected 10.0.0.2, got %s", n.RemoteID)
	}
	if n.State != "operational" {
		t.Errorf("expected operational, got %s", n.State)
	}
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/parser/huawei/ -v -run "TestParseOspfPeer|TestParseLdpSession"`
Expected: FAIL

- [ ] **Step 3: Implement ospf_peer.go**

```go
// internal/parser/huawei/ospf_peer.go
package huawei

import (
	"strings"

	"github.com/xavierli/nethelper/internal/model"
)

// parseOspfPeer parses "display ospf peer" / "display ospf peer brief" output.
func parseOspfPeer(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdNeighbor, RawText: raw}

	lines := strings.Split(raw, "\n")
	headerFound := false

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" {
			continue
		}

		if !headerFound {
			if strings.Contains(trimmed, "Area Id") && strings.Contains(trimmed, "Neighbor id") {
				headerFound = true
			}
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) < 4 {
			continue
		}

		neighbor := model.NeighborInfo{
			Protocol:       "ospf",
			AreaID:         fields[0],
			LocalInterface: fields[1],
			RemoteID:       fields[2],
			State:          strings.ToLower(fields[3]),
		}

		result.Neighbors = append(result.Neighbors, neighbor)
	}

	return result, nil
}
```

- [ ] **Step 4: Implement ldp_session.go**

```go
// internal/parser/huawei/ldp_session.go
package huawei

import (
	"strings"

	"github.com/xavierli/nethelper/internal/model"
)

// parseLdpSession parses "display mpls ldp session" output.
func parseLdpSession(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdNeighbor, RawText: raw}

	lines := strings.Split(raw, "\n")
	headerFound := false

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" {
			continue
		}

		if !headerFound {
			if strings.Contains(trimmed, "Peer LDP ID") && strings.Contains(trimmed, "State") {
				headerFound = true
			}
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}

		// Peer LDP ID format: "10.0.0.2:0"
		peerID := fields[0]
		remoteID := strings.Split(peerID, ":")[0]
		state := strings.ToLower(fields[1])

		var uptime string
		if len(fields) >= 5 {
			uptime = fields[4]
		}

		neighbor := model.NeighborInfo{
			Protocol:  "ldp",
			RemoteID:  remoteID,
			State:     state,
			Uptime:    uptime,
		}

		result.Neighbors = append(result.Neighbors, neighbor)
	}

	return result, nil
}
```

- [ ] **Step 5: Update ParseNeighbor in huawei.go**

Replace the placeholder `ParseNeighbor` function in `huawei.go`:

```go
// ParseNeighbor routes to the appropriate neighbor parser based on content.
func ParseNeighbor(raw string) (model.ParseResult, error) {
	lower := strings.ToLower(raw)

	switch {
	case strings.Contains(lower, "ospf") && strings.Contains(lower, "area id"):
		return parseOspfPeer(raw)
	case strings.Contains(lower, "peer ldp id"):
		return parseLdpSession(raw)
	default:
		// L3 fallback
		return model.ParseResult{Type: model.CmdNeighbor, RawText: raw}, nil
	}
}
```

Remove the old placeholder.

- [ ] **Step 6: Run test — verify PASS**

Run: `go test ./internal/parser/huawei/ -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/parser/huawei/
git commit -m "feat: add Huawei OSPF peer and LDP session parsers"
```

---

### Task 7: Huawei Parser — MPLS LSP (LFIB)

**Files:**
- Modify: `internal/parser/huawei/huawei.go` (remove placeholder)
- Create: `internal/parser/huawei/mpls_lsp.go`
- Modify: `internal/parser/huawei/huawei_test.go` (add test)

- [ ] **Step 1: Add test**

Append to `huawei_test.go`:

```go
func TestParseMplsLsp(t *testing.T) {
	input := ` -------------------------------------------------------------------------------
                 LSP Information: LDP LSP
 -------------------------------------------------------------------------------
 FEC                In/Out Label  In/Out IF                      Vrf Name
 10.0.0.2/32        3/1024        -/GE0/0/1
 10.0.0.3/32        1025/1026     GE0/0/2/GE0/0/1
 10.0.0.4/32        1027/3        GE0/0/3/-                      `

	result, err := ParseMplsLsp(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.LFIBEntries) != 3 {
		t.Fatalf("expected 3, got %d", len(result.LFIBEntries))
	}

	// First: push (in=3 means implicit-null → this is egress, actually in=3 is a real label)
	e0 := result.LFIBEntries[0]
	if e0.FEC != "10.0.0.2/32" {
		t.Errorf("expected FEC 10.0.0.2/32, got %s", e0.FEC)
	}
	if e0.InLabel != 3 {
		t.Errorf("expected in_label 3, got %d", e0.InLabel)
	}
	if e0.Protocol != "ldp" {
		t.Errorf("expected ldp, got %s", e0.Protocol)
	}

	// Second: swap
	e1 := result.LFIBEntries[1]
	if e1.InLabel != 1025 || e1.OutLabel != "1026" {
		t.Errorf("expected 1025→1026, got %d→%s", e1.InLabel, e1.OutLabel)
	}
	if e1.Action != "swap" {
		t.Errorf("expected swap, got %s", e1.Action)
	}

	// Third: pop (out=3 means implicit-null)
	e2 := result.LFIBEntries[2]
	if e2.InLabel != 1027 {
		t.Errorf("expected 1027, got %d", e2.InLabel)
	}
	if e2.Action != "pop" {
		t.Errorf("expected pop, got %s", e2.Action)
	}
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/parser/huawei/ -v -run TestParseMplsLsp`
Expected: FAIL

- [ ] **Step 3: Implement mpls_lsp.go**

```go
// internal/parser/huawei/mpls_lsp.go
package huawei

import (
	"strconv"
	"strings"

	"github.com/xavierli/nethelper/internal/model"
)

// ParseMplsLsp parses "display mpls lsp" output.
// Format:
//
//	FEC                In/Out Label  In/Out IF                      Vrf Name
//	10.0.0.2/32        3/1024        -/GE0/0/1
func ParseMplsLsp(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdLFIB, RawText: raw}

	lines := strings.Split(raw, "\n")
	headerFound := false
	protocol := "ldp" // default, may be overridden by section headers

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" {
			continue
		}

		// Detect protocol from section headers
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "ldp lsp") {
			protocol = "ldp"
			continue
		}
		if strings.Contains(lower, "rsvp lsp") || strings.Contains(lower, "te lsp") {
			protocol = "rsvp"
			continue
		}
		if strings.Contains(lower, "sr lsp") || strings.Contains(lower, "segment-routing") {
			protocol = "sr"
			continue
		}

		if !headerFound {
			if strings.Contains(trimmed, "FEC") && strings.Contains(trimmed, "In/Out Label") {
				headerFound = true
			}
			continue
		}

		// Skip separator lines
		if strings.HasPrefix(trimmed, " ---") || strings.HasPrefix(trimmed, "---") {
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}

		fec := fields[0]
		labelPair := fields[1] // "3/1024"

		labels := strings.SplitN(labelPair, "/", 2)
		if len(labels) != 2 {
			continue
		}

		inLabel, err := strconv.Atoi(labels[0])
		if err != nil {
			continue
		}
		outLabelStr := labels[1]

		// Parse interfaces if present
		var outInterface string
		if len(fields) >= 3 {
			ifPair := fields[2] // "-/GE0/0/1" or "GE0/0/2/GE0/0/1"
			parts := strings.SplitN(ifPair, "/", 2)
			// For out interface, try the last meaningful part
			if len(parts) == 2 && parts[1] != "-" && parts[1] != "" {
				outInterface = parts[1]
			}
			// Handle complex cases like "GE0/0/2/GE0/0/1"
			// The interface field uses / as both separator and part of interface names
			// Heuristic: if the full field starts with "-/", take everything after "-/"
			if strings.HasPrefix(ifPair, "-/") {
				outInterface = ifPair[2:]
			} else if strings.HasSuffix(ifPair, "/-") {
				// inIF/- → no out interface
				outInterface = ""
			}
		}

		// Determine action
		outLabelNum, outErr := strconv.Atoi(outLabelStr)
		action := determineLabelAction(inLabel, outLabelStr, outLabelNum, outErr)

		entry := model.LFIBEntry{
			InLabel:           inLabel,
			Action:            action,
			OutLabel:          outLabelStr,
			OutgoingInterface: outInterface,
			FEC:               fec,
			Protocol:          protocol,
		}

		result.LFIBEntries = append(result.LFIBEntries, entry)
	}

	return result, nil
}

// determineLabelAction infers the MPLS action from label values.
// Label 0 = explicit-null (IPv4), 3 = implicit-null (PHP).
func determineLabelAction(inLabel int, outLabelStr string, outLabelNum int, outErr error) string {
	switch {
	case outLabelStr == "-" || outLabelStr == "":
		return "pop"
	case outErr == nil && outLabelNum == 3:
		return "pop" // implicit-null → PHP
	case outErr == nil && outLabelNum == 0:
		return "pop" // explicit-null
	case inLabel == 3:
		// incoming implicit-null with a real out label = push
		if outErr == nil && outLabelNum > 3 {
			return "push"
		}
		return "pop"
	default:
		return "swap"
	}
}
```

- [ ] **Step 4: Remove placeholder from huawei.go**

Remove the `ParseMplsLsp` placeholder function and update the `"fmt"` import if unused.

- [ ] **Step 5: Run test — verify PASS**

Run: `go test ./internal/parser/huawei/ -v`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/parser/huawei/
git commit -m "feat: add Huawei MPLS LSP (LFIB) parser"
```

---

### Task 8: Pipeline — Orchestrate Parse + Store

**Files:**
- Create: `internal/parser/pipeline.go`
- Create: `internal/parser/pipeline_test.go`
- Modify: `internal/cli/watch.go` (wire ingest to pipeline)

- [ ] **Step 1: Write pipeline test**

```go
// internal/parser/pipeline_test.go
package parser

import (
	"path/filepath"
	"testing"

	"github.com/xavierli/nethelper/internal/parser/huawei"
	"github.com/xavierli/nethelper/internal/store"
)

func TestPipelineIngestFile(t *testing.T) {
	// Create test DB
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// Create registry with Huawei parser
	registry := NewRegistry()
	registry.Register(huawei.New())

	pipeline := NewPipeline(db, registry)

	// Simulate a log file content
	content := `<Core-SW01>display interface brief
Interface                   PHY   Protocol InUti OutUti   inErr  outErr
GE0/0/1                     up    up       0.5%  0.3%         0       0
GE0/0/2                     down  down     0%    0%           0       0
<Core-SW01>display ip routing-table
Route Flags: R - relay, D - download to fib
------------------------------------------------------------------------------
Routing Tables: Public
         Destinations : 2        Routes : 2

Destination/Mask    Proto   Pre  Cost  Flags NextHop         Interface
10.1.1.0/24         OSPF    10   2           10.0.0.1        GE0/0/1
172.16.0.0/16       Static  60   0     RD    10.0.0.2        GE0/0/2
<Core-SW01>display ospf peer
OSPF Process 1 with Router ID 10.0.0.1
                 Peer Statistic Information
 ----------------------------------------------------------------------------
 Area Id          Interface                        Neighbor id      State
 0.0.0.0          GE0/0/1                          10.0.0.2         Full
`

	result, err := pipeline.Ingest("test-log.txt", content)
	if err != nil {
		t.Fatalf("ingest error: %v", err)
	}

	if result.DevicesFound != 1 {
		t.Errorf("expected 1 device, got %d", result.DevicesFound)
	}
	if result.BlocksParsed != 3 {
		t.Errorf("expected 3 blocks parsed, got %d", result.BlocksParsed)
	}

	// Verify device was created in store
	dev, err := db.GetDevice("core-sw01")
	if err != nil {
		t.Fatalf("device not found: %v", err)
	}
	if dev.Hostname != "Core-SW01" {
		t.Errorf("expected Core-SW01, got %s", dev.Hostname)
	}
	if dev.Vendor != "huawei" {
		t.Errorf("expected huawei, got %s", dev.Vendor)
	}

	// Verify interfaces were stored
	ifaces, err := db.GetInterfaces("core-sw01")
	if err != nil {
		t.Fatalf("get interfaces: %v", err)
	}
	if len(ifaces) != 2 {
		t.Errorf("expected 2 interfaces, got %d", len(ifaces))
	}

	// Verify RIB entries
	snapID, err := db.LatestSnapshotID("core-sw01")
	if err != nil {
		t.Fatalf("no snapshot: %v", err)
	}
	ribs, err := db.GetRIBEntries("core-sw01", snapID)
	if err != nil {
		t.Fatalf("get rib: %v", err)
	}
	if len(ribs) != 2 {
		t.Errorf("expected 2 RIB entries, got %d", len(ribs))
	}

	// Verify neighbors
	neighbors, err := db.GetNeighbors("core-sw01", snapID)
	if err != nil {
		t.Fatalf("get neighbors: %v", err)
	}
	if len(neighbors) != 1 {
		t.Errorf("expected 1 neighbor, got %d", len(neighbors))
	}
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/parser/ -v -run TestPipeline`
Expected: FAIL

- [ ] **Step 3: Implement pipeline.go**

```go
// internal/parser/pipeline.go
package parser

import (
	"log/slog"
	"strings"
	"time"

	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/store"
)

// IngestResult summarizes what happened during ingestion.
type IngestResult struct {
	DevicesFound  int
	BlocksParsed  int
	BlocksFailed  int
	BlocksSkipped int
}

// Pipeline orchestrates: split → detect → parse → store.
type Pipeline struct {
	db       *store.DB
	registry *Registry
}

func NewPipeline(db *store.DB, registry *Registry) *Pipeline {
	return &Pipeline{db: db, registry: registry}
}

// Ingest processes raw log content from a file.
func (p *Pipeline) Ingest(sourceFile, content string) (IngestResult, error) {
	var result IngestResult

	blocks := Split(content, p.registry)
	if len(blocks) == 0 {
		return result, nil
	}

	// Group blocks by hostname
	type deviceBlocks struct {
		hostname string
		vendor   string
		blocks   []CommandBlock
	}
	deviceMap := make(map[string]*deviceBlocks)

	for i := range blocks {
		b := &blocks[i]
		// Classify command using the vendor parser
		if vp, ok := p.registry.Get(b.Vendor); ok {
			b.CmdType = vp.ClassifyCommand(b.Command)
		} else {
			b.CmdType = model.CmdUnknown
		}

		key := strings.ToLower(b.Hostname)
		if _, exists := deviceMap[key]; !exists {
			deviceMap[key] = &deviceBlocks{hostname: b.Hostname, vendor: b.Vendor}
		}
		deviceMap[key].blocks = append(deviceMap[key].blocks, *b)
	}

	result.DevicesFound = len(deviceMap)

	for deviceID, db := range deviceMap {
		// Upsert device
		dev := model.Device{
			ID:       deviceID,
			Hostname: db.hostname,
			Vendor:   db.vendor,
			LastSeen: time.Now(),
		}
		if err := p.db.UpsertDevice(dev); err != nil {
			slog.Error("upsert device failed", "device", deviceID, "error", err)
			continue
		}

		// Create snapshot
		var cmdNames []string
		for _, b := range db.blocks {
			cmdNames = append(cmdNames, b.Command)
		}
		snapshot := model.Snapshot{
			DeviceID:   deviceID,
			SourceFile: sourceFile,
			Commands:   `["` + strings.Join(cmdNames, `","`) + `"]`,
		}
		snapID, err := p.db.CreateSnapshot(snapshot)
		if err != nil {
			slog.Error("create snapshot failed", "device", deviceID, "error", err)
			continue
		}

		// Parse and store each block
		for _, b := range db.blocks {
			vp, ok := p.registry.Get(b.Vendor)
			if !ok {
				result.BlocksSkipped++
				continue
			}

			parseResult, err := vp.ParseOutput(b.CmdType, b.Output)
			if err != nil {
				slog.Warn("parse failed, storing raw", "cmd", b.Command, "error", err)
				result.BlocksFailed++
				continue
			}

			if err := p.storeResult(deviceID, snapID, parseResult); err != nil {
				slog.Error("store result failed", "cmd", b.Command, "error", err)
				result.BlocksFailed++
				continue
			}

			result.BlocksParsed++
		}
	}

	return result, nil
}

// storeResult persists a ParseResult into the appropriate store tables.
func (p *Pipeline) storeResult(deviceID string, snapID int, pr model.ParseResult) error {
	// Store interfaces (no snapshot needed — they're upserted)
	for i := range pr.Interfaces {
		iface := &pr.Interfaces[i]
		iface.DeviceID = deviceID
		if iface.ID == "" {
			iface.ID = deviceID + ":" + iface.Name
		}
		iface.LastUpdated = time.Now()
		if err := p.db.UpsertInterface(*iface); err != nil {
			return err
		}
	}

	// Store RIB entries
	if len(pr.RIBEntries) > 0 {
		for i := range pr.RIBEntries {
			pr.RIBEntries[i].DeviceID = deviceID
			pr.RIBEntries[i].SnapshotID = snapID
		}
		if err := p.db.InsertRIBEntries(pr.RIBEntries); err != nil {
			return err
		}
	}

	// Store FIB entries
	if len(pr.FIBEntries) > 0 {
		for i := range pr.FIBEntries {
			pr.FIBEntries[i].DeviceID = deviceID
			pr.FIBEntries[i].SnapshotID = snapID
		}
		if err := p.db.InsertFIBEntries(pr.FIBEntries); err != nil {
			return err
		}
	}

	// Store LFIB entries
	if len(pr.LFIBEntries) > 0 {
		for i := range pr.LFIBEntries {
			pr.LFIBEntries[i].DeviceID = deviceID
			pr.LFIBEntries[i].SnapshotID = snapID
		}
		if err := p.db.InsertLFIBEntries(pr.LFIBEntries); err != nil {
			return err
		}
	}

	// Store neighbors
	if len(pr.Neighbors) > 0 {
		for i := range pr.Neighbors {
			pr.Neighbors[i].DeviceID = deviceID
			pr.Neighbors[i].SnapshotID = snapID
		}
		if err := p.db.InsertNeighbors(pr.Neighbors); err != nil {
			return err
		}
	}

	// Store tunnels
	if len(pr.Tunnels) > 0 {
		for i := range pr.Tunnels {
			pr.Tunnels[i].DeviceID = deviceID
			pr.Tunnels[i].SnapshotID = snapID
		}
		if err := p.db.InsertTunnels(pr.Tunnels); err != nil {
			return err
		}
	}

	// Store SR mappings
	if len(pr.SRMappings) > 0 {
		for i := range pr.SRMappings {
			pr.SRMappings[i].DeviceID = deviceID
			pr.SRMappings[i].SnapshotID = snapID
		}
		if err := p.db.InsertSRMappings(pr.SRMappings); err != nil {
			return err
		}
	}

	return nil
}
```

- [ ] **Step 4: Run test — verify PASS**

Run: `go test ./internal/parser/ -v -run TestPipeline`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/parser/pipeline.go internal/parser/pipeline_test.go
git commit -m "feat: add parser pipeline orchestrator with split→detect→parse→store"
```

---

### Task 9: Wire Ingest CLI to Pipeline

**Files:**
- Modify: `internal/cli/watch.go`
- Modify: `internal/cli/root.go` (add pipeline setup)

- [ ] **Step 1: Update root.go to initialize parser registry**

Add to `internal/cli/root.go`, after the `db` variable declaration:

```go
import (
	// add these imports
	"github.com/xavierli/nethelper/internal/parser"
	"github.com/xavierli/nethelper/internal/parser/huawei"
)

var (
	// ... existing vars ...
	pipeline *parser.Pipeline
)
```

In `PersistentPreRunE`, after `db` is opened, add:

```go
		// Initialize parser pipeline
		registry := parser.NewRegistry()
		registry.Register(huawei.New())
		pipeline = parser.NewPipeline(db, registry)
```

- [ ] **Step 2: Update watch.go ingest command**

Replace `newWatchIngestCmd` in `internal/cli/watch.go`:

```go
func newWatchIngestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ingest <file>",
		Short: "Manually import a log file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			data, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("read file: %w", err)
			}

			result, err := pipeline.Ingest(filePath, string(data))
			if err != nil {
				return fmt.Errorf("ingest: %w", err)
			}

			// Record ingestion
			ing := model.LogIngestion{
				FilePath:    filePath,
				LastOffset:  int64(len(data)),
				ProcessedAt: time.Now(),
			}
			if err := db.UpsertIngestion(ing); err != nil {
				return fmt.Errorf("record ingestion: %w", err)
			}

			fmt.Printf("Ingested %s (%d bytes)\n", filePath, len(data))
			fmt.Printf("  Devices: %d, Blocks parsed: %d, Failed: %d, Skipped: %d\n",
				result.DevicesFound, result.BlocksParsed, result.BlocksFailed, result.BlocksSkipped)
			return nil
		},
	}
}
```

- [ ] **Step 3: Build and verify**

Run: `go build ./cmd/nethelper`
Expected: compiles successfully

- [ ] **Step 4: Create a test log file and run ingest**

```bash
cat > /tmp/huawei-test.log << 'EOF'
<Core-SW01>display interface brief
Interface                   PHY   Protocol InUti OutUti   inErr  outErr
GE0/0/1                     up    up       0.5%  0.3%         0       0
GE0/0/2                     down  down     0%    0%           0       0
<Core-SW01>display ip routing-table
Route Flags: R - relay, D - download to fib
------------------------------------------------------------------------------
Routing Tables: Public
         Destinations : 2        Routes : 2

Destination/Mask    Proto   Pre  Cost  Flags NextHop         Interface
10.1.1.0/24         OSPF    10   2           10.0.0.1        GE0/0/1
172.16.0.0/16       Static  60   0     RD    10.0.0.2        GE0/0/2
EOF
```

Run: `./nethelper watch ingest /tmp/huawei-test.log --db /tmp/test-ingest.db`
Expected output:
```
Ingested /tmp/huawei-test.log (XXX bytes)
  Devices: 1, Blocks parsed: 2, Failed: 0, Skipped: 0
```

Run: `./nethelper show device --db /tmp/test-ingest.db`
Expected: shows Core-SW01

Run: `./nethelper show route --device core-sw01 --db /tmp/test-ingest.db`
Expected: shows 2 RIB entries

- [ ] **Step 5: Run full test suite**

Run: `go test ./... -v`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/cli/root.go internal/cli/watch.go
git commit -m "feat: wire ingest CLI to parser pipeline — real parsing now works"
```

- [ ] **Step 7: Clean up**

Run: `rm -f nethelper /tmp/huawei-test.log /tmp/test-ingest.db /tmp/test-ingest.db-wal /tmp/test-ingest.db-shm`
