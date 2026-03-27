# Multi-Session Concurrency + Data Freshness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the data pipeline so multiple log files for the same device produce correct, deduplicated data with proper temporal ordering.

**Architecture:** Five targeted fixes to the store layer: (1) UPSERT dedup for neighbors/BGP/tunnels/SR with new unique indexes, (2) capture-time-aware conditional updates for interfaces/devices, (3) convenience methods for latest-state queries, (4) config hash dedup, (5) per-file mutex. Each fix is independent and can be tested in isolation.

**Tech Stack:** Go, SQLite (UPSERT / ON CONFLICT), existing `store.DB` patterns.

**Spec:** `docs/superpowers/specs/2026-03-22-multi-session-data-freshness-design.md`

---

## File Map

| File | Action | Change |
|------|--------|--------|
| `internal/store/migrations.go` | Modify | Add 4 UNIQUE indexes + content_hash column (append to migrations array) |
| `internal/store/neighbor_store.go` | Modify | INSERT → UPSERT + add `GetLatestNeighbors` |
| `internal/store/bgp_store.go` | Modify | INSERT → UPSERT + add `GetLatestBGPPeers` |
| `internal/store/interface_store.go` | Modify | Conditional UPSERT (only update if newer capture time) |
| `internal/store/device_store.go` | Modify | Conditional last_seen update |
| `internal/store/config_snapshot_store.go` | Modify | Hash dedup before insert |
| `internal/parser/pipeline.go` | Modify | Pass capturedAt to UpsertInterface/UpsertDevice |
| `internal/cli/watch.go` | Modify | Per-file mutex |
| `internal/plan/topology.go` | Modify | Use GetLatestBGPPeers instead of manual snapshot search |
| `internal/store/store_test.go` | Create | Tests for all new behavior |

---

## Task 1: Schema migrations (UNIQUE indexes + content_hash)

**Files:**
- Modify: `internal/store/migrations.go` (append to migrations array, currently 46 entries ending at index 45)

- [ ] **Step 1: Add new migrations**

Append these to the `migrations` array (they become indices 46-50):

```go
// index 46: UNIQUE constraint for protocol_neighbors dedup
`CREATE UNIQUE INDEX IF NOT EXISTS idx_neighbors_dedup ON protocol_neighbors(device_id, protocol, remote_id, remote_address, snapshot_id)`,

// index 47: UNIQUE constraint for bgp_peers dedup
`CREATE UNIQUE INDEX IF NOT EXISTS idx_bgp_peers_dedup ON bgp_peers(device_id, peer_ip, address_family, vrf, snapshot_id)`,

// index 48: UNIQUE constraint for mpls_te_tunnels dedup
`CREATE UNIQUE INDEX IF NOT EXISTS idx_tunnels_dedup ON mpls_te_tunnels(device_id, tunnel_name, snapshot_id)`,

// index 49: UNIQUE constraint for sr_mappings dedup
`CREATE UNIQUE INDEX IF NOT EXISTS idx_sr_dedup ON sr_mappings(device_id, prefix, sid_index, snapshot_id)`,

// index 50: content_hash column for config dedup
`ALTER TABLE config_snapshots ADD COLUMN content_hash TEXT NOT NULL DEFAULT ''`,
```

Note: SQLite `CREATE UNIQUE INDEX IF NOT EXISTS` is safe to run on existing data IF there are no duplicates. If duplicates exist, the migration will fail. Handle this by catching the error in `migrate()` — the existing code already tolerates `ALTER TABLE` "duplicate column" errors; extend it to tolerate `UNIQUE constraint failed` during index creation by deleting duplicates first.

Actually, simpler approach: just use `IF NOT EXISTS` and let it succeed. If there are existing duplicates, the index creation will fail — but that's OK because the `migrate()` function already has error tolerance for certain patterns. Add a pre-cleanup step:

```go
// index 46: cleanup duplicates before creating unique index
`DELETE FROM protocol_neighbors WHERE id NOT IN (SELECT MIN(id) FROM protocol_neighbors GROUP BY device_id, protocol, remote_id, remote_address, snapshot_id)`,
```

So the full migration set is:

```go
// Dedup cleanup + unique indexes for multi-session support
`DELETE FROM protocol_neighbors WHERE id NOT IN (SELECT MIN(id) FROM protocol_neighbors GROUP BY device_id, protocol, remote_id, remote_address, snapshot_id)`,
`CREATE UNIQUE INDEX IF NOT EXISTS idx_neighbors_dedup ON protocol_neighbors(device_id, protocol, remote_id, remote_address, snapshot_id)`,

`DELETE FROM bgp_peers WHERE id NOT IN (SELECT MIN(id) FROM bgp_peers GROUP BY device_id, peer_ip, address_family, vrf, snapshot_id)`,
`CREATE UNIQUE INDEX IF NOT EXISTS idx_bgp_peers_dedup ON bgp_peers(device_id, peer_ip, address_family, vrf, snapshot_id)`,

`DELETE FROM mpls_te_tunnels WHERE id NOT IN (SELECT MIN(id) FROM mpls_te_tunnels GROUP BY device_id, tunnel_name, snapshot_id)`,
`CREATE UNIQUE INDEX IF NOT EXISTS idx_tunnels_dedup ON mpls_te_tunnels(device_id, tunnel_name, snapshot_id)`,

`DELETE FROM sr_mappings WHERE id NOT IN (SELECT MIN(id) FROM sr_mappings GROUP BY device_id, prefix, sid_index, snapshot_id)`,
`CREATE UNIQUE INDEX IF NOT EXISTS idx_sr_dedup ON sr_mappings(device_id, prefix, sid_index, snapshot_id)`,

`ALTER TABLE config_snapshots ADD COLUMN content_hash TEXT NOT NULL DEFAULT ''`,
```

- [ ] **Step 2: Build and verify migrations run**

Run: `go build ./cmd/nethelper && ./nethelper version`
Expected: Success (migrations auto-run on startup via `store.Open`)

- [ ] **Step 3: Verify indexes exist**

Run: `sqlite3 ~/.nethelper/nethelper.db ".indexes protocol_neighbors"`
Expected: Shows `idx_neighbors_dedup` among the indexes

- [ ] **Step 4: Commit**

```bash
git add internal/store/migrations.go
git commit -m "feat(store): add unique indexes for dedup + config content_hash column"
```

---

## Task 2: Neighbor UPSERT + GetLatestNeighbors

**Files:**
- Modify: `internal/store/neighbor_store.go`
- Modify: `internal/store/store_test.go` (create if not exists)

- [ ] **Step 1: Write failing test**

```go
// internal/store/store_test.go (or add to existing test file)
package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xavierli/nethelper/internal/model"
)

func setupDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { db.Close() })
	return db
}

func TestInsertNeighbors_Dedup(t *testing.T) {
	db := setupDB(t)
	db.UpsertDevice(model.Device{ID: "dev-a", Hostname: "DEV-A", Vendor: "huawei"})
	snapID, _ := db.CreateSnapshot(model.Snapshot{DeviceID: "dev-a", Commands: "[]"})

	neighbor := model.NeighborInfo{
		DeviceID: "dev-a", Protocol: "ospf", RemoteID: "1.1.1.1",
		RemoteAddress: "10.0.0.1", State: "Full", SnapshotID: snapID,
	}

	// Insert twice — should not duplicate
	db.InsertNeighbors([]model.NeighborInfo{neighbor})
	db.InsertNeighbors([]model.NeighborInfo{neighbor})

	results, _ := db.GetNeighbors("dev-a", snapID)
	if len(results) != 1 {
		t.Errorf("expected 1 neighbor after dedup, got %d", len(results))
	}
}

func TestInsertNeighbors_UpdateOnConflict(t *testing.T) {
	db := setupDB(t)
	db.UpsertDevice(model.Device{ID: "dev-a", Hostname: "DEV-A", Vendor: "huawei"})
	snapID, _ := db.CreateSnapshot(model.Snapshot{DeviceID: "dev-a", Commands: "[]"})

	n1 := model.NeighborInfo{
		DeviceID: "dev-a", Protocol: "ospf", RemoteID: "1.1.1.1",
		RemoteAddress: "10.0.0.1", State: "Init", SnapshotID: snapID,
	}
	n2 := n1
	n2.State = "Full" // state changed

	db.InsertNeighbors([]model.NeighborInfo{n1})
	db.InsertNeighbors([]model.NeighborInfo{n2})

	results, _ := db.GetNeighbors("dev-a", snapID)
	if len(results) != 1 { t.Fatalf("expected 1, got %d", len(results)) }
	if results[0].State != "Full" {
		t.Errorf("state should be updated to Full, got %s", results[0].State)
	}
}

func TestGetLatestNeighbors(t *testing.T) {
	db := setupDB(t)
	db.UpsertDevice(model.Device{ID: "dev-a", Hostname: "DEV-A", Vendor: "huawei"})
	snap1, _ := db.CreateSnapshot(model.Snapshot{DeviceID: "dev-a", Commands: "[]"})
	snap2, _ := db.CreateSnapshot(model.Snapshot{DeviceID: "dev-a", Commands: "[]"})

	db.InsertNeighbors([]model.NeighborInfo{
		{DeviceID: "dev-a", Protocol: "ospf", RemoteID: "1.1.1.1", State: "Full", SnapshotID: snap1},
	})
	db.InsertNeighbors([]model.NeighborInfo{
		{DeviceID: "dev-a", Protocol: "ospf", RemoteID: "1.1.1.1", State: "Down", SnapshotID: snap2},
	})

	// GetLatestNeighbors should return snap2's data
	results, _ := db.GetLatestNeighbors("dev-a")
	if len(results) != 1 { t.Fatalf("expected 1, got %d", len(results)) }
	if results[0].State != "Down" {
		t.Errorf("latest state should be Down, got %s", results[0].State)
	}
}
```

- [ ] **Step 2: Run tests — should fail**

Run: `go test ./internal/store/ -v -run "TestInsertNeighbors|TestGetLatest"`
Expected: FAIL (dedup test fails with 2 rows; GetLatestNeighbors undefined)

- [ ] **Step 3: Modify InsertNeighbors to UPSERT**

In `neighbor_store.go`, change the INSERT SQL to:

```go
stmt, err := tx.Prepare(`INSERT INTO protocol_neighbors
	(device_id, protocol, local_id, remote_id, local_interface, remote_address, state, area_id, as_number, uptime, snapshot_id)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(device_id, protocol, remote_id, remote_address, snapshot_id) DO UPDATE SET
		local_id=excluded.local_id, local_interface=excluded.local_interface,
		state=excluded.state, area_id=excluded.area_id,
		as_number=excluded.as_number, uptime=excluded.uptime`)
```

- [ ] **Step 4: Add GetLatestNeighbors**

```go
func (db *DB) GetLatestNeighbors(deviceID string) ([]model.NeighborInfo, error) {
	snapID, err := db.LatestSnapshotID(deviceID)
	if err != nil {
		return nil, nil // no snapshots = no neighbors
	}
	return db.GetNeighbors(deviceID, snapID)
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/store/ -v -run "TestInsertNeighbors|TestGetLatest"`
Expected: All 3 PASS

- [ ] **Step 6: Commit**

```bash
git add internal/store/neighbor_store.go internal/store/store_test.go
git commit -m "feat(store): neighbor UPSERT dedup + GetLatestNeighbors"
```

---

## Task 3: BGP Peers UPSERT + GetLatestBGPPeers

**Files:**
- Modify: `internal/store/bgp_store.go`
- Modify: `internal/store/store_test.go`

- [ ] **Step 1: Write failing tests**

Similar to Task 2: `TestInsertBGPPeers_Dedup`, `TestInsertBGPPeers_UpdateOnConflict`, `TestGetLatestBGPPeers`.

For `GetLatestBGPPeers`, use the smarter query that finds the latest snapshot with actual BGP peers (fixing the bug we hit in v2):

```go
func TestGetLatestBGPPeers(t *testing.T) {
	db := setupDB(t)
	db.UpsertDevice(model.Device{ID: "dev-a", Hostname: "DEV-A", Vendor: "huawei"})
	snap1, _ := db.CreateSnapshot(model.Snapshot{DeviceID: "dev-a", Commands: "[]"})
	snap2, _ := db.CreateSnapshot(model.Snapshot{DeviceID: "dev-a", Commands: "[]"})

	// BGP peers only on snap1 (like the LC bug: config on older snapshot)
	db.InsertBGPPeers([]model.BGPPeer{
		{DeviceID: "dev-a", PeerIP: "10.0.0.1", RemoteAS: 200, PeerGroup: "X",
		 AddressFamily: "ipv4-unicast", VRF: "default", LocalAS: 100, SnapshotID: snap1},
	})
	// snap2 has no BGP peers

	results, _ := db.GetLatestBGPPeers("dev-a")
	if len(results) != 1 {
		t.Fatalf("expected 1 BGP peer (from snap1), got %d", len(results))
	}
}
```

- [ ] **Step 2: Implement UPSERT + GetLatestBGPPeers**

Change `InsertBGPPeers` SQL to use `ON CONFLICT(device_id, peer_ip, address_family, vrf, snapshot_id) DO UPDATE SET ...`

```go
func (db *DB) GetLatestBGPPeers(deviceID string) ([]model.BGPPeer, error) {
	// Find the latest snapshot that actually has BGP peers
	var snapID int
	err := db.QueryRow(`SELECT snapshot_id FROM bgp_peers WHERE device_id = ? ORDER BY snapshot_id DESC LIMIT 1`, deviceID).Scan(&snapID)
	if err != nil {
		return nil, nil
	}
	return db.GetBGPPeers(deviceID, snapID)
}
```

- [ ] **Step 3: Run tests, commit**

```bash
git commit -m "feat(store): BGP peers UPSERT dedup + GetLatestBGPPeers"
```

---

## Task 4: Capture-time conditional interface updates

**Files:**
- Modify: `internal/store/interface_store.go`
- Modify: `internal/parser/pipeline.go` (pass capturedAt to UpsertInterface)
- Modify: `internal/store/store_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestUpsertInterface_NewerWins(t *testing.T) {
	db := setupDB(t)
	db.UpsertDevice(model.Device{ID: "dev-a", Hostname: "DEV-A"})

	old := model.Interface{
		ID: "dev-a:GE0/0/1", DeviceID: "dev-a", Name: "GE0/0/1",
		Type: model.IfTypePhysical, Status: "up",
		LastUpdated: time.Date(2026, 3, 22, 10, 0, 0, 0, time.Local),
	}
	new := old
	new.Status = "down"
	new.LastUpdated = time.Date(2026, 3, 22, 10, 30, 0, 0, time.Local) // newer

	db.UpsertInterface(old)
	db.UpsertInterface(new)

	ifaces, _ := db.GetInterfaces("dev-a")
	if ifaces[0].Status != "down" {
		t.Errorf("newer data should win, got status=%s", ifaces[0].Status)
	}
}

func TestUpsertInterface_OlderLoses(t *testing.T) {
	db := setupDB(t)
	db.UpsertDevice(model.Device{ID: "dev-a", Hostname: "DEV-A"})

	newer := model.Interface{
		ID: "dev-a:GE0/0/1", DeviceID: "dev-a", Name: "GE0/0/1",
		Type: model.IfTypePhysical, Status: "down",
		LastUpdated: time.Date(2026, 3, 22, 10, 30, 0, 0, time.Local),
	}
	older := newer
	older.Status = "up"
	older.LastUpdated = time.Date(2026, 3, 22, 10, 0, 0, 0, time.Local) // older

	db.UpsertInterface(newer) // insert newer first
	db.UpsertInterface(older) // then older — should NOT overwrite

	ifaces, _ := db.GetInterfaces("dev-a")
	if ifaces[0].Status != "down" {
		t.Errorf("older data should lose, got status=%s", ifaces[0].Status)
	}
}
```

- [ ] **Step 2: Modify UpsertInterface SQL**

```sql
INSERT INTO interfaces (id, device_id, name, type, status, ip_address, mask, vlan, bandwidth, description, parent_id, last_updated)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  name = CASE WHEN excluded.last_updated >= interfaces.last_updated THEN excluded.name ELSE interfaces.name END,
  type = CASE WHEN excluded.last_updated >= interfaces.last_updated THEN excluded.type ELSE interfaces.type END,
  status = CASE WHEN excluded.last_updated >= interfaces.last_updated THEN excluded.status ELSE interfaces.status END,
  ip_address = CASE WHEN excluded.last_updated >= interfaces.last_updated THEN excluded.ip_address ELSE interfaces.ip_address END,
  mask = CASE WHEN excluded.last_updated >= interfaces.last_updated THEN excluded.mask ELSE interfaces.mask END,
  vlan = CASE WHEN excluded.last_updated >= interfaces.last_updated THEN excluded.vlan ELSE interfaces.vlan END,
  bandwidth = CASE WHEN excluded.last_updated >= interfaces.last_updated THEN excluded.bandwidth ELSE interfaces.bandwidth END,
  description = CASE WHEN excluded.last_updated >= interfaces.last_updated THEN excluded.description ELSE interfaces.description END,
  parent_id = CASE WHEN excluded.last_updated >= interfaces.last_updated THEN excluded.parent_id ELSE interfaces.parent_id END,
  last_updated = CASE WHEN excluded.last_updated >= interfaces.last_updated THEN excluded.last_updated ELSE interfaces.last_updated END
```

- [ ] **Step 3: Update pipeline.go to use capturedAt**

In `storeResult` (around line 184-194), change:
```go
// OLD:
iface.LastUpdated = time.Now()

// NEW:
if !capturedAt.IsZero() {
    iface.LastUpdated = capturedAt
} else {
    iface.LastUpdated = time.Now()
}
```

Same for the config-extracted interfaces block (around line 285):
```go
if !capturedAt.IsZero() {
    ifaces[i].LastUpdated = capturedAt
} else {
    ifaces[i].LastUpdated = time.Now()
}
```

- [ ] **Step 4: Run tests, commit**

```bash
git commit -m "feat(store): capture-time conditional interface updates"
```

---

## Task 5: Config hash dedup

**Files:**
- Modify: `internal/store/config_snapshot_store.go`
- Modify: `internal/parser/pipeline.go`
- Modify: `internal/store/store_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestInsertConfigSnapshot_HashDedup(t *testing.T) {
	db := setupDB(t)
	db.UpsertDevice(model.Device{ID: "dev-a", Hostname: "DEV-A"})

	config := "interface GE0/0/1\n ip address 10.0.0.1 255.255.255.0\n"

	db.InsertConfigSnapshot(model.ConfigSnapshot{DeviceID: "dev-a", ConfigText: config})
	db.InsertConfigSnapshot(model.ConfigSnapshot{DeviceID: "dev-a", ConfigText: config}) // same content

	results, _ := db.GetConfigSnapshots("dev-a")
	if len(results) != 1 {
		t.Errorf("identical config should not be stored twice, got %d", len(results))
	}
}

func TestInsertConfigSnapshot_DifferentStored(t *testing.T) {
	db := setupDB(t)
	db.UpsertDevice(model.Device{ID: "dev-a", Hostname: "DEV-A"})

	db.InsertConfigSnapshot(model.ConfigSnapshot{DeviceID: "dev-a", ConfigText: "config v1"})
	db.InsertConfigSnapshot(model.ConfigSnapshot{DeviceID: "dev-a", ConfigText: "config v2"})

	results, _ := db.GetConfigSnapshots("dev-a")
	if len(results) != 2 {
		t.Errorf("different configs should both be stored, got %d", len(results))
	}
}
```

- [ ] **Step 2: Implement hash dedup in InsertConfigSnapshot**

Add to `config_snapshot_store.go`:

```go
import (
	"crypto/sha256"
	"encoding/hex"
)

func (db *DB) InsertConfigSnapshot(cs model.ConfigSnapshot) (int, error) {
	// Compute hash
	hash := sha256.Sum256([]byte(cs.ConfigText))
	hashStr := hex.EncodeToString(hash[:])

	// Check if identical config already exists for this device
	var existing int
	err := db.QueryRow(`SELECT COUNT(*) FROM config_snapshots WHERE device_id = ? AND content_hash = ?`,
		cs.DeviceID, hashStr).Scan(&existing)
	if err == nil && existing > 0 {
		return 0, nil // skip duplicate
	}

	// Original insert logic with hash
	// ... (keep existing conditional insert, add content_hash to columns)
}
```

- [ ] **Step 3: Run tests, commit**

```bash
git commit -m "feat(store): config snapshot hash dedup"
```

---

## Task 6: Per-file mutex

**Files:**
- Modify: `internal/cli/watch.go`

- [ ] **Step 1: Replace global mutex with per-file mutex**

Change lines 70-78 from:
```go
var ingestMu sync.Mutex
// ...
OnFileChange: func(path string) {
    ingestMu.Lock()
    defer ingestMu.Unlock()
```

To:
```go
var fileMutexes sync.Map

OnFileChange: func(path string) {
    muVal, _ := fileMutexes.LoadOrStore(path, &sync.Mutex{})
    mu := muVal.(*sync.Mutex)
    mu.Lock()
    defer mu.Unlock()
```

- [ ] **Step 2: Build and verify**

Run: `go build ./cmd/nethelper`
Expected: Success

- [ ] **Step 3: Commit**

```bash
git add internal/cli/watch.go
git commit -m "feat(watch): per-file mutex for concurrent multi-session ingestion"
```

---

## Task 7: Update topology.go to use GetLatestBGPPeers

**Files:**
- Modify: `internal/plan/topology.go`

- [ ] **Step 1: Replace manual snapshot search with GetLatestBGPPeers**

In `BuildTopology`, replace the `allSnapshotIDs` loop with:

```go
peers, err := db.GetLatestBGPPeers(deviceID)
if err == nil && len(peers) > 0 {
    // ... aggregate into PeerGroups (same logic, just sourced from GetLatestBGPPeers)
}
```

Remove the `allSnapshotIDs` helper function.

- [ ] **Step 2: Run all plan tests**

Run: `go test ./internal/plan/ -v -count=1`
Expected: All pass

- [ ] **Step 3: Commit**

```bash
git add internal/plan/topology.go
git commit -m "refactor(plan): use GetLatestBGPPeers instead of manual snapshot search"
```

---

## Task 8: Integration test — re-ingest and verify

- [ ] **Step 1: Re-ingest LC log twice and verify no duplicates**

```bash
# Clear and re-ingest
sqlite3 ~/.nethelper/nethelper.db "DELETE FROM log_ingestions"
./nethelper watch ingest /Users/xavierli/Work/session_log/netdevice/teg_20260322112428.log
./nethelper watch ingest /Users/xavierli/Work/session_log/netdevice/teg_20260322112428.log
```

- [ ] **Step 2: Verify no duplicate BGP peers**

```bash
sqlite3 ~/.nethelper/nethelper.db "SELECT count(*) FROM bgp_peers WHERE device_id='cd-gx-0201-g17-h12516af-lc-01'"
```
Expected: 234 (not 468)

- [ ] **Step 3: Verify no duplicate config snapshots**

```bash
sqlite3 ~/.nethelper/nethelper.db "SELECT count(*) FROM config_snapshots WHERE device_id='cd-gx-0201-g17-h12516af-lc-01'"
```
Expected: 1 (not 2)

- [ ] **Step 4: Run plan isolate to verify end-to-end**

```bash
./nethelper plan isolate cd-gx-0201-g17-h12516af-lc-01 2>&1 | head -20
```
Expected: Shows 6 peer groups, AS 65508, no duplicates

- [ ] **Step 5: Full test suite**

Run: `go test ./... && go vet ./...`
Expected: All pass

- [ ] **Step 6: Commit**

```bash
git commit -m "test: verify multi-session dedup with real LC data"
```
