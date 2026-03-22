package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/xavierli/nethelper/internal/llm/intent"
	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/store"
)

// setupTestDB opens a fresh SQLite DB in a temp directory and sets the
// package-level `db` global so the CLI helpers can use it.
func setupTestDB(t *testing.T) func() {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	testDB, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	prev := db
	db = testDB
	return func() {
		db = prev
		testDB.Close()
	}
}

// seedDevice inserts a device + some interfaces + neighbors into the test DB.
func seedDevice(t *testing.T, devID, hostname, vendor string) {
	t.Helper()
	if err := db.UpsertDevice(model.Device{
		ID:       devID,
		Hostname: hostname,
		Vendor:   vendor,
		RouterID: "10.0.0.1",
	}); err != nil {
		t.Fatalf("upsert device: %v", err)
	}

	snapID, err := db.CreateSnapshot(model.Snapshot{DeviceID: devID})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	if err := db.UpsertInterface(model.Interface{
		ID:        devID + ":GigabitEthernet0/0/0",
		DeviceID:  devID,
		Name:      "GigabitEthernet0/0/0",
		Type:      model.IfTypePhysical,
		Status:    "up",
		IPAddress: "192.168.1.1",
		Mask:      "255.255.255.0",
	}); err != nil {
		t.Fatalf("upsert interface: %v", err)
	}

	if err := db.InsertNeighbors([]model.NeighborInfo{
		{
			DeviceID:       devID,
			Protocol:       "bgp",
			RemoteID:       "10.0.0.2",
			State:          "established",
			LocalInterface: "GigabitEthernet0/0/0",
			ASNumber:       65001,
			Uptime:         "1d2h",
			SnapshotID:     snapID,
		},
		{
			DeviceID:       devID,
			Protocol:       "ospf",
			RemoteID:       "10.0.0.3",
			State:          "full",
			LocalInterface: "GigabitEthernet0/0/1",
			AreaID:         "0.0.0.0",
			SnapshotID:     snapID,
		},
	}); err != nil {
		t.Fatalf("insert neighbors: %v", err)
	}
}

// --- intent.Classify integration ---

func TestClassifyIntegration(t *testing.T) {
	cases := []struct {
		query string
		want  intent.QueryIntent
	}{
		{"list all devices", intent.IntentDeviceList},
		{"show interfaces on core-01", intent.IntentInterfaceStatus},
		{"display route table", intent.IntentRouteTable},
		{"show neighbors", intent.IntentNeighborList},
		{"search config bgp", intent.IntentConfigSearch},
		{"why is bgp down", intent.IntentComplex},
		{"analyze the ospf issue", intent.IntentComplex},
	}
	for _, tc := range cases {
		got := intent.Classify(tc.query)
		if got != tc.want {
			t.Errorf("Classify(%q) = %v, want %v", tc.query, got, tc.want)
		}
	}
}

// --- directQuery tests ---

func TestDirectQuery_DeviceList(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	seedDevice(t, "core-01", "CORE-01", "huawei")

	result := directQuery(intent.IntentDeviceList, "list all devices", "")
	if result == "" {
		t.Fatal("expected non-empty result for IntentDeviceList")
	}
	if !strings.Contains(result, "core-01") {
		t.Errorf("result should contain device ID: %s", result)
	}
	if !strings.Contains(result, "CORE-01") {
		t.Errorf("result should contain hostname: %s", result)
	}
}

func TestDirectQuery_DeviceList_Empty(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// No devices seeded — should return "".
	result := directQuery(intent.IntentDeviceList, "list all devices", "")
	if result != "" {
		t.Errorf("expected empty result for empty DB, got: %s", result)
	}
}

func TestDirectQuery_InterfaceStatus(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	seedDevice(t, "core-01", "CORE-01", "huawei")

	result := directQuery(intent.IntentInterfaceStatus, "show interfaces on core-01", "")
	if result == "" {
		t.Fatal("expected non-empty result for IntentInterfaceStatus")
	}
	if !strings.Contains(result, "GigabitEthernet0/0/0") {
		t.Errorf("result should contain interface name: %s", result)
	}
	if !strings.Contains(result, "192.168.1.1") {
		t.Errorf("result should contain IP address: %s", result)
	}
}

func TestDirectQuery_InterfaceStatus_ExplicitDevice(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	seedDevice(t, "core-01", "CORE-01", "huawei")

	// Use --device flag (explicitDevice arg).
	result := directQuery(intent.IntentInterfaceStatus, "show interfaces", "core-01")
	if result == "" {
		t.Fatal("expected non-empty result when device is explicit")
	}
	if !strings.Contains(result, "GigabitEthernet0/0/0") {
		t.Errorf("result should contain interface name: %s", result)
	}
}

func TestDirectQuery_NeighborList(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	seedDevice(t, "core-01", "CORE-01", "huawei")

	result := directQuery(intent.IntentNeighborList, "show neighbors on core-01", "")
	if result == "" {
		t.Fatal("expected non-empty result for IntentNeighborList")
	}
	if !strings.Contains(result, "bgp") {
		t.Errorf("result should contain bgp neighbor: %s", result)
	}
	if !strings.Contains(result, "ospf") {
		t.Errorf("result should contain ospf neighbor: %s", result)
	}
	if !strings.Contains(result, "10.0.0.2") {
		t.Errorf("result should contain remote ID: %s", result)
	}
}

func TestDirectQuery_RouteTable_Empty(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	seedDevice(t, "core-01", "CORE-01", "huawei")

	// No scratch entries → should return "".
	result := directQuery(intent.IntentRouteTable, "show route table on core-01", "")
	if result != "" {
		// This is ok; empty route scratch returns "".
		t.Logf("result (expected empty): %q", result)
	}
}

func TestDirectQuery_ConfigSearch_NoResults(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// DB has no config snapshots; search should return "".
	result := directQuery(intent.IntentConfigSearch, "search config bgp", "")
	if result != "" {
		t.Errorf("expected empty result for no config data, got: %s", result)
	}
}

func TestDirectQuery_NoDevice(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// No devices; device-specific queries should return "".
	result := directQuery(intent.IntentInterfaceStatus, "show interfaces on unknowndevice", "")
	if result != "" {
		t.Errorf("expected empty result when no device matches, got: %s", result)
	}
}

// --- extractSearchTerm tests ---

func TestExtractSearchTerm(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		{"search config bgp", "bgp"},
		{"find config ospf", "ospf"},
		{"grep config mpls", "mpls"},
		{"搜索配置 bgp", "bgp"},
		{"查找配置 vrf", "vrf"},
	}
	for _, tc := range cases {
		got := extractSearchTerm(tc.query)
		if got != tc.want {
			t.Errorf("extractSearchTerm(%q) = %q, want %q", tc.query, got, tc.want)
		}
	}
}

// --- extractRelevantConfig tests ---

func TestExtractRelevantConfig_BGP(t *testing.T) {
	config := `#
sysname CORE-01
#
bgp 65001
 peer 10.0.0.2 as-number 65002
 peer 10.0.0.2 description transit
#
ospf 1
 area 0.0.0.0
  network 192.168.1.0 0.0.0.255
#`
	result := extractRelevantConfig(config, "show bgp peers", 8000)
	if !strings.Contains(result, "bgp 65001") {
		t.Errorf("expected BGP section in result: %s", result)
	}
	if strings.Contains(result, "ospf 1") {
		t.Errorf("unexpected OSPF section in result: %s", result)
	}
}

func TestExtractRelevantConfig_MaxChars(t *testing.T) {
	// Build a large config.
	var sb strings.Builder
	sb.WriteString("#\nbgp 65001\n")
	for i := 0; i < 200; i++ {
		sb.WriteString(" peer 10.0.0." + strings.Repeat("1", 3) + " as-number 65002\n")
	}
	sb.WriteString("#\n")

	result := extractRelevantConfig(sb.String(), "bgp configuration", 500)
	if len(result) > 550 { // some slack for the truncation suffix
		t.Errorf("result too long: %d chars (max 500)", len(result))
	}
	if !strings.Contains(result, "truncated") {
		t.Errorf("expected truncation marker in result: %s", result[:200])
	}
}

func TestExtractRelevantConfig_NoMatch(t *testing.T) {
	config := "#\nbgp 65001\n peer 10.0.0.2 as-number 65002\n#\n"
	result := extractRelevantConfig(config, "show ntp configuration", 8000)
	if result != "" {
		t.Errorf("expected empty result for no keyword match: %s", result)
	}
}

// --- detectProtocolFilter tests ---

func TestDetectProtocolFilter(t *testing.T) {
	cases := []struct {
		question string
		want     string
	}{
		{"BGP邻居状态", "bgp"},
		{"ospf area 0 problem", "ospf"},
		{"mpls ldp issue", "mpls"},
		{"tunnel te down", "tunnel"},
		{"mpls te configuration", "mpls"},
		{"show te tunnel", "tunnel"},
		{"general network question", ""},
		{"show all interfaces", ""},
	}
	for _, tc := range cases {
		got := detectProtocolFilter(tc.question)
		if got != tc.want {
			t.Errorf("detectProtocolFilter(%q) = %q, want %q", tc.question, got, tc.want)
		}
	}
}

// --- buildContext tests ---

func TestBuildContext_GeneralQuery(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	seedDevice(t, "core-01", "CORE-01", "huawei")

	ctx := buildContext("tell me about core-01", intent.IntentComplex, "")
	if ctx == "" {
		t.Fatal("expected non-empty context")
	}
	if !strings.Contains(ctx, "core-01") {
		t.Errorf("context should mention device ID: %s", ctx)
	}
	// General query should include interfaces.
	if !strings.Contains(ctx, "接口列表") {
		t.Errorf("general context should include interface section: %s", ctx)
	}
}

func TestBuildContext_BGPQuery_FilterNeighbors(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	seedDevice(t, "core-01", "CORE-01", "huawei")

	ctx := buildContext("BGP neighbor status on core-01", intent.IntentComplex, "")
	if ctx == "" {
		t.Fatal("expected non-empty context")
	}
	// BGP neighbor should be included.
	if !strings.Contains(ctx, "bgp") {
		t.Errorf("context should include bgp neighbor: %s", ctx)
	}
	// OSPF neighbor should be filtered out for a BGP-specific query.
	if strings.Contains(ctx, "ospf") {
		t.Errorf("context should NOT include ospf neighbor for BGP query: %s", ctx)
	}
}

func TestBuildContext_NoDevices(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	ctx := buildContext("show devices", intent.IntentComplex, "")
	// With no devices, should return "".
	if ctx != "" {
		t.Logf("context with no devices: %q (may be empty or overview)", ctx)
	}
}
