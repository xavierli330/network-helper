# ISIS/OSPF/LDP Isolation Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add ISIS overload, OSPF stub-router, and LDP disable commands to plan isolate, so non-BGP devices get proper protocol isolation.

**Architecture:** Extend `DeviceTopology` with IGP info extracted from config, add `commands_igp.go` for ISIS/OSPF/LDP command generation, insert IGP steps into Phase 2 before BGP.

**Tech Stack:** Go, existing plan package patterns.

**Spec:** `docs/superpowers/specs/2026-03-22-igp-isolation-design.md`

---

## File Map

| File | Action | Change |
|------|--------|--------|
| `internal/plan/topology.go` | Modify | Add `IGPInfo`, `HasLDP`, `LDPInterfaces` fields + config extraction |
| `internal/plan/commands_igp.go` | Create | ISIS/OSPF/LDP isolate/checkpoint/rollback functions |
| `internal/plan/commands_igp_test.go` | Create | Tests |
| `internal/plan/isolate_v2.go` | Modify | Insert IGP steps into Phase 2 before BGP |

---

## Task 1: Extend DeviceTopology with IGP info

**Files:**
- Modify: `internal/plan/topology.go`

- [ ] **Step 1: Add IGP types and fields to DeviceTopology**

Add to topology.go after the existing types:

```go
// IGPInfo represents an ISIS or OSPF routing instance.
type IGPInfo struct {
    Protocol   string   // "isis" or "ospf"
    ProcessID  string   // e.g. "1", "100"
    Interfaces []string // interfaces with this protocol enabled
}
```

Add fields to `DeviceTopology`:

```go
type DeviceTopology struct {
    // ... existing fields ...
    IGPs          []IGPInfo
    HasLDP        bool
    LDPInterfaces []string // interfaces with mpls ldp enabled
}
```

- [ ] **Step 2: Extract IGP info in BuildTopology**

In the config parsing section of BuildTopology (where protocols are detected), add extraction logic:

```go
// Extract ISIS instances
extractIGPInstances(configText, deviceID, ifaces, &topo)
// Extract LDP interfaces
extractLDPInterfaces(configText, deviceID, ifaces, &topo)
```

Helper functions:

```go
func extractIGPInstances(configText, deviceID string, ifaces []model.Interface, topo *DeviceTopology) {
    // Parse ISIS: look for "isis <N>" blocks and "isis enable <N>" on interfaces
    // Parse OSPF: look for "ospf <N>" blocks and "ospf enable <N> area <A>" on interfaces

    // Strategy: scan all interface configs for "isis enable" / "ospf enable"
    // Group by process ID
}

func extractLDPInterfaces(configText, deviceID string, ifaces []model.Interface, topo *DeviceTopology) {
    // Scan interface configs for "mpls ldp" lines
    // Set topo.HasLDP = true if any found
}
```

For config extraction, use the same approach as the existing `enrichProtocols` — scan the config text line by line. Interface-protocol mapping can be done by:
1. Split config by `#` separator (H3C/Huawei style)
2. For each interface block, check if it contains `isis enable` or `ospf enable` or `mpls ldp`

Read `internal/parser/config_extract.go` for the existing pattern of parsing config blocks.

- [ ] **Step 3: Write test**

Add to `internal/plan/topology_test.go`:

```go
func TestBuildTopology_IGPDetection(t *testing.T) {
    db := setupTestDB(t)
    db.UpsertDevice(model.Device{ID: "qcdr-01", Hostname: "QCDR-01", Vendor: "huawei"})
    db.InsertConfigSnapshot(model.ConfigSnapshot{
        DeviceID: "qcdr-01",
        ConfigText: `#
isis 1
 is-level level-2
 network-entity 04.xxxx.xxxx.00
#
mpls ldp
#
interface Eth-Trunk1
 isis enable 1
 mpls ldp
#
interface Eth-Trunk2
 isis enable 1
 mpls ldp
#
interface LoopBack0
 isis enable 1
#`,
    })

    topo, err := plan.BuildTopology(db, "qcdr-01")
    if err != nil { t.Fatal(err) }

    // Should detect ISIS
    if len(topo.IGPs) != 1 { t.Fatalf("expected 1 IGP, got %d", len(topo.IGPs)) }
    if topo.IGPs[0].Protocol != "isis" { t.Errorf("expected isis, got %s", topo.IGPs[0].Protocol) }
    if topo.IGPs[0].ProcessID != "1" { t.Errorf("expected process 1, got %s", topo.IGPs[0].ProcessID) }
    if len(topo.IGPs[0].Interfaces) < 2 { t.Errorf("expected >= 2 ISIS interfaces, got %d", len(topo.IGPs[0].Interfaces)) }

    // Should detect LDP
    if !topo.HasLDP { t.Error("expected HasLDP=true") }
    if len(topo.LDPInterfaces) < 2 { t.Errorf("expected >= 2 LDP interfaces, got %d", len(topo.LDPInterfaces)) }
}
```

- [ ] **Step 4: Run tests, commit**

Run: `go test ./internal/plan/ -v -run TestBuildTopology`
Commit: `feat(plan): extract ISIS/OSPF/LDP info from config into DeviceTopology`

---

## Task 2: IGP isolation command generation

**Files:**
- Create: `internal/plan/commands_igp.go`
- Create: `internal/plan/commands_igp_test.go`

- [ ] **Step 1: Implement ISIS/OSPF/LDP command functions**

```go
// internal/plan/commands_igp.go
package plan

import "fmt"

// isisIsolateStep generates ISIS overload command.
func isisIsolateStep(deviceID string, igp IGPInfo, vendor string) DeviceCommand {
    var cmds []string
    cmds = append(cmds, "system-view")
    switch vendor {
    case "huawei", "h3c":
        cmds = append(cmds, fmt.Sprintf("isis %s", igp.ProcessID))
        cmds = append(cmds, "set-overload")
        cmds = append(cmds, "quit")
    case "cisco":
        cmds = append(cmds, fmt.Sprintf("router isis %s", igp.ProcessID))
        cmds = append(cmds, "set-overload-bit")
        cmds = append(cmds, "exit")
    }
    cmds = append(cmds, "return")
    return DeviceCommand{
        DeviceID: deviceID, Commands: cmds,
        Purpose: fmt.Sprintf("ISIS 隔离 — set-overload (进程 %s)", igp.ProcessID),
    }
}

func isisCheckpoint(deviceID string, igp IGPInfo, vendor string) DeviceCommand {
    cmds := []string{"display isis peer"}
    if vendor == "cisco" {
        cmds = []string{"show isis neighbors"}
    }
    return DeviceCommand{
        DeviceID: deviceID, Commands: cmds,
        Purpose: fmt.Sprintf(">>> 检查点: ISIS 进程 %s overload 生效 <<<", igp.ProcessID),
    }
}

func isisRollbackStep(deviceID string, igp IGPInfo, vendor string) DeviceCommand {
    var cmds []string
    cmds = append(cmds, "system-view")
    switch vendor {
    case "huawei", "h3c":
        cmds = append(cmds, fmt.Sprintf("isis %s", igp.ProcessID))
        cmds = append(cmds, "undo set-overload")
        cmds = append(cmds, "quit")
    case "cisco":
        cmds = append(cmds, fmt.Sprintf("router isis %s", igp.ProcessID))
        cmds = append(cmds, "no set-overload-bit")
        cmds = append(cmds, "exit")
    }
    cmds = append(cmds, "return")
    return DeviceCommand{
        DeviceID: deviceID, Commands: cmds,
        Purpose: fmt.Sprintf("ISIS 回退 — undo set-overload (进程 %s)", igp.ProcessID),
    }
}

// ospfIsolateStep generates OSPF stub-router command.
func ospfIsolateStep(deviceID string, igp IGPInfo, vendor string) DeviceCommand {
    var cmds []string
    cmds = append(cmds, "system-view")
    switch vendor {
    case "huawei", "h3c":
        cmds = append(cmds, fmt.Sprintf("ospf %s", igp.ProcessID))
        cmds = append(cmds, "stub-router")
        cmds = append(cmds, "quit")
    case "cisco":
        cmds = append(cmds, fmt.Sprintf("router ospf %s", igp.ProcessID))
        cmds = append(cmds, "max-metric router-lsa")
        cmds = append(cmds, "exit")
    }
    cmds = append(cmds, "return")
    return DeviceCommand{
        DeviceID: deviceID, Commands: cmds,
        Purpose: fmt.Sprintf("OSPF 隔离 — stub-router (进程 %s)", igp.ProcessID),
    }
}

// Similar: ospfCheckpoint, ospfRollbackStep, ldpIsolateSteps, ldpRollbackSteps
```

LDP isolation: per-interface `undo mpls ldp`:
```go
func ldpIsolateSteps(deviceID string, interfaces []string, vendor string) DeviceCommand {
    var cmds []string
    cmds = append(cmds, "system-view")
    for _, iface := range interfaces {
        cmds = append(cmds, fmt.Sprintf("interface %s", iface))
        cmds = append(cmds, "undo mpls ldp")
        cmds = append(cmds, "quit")
    }
    cmds = append(cmds, "return")
    return DeviceCommand{
        DeviceID: deviceID, Commands: cmds,
        Purpose: fmt.Sprintf("LDP 隔离 — 禁用 %d 个接口的 MPLS LDP", len(interfaces)),
    }
}
```

- [ ] **Step 2: Write tests**

Test ISIS overload for Huawei, OSPF stub-router, LDP per-interface, rollback commands.

- [ ] **Step 3: Run tests, commit**

Commit: `feat(plan): add ISIS/OSPF/LDP isolation command generation`

---

## Task 3: Wire IGP into Phase 2

**Files:**
- Modify: `internal/plan/isolate_v2.go`

- [ ] **Step 1: Insert IGP steps before BGP in buildProtocolIsolationPhase**

In `buildProtocolIsolationPhase`, before the BGP peer group loop, add:

```go
// IGP isolation first (ISIS overload / OSPF stub-router)
for _, igp := range topo.IGPs {
    switch igp.Protocol {
    case "isis":
        steps = append(steps, isisIsolateStep(topo.DeviceID, igp, topo.Vendor))
        steps = append(steps, isisCheckpoint(topo.DeviceID, igp, topo.Vendor))
    case "ospf":
        steps = append(steps, ospfIsolateStep(topo.DeviceID, igp, topo.Vendor))
        steps = append(steps, ospfCheckpoint(topo.DeviceID, igp, topo.Vendor))
    }
}

// Wait for IGP convergence if any IGP was configured
if len(topo.IGPs) > 0 {
    steps = append(steps, DeviceCommand{
        DeviceID: topo.DeviceID,
        Commands: []string{"# 等待 IGP 收敛（建议至少 60 秒）"},
        Purpose:  "等待 IGP 路由收敛",
    })
}

// LDP disable (after IGP convergence)
if topo.HasLDP && len(topo.LDPInterfaces) > 0 {
    steps = append(steps, ldpIsolateSteps(topo.DeviceID, topo.LDPInterfaces, topo.Vendor))
    steps = append(steps, ldpCheckpoint(topo.DeviceID, topo.Vendor))
}

// Then BGP (existing logic)
```

In `buildRollbackPhase`, reverse: first BGP (already done), then LDP restore, then IGP undo-overload.

Also update `buildCollectionPhase` to include ISIS/OSPF/LDP display commands based on `topo.IGPs` and `topo.HasLDP`.

- [ ] **Step 2: Write test**

```go
func TestGenerateIsolationPlanV2_WithISIS(t *testing.T) {
    topo := DeviceTopology{
        DeviceID: "qcdr-01", Hostname: "QCDR-01", Vendor: "huawei",
        Protocols: []string{"isis", "ldp", "bgp"}, LocalAS: 45090,
        IGPs: []IGPInfo{{Protocol: "isis", ProcessID: "1", Interfaces: []string{"Eth-Trunk1"}}},
        HasLDP: true, LDPInterfaces: []string{"Eth-Trunk1"},
        PeerGroups: []PeerGroup{{Name: "PEERS", Type: "external", Role: RoleDownlink,
            Peers: []BGPPeerDetail{{PeerIP: "10.0.0.1", RemoteAS: 65508}}}},
    }
    p := GenerateIsolationPlanV2(topo)

    // Phase 2 should have ISIS overload BEFORE BGP
    phase2Text := ""
    for _, s := range p.Phases[2].Steps {
        phase2Text += s.Purpose + "\n"
    }
    isisIdx := strings.Index(phase2Text, "ISIS")
    bgpIdx := strings.Index(phase2Text, "BGP")
    if isisIdx < 0 { t.Error("expected ISIS step in phase 2") }
    if bgpIdx < 0 { t.Error("expected BGP step in phase 2") }
    if isisIdx > bgpIdx { t.Error("ISIS should come before BGP") }
}
```

- [ ] **Step 3: Run tests, commit**

Commit: `feat(plan): wire ISIS/OSPF/LDP into isolation Phase 2`

---

## Task 4: Integration test with QCDR

- [ ] **Step 1: Run plan isolate on QCDR**

```bash
./nethelper plan isolate cd-gx-0201-h10-hw12816-qcdr-01 2>&1 | head -60
```

Verify: Phase 2 shows ISIS set-overload, LDP disable per-interface, then BGP peer ignore.

- [ ] **Step 2: Full test suite**

```bash
go test ./... && go vet ./...
```

- [ ] **Step 3: Commit + update README**

Update README.md: change OSPF/ISIS line from 🔲 to ✅
