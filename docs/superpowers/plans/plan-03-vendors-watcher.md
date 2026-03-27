# Nethelper Plan 3: Cisco, H3C, Juniper Parsers + Watcher Daemon

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Cisco IOS, H3C Comware, and Juniper JUNOS parsers (interface brief, routing table, OSPF neighbors, MPLS LFIB), then build the fsnotify-based watcher daemon for automatic log ingestion.

**Architecture:** Each vendor gets its own sub-package under `internal/parser/` following the same pattern as the existing Huawei parser: a struct implementing `VendorParser`, per-command parse functions, and tests with real output samples. The watcher is a standalone package `internal/watcher/` using fsnotify with debouncing, incremental reads, and a PID file for daemon management. All new parsers are registered in `root.go` alongside the existing Huawei parser.

**Tech Stack:** Go 1.24+, regexp (stdlib), fsnotify, existing internal/parser and internal/store packages

**Spec:** `docs/superpowers/specs/2026-03-21-network-helper-design.md` (Sections 4, 8)

**Depends on:** Plan 1 (Core Foundation), Plan 2 (Parser Pipeline + Huawei)

---

## File Structure

```
internal/
├── parser/
│   ├── cisco/
│   │   ├── cisco.go               # CiscoParser implementing VendorParser
│   │   ├── show_ip_route.go       # Parse "show ip route"
│   │   ├── show_interfaces.go     # Parse "show interfaces brief" / "show ip interface brief"
│   │   ├── show_ospf.go           # Parse "show ip ospf neighbor"
│   │   ├── show_mpls.go           # Parse "show mpls forwarding-table"
│   │   └── cisco_test.go          # Tests
│   ├── h3c/
│   │   ├── h3c.go                 # H3CParser implementing VendorParser
│   │   ├── display_route.go       # Parse "display ip routing-table"
│   │   ├── display_interface.go   # Parse "display interface brief"
│   │   ├── display_ospf.go        # Parse "display ospf peer"
│   │   ├── display_mpls.go        # Parse "display mpls lsp"
│   │   └── h3c_test.go            # Tests
│   └── juniper/
│       ├── juniper.go             # JuniperParser implementing VendorParser
│       ├── show_route.go          # Parse "show route"
│       ├── show_interfaces.go     # Parse "show interfaces terse"
│       ├── show_ospf.go           # Parse "show ospf neighbor"
│       ├── show_mpls.go           # Parse "show route table mpls.0"
│       └── juniper_test.go        # Tests
├── watcher/
│   ├── watcher.go                 # Watcher struct: fsnotify + debounce + incremental read
│   ├── daemon.go                  # PID file management, start/stop/status
│   ├── watcher_test.go            # Tests
│   └── daemon_test.go             # Tests
├── cli/
│   ├── root.go                    # Modify: register Cisco/H3C/Juniper parsers
│   └── watch.go                   # Modify: implement start/stop/status with watcher daemon
```

---

### Task 1: Cisco Parser — Interface + Routing Table

**Files:**
- Create: `internal/parser/cisco/cisco.go`
- Create: `internal/parser/cisco/show_interfaces.go`
- Create: `internal/parser/cisco/show_ip_route.go`
- Create: `internal/parser/cisco/cisco_test.go`

- [ ] **Step 1: Write test**

```go
// internal/parser/cisco/cisco_test.go
package cisco

import (
	"testing"
	"github.com/xavierli/nethelper/internal/model"
)

func TestCiscoVendor(t *testing.T) {
	if New().Vendor() != "cisco" { t.Error("expected cisco") }
}

func TestCiscoDetectPrompt(t *testing.T) {
	p := New()
	tests := []struct{ line, wantHost string; wantOK bool }{
		{"Router-PE01#show version", "Router-PE01", true},
		{"Router-PE01(config)#interface GE0/0", "Router-PE01", true},
		{"<HUAWEI>display version", "", false},
	}
	for _, tt := range tests {
		host, ok := p.DetectPrompt(tt.line)
		if ok != tt.wantOK || host != tt.wantHost {
			t.Errorf("line %q: got (%q,%v), want (%q,%v)", tt.line, host, ok, tt.wantHost, tt.wantOK)
		}
	}
}

func TestParseShowIPInterfaceBrief(t *testing.T) {
	input := `Interface              IP-Address      OK? Method Status                Protocol
GigabitEthernet0/0     10.0.0.1    YES manual up                    up
GigabitEthernet0/1     unassigned      YES unset  administratively down down
Loopback0              1.1.1.1    YES manual up                    up
Port-channel1          10.0.0.2  YES manual up                    up`

	result, err := ParseShowIPInterfaceBrief(input)
	if err != nil { t.Fatalf("error: %v", err) }
	if len(result.Interfaces) != 4 { t.Fatalf("expected 4, got %d", len(result.Interfaces)) }

	if result.Interfaces[0].Name != "GigabitEthernet0/0" { t.Errorf("name: %s", result.Interfaces[0].Name) }
	if result.Interfaces[0].Status != "up" { t.Errorf("status: %s", result.Interfaces[0].Status) }
	if result.Interfaces[0].IPAddress != "10.0.0.1" { t.Errorf("ip: %s", result.Interfaces[0].IPAddress) }
	if result.Interfaces[0].Type != model.IfTypePhysical { t.Errorf("type: %s", result.Interfaces[0].Type) }

	if result.Interfaces[1].Status != "admin-down" { t.Errorf("status: %s", result.Interfaces[1].Status) }
	if result.Interfaces[2].Type != model.IfTypeLoopback { t.Errorf("type: %s", result.Interfaces[2].Type) }
	if result.Interfaces[3].Type != model.IfTypeEthTrunk { t.Errorf("type: %s", result.Interfaces[3].Type) }
}

func TestParseShowIPRoute(t *testing.T) {
	input := `Codes: L - local, C - connected, S - static, R - RIP, M - mobile, B - BGP
       D - EIGRP, EX - EIGRP external, O - OSPF, IA - OSPF inter area
Gateway of last resort is 10.0.0.1 to network 0.0.0.0

      10.0.0.0/8 is variably subnetted, 4 subnets, 2 masks
C        10.1.1.0/24 is directly connected, GigabitEthernet0/0
L        10.0.0.1/32 is directly connected, GigabitEthernet0/0
O        172.16.0.0/24 [110/2] via 10.0.0.2, 00:05:30, GigabitEthernet0/1
B        192.168.1.0/16 [20/0] via 10.0.0.3, 01:23:45
S*    0.0.0.0/0 [1/0] via 10.0.0.1`

	result, err := ParseShowIPRoute(input)
	if err != nil { t.Fatalf("error: %v", err) }
	if len(result.RIBEntries) != 5 { t.Fatalf("expected 5, got %d", len(result.RIBEntries)) }

	// Check OSPF entry
	ospf := result.RIBEntries[2]
	if ospf.Prefix != "172.16.0.0" || ospf.MaskLen != 24 { t.Errorf("prefix: %s/%d", ospf.Prefix, ospf.MaskLen) }
	if ospf.Protocol != "ospf" { t.Errorf("proto: %s", ospf.Protocol) }
	if ospf.NextHop != "10.0.0.2" { t.Errorf("nexthop: %s", ospf.NextHop) }

	// Check default route
	def := result.RIBEntries[4]
	if def.Prefix != "0.0.0.0" || def.MaskLen != 0 { t.Errorf("default: %s/%d", def.Prefix, def.MaskLen) }
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/parser/cisco/ -v`
Expected: FAIL

- [ ] **Step 3: Implement cisco.go**

```go
// internal/parser/cisco/cisco.go
package cisco

import (
	"regexp"
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

var promptRe = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9._-]*)(?:\([^)]*\))?#`)

type Parser struct{}
func New() *Parser { return &Parser{} }
func (p *Parser) Vendor() string { return "cisco" }

func (p *Parser) DetectPrompt(line string) (string, bool) {
	m := promptRe.FindStringSubmatch(strings.TrimRight(line, "\r \t"))
	if m == nil { return "", false }
	return m[1], true
}

func (p *Parser) ClassifyCommand(cmd string) model.CommandType {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	switch {
	case strings.HasPrefix(lower, "show ip route"), strings.HasPrefix(lower, "show route"):
		return model.CmdRIB
	case strings.HasPrefix(lower, "show ip cef"):
		return model.CmdFIB
	case strings.HasPrefix(lower, "show mpls forwarding"):
		return model.CmdLFIB
	case strings.HasPrefix(lower, "show interface"), strings.HasPrefix(lower, "show ip interface"):
		return model.CmdInterface
	case strings.HasPrefix(lower, "show ip ospf neighbor"),
		strings.HasPrefix(lower, "show ip bgp summary"),
		strings.HasPrefix(lower, "show bgp summary"),
		strings.HasPrefix(lower, "show isis neighbor"),
		strings.HasPrefix(lower, "show mpls ldp neighbor"),
		strings.HasPrefix(lower, "show lldp neighbor"):
		return model.CmdNeighbor
	case strings.HasPrefix(lower, "show mpls traffic-eng tunnel"):
		return model.CmdTunnel
	case strings.HasPrefix(lower, "show running-config"), strings.HasPrefix(lower, "show startup-config"):
		return model.CmdConfig
	default:
		return model.CmdUnknown
	}
}

func (p *Parser) ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error) {
	switch cmdType {
	case model.CmdInterface: return ParseShowIPInterfaceBrief(raw)
	case model.CmdRIB: return ParseShowIPRoute(raw)
	case model.CmdNeighbor: return ParseShowOSPFNeighbor(raw)
	case model.CmdLFIB: return ParseShowMplsForwarding(raw)
	default: return model.ParseResult{Type: cmdType, RawText: raw}, nil
	}
}

func inferInterfaceType(name string) model.InterfaceType {
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(lower, "loopback"): return model.IfTypeLoopback
	case strings.HasPrefix(lower, "vlan"): return model.IfTypeVlanif
	case strings.HasPrefix(lower, "port-channel"): return model.IfTypeEthTrunk
	case strings.HasPrefix(lower, "tunnel"): return model.IfTypeTunnelGRE
	case strings.HasPrefix(lower, "null"): return model.IfTypeNull
	case strings.Contains(lower, "."): return model.IfTypeSubInterface
	default: return model.IfTypePhysical
	}
}
```

- [ ] **Step 4: Implement show_interfaces.go**

```go
// internal/parser/cisco/show_interfaces.go
package cisco

import (
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

// ParseShowIPInterfaceBrief parses "show ip interface brief" output.
func ParseShowIPInterfaceBrief(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdInterface, RawText: raw}
	lines := strings.Split(raw, "\n")
	headerFound := false

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" { continue }
		if !headerFound {
			if strings.Contains(trimmed, "Interface") && strings.Contains(trimmed, "IP-Address") { headerFound = true }
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 6 { continue }

		name := fields[0]
		ip := fields[1]
		if ip == "unassigned" { ip = "" }
		status := strings.ToLower(fields[4])
		proto := strings.ToLower(fields[5])

		// "administratively down" → "admin-down"
		if status == "administratively" && len(fields) > 5 {
			status = "admin-down"
			// Re-parse: "administratively down down" shifts fields
			if len(fields) >= 7 { proto = strings.ToLower(fields[6]) }
		}

		iface := model.Interface{
			Name: name, Type: inferInterfaceType(name), Status: status, IPAddress: ip,
		}
		result.Interfaces = append(result.Interfaces, iface)
	}
	return result, nil
}
```

- [ ] **Step 5: Implement show_ip_route.go**

```go
// internal/parser/cisco/show_ip_route.go
package cisco

import (
	"regexp"
	"strconv"
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

// Cisco route line patterns:
// C        10.1.1.0/24 is directly connected, GigabitEthernet0/0
// O        172.16.0.0/24 [110/2] via 10.0.0.2, 00:05:30, GigabitEthernet0/1
// S*    0.0.0.0/0 [1/0] via 10.0.0.1
var routeLineRe = regexp.MustCompile(`^\s*([A-Za-z*]+\*?)\s+(\d+\.\d+\.\d+\.\d+/\d+)`)

func ParseShowIPRoute(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdRIB, RawText: raw}
	lines := strings.Split(raw, "\n")

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		m := routeLineRe.FindStringSubmatch(trimmed)
		if m == nil { continue }

		code := strings.TrimSpace(m[1])
		prefixStr := m[2]

		parts := strings.SplitN(prefixStr, "/", 2)
		if len(parts) != 2 { continue }
		prefix := parts[0]
		maskLen, _ := strconv.Atoi(parts[1])

		proto := ciscoCodeToProtocol(code)
		var pref, metric int
		var nextHop, outIface string

		// Extract [pref/metric]
		if idx := strings.Index(trimmed, "["); idx >= 0 {
			end := strings.Index(trimmed[idx:], "]")
			if end > 0 {
				pm := trimmed[idx+1 : idx+end]
				pmParts := strings.SplitN(pm, "/", 2)
				if len(pmParts) == 2 {
					pref, _ = strconv.Atoi(pmParts[0])
					metric, _ = strconv.Atoi(pmParts[1])
				}
			}
		}

		// Extract "via X.X.X.X"
		if idx := strings.Index(strings.ToLower(trimmed), "via "); idx >= 0 {
			rest := trimmed[idx+4:]
			viaFields := strings.FieldsFunc(rest, func(r rune) bool { return r == ',' || r == ' ' })
			if len(viaFields) > 0 { nextHop = viaFields[0] }
		}

		// Extract interface: last comma-separated field, or "directly connected, X"
		commaFields := strings.Split(trimmed, ",")
		last := strings.TrimSpace(commaFields[len(commaFields)-1])
		if isInterfaceName(last) { outIface = last }

		result.RIBEntries = append(result.RIBEntries, model.RIBEntry{
			Prefix: prefix, MaskLen: maskLen, Protocol: proto,
			Preference: pref, Metric: metric, NextHop: nextHop,
			OutgoingInterface: outIface, VRF: "default",
		})
	}
	return result, nil
}

func ciscoCodeToProtocol(code string) string {
	code = strings.TrimRight(code, "*")
	switch code {
	case "C", "L": return "direct"
	case "S": return "static"
	case "O", "IA": return "ospf"
	case "B": return "bgp"
	case "D", "EX": return "eigrp"
	case "R": return "rip"
	case "i", "ia", "L1", "L2": return "isis"
	default: return strings.ToLower(code)
	}
}

func isInterfaceName(s string) bool {
	prefixes := []string{"gi", "fa", "te", "hu", "et", "lo", "vl", "po", "tu", "nu", "se"}
	lower := strings.ToLower(s)
	for _, p := range prefixes { if strings.HasPrefix(lower, p) { return true } }
	return false
}
```

- [ ] **Step 6: Create placeholder files for OSPF and MPLS**

```go
// internal/parser/cisco/show_ospf.go
package cisco

import "github.com/xavierli/nethelper/internal/model"

func ParseShowOSPFNeighbor(raw string) (model.ParseResult, error) {
	return model.ParseResult{Type: model.CmdNeighbor, RawText: raw}, nil
}
```

```go
// internal/parser/cisco/show_mpls.go
package cisco

import "github.com/xavierli/nethelper/internal/model"

func ParseShowMplsForwarding(raw string) (model.ParseResult, error) {
	return model.ParseResult{Type: model.CmdLFIB, RawText: raw}, nil
}
```

- [ ] **Step 7: Run test — verify PASS**

Run: `go test ./internal/parser/cisco/ -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/parser/cisco/
git commit -m "feat: add Cisco IOS parser with interface brief and routing table"
```

---

### Task 2: Cisco Parser — OSPF Neighbor + MPLS Forwarding

**Files:**
- Modify: `internal/parser/cisco/show_ospf.go`
- Modify: `internal/parser/cisco/show_mpls.go`
- Modify: `internal/parser/cisco/cisco_test.go`

- [ ] **Step 1: Add tests**

Append to `cisco_test.go`:

```go
func TestParseShowOSPFNeighbor(t *testing.T) {
	input := `Neighbor ID     Pri   State           Dead Time   Address         Interface
10.0.0.2       1   FULL/DR         00:00:32    10.0.0.1    GigabitEthernet0/0
10.0.0.3       1   FULL/BDR        00:00:35    10.0.0.2  GigabitEthernet0/1`

	result, err := ParseShowOSPFNeighbor(input)
	if err != nil { t.Fatalf("error: %v", err) }
	if len(result.Neighbors) != 2 { t.Fatalf("expected 2, got %d", len(result.Neighbors)) }

	n := result.Neighbors[0]
	if n.RemoteID != "10.0.0.2" { t.Errorf("id: %s", n.RemoteID) }
	if n.State != "full/dr" { t.Errorf("state: %s", n.State) }
	if n.LocalInterface != "GigabitEthernet0/0" { t.Errorf("iface: %s", n.LocalInterface) }
	if n.Protocol != "ospf" { t.Errorf("proto: %s", n.Protocol) }
}

func TestParseShowMplsForwarding(t *testing.T) {
	input := `Local      Outgoing   Prefix           Bytes Label   Outgoing         Next Hop
Label      Label      or Tunnel Id     Switched        interface
16         Pop Label  10.1.1.0/24      0           Gi0/0            10.0.0.1
17         18         172.16.0.0/24    12345       Gi0/1            10.0.0.2
18         No Label   192.168.1.0/16       0           Gi0/2            10.0.0.3`

	result, err := ParseShowMplsForwarding(input)
	if err != nil { t.Fatalf("error: %v", err) }
	if len(result.LFIBEntries) != 3 { t.Fatalf("expected 3, got %d", len(result.LFIBEntries)) }

	e0 := result.LFIBEntries[0]
	if e0.InLabel != 16 { t.Errorf("in: %d", e0.InLabel) }
	if e0.Action != "pop" { t.Errorf("action: %s", e0.Action) }

	e1 := result.LFIBEntries[1]
	if e1.InLabel != 17 || e1.OutLabel != "18" { t.Errorf("labels: %d→%s", e1.InLabel, e1.OutLabel) }
	if e1.Action != "swap" { t.Errorf("action: %s", e1.Action) }
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/parser/cisco/ -v -run "TestParseShowOSPF|TestParseShowMpls"`
Expected: FAIL (placeholders return empty)

- [ ] **Step 3: Implement show_ospf.go**

```go
// internal/parser/cisco/show_ospf.go
package cisco

import (
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

// ParseShowOSPFNeighbor parses "show ip ospf neighbor" output.
func ParseShowOSPFNeighbor(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdNeighbor, RawText: raw}
	lines := strings.Split(raw, "\n")
	headerFound := false

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" { continue }
		if !headerFound {
			if strings.Contains(trimmed, "Neighbor ID") && strings.Contains(trimmed, "State") { headerFound = true }
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 6 { continue }

		result.Neighbors = append(result.Neighbors, model.NeighborInfo{
			Protocol: "ospf", RemoteID: fields[0],
			State: strings.ToLower(fields[2]),
			RemoteAddress: fields[4], LocalInterface: fields[5],
		})
	}
	return result, nil
}
```

- [ ] **Step 4: Implement show_mpls.go**

```go
// internal/parser/cisco/show_mpls.go
package cisco

import (
	"strconv"
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

// ParseShowMplsForwarding parses "show mpls forwarding-table" output.
func ParseShowMplsForwarding(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdLFIB, RawText: raw}
	lines := strings.Split(raw, "\n")
	headerFound := false

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" { continue }
		if !headerFound {
			if strings.Contains(trimmed, "Local") && strings.Contains(trimmed, "Outgoing") { headerFound = true }
			// Skip the second header line too
			continue
		}
		// Skip continuation header lines
		if strings.Contains(trimmed, "Label") && strings.Contains(trimmed, "interface") { continue }

		fields := strings.Fields(trimmed)
		if len(fields) < 5 { continue }

		inLabel, err := strconv.Atoi(fields[0])
		if err != nil { continue }

		outLabelStr := fields[1]
		var action string
		var outLabel string

		lower := strings.ToLower(outLabelStr)
		switch {
		case lower == "pop":
			action = "pop"
			outLabel = ""
			// "Pop Label" is two words
		case strings.HasPrefix(lower, "no"):
			action = "pop"
			outLabel = ""
			// "No Label" is two words
		default:
			outLabel = outLabelStr
			action = "swap"
		}

		// Handle two-word labels: "Pop Label", "No Label"
		// If outLabelStr is "Pop" or "No", the actual fields shift
		var fec, outIface, nextHop string
		if lower == "pop" || lower == "no" {
			// fields[1]="Pop" fields[2]="Label" fields[3]=prefix fields[4]=bytes fields[5]=iface fields[6]=nexthop
			if len(fields) >= 7 {
				fec = fields[3]
				outIface = fields[5]
				nextHop = fields[6]
			}
		} else {
			// fields[1]=outlabel fields[2]=prefix fields[3]=bytes fields[4]=iface fields[5]=nexthop
			if len(fields) >= 6 {
				fec = fields[2]
				outIface = fields[4]
				nextHop = fields[5]
			}
		}

		result.LFIBEntries = append(result.LFIBEntries, model.LFIBEntry{
			InLabel: inLabel, Action: action, OutLabel: outLabel,
			FEC: fec, OutgoingInterface: outIface, NextHop: nextHop,
		})
	}
	return result, nil
}
```

- [ ] **Step 5: Run test — verify PASS**

Run: `go test ./internal/parser/cisco/ -v`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/parser/cisco/
git commit -m "feat: add Cisco OSPF neighbor and MPLS forwarding parsers"
```

---

### Task 3: H3C Comware Parser

H3C uses the same `<hostname>` prompt and `display` commands as Huawei, but has subtle output format differences. We differentiate by vendor string but reuse similar parsing logic.

**Files:**
- Create: `internal/parser/h3c/h3c.go`
- Create: `internal/parser/h3c/display_interface.go`
- Create: `internal/parser/h3c/display_route.go`
- Create: `internal/parser/h3c/display_ospf.go`
- Create: `internal/parser/h3c/display_mpls.go`
- Create: `internal/parser/h3c/h3c_test.go`

- [ ] **Step 1: Write test**

```go
// internal/parser/h3c/h3c_test.go
package h3c

import (
	"testing"
	"github.com/xavierli/nethelper/internal/model"
)

func TestH3CVendor(t *testing.T) {
	if New().Vendor() != "h3c" { t.Error("expected h3c") }
}

func TestH3CDetectPrompt(t *testing.T) {
	p := New()
	// H3C uses same <hostname> as Huawei
	host, ok := p.DetectPrompt("<H3C-SW01>display version")
	if !ok || host != "H3C-SW01" { t.Errorf("got (%q, %v)", host, ok) }

	host, ok = p.DetectPrompt("[H3C-SW01]interface GE1/0/1")
	if !ok || host != "H3C-SW01" { t.Errorf("got (%q, %v)", host, ok) }

	_, ok = p.DetectPrompt("Router#show version")
	if ok { t.Error("should not match Cisco") }
}

func TestH3CParseInterfaceBrief(t *testing.T) {
	input := `Brief information on interfaces in route mode:
Link: ADM - administratively down; Stby - standby
Protocol: (s) - spoofing
Interface            Link Protocol Primary IP      Description
GE1/0/1              UP   UP       10.0.0.1
GE1/0/2              DOWN DOWN     --
Loop0                UP   UP(s)    1.1.1.1`

	result, err := ParseInterfaceBrief(input)
	if err != nil { t.Fatalf("error: %v", err) }
	if len(result.Interfaces) != 3 { t.Fatalf("expected 3, got %d", len(result.Interfaces)) }
	if result.Interfaces[0].Name != "GE1/0/1" { t.Errorf("name: %s", result.Interfaces[0].Name) }
	if result.Interfaces[0].Status != "up" { t.Errorf("status: %s", result.Interfaces[0].Status) }
	if result.Interfaces[2].Type != model.IfTypeLoopback { t.Errorf("type: %s", result.Interfaces[2].Type) }
}

func TestH3CParseRoutingTable(t *testing.T) {
	input := `Routing Tables: Public
	Destinations : 3        Routes : 3

Destination/Mask    Proto  Pre  Cost         NextHop         Interface
10.1.1.0/24        O_INTRA 10   2            10.0.0.1        GE1/0/1
172.16.0.0/16      S      60   0            10.0.0.2        GE1/0/2
0.0.0.0/0           S      60   0            10.0.0.1        GE1/0/1`

	result, err := ParseRoutingTable(input)
	if err != nil { t.Fatalf("error: %v", err) }
	if len(result.RIBEntries) != 3 { t.Fatalf("expected 3, got %d", len(result.RIBEntries)) }
	if result.RIBEntries[0].Protocol != "ospf" { t.Errorf("proto: %s", result.RIBEntries[0].Protocol) }
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/parser/h3c/ -v`
Expected: FAIL

- [ ] **Step 3: Implement h3c.go**

```go
// internal/parser/h3c/h3c.go
package h3c

import (
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

type Parser struct{}
func New() *Parser { return &Parser{} }
func (p *Parser) Vendor() string { return "h3c" }

func (p *Parser) DetectPrompt(line string) (string, bool) {
	trimmed := strings.TrimRight(line, "\r \t")
	if len(trimmed) > 2 && trimmed[0] == '<' {
		end := strings.Index(trimmed, ">")
		if end > 1 { return trimmed[1:end], true }
	}
	if len(trimmed) > 2 && trimmed[0] == '[' {
		end := strings.Index(trimmed, "]")
		if end > 1 { return trimmed[1:end], true }
	}
	return "", false
}

func (p *Parser) ClassifyCommand(cmd string) model.CommandType {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	switch {
	case strings.HasPrefix(lower, "display ip routing-table"): return model.CmdRIB
	case strings.HasPrefix(lower, "display fib"): return model.CmdFIB
	case strings.HasPrefix(lower, "display mpls lsp"), strings.HasPrefix(lower, "display mpls forwarding"): return model.CmdLFIB
	case strings.HasPrefix(lower, "display interface"): return model.CmdInterface
	case strings.HasPrefix(lower, "display ospf peer"), strings.HasPrefix(lower, "display bgp peer"),
		strings.HasPrefix(lower, "display isis peer"), strings.HasPrefix(lower, "display mpls ldp session"):
		return model.CmdNeighbor
	case strings.HasPrefix(lower, "display current-configuration"): return model.CmdConfig
	default: return model.CmdUnknown
	}
}

func (p *Parser) ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error) {
	switch cmdType {
	case model.CmdInterface: return ParseInterfaceBrief(raw)
	case model.CmdRIB: return ParseRoutingTable(raw)
	case model.CmdNeighbor: return ParseOspfPeer(raw)
	case model.CmdLFIB: return ParseMplsLsp(raw)
	default: return model.ParseResult{Type: cmdType, RawText: raw}, nil
	}
}

func inferInterfaceType(name string) model.InterfaceType {
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(lower, "loop"): return model.IfTypeLoopback
	case strings.HasPrefix(lower, "vlan"): return model.IfTypeVlanif
	case strings.HasPrefix(lower, "bridge-aggregation"), strings.HasPrefix(lower, "bagg"): return model.IfTypeEthTrunk
	case strings.HasPrefix(lower, "tunnel"): return model.IfTypeTunnelGRE
	case strings.HasPrefix(lower, "null"): return model.IfTypeNull
	case strings.Contains(lower, "."): return model.IfTypeSubInterface
	default: return model.IfTypePhysical
	}
}
```

- [ ] **Step 4: Implement display_interface.go**

```go
// internal/parser/h3c/display_interface.go
package h3c

import (
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

func ParseInterfaceBrief(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdInterface, RawText: raw}
	lines := strings.Split(raw, "\n")
	headerFound := false

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" { continue }
		if !headerFound {
			if strings.Contains(trimmed, "Interface") && strings.Contains(trimmed, "Link") && strings.Contains(trimmed, "Protocol") {
				headerFound = true
			}
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 3 { continue }

		name := fields[0]
		linkStatus := strings.ToLower(fields[1])
		if linkStatus == "adm" { linkStatus = "admin-down" }

		var ip string
		if len(fields) >= 4 && fields[3] != "--" { ip = fields[3] }

		result.Interfaces = append(result.Interfaces, model.Interface{
			Name: name, Type: inferInterfaceType(name), Status: linkStatus, IPAddress: ip,
		})
	}
	return result, nil
}
```

- [ ] **Step 5: Implement display_route.go**

```go
// internal/parser/h3c/display_route.go
package h3c

import (
	"strconv"
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

func ParseRoutingTable(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdRIB, RawText: raw}
	lines := strings.Split(raw, "\n")
	headerFound := false
	vrf := "default"

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" { continue }
		if strings.HasPrefix(strings.TrimSpace(trimmed), "Routing Tables:") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				vrfName := strings.TrimSpace(parts[1])
				if vrfName != "" && !strings.EqualFold(vrfName, "Public") { vrf = vrfName }
			}
			continue
		}
		if !headerFound {
			if strings.Contains(trimmed, "Destination/Mask") && strings.Contains(trimmed, "Proto") { headerFound = true }
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 6 { continue }

		parts := strings.SplitN(fields[0], "/", 2)
		if len(parts) != 2 { continue }
		prefix := parts[0]
		maskLen, err := strconv.Atoi(parts[1])
		if err != nil { continue }

		proto := h3cProtoToStandard(fields[1])
		pref, _ := strconv.Atoi(fields[2])
		cost, _ := strconv.Atoi(fields[3])

		var nextHop, outIface string
		remaining := fields[4:]
		for i, f := range remaining {
			if isIPLike(f) { nextHop = f; if i+1 < len(remaining) { outIface = remaining[i+1] }; break }
		}
		if nextHop == "" && len(remaining) >= 2 { nextHop = remaining[len(remaining)-2]; outIface = remaining[len(remaining)-1] }

		result.RIBEntries = append(result.RIBEntries, model.RIBEntry{
			Prefix: prefix, MaskLen: maskLen, Protocol: proto,
			Preference: pref, Metric: cost, NextHop: nextHop,
			OutgoingInterface: outIface, VRF: vrf,
		})
	}
	return result, nil
}

func h3cProtoToStandard(proto string) string {
	lower := strings.ToLower(proto)
	switch {
	case strings.HasPrefix(lower, "o_"): return "ospf"
	case lower == "o": return "ospf"
	case lower == "s": return "static"
	case lower == "b": return "bgp"
	case lower == "d": return "direct"
	case lower == "i": return "isis"
	default: return lower
	}
}

func isIPLike(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 { return false }
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 255 { return false }
	}
	return true
}
```

- [ ] **Step 6: Create placeholder files**

```go
// internal/parser/h3c/display_ospf.go
package h3c

import (
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

func ParseOspfPeer(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdNeighbor, RawText: raw}
	lines := strings.Split(raw, "\n")
	headerFound := false

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" { continue }
		if !headerFound {
			if strings.Contains(trimmed, "Area Id") && strings.Contains(trimmed, "Neighbor") { headerFound = true }
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 4 { continue }
		result.Neighbors = append(result.Neighbors, model.NeighborInfo{
			Protocol: "ospf", AreaID: fields[0], LocalInterface: fields[1],
			RemoteID: fields[2], State: strings.ToLower(fields[3]),
		})
	}
	return result, nil
}
```

```go
// internal/parser/h3c/display_mpls.go
package h3c

import "github.com/xavierli/nethelper/internal/model"

func ParseMplsLsp(raw string) (model.ParseResult, error) {
	// H3C MPLS LSP format is very similar to Huawei — reuse same pattern
	return model.ParseResult{Type: model.CmdLFIB, RawText: raw}, nil
}
```

- [ ] **Step 7: Run test — verify PASS**

Run: `go test ./internal/parser/h3c/ -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/parser/h3c/
git commit -m "feat: add H3C Comware parser with interface, routing table, and OSPF"
```

---

### Task 4: Juniper JUNOS Parser

**Files:**
- Create: `internal/parser/juniper/juniper.go`
- Create: `internal/parser/juniper/show_interfaces.go`
- Create: `internal/parser/juniper/show_route.go`
- Create: `internal/parser/juniper/show_ospf.go`
- Create: `internal/parser/juniper/show_mpls.go`
- Create: `internal/parser/juniper/juniper_test.go`

- [ ] **Step 1: Write test**

```go
// internal/parser/juniper/juniper_test.go
package juniper

import (
	"testing"
	"github.com/xavierli/nethelper/internal/model"
)

func TestJuniperVendor(t *testing.T) {
	if New().Vendor() != "juniper" { t.Error("expected juniper") }
}

func TestJuniperDetectPrompt(t *testing.T) {
	p := New()
	tests := []struct{ line, wantHost string; wantOK bool }{
		{"admin@MX204-01> show version", "MX204-01", true},
		{"admin@MX204-01# set interfaces", "MX204-01", true},
		{"<HUAWEI>display version", "", false},
	}
	for _, tt := range tests {
		host, ok := p.DetectPrompt(tt.line)
		if ok != tt.wantOK || host != tt.wantHost {
			t.Errorf("line %q: got (%q,%v), want (%q,%v)", tt.line, host, ok, tt.wantHost, tt.wantOK)
		}
	}
}

func TestParseShowInterfacesTerse(t *testing.T) {
	input := `Interface               Admin Link Proto    Local                 Remote
ge-0/0/0                up    up
ge-0/0/0.0              up    up   inet     10.0.0.1/24
ge-0/0/1                up    down
lo0.0                   up    up   inet     1.1.1.1/32
ae0                     up    up`

	result, err := ParseShowInterfacesTerse(input)
	if err != nil { t.Fatalf("error: %v", err) }
	if len(result.Interfaces) != 5 { t.Fatalf("expected 5, got %d", len(result.Interfaces)) }

	if result.Interfaces[0].Name != "ge-0/0/0" { t.Errorf("name: %s", result.Interfaces[0].Name) }
	if result.Interfaces[0].Status != "up" { t.Errorf("status: %s", result.Interfaces[0].Status) }
	if result.Interfaces[0].Type != model.IfTypePhysical { t.Errorf("type: %s", result.Interfaces[0].Type) }
	if result.Interfaces[1].Type != model.IfTypeSubInterface { t.Errorf("type: %s", result.Interfaces[1].Type) }
	if result.Interfaces[1].IPAddress != "10.0.0.1" { t.Errorf("ip: %s", result.Interfaces[1].IPAddress) }
	if result.Interfaces[3].Type != model.IfTypeLoopback { t.Errorf("type: %s", result.Interfaces[3].Type) }
	if result.Interfaces[4].Type != model.IfTypeEthTrunk { t.Errorf("type: %s", result.Interfaces[4].Type) }
}

func TestParseShowRoute(t *testing.T) {
	input := `inet.0: 5 destinations, 5 routes (5 active, 0 holddown, 0 hidden)
+ = Active Route, - = Last Active, * = Both

10.1.1.0/24      *[OSPF/10] 00:05:12, metric 2
                    >  to 10.0.0.1 via ge-0/0/0.0
172.16.0.0/16       *[Static/5] 00:10:00
                    >  to 10.0.0.2 via ge-0/0/1.0
0.0.0.0/0          *[Static/5] 01:00:00
                    >  to 10.0.0.1 via ge-0/0/0.0`

	result, err := ParseShowRoute(input)
	if err != nil { t.Fatalf("error: %v", err) }
	if len(result.RIBEntries) != 3 { t.Fatalf("expected 3, got %d", len(result.RIBEntries)) }

	e := result.RIBEntries[0]
	if e.Prefix != "10.1.1.0" || e.MaskLen != 24 { t.Errorf("prefix: %s/%d", e.Prefix, e.MaskLen) }
	if e.Protocol != "ospf" { t.Errorf("proto: %s", e.Protocol) }
	if e.Preference != 10 { t.Errorf("pref: %d", e.Preference) }
	if e.NextHop != "10.0.0.1" { t.Errorf("nexthop: %s", e.NextHop) }
	if e.OutgoingInterface != "ge-0/0/0.0" { t.Errorf("iface: %s", e.OutgoingInterface) }
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/parser/juniper/ -v`
Expected: FAIL

- [ ] **Step 3: Implement juniper.go**

```go
// internal/parser/juniper/juniper.go
package juniper

import (
	"regexp"
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

var promptRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9._-]*@([A-Za-z][A-Za-z0-9._-]*)[>#]`)

type Parser struct{}
func New() *Parser { return &Parser{} }
func (p *Parser) Vendor() string { return "juniper" }

func (p *Parser) DetectPrompt(line string) (string, bool) {
	m := promptRe.FindStringSubmatch(strings.TrimRight(line, "\r \t"))
	if m == nil { return "", false }
	return m[1], true
}

func (p *Parser) ClassifyCommand(cmd string) model.CommandType {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	switch {
	case strings.HasPrefix(lower, "show route"): return model.CmdRIB
	case strings.HasPrefix(lower, "show interface"): return model.CmdInterface
	case strings.HasPrefix(lower, "show ospf neighbor"): return model.CmdNeighbor
	case strings.HasPrefix(lower, "show bgp summary"): return model.CmdNeighbor
	case strings.HasPrefix(lower, "show isis adjacency"): return model.CmdNeighbor
	case strings.HasPrefix(lower, "show ldp session"), strings.HasPrefix(lower, "show ldp neighbor"): return model.CmdNeighbor
	case strings.HasPrefix(lower, "show rsvp session"): return model.CmdTunnel
	case strings.HasPrefix(lower, "show route table mpls"): return model.CmdLFIB
	case strings.HasPrefix(lower, "show configuration"): return model.CmdConfig
	default: return model.CmdUnknown
	}
}

func (p *Parser) ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error) {
	switch cmdType {
	case model.CmdInterface: return ParseShowInterfacesTerse(raw)
	case model.CmdRIB: return ParseShowRoute(raw)
	case model.CmdNeighbor: return ParseShowOSPFNeighbor(raw)
	case model.CmdLFIB: return ParseShowMplsRoute(raw)
	default: return model.ParseResult{Type: cmdType, RawText: raw}, nil
	}
}

func inferInterfaceType(name string) model.InterfaceType {
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(lower, "lo"): return model.IfTypeLoopback
	case strings.HasPrefix(lower, "ae"): return model.IfTypeEthTrunk
	case strings.HasPrefix(lower, "irb"), strings.HasPrefix(lower, "vlan"): return model.IfTypeVlanif
	case strings.HasPrefix(lower, "gr-"), strings.HasPrefix(lower, "ip-"): return model.IfTypeTunnelGRE
	case strings.Contains(lower, "."): return model.IfTypeSubInterface
	default: return model.IfTypePhysical
	}
}
```

- [ ] **Step 4: Implement show_interfaces.go**

```go
// internal/parser/juniper/show_interfaces.go
package juniper

import (
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

func ParseShowInterfacesTerse(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdInterface, RawText: raw}
	lines := strings.Split(raw, "\n")
	headerFound := false

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" { continue }
		if !headerFound {
			if strings.Contains(trimmed, "Interface") && strings.Contains(trimmed, "Admin") && strings.Contains(trimmed, "Link") {
				headerFound = true
			}
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 3 { continue }

		name := fields[0]
		admin := strings.ToLower(fields[1])
		link := strings.ToLower(fields[2])

		status := link
		if admin == "down" { status = "admin-down" }

		var ip string
		// If inet proto present, IP is after it: "inet 10.0.0.1/24"
		for i, f := range fields {
			if f == "inet" && i+1 < len(fields) {
				ipMask := fields[i+1]
				ip = strings.SplitN(ipMask, "/", 2)[0]
				break
			}
		}

		result.Interfaces = append(result.Interfaces, model.Interface{
			Name: name, Type: inferInterfaceType(name), Status: status, IPAddress: ip,
		})
	}
	return result, nil
}
```

- [ ] **Step 5: Implement show_route.go**

```go
// internal/parser/juniper/show_route.go
package juniper

import (
	"regexp"
	"strconv"
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

// Juniper route format:
// 10.1.1.0/24      *[OSPF/10] 00:05:12, metric 2
//                     >  to 10.0.0.1 via ge-0/0/0.0
var routeHeadRe = regexp.MustCompile(`^(\d+\.\d+\.\d+\.\d+/\d+)\s+\*?\[([^/]+)/(\d+)\]`)
var routeNextHopRe = regexp.MustCompile(`to\s+(\d+\.\d+\.\d+\.\d+)\s+via\s+(\S+)`)

func ParseShowRoute(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdRIB, RawText: raw}
	lines := strings.Split(raw, "\n")

	var current *model.RIBEntry

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" { continue }

		// Try route header: "10.1.1.0/24 *[OSPF/10] ..."
		if m := routeHeadRe.FindStringSubmatch(trimmed); m != nil {
			// Save previous entry
			if current != nil { result.RIBEntries = append(result.RIBEntries, *current) }

			parts := strings.SplitN(m[1], "/", 2)
			maskLen, _ := strconv.Atoi(parts[1])
			pref, _ := strconv.Atoi(m[3])
			proto := strings.ToLower(m[2])

			var metric int
			if idx := strings.Index(trimmed, "metric "); idx >= 0 {
				metricStr := strings.Fields(trimmed[idx+7:])[0]
				metric, _ = strconv.Atoi(strings.TrimRight(metricStr, ","))
			}

			current = &model.RIBEntry{
				Prefix: parts[0], MaskLen: maskLen, Protocol: proto,
				Preference: pref, Metric: metric, VRF: "default",
			}
			continue
		}

		// Try next-hop line: "> to X.X.X.X via ge-0/0/0.0"
		if current != nil {
			if m := routeNextHopRe.FindStringSubmatch(trimmed); m != nil {
				current.NextHop = m[1]
				current.OutgoingInterface = m[2]
			}
		}
	}

	// Don't forget last entry
	if current != nil { result.RIBEntries = append(result.RIBEntries, *current) }

	return result, nil
}
```

- [ ] **Step 6: Create placeholder files**

```go
// internal/parser/juniper/show_ospf.go
package juniper

import (
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

func ParseShowOSPFNeighbor(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdNeighbor, RawText: raw}
	lines := strings.Split(raw, "\n")
	headerFound := false

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" { continue }
		if !headerFound {
			if strings.Contains(trimmed, "Address") && strings.Contains(trimmed, "State") { headerFound = true }
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 4 { continue }
		result.Neighbors = append(result.Neighbors, model.NeighborInfo{
			Protocol: "ospf", RemoteAddress: fields[0], LocalInterface: fields[1],
			State: strings.ToLower(fields[2]), RemoteID: fields[3],
		})
	}
	return result, nil
}
```

```go
// internal/parser/juniper/show_mpls.go
package juniper

import "github.com/xavierli/nethelper/internal/model"

func ParseShowMplsRoute(raw string) (model.ParseResult, error) {
	return model.ParseResult{Type: model.CmdLFIB, RawText: raw}, nil
}
```

- [ ] **Step 7: Run test — verify PASS**

Run: `go test ./internal/parser/juniper/ -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/parser/juniper/
git commit -m "feat: add Juniper JUNOS parser with interfaces, routing table, and OSPF"
```

---

### Task 5: Register All Parsers + Vendor Disambiguation

**Files:**
- Modify: `internal/cli/root.go`
- Modify: `internal/parser/detector.go`

- [ ] **Step 1: Update root.go to register all parsers**

In `internal/cli/root.go`, add imports and register parsers:

```go
import (
	// existing imports...
	"github.com/xavierli/nethelper/internal/parser/cisco"
	"github.com/xavierli/nethelper/internal/parser/h3c"
	"github.com/xavierli/nethelper/internal/parser/juniper"
)
```

In `PersistentPreRunE`, update the registry block:

```go
		registry := parser.NewRegistry()
		registry.Register(huawei.New())
		registry.Register(cisco.New())
		registry.Register(h3c.New())
		registry.Register(juniper.New())
		pipeline = parser.NewPipeline(db, registry)
```

- [ ] **Step 2: Handle Huawei/H3C prompt disambiguation in detector.go**

The detector currently returns "huawei" for all `<hostname>` prompts. H3C uses the same pattern. We need a way to disambiguate. Add a note comment to `detector.go`:

In `DetectVendor`, after the Huawei angle bracket check, add a comment:

```go
	// NOTE: H3C uses the same <hostname> prompt as Huawei.
	// DetectVendor returns "huawei" for both. In the pipeline, when the
	// full VendorParser registry is available, H3C detection happens via
	// the H3C parser's DetectPrompt — but since both match <hostname>,
	// the registry iteration order determines which parser claims the block.
	// For explicit H3C support, users should configure device-vendor mappings
	// in config.yaml (future feature). For now, Huawei parser handles both
	// since the output formats are very similar.
```

- [ ] **Step 3: Build and test**

Run: `go build ./cmd/nethelper && go test ./... -v`
Expected: all PASS, builds successfully

- [ ] **Step 4: Commit**

```bash
git add internal/cli/root.go internal/parser/detector.go
git commit -m "feat: register Cisco, H3C, Juniper parsers and document vendor disambiguation"
```

---

### Task 6: Watcher — Core File Monitoring

**Files:**
- Create: `internal/watcher/watcher.go`
- Create: `internal/watcher/watcher_test.go`

- [ ] **Step 1: Add fsnotify dependency**

Run: `go get github.com/fsnotify/fsnotify`

- [ ] **Step 2: Write test**

```go
// internal/watcher/watcher_test.go
package watcher

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatcherDetectsNewFile(t *testing.T) {
	dir := t.TempDir()
	var callCount atomic.Int32

	w, err := New(Config{
		Dirs:         []string{dir},
		Debounce:     100 * time.Millisecond,
		OnFileChange: func(path string) { callCount.Add(1) },
	})
	if err != nil { t.Fatalf("new watcher: %v", err) }

	go w.Start()
	defer w.Stop()

	// Wait for watcher to be ready
	time.Sleep(200 * time.Millisecond)

	// Create a file
	os.WriteFile(filepath.Join(dir, "test.log"), []byte("hello"), 0644)
	time.Sleep(500 * time.Millisecond)

	if callCount.Load() < 1 {
		t.Error("expected at least 1 callback")
	}
}

func TestWatcherDebounce(t *testing.T) {
	dir := t.TempDir()
	var callCount atomic.Int32

	w, err := New(Config{
		Dirs:         []string{dir},
		Debounce:     300 * time.Millisecond,
		OnFileChange: func(path string) { callCount.Add(1) },
	})
	if err != nil { t.Fatalf("new watcher: %v", err) }

	go w.Start()
	defer w.Stop()

	time.Sleep(200 * time.Millisecond)

	// Write multiple times in quick succession
	f := filepath.Join(dir, "test.log")
	for i := 0; i < 5; i++ {
		os.WriteFile(f, []byte("update"), 0644)
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(600 * time.Millisecond)

	// Debounce should collapse to ~1 callback
	count := callCount.Load()
	if count > 2 {
		t.Errorf("expected <=2 callbacks (debounced), got %d", count)
	}
}

func TestWatcherStopClean(t *testing.T) {
	dir := t.TempDir()
	w, err := New(Config{
		Dirs:         []string{dir},
		Debounce:     100 * time.Millisecond,
		OnFileChange: func(path string) {},
	})
	if err != nil { t.Fatalf("new watcher: %v", err) }

	go w.Start()
	time.Sleep(100 * time.Millisecond)
	w.Stop()
	// Should not panic or hang
}
```

- [ ] **Step 3: Run test — verify FAIL**

Run: `go test ./internal/watcher/ -v`
Expected: FAIL

- [ ] **Step 4: Implement watcher.go**

```go
// internal/watcher/watcher.go
package watcher

import (
	"log/slog"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Config configures the file watcher.
type Config struct {
	Dirs         []string
	Debounce     time.Duration
	OnFileChange func(path string)
}

// Watcher monitors directories for file changes with debouncing.
type Watcher struct {
	config  Config
	fsw     *fsnotify.Watcher
	stop    chan struct{}
	stopped chan struct{}
	mu      sync.Mutex
	timers  map[string]*time.Timer
}

func New(cfg Config) (*Watcher, error) {
	if cfg.Debounce == 0 {
		cfg.Debounce = 500 * time.Millisecond
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	for _, dir := range cfg.Dirs {
		if err := fsw.Add(dir); err != nil {
			fsw.Close()
			return nil, err
		}
	}

	return &Watcher{
		config:  cfg,
		fsw:     fsw,
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
		timers:  make(map[string]*time.Timer),
	}, nil
}

func (w *Watcher) Start() {
	defer close(w.stopped)

	for {
		select {
		case <-w.stop:
			return
		case event, ok := <-w.fsw.Events:
			if !ok { return }
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 { continue }
			w.debounce(event.Name)
		case err, ok := <-w.fsw.Errors:
			if !ok { return }
			slog.Error("watcher error", "error", err)
		}
	}
}

func (w *Watcher) debounce(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if timer, exists := w.timers[path]; exists {
		timer.Stop()
	}

	w.timers[path] = time.AfterFunc(w.config.Debounce, func() {
		w.mu.Lock()
		delete(w.timers, path)
		w.mu.Unlock()

		if w.config.OnFileChange != nil {
			w.config.OnFileChange(path)
		}
	})
}

func (w *Watcher) Stop() {
	close(w.stop)
	w.fsw.Close()
	<-w.stopped

	// Cancel pending timers
	w.mu.Lock()
	for _, t := range w.timers { t.Stop() }
	w.timers = nil
	w.mu.Unlock()
}
```

- [ ] **Step 5: Run test — verify PASS**

Run: `go test ./internal/watcher/ -v -timeout 30s`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/watcher/watcher.go internal/watcher/watcher_test.go go.mod go.sum
git commit -m "feat: add file watcher with fsnotify and debouncing"
```

---

### Task 7: Watcher — Daemon PID Management

**Files:**
- Create: `internal/watcher/daemon.go`
- Create: `internal/watcher/daemon_test.go`

- [ ] **Step 1: Write test**

```go
// internal/watcher/daemon_test.go
package watcher

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndReadPID(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "test.pid")

	if err := WritePID(pidFile); err != nil {
		t.Fatalf("write: %v", err)
	}

	pid, err := ReadPID(pidFile)
	if err != nil { t.Fatalf("read: %v", err) }
	if pid != os.Getpid() { t.Errorf("expected %d, got %d", os.Getpid(), pid) }
}

func TestReadPIDMissing(t *testing.T) {
	_, err := ReadPID("/nonexistent/pid")
	if err == nil { t.Error("expected error") }
}

func TestRemovePID(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "test.pid")
	WritePID(pidFile)
	RemovePID(pidFile)
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Error("pid file should be removed")
	}
}

func TestIsRunningCurrentProcess(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "test.pid")
	WritePID(pidFile)
	if !IsRunning(pidFile) { t.Error("current process should be running") }
}

func TestIsRunningStaleProcess(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "test.pid")
	// Write a PID that likely doesn't exist
	os.WriteFile(pidFile, []byte("999999999"), 0644)
	if IsRunning(pidFile) { t.Error("stale PID should not be running") }
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/watcher/ -v -run TestWrite`
Expected: FAIL

- [ ] **Step 3: Implement daemon.go**

```go
// internal/watcher/daemon.go
package watcher

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// WritePID writes the current process PID to a file.
func WritePID(path string) error {
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0644)
}

// ReadPID reads a PID from a file.
func ReadPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil { return 0, err }
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil { return 0, fmt.Errorf("invalid PID in %s: %w", path, err) }
	return pid, nil
}

// RemovePID removes the PID file.
func RemovePID(path string) {
	os.Remove(path)
}

// IsRunning checks if a process with the PID in the file is still running.
func IsRunning(path string) bool {
	pid, err := ReadPID(path)
	if err != nil { return false }

	process, err := os.FindProcess(pid)
	if err != nil { return false }

	// On Unix, FindProcess always succeeds. Send signal 0 to check if alive.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}
```

- [ ] **Step 4: Run test — verify PASS**

Run: `go test ./internal/watcher/ -v -timeout 30s`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/watcher/daemon.go internal/watcher/daemon_test.go
git commit -m "feat: add PID file management for watcher daemon"
```

---

### Task 8: Wire Watcher to CLI — start/stop/status

**Files:**
- Modify: `internal/cli/watch.go`
- Modify: `internal/cli/root.go`

- [ ] **Step 1: Update watch.go with real start/stop/status**

Replace the stubs in `internal/cli/watch.go`:

```go
package cli

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/watcher"
)

func newWatchCmd() *cobra.Command {
	watch := &cobra.Command{
		Use:   "watch",
		Short: "File monitoring and ingestion",
	}
	watch.AddCommand(newWatchIngestCmd())
	watch.AddCommand(newWatchStartCmd())
	watch.AddCommand(newWatchStopCmd())
	watch.AddCommand(newWatchStatusCmd())
	return watch
}

func newWatchIngestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ingest <file>",
		Short: "Manually import a log file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			data, err := os.ReadFile(filePath)
			if err != nil { return fmt.Errorf("read file: %w", err) }

			result, err := pipeline.Ingest(filePath, string(data))
			if err != nil { return fmt.Errorf("ingest: %w", err) }

			ing := model.LogIngestion{FilePath: filePath, LastOffset: int64(len(data)), ProcessedAt: time.Now()}
			if err := db.UpsertIngestion(ing); err != nil { return fmt.Errorf("record ingestion: %w", err) }

			fmt.Printf("Ingested %s (%d bytes)\n", filePath, len(data))
			fmt.Printf("  Devices: %d, Blocks parsed: %d, Failed: %d, Skipped: %d\n",
				result.DevicesFound, result.BlocksParsed, result.BlocksFailed, result.BlocksSkipped)
			return nil
		},
	}
}

func pidFilePath() string {
	return filepath.Join(cfg.DataDir, "watcher.pid")
}

func newWatchStartCmd() *cobra.Command {
	var dirs []string
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start file watcher in foreground",
		RunE: func(cmd *cobra.Command, args []string) error {
			watchDirs := dirs
			if len(watchDirs) == 0 { watchDirs = cfg.WatchDirs }
			if len(watchDirs) == 0 { return fmt.Errorf("no watch directories specified (use --dir or set watch_dirs in config)") }

			pidFile := pidFilePath()
			if watcher.IsRunning(pidFile) { return fmt.Errorf("watcher already running (PID file: %s)", pidFile) }

			// Mutex to serialize file processing (spec: 并发安全)
			var ingestMu sync.Mutex

			w, err := watcher.New(watcher.Config{
				Dirs:     watchDirs,
				Debounce: 500 * time.Millisecond,
				OnFileChange: func(path string) {
					ingestMu.Lock()
					defer ingestMu.Unlock()

					// Incremental read: check last_offset from store
					var offset int64
					ing, err := db.GetIngestion(path)
					if err == nil { offset = ing.LastOffset }

					f, err := os.Open(path)
					if err != nil {
						fmt.Fprintf(os.Stderr, "open %s: %v\n", path, err)
						return
					}
					defer f.Close()

					info, err := f.Stat()
					if err != nil {
						fmt.Fprintf(os.Stderr, "stat %s: %v\n", path, err)
						return
					}

					// If file is smaller than offset, it was rotated — read from start
					if info.Size() < offset { offset = 0 }
					if info.Size() == offset { return } // no new content

					// Read only new content from offset
					newData := make([]byte, info.Size()-offset)
					if _, err := f.ReadAt(newData, offset); err != nil {
						fmt.Fprintf(os.Stderr, "read %s: %v\n", path, err)
						return
					}

					result, err := pipeline.Ingest(path, string(newData))
					if err != nil {
						fmt.Fprintf(os.Stderr, "ingest %s: %v\n", path, err)
						return
					}

					newIng := model.LogIngestion{FilePath: path, LastOffset: info.Size(), ProcessedAt: time.Now()}
					db.UpsertIngestion(newIng)
					fmt.Printf("[%s] Ingested %s (new=%d bytes, devices=%d, parsed=%d)\n",
						time.Now().Format("15:04:05"), filepath.Base(path),
						len(newData), result.DevicesFound, result.BlocksParsed)
				},
			})
			if err != nil { return fmt.Errorf("create watcher: %w", err) }

			if err := watcher.WritePID(pidFile); err != nil {
				return fmt.Errorf("write PID: %w", err)
			}

			fmt.Printf("Watching directories: %v\n", watchDirs)
			fmt.Println("Press Ctrl+C to stop.")

			// Handle Ctrl+C / SIGTERM for graceful shutdown
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			go func() {
				<-sigCh
				fmt.Println("\nStopping watcher...")
				w.Stop()
			}()

			w.Start() // blocks until w.Stop() is called
			watcher.RemovePID(pidFile)
			fmt.Println("Watcher stopped.")
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&dirs, "dir", nil, "directories to watch (can specify multiple)")
	return cmd
}

func newWatchStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the watcher daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			pidFile := pidFilePath()
			if !watcher.IsRunning(pidFile) {
				fmt.Println("Watcher is not running.")
				return nil
			}
			pid, err := watcher.ReadPID(pidFile)
			if err != nil { return err }

			p, err := os.FindProcess(pid)
			if err != nil { return fmt.Errorf("find process: %w", err) }

			if err := p.Signal(os.Interrupt); err != nil {
				return fmt.Errorf("send signal: %w", err)
			}
			watcher.RemovePID(pidFile)
			fmt.Printf("Stopped watcher (PID %d)\n", pid)
			return nil
		},
	}
}

func newWatchStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show watcher status",
		Run: func(cmd *cobra.Command, args []string) {
			pidFile := pidFilePath()
			if watcher.IsRunning(pidFile) {
				pid, _ := watcher.ReadPID(pidFile)
				fmt.Printf("Watcher is running (PID %d)\n", pid)
			} else {
				fmt.Println("Watcher is not running.")
			}
		},
	}
}
```

- [ ] **Step 2: Build and verify**

Run: `go build ./cmd/nethelper`
Expected: compiles

Run: `./nethelper watch status`
Expected: `Watcher is not running.`

Run: `./nethelper watch start --help`
Expected: shows `--dir` flag

- [ ] **Step 3: Run full test suite**

Run: `go test ./... -timeout 60s`
Expected: all PASS

- [ ] **Step 4: Commit**

```bash
git add internal/cli/watch.go
git commit -m "feat: wire watcher daemon to CLI — start/stop/status now functional"
```

- [ ] **Step 5: Clean up**

Run: `rm -f nethelper`
