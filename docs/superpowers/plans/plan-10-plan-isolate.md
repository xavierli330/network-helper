# `plan isolate` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `nethelper plan isolate <device>` command that generates a structured, multi-phase device isolation change plan with vendor-specific CLI commands.

**Architecture:** New `internal/plan/` package with three layers: link discovery (infers interconnections from DB data), command generation (vendor-specific CLI templates), and rendering (text/markdown output). A new `internal/cli/plan.go` wires it into the Cobra command tree. Two-step interaction: first run outputs collection commands, second run with `--check` includes pre-check validation with baseline persistence.

**Tech Stack:** Go, Cobra CLI, SQLite (via existing `store.DB`), existing `graph.BuildFromDB` for subnet matching.

**Spec:** `docs/superpowers/specs/2026-03-22-plan-isolate-design.md`

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/plan/plan.go` | Create | `Link`, `Plan`, `Phase`, `DeviceCommand` data models (all types in one file) |
| `internal/plan/link.go` | Create | `DiscoverLinks()` — interconnection inference engine |
| `internal/plan/link_test.go` | Create | Unit tests for link discovery |
| `internal/plan/command.go` | Create | `CommandGenerator` interface + `GeneratorForVendor()` |
| `internal/plan/command_huawei.go` | Create | Huawei VRP command templates |
| `internal/plan/command_h3c.go` | Create | H3C Comware command templates |
| `internal/plan/command_test.go` | Create | Unit tests for command generators |
| `internal/plan/isolate.go` | Create | `BuildIsolationPlan()` orchestrator + `--check` pre-check logic |
| `internal/plan/isolate_test.go` | Create | Unit tests for plan generation |
| `internal/plan/render.go` | Create | `RenderText()` and `RenderMarkdown()` output formatters |
| `internal/plan/render_test.go` | Create | Unit tests for rendering |
| `internal/cli/plan.go` | Create | `plan` and `plan isolate` Cobra commands |
| `internal/cli/root.go` | Modify | Register `plan` command (add `root.AddCommand(newPlanCmd())`) |

---

## Task 1: Data Models (`plan.go`)

All data types in one file so the package compiles from the first task.

**Files:**
- Create: `internal/plan/plan.go`

- [ ] **Step 1: Create the plan package with all data types**

```go
// internal/plan/plan.go
package plan

import "time"

// Link represents a discovered interconnection between two devices.
type Link struct {
	LocalDevice    string   // local device ID (lowercase slug)
	LocalInterface string   // local interface name (resolved to parent for trunk members)
	LocalIP        string   // local IP address
	PeerDevice     string   // peer device ID
	PeerInterface  string   // peer interface name (may be empty)
	PeerIP         string   // peer IP (may be empty)
	Protocols      []string // inferred protocols: "ospf", "bgp", "ldp"
	Sources        []string // discovery source: "description", "subnet", "config"
	VRF            string   // VRF name, empty = global
}

// DeviceCommand is an ordered list of CLI commands to run on one device.
type DeviceCommand struct {
	DeviceID   string   // device ID (lowercase slug)
	DeviceHost string   // display hostname
	Vendor     string   // "huawei", "h3c", etc.
	Commands   []string // ordered CLI commands
	Purpose    string   // human-readable purpose
}

// Phase is one stage of the isolation change plan.
type Phase struct {
	Number      int             // 0-5
	Name        string          // e.g. "方案规划"
	Description string          // what this phase does
	Steps       []DeviceCommand // commands grouped by device
	Notes       []string        // warnings, wait instructions
}

// Plan is a complete device isolation change plan.
type Plan struct {
	TargetDevice   string    // target device ID
	TargetHostname string    // target device hostname
	TargetVendor   string    // target device vendor
	Links          []Link    // discovered interconnections
	IsSPOF         bool      // whether target is a single point of failure
	ImpactDevices  []string  // device hostnames that would be isolated
	Phases         []Phase   // ordered phases (0-5)
	GeneratedAt    time.Time // when plan was generated
}

// PreCheckResult holds the baseline state from Phase 1 pre-check.
type PreCheckResult struct {
	DeviceID       string `json:"device_id"`
	OSPFPeerCount  int    `json:"ospf_peer_count"`
	OSPFAllFull    bool   `json:"ospf_all_full"`
	BGPPeerCount   int    `json:"bgp_peer_count"`
	BGPAllEstab    bool   `json:"bgp_all_established"`
	InterfaceUp    int    `json:"interface_up_count"`
	InterfaceTotal int    `json:"interface_total"`
	Safe           bool   `json:"safe"`
	Warnings       []string `json:"warnings,omitempty"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/plan/`
Expected: Success (no output)

- [ ] **Step 3: Commit**

```bash
git add internal/plan/plan.go
git commit -m "feat(plan): add Link/Plan/Phase/DeviceCommand/PreCheckResult data models"
```

---

## Task 2: Link Discovery Engine (`link.go`)

**Files:**
- Create: `internal/plan/link.go`
- Create: `internal/plan/link_test.go`

**Key codebase APIs used (verified):**
- `db.ListDevices() ([]model.Device, error)` — `internal/store/device_store.go:36`
- `db.GetInterfaces(deviceID string) ([]model.Interface, error)` — `internal/store/interface_store.go:18`
- `db.GetConfigSnapshots(deviceID string) ([]model.ConfigSnapshot, error)` — `internal/store/config_snapshot_store.go:24`
- `graph.BuildFromDB(db) (*Graph, error)` — `internal/graph/builder.go:12`
- `g.NodesByType(NodeType) []*Node` — `internal/graph/graph.go:119`
- `g.NeighborsByType(id string, EdgeType) []Edge` — `internal/graph/graph.go:97`
- `g.GetNode(id string) (*Node, bool)` — `internal/graph/graph.go:72`
- `model.Interface` has `ParentID string` field — `internal/model/device.go`
- Graph `EdgeConnectsTo` connects **interface-to-interface** nodes (not device-to-device)
- Graph `EdgeHasInterface` connects device → interface
- `EdgeConnectsTo` edges have **nil Props** (no IP data on edge)
- Interface node Props include: `"name"`, `"device_id"`, `"ip"`, `"mask"`, `"status"`

- [ ] **Step 1: Write failing tests**

```go
// internal/plan/link_test.go
package plan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/store"
)

func setupTestDB(t *testing.T) *store.DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		os.Remove(dbPath)
	})
	return db
}

func TestDiscoverLinks_DescriptionMatching(t *testing.T) {
	db := setupTestDB(t)

	// Insert two devices
	db.UpsertDevice(model.Device{ID: "lc-01", Hostname: "LC-01", Vendor: "huawei"})
	db.UpsertDevice(model.Device{ID: "la-01", Hostname: "LA-01", Vendor: "huawei"})

	// Insert interface on lc-01 with description mentioning LA-01
	db.UpsertInterfaces("lc-01", []model.Interface{
		{
			ID:          "lc-01:FortyGigE2/0/27",
			Name:        "FortyGigE2/0/27",
			Type:        model.IfTypePhysical,
			Status:      "up",
			IPAddress:   "10.0.0.1",
			Mask:        "30",
			Description: "LA-01-FG1/0/50",
		},
	})

	links, err := DiscoverLinks(db, "lc-01")
	if err != nil {
		t.Fatalf("DiscoverLinks: %v", err)
	}
	if len(links) == 0 {
		t.Fatal("expected at least one link from description match")
	}

	found := false
	for _, l := range links {
		if l.PeerDevice == "la-01" && containsStr(l.Sources, "description") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected link to la-01 via description, got: %+v", links)
	}
}

func TestDiscoverLinks_SubnetMatching(t *testing.T) {
	db := setupTestDB(t)

	db.UpsertDevice(model.Device{ID: "dev-a", Hostname: "DEV-A", Vendor: "huawei"})
	db.UpsertDevice(model.Device{ID: "dev-b", Hostname: "DEV-B", Vendor: "huawei"})

	// Two interfaces on the same /30 subnet
	db.UpsertInterfaces("dev-a", []model.Interface{
		{ID: "dev-a:GE0/0/1", Name: "GE0/0/1", Type: model.IfTypePhysical, Status: "up", IPAddress: "192.168.1.1", Mask: "30"},
	})
	db.UpsertInterfaces("dev-b", []model.Interface{
		{ID: "dev-b:GE0/0/1", Name: "GE0/0/1", Type: model.IfTypePhysical, Status: "up", IPAddress: "192.168.1.2", Mask: "30"},
	})

	links, err := DiscoverLinks(db, "dev-a")
	if err != nil {
		t.Fatalf("DiscoverLinks: %v", err)
	}

	found := false
	for _, l := range links {
		if l.PeerDevice == "dev-b" && containsStr(l.Sources, "subnet") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected link to dev-b via subnet, got: %+v", links)
	}
}

func TestDiscoverLinks_ProtocolEnrichment(t *testing.T) {
	db := setupTestDB(t)

	db.UpsertDevice(model.Device{ID: "rtr-a", Hostname: "RTR-A", Vendor: "huawei"})
	db.UpsertDevice(model.Device{ID: "rtr-b", Hostname: "RTR-B", Vendor: "huawei"})

	db.UpsertInterfaces("rtr-a", []model.Interface{
		{ID: "rtr-a:GE0/0/1", Name: "GE0/0/1", Type: model.IfTypePhysical, Status: "up",
			IPAddress: "10.1.1.1", Mask: "30", Description: "RTR-B-GE0/0/1"},
	})

	// Insert config with OSPF and BGP
	snap, _ := db.CreateSnapshot(model.Snapshot{DeviceID: "rtr-a", Commands: "[]"})
	_ = snap
	db.InsertConfigSnapshot(model.ConfigSnapshot{
		DeviceID:   "rtr-a",
		ConfigText: "#\nospf 1\n area 0.0.0.0\n  network 10.1.1.0 0.0.0.3\n#\nbgp 65000\n peer 10.1.1.2 as-number 65001\n#",
	})

	links, err := DiscoverLinks(db, "rtr-a")
	if err != nil {
		t.Fatalf("DiscoverLinks: %v", err)
	}
	if len(links) == 0 {
		t.Fatal("expected links")
	}
	if !containsStr(links[0].Protocols, "ospf") {
		t.Errorf("expected ospf in protocols, got %v", links[0].Protocols)
	}
	if !containsStr(links[0].Protocols, "bgp") {
		t.Errorf("expected bgp in protocols, got %v", links[0].Protocols)
	}
}

func TestContainsStr(t *testing.T) {
	tests := []struct {
		ss   []string
		s    string
		want bool
	}{
		{[]string{"a", "b"}, "a", true},
		{[]string{"a", "b"}, "c", false},
		{nil, "a", false},
	}
	for _, tt := range tests {
		if got := containsStr(tt.ss, tt.s); got != tt.want {
			t.Errorf("containsStr(%v, %q) = %v, want %v", tt.ss, tt.s, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/plan/ -v -run TestDiscoverLinks`
Expected: FAIL (DiscoverLinks undefined)

- [ ] **Step 3: Implement DiscoverLinks**

```go
// internal/plan/link.go
package plan

import (
	"fmt"
	"strings"

	"github.com/xavierli/nethelper/internal/graph"
	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/store"
)

// DiscoverLinks infers interconnections for a device from DB data.
// Combines three strategies: interface description matching,
// subnet-based matching (via graph), and config inference.
func DiscoverLinks(db *store.DB, deviceID string) ([]Link, error) {
	// Load all known device hostnames for description matching
	devices, err := db.ListDevices()
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	hostMap := make(map[string]string) // lowercase hostname -> device ID
	for _, d := range devices {
		hostMap[strings.ToLower(d.Hostname)] = d.ID
	}

	// Load target device interfaces
	ifaces, err := db.GetInterfaces(deviceID)
	if err != nil {
		return nil, fmt.Errorf("get interfaces: %w", err)
	}

	linkMap := make(map[string]*Link) // key: "localIf->peerDevice"

	// Strategy 1: Description matching against known hostnames
	descriptionMatch(deviceID, ifaces, hostMap, linkMap)

	// Strategy 2: Subnet matching via graph
	if err := subnetMatch(db, deviceID, linkMap); err != nil {
		// Non-fatal: log but continue
		fmt.Printf("Warning: subnet matching failed: %v\n", err)
	}

	// Strategy 3: Enrich protocols from config
	enrichProtocols(db, deviceID, linkMap)

	// Flatten map to slice
	links := make([]Link, 0, len(linkMap))
	for _, l := range linkMap {
		links = append(links, *l)
	}
	return links, nil
}

// descriptionMatch finds links by matching known device hostnames
// in interface description fields. Does case-insensitive substring matching.
func descriptionMatch(deviceID string, ifaces []model.Interface, hostMap map[string]string, linkMap map[string]*Link) {
	for _, iface := range ifaces {
		if iface.Description == "" {
			continue
		}
		descLower := strings.ToLower(iface.Description)
		for hostLower, peerID := range hostMap {
			if peerID == deviceID {
				continue
			}
			if strings.Contains(descLower, hostLower) {
				localIf := resolveParent(iface, ifaces)
				key := localIf + "->" + peerID
				if existing, ok := linkMap[key]; ok {
					addSource(existing, "description")
				} else {
					linkMap[key] = &Link{
						LocalDevice:    deviceID,
						LocalInterface: localIf,
						LocalIP:        iface.IPAddress,
						PeerDevice:     peerID,
						Sources:        []string{"description"},
					}
				}
				break // one match per interface
			}
		}
	}
}

// subnetMatch finds links by walking the graph:
// target device → HAS_INTERFACE → interface → CONNECTS_TO → remote interface → find parent device.
func subnetMatch(db *store.DB, deviceID string, linkMap map[string]*Link) error {
	g, err := graph.BuildFromDB(db)
	if err != nil {
		return fmt.Errorf("build graph: %w", err)
	}

	// Walk: device → HAS_INTERFACE edges → local interfaces
	localIfEdges := g.NeighborsByType(deviceID, graph.EdgeHasInterface)
	for _, ifEdge := range localIfEdges {
		localIfNode, ok := g.GetNode(ifEdge.To)
		if !ok {
			continue
		}
		localIfName := localIfNode.Props["name"]
		localIP := localIfNode.Props["ip"]

		// Walk: local interface → CONNECTS_TO edges → remote interfaces
		connectEdges := g.NeighborsByType(ifEdge.To, graph.EdgeConnectsTo)
		for _, connEdge := range connectEdges {
			remoteIfNode, ok := g.GetNode(connEdge.To)
			if !ok {
				continue
			}
			remoteDevID := remoteIfNode.Props["device_id"]
			if remoteDevID == "" || remoteDevID == deviceID {
				continue
			}
			remoteIP := remoteIfNode.Props["ip"]
			remoteIfName := remoteIfNode.Props["name"]

			key := localIfName + "->" + remoteDevID
			if existing, ok := linkMap[key]; ok {
				addSource(existing, "subnet")
				if existing.LocalIP == "" {
					existing.LocalIP = localIP
				}
				if existing.PeerIP == "" {
					existing.PeerIP = remoteIP
				}
				if existing.PeerInterface == "" {
					existing.PeerInterface = remoteIfName
				}
			} else {
				linkMap[key] = &Link{
					LocalDevice:    deviceID,
					LocalInterface: localIfName,
					LocalIP:        localIP,
					PeerDevice:     remoteDevID,
					PeerInterface:  remoteIfName,
					PeerIP:         remoteIP,
					Sources:        []string{"subnet"},
				}
			}
		}
	}
	return nil
}

// enrichProtocols checks config snapshots to annotate links with protocol info.
func enrichProtocols(db *store.DB, deviceID string, linkMap map[string]*Link) {
	configs, err := db.GetConfigSnapshots(deviceID)
	if err != nil || len(configs) == 0 {
		return
	}
	configText := strings.ToLower(configs[0].ConfigText)

	hasOSPF := strings.Contains(configText, "ospf") || strings.Contains(configText, "router ospf")
	hasBGP := strings.Contains(configText, "bgp ")
	hasLDP := strings.Contains(configText, "mpls ldp") || strings.Contains(configText, "mpls l")

	for _, link := range linkMap {
		if hasOSPF && !containsStr(link.Protocols, "ospf") {
			link.Protocols = append(link.Protocols, "ospf")
		}
		if hasBGP && !containsStr(link.Protocols, "bgp") {
			link.Protocols = append(link.Protocols, "bgp")
		}
		if hasLDP && !containsStr(link.Protocols, "ldp") {
			link.Protocols = append(link.Protocols, "ldp")
		}
	}
}

// resolveParent returns the parent interface name if this interface
// is a trunk/LAG member (has ParentID), otherwise returns the name as-is.
func resolveParent(iface model.Interface, all []model.Interface) string {
	if iface.ParentID != "" {
		for _, a := range all {
			if a.ID == iface.ParentID {
				return a.Name
			}
		}
	}
	return iface.Name
}

func addSource(l *Link, source string) {
	if !containsStr(l.Sources, source) {
		l.Sources = append(l.Sources, source)
	}
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/plan/ -v -run "TestDiscoverLinks|TestContainsStr"`
Expected: PASS (all 4 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/plan/link.go internal/plan/link_test.go
git commit -m "feat(plan): add DiscoverLinks interconnection inference engine"
```

---

## Task 3: Command Generator Interface and Huawei Implementation

**Files:**
- Create: `internal/plan/command.go`
- Create: `internal/plan/command_huawei.go`
- Create: `internal/plan/command_test.go`

- [ ] **Step 1: Write failing tests with real assertions**

```go
// internal/plan/command_test.go
package plan

import (
	"strings"
	"testing"
)

func allCommandText(cmds []DeviceCommand) string {
	var b strings.Builder
	for _, dc := range cmds {
		for _, c := range dc.Commands {
			b.WriteString(c)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func TestHuaweiGenerator_CollectionCommands(t *testing.T) {
	gen := &HuaweiGenerator{}
	links := []Link{
		{
			LocalDevice: "lc-01", LocalInterface: "FortyGigE2/0/27",
			PeerDevice: "la-01", Protocols: []string{"ospf", "bgp"},
		},
	}
	cmds := gen.CollectionCommands("lc-01", links)
	if len(cmds) == 0 {
		t.Fatal("expected at least one DeviceCommand")
	}
	// Should have commands for target and peer
	var targetFound, peerFound bool
	for _, dc := range cmds {
		if dc.DeviceID == "lc-01" {
			targetFound = true
		}
		if dc.DeviceID == "la-01" {
			peerFound = true
		}
	}
	if !targetFound {
		t.Error("missing commands for target device lc-01")
	}
	if !peerFound {
		t.Error("missing commands for peer device la-01")
	}
	text := allCommandText(cmds)
	if !strings.Contains(text, "display ospf peer") {
		t.Error("expected 'display ospf peer' in collection commands")
	}
	if !strings.Contains(text, "display bgp peer") {
		t.Error("expected 'display bgp peer' in collection commands")
	}
}

func TestHuaweiGenerator_ProtocolIsolateCommands(t *testing.T) {
	gen := &HuaweiGenerator{}
	links := []Link{
		{
			LocalDevice: "lc-01", LocalInterface: "FortyGigE2/0/27",
			PeerDevice: "la-01", PeerIP: "10.0.0.2",
			Protocols: []string{"ospf", "bgp"},
		},
	}
	cmds := gen.ProtocolIsolateCommands("lc-01", links)
	text := allCommandText(cmds)
	if !strings.Contains(text, "ospf cost 65535") {
		t.Errorf("expected 'ospf cost 65535' in protocol isolation, got:\n%s", text)
	}
	if !strings.Contains(text, "peer 10.0.0.2 ignore") {
		t.Errorf("expected 'peer 10.0.0.2 ignore' in protocol isolation, got:\n%s", text)
	}
	if !strings.Contains(text, "interface FortyGigE2/0/27") {
		t.Errorf("expected interface reference, got:\n%s", text)
	}
}

func TestHuaweiGenerator_InterfaceIsolateCommands(t *testing.T) {
	gen := &HuaweiGenerator{}
	links := []Link{
		{LocalDevice: "lc-01", LocalInterface: "FortyGigE2/0/27", PeerDevice: "la-01"},
	}
	cmds := gen.InterfaceIsolateCommands("lc-01", links)
	text := allCommandText(cmds)
	if !strings.Contains(text, "shutdown") {
		t.Errorf("expected 'shutdown' in interface isolation, got:\n%s", text)
	}
	if !strings.Contains(text, "interface FortyGigE2/0/27") {
		t.Errorf("expected interface reference, got:\n%s", text)
	}
}

func TestHuaweiGenerator_RollbackCommands(t *testing.T) {
	gen := &HuaweiGenerator{}
	links := []Link{
		{
			LocalDevice: "lc-01", LocalInterface: "FortyGigE2/0/27",
			PeerDevice: "la-01", PeerIP: "10.0.0.2",
			Protocols: []string{"ospf", "bgp"},
		},
	}
	cmds := gen.RollbackCommands("lc-01", links)
	text := allCommandText(cmds)
	if !strings.Contains(text, "undo shutdown") {
		t.Errorf("expected 'undo shutdown' in rollback, got:\n%s", text)
	}
	if !strings.Contains(text, "undo ospf cost") {
		t.Errorf("expected 'undo ospf cost' in rollback, got:\n%s", text)
	}
	if !strings.Contains(text, "undo peer 10.0.0.2 ignore") {
		t.Errorf("expected 'undo peer 10.0.0.2 ignore' in rollback, got:\n%s", text)
	}
}

func TestHuaweiGenerator_DedupInterfaces(t *testing.T) {
	gen := &HuaweiGenerator{}
	// Two links via same interface (e.g., description + subnet both found it)
	links := []Link{
		{LocalDevice: "lc-01", LocalInterface: "FortyGigE2/0/27", PeerDevice: "la-01"},
		{LocalDevice: "lc-01", LocalInterface: "FortyGigE2/0/27", PeerDevice: "la-01"},
	}
	cmds := gen.InterfaceIsolateCommands("lc-01", links)
	text := allCommandText(cmds)
	// Should only have ONE "shutdown" per interface, not two
	if strings.Count(text, "shutdown") != 1 {
		t.Errorf("expected exactly 1 shutdown, got:\n%s", text)
	}
}

func TestHuaweiGenerator_BGPSkippedWhenNoPeerIP(t *testing.T) {
	gen := &HuaweiGenerator{}
	links := []Link{
		{
			LocalDevice: "lc-01", LocalInterface: "FortyGigE2/0/27",
			PeerDevice: "la-01", PeerIP: "", // empty!
			Protocols: []string{"bgp"},
		},
	}
	cmds := gen.ProtocolIsolateCommands("lc-01", links)
	text := allCommandText(cmds)
	// Should NOT contain "peer  ignore" (empty IP)
	if strings.Contains(text, "peer  ignore") {
		t.Errorf("should skip BGP peer when PeerIP is empty, got:\n%s", text)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/plan/ -v -run TestHuawei`
Expected: FAIL (HuaweiGenerator undefined)

- [ ] **Step 3: Create CommandGenerator interface**

```go
// internal/plan/command.go
package plan

// CommandGenerator produces vendor-specific CLI commands for each isolation phase.
type CommandGenerator interface {
	CollectionCommands(target string, links []Link) []DeviceCommand
	PreCheckCommands(target string, links []Link) []DeviceCommand
	ProtocolIsolateCommands(target string, links []Link) []DeviceCommand
	InterfaceIsolateCommands(target string, links []Link) []DeviceCommand
	PostCheckCommands(target string, links []Link) []DeviceCommand
	RollbackCommands(target string, links []Link) []DeviceCommand
}

// GeneratorForVendor returns the CommandGenerator for a vendor string.
func GeneratorForVendor(vendor string) CommandGenerator {
	switch vendor {
	case "h3c":
		return &H3CGenerator{}
	default:
		return &HuaweiGenerator{} // fallback for huawei and unknown
	}
}

// --- shared helpers ---

func collectProtocols(links []Link) map[string]bool {
	m := make(map[string]bool)
	for _, l := range links {
		for _, p := range l.Protocols {
			m[p] = true
		}
	}
	return m
}

func uniquePeers(links []Link) []string {
	seen := make(map[string]bool)
	var result []string
	for _, l := range links {
		if !seen[l.PeerDevice] {
			seen[l.PeerDevice] = true
			result = append(result, l.PeerDevice)
		}
	}
	return result
}
```

- [ ] **Step 4: Implement HuaweiGenerator**

```go
// internal/plan/command_huawei.go
package plan

import "fmt"

// HuaweiGenerator produces Huawei VRP CLI commands.
type HuaweiGenerator struct{}

func (g *HuaweiGenerator) CollectionCommands(target string, links []Link) []DeviceCommand {
	targetCmds := []string{"display interface brief", "display ip routing-table statistics"}
	protocols := collectProtocols(links)
	if protocols["ospf"] {
		targetCmds = append(targetCmds, "display ospf peer brief")
	}
	if protocols["bgp"] {
		targetCmds = append(targetCmds, "display bgp peer")
	}
	if protocols["ldp"] {
		targetCmds = append(targetCmds, "display mpls ldp session")
	}
	result := []DeviceCommand{
		{DeviceID: target, Commands: targetCmds, Purpose: "采集目标设备当前状态"},
	}
	for _, peerID := range uniquePeers(links) {
		peerCmds := []string{"display interface brief"}
		if protocols["ospf"] {
			peerCmds = append(peerCmds, "display ospf peer brief")
		}
		if protocols["bgp"] {
			peerCmds = append(peerCmds, "display bgp peer")
		}
		result = append(result, DeviceCommand{DeviceID: peerID, Commands: peerCmds, Purpose: "采集邻居设备当前状态"})
	}
	return result
}

func (g *HuaweiGenerator) PreCheckCommands(target string, links []Link) []DeviceCommand {
	cmds := []string{"display interface brief"}
	protocols := collectProtocols(links)
	if protocols["ospf"] {
		cmds = append(cmds, "display ospf peer brief")
	}
	if protocols["bgp"] {
		cmds = append(cmds, "display bgp peer")
	}
	if protocols["ldp"] {
		cmds = append(cmds, "display mpls ldp session")
	}
	return []DeviceCommand{{DeviceID: target, Commands: cmds, Purpose: "验证基线状态"}}
}

func (g *HuaweiGenerator) ProtocolIsolateCommands(target string, links []Link) []DeviceCommand {
	var cmds []string
	protocols := collectProtocols(links)
	cmds = append(cmds, "system-view")

	if protocols["ospf"] {
		seen := make(map[string]bool)
		for _, link := range links {
			if link.LocalDevice == target && link.LocalInterface != "" && !seen[link.LocalInterface] {
				seen[link.LocalInterface] = true
				cmds = append(cmds, fmt.Sprintf("interface %s", link.LocalInterface), "ospf cost 65535", "quit")
			}
		}
		cmds = append(cmds, "# 等待 OSPF 路由收敛（建议等待 60s）")
	}
	if protocols["bgp"] {
		for _, link := range links {
			if link.LocalDevice == target && link.PeerIP != "" {
				cmds = append(cmds, "bgp", fmt.Sprintf("peer %s ignore", link.PeerIP), "quit")
			}
		}
	}
	if protocols["ldp"] {
		seen := make(map[string]bool)
		for _, link := range links {
			if link.LocalDevice == target && link.LocalInterface != "" && !seen[link.LocalInterface] {
				seen[link.LocalInterface] = true
				cmds = append(cmds, fmt.Sprintf("interface %s", link.LocalInterface), "undo mpls ldp", "quit")
			}
		}
	}
	cmds = append(cmds, "return")

	result := []DeviceCommand{{DeviceID: target, Commands: cmds, Purpose: "协议级隔离 — 排干流量"}}
	var verifyCmds []string
	if protocols["ospf"] {
		verifyCmds = append(verifyCmds, "display ospf peer brief")
	}
	if protocols["bgp"] {
		verifyCmds = append(verifyCmds, "display bgp peer")
	}
	if len(verifyCmds) > 0 {
		result = append(result, DeviceCommand{DeviceID: target, Commands: verifyCmds, Purpose: "中间验证 — 确认协议排干"})
	}
	return result
}

func (g *HuaweiGenerator) InterfaceIsolateCommands(target string, links []Link) []DeviceCommand {
	var cmds []string
	cmds = append(cmds, "system-view")
	seen := make(map[string]bool)
	for _, link := range links {
		if link.LocalDevice == target && link.LocalInterface != "" && !seen[link.LocalInterface] {
			seen[link.LocalInterface] = true
			cmds = append(cmds, fmt.Sprintf("interface %s", link.LocalInterface), "shutdown", "quit")
		}
	}
	cmds = append(cmds, "return")
	return []DeviceCommand{
		{DeviceID: target, Commands: cmds, Purpose: "接口级隔离 — 关闭互联接口"},
		{DeviceID: target, Commands: []string{"display interface brief"}, Purpose: "验证接口已关闭"},
	}
}

func (g *HuaweiGenerator) PostCheckCommands(target string, links []Link) []DeviceCommand {
	protocols := collectProtocols(links)
	var result []DeviceCommand
	for _, peerID := range uniquePeers(links) {
		var cmds []string
		if protocols["ospf"] {
			cmds = append(cmds, "display ospf peer brief")
		}
		if protocols["bgp"] {
			cmds = append(cmds, "display bgp peer")
		}
		cmds = append(cmds, "display ip routing-table statistics")
		result = append(result, DeviceCommand{
			DeviceID: peerID, Commands: cmds,
			Purpose: fmt.Sprintf("变更后检查 — 确认 %s 已从邻居表消失", target),
		})
	}
	return result
}

func (g *HuaweiGenerator) RollbackCommands(target string, links []Link) []DeviceCommand {
	var cmds []string
	protocols := collectProtocols(links)
	cmds = append(cmds, "system-view")

	// Undo interface shutdown first
	seen := make(map[string]bool)
	for _, link := range links {
		if link.LocalDevice == target && link.LocalInterface != "" && !seen[link.LocalInterface] {
			seen[link.LocalInterface] = true
			cmds = append(cmds, fmt.Sprintf("interface %s", link.LocalInterface), "undo shutdown", "quit")
		}
	}
	// Undo OSPF cost
	if protocols["ospf"] {
		seen2 := make(map[string]bool)
		for _, link := range links {
			if link.LocalDevice == target && link.LocalInterface != "" && !seen2[link.LocalInterface] {
				seen2[link.LocalInterface] = true
				cmds = append(cmds, fmt.Sprintf("interface %s", link.LocalInterface), "undo ospf cost", "quit")
			}
		}
	}
	// Undo BGP peer ignore
	if protocols["bgp"] {
		for _, link := range links {
			if link.LocalDevice == target && link.PeerIP != "" {
				cmds = append(cmds, "bgp", fmt.Sprintf("undo peer %s ignore", link.PeerIP), "quit")
			}
		}
	}
	// Undo LDP disable
	if protocols["ldp"] {
		seen3 := make(map[string]bool)
		for _, link := range links {
			if link.LocalDevice == target && link.LocalInterface != "" && !seen3[link.LocalInterface] {
				seen3[link.LocalInterface] = true
				cmds = append(cmds, fmt.Sprintf("interface %s", link.LocalInterface), "mpls ldp", "quit")
			}
		}
	}
	cmds = append(cmds, "return")
	return []DeviceCommand{{DeviceID: target, Commands: cmds, Purpose: "回退 — 恢复所有变更"}}
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/plan/ -v -run TestHuawei`
Expected: All 6 PASS

- [ ] **Step 6: Commit**

```bash
git add internal/plan/command.go internal/plan/command_huawei.go internal/plan/command_test.go
git commit -m "feat(plan): add CommandGenerator interface and Huawei VRP implementation"
```

---

## Task 4: H3C Command Generator

**Files:**
- Create: `internal/plan/command_h3c.go`
- Modify: `internal/plan/command_test.go` (add H3C tests)

- [ ] **Step 1: Add H3C tests to command_test.go**

```go
func TestH3CGenerator_CollectionCommands(t *testing.T) {
	gen := &H3CGenerator{}
	links := []Link{
		{LocalDevice: "lc-01", LocalInterface: "FortyGigE2/0/27", PeerDevice: "la-01", Protocols: []string{"ospf"}},
	}
	cmds := gen.CollectionCommands("lc-01", links)
	if len(cmds) == 0 {
		t.Fatal("expected commands")
	}
	text := allCommandText(cmds)
	if !strings.Contains(text, "display ospf peer") {
		t.Error("expected 'display ospf peer' for H3C")
	}
}

func TestH3CGenerator_ProtocolIsolateCommands(t *testing.T) {
	gen := &H3CGenerator{}
	links := []Link{
		{LocalDevice: "lc-01", LocalInterface: "FortyGigE2/0/27", PeerDevice: "la-01", PeerIP: "10.0.0.2", Protocols: []string{"ospf", "bgp"}},
	}
	cmds := gen.ProtocolIsolateCommands("lc-01", links)
	text := allCommandText(cmds)
	if !strings.Contains(text, "ospf cost 65535") {
		t.Errorf("expected ospf cost in H3C output, got:\n%s", text)
	}
	if !strings.Contains(text, "peer 10.0.0.2 ignore") {
		t.Errorf("expected peer ignore in H3C output, got:\n%s", text)
	}
}

func TestH3CGenerator_RollbackCommands(t *testing.T) {
	gen := &H3CGenerator{}
	links := []Link{
		{LocalDevice: "lc-01", LocalInterface: "FortyGigE2/0/27", PeerDevice: "la-01", PeerIP: "10.0.0.2", Protocols: []string{"ospf"}},
	}
	cmds := gen.RollbackCommands("lc-01", links)
	text := allCommandText(cmds)
	if !strings.Contains(text, "undo shutdown") {
		t.Error("expected undo shutdown in H3C rollback")
	}
	if !strings.Contains(text, "undo ospf cost") {
		t.Error("expected undo ospf cost in H3C rollback")
	}
}
```

- [ ] **Step 2: Run test to verify fail**

Run: `go test ./internal/plan/ -v -run TestH3C`
Expected: FAIL (H3CGenerator undefined)

- [ ] **Step 3: Implement H3CGenerator**

H3C Comware 7 syntax is nearly identical to Huawei VRP. Main differences:
- `display ospf peer` (not `display ospf peer brief`)
- Trunk: `bridge-aggregation` (not `eth-trunk`)

```go
// internal/plan/command_h3c.go
package plan

import "fmt"

// H3CGenerator produces H3C Comware CLI commands.
type H3CGenerator struct{}

func (g *H3CGenerator) CollectionCommands(target string, links []Link) []DeviceCommand {
	targetCmds := []string{"display interface brief"}
	protocols := collectProtocols(links)
	if protocols["ospf"] {
		targetCmds = append(targetCmds, "display ospf peer")
	}
	if protocols["bgp"] {
		targetCmds = append(targetCmds, "display bgp peer")
	}
	if protocols["ldp"] {
		targetCmds = append(targetCmds, "display mpls ldp session")
	}
	result := []DeviceCommand{{DeviceID: target, Commands: targetCmds, Purpose: "采集目标设备当前状态"}}
	for _, peerID := range uniquePeers(links) {
		peerCmds := []string{"display interface brief"}
		if protocols["ospf"] {
			peerCmds = append(peerCmds, "display ospf peer")
		}
		if protocols["bgp"] {
			peerCmds = append(peerCmds, "display bgp peer")
		}
		result = append(result, DeviceCommand{DeviceID: peerID, Commands: peerCmds, Purpose: "采集邻居设备当前状态"})
	}
	return result
}

func (g *H3CGenerator) PreCheckCommands(target string, links []Link) []DeviceCommand {
	cmds := []string{"display interface brief"}
	protocols := collectProtocols(links)
	if protocols["ospf"] {
		cmds = append(cmds, "display ospf peer")
	}
	if protocols["bgp"] {
		cmds = append(cmds, "display bgp peer")
	}
	return []DeviceCommand{{DeviceID: target, Commands: cmds, Purpose: "验证基线状态"}}
}

func (g *H3CGenerator) ProtocolIsolateCommands(target string, links []Link) []DeviceCommand {
	var cmds []string
	protocols := collectProtocols(links)
	cmds = append(cmds, "system-view")
	if protocols["ospf"] {
		seen := make(map[string]bool)
		for _, link := range links {
			if link.LocalDevice == target && link.LocalInterface != "" && !seen[link.LocalInterface] {
				seen[link.LocalInterface] = true
				cmds = append(cmds, fmt.Sprintf("interface %s", link.LocalInterface), "ospf cost 65535", "quit")
			}
		}
		cmds = append(cmds, "# 等待 OSPF 路由收敛（建议等待 60s）")
	}
	if protocols["bgp"] {
		for _, link := range links {
			if link.LocalDevice == target && link.PeerIP != "" {
				cmds = append(cmds, "bgp", fmt.Sprintf("peer %s ignore", link.PeerIP), "quit")
			}
		}
	}
	cmds = append(cmds, "return")
	result := []DeviceCommand{{DeviceID: target, Commands: cmds, Purpose: "协议级隔离 — 排干流量"}}
	var verifyCmds []string
	if protocols["ospf"] {
		verifyCmds = append(verifyCmds, "display ospf peer")
	}
	if protocols["bgp"] {
		verifyCmds = append(verifyCmds, "display bgp peer")
	}
	if len(verifyCmds) > 0 {
		result = append(result, DeviceCommand{DeviceID: target, Commands: verifyCmds, Purpose: "中间验证"})
	}
	return result
}

func (g *H3CGenerator) InterfaceIsolateCommands(target string, links []Link) []DeviceCommand {
	var cmds []string
	cmds = append(cmds, "system-view")
	seen := make(map[string]bool)
	for _, link := range links {
		if link.LocalDevice == target && link.LocalInterface != "" && !seen[link.LocalInterface] {
			seen[link.LocalInterface] = true
			cmds = append(cmds, fmt.Sprintf("interface %s", link.LocalInterface), "shutdown", "quit")
		}
	}
	cmds = append(cmds, "return")
	return []DeviceCommand{
		{DeviceID: target, Commands: cmds, Purpose: "接口级隔离 — 关闭互联接口"},
		{DeviceID: target, Commands: []string{"display interface brief"}, Purpose: "验证接口已关闭"},
	}
}

func (g *H3CGenerator) PostCheckCommands(target string, links []Link) []DeviceCommand {
	protocols := collectProtocols(links)
	var result []DeviceCommand
	for _, peerID := range uniquePeers(links) {
		var cmds []string
		if protocols["ospf"] {
			cmds = append(cmds, "display ospf peer")
		}
		if protocols["bgp"] {
			cmds = append(cmds, "display bgp peer")
		}
		result = append(result, DeviceCommand{DeviceID: peerID, Commands: cmds, Purpose: fmt.Sprintf("变更后检查 — 确认 %s 已脱离", target)})
	}
	return result
}

func (g *H3CGenerator) RollbackCommands(target string, links []Link) []DeviceCommand {
	var cmds []string
	protocols := collectProtocols(links)
	cmds = append(cmds, "system-view")
	seen := make(map[string]bool)
	for _, link := range links {
		if link.LocalDevice == target && link.LocalInterface != "" && !seen[link.LocalInterface] {
			seen[link.LocalInterface] = true
			cmds = append(cmds, fmt.Sprintf("interface %s", link.LocalInterface), "undo shutdown", "quit")
		}
	}
	if protocols["ospf"] {
		seen2 := make(map[string]bool)
		for _, link := range links {
			if link.LocalDevice == target && link.LocalInterface != "" && !seen2[link.LocalInterface] {
				seen2[link.LocalInterface] = true
				cmds = append(cmds, fmt.Sprintf("interface %s", link.LocalInterface), "undo ospf cost", "quit")
			}
		}
	}
	if protocols["bgp"] {
		for _, link := range links {
			if link.LocalDevice == target && link.PeerIP != "" {
				cmds = append(cmds, "bgp", fmt.Sprintf("undo peer %s ignore", link.PeerIP), "quit")
			}
		}
	}
	cmds = append(cmds, "return")
	return []DeviceCommand{{DeviceID: target, Commands: cmds, Purpose: "回退 — 恢复所有变更"}}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/plan/ -v -run TestH3C`
Expected: All 3 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/plan/command_h3c.go internal/plan/command_test.go
git commit -m "feat(plan): add H3C Comware command generator"
```

---

## Task 5: Plan Generation Orchestrator (`isolate.go`)

**Files:**
- Create: `internal/plan/isolate.go`
- Create: `internal/plan/isolate_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/plan/isolate_test.go
package plan

import "testing"

func TestBuildIsolationPlan_BasicStructure(t *testing.T) {
	links := []Link{
		{
			LocalDevice: "lc-01", LocalInterface: "FortyGigE2/0/27",
			PeerDevice: "la-01", PeerIP: "10.0.0.2",
			Protocols: []string{"ospf", "bgp"}, Sources: []string{"description"},
		},
	}
	p := BuildIsolationPlan(PlanInput{
		TargetDevice: "lc-01", TargetHostname: "LC-01", TargetVendor: "huawei",
		Links: links, IsSPOF: true, ImpactDevices: []string{"LA-01", "QCDR-01"},
	})

	if p.TargetDevice != "lc-01" {
		t.Errorf("TargetDevice = %q, want lc-01", p.TargetDevice)
	}
	if len(p.Phases) != 6 {
		t.Fatalf("got %d phases, want 6", len(p.Phases))
	}
	wantNames := []string{"方案规划", "预检查", "协议级隔离", "接口级隔离", "变更后检查", "回退方案"}
	for i, name := range wantNames {
		if p.Phases[i].Name != name {
			t.Errorf("Phase %d name = %q, want %q", i, p.Phases[i].Name, name)
		}
	}
	if !p.IsSPOF {
		t.Error("expected IsSPOF=true")
	}
	if p.GeneratedAt.IsZero() {
		t.Error("GeneratedAt should not be zero")
	}
	// Phase 0 should have commands for both target and peer
	if len(p.Phases[0].Steps) < 2 {
		t.Errorf("Phase 0 should have steps for target + peer, got %d", len(p.Phases[0].Steps))
	}
}

func TestBuildIsolationPlan_H3CVendor(t *testing.T) {
	links := []Link{
		{LocalDevice: "lc-01", LocalInterface: "FG2/0/1", PeerDevice: "la-01", Protocols: []string{"ospf"}},
	}
	p := BuildIsolationPlan(PlanInput{
		TargetDevice: "lc-01", TargetHostname: "LC-01", TargetVendor: "h3c",
		Links: links,
	})
	// H3C should use "display ospf peer" (not "display ospf peer brief")
	for _, step := range p.Phases[0].Steps {
		for _, cmd := range step.Commands {
			if cmd == "display ospf peer brief" {
				t.Error("H3C should use 'display ospf peer', not 'display ospf peer brief'")
			}
		}
	}
}
```

- [ ] **Step 2: Run test to verify fail**

Run: `go test ./internal/plan/ -v -run TestBuildIsolationPlan`
Expected: FAIL

- [ ] **Step 3: Implement BuildIsolationPlan**

```go
// internal/plan/isolate.go
package plan

import "time"

// PlanInput contains the inputs for building an isolation plan.
type PlanInput struct {
	TargetDevice   string
	TargetHostname string
	TargetVendor   string
	Links          []Link
	IsSPOF         bool
	ImpactDevices  []string
}

// BuildIsolationPlan creates a complete 6-phase isolation plan.
func BuildIsolationPlan(input PlanInput) Plan {
	gen := GeneratorForVendor(input.TargetVendor)

	plan := Plan{
		TargetDevice:   input.TargetDevice,
		TargetHostname: input.TargetHostname,
		TargetVendor:   input.TargetVendor,
		Links:          input.Links,
		IsSPOF:         input.IsSPOF,
		ImpactDevices:  input.ImpactDevices,
		GeneratedAt:    time.Now(),
	}

	phase0 := Phase{
		Number: 0, Name: "方案规划",
		Description: "基于配置推断互联关系，输出需人工采集的命令清单",
		Steps:       gen.CollectionCommands(input.TargetDevice, input.Links),
	}
	if input.IsSPOF {
		phase0.Notes = append(phase0.Notes, "⚠️ 目标设备是单点故障(SPOF)，隔离后将影响以下设备的可达性")
	}

	phase1 := Phase{
		Number: 1, Name: "预检查",
		Description: "验证基线状态，确认可安全隔离",
		Steps:       gen.PreCheckCommands(input.TargetDevice, input.Links),
		Notes: []string{
			"确认所有 OSPF 邻居状态为 Full/2-Way",
			"确认所有 BGP 邻居状态为 Established",
			"确认所有互联接口 oper-status 为 Up",
		},
	}

	phase2 := Phase{
		Number: 2, Name: "协议级隔离",
		Description: "调整协议参数排干流量",
		Steps:       gen.ProtocolIsolateCommands(input.TargetDevice, input.Links),
		Notes:       []string{"执行后等待路由收敛，建议至少等待 60 秒"},
	}

	phase3 := Phase{
		Number: 3, Name: "接口级隔离",
		Description: "关闭互联物理接口",
		Steps:       gen.InterfaceIsolateCommands(input.TargetDevice, input.Links),
	}

	phase4 := Phase{
		Number: 4, Name: "变更后检查",
		Description: "在邻居设备上验证目标设备已脱离网络",
		Steps:       gen.PostCheckCommands(input.TargetDevice, input.Links),
		Notes:       []string{"确认目标设备已从邻居表消失", "确认路由已收敛到备用路径"},
	}

	phase5 := Phase{
		Number: 5, Name: "回退方案",
		Description: "如需回退，按以下步骤恢复（先恢复接口，再恢复协议）",
		Steps:       gen.RollbackCommands(input.TargetDevice, input.Links),
		Notes:       []string{"回退后等待协议收敛，确认邻居关系恢复正常"},
	}

	plan.Phases = []Phase{phase0, phase1, phase2, phase3, phase4, phase5}
	return plan
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/plan/ -v -run TestBuildIsolationPlan`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/plan/isolate.go internal/plan/isolate_test.go
git commit -m "feat(plan): add BuildIsolationPlan orchestrator"
```

---

## Task 6: Text and Markdown Renderer

**Files:**
- Create: `internal/plan/render.go`
- Create: `internal/plan/render_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/plan/render_test.go
package plan

import (
	"strings"
	"testing"
	"time"
)

func newTestPlan() Plan {
	return Plan{
		TargetDevice: "lc-01", TargetHostname: "LC-01", TargetVendor: "huawei",
		Links: []Link{
			{LocalDevice: "lc-01", LocalInterface: "FortyGigE2/0/27", PeerDevice: "la-01",
				Protocols: []string{"ospf"}, Sources: []string{"description"}},
		},
		IsSPOF: true, ImpactDevices: []string{"LA-01"},
		Phases: []Phase{
			{Number: 0, Name: "方案规划", Steps: []DeviceCommand{
				{DeviceID: "lc-01", Commands: []string{"display ospf peer brief"}, Purpose: "采集"},
			}},
		},
		GeneratedAt: time.Date(2026, 3, 22, 14, 0, 0, 0, time.Local),
	}
}

func TestRenderText(t *testing.T) {
	p := newTestPlan()
	out := RenderText(p)
	for _, want := range []string{"LC-01", "方案规划", "display ospf peer brief", "SPOF"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in text output, got:\n%s", want, out)
		}
	}
}

func TestRenderMarkdown(t *testing.T) {
	p := newTestPlan()
	out := RenderMarkdown(p)
	for _, want := range []string{"# ", "LC-01", "```", "SPOF"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in markdown output, got:\n%s", want, out)
		}
	}
}
```

- [ ] **Step 2: Run test to verify fail**

Run: `go test ./internal/plan/ -v -run TestRender`
Expected: FAIL

- [ ] **Step 3: Implement renderers**

```go
// internal/plan/render.go
package plan

import (
	"fmt"
	"strings"
)

// RenderText produces a plain-text change plan.
func RenderText(p Plan) string {
	var b strings.Builder
	b.WriteString("=== 设备隔离变更方案 ===\n")
	b.WriteString(fmt.Sprintf("目标设备: %s (%s)\n", p.TargetHostname, p.TargetVendor))
	b.WriteString(fmt.Sprintf("生成时间: %s\n", p.GeneratedAt.Format("2006-01-02 15:04")))
	b.WriteString(fmt.Sprintf("互联设备: %d 台\n", len(p.Links)))
	if p.IsSPOF {
		b.WriteString(fmt.Sprintf("影响评估: ⚠️ SPOF — 移除后 %d 台设备受影响\n", len(p.ImpactDevices)))
	}

	b.WriteString("\n互联关系:\n")
	for _, l := range p.Links {
		peerIf := l.PeerInterface
		if peerIf == "" {
			peerIf = "—"
		}
		b.WriteString(fmt.Sprintf("  %s %s  <-->  %s %s  (来源: %s, 协议: %s)\n",
			l.LocalDevice, l.LocalInterface, l.PeerDevice, peerIf,
			strings.Join(l.Sources, "+"), strings.Join(l.Protocols, ", ")))
	}

	for _, phase := range p.Phases {
		b.WriteString(fmt.Sprintf("\n─── 阶段%d: %s ───\n", phase.Number, phase.Name))
		if phase.Description != "" {
			b.WriteString(fmt.Sprintf("  %s\n", phase.Description))
		}
		for _, note := range phase.Notes {
			b.WriteString(fmt.Sprintf("  > %s\n", note))
		}
		for _, step := range phase.Steps {
			b.WriteString(fmt.Sprintf("\n  [%s] %s\n", step.DeviceID, step.Purpose))
			for _, cmd := range step.Commands {
				b.WriteString(fmt.Sprintf("    %s\n", cmd))
			}
		}
	}
	return b.String()
}

// RenderMarkdown produces a Markdown-formatted change plan.
func RenderMarkdown(p Plan) string {
	var b strings.Builder
	b.WriteString("# 设备隔离变更方案\n\n")
	b.WriteString(fmt.Sprintf("**目标设备:** %s (%s)\n", p.TargetHostname, p.TargetVendor))
	b.WriteString(fmt.Sprintf("**生成时间:** %s\n", p.GeneratedAt.Format("2006-01-02 15:04")))
	if p.IsSPOF {
		b.WriteString(fmt.Sprintf("**影响评估:** ⚠️ SPOF — 移除后 %d 台设备受影响\n", len(p.ImpactDevices)))
	}

	b.WriteString("\n## 互联关系\n\n")
	b.WriteString("| 本端接口 | 对端设备 | 对端接口 | 协议 | 来源 |\n")
	b.WriteString("|----------|---------|---------|------|------|\n")
	for _, l := range p.Links {
		peerIf := l.PeerInterface
		if peerIf == "" {
			peerIf = "—"
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
			l.LocalInterface, l.PeerDevice, peerIf,
			strings.Join(l.Protocols, ", "), strings.Join(l.Sources, "+")))
	}

	for _, phase := range p.Phases {
		b.WriteString(fmt.Sprintf("\n## 阶段%d: %s\n\n", phase.Number, phase.Name))
		if phase.Description != "" {
			b.WriteString(phase.Description + "\n\n")
		}
		for _, note := range phase.Notes {
			b.WriteString(fmt.Sprintf("> %s\n", note))
		}
		if len(phase.Notes) > 0 {
			b.WriteString("\n")
		}
		for _, step := range phase.Steps {
			b.WriteString(fmt.Sprintf("### %s — %s\n\n```\n", step.DeviceID, step.Purpose))
			for _, cmd := range step.Commands {
				b.WriteString(cmd + "\n")
			}
			b.WriteString("```\n\n")
		}
	}
	return b.String()
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/plan/ -v -run TestRender`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/plan/render.go internal/plan/render_test.go
git commit -m "feat(plan): add text and markdown renderers"
```

---

## Task 7: CLI Command (`plan isolate`) + `--check` Implementation

**Files:**
- Create: `internal/cli/plan.go`
- Modify: `internal/cli/root.go` (add `root.AddCommand(newPlanCmd())`)

**Key codebase APIs (verified from trace.go pattern):**
- `graph.BuildFromDB(db)` returns `(*graph.Graph, error)` — global `db` var
- `graph.ImpactAnalysis(g, nodeID, graph.NodeTypeDevice)` returns `[]string`
- `db.GetDevice(id)` returns `(model.Device, error)`
- `db.LatestSnapshotID(deviceID)` returns `(int, error)`
- `db.GetNeighbors(deviceID, snapshotID)` returns `([]model.NeighborInfo, error)`
- `db.GetInterfaces(deviceID)` returns `([]model.Interface, error)`
- `db.InsertScratch(model.ScratchEntry)` returns `(int, error)`
- `db.ListScratch(deviceID, category, limit)` returns `([]model.ScratchEntry, error)`
- Root command uses `root.AddCommand(...)` (local var in `NewRootCmd()`)

- [ ] **Step 1: Create the CLI command with --check support**

```go
// internal/cli/plan.go
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/graph"
	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/plan"
)

func newPlanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Generate change plans",
	}
	cmd.AddCommand(newPlanIsolateCmd())
	return cmd
}

func newPlanIsolateCmd() *cobra.Command {
	var (
		format string
		check  bool
		output string
	)
	cmd := &cobra.Command{
		Use:   "isolate <device-id>",
		Short: "Generate a device isolation change plan",
		Long: `Generate a structured, multi-phase change plan for isolating a network device.

First run (without --check): outputs interconnection analysis and collection commands.
Second run (with --check): includes pre-check validation based on collected data.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deviceID := strings.ToLower(args[0])

			// Verify device exists
			device, err := db.GetDevice(deviceID)
			if err != nil {
				return fmt.Errorf("device %q not found — run 'nethelper show device' to list: %w", deviceID, err)
			}

			// Discover links
			links, err := plan.DiscoverLinks(db, deviceID)
			if err != nil {
				return fmt.Errorf("discover links: %w", err)
			}
			if len(links) == 0 {
				fmt.Fprintf(os.Stderr, "⚠️  未发现 %s 的互联关系，建议补充数据\n", device.Hostname)
			}

			// Impact analysis (same pattern as trace.go)
			g, err := graph.BuildFromDB(db)
			if err != nil {
				return fmt.Errorf("build graph: %w", err)
			}
			affected := graph.ImpactAnalysis(g, deviceID, graph.NodeTypeDevice)
			isSPOF := len(affected) > 0
			var impactNames []string
			for _, id := range affected {
				n, ok := g.GetNode(id)
				if ok && n.Props["hostname"] != "" {
					impactNames = append(impactNames, n.Props["hostname"])
				} else {
					impactNames = append(impactNames, id)
				}
			}

			// Build plan
			p := plan.BuildIsolationPlan(plan.PlanInput{
				TargetDevice:   deviceID,
				TargetHostname: device.Hostname,
				TargetVendor:   device.Vendor,
				Links:          links,
				IsSPOF:         isSPOF,
				ImpactDevices:  impactNames,
			})

			// --check: run pre-check validation
			if check {
				result, err := runPreCheck(deviceID, links)
				if err != nil {
					fmt.Fprintf(os.Stderr, "⚠️  预检查失败: %v\n", err)
				} else {
					// Add pre-check results to Phase 1 notes
					p.Phases[1].Notes = append(p.Phases[1].Notes, formatPreCheckResult(result)...)
					// Persist baseline to scratch_entries
					baseline, _ := json.Marshal(result)
					db.InsertScratch(model.ScratchEntry{
						DeviceID: deviceID,
						Category: "plan-baseline",
						Query:    deviceID,
						Content:  string(baseline),
					})
				}
			}

			// Render
			var rendered string
			switch format {
			case "markdown":
				rendered = plan.RenderMarkdown(p)
			default:
				rendered = plan.RenderText(p)
			}

			if output != "" {
				if err := os.WriteFile(output, []byte(rendered), 0644); err != nil {
					return fmt.Errorf("write output: %w", err)
				}
				fmt.Printf("变更方案已保存到 %s\n", output)
			} else {
				fmt.Print(rendered)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "Output format: text or markdown")
	cmd.Flags().BoolVar(&check, "check", false, "Enable pre-check validation (requires collected data)")
	cmd.Flags().StringVar(&output, "output", "", "Write output to file")
	return cmd
}

// runPreCheck validates the baseline state by querying latest snapshot data.
func runPreCheck(deviceID string, links []plan.Link) (plan.PreCheckResult, error) {
	result := plan.PreCheckResult{DeviceID: deviceID}

	snapID, err := db.LatestSnapshotID(deviceID)
	if err != nil {
		return result, fmt.Errorf("no snapshot for %s — run collection commands first: %w", deviceID, err)
	}

	// Check snapshot freshness (warn if >24h)
	// Note: snapshots table has captured_at but we'd need a GetSnapshot method.
	// For now, skip freshness check — can be added later.

	// Check OSPF neighbors
	neighbors, err := db.GetNeighbors(deviceID, snapID)
	if err == nil {
		ospfAllFull := true
		bgpAllEstab := true
		for _, n := range neighbors {
			if strings.EqualFold(n.Protocol, "ospf") {
				result.OSPFPeerCount++
				state := strings.ToLower(n.State)
				if state != "full" && state != "2-way" && state != "full/" {
					ospfAllFull = false
					result.Warnings = append(result.Warnings,
						fmt.Sprintf("OSPF 邻居 %s 状态异常: %s", n.RemoteID, n.State))
				}
			}
			if strings.EqualFold(n.Protocol, "bgp") {
				result.BGPPeerCount++
				state := strings.ToLower(n.State)
				if state != "established" {
					bgpAllEstab = false
					result.Warnings = append(result.Warnings,
						fmt.Sprintf("BGP 邻居 %s 状态异常: %s", n.RemoteAddress, n.State))
				}
			}
		}
		result.OSPFAllFull = ospfAllFull
		result.BGPAllEstab = bgpAllEstab
	}

	// Check interfaces
	ifaces, err := db.GetInterfaces(deviceID)
	if err == nil {
		result.InterfaceTotal = len(ifaces)
		for _, iface := range ifaces {
			if strings.EqualFold(iface.Status, "up") {
				result.InterfaceUp++
			}
		}
	}

	result.Safe = result.OSPFAllFull && result.BGPAllEstab && len(result.Warnings) == 0
	return result, nil
}

func formatPreCheckResult(r plan.PreCheckResult) []string {
	var notes []string
	if r.Safe {
		notes = append(notes, "✅ 预检查通过 — 可安全执行隔离")
	} else {
		notes = append(notes, "⚠️ 预检查发现异常 — 请确认后再执行")
	}
	notes = append(notes, fmt.Sprintf("OSPF 邻居: %d 个 (全部 Full: %v)", r.OSPFPeerCount, r.OSPFAllFull))
	notes = append(notes, fmt.Sprintf("BGP 邻居: %d 个 (全部 Established: %v)", r.BGPPeerCount, r.BGPAllEstab))
	notes = append(notes, fmt.Sprintf("接口: %d/%d Up", r.InterfaceUp, r.InterfaceTotal))
	for _, w := range r.Warnings {
		notes = append(notes, "⚠️ "+w)
	}
	return notes
}
```

- [ ] **Step 2: Register in root.go**

In `internal/cli/root.go`, add after the last `root.AddCommand(...)` line:

```go
root.AddCommand(newPlanCmd())
```

- [ ] **Step 3: Build**

Run: `go build ./cmd/nethelper`
Expected: Success

- [ ] **Step 4: Smoke test help**

Run: `./nethelper plan isolate --help`
Expected: Help text with --format, --check, --output flags

- [ ] **Step 5: Commit**

```bash
git add internal/cli/plan.go internal/cli/root.go
git commit -m "feat(cli): add 'plan isolate' command with --check pre-validation"
```

---

## Task 8: Integration Smoke Test with Real Data

- [ ] **Step 1: Run against real data (no --check)**

Run: `./nethelper plan isolate cd-gx-0201-g17-h12516af-lc-01`
Expected: Output showing discovered links (at least LA and QCDR), 6 phases with Huawei/H3C commands.

- [ ] **Step 2: Run with markdown format**

Run: `./nethelper plan isolate cd-gx-0201-g17-h12516af-lc-01 --format markdown`
Expected: Markdown with tables and code blocks.

- [ ] **Step 3: Run with --check**

Run: `./nethelper plan isolate cd-gx-0201-g17-h12516af-lc-01 --check`
Expected: Same output plus pre-check results in Phase 1 notes (may show warnings if neighbor data is missing).

- [ ] **Step 4: Run with output to file**

Run: `./nethelper plan isolate cd-gx-0201-g17-h12516af-lc-01 --format markdown --output /tmp/lc-plan.md`
Expected: File created successfully.

- [ ] **Step 5: Run all tests**

Run: `go test ./... 2>&1 | tail -30`
Expected: All tests pass.

- [ ] **Step 6: Run vet**

Run: `go vet ./...`
Expected: No issues.

- [ ] **Step 7: Fix any issues and commit**

```bash
git add -A
git commit -m "fix(plan): integration fixes from smoke testing"
```

---

## Task 9: Three-Role Collaboration — Network Engineer Review

Performed by a **network engineer** subagent:

- [ ] **Step 1: Run `nethelper plan isolate cd-gx-0201-g17-h12516af-lc-01` and capture output**
- [ ] **Step 2: Review each phase for correctness:**
  - Phase 0: Are collection commands complete and appropriate for Huawei VRP / H3C Comware?
  - Phase 2: Is OSPF cost 65535 correct? Is `peer X ignore` the right BGP isolation command?
  - Phase 3: Are the correct interfaces being shut down?
  - Phase 5: Are rollback commands the correct inverse (undo shutdown, undo ospf cost, undo peer ignore)?
- [ ] **Step 3: Check if the discovered links match the actual topology (LC connects LA and QCDR)**
- [ ] **Step 4: Document issues or approve**

---

## Task 10: Three-Role Collaboration — Test Engineer Validation

Performed by a **test engineer** subagent:

- [ ] **Step 1: Review test coverage in `internal/plan/`**
- [ ] **Step 2: Add missing edge case tests:**
  - Device with no interfaces → DiscoverLinks returns empty
  - Device not in DB → CLI returns clear error
  - Links with empty PeerIP → BGP commands skip gracefully (already tested)
  - Multiple links to same peer → deduplication works
  - Unknown vendor → falls back to Huawei generator
- [ ] **Step 3: Run `go test ./internal/plan/ -v -cover`**
- [ ] **Step 4: Report coverage and issues**

---

## Task 11: Three-Role Alignment and Final LC Isolation Plan

- [ ] **Step 1: Address all feedback from Tasks 9 and 10**
- [ ] **Step 2: Generate final plan**

Run: `./nethelper plan isolate cd-gx-0201-g17-h12516af-lc-01 --format markdown --output docs/lc-isolation-plan.md`

- [ ] **Step 3: All three roles confirm the output is correct and complete**
- [ ] **Step 4: Final commit**

```bash
git add docs/lc-isolation-plan.md
git commit -m "docs: add LC device isolation change plan generated by nethelper"
```
