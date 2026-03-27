# Nethelper Plan 1: Core Foundation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the foundational layer — Go module, data models, SQLite store with all tables, config loading, CLI skeleton, manual file ingest, and basic `show` commands.

**Architecture:** Single Go binary using Cobra for CLI. Data stored in SQLite via `ncruces/go-sqlite3` (no CGo, WASM-based, supports sqlite-vec for later plans). Config loaded from `~/.nethelper/config.yaml`. All domain types defined in `internal/model/`, database operations in `internal/store/`.

**Tech Stack:** Go 1.22+, ncruces/go-sqlite3, cobra, yaml.v3, slog (stdlib)

**Spec:** `docs/superpowers/specs/2026-03-21-network-helper-design.md`

**Subsequent plans:** Plan 2 (Parser Pipeline + Huawei), Plan 3 (Additional Vendors + Watcher), Plan 4 (Graph Engine + Analysis), Plan 5 (Knowledge + Search), Plan 6 (LLM + Embedding + Export)

---

## File Structure

```
nethelper/
├── cmd/nethelper/main.go                  # Entry point, initializes app and runs Cobra root
├── internal/
│   ├── model/
│   │   ├── device.go                      # Device, Interface structs + type enums
│   │   ├── routing.go                     # RIBEntry, FIBEntry, LFIBEntry structs
│   │   ├── neighbor.go                    # NeighborInfo struct
│   │   ├── tunnel.go                      # TunnelInfo, SRMapping structs
│   │   ├── knowledge.go                   # Snapshot, ConfigSnapshot, TroubleshootLog, CommandReference, LogIngestion
│   │   └── parse_result.go               # ParseResult (unified parser output), CommandType enum
│   ├── config/
│   │   └── config.go                      # Config struct, Load(), default paths
│   ├── store/
│   │   ├── db.go                          # DB struct, Open(), Close(), migration runner
│   │   ├── migrations.go                  # SQL migration strings (all CREATE TABLEs)
│   │   ├── device_store.go               # UpsertDevice, GetDevice, ListDevices
│   │   ├── interface_store.go            # UpsertInterface, GetInterfaces
│   │   ├── snapshot_store.go             # CreateSnapshot, GetSnapshot
│   │   ├── rib_store.go                  # InsertRIBEntries, GetRIBEntries
│   │   ├── fib_store.go                  # InsertFIBEntries, GetFIBEntries
│   │   ├── lfib_store.go                # InsertLFIBEntries, GetLFIBEntries
│   │   ├── neighbor_store.go            # InsertNeighbors, GetNeighbors
│   │   ├── tunnel_store.go              # InsertTunnels, GetTunnels
│   │   ├── ingestion_store.go           # UpsertIngestion, GetIngestion
│   │   └── config_snapshot_store.go     # InsertConfigSnapshot, GetConfigSnapshots
│   └── cli/
│       ├── root.go                        # Root command, global flags (--db, --config)
│       ├── show.go                        # show device/interface/route/fib/label/neighbor/tunnel
│       └── watch.go                       # watch ingest (manual file import, stub for start/stop/status)
├── go.mod
└── go.sum
```

---

### Task 1: Go Module + Entry Point

**Files:**
- Create: `go.mod`
- Create: `cmd/nethelper/main.go`

- [ ] **Step 1: Initialize Go module**

Run: `go mod init github.com/xavierli/nethelper`

- [ ] **Step 2: Create main.go**

```go
// cmd/nethelper/main.go
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("nethelper v0.1.0")
	os.Exit(0)
}
```

- [ ] **Step 3: Verify it compiles and runs**

Run: `go run ./cmd/nethelper`
Expected: `nethelper v0.1.0`

- [ ] **Step 4: Commit**

```bash
git add go.mod cmd/
git commit -m "feat: initialize Go module and entry point"
```

---

### Task 2: Data Model — Device & Interface

**Files:**
- Create: `internal/model/device.go`

- [ ] **Step 1: Write test for interface type validation**

Create: `internal/model/device_test.go`

```go
package model

import "testing"

func TestInterfaceTypeValid(t *testing.T) {
	valid := []InterfaceType{
		IfTypePhysical, IfTypeLoopback, IfTypeVlanif,
		IfTypeEthTrunk, IfTypeTunnelTE, IfTypeTunnelSR,
		IfTypeTunnelGRE, IfTypeNVE, IfTypeNull, IfTypeSubInterface,
	}
	for _, v := range valid {
		if !v.Valid() {
			t.Errorf("expected %q to be valid", v)
		}
	}
	if InterfaceType("bogus").Valid() {
		t.Error("expected 'bogus' to be invalid")
	}
}

func TestDeviceID(t *testing.T) {
	d := Device{Hostname: "Core-01", Vendor: "huawei"}
	if d.Hostname == "" {
		t.Error("hostname should not be empty")
	}
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/model/ -v -run TestInterfaceType`
Expected: FAIL (types not defined)

- [ ] **Step 3: Implement device.go**

```go
// internal/model/device.go
package model

import "time"

type InterfaceType string

const (
	IfTypePhysical     InterfaceType = "physical"
	IfTypeLoopback     InterfaceType = "loopback"
	IfTypeVlanif       InterfaceType = "vlanif"
	IfTypeEthTrunk     InterfaceType = "eth-trunk"
	IfTypeTunnelTE     InterfaceType = "tunnel-te"
	IfTypeTunnelSR     InterfaceType = "tunnel-sr"
	IfTypeTunnelGRE    InterfaceType = "tunnel-gre"
	IfTypeNVE          InterfaceType = "nve"
	IfTypeNull         InterfaceType = "null"
	IfTypeSubInterface InterfaceType = "sub-interface"
)

var validInterfaceTypes = map[InterfaceType]bool{
	IfTypePhysical: true, IfTypeLoopback: true, IfTypeVlanif: true,
	IfTypeEthTrunk: true, IfTypeTunnelTE: true, IfTypeTunnelSR: true,
	IfTypeTunnelGRE: true, IfTypeNVE: true, IfTypeNull: true,
	IfTypeSubInterface: true,
}

func (t InterfaceType) Valid() bool {
	return validInterfaceTypes[t]
}

type Device struct {
	ID        string    `json:"id"`
	Hostname  string    `json:"hostname"`
	Vendor    string    `json:"vendor"`
	Model     string    `json:"model"`
	OSVersion string    `json:"os_version"`
	MgmtIP    string    `json:"mgmt_ip"`
	RouterID  string    `json:"router_id"`
	MPLSLsrID string    `json:"mpls_lsr_id"`
	LastSeen  time.Time `json:"last_seen"`
}

type Interface struct {
	ID          string        `json:"id"`
	DeviceID    string        `json:"device_id"`
	Name        string        `json:"name"`
	Type        InterfaceType `json:"type"`
	Status      string        `json:"status"`
	IPAddress   string        `json:"ip_address"`
	Mask        string        `json:"mask"`
	VLAN        int           `json:"vlan"`
	Bandwidth   string        `json:"bandwidth"`
	Description string        `json:"description"`
	ParentID    string        `json:"parent_id"`
	LastUpdated time.Time     `json:"last_updated"`
}
```

- [ ] **Step 4: Run test — verify PASS**

Run: `go test ./internal/model/ -v -run TestInterfaceType`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/model/device.go internal/model/device_test.go
git commit -m "feat: add Device and Interface data models with type enums"
```

---

### Task 3: Data Model — Routing (RIB + FIB + LFIB)

**Files:**
- Create: `internal/model/routing.go`

- [ ] **Step 1: Write test**

Create: `internal/model/routing_test.go`

```go
package model

import "testing"

func TestRIBEntryPrefixString(t *testing.T) {
	r := RIBEntry{Prefix: "10.0.0.0", MaskLen: 24, Protocol: "ospf"}
	if r.PrefixString() != "10.0.0.0/24" {
		t.Errorf("expected 10.0.0.0/24, got %s", r.PrefixString())
	}
}

func TestFIBEntryLabelAction(t *testing.T) {
	f := FIBEntry{LabelAction: "push", OutLabel: "16001"}
	if f.LabelAction == "" {
		t.Error("label_action should not be empty")
	}
}

func TestLFIBEntryFields(t *testing.T) {
	l := LFIBEntry{InLabel: 16001, Action: "swap", OutLabel: "16002", Protocol: "ldp"}
	if l.InLabel != 16001 {
		t.Errorf("expected 16001, got %d", l.InLabel)
	}
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/model/ -v -run TestRIB`
Expected: FAIL

- [ ] **Step 3: Implement routing.go**

```go
// internal/model/routing.go
package model

import "fmt"

type RIBEntry struct {
	ID                int    `json:"id"`
	DeviceID          string `json:"device_id"`
	VRF               string `json:"vrf"`
	Prefix            string `json:"prefix"`
	MaskLen           int    `json:"mask_len"`
	Protocol          string `json:"protocol"`
	NextHop           string `json:"next_hop"`
	OutgoingInterface string `json:"outgoing_interface"`
	Preference        int    `json:"preference"`
	Metric            int    `json:"metric"`
	Tag               int    `json:"tag"`
	SnapshotID        int    `json:"snapshot_id"`
}

func (r RIBEntry) PrefixString() string {
	return fmt.Sprintf("%s/%d", r.Prefix, r.MaskLen)
}

type FIBEntry struct {
	ID                int    `json:"id"`
	DeviceID          string `json:"device_id"`
	VRF               string `json:"vrf"`
	Prefix            string `json:"prefix"`
	MaskLen           int    `json:"mask_len"`
	NextHop           string `json:"next_hop"`
	OutgoingInterface string `json:"outgoing_interface"`
	LabelAction       string `json:"label_action"`
	OutLabel          string `json:"out_label"`
	TunnelID          string `json:"tunnel_id"`
	SnapshotID        int    `json:"snapshot_id"`
}

func (f FIBEntry) PrefixString() string {
	return fmt.Sprintf("%s/%d", f.Prefix, f.MaskLen)
}

type LFIBEntry struct {
	ID                int    `json:"id"`
	DeviceID          string `json:"device_id"`
	InLabel           int    `json:"in_label"`
	Action            string `json:"action"`
	OutLabel          string `json:"out_label"`
	NextHop           string `json:"next_hop"`
	OutgoingInterface string `json:"outgoing_interface"`
	FEC               string `json:"fec"`
	Protocol          string `json:"protocol"`
	SnapshotID        int    `json:"snapshot_id"`
}
```

- [ ] **Step 4: Run test — verify PASS**

Run: `go test ./internal/model/ -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/model/routing.go internal/model/routing_test.go
git commit -m "feat: add RIB, FIB, LFIB data models"
```

---

### Task 4: Data Model — Neighbor, Tunnel, Knowledge, ParseResult

**Files:**
- Create: `internal/model/neighbor.go`
- Create: `internal/model/tunnel.go`
- Create: `internal/model/knowledge.go`
- Create: `internal/model/parse_result.go`

- [ ] **Step 1: Write test**

Create: `internal/model/all_models_test.go`

```go
package model

import "testing"

func TestCommandTypeString(t *testing.T) {
	tests := []struct {
		ct   CommandType
		want string
	}{
		{CmdRIB, "rib"},
		{CmdFIB, "fib"},
		{CmdLFIB, "lfib"},
		{CmdInterface, "interface"},
		{CmdNeighbor, "neighbor"},
		{CmdTunnel, "tunnel"},
		{CmdSRMapping, "sr_mapping"},
		{CmdConfig, "config"},
		{CmdUnknown, "unknown"},
	}
	for _, tt := range tests {
		if string(tt.ct) != tt.want {
			t.Errorf("expected %q, got %q", tt.want, string(tt.ct))
		}
	}
}

func TestParseResultIsEmpty(t *testing.T) {
	pr := ParseResult{}
	if !pr.IsEmpty() {
		t.Error("empty ParseResult should report IsEmpty=true")
	}
	pr.RIBEntries = []RIBEntry{{Prefix: "10.0.0.0"}}
	if pr.IsEmpty() {
		t.Error("non-empty ParseResult should report IsEmpty=false")
	}
}

func TestSnapshotFields(t *testing.T) {
	s := Snapshot{DeviceID: "dev1", SourceFile: "/tmp/log.txt"}
	if s.DeviceID != "dev1" {
		t.Error("unexpected device_id")
	}
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/model/ -v -run TestCommandType`
Expected: FAIL

- [ ] **Step 3: Implement neighbor.go**

```go
// internal/model/neighbor.go
package model

type NeighborInfo struct {
	ID             int    `json:"id"`
	DeviceID       string `json:"device_id"`
	Protocol       string `json:"protocol"`
	LocalID        string `json:"local_id"`
	RemoteID       string `json:"remote_id"`
	LocalInterface string `json:"local_interface"`
	RemoteAddress  string `json:"remote_address"`
	State          string `json:"state"`
	AreaID         string `json:"area_id"`
	ASNumber       int    `json:"as_number"`
	Uptime         string `json:"uptime"`
	SnapshotID     int    `json:"snapshot_id"`
}
```

- [ ] **Step 4: Implement tunnel.go**

```go
// internal/model/tunnel.go
package model

type TunnelInfo struct {
	ID           int    `json:"id"`
	DeviceID     string `json:"device_id"`
	TunnelName   string `json:"tunnel_name"`
	Type         string `json:"type"`
	Destination  string `json:"destination"`
	State        string `json:"state"`
	SignaledBW   string `json:"signaled_bw"`
	ExplicitPath string `json:"explicit_path"`
	ActualPath   string `json:"actual_path"`
	BindingSID   int    `json:"binding_sid"`
	SnapshotID   int    `json:"snapshot_id"`
}

type SRMapping struct {
	ID        int    `json:"id"`
	DeviceID  string `json:"device_id"`
	Prefix    string `json:"prefix"`
	SIDIndex  int    `json:"sid_index"`
	SIDLabel  int    `json:"sid_label"`
	Algorithm int    `json:"algorithm"`
	Flags     string `json:"flags"`
	Source    string `json:"source"`
	SnapshotID int   `json:"snapshot_id"`
}
```

- [ ] **Step 5: Implement knowledge.go**

```go
// internal/model/knowledge.go
package model

import "time"

type Snapshot struct {
	ID         int       `json:"id"`
	DeviceID   string    `json:"device_id"`
	CapturedAt time.Time `json:"captured_at"`
	SourceFile string    `json:"source_file"`
	Commands   string    `json:"commands"`
}

type ConfigSnapshot struct {
	ID           int       `json:"id"`
	DeviceID     string    `json:"device_id"`
	ConfigText   string    `json:"config_text"`
	DiffFromPrev string    `json:"diff_from_prev"`
	CapturedAt   time.Time `json:"captured_at"`
	SourceFile   string    `json:"source_file"`
}

type TroubleshootLog struct {
	ID           int       `json:"id"`
	DeviceID     string    `json:"device_id"`
	Symptom      string    `json:"symptom"`
	CommandsUsed string    `json:"commands_used"`
	Findings     string    `json:"findings"`
	Resolution   string    `json:"resolution"`
	Tags         string    `json:"tags"`
	CreatedAt    time.Time `json:"created_at"`
}

type CommandReference struct {
	ID            int    `json:"id"`
	Vendor        string `json:"vendor"`
	Command       string `json:"command"`
	Description   string `json:"description"`
	ExampleOutput string `json:"example_output"`
	ParseHint     string `json:"parse_hint"`
	SourceURL     string `json:"source_url"`
}

type LogIngestion struct {
	ID          int       `json:"id"`
	FilePath    string    `json:"file_path"`
	FileHash    string    `json:"file_hash"`
	LastOffset  int64     `json:"last_offset"`
	ProcessedAt time.Time `json:"processed_at"`
}
```

- [ ] **Step 6: Implement parse_result.go**

```go
// internal/model/parse_result.go
package model

type CommandType string

const (
	CmdRIB       CommandType = "rib"
	CmdFIB       CommandType = "fib"
	CmdLFIB      CommandType = "lfib"
	CmdInterface CommandType = "interface"
	CmdNeighbor  CommandType = "neighbor"
	CmdTunnel    CommandType = "tunnel"
	CmdSRMapping CommandType = "sr_mapping"
	CmdConfig    CommandType = "config"
	CmdUnknown   CommandType = "unknown"
)

type ParseResult struct {
	Type        CommandType     `json:"type"`
	Interfaces  []Interface     `json:"interfaces,omitempty"`
	RIBEntries  []RIBEntry      `json:"rib_entries,omitempty"`
	FIBEntries  []FIBEntry      `json:"fib_entries,omitempty"`
	LFIBEntries []LFIBEntry     `json:"lfib_entries,omitempty"`
	Neighbors   []NeighborInfo  `json:"neighbors,omitempty"`
	Tunnels     []TunnelInfo    `json:"tunnels,omitempty"`
	SRMappings  []SRMapping     `json:"sr_mappings,omitempty"`
	RawText     string          `json:"raw_text"`
}

func (pr ParseResult) IsEmpty() bool {
	return len(pr.Interfaces) == 0 &&
		len(pr.RIBEntries) == 0 &&
		len(pr.FIBEntries) == 0 &&
		len(pr.LFIBEntries) == 0 &&
		len(pr.Neighbors) == 0 &&
		len(pr.Tunnels) == 0 &&
		len(pr.SRMappings) == 0
}
```

- [ ] **Step 7: Run all model tests — verify PASS**

Run: `go test ./internal/model/ -v`
Expected: all PASS

- [ ] **Step 8: Commit**

```bash
git add internal/model/
git commit -m "feat: add neighbor, tunnel, knowledge, and parse_result models"
```

---

### Task 5: Config Loading

**Files:**
- Create: `internal/config/config.go`

- [ ] **Step 1: Write test**

Create: `internal/config/config_test.go`

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := Default()
	if cfg.DBPath == "" {
		t.Error("DBPath should have a default")
	}
	if cfg.DataDir == "" {
		t.Error("DataDir should have a default")
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte(`
db_path: /tmp/test.db
watch_dirs:
  - /tmp/logs
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DBPath != "/tmp/test.db" {
		t.Errorf("expected /tmp/test.db, got %s", cfg.DBPath)
	}
	if len(cfg.WatchDirs) != 1 || cfg.WatchDirs[0] != "/tmp/logs" {
		t.Errorf("unexpected watch_dirs: %v", cfg.WatchDirs)
	}
}

func TestLoadMissingFileReturnsDefault(t *testing.T) {
	cfg, err := LoadFrom("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("should not error for missing file: %v", err)
	}
	if cfg.DBPath == "" {
		t.Error("should return default config")
	}
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/config/ -v`
Expected: FAIL

- [ ] **Step 3: Implement config.go**

```go
// internal/config/config.go
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type LLMProviderConfig struct {
	APIKey  string `yaml:"api_key"`
	Model   string `yaml:"model"`
	BaseURL string `yaml:"base_url"`
}

type LLMConfig struct {
	Default   string                       `yaml:"default"`
	Providers map[string]LLMProviderConfig `yaml:"providers"`
	Routing   map[string]string            `yaml:"routing"`
}

type EmbeddingConfig struct {
	Provider  string                       `yaml:"provider"`
	Providers map[string]LLMProviderConfig `yaml:"providers"`
}

type Config struct {
	DataDir   string          `yaml:"data_dir"`
	DBPath    string          `yaml:"db_path"`
	WatchDirs []string        `yaml:"watch_dirs"`
	LLM       LLMConfig       `yaml:"llm"`
	Embedding EmbeddingConfig `yaml:"embedding"`
}

func DefaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".nethelper")
}

func Default() *Config {
	dataDir := DefaultDataDir()
	return &Config{
		DataDir: dataDir,
		DBPath:  filepath.Join(dataDir, "nethelper.db"),
	}
}

func DefaultConfigPath() string {
	return filepath.Join(DefaultDataDir(), "config.yaml")
}

func LoadFrom(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func Load() (*Config, error) {
	return LoadFrom(DefaultConfigPath())
}
```

- [ ] **Step 4: Add yaml.v3 dependency**

Run: `go get gopkg.in/yaml.v3`

- [ ] **Step 5: Run test — verify PASS**

Run: `go test ./internal/config/ -v`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/config/ go.mod go.sum
git commit -m "feat: add config loading with YAML support and defaults"
```

---

### Task 6: SQLite Store — Database + Migrations

**Files:**
- Create: `internal/store/db.go`
- Create: `internal/store/migrations.go`

- [ ] **Step 1: Add ncruces/go-sqlite3 dependency**

Run: `go get github.com/ncruces/go-sqlite3 github.com/ncruces/go-sqlite3/driver github.com/ncruces/go-sqlite3/embed`

- [ ] **Step 2: Write test**

Create: `internal/store/db_test.go`

```go
package store

import (
	"path/filepath"
	"testing"
)

func TestOpenAndMigrate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}
	defer db.Close()

	// Verify tables exist by querying each one
	tables := []string{
		"devices", "interfaces", "snapshots", "rib_entries",
		"fib_entries", "lfib_entries", "protocol_neighbors",
		"mpls_te_tunnels", "sr_mappings", "config_snapshots",
		"troubleshoot_logs", "command_references", "log_ingestions",
		"embedding_meta",
	}
	for _, table := range tables {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
		if err != nil {
			t.Errorf("table %s does not exist: %v", table, err)
		}
	}
}

func TestOpenCreatesDirectory(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sub", "dir", "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}
	db.Close()
}
```

- [ ] **Step 3: Run test — verify FAIL**

Run: `go test ./internal/store/ -v -run TestOpen`
Expected: FAIL

- [ ] **Step 4: Implement migrations.go**

```go
// internal/store/migrations.go
package store

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS devices (
		id TEXT PRIMARY KEY,
		hostname TEXT NOT NULL,
		vendor TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		os_version TEXT NOT NULL DEFAULT '',
		mgmt_ip TEXT NOT NULL DEFAULT '',
		router_id TEXT NOT NULL DEFAULT '',
		mpls_lsr_id TEXT NOT NULL DEFAULT '',
		last_seen TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	`CREATE TABLE IF NOT EXISTS interfaces (
		id TEXT PRIMARY KEY,
		device_id TEXT NOT NULL REFERENCES devices(id),
		name TEXT NOT NULL,
		type TEXT NOT NULL DEFAULT 'physical',
		status TEXT NOT NULL DEFAULT 'down',
		ip_address TEXT NOT NULL DEFAULT '',
		mask TEXT NOT NULL DEFAULT '',
		vlan INTEGER NOT NULL DEFAULT 0,
		bandwidth TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		parent_id TEXT NOT NULL DEFAULT '',
		last_updated TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_interfaces_device ON interfaces(device_id)`,

	`CREATE TABLE IF NOT EXISTS snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL REFERENCES devices(id),
		captured_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		source_file TEXT NOT NULL DEFAULT '',
		commands TEXT NOT NULL DEFAULT '[]'
	)`,
	`CREATE INDEX IF NOT EXISTS idx_snapshots_device ON snapshots(device_id)`,

	`CREATE TABLE IF NOT EXISTS rib_entries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL,
		vrf TEXT NOT NULL DEFAULT 'default',
		prefix TEXT NOT NULL,
		mask_len INTEGER NOT NULL,
		protocol TEXT NOT NULL DEFAULT '',
		next_hop TEXT NOT NULL DEFAULT '',
		outgoing_interface TEXT NOT NULL DEFAULT '',
		preference INTEGER NOT NULL DEFAULT 0,
		metric INTEGER NOT NULL DEFAULT 0,
		tag INTEGER NOT NULL DEFAULT 0,
		snapshot_id INTEGER NOT NULL REFERENCES snapshots(id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_rib_device ON rib_entries(device_id)`,
	`CREATE INDEX IF NOT EXISTS idx_rib_snapshot ON rib_entries(snapshot_id)`,
	`CREATE INDEX IF NOT EXISTS idx_rib_prefix ON rib_entries(prefix, mask_len)`,

	`CREATE TABLE IF NOT EXISTS fib_entries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL,
		vrf TEXT NOT NULL DEFAULT 'default',
		prefix TEXT NOT NULL,
		mask_len INTEGER NOT NULL,
		next_hop TEXT NOT NULL DEFAULT '',
		outgoing_interface TEXT NOT NULL DEFAULT '',
		label_action TEXT NOT NULL DEFAULT 'none',
		out_label TEXT NOT NULL DEFAULT '',
		tunnel_id TEXT NOT NULL DEFAULT '',
		snapshot_id INTEGER NOT NULL REFERENCES snapshots(id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_fib_device ON fib_entries(device_id)`,
	`CREATE INDEX IF NOT EXISTS idx_fib_snapshot ON fib_entries(snapshot_id)`,

	`CREATE TABLE IF NOT EXISTS lfib_entries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL,
		in_label INTEGER NOT NULL,
		action TEXT NOT NULL DEFAULT '',
		out_label TEXT NOT NULL DEFAULT '',
		next_hop TEXT NOT NULL DEFAULT '',
		outgoing_interface TEXT NOT NULL DEFAULT '',
		fec TEXT NOT NULL DEFAULT '',
		protocol TEXT NOT NULL DEFAULT '',
		snapshot_id INTEGER NOT NULL REFERENCES snapshots(id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_lfib_device ON lfib_entries(device_id)`,
	`CREATE INDEX IF NOT EXISTS idx_lfib_snapshot ON lfib_entries(snapshot_id)`,
	`CREATE INDEX IF NOT EXISTS idx_lfib_label ON lfib_entries(in_label)`,

	`CREATE TABLE IF NOT EXISTS protocol_neighbors (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL,
		protocol TEXT NOT NULL,
		local_id TEXT NOT NULL DEFAULT '',
		remote_id TEXT NOT NULL DEFAULT '',
		local_interface TEXT NOT NULL DEFAULT '',
		remote_address TEXT NOT NULL DEFAULT '',
		state TEXT NOT NULL DEFAULT '',
		area_id TEXT NOT NULL DEFAULT '',
		as_number INTEGER NOT NULL DEFAULT 0,
		uptime TEXT NOT NULL DEFAULT '',
		snapshot_id INTEGER NOT NULL REFERENCES snapshots(id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_neighbors_device ON protocol_neighbors(device_id)`,
	`CREATE INDEX IF NOT EXISTS idx_neighbors_snapshot ON protocol_neighbors(snapshot_id)`,

	`CREATE TABLE IF NOT EXISTS mpls_te_tunnels (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL,
		tunnel_name TEXT NOT NULL,
		type TEXT NOT NULL DEFAULT '',
		destination TEXT NOT NULL DEFAULT '',
		state TEXT NOT NULL DEFAULT '',
		signaled_bw TEXT NOT NULL DEFAULT '',
		explicit_path TEXT NOT NULL DEFAULT '[]',
		actual_path TEXT NOT NULL DEFAULT '[]',
		binding_sid INTEGER NOT NULL DEFAULT 0,
		snapshot_id INTEGER NOT NULL REFERENCES snapshots(id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_tunnels_device ON mpls_te_tunnels(device_id)`,

	`CREATE TABLE IF NOT EXISTS sr_mappings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL,
		prefix TEXT NOT NULL,
		sid_index INTEGER NOT NULL DEFAULT 0,
		sid_label INTEGER NOT NULL DEFAULT 0,
		algorithm INTEGER NOT NULL DEFAULT 0,
		flags TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT '',
		snapshot_id INTEGER NOT NULL REFERENCES snapshots(id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_sr_device ON sr_mappings(device_id)`,

	`CREATE TABLE IF NOT EXISTS config_snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL REFERENCES devices(id),
		config_text TEXT NOT NULL DEFAULT '',
		diff_from_prev TEXT NOT NULL DEFAULT '',
		captured_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		source_file TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS idx_config_device ON config_snapshots(device_id)`,

	`CREATE TABLE IF NOT EXISTS troubleshoot_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL DEFAULT '',
		symptom TEXT NOT NULL DEFAULT '',
		commands_used TEXT NOT NULL DEFAULT '',
		findings TEXT NOT NULL DEFAULT '',
		resolution TEXT NOT NULL DEFAULT '',
		tags TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	`CREATE TABLE IF NOT EXISTS command_references (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		vendor TEXT NOT NULL DEFAULT '',
		command TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		example_output TEXT NOT NULL DEFAULT '',
		parse_hint TEXT NOT NULL DEFAULT '',
		source_url TEXT NOT NULL DEFAULT ''
	)`,

	`CREATE TABLE IF NOT EXISTS log_ingestions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_path TEXT NOT NULL UNIQUE,
		file_hash TEXT NOT NULL DEFAULT '',
		last_offset INTEGER NOT NULL DEFAULT 0,
		processed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	`CREATE TABLE IF NOT EXISTS embedding_meta (
		rowid INTEGER PRIMARY KEY AUTOINCREMENT,
		source_type TEXT NOT NULL DEFAULT '',
		source_id INTEGER NOT NULL DEFAULT 0,
		chunk_text TEXT NOT NULL DEFAULT '',
		model_name TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
}
```

- [ ] **Step 5: Implement db.go**

```go
// internal/store/db.go
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

type DB struct {
	*sql.DB
	path string
}

func Open(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	sqlDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for better concurrent read performance
	if _, err := sqlDB.Exec("PRAGMA journal_mode=WAL"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	// Enable foreign keys
	if _, err := sqlDB.Exec("PRAGMA foreign_keys=ON"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	db := &DB{DB: sqlDB, path: dbPath}

	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return db, nil
}

func (db *DB) migrate() error {
	for i, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("migration %d failed: %w", i, err)
		}
	}
	return nil
}

func (db *DB) Path() string {
	return db.path
}
```

- [ ] **Step 6: Run test — verify PASS**

Run: `go test ./internal/store/ -v -run TestOpen`
Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add internal/store/db.go internal/store/migrations.go internal/store/db_test.go go.mod go.sum
git commit -m "feat: add SQLite store with all table migrations"
```

---

### Task 7: Device & Interface Store Operations

**Files:**
- Create: `internal/store/device_store.go`
- Create: `internal/store/interface_store.go`

- [ ] **Step 1: Write test**

Create: `internal/store/device_store_test.go`

```go
package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/xavierli/nethelper/internal/model"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestUpsertAndGetDevice(t *testing.T) {
	db := testDB(t)

	dev := model.Device{
		ID: "core-01", Hostname: "Core-01", Vendor: "huawei",
		Model: "S12700", MgmtIP: "10.0.0.1", LastSeen: time.Now(),
	}
	if err := db.UpsertDevice(dev); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := db.GetDevice("core-01")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Hostname != "Core-01" {
		t.Errorf("expected Core-01, got %s", got.Hostname)
	}

	// Upsert again with updated model
	dev.Model = "S12708"
	if err := db.UpsertDevice(dev); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	got, err = db.GetDevice("core-01")
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.Model != "S12708" {
		t.Errorf("expected S12708, got %s", got.Model)
	}
}

func TestListDevices(t *testing.T) {
	db := testDB(t)

	db.UpsertDevice(model.Device{ID: "d1", Hostname: "D1", Vendor: "huawei", LastSeen: time.Now()})
	db.UpsertDevice(model.Device{ID: "d2", Hostname: "D2", Vendor: "cisco", LastSeen: time.Now()})

	devices, err := db.ListDevices()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(devices) != 2 {
		t.Errorf("expected 2 devices, got %d", len(devices))
	}
}

func TestGetDeviceNotFound(t *testing.T) {
	db := testDB(t)
	_, err := db.GetDevice("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent device")
	}
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/store/ -v -run TestUpsert`
Expected: FAIL

- [ ] **Step 3: Implement device_store.go**

```go
// internal/store/device_store.go
package store

import (
	"database/sql"
	"fmt"

	"github.com/xavierli/nethelper/internal/model"
)

func (db *DB) UpsertDevice(d model.Device) error {
	_, err := db.Exec(`
		INSERT INTO devices (id, hostname, vendor, model, os_version, mgmt_ip, router_id, mpls_lsr_id, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			hostname=excluded.hostname, vendor=excluded.vendor, model=excluded.model,
			os_version=excluded.os_version, mgmt_ip=excluded.mgmt_ip,
			router_id=excluded.router_id, mpls_lsr_id=excluded.mpls_lsr_id,
			last_seen=excluded.last_seen`,
		d.ID, d.Hostname, d.Vendor, d.Model, d.OSVersion, d.MgmtIP, d.RouterID, d.MPLSLsrID, d.LastSeen,
	)
	return err
}

func (db *DB) GetDevice(id string) (model.Device, error) {
	var d model.Device
	err := db.QueryRow(`SELECT id, hostname, vendor, model, os_version, mgmt_ip, router_id, mpls_lsr_id, last_seen
		FROM devices WHERE id = ?`, id).Scan(
		&d.ID, &d.Hostname, &d.Vendor, &d.Model, &d.OSVersion, &d.MgmtIP, &d.RouterID, &d.MPLSLsrID, &d.LastSeen,
	)
	if err == sql.ErrNoRows {
		return d, fmt.Errorf("device %q not found", id)
	}
	return d, err
}

func (db *DB) ListDevices() ([]model.Device, error) {
	rows, err := db.Query(`SELECT id, hostname, vendor, model, os_version, mgmt_ip, router_id, mpls_lsr_id, last_seen
		FROM devices ORDER BY hostname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []model.Device
	for rows.Next() {
		var d model.Device
		if err := rows.Scan(&d.ID, &d.Hostname, &d.Vendor, &d.Model, &d.OSVersion, &d.MgmtIP, &d.RouterID, &d.MPLSLsrID, &d.LastSeen); err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}
```

- [ ] **Step 4: Write interface store test**

Create: `internal/store/interface_store_test.go`

```go
package store

import (
	"testing"
	"time"

	"github.com/xavierli/nethelper/internal/model"
)

func TestUpsertAndGetInterfaces(t *testing.T) {
	db := testDB(t)

	db.UpsertDevice(model.Device{ID: "d1", Hostname: "D1", Vendor: "huawei", LastSeen: time.Now()})

	iface := model.Interface{
		ID: "d1:GE0/0/1", DeviceID: "d1", Name: "GE0/0/1",
		Type: model.IfTypePhysical, Status: "up",
		IPAddress: "10.0.0.1", Mask: "255.255.255.0",
		LastUpdated: time.Now(),
	}
	if err := db.UpsertInterface(iface); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	ifaces, err := db.GetInterfaces("d1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(ifaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(ifaces))
	}
	if ifaces[0].Name != "GE0/0/1" {
		t.Errorf("expected GE0/0/1, got %s", ifaces[0].Name)
	}
}
```

- [ ] **Step 5: Implement interface_store.go**

```go
// internal/store/interface_store.go
package store

import (
	"github.com/xavierli/nethelper/internal/model"
)

func (db *DB) UpsertInterface(i model.Interface) error {
	_, err := db.Exec(`
		INSERT INTO interfaces (id, device_id, name, type, status, ip_address, mask, vlan, bandwidth, description, parent_id, last_updated)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, type=excluded.type, status=excluded.status,
			ip_address=excluded.ip_address, mask=excluded.mask, vlan=excluded.vlan,
			bandwidth=excluded.bandwidth, description=excluded.description,
			parent_id=excluded.parent_id, last_updated=excluded.last_updated`,
		i.ID, i.DeviceID, i.Name, string(i.Type), i.Status, i.IPAddress, i.Mask, i.VLAN, i.Bandwidth, i.Description, i.ParentID, i.LastUpdated,
	)
	return err
}

func (db *DB) GetInterfaces(deviceID string) ([]model.Interface, error) {
	rows, err := db.Query(`SELECT id, device_id, name, type, status, ip_address, mask, vlan, bandwidth, description, parent_id, last_updated
		FROM interfaces WHERE device_id = ? ORDER BY name`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ifaces []model.Interface
	for rows.Next() {
		var i model.Interface
		var ifType string
		if err := rows.Scan(&i.ID, &i.DeviceID, &i.Name, &ifType, &i.Status, &i.IPAddress, &i.Mask, &i.VLAN, &i.Bandwidth, &i.Description, &i.ParentID, &i.LastUpdated); err != nil {
			return nil, err
		}
		i.Type = model.InterfaceType(ifType)
		ifaces = append(ifaces, i)
	}
	return ifaces, rows.Err()
}
```

- [ ] **Step 6: Run all store tests — verify PASS**

Run: `go test ./internal/store/ -v`
Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add internal/store/device_store.go internal/store/device_store_test.go internal/store/interface_store.go internal/store/interface_store_test.go
git commit -m "feat: add device and interface store operations with upsert"
```

---

### Task 8: Snapshot + RIB/FIB/LFIB Store Operations

**Files:**
- Create: `internal/store/snapshot_store.go`
- Create: `internal/store/rib_store.go`
- Create: `internal/store/fib_store.go`
- Create: `internal/store/lfib_store.go`

- [ ] **Step 1: Write test**

Create: `internal/store/routing_store_test.go`

```go
package store

import (
	"testing"
	"time"

	"github.com/xavierli/nethelper/internal/model"
)

func seedDevice(t *testing.T, db *DB) {
	t.Helper()
	db.UpsertDevice(model.Device{ID: "d1", Hostname: "D1", Vendor: "huawei", LastSeen: time.Now()})
}

func TestCreateSnapshotAndRIB(t *testing.T) {
	db := testDB(t)
	seedDevice(t, db)

	snapID, err := db.CreateSnapshot(model.Snapshot{DeviceID: "d1", SourceFile: "/tmp/log.txt", Commands: `["display ip routing-table"]`})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	if snapID == 0 {
		t.Error("snapshot ID should be non-zero")
	}

	entries := []model.RIBEntry{
		{DeviceID: "d1", Prefix: "10.0.0.0", MaskLen: 24, Protocol: "ospf", NextHop: "10.0.0.2", SnapshotID: snapID},
		{DeviceID: "d1", Prefix: "172.16.0.0", MaskLen: 16, Protocol: "bgp", NextHop: "10.0.0.3", SnapshotID: snapID},
	}
	if err := db.InsertRIBEntries(entries); err != nil {
		t.Fatalf("insert rib: %v", err)
	}

	got, err := db.GetRIBEntries("d1", snapID)
	if err != nil {
		t.Fatalf("get rib: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 RIB entries, got %d", len(got))
	}
}

func TestInsertAndGetFIB(t *testing.T) {
	db := testDB(t)
	seedDevice(t, db)

	snapID, _ := db.CreateSnapshot(model.Snapshot{DeviceID: "d1", SourceFile: "/tmp/log.txt"})
	entries := []model.FIBEntry{
		{DeviceID: "d1", Prefix: "10.0.0.0", MaskLen: 24, NextHop: "10.0.0.2", LabelAction: "push", OutLabel: "16001", SnapshotID: snapID},
	}
	if err := db.InsertFIBEntries(entries); err != nil {
		t.Fatalf("insert fib: %v", err)
	}
	got, err := db.GetFIBEntries("d1", snapID)
	if err != nil {
		t.Fatalf("get fib: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1, got %d", len(got))
	}
}

func TestInsertAndGetLFIB(t *testing.T) {
	db := testDB(t)
	seedDevice(t, db)

	snapID, _ := db.CreateSnapshot(model.Snapshot{DeviceID: "d1", SourceFile: "/tmp/log.txt"})
	entries := []model.LFIBEntry{
		{DeviceID: "d1", InLabel: 16001, Action: "swap", OutLabel: "16002", NextHop: "10.0.0.2", Protocol: "ldp", SnapshotID: snapID},
	}
	if err := db.InsertLFIBEntries(entries); err != nil {
		t.Fatalf("insert lfib: %v", err)
	}
	got, err := db.GetLFIBEntries("d1", snapID)
	if err != nil {
		t.Fatalf("get lfib: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1, got %d", len(got))
	}
	if got[0].InLabel != 16001 {
		t.Errorf("expected in_label 16001, got %d", got[0].InLabel)
	}
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/store/ -v -run TestCreateSnapshot`
Expected: FAIL

- [ ] **Step 3: Implement snapshot_store.go**

```go
// internal/store/snapshot_store.go
package store

import (
	"github.com/xavierli/nethelper/internal/model"
)

func (db *DB) CreateSnapshot(s model.Snapshot) (int, error) {
	if s.Commands == "" {
		s.Commands = "[]"
	}
	result, err := db.Exec(`INSERT INTO snapshots (device_id, source_file, commands) VALUES (?, ?, ?)`,
		s.DeviceID, s.SourceFile, s.Commands)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return int(id), err
}

func (db *DB) GetSnapshot(id int) (model.Snapshot, error) {
	var s model.Snapshot
	err := db.QueryRow(`SELECT id, device_id, captured_at, source_file, commands FROM snapshots WHERE id = ?`, id).
		Scan(&s.ID, &s.DeviceID, &s.CapturedAt, &s.SourceFile, &s.Commands)
	return s, err
}

func (db *DB) LatestSnapshotID(deviceID string) (int, error) {
	var id int
	err := db.QueryRow(`SELECT id FROM snapshots WHERE device_id = ? ORDER BY captured_at DESC LIMIT 1`, deviceID).Scan(&id)
	return id, err
}
```

- [ ] **Step 4: Implement rib_store.go**

```go
// internal/store/rib_store.go
package store

import (
	"github.com/xavierli/nethelper/internal/model"
)

func (db *DB) InsertRIBEntries(entries []model.RIBEntry) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO rib_entries
		(device_id, vrf, prefix, mask_len, protocol, next_hop, outgoing_interface, preference, metric, tag, snapshot_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, e := range entries {
		if e.VRF == "" {
			e.VRF = "default"
		}
		if _, err := stmt.Exec(e.DeviceID, e.VRF, e.Prefix, e.MaskLen, e.Protocol, e.NextHop, e.OutgoingInterface, e.Preference, e.Metric, e.Tag, e.SnapshotID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) GetRIBEntries(deviceID string, snapshotID int) ([]model.RIBEntry, error) {
	rows, err := db.Query(`SELECT id, device_id, vrf, prefix, mask_len, protocol, next_hop, outgoing_interface, preference, metric, tag, snapshot_id
		FROM rib_entries WHERE device_id = ? AND snapshot_id = ? ORDER BY prefix, mask_len`, deviceID, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.RIBEntry
	for rows.Next() {
		var e model.RIBEntry
		if err := rows.Scan(&e.ID, &e.DeviceID, &e.VRF, &e.Prefix, &e.MaskLen, &e.Protocol, &e.NextHop, &e.OutgoingInterface, &e.Preference, &e.Metric, &e.Tag, &e.SnapshotID); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
```

- [ ] **Step 5: Implement fib_store.go**

```go
// internal/store/fib_store.go
package store

import (
	"github.com/xavierli/nethelper/internal/model"
)

func (db *DB) InsertFIBEntries(entries []model.FIBEntry) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO fib_entries
		(device_id, vrf, prefix, mask_len, next_hop, outgoing_interface, label_action, out_label, tunnel_id, snapshot_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, e := range entries {
		if e.VRF == "" {
			e.VRF = "default"
		}
		if _, err := stmt.Exec(e.DeviceID, e.VRF, e.Prefix, e.MaskLen, e.NextHop, e.OutgoingInterface, e.LabelAction, e.OutLabel, e.TunnelID, e.SnapshotID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) GetFIBEntries(deviceID string, snapshotID int) ([]model.FIBEntry, error) {
	rows, err := db.Query(`SELECT id, device_id, vrf, prefix, mask_len, next_hop, outgoing_interface, label_action, out_label, tunnel_id, snapshot_id
		FROM fib_entries WHERE device_id = ? AND snapshot_id = ? ORDER BY prefix, mask_len`, deviceID, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.FIBEntry
	for rows.Next() {
		var e model.FIBEntry
		if err := rows.Scan(&e.ID, &e.DeviceID, &e.VRF, &e.Prefix, &e.MaskLen, &e.NextHop, &e.OutgoingInterface, &e.LabelAction, &e.OutLabel, &e.TunnelID, &e.SnapshotID); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
```

- [ ] **Step 6: Implement lfib_store.go**

```go
// internal/store/lfib_store.go
package store

import (
	"github.com/xavierli/nethelper/internal/model"
)

func (db *DB) InsertLFIBEntries(entries []model.LFIBEntry) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO lfib_entries
		(device_id, in_label, action, out_label, next_hop, outgoing_interface, fec, protocol, snapshot_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, e := range entries {
		if _, err := stmt.Exec(e.DeviceID, e.InLabel, e.Action, e.OutLabel, e.NextHop, e.OutgoingInterface, e.FEC, e.Protocol, e.SnapshotID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) GetLFIBEntries(deviceID string, snapshotID int) ([]model.LFIBEntry, error) {
	rows, err := db.Query(`SELECT id, device_id, in_label, action, out_label, next_hop, outgoing_interface, fec, protocol, snapshot_id
		FROM lfib_entries WHERE device_id = ? AND snapshot_id = ? ORDER BY in_label`, deviceID, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.LFIBEntry
	for rows.Next() {
		var e model.LFIBEntry
		if err := rows.Scan(&e.ID, &e.DeviceID, &e.InLabel, &e.Action, &e.OutLabel, &e.NextHop, &e.OutgoingInterface, &e.FEC, &e.Protocol, &e.SnapshotID); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
```

- [ ] **Step 7: Run all tests — verify PASS**

Run: `go test ./internal/store/ -v`
Expected: all PASS

- [ ] **Step 8: Commit**

```bash
git add internal/store/snapshot_store.go internal/store/rib_store.go internal/store/fib_store.go internal/store/lfib_store.go internal/store/routing_store_test.go
git commit -m "feat: add snapshot, RIB, FIB, LFIB store operations"
```

---

### Task 9: Neighbor + Tunnel + Ingestion Store Operations

**Files:**
- Create: `internal/store/neighbor_store.go`
- Create: `internal/store/tunnel_store.go`
- Create: `internal/store/ingestion_store.go`
- Create: `internal/store/config_snapshot_store.go`

- [ ] **Step 1: Write test**

Create: `internal/store/remaining_store_test.go`

```go
package store

import (
	"testing"
	"time"

	"github.com/xavierli/nethelper/internal/model"
)

func TestInsertAndGetNeighbors(t *testing.T) {
	db := testDB(t)
	seedDevice(t, db)
	snapID, _ := db.CreateSnapshot(model.Snapshot{DeviceID: "d1", SourceFile: "/tmp/log.txt"})

	neighbors := []model.NeighborInfo{
		{DeviceID: "d1", Protocol: "ospf", RemoteID: "2.2.2.2", State: "full", AreaID: "0.0.0.0", SnapshotID: snapID},
	}
	if err := db.InsertNeighbors(neighbors); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := db.GetNeighbors("d1", snapID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 1 || got[0].Protocol != "ospf" {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestInsertAndGetTunnels(t *testing.T) {
	db := testDB(t)
	seedDevice(t, db)
	snapID, _ := db.CreateSnapshot(model.Snapshot{DeviceID: "d1", SourceFile: "/tmp/log.txt"})

	tunnels := []model.TunnelInfo{
		{DeviceID: "d1", TunnelName: "Tunnel1", Type: "rsvp-te", State: "up", Destination: "3.3.3.3", SnapshotID: snapID},
	}
	if err := db.InsertTunnels(tunnels); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := db.GetTunnels("d1", snapID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 1 || got[0].TunnelName != "Tunnel1" {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestUpsertIngestion(t *testing.T) {
	db := testDB(t)

	ing := model.LogIngestion{FilePath: "/tmp/log.txt", FileHash: "abc123", LastOffset: 1024, ProcessedAt: time.Now()}
	if err := db.UpsertIngestion(ing); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := db.GetIngestion("/tmp/log.txt")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.LastOffset != 1024 {
		t.Errorf("expected 1024, got %d", got.LastOffset)
	}

	// Update offset
	ing.LastOffset = 2048
	if err := db.UpsertIngestion(ing); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	got, err = db.GetIngestion("/tmp/log.txt")
	if err != nil {
		t.Fatalf("get updated: %v", err)
	}
	if got.LastOffset != 2048 {
		t.Errorf("expected 2048, got %d", got.LastOffset)
	}
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/store/ -v -run TestInsertAndGetNeighbors`
Expected: FAIL

- [ ] **Step 3: Implement neighbor_store.go**

```go
// internal/store/neighbor_store.go
package store

import "github.com/xavierli/nethelper/internal/model"

func (db *DB) InsertNeighbors(entries []model.NeighborInfo) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO protocol_neighbors
		(device_id, protocol, local_id, remote_id, local_interface, remote_address, state, area_id, as_number, uptime, snapshot_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, e := range entries {
		if _, err := stmt.Exec(e.DeviceID, e.Protocol, e.LocalID, e.RemoteID, e.LocalInterface, e.RemoteAddress, e.State, e.AreaID, e.ASNumber, e.Uptime, e.SnapshotID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) GetNeighbors(deviceID string, snapshotID int) ([]model.NeighborInfo, error) {
	rows, err := db.Query(`SELECT id, device_id, protocol, local_id, remote_id, local_interface, remote_address, state, area_id, as_number, uptime, snapshot_id
		FROM protocol_neighbors WHERE device_id = ? AND snapshot_id = ? ORDER BY protocol, remote_id`, deviceID, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.NeighborInfo
	for rows.Next() {
		var e model.NeighborInfo
		if err := rows.Scan(&e.ID, &e.DeviceID, &e.Protocol, &e.LocalID, &e.RemoteID, &e.LocalInterface, &e.RemoteAddress, &e.State, &e.AreaID, &e.ASNumber, &e.Uptime, &e.SnapshotID); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
```

- [ ] **Step 4: Implement tunnel_store.go**

```go
// internal/store/tunnel_store.go
package store

import "github.com/xavierli/nethelper/internal/model"

func (db *DB) InsertTunnels(entries []model.TunnelInfo) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO mpls_te_tunnels
		(device_id, tunnel_name, type, destination, state, signaled_bw, explicit_path, actual_path, binding_sid, snapshot_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, e := range entries {
		if e.ExplicitPath == "" {
			e.ExplicitPath = "[]"
		}
		if e.ActualPath == "" {
			e.ActualPath = "[]"
		}
		if _, err := stmt.Exec(e.DeviceID, e.TunnelName, e.Type, e.Destination, e.State, e.SignaledBW, e.ExplicitPath, e.ActualPath, e.BindingSID, e.SnapshotID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) GetTunnels(deviceID string, snapshotID int) ([]model.TunnelInfo, error) {
	rows, err := db.Query(`SELECT id, device_id, tunnel_name, type, destination, state, signaled_bw, explicit_path, actual_path, binding_sid, snapshot_id
		FROM mpls_te_tunnels WHERE device_id = ? AND snapshot_id = ? ORDER BY tunnel_name`, deviceID, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.TunnelInfo
	for rows.Next() {
		var e model.TunnelInfo
		if err := rows.Scan(&e.ID, &e.DeviceID, &e.TunnelName, &e.Type, &e.Destination, &e.State, &e.SignaledBW, &e.ExplicitPath, &e.ActualPath, &e.BindingSID, &e.SnapshotID); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
```

- [ ] **Step 5: Implement ingestion_store.go**

```go
// internal/store/ingestion_store.go
package store

import (
	"database/sql"
	"fmt"

	"github.com/xavierli/nethelper/internal/model"
)

func (db *DB) UpsertIngestion(ing model.LogIngestion) error {
	_, err := db.Exec(`
		INSERT INTO log_ingestions (file_path, file_hash, last_offset, processed_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(file_path) DO UPDATE SET
			file_hash=excluded.file_hash, last_offset=excluded.last_offset, processed_at=excluded.processed_at`,
		ing.FilePath, ing.FileHash, ing.LastOffset, ing.ProcessedAt,
	)
	return err
}

func (db *DB) GetIngestion(filePath string) (model.LogIngestion, error) {
	var ing model.LogIngestion
	err := db.QueryRow(`SELECT id, file_path, file_hash, last_offset, processed_at FROM log_ingestions WHERE file_path = ?`, filePath).
		Scan(&ing.ID, &ing.FilePath, &ing.FileHash, &ing.LastOffset, &ing.ProcessedAt)
	if err == sql.ErrNoRows {
		return ing, fmt.Errorf("ingestion record for %q not found", filePath)
	}
	return ing, err
}
```

- [ ] **Step 6: Implement config_snapshot_store.go**

```go
// internal/store/config_snapshot_store.go
package store

import "github.com/xavierli/nethelper/internal/model"

func (db *DB) InsertConfigSnapshot(cs model.ConfigSnapshot) (int, error) {
	result, err := db.Exec(`INSERT INTO config_snapshots (device_id, config_text, diff_from_prev, source_file)
		VALUES (?, ?, ?, ?)`, cs.DeviceID, cs.ConfigText, cs.DiffFromPrev, cs.SourceFile)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return int(id), err
}

func (db *DB) GetConfigSnapshots(deviceID string) ([]model.ConfigSnapshot, error) {
	rows, err := db.Query(`SELECT id, device_id, config_text, diff_from_prev, captured_at, source_file
		FROM config_snapshots WHERE device_id = ? ORDER BY captured_at DESC`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []model.ConfigSnapshot
	for rows.Next() {
		var cs model.ConfigSnapshot
		if err := rows.Scan(&cs.ID, &cs.DeviceID, &cs.ConfigText, &cs.DiffFromPrev, &cs.CapturedAt, &cs.SourceFile); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, cs)
	}
	return snapshots, rows.Err()
}
```

- [ ] **Step 7: Implement sr_mapping_store.go**

```go
// internal/store/sr_mapping_store.go
package store

import "github.com/xavierli/nethelper/internal/model"

func (db *DB) InsertSRMappings(entries []model.SRMapping) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO sr_mappings
		(device_id, prefix, sid_index, sid_label, algorithm, flags, source, snapshot_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, e := range entries {
		if _, err := stmt.Exec(e.DeviceID, e.Prefix, e.SIDIndex, e.SIDLabel, e.Algorithm, e.Flags, e.Source, e.SnapshotID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) GetSRMappings(deviceID string, snapshotID int) ([]model.SRMapping, error) {
	rows, err := db.Query(`SELECT id, device_id, prefix, sid_index, sid_label, algorithm, flags, source, snapshot_id
		FROM sr_mappings WHERE device_id = ? AND snapshot_id = ? ORDER BY prefix`, deviceID, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.SRMapping
	for rows.Next() {
		var e model.SRMapping
		if err := rows.Scan(&e.ID, &e.DeviceID, &e.Prefix, &e.SIDIndex, &e.SIDLabel, &e.Algorithm, &e.Flags, &e.Source, &e.SnapshotID); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
```

- [ ] **Step 8: Add SR mapping test to remaining_store_test.go**

Append to the test file:

```go
func TestInsertAndGetSRMappings(t *testing.T) {
	db := testDB(t)
	seedDevice(t, db)
	snapID, _ := db.CreateSnapshot(model.Snapshot{DeviceID: "d1", SourceFile: "/tmp/log.txt"})

	mappings := []model.SRMapping{
		{DeviceID: "d1", Prefix: "1.1.1.1", SIDIndex: 100, SIDLabel: 16100, Algorithm: 0, Source: "isis", SnapshotID: snapID},
	}
	if err := db.InsertSRMappings(mappings); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := db.GetSRMappings("d1", snapID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 1 || got[0].SIDLabel != 16100 {
		t.Errorf("unexpected: %+v", got)
	}
}
```

- [ ] **Step 9: Run all store tests — verify PASS**

Run: `go test ./internal/store/ -v`
Expected: all PASS

- [ ] **Step 10: Commit**

```bash
git add internal/store/neighbor_store.go internal/store/tunnel_store.go internal/store/ingestion_store.go internal/store/config_snapshot_store.go internal/store/sr_mapping_store.go internal/store/remaining_store_test.go
git commit -m "feat: add neighbor, tunnel, SR mapping, ingestion, config_snapshot store operations"
```

---

### Task 10: CLI Skeleton — Root + Show Commands

**Files:**
- Create: `internal/cli/root.go`
- Create: `internal/cli/show.go`
- Modify: `cmd/nethelper/main.go`

- [ ] **Step 1: Add cobra dependency**

Run: `go get github.com/spf13/cobra`

- [ ] **Step 2: Implement root.go**

```go
// internal/cli/root.go
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/config"
	"github.com/xavierli/nethelper/internal/store"
)

var (
	cfgFile string
	dbPath  string
	cfg     *config.Config
	db      *store.DB
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "nethelper",
		Short: "Network troubleshooting helper with memory",
		Long:  "CLI tool for network engineers — parses device logs, builds topology, tracks changes, and learns from troubleshooting history.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			var err error
			cfg, err = config.LoadFrom(cfgFile)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if dbPath != "" {
				cfg.DBPath = dbPath
			}

			// Commands that don't need DB
			if cmd.Name() == "version" {
				return nil
			}

			db, err = store.Open(cfg.DBPath)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if db != nil {
				db.Close()
			}
		},
	}

	root.PersistentFlags().StringVar(&cfgFile, "config", config.DefaultConfigPath(), "config file path")
	root.PersistentFlags().StringVar(&dbPath, "db", "", "database file path (overrides config)")

	root.AddCommand(newVersionCmd())
	root.AddCommand(newShowCmd())

	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("nethelper v0.1.0")
		},
	}
}

func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
```

- [ ] **Step 3: Implement show.go**

```go
// internal/cli/show.go
package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newShowCmd() *cobra.Command {
	show := &cobra.Command{
		Use:   "show",
		Short: "Query network data",
	}

	show.AddCommand(newShowDeviceCmd())
	show.AddCommand(newShowInterfaceCmd())
	show.AddCommand(newShowRouteCmd())
	show.AddCommand(newShowFIBCmd())
	show.AddCommand(newShowLabelCmd())
	show.AddCommand(newShowNeighborCmd())
	show.AddCommand(newShowTunnelCmd())

	return show
}

func newShowDeviceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "device [device-id]",
		Short: "Show device information",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				d, err := db.GetDevice(args[0])
				if err != nil {
					return err
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintf(w, "ID:\t%s\n", d.ID)
				fmt.Fprintf(w, "Hostname:\t%s\n", d.Hostname)
				fmt.Fprintf(w, "Vendor:\t%s\n", d.Vendor)
				fmt.Fprintf(w, "Model:\t%s\n", d.Model)
				fmt.Fprintf(w, "OS Version:\t%s\n", d.OSVersion)
				fmt.Fprintf(w, "Mgmt IP:\t%s\n", d.MgmtIP)
				fmt.Fprintf(w, "Router-ID:\t%s\n", d.RouterID)
				fmt.Fprintf(w, "MPLS LSR-ID:\t%s\n", d.MPLSLsrID)
				fmt.Fprintf(w, "Last Seen:\t%s\n", d.LastSeen.Format("2006-01-02 15:04:05"))
				return w.Flush()
			}

			devices, err := db.ListDevices()
			if err != nil {
				return err
			}
			if len(devices) == 0 {
				fmt.Println("No devices found. Use 'nethelper watch ingest <file>' to import logs.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "ID\tHOSTNAME\tVENDOR\tMODEL\tMGMT IP\tLAST SEEN\n")
			for _, d := range devices {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
					d.ID, d.Hostname, d.Vendor, d.Model, d.MgmtIP, d.LastSeen.Format("2006-01-02 15:04"))
			}
			return w.Flush()
		},
	}
}

func newShowInterfaceCmd() *cobra.Command {
	var deviceID string
	cmd := &cobra.Command{
		Use:   "interface",
		Short: "Show interface information",
		RunE: func(cmd *cobra.Command, args []string) error {
			if deviceID == "" {
				return fmt.Errorf("--device is required")
			}
			ifaces, err := db.GetInterfaces(deviceID)
			if err != nil {
				return err
			}
			if len(ifaces) == 0 {
				fmt.Println("No interfaces found.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "NAME\tTYPE\tSTATUS\tIP\tDESCRIPTION\n")
			for _, i := range ifaces {
				ip := i.IPAddress
				if ip != "" && i.Mask != "" {
					ip = ip + "/" + i.Mask
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", i.Name, i.Type, i.Status, ip, i.Description)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&deviceID, "device", "", "device ID")
	return cmd
}

func newShowRouteCmd() *cobra.Command {
	var deviceID, prefix, protocol string
	cmd := &cobra.Command{
		Use:   "route",
		Short: "Show routing table (RIB)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if deviceID == "" {
				return fmt.Errorf("--device is required")
			}
			snapID, err := db.LatestSnapshotID(deviceID)
			if err != nil {
				return fmt.Errorf("no snapshots found for device %s", deviceID)
			}
			entries, err := db.GetRIBEntries(deviceID, snapID)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("No RIB entries found.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "PREFIX\tPROTOCOL\tNEXT-HOP\tINTERFACE\tPREF\tMETRIC\tVRF\n")
			for _, e := range entries {
				if prefix != "" && e.Prefix != prefix {
					continue
				}
				if protocol != "" && e.Protocol != protocol {
					continue
				}
				fmt.Fprintf(w, "%s/%d\t%s\t%s\t%s\t%d\t%d\t%s\n",
					e.Prefix, e.MaskLen, e.Protocol, e.NextHop, e.OutgoingInterface, e.Preference, e.Metric, e.VRF)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&deviceID, "device", "", "device ID")
	cmd.Flags().StringVar(&prefix, "prefix", "", "filter by prefix")
	cmd.Flags().StringVar(&protocol, "protocol", "", "filter by protocol")
	return cmd
}

func newShowFIBCmd() *cobra.Command {
	var deviceID string
	cmd := &cobra.Command{
		Use:   "fib",
		Short: "Show forwarding table (FIB)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if deviceID == "" {
				return fmt.Errorf("--device is required")
			}
			snapID, err := db.LatestSnapshotID(deviceID)
			if err != nil {
				return fmt.Errorf("no snapshots found for device %s", deviceID)
			}
			entries, err := db.GetFIBEntries(deviceID, snapID)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("No FIB entries found.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "PREFIX\tNEXT-HOP\tINTERFACE\tLABEL-ACTION\tOUT-LABEL\tTUNNEL\n")
			for _, e := range entries {
				fmt.Fprintf(w, "%s/%d\t%s\t%s\t%s\t%s\t%s\n",
					e.Prefix, e.MaskLen, e.NextHop, e.OutgoingInterface, e.LabelAction, e.OutLabel, e.TunnelID)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&deviceID, "device", "", "device ID")
	return cmd
}

func newShowLabelCmd() *cobra.Command {
	var deviceID string
	cmd := &cobra.Command{
		Use:   "label",
		Short: "Show label forwarding table (LFIB)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if deviceID == "" {
				return fmt.Errorf("--device is required")
			}
			snapID, err := db.LatestSnapshotID(deviceID)
			if err != nil {
				return fmt.Errorf("no snapshots found for device %s", deviceID)
			}
			entries, err := db.GetLFIBEntries(deviceID, snapID)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("No LFIB entries found.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "IN-LABEL\tACTION\tOUT-LABEL\tNEXT-HOP\tINTERFACE\tFEC\tPROTOCOL\n")
			for _, e := range entries {
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
					e.InLabel, e.Action, e.OutLabel, e.NextHop, e.OutgoingInterface, e.FEC, e.Protocol)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&deviceID, "device", "", "device ID")
	return cmd
}

func newShowNeighborCmd() *cobra.Command {
	var deviceID, protocol string
	cmd := &cobra.Command{
		Use:   "neighbor",
		Short: "Show protocol neighbors",
		RunE: func(cmd *cobra.Command, args []string) error {
			if deviceID == "" {
				return fmt.Errorf("--device is required")
			}
			snapID, err := db.LatestSnapshotID(deviceID)
			if err != nil {
				return fmt.Errorf("no snapshots found for device %s", deviceID)
			}
			entries, err := db.GetNeighbors(deviceID, snapID)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("No neighbors found.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "PROTOCOL\tREMOTE-ID\tSTATE\tINTERFACE\tAREA\tAS\tUPTIME\n")
			for _, e := range entries {
				if protocol != "" && e.Protocol != protocol {
					continue
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\t%s\n",
					e.Protocol, e.RemoteID, e.State, e.LocalInterface, e.AreaID, e.ASNumber, e.Uptime)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&deviceID, "device", "", "device ID")
	cmd.Flags().StringVar(&protocol, "protocol", "", "filter by protocol (ospf/bgp/isis/ldp/rsvp)")
	return cmd
}

func newShowTunnelCmd() *cobra.Command {
	var deviceID, tunnelType string
	cmd := &cobra.Command{
		Use:   "tunnel",
		Short: "Show TE/SR tunnels",
		RunE: func(cmd *cobra.Command, args []string) error {
			if deviceID == "" {
				return fmt.Errorf("--device is required")
			}
			snapID, err := db.LatestSnapshotID(deviceID)
			if err != nil {
				return fmt.Errorf("no snapshots found for device %s", deviceID)
			}
			entries, err := db.GetTunnels(deviceID, snapID)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("No tunnels found.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "NAME\tTYPE\tSTATE\tDESTINATION\tBINDING-SID\tBW\n")
			for _, e := range entries {
				if tunnelType != "" && e.Type != tunnelType {
					continue
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n",
					e.TunnelName, e.Type, e.State, e.Destination, e.BindingSID, e.SignaledBW)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&deviceID, "device", "", "device ID")
	cmd.Flags().StringVar(&tunnelType, "type", "", "filter by type (rsvp-te/sr-te/sr-policy)")
	return cmd
}
```

- [ ] **Step 4: Update main.go**

```go
// cmd/nethelper/main.go
package main

import "github.com/xavierli/nethelper/internal/cli"

func main() {
	cli.Execute()
}
```

- [ ] **Step 5: Build and verify**

Run: `go build ./cmd/nethelper && ./nethelper version`
Expected: `nethelper v0.1.0`

Run: `./nethelper show device --db /tmp/test-cli.db`
Expected: `No devices found. Use 'nethelper watch ingest <file>' to import logs.`

Run: `./nethelper show --help`
Expected: Shows all show subcommands (device, interface, route, fib, label, neighbor, tunnel)

- [ ] **Step 6: Commit**

```bash
git add internal/cli/ cmd/nethelper/main.go go.mod go.sum
git commit -m "feat: add CLI skeleton with root, version, and show commands"
```

---

### Task 11: Watch Ingest — Manual File Import (Stub)

This is a stub that reads a file and stores it as raw ingestion. The actual parsing happens in Plan 2. For now, it records the ingestion and stores raw text.

**Files:**
- Create: `internal/cli/watch.go`

- [ ] **Step 1: Implement watch.go with ingest subcommand**

```go
// internal/cli/watch.go
package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/model"
)

func newWatchCmd() *cobra.Command {
	watch := &cobra.Command{
		Use:   "watch",
		Short: "File monitoring and ingestion",
	}

	watch.AddCommand(newWatchIngestCmd())
	watch.AddCommand(newWatchStartStub())
	watch.AddCommand(newWatchStopStub())
	watch.AddCommand(newWatchStatusStub())

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
			if err != nil {
				return fmt.Errorf("read file: %w", err)
			}

			ing := model.LogIngestion{
				FilePath:    filePath,
				LastOffset:  int64(len(data)),
				ProcessedAt: time.Now(),
			}
			if err := db.UpsertIngestion(ing); err != nil {
				return fmt.Errorf("record ingestion: %w", err)
			}

			fmt.Printf("Ingested %s (%d bytes)\n", filePath, len(data))
			fmt.Println("Note: parsing not yet implemented (Plan 2). Raw file recorded.")
			return nil
		},
	}
}

func newWatchStartStub() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start file watcher daemon (not yet implemented)",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Watch daemon not yet implemented (Plan 3).")
		},
	}
}

func newWatchStopStub() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop file watcher daemon (not yet implemented)",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Watch daemon not yet implemented (Plan 3).")
		},
	}
}

func newWatchStatusStub() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show watcher status (not yet implemented)",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Watch daemon not yet implemented (Plan 3).")
		},
	}
}
```

- [ ] **Step 2: Register watch command in root.go**

Add `root.AddCommand(newWatchCmd())` to the `NewRootCmd` function, after the existing `AddCommand` calls.

- [ ] **Step 3: Build and verify**

Run: `go build ./cmd/nethelper && echo "test data" > /tmp/test-log.txt && ./nethelper watch ingest /tmp/test-log.txt --db /tmp/test-cli.db`
Expected: `Ingested /tmp/test-log.txt (10 bytes)`

Run: `./nethelper watch start`
Expected: `Watch daemon not yet implemented (Plan 3).`

- [ ] **Step 4: Commit**

```bash
git add internal/cli/watch.go internal/cli/root.go
git commit -m "feat: add watch command with manual ingest and stubs for daemon"
```

---

### Task 12: End-to-End Integration Test

**Files:**
- Create: `internal/cli/cli_test.go`

- [ ] **Step 1: Write integration test**

```go
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCLIEndToEnd(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// Test: version
	root := NewRootCmd()
	root.SetArgs([]string{"version"})
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	if err := root.Execute(); err != nil {
		t.Fatalf("version: %v", err)
	}

	// Test: show device (empty)
	root = NewRootCmd()
	root.SetArgs([]string{"show", "device", "--db", dbPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("show device: %v", err)
	}

	// Test: watch ingest
	logFile := filepath.Join(t.TempDir(), "test.log")
	os.WriteFile(logFile, []byte("<Huawei>display version\nsome output here\n"), 0644)

	root = NewRootCmd()
	root.SetArgs([]string{"watch", "ingest", logFile, "--db", dbPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("watch ingest: %v", err)
	}

	// Test: show route with no data
	root = NewRootCmd()
	root.SetArgs([]string{"show", "route", "--device", "nonexistent", "--db", dbPath})
	err := root.Execute()
	if err == nil {
		t.Log("show route for nonexistent device returned no error (expected)")
	}
}
```

- [ ] **Step 2: Run test — verify PASS**

Run: `go test ./internal/cli/ -v -run TestCLIEndToEnd`
Expected: PASS

- [ ] **Step 3: Run full test suite**

Run: `go test ./... -v`
Expected: all PASS

- [ ] **Step 4: Final build verification**

Run: `go build -o nethelper ./cmd/nethelper && ls -la nethelper`
Expected: produces a single binary

- [ ] **Step 5: Commit**

```bash
git add internal/cli/cli_test.go
git commit -m "feat: add end-to-end CLI integration test"
```

- [ ] **Step 6: Clean up temp binary**

Run: `rm -f nethelper`
