package parser

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/xavierli/nethelper/internal/parser/cisco"
	"github.com/xavierli/nethelper/internal/parser/h3c"
	"github.com/xavierli/nethelper/internal/parser/huawei"
	"github.com/xavierli/nethelper/internal/store"
)

func TestPipelineIngestFile(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	registry := NewRegistry()
	registry.Register(huawei.New())
	pipeline := NewPipeline(db, registry)

	content := "<Core-SW01>display interface brief\n" +
		"Interface                   PHY   Protocol InUti OutUti   inErr  outErr\n" +
		"GE0/0/1                     up    up       0.5%  0.3%         0       0\n" +
		"GE0/0/2                     down  down     0%    0%           0       0\n" +
		"<Core-SW01>display ip routing-table\n" +
		"Route Flags: R - relay\n" +
		"------------------------------------------------------------------------------\n" +
		"Routing Tables: Public\n" +
		"         Destinations : 2        Routes : 2\n\n" +
		"Destination/Mask    Proto   Pre  Cost  Flags NextHop         Interface\n" +
		"192.168.1.0/24         OSPF    10   2           10.0.0.1        GE0/0/1\n" +
		"10.0.0.0/16       Static  60   0     RD    10.0.0.2        GE0/0/2\n" +
		"<Core-SW01>display ospf peer\n" +
		"OSPF Process 1 with Router ID 10.0.0.1\n" +
		"                 Peer Statistic Information\n" +
		" ----------------------------------------------------------------------------\n" +
		" Area Id          Interface                        Neighbor id      State\n" +
		" 0.0.0.0          GE0/0/1                          10.0.0.2         Full\n"

	result, err := pipeline.Ingest("test-log.txt", content)
	if err != nil {
		t.Fatalf("ingest error: %v", err)
	}
	if result.DevicesFound != 1 {
		t.Errorf("devices: %d", result.DevicesFound)
	}
	if result.BlocksParsed != 3 {
		t.Errorf("blocks parsed: %d", result.BlocksParsed)
	}

	dev, err := db.GetDevice("core-sw01")
	if err != nil {
		t.Fatalf("device not found: %v", err)
	}
	if dev.Hostname != "Core-SW01" {
		t.Errorf("hostname: %s", dev.Hostname)
	}
	if dev.Vendor != "huawei" {
		t.Errorf("vendor: %s", dev.Vendor)
	}

	ifaces, _ := db.GetInterfaces("core-sw01")
	if len(ifaces) != 2 {
		t.Errorf("interfaces: %d", len(ifaces))
	}

	snapID, err := db.LatestSnapshotID("core-sw01")
	if err != nil {
		t.Fatalf("no snapshot: %v", err)
	}
	// RIB entries now go to scratch pad (not structured storage)
	scratches, _ := db.ListScratch("core-sw01", "route", 10)
	if len(scratches) != 1 {
		t.Errorf("scratch route entries: %d (expected 1)", len(scratches))
	}

	neighbors, _ := db.GetNeighbors("core-sw01", snapID)
	if len(neighbors) != 1 {
		t.Errorf("neighbors: %d", len(neighbors))
	}
}

func TestPipelineIngest_CapturedAtPassthrough(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	registry := NewRegistry()
	registry.Register(huawei.New())
	pipeline := NewPipeline(db, registry)

	// Log content with timestamp prefixes; the first command has the earliest timestamp.
	content := "2026-03-21-10-00-00: <SW01>display version\nHuawei VRP V200R020\n" +
		"2026-03-21-10-05-00: <SW01>display interface brief\n" +
		"Interface                   PHY   Protocol InUti OutUti   inErr  outErr\n" +
		"GE0/0/1                     up    up       0.5%  0.3%         0       0\n"

	_, err = pipeline.Ingest("ts-log.txt", content)
	if err != nil {
		t.Fatalf("ingest error: %v", err)
	}

	snapID, err := db.LatestSnapshotID("sw01")
	if err != nil {
		t.Fatalf("no snapshot: %v", err)
	}
	snap, err := db.GetSnapshot(snapID)
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}

	wantTime, _ := time.ParseInLocation("2006-01-02-15-04-05", "2026-03-21-10-00-00", time.Local)
	if snap.CapturedAt.IsZero() {
		t.Error("snapshot CapturedAt should not be zero")
	} else if !snap.CapturedAt.Equal(wantTime) {
		t.Errorf("snapshot CapturedAt: got %v, want %v", snap.CapturedAt, wantTime)
	}
}

func TestPipelineIngest_CapturedAtFallback(t *testing.T) {
	// Without timestamps in the log, CapturedAt should be zero (db DEFAULT takes over).
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	registry := NewRegistry()
	registry.Register(huawei.New())
	pipeline := NewPipeline(db, registry)

	before := time.Now().Truncate(time.Second)
	content := "<SW02>display version\nHuawei VRP V200R020\n"
	_, err = pipeline.Ingest("no-ts-log.txt", content)
	if err != nil {
		t.Fatalf("ingest error: %v", err)
	}

	snapID, err := db.LatestSnapshotID("sw02")
	if err != nil {
		t.Fatalf("no snapshot: %v", err)
	}
	snap, err := db.GetSnapshot(snapID)
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}

	// Without an explicit CapturedAt the DB DEFAULT CURRENT_TIMESTAMP takes effect.
	// Verify the timestamp is recent (within a few seconds of ingestion).
	after := time.Now().Add(time.Second)
	if snap.CapturedAt.IsZero() {
		t.Error("snapshot CapturedAt should not be zero (db default should apply)")
	} else if snap.CapturedAt.Before(before) || snap.CapturedAt.After(after) {
		t.Errorf("snapshot CapturedAt %v is not within expected range [%v, %v]", snap.CapturedAt, before, after)
	}
}

// TestPipelineIngest_SkipHelpQueries verifies that command blocks whose command
// ends with '?' (IOS help queries) are counted as skipped and not stored.
func TestPipelineIngest_SkipHelpQueries(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	registry := NewRegistry()
	registry.Register(cisco.New())
	pipeline := NewPipeline(db, registry)

	// Two help queries + one real command.
	content := "Router-PE01#show running-config ?\n" +
		"  <cr>   show full config\n" +
		"Router-PE01#show running-config | ?\n" +
		"  include  Include lines that match\n" +
		"Router-PE01#show interfaces GigabitEthernet0/0\n" +
		"GigabitEthernet0/0 is up, line protocol is up\n"

	result, err := pipeline.Ingest("cisco-log.txt", content)
	if err != nil {
		t.Fatalf("ingest error: %v", err)
	}
	if result.BlocksSkipped != 2 {
		t.Errorf("BlocksSkipped: got %d, want 2", result.BlocksSkipped)
	}
	if result.BlocksParsed != 1 {
		t.Errorf("BlocksParsed: got %d, want 1", result.BlocksParsed)
	}
}

// TestPipelineIngest_IOSXRPrompt verifies that IOS-XR style prompts are correctly
// recognised by the Cisco parser and produce a stored device.
func TestPipelineIngest_IOSXRPrompt(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	registry := NewRegistry()
	registry.Register(cisco.New())
	pipeline := NewPipeline(db, registry)

	content := "RP/0/RP0/CPU0:GZ-YS-0101-ASR9912-01#show interfaces brief\n" +
		"Interface                     Intf        IP-Address      Status          Protocol\n" +
		"GigabitEthernet0/0/0/0        up          10.0.0.1/24     Up              Up\n"

	result, err := pipeline.Ingest("xr-log.txt", content)
	if err != nil {
		t.Fatalf("ingest error: %v", err)
	}
	if result.DevicesFound != 1 {
		t.Errorf("DevicesFound: got %d, want 1", result.DevicesFound)
	}

	dev, err := db.GetDevice("gz-ys-0101-asr9912-01")
	if err != nil {
		t.Fatalf("device not found: %v", err)
	}
	if dev.Vendor != "cisco" {
		t.Errorf("vendor: got %s, want cisco", dev.Vendor)
	}
	if dev.Hostname != "GZ-YS-0101-ASR9912-01" {
		t.Errorf("hostname: got %s, want GZ-YS-0101-ASR9912-01", dev.Hostname)
	}
}

// TestReclassifyH3C verifies that blocks initially assigned vendor="huawei" are
// flipped to "h3c" when a Comware-7 signature appears in their config output.
func TestReclassifyH3C(t *testing.T) {
	registry := NewRegistry()
	registry.Register(huawei.New())
	registry.Register(h3c.New())

	// Construct blocks as Split would produce them: vendor=huawei (first match),
	// but the config output contains the H3C "version 7." signature.
	blocks := []CommandBlock{
		{
			Hostname: "H3C-Core",
			Vendor:   "huawei",
			Command:  "display current-configuration",
			Output:   "version 7.1.070\n#\nsysname H3C-Core\n",
		},
		{
			Hostname: "H3C-Core",
			Vendor:   "huawei",
			Command:  "display interface brief",
			Output:   "Brief information on interfaces in route mode:\n",
		},
	}

	reclassifyH3C(blocks, registry)

	for _, b := range blocks {
		if b.Vendor != "h3c" {
			t.Errorf("block %q: expected vendor=h3c, got %s", b.Command, b.Vendor)
		}
	}
}

// TestReclassifyH3C_MDCMarker verifies reclassification also fires on "mdc admin id".
func TestReclassifyH3C_MDCMarker(t *testing.T) {
	registry := NewRegistry()
	registry.Register(huawei.New())
	registry.Register(h3c.New())

	blocks := []CommandBlock{
		{
			Hostname: "H3C-MDC",
			Vendor:   "huawei",
			Command:  "display current-configuration",
			Output:   "#\nmdc admin id 1\nsysname H3C-MDC\n",
		},
	}

	reclassifyH3C(blocks, registry)

	if blocks[0].Vendor != "h3c" {
		t.Errorf("expected vendor=h3c after MDC marker, got %s", blocks[0].Vendor)
	}
}

// TestReclassifyH3C_NoChange verifies that genuine Huawei blocks are not touched.
func TestReclassifyH3C_NoChange(t *testing.T) {
	registry := NewRegistry()
	registry.Register(huawei.New())
	registry.Register(h3c.New())

	blocks := []CommandBlock{
		{
			Hostname: "Huawei-CE",
			Vendor:   "huawei",
			Command:  "display current-configuration",
			Output:   "version V200R021C10SPC600\nsysname Huawei-CE\n",
		},
	}

	reclassifyH3C(blocks, registry)

	if blocks[0].Vendor != "huawei" {
		t.Errorf("expected vendor=huawei (unchanged), got %s", blocks[0].Vendor)
	}
}

// TestPipelineIngest_H3CDisambiguation is a full pipeline integration test that
// ingests H3C content (with a "version 7." header) and verifies the stored
// device has vendor="h3c", not "huawei".
func TestPipelineIngest_H3CDisambiguation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// Register huawei FIRST (same order as production root.go); h3c second.
	registry := NewRegistry()
	registry.Register(huawei.New())
	registry.Register(h3c.New())
	pipeline := NewPipeline(db, registry)

	// The log uses <hostname> prompts, so Split will assign vendor=huawei.
	// The config block contains "version 7." which must trigger reclassification.
	content := "<H3C-Core01>display current-configuration\n" +
		"version 7.1.070\n" +
		"#\n" +
		"sysname H3C-Core01\n" +
		"<H3C-Core01>display interface brief\n" +
		"Brief information on interfaces in route mode:\n" +
		"Link: ADM - administratively down; Stby - standby\n" +
		"Protocol: (s) - spoofing\n" +
		"Interface            Link Protocol Primary IP      Description\n" +
		"GE1/0/1              UP   UP       10.0.0.1\n"

	result, err := pipeline.Ingest("h3c-log.txt", content)
	if err != nil {
		t.Fatalf("ingest error: %v", err)
	}
	if result.DevicesFound != 1 {
		t.Errorf("DevicesFound: got %d, want 1", result.DevicesFound)
	}

	dev, err := db.GetDevice("h3c-core01")
	if err != nil {
		t.Fatalf("device not found: %v", err)
	}
	if dev.Vendor != "h3c" {
		t.Errorf("vendor: got %q, want h3c — reclassification did not fire", dev.Vendor)
	}
	if dev.Hostname != "H3C-Core01" {
		t.Errorf("hostname: got %q, want H3C-Core01", dev.Hostname)
	}

	// The interface block should have been parsed with the H3C parser and stored.
	ifaces, _ := db.GetInterfaces("h3c-core01")
	if len(ifaces) != 1 {
		t.Errorf("interfaces: got %d, want 1", len(ifaces))
	}
}
