# `plan isolate` v2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the generic template-based isolation plan generator with a topology-aware engine that produces per-peer, per-interface commands based on what the device actually runs.

**Architecture:** New `BuildTopology()` constructs a `DeviceTopology` from DB data (BGP peers, interfaces, config, VRFs). New `GenerateIsolationPlanV2()` consumes the topology to produce a `Plan` with per-peer-group steps, checkpoints, and role-based ordering (downlink → uplink → management). CLI wiring switches to the new path.

**Tech Stack:** Go, SQLite (existing `store.DB`), existing `graph` package for impact analysis.

**Spec:** `docs/superpowers/specs/2026-03-22-plan-isolate-v2-design.md`

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/plan/topology.go` | Create | `DeviceTopology` + all sub-types + `BuildTopology()` |
| `internal/plan/topology_test.go` | Create | Tests for topology building |
| `internal/plan/isolate_v2.go` | Create | `GenerateIsolationPlanV2()` — consumes topology, produces Plan |
| `internal/plan/isolate_v2_test.go` | Create | Tests for v2 plan generation |
| `internal/plan/commands_bgp.go` | Create | BGP isolation/rollback command generation per vendor |
| `internal/plan/commands_bgp_test.go` | Create | Tests for BGP commands |
| `internal/plan/commands_iface.go` | Create | Interface isolation/rollback command generation |
| `internal/cli/plan.go` | Modify | Switch to `BuildTopology` → `GenerateIsolationPlanV2` |
| `internal/plan/render.go` | Modify | Minor: handle multi-step phases with checkpoints |

---

## Task 1: DeviceTopology data types

**Files:**
- Create: `internal/plan/topology.go`

- [ ] **Step 1: Define all topology types**

```go
// internal/plan/topology.go
package plan

import "github.com/xavierli/nethelper/internal/store"

// PeerGroupRole classifies a BGP peer group's function in the network.
type PeerGroupRole string

const (
	RoleDownlink   PeerGroupRole = "downlink"
	RoleUplink     PeerGroupRole = "uplink"
	RoleManagement PeerGroupRole = "management"
)

// BGPPeerDetail holds one BGP peer with its associated physical interface.
type BGPPeerDetail struct {
	PeerIP      string
	RemoteAS    int
	Description string // peer description from config (usually remote device name)
	Interface   string // matched local interface name (empty if no match)
}

// PeerGroup is a BGP peer-group with all its members and inferred role.
type PeerGroup struct {
	Name  string
	Type  string        // "external" / "internal"
	Role  PeerGroupRole // inferred: downlink / uplink / management
	Peers []BGPPeerDetail
}

// LAGBundle represents a Link Aggregation Group.
type LAGBundle struct {
	Name        string   // e.g. "Route-Aggregation1", "Eth-Trunk1"
	IP          string
	Mask        string
	Description string   // peer device name
	Members     []string // member physical interface names
}

// PhysicalLink represents a routed physical interface.
type PhysicalLink struct {
	Interface   string
	IP          string
	Mask        string
	Description string // peer device + interface from description field
	PeerGroup   string // associated BGP peer group (empty if none)
}

// StaticRouteEntry represents a parsed static route.
type StaticRouteEntry struct {
	Prefix    string
	NextHop   string
	Interface string
	VRF       string
}

// VRFSummary is a simplified VRF record.
type VRFSummary struct {
	Name string
	RD   string
}

// DeviceTopology is the complete interconnection view of a device.
type DeviceTopology struct {
	DeviceID   string
	Hostname   string
	Vendor     string
	LocalAS    int           // 0 if no BGP
	Protocols  []string      // e.g. ["bgp"] or ["isis","ldp","bgp"]

	PeerGroups    []PeerGroup
	LAGs          []LAGBundle
	PhysicalLinks []PhysicalLink
	StaticRoutes  []StaticRouteEntry
	VRFs          []VRFSummary

	IsSPOF        bool
	ImpactDevices []string
}

// BuildTopology constructs a DeviceTopology from database data.
func BuildTopology(db *store.DB, deviceID string) (DeviceTopology, error) {
	// Implementation in next step
	return DeviceTopology{}, nil
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/plan/`
Expected: Success

- [ ] **Step 3: Commit**

```bash
git add internal/plan/topology.go
git commit -m "feat(plan): add DeviceTopology types for multi-dimensional link discovery"
```

---

## Task 2: BuildTopology implementation

**Files:**
- Modify: `internal/plan/topology.go`
- Create: `internal/plan/topology_test.go`

**Codebase APIs (verified):**
- `db.GetDevice(id) (model.Device, error)`
- `db.LatestSnapshotID(deviceID) (int, error)`
- `db.GetBGPPeers(deviceID, snapshotID) ([]model.BGPPeer, error)`
- `db.GetInterfaces(deviceID) ([]model.Interface, error)`
- `db.GetConfigSnapshots(deviceID) ([]model.ConfigSnapshot, error)`
- `db.GetVRFInstances(deviceID) ([]model.VRFInstance, error)`
- `graph.BuildFromDB(db) (*Graph, error)`
- `graph.ImpactAnalysis(g, nodeID, graph.NodeTypeDevice) []string`
- `model.BGPPeer` has: `.PeerIP`, `.RemoteAS`, `.PeerGroup`, `.Description`, `.LocalAS`
- `model.Interface` has: `.Name`, `.Type`, `.IPAddress`, `.Mask`, `.Description`, `.ParentID`, `.Status`
- `model.InterfaceType`: `IfTypeEthTrunk`, `IfTypePhysical`, `IfTypeLoopback`, `IfTypeNull`

- [ ] **Step 1: Write failing tests**

```go
// internal/plan/topology_test.go
package plan_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/plan"
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
	t.Cleanup(func() { db.Close(); os.Remove(dbPath) })
	return db
}

func TestBuildTopology_BGPPeerGroups(t *testing.T) {
	db := setupTestDB(t)

	db.UpsertDevice(model.Device{ID: "lc-01", Hostname: "LC-01", Vendor: "h3c"})
	snap, _ := db.CreateSnapshot(model.Snapshot{DeviceID: "lc-01", Commands: "[]"})
	db.InsertBGPPeers([]model.BGPPeer{
		{DeviceID: "lc-01", LocalAS: 65508, PeerIP: "10.0.0.1", RemoteAS: 1001, PeerGroup: "LA1", Description: "R08-LA-01", SnapshotID: snap},
		{DeviceID: "lc-01", LocalAS: 65508, PeerIP: "10.0.0.2", RemoteAS: 1001, PeerGroup: "LA1", Description: "R09-LA-01", SnapshotID: snap},
		{DeviceID: "lc-01", LocalAS: 65508, PeerIP: "10.1.0.1", RemoteAS: 45090, PeerGroup: "QCDR", Description: "QCDR-01", SnapshotID: snap},
		{DeviceID: "lc-01", LocalAS: 65508, PeerIP: "10.2.0.1", RemoteAS: 65508, PeerGroup: "SDN-Controller", Description: "SDN", SnapshotID: snap},
	})

	topo, err := plan.BuildTopology(db, "lc-01")
	if err != nil {
		t.Fatalf("BuildTopology: %v", err)
	}
	if topo.LocalAS != 65508 {
		t.Errorf("LocalAS = %d, want 65508", topo.LocalAS)
	}
	if len(topo.PeerGroups) != 3 {
		t.Fatalf("expected 3 peer groups, got %d", len(topo.PeerGroups))
	}
	// Verify peer counts per group
	for _, pg := range topo.PeerGroups {
		switch pg.Name {
		case "LA1":
			if len(pg.Peers) != 2 { t.Errorf("LA1 should have 2 peers, got %d", len(pg.Peers)) }
			if pg.Role != plan.RoleDownlink { t.Errorf("LA1 role = %s, want downlink", pg.Role) }
		case "QCDR":
			if len(pg.Peers) != 1 { t.Errorf("QCDR should have 1 peer, got %d", len(pg.Peers)) }
		case "SDN-Controller":
			if pg.Role != plan.RoleManagement { t.Errorf("SDN role = %s, want management", pg.Role) }
		}
	}
}

func TestBuildTopology_PeerInterfaceMapping(t *testing.T) {
	db := setupTestDB(t)

	db.UpsertDevice(model.Device{ID: "dev-a", Hostname: "DEV-A", Vendor: "huawei"})
	db.UpsertInterfaces("dev-a", []model.Interface{
		{ID: "dev-a:GE0/0/1", Name: "GE0/0/1", Type: model.IfTypePhysical, Status: "up",
			IPAddress: "10.0.0.2", Mask: "30", Description: "PEER-B-GE0/0/1"},
	})
	snap, _ := db.CreateSnapshot(model.Snapshot{DeviceID: "dev-a", Commands: "[]"})
	db.InsertBGPPeers([]model.BGPPeer{
		{DeviceID: "dev-a", LocalAS: 100, PeerIP: "10.0.0.1", RemoteAS: 200, PeerGroup: "PEERS", Description: "PEER-B", SnapshotID: snap},
	})

	topo, err := plan.BuildTopology(db, "dev-a")
	if err != nil { t.Fatalf("BuildTopology: %v", err) }

	if len(topo.PeerGroups) != 1 { t.Fatalf("expected 1 group, got %d", len(topo.PeerGroups)) }
	if topo.PeerGroups[0].Peers[0].Interface != "GE0/0/1" {
		t.Errorf("peer interface = %q, want GE0/0/1", topo.PeerGroups[0].Peers[0].Interface)
	}
}

func TestBuildTopology_LAGDetection(t *testing.T) {
	db := setupTestDB(t)

	db.UpsertDevice(model.Device{ID: "dev-a", Hostname: "DEV-A", Vendor: "h3c"})
	db.UpsertInterfaces("dev-a", []model.Interface{
		{ID: "dev-a:Route-Aggregation1", Name: "Route-Aggregation1", Type: model.IfTypeEthTrunk,
			Status: "up", IPAddress: "10.1.0.1", Mask: "30", Description: "UPLINK-QCMAN"},
		{ID: "dev-a:FGE2/0/24", Name: "FortyGigE2/0/24", Type: model.IfTypePhysical,
			Status: "up", ParentID: "dev-a:Route-Aggregation1", Description: "QCMAN-FG1/0/1"},
		{ID: "dev-a:FGE2/0/35", Name: "FortyGigE2/0/35", Type: model.IfTypePhysical,
			Status: "up", ParentID: "dev-a:Route-Aggregation1"},
	})

	topo, err := plan.BuildTopology(db, "dev-a")
	if err != nil { t.Fatalf("BuildTopology: %v", err) }

	if len(topo.LAGs) != 1 { t.Fatalf("expected 1 LAG, got %d", len(topo.LAGs)) }
	lag := topo.LAGs[0]
	if lag.Name != "Route-Aggregation1" { t.Errorf("LAG name = %s", lag.Name) }
	if len(lag.Members) != 2 { t.Errorf("LAG members = %d, want 2", len(lag.Members)) }
}

func TestBuildTopology_ProtocolDetection(t *testing.T) {
	db := setupTestDB(t)

	db.UpsertDevice(model.Device{ID: "dev-a", Hostname: "DEV-A", Vendor: "huawei"})
	snap, _ := db.CreateSnapshot(model.Snapshot{DeviceID: "dev-a", Commands: "[]"})
	// Has BGP peers → protocols should include "bgp"
	db.InsertBGPPeers([]model.BGPPeer{
		{DeviceID: "dev-a", LocalAS: 100, PeerIP: "1.1.1.1", RemoteAS: 200, PeerGroup: "X", SnapshotID: snap},
	})
	// Has config with isis → protocols should include "isis"
	db.InsertConfigSnapshot(model.ConfigSnapshot{
		DeviceID: "dev-a", ConfigText: "#\nisis 1\n is-level level-2\n#\nmpls ldp\n#",
	})

	topo, err := plan.BuildTopology(db, "dev-a")
	if err != nil { t.Fatalf("BuildTopology: %v", err) }

	has := func(p string) bool {
		for _, x := range topo.Protocols { if x == p { return true } }; return false
	}
	if !has("bgp") { t.Error("expected bgp in protocols") }
	if !has("isis") { t.Error("expected isis in protocols") }
	if !has("ldp") { t.Error("expected ldp in protocols") }
}

func TestBuildTopology_NoBGP(t *testing.T) {
	db := setupTestDB(t)
	db.UpsertDevice(model.Device{ID: "sw-01", Hostname: "SW-01", Vendor: "huawei"})

	topo, err := plan.BuildTopology(db, "sw-01")
	if err != nil { t.Fatalf("BuildTopology: %v", err) }

	if topo.LocalAS != 0 { t.Errorf("LocalAS should be 0 for device without BGP") }
	if len(topo.PeerGroups) != 0 { t.Errorf("expected 0 peer groups") }
}
```

- [ ] **Step 2: Run tests — should fail**

Run: `go test ./internal/plan/ -v -run TestBuildTopology`
Expected: FAIL (BuildTopology returns empty)

- [ ] **Step 3: Implement BuildTopology**

The full implementation of `BuildTopology` in `topology.go`. Key steps:

1. `db.GetDevice(deviceID)` → fill DeviceID, Hostname, Vendor
2. `db.LatestSnapshotID(deviceID)` → get latest snapshot
3. `db.GetBGPPeers(deviceID, snapID)` → aggregate into PeerGroups by `.PeerGroup` field. Set `LocalAS` from first peer's `.LocalAS`. Set Type: if peer.RemoteAS == LocalAS → "internal", else "external".
4. `db.GetInterfaces(deviceID)` → separate into LAGs (Type==IfTypeEthTrunk) and PhysicalLinks (Type==IfTypePhysical with IPAddress). Build parent→children map for LAG members (ParentID != "").
5. Match BGP peer IPs to interface IPs: for each peer, find the interface whose IP is in the same /30 or /31 subnet. Use same `computeSubnetID` approach: compare subnet of peer IP with subnet of each interface IP.
6. `db.GetConfigSnapshots(deviceID)` → scan for protocol keywords (ospf/isis/ldp/rsvp) to set Protocols. Also extract `ip route-static` lines for StaticRoutes.
7. `db.GetVRFInstances(deviceID)` → fill VRFs.
8. Infer PeerGroup roles: internal or name contains "sdn"/"controller" → Management; linked to LAG interface → Uplink; else → Downlink.
9. `graph.BuildFromDB(db)` + `graph.ImpactAnalysis(g, deviceID, graph.NodeTypeDevice)` → IsSPOF + ImpactDevices.

**Subnet matching helper for peer→interface mapping:**

```go
// sameSubnet checks if two IPs with the given mask are in the same subnet.
// mask is a prefix length string like "30".
func sameSubnet(ip1, ip2, mask string) bool {
	// Parse both IPs and mask, compute network address, compare
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/plan/ -v -run TestBuildTopology`
Expected: All 5 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/plan/topology.go internal/plan/topology_test.go
git commit -m "feat(plan): implement BuildTopology multi-dimensional link discovery"
```

---

## Task 3: BGP command generation

**Files:**
- Create: `internal/plan/commands_bgp.go`
- Create: `internal/plan/commands_bgp_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/plan/commands_bgp_test.go
package plan

import (
	"strings"
	"testing"
)

func TestBGPIsolateCommands_H3C(t *testing.T) {
	pg := PeerGroup{
		Name: "LA1", Type: "external", Role: RoleDownlink,
		Peers: []BGPPeerDetail{
			{PeerIP: "10.0.0.1", RemoteAS: 1001, Description: "R08-LA-01"},
			{PeerIP: "10.0.0.2", RemoteAS: 1001, Description: "R09-LA-01"},
		},
	}
	cmds := bgpIsolateStep("lc-01", 65508, pg, "h3c")

	text := strings.Join(cmds.Commands, "\n")
	if !strings.Contains(text, "bgp 65508") {
		t.Errorf("expected 'bgp 65508', got:\n%s", text)
	}
	if !strings.Contains(text, "peer 10.0.0.1 ignore") {
		t.Errorf("expected 'peer 10.0.0.1 ignore', got:\n%s", text)
	}
	if !strings.Contains(text, "# R08-LA-01") {
		t.Errorf("expected description comment, got:\n%s", text)
	}
}

func TestBGPCheckpointCommands(t *testing.T) {
	pg := PeerGroup{
		Name: "LA1", Peers: []BGPPeerDetail{
			{PeerIP: "10.0.0.1"}, {PeerIP: "10.0.0.2"},
		},
	}
	cmds := bgpCheckpoint("lc-01", pg, "h3c")

	text := strings.Join(cmds.Commands, "\n")
	// Should include per-peer verification
	if !strings.Contains(text, "10.0.0.1") {
		t.Error("checkpoint should reference peer IPs")
	}
}

func TestBGPRollbackCommands(t *testing.T) {
	pg := PeerGroup{
		Name: "LA1", Peers: []BGPPeerDetail{
			{PeerIP: "10.0.0.1", Description: "R08"},
		},
	}
	cmds := bgpRollbackStep("lc-01", 65508, pg, "h3c")

	text := strings.Join(cmds.Commands, "\n")
	if !strings.Contains(text, "undo peer 10.0.0.1 ignore") {
		t.Errorf("expected 'undo peer 10.0.0.1 ignore', got:\n%s", text)
	}
}

func TestBGPIsolateCommands_Huawei(t *testing.T) {
	pg := PeerGroup{
		Name: "PEERS", Peers: []BGPPeerDetail{
			{PeerIP: "172.16.0.1", RemoteAS: 200, Description: "PEER-B"},
		},
	}
	cmds := bgpIsolateStep("rtr-a", 100, pg, "huawei")

	text := strings.Join(cmds.Commands, "\n")
	if !strings.Contains(text, "bgp 100") {
		t.Errorf("expected 'bgp 100', got:\n%s", text)
	}
	if !strings.Contains(text, "peer 172.16.0.1 ignore") {
		t.Errorf("expected peer ignore command")
	}
}
```

- [ ] **Step 2: Run tests — should fail**

Run: `go test ./internal/plan/ -v -run TestBGP`
Expected: FAIL (bgpIsolateStep undefined)

- [ ] **Step 3: Implement BGP command functions**

```go
// internal/plan/commands_bgp.go
package plan

import "fmt"

// bgpIsolateStep generates the BGP isolation commands for one peer group.
// Returns a DeviceCommand with system-view → bgp <AS> → peer X ignore per peer → quit → return.
func bgpIsolateStep(deviceID string, localAS int, pg PeerGroup, vendor string) DeviceCommand {
	var cmds []string
	cmds = append(cmds, "system-view")

	switch vendor {
	case "huawei", "h3c":
		cmds = append(cmds, fmt.Sprintf("bgp %d", localAS))
		for _, p := range pg.Peers {
			comment := ""
			if p.Description != "" {
				comment = fmt.Sprintf("  # %s", p.Description)
			}
			cmds = append(cmds, fmt.Sprintf("peer %s ignore%s", p.PeerIP, comment))
		}
		cmds = append(cmds, "quit")
	// cisco, juniper can be added later
	default:
		cmds = append(cmds, fmt.Sprintf("bgp %d", localAS))
		for _, p := range pg.Peers {
			cmds = append(cmds, fmt.Sprintf("peer %s ignore", p.PeerIP))
		}
		cmds = append(cmds, "quit")
	}

	cmds = append(cmds, "return")

	purpose := fmt.Sprintf("BGP 隔离 — %s (%d peers, AS %s)",
		pg.Name, len(pg.Peers), formatASList(pg))
	return DeviceCommand{DeviceID: deviceID, Commands: cmds, Purpose: purpose}
}

// bgpCheckpoint generates verification commands after isolating a peer group.
func bgpCheckpoint(deviceID string, pg PeerGroup, vendor string) DeviceCommand {
	var cmds []string
	// Per-peer check: look for Idle state
	for _, p := range pg.Peers {
		cmds = append(cmds, fmt.Sprintf("display bgp peer %s", p.PeerIP))
	}
	// Summary check
	cmds = append(cmds, "display bgp peer | include Established")
	cmds = append(cmds, "display bgp routing-table statistics")

	return DeviceCommand{
		DeviceID: deviceID, Commands: cmds,
		Purpose: fmt.Sprintf(">>> 检查点: %s 组 peers 应变为 Idle <<<", pg.Name),
	}
}

// bgpRollbackStep generates the rollback commands for one peer group.
func bgpRollbackStep(deviceID string, localAS int, pg PeerGroup, vendor string) DeviceCommand {
	var cmds []string
	cmds = append(cmds, "system-view")

	switch vendor {
	case "huawei", "h3c":
		cmds = append(cmds, fmt.Sprintf("bgp %d", localAS))
		for _, p := range pg.Peers {
			comment := ""
			if p.Description != "" {
				comment = fmt.Sprintf("  # %s", p.Description)
			}
			cmds = append(cmds, fmt.Sprintf("undo peer %s ignore%s", p.PeerIP, comment))
		}
		cmds = append(cmds, "quit")
	default:
		cmds = append(cmds, fmt.Sprintf("bgp %d", localAS))
		for _, p := range pg.Peers {
			cmds = append(cmds, fmt.Sprintf("undo peer %s ignore", p.PeerIP))
		}
		cmds = append(cmds, "quit")
	}

	cmds = append(cmds, "return")
	return DeviceCommand{
		DeviceID: deviceID, Commands: cmds,
		Purpose: fmt.Sprintf("BGP 回退 — 恢复 %s (%d peers)", pg.Name, len(pg.Peers)),
	}
}

// formatASList returns a readable AS summary for a peer group.
func formatASList(pg PeerGroup) string {
	asSet := map[int]bool{}
	for _, p := range pg.Peers { asSet[p.RemoteAS] = true }
	if len(asSet) == 1 {
		for as := range asSet { return fmt.Sprintf("%d", as) }
	}
	return fmt.Sprintf("%d ASes", len(asSet))
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/plan/ -v -run TestBGP`
Expected: All 4 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/plan/commands_bgp.go internal/plan/commands_bgp_test.go
git commit -m "feat(plan): add per-peer-group BGP isolation/checkpoint/rollback commands"
```

---

## Task 4: Interface command generation

**Files:**
- Create: `internal/plan/commands_iface.go`
- Create: `internal/plan/commands_iface_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/plan/commands_iface_test.go
package plan

import (
	"strings"
	"testing"
)

func TestIfaceIsolateCommands_LAGFirst(t *testing.T) {
	lags := []LAGBundle{
		{Name: "Route-Aggregation1", Description: "QCMAN-01"},
	}
	links := []PhysicalLink{
		{Interface: "FortyGigE2/0/1", Description: "R08-LA-01"},
		{Interface: "FortyGigE2/0/2", Description: "R09-LA-01"},
	}

	cmds := ifaceIsolateCommands("lc-01", lags, links, "h3c")

	// Should have 2 DeviceCommands: first LAGs, then physical
	if len(cmds) < 2 {
		t.Fatalf("expected >= 2 DeviceCommands, got %d", len(cmds))
	}

	// First command group should be LAG shutdown
	lagText := strings.Join(cmds[0].Commands, "\n")
	if !strings.Contains(lagText, "Route-Aggregation1") {
		t.Error("first group should shut LAGs")
	}
	if !strings.Contains(lagText, "shutdown") {
		t.Error("expected shutdown command")
	}

	// Second should be physical interfaces
	physText := strings.Join(cmds[1].Commands, "\n")
	if !strings.Contains(physText, "FortyGigE2/0/1") {
		t.Error("second group should shut physical links")
	}
}

func TestIfaceRollbackCommands(t *testing.T) {
	lags := []LAGBundle{{Name: "Route-Aggregation1"}}
	links := []PhysicalLink{{Interface: "FortyGigE2/0/1"}}

	cmds := ifaceRollbackCommands("lc-01", lags, links, "h3c")

	// Rollback: physical first, then LAG (reverse of isolation)
	if len(cmds) < 2 {
		t.Fatalf("expected >= 2 DeviceCommands, got %d", len(cmds))
	}

	text0 := strings.Join(cmds[0].Commands, "\n")
	text1 := strings.Join(cmds[1].Commands, "\n")
	if !strings.Contains(text0, "undo shutdown") { t.Error("expected undo shutdown") }
	if !strings.Contains(text1, "undo shutdown") { t.Error("expected undo shutdown in LAG group") }
}
```

- [ ] **Step 2: Implement interface commands**

```go
// internal/plan/commands_iface.go
package plan

import "fmt"

// ifaceIsolateCommands generates interface shutdown commands.
// LAGs are shut first (uplinks), then physical links (downlinks).
func ifaceIsolateCommands(deviceID string, lags []LAGBundle, links []PhysicalLink, vendor string) []DeviceCommand {
	var result []DeviceCommand

	// LAGs first
	if len(lags) > 0 {
		var cmds []string
		cmds = append(cmds, "system-view")
		for _, lag := range lags {
			cmds = append(cmds, fmt.Sprintf("interface %s", lag.Name))
			cmds = append(cmds, "shutdown")
			cmds = append(cmds, "quit")
		}
		cmds = append(cmds, "return")
		result = append(result, DeviceCommand{
			DeviceID: deviceID, Commands: cmds,
			Purpose: fmt.Sprintf("接口隔离 — 关闭 %d 个 LAG 上联", len(lags)),
		})
	}

	// Physical links
	if len(links) > 0 {
		var cmds []string
		cmds = append(cmds, "system-view")
		for _, link := range links {
			comment := ""
			if link.Description != "" {
				comment = fmt.Sprintf("  # %s", link.Description)
			}
			cmds = append(cmds, fmt.Sprintf("interface %s%s", link.Interface, comment))
			cmds = append(cmds, "shutdown")
			cmds = append(cmds, "quit")
		}
		cmds = append(cmds, "return")
		result = append(result, DeviceCommand{
			DeviceID: deviceID, Commands: cmds,
			Purpose: fmt.Sprintf("接口隔离 — 关闭 %d 个物理下联", len(links)),
		})
	}

	return result
}

// ifaceRollbackCommands generates undo shutdown commands.
// Reverse order: physical links first, then LAGs.
func ifaceRollbackCommands(deviceID string, lags []LAGBundle, links []PhysicalLink, vendor string) []DeviceCommand {
	var result []DeviceCommand

	// Physical links first (reverse of isolation)
	if len(links) > 0 {
		var cmds []string
		cmds = append(cmds, "system-view")
		for _, link := range links {
			cmds = append(cmds, fmt.Sprintf("interface %s", link.Interface))
			cmds = append(cmds, "undo shutdown")
			cmds = append(cmds, "quit")
		}
		cmds = append(cmds, "return")
		result = append(result, DeviceCommand{
			DeviceID: deviceID, Commands: cmds,
			Purpose: fmt.Sprintf("接口恢复 — 开启 %d 个物理下联", len(links)),
		})
	}

	// LAGs second
	if len(lags) > 0 {
		var cmds []string
		cmds = append(cmds, "system-view")
		for _, lag := range lags {
			cmds = append(cmds, fmt.Sprintf("interface %s", lag.Name))
			cmds = append(cmds, "undo shutdown")
			cmds = append(cmds, "quit")
		}
		cmds = append(cmds, "return")
		result = append(result, DeviceCommand{
			DeviceID: deviceID, Commands: cmds,
			Purpose: fmt.Sprintf("接口恢复 — 开启 %d 个 LAG 上联", len(lags)),
		})
	}

	return result
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/plan/ -v -run TestIface`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add internal/plan/commands_iface.go internal/plan/commands_iface_test.go
git commit -m "feat(plan): add interface isolation/rollback commands with LAG ordering"
```

---

## Task 5: GenerateIsolationPlanV2

**Files:**
- Create: `internal/plan/isolate_v2.go`
- Create: `internal/plan/isolate_v2_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/plan/isolate_v2_test.go
package plan

import (
	"strings"
	"testing"
)

func testTopology() DeviceTopology {
	return DeviceTopology{
		DeviceID: "lc-01", Hostname: "LC-01", Vendor: "h3c", LocalAS: 65508,
		Protocols: []string{"bgp"},
		PeerGroups: []PeerGroup{
			{Name: "LA1", Type: "external", Role: RoleDownlink, Peers: []BGPPeerDetail{
				{PeerIP: "10.0.0.1", RemoteAS: 1001, Description: "R08-LA"},
			}},
			{Name: "QCDR", Type: "external", Role: RoleUplink, Peers: []BGPPeerDetail{
				{PeerIP: "10.1.0.1", RemoteAS: 45090, Description: "QCDR-01"},
			}},
			{Name: "SDN", Type: "internal", Role: RoleManagement, Peers: []BGPPeerDetail{
				{PeerIP: "10.2.0.1", RemoteAS: 65508, Description: "SDN-Ctrl"},
			}},
		},
		LAGs: []LAGBundle{
			{Name: "Route-Aggregation1", IP: "10.3.0.1", Description: "QCMAN-01"},
		},
		PhysicalLinks: []PhysicalLink{
			{Interface: "FGE2/0/1", IP: "10.0.0.2", Description: "R08-LA"},
		},
		IsSPOF: true, ImpactDevices: []string{"DEV-X"},
	}
}

func TestGenerateIsolationPlanV2_PhaseCount(t *testing.T) {
	p := GenerateIsolationPlanV2(testTopology())
	if len(p.Phases) != 6 {
		t.Fatalf("expected 6 phases, got %d", len(p.Phases))
	}
}

func TestGenerateIsolationPlanV2_Phase2_OrderDownlinkFirst(t *testing.T) {
	p := GenerateIsolationPlanV2(testTopology())
	phase2 := p.Phases[2] // 协议隔离

	// Steps should be ordered: downlink (LA1) → uplink (QCDR) → management (SDN)
	if len(phase2.Steps) < 6 { // 3 isolate + 3 checkpoint
		t.Fatalf("expected >= 6 steps in phase 2, got %d", len(phase2.Steps))
	}

	// First step should be LA1 (downlink)
	if !strings.Contains(phase2.Steps[0].Purpose, "LA1") {
		t.Errorf("first isolation step should be LA1 (downlink), got: %s", phase2.Steps[0].Purpose)
	}
}

func TestGenerateIsolationPlanV2_Phase2_ContainsPeerIPs(t *testing.T) {
	p := GenerateIsolationPlanV2(testTopology())
	phase2 := p.Phases[2]

	allText := ""
	for _, s := range phase2.Steps {
		allText += strings.Join(s.Commands, "\n") + "\n"
	}

	if !strings.Contains(allText, "peer 10.0.0.1 ignore") {
		t.Error("expected 'peer 10.0.0.1 ignore' in phase 2")
	}
	if !strings.Contains(allText, "bgp 65508") {
		t.Error("expected 'bgp 65508' in phase 2")
	}
}

func TestGenerateIsolationPlanV2_Phase0_ProtocolSpecific(t *testing.T) {
	p := GenerateIsolationPlanV2(testTopology())
	phase0 := p.Phases[0]

	allText := ""
	for _, s := range phase0.Steps {
		allText += strings.Join(s.Commands, "\n") + "\n"
	}

	// Has BGP → should have display bgp peer
	if !strings.Contains(allText, "display bgp peer") {
		t.Error("phase 0 should include 'display bgp peer' for BGP device")
	}
	// No OSPF → should NOT have display ospf peer
	if strings.Contains(allText, "display ospf peer") {
		t.Error("phase 0 should NOT include 'display ospf peer' — device has no OSPF")
	}
}

func TestGenerateIsolationPlanV2_Phase5_ReverseOrder(t *testing.T) {
	p := GenerateIsolationPlanV2(testTopology())
	phase5 := p.Phases[5] // 回退

	// Rollback BGP should be in reverse: management → uplink → downlink
	firstBGPStep := ""
	for _, s := range phase5.Steps {
		if strings.Contains(s.Purpose, "BGP 回退") {
			firstBGPStep = s.Purpose
			break
		}
	}
	if !strings.Contains(firstBGPStep, "SDN") {
		t.Errorf("first BGP rollback should be management (SDN), got: %s", firstBGPStep)
	}
}
```

- [ ] **Step 2: Implement GenerateIsolationPlanV2**

```go
// internal/plan/isolate_v2.go
package plan

import (
	"fmt"
	"sort"
	"time"
)

// GenerateIsolationPlanV2 produces a detailed isolation plan from a DeviceTopology.
func GenerateIsolationPlanV2(topo DeviceTopology) Plan {
	p := Plan{
		TargetDevice:   topo.DeviceID,
		TargetHostname: topo.Hostname,
		TargetVendor:   topo.Vendor,
		IsSPOF:         topo.IsSPOF,
		ImpactDevices:  topo.ImpactDevices,
		GeneratedAt:    time.Now(),
	}

	// Sort peer groups by isolation order: downlink → uplink → management
	sorted := sortPeerGroups(topo.PeerGroups)

	p.Phases = []Phase{
		buildCollectionPhase(topo),
		buildPreCheckPhase(topo, sorted),
		buildProtocolIsolationPhase(topo, sorted),
		buildInterfaceIsolationPhase(topo),
		buildPostCheckPhase(topo),
		buildRollbackPhase(topo, sorted),
	}
	return p
}

// sortPeerGroups orders: downlink first, then uplink, then management.
func sortPeerGroups(groups []PeerGroup) []PeerGroup {
	sorted := make([]PeerGroup, len(groups))
	copy(sorted, groups)
	sort.SliceStable(sorted, func(i, j int) bool {
		return roleOrder(sorted[i].Role) < roleOrder(sorted[j].Role)
	})
	return sorted
}

func roleOrder(r PeerGroupRole) int {
	switch r {
	case RoleDownlink:   return 0
	case RoleUplink:     return 1
	case RoleManagement: return 2
	default:             return 1
	}
}

func buildCollectionPhase(topo DeviceTopology) Phase {
	var cmds []string
	cmds = append(cmds, "display interface brief")
	cmds = append(cmds, "display ip routing-table statistics")

	for _, proto := range topo.Protocols {
		switch proto {
		case "bgp":
			cmds = append(cmds, "display bgp peer")
			cmds = append(cmds, "display bgp routing-table statistics")
		case "ospf":
			if topo.Vendor == "h3c" {
				cmds = append(cmds, "display ospf peer")
			} else {
				cmds = append(cmds, "display ospf peer brief")
			}
		case "isis":
			cmds = append(cmds, "display isis peer")
		case "ldp":
			cmds = append(cmds, "display mpls ldp session")
		}
	}

	if len(topo.LAGs) > 0 {
		cmds = append(cmds, "display link-aggregation verbose")
	}
	cmds = append(cmds, "display current-configuration")
	cmds = append(cmds, "display version")

	phase := Phase{
		Number: 0, Name: "采集",
		Description: "采集目标设备当前运行状态",
		Steps: []DeviceCommand{{DeviceID: topo.DeviceID, Commands: cmds, Purpose: "基线状态采集"}},
	}
	if topo.IsSPOF {
		phase.Notes = append(phase.Notes,
			fmt.Sprintf("⚠️ SPOF — 隔离后 %d 台设备受影响", len(topo.ImpactDevices)))
	}
	return phase
}

func buildPreCheckPhase(topo DeviceTopology, sorted []PeerGroup) Phase {
	var notes []string
	for _, pg := range sorted {
		notes = append(notes,
			fmt.Sprintf("✓ BGP peer group %s: %d peers 应全部 Established", pg.Name, len(pg.Peers)))
	}
	for _, lag := range topo.LAGs {
		notes = append(notes, fmt.Sprintf("✓ %s: 状态应为 Up", lag.Name))
	}

	return Phase{
		Number: 1, Name: "预检查",
		Description: "确认所有邻居状态正常",
		Steps:       []DeviceCommand{{DeviceID: topo.DeviceID,
			Commands: []string{"display bgp peer", "display interface brief"},
			Purpose: "验证基线状态"}},
		Notes: notes,
	}
}

func buildProtocolIsolationPhase(topo DeviceTopology, sorted []PeerGroup) Phase {
	var steps []DeviceCommand

	for _, pg := range sorted {
		// Isolation step
		steps = append(steps, bgpIsolateStep(topo.DeviceID, topo.LocalAS, pg, topo.Vendor))
		// Checkpoint
		steps = append(steps, bgpCheckpoint(topo.DeviceID, pg, topo.Vendor))
	}

	phase := Phase{
		Number: 2, Name: "协议级隔离",
		Description: fmt.Sprintf("按 peer group 分批隔离 BGP（先下联 → 上联 → 管理，共 %d 组）", len(sorted)),
		Steps:       steps,
	}
	if len(topo.Protocols) == 0 {
		phase.Notes = append(phase.Notes, "⚠️ 未检测到协议信息，阶段3将执行硬切")
	}
	return phase
}

func buildInterfaceIsolationPhase(topo DeviceTopology) Phase {
	steps := ifaceIsolateCommands(topo.DeviceID, topo.LAGs, topo.PhysicalLinks, topo.Vendor)
	steps = append(steps, DeviceCommand{
		DeviceID: topo.DeviceID,
		Commands: []string{"display interface brief"},
		Purpose:  "验证所有接口已关闭",
	})
	return Phase{
		Number: 3, Name: "接口级隔离",
		Description: "关闭所有互联接口",
		Steps:       steps,
	}
}

func buildPostCheckPhase(topo DeviceTopology) Phase {
	return Phase{
		Number: 4, Name: "变更后检查",
		Description: "确认设备已完全脱离网络",
		Steps: []DeviceCommand{{
			DeviceID: topo.DeviceID,
			Commands: []string{
				"display bgp peer",
				"display interface brief",
				"display ip routing-table statistics",
			},
			Purpose: "确认所有邻居已断开、路由已清空",
		}},
		Notes: []string{"确认对端设备邻居表中目标设备已消失", "确认路由已收敛到备用路径"},
	}
}

func buildRollbackPhase(topo DeviceTopology, sorted []PeerGroup) Phase {
	var steps []DeviceCommand

	// BGP rollback: reverse order (management → uplink → downlink)
	reversed := make([]PeerGroup, len(sorted))
	for i, pg := range sorted {
		reversed[len(sorted)-1-i] = pg
	}
	for _, pg := range reversed {
		steps = append(steps, bgpRollbackStep(topo.DeviceID, topo.LocalAS, pg, topo.Vendor))
	}

	// Interface rollback
	steps = append(steps, ifaceRollbackCommands(topo.DeviceID, topo.LAGs, topo.PhysicalLinks, topo.Vendor)...)

	return Phase{
		Number: 5, Name: "回退方案",
		Description: "如需回退，先恢复协议（管理→上联→下联），再恢复接口",
		Steps:       steps,
		Notes:       []string{"回退后等待 BGP 邻居重建，确认路由收敛"},
	}
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/plan/ -v -run TestGenerateIsolationPlanV2`
Expected: All 5 PASS

- [ ] **Step 4: Commit**

```bash
git add internal/plan/isolate_v2.go internal/plan/isolate_v2_test.go
git commit -m "feat(plan): add GenerateIsolationPlanV2 with topology-aware phases"
```

---

## Task 6: Wire CLI to v2

**Files:**
- Modify: `internal/cli/plan.go`

- [ ] **Step 1: Modify newPlanIsolateCmd to use BuildTopology + GenerateIsolationPlanV2**

Replace the current RunE logic. Key changes:
1. Replace `plan.DiscoverLinks(db, deviceID)` with `plan.BuildTopology(db, deviceID)`
2. Replace `plan.BuildIsolationPlan(plan.PlanInput{...})` with `plan.GenerateIsolationPlanV2(topo)`
3. Remove the separate `graph.BuildFromDB` + `graph.ImpactAnalysis` calls (BuildTopology does this internally)
4. Update the `--check` path to work with `DeviceTopology` instead of `[]Link`
5. Remove the "suspicious link count" warning (BuildTopology provides complete data)

Read `internal/cli/plan.go` first to see the exact current code.

- [ ] **Step 2: Build and smoke test**

Run: `go build ./cmd/nethelper && ./nethelper plan isolate cd-gx-0201-g17-h12516af-lc-01 2>&1 | head -50`
Expected: New output showing peer groups, per-peer commands, checkpoints.

- [ ] **Step 3: Commit**

```bash
git add internal/cli/plan.go
git commit -m "feat(cli): wire plan isolate to topology-aware v2 generator"
```

---

## Task 7: Integration test with real LC data

- [ ] **Step 1: Run full plan**

Run: `./nethelper plan isolate cd-gx-0201-g17-h12516af-lc-01`

Verify:
- Shows "AS 65508"
- Shows 6 peer groups (LA1, LA2~448, XGWL, QCDR, SDN-Controller-Read, SDN-Controller-Write)
- Phase 2 has per-peer `peer X ignore` commands for all 234 peers
- Order: LA1 → LA2~448 → XGWL → QCDR → SDN-Controllers
- Checkpoints after each group
- Phase 3 shuts LAGs first then physical links
- Phase 5 rollback in reverse order
- No "display ospf peer" (LC has no OSPF)

- [ ] **Step 2: Run markdown output**

Run: `./nethelper plan isolate cd-gx-0201-g17-h12516af-lc-01 --format markdown --output docs/lc-isolation-plan-v2.md`

- [ ] **Step 3: Run full test suite**

Run: `go test ./... && go vet ./...`
Expected: All pass

- [ ] **Step 4: Commit**

```bash
git add docs/lc-isolation-plan-v2.md
git commit -m "docs: generate LC isolation plan v2 with per-peer BGP commands"
```
