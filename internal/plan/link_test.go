package plan_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/plan"
	"github.com/xavierli/nethelper/internal/store"
)

func openTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestDiscoverLinks_DescriptionMatching inserts two devices and gives one
// interface a description containing the peer's hostname. The engine should
// discover a link tagged with source "description".
func TestDiscoverLinks_DescriptionMatching(t *testing.T) {
	db := openTestDB(t)

	devA := model.Device{ID: "device-a", Hostname: "RouterA", Vendor: "huawei", LastSeen: time.Now()}
	devB := model.Device{ID: "device-b", Hostname: "RouterB", Vendor: "huawei", LastSeen: time.Now()}
	mustUpsertDevice(t, db, devA)
	mustUpsertDevice(t, db, devB)

	// Interface on device-a whose description mentions the peer hostname.
	ifaceA := model.Interface{
		ID:          "iface-a1",
		DeviceID:    "device-a",
		Name:        "GigabitEthernet0/0/0",
		Type:        model.IfTypePhysical,
		Status:      "up",
		IPAddress:   "192.168.1.1",
		Mask:        "30",
		Description: "uplink to RouterB GE0/0/1",
		LastUpdated: time.Now(),
	}
	if err := db.UpsertInterface(ifaceA); err != nil {
		t.Fatalf("upsert interface: %v", err)
	}

	links, err := plan.DiscoverLinks(db, "device-a")
	if err != nil {
		t.Fatalf("DiscoverLinks: %v", err)
	}

	found := findLink(links, "device-a", "device-b")
	if found == nil {
		t.Fatalf("expected link to device-b, got %v", links)
	}
	if !containsStr(found.Sources, "description") {
		t.Errorf("expected source 'description', got %v", found.Sources)
	}
}

// TestDiscoverLinks_SubnetMatching inserts two devices with interfaces on the
// same /30 subnet. The graph engine should discover a link tagged "subnet".
func TestDiscoverLinks_SubnetMatching(t *testing.T) {
	db := openTestDB(t)

	devA := model.Device{ID: "node-a", Hostname: "NodeA", Vendor: "huawei", LastSeen: time.Now()}
	devB := model.Device{ID: "node-b", Hostname: "NodeB", Vendor: "huawei", LastSeen: time.Now()}
	mustUpsertDevice(t, db, devA)
	mustUpsertDevice(t, db, devB)

	// Both interfaces on the same /30 — 10.1.1.0/30
	ifaceA := model.Interface{
		ID:          "na-eth0",
		DeviceID:    "node-a",
		Name:        "GigabitEthernet0/0/0",
		Type:        model.IfTypePhysical,
		Status:      "up",
		IPAddress:   "10.1.1.1",
		Mask:        "30",
		LastUpdated: time.Now(),
	}
	ifaceB := model.Interface{
		ID:          "nb-eth0",
		DeviceID:    "node-b",
		Name:        "GigabitEthernet0/0/0",
		Type:        model.IfTypePhysical,
		Status:      "up",
		IPAddress:   "10.1.1.2",
		Mask:        "30",
		LastUpdated: time.Now(),
	}
	if err := db.UpsertInterface(ifaceA); err != nil {
		t.Fatalf("upsert ifaceA: %v", err)
	}
	if err := db.UpsertInterface(ifaceB); err != nil {
		t.Fatalf("upsert ifaceB: %v", err)
	}

	links, err := plan.DiscoverLinks(db, "node-a")
	if err != nil {
		t.Fatalf("DiscoverLinks: %v", err)
	}

	found := findLink(links, "node-a", "node-b")
	if found == nil {
		t.Fatalf("expected link to node-b, got %v", links)
	}
	if !containsStr(found.Sources, "subnet") {
		t.Errorf("expected source 'subnet', got %v", found.Sources)
	}
}

// TestDiscoverLinks_ProtocolEnrichment verifies that protocols detected in the
// config snapshot are added to all discovered links.
func TestDiscoverLinks_ProtocolEnrichment(t *testing.T) {
	db := openTestDB(t)

	devA := model.Device{ID: "pe-a", Hostname: "PE-A", Vendor: "huawei", LastSeen: time.Now()}
	devB := model.Device{ID: "pe-b", Hostname: "PE-B", Vendor: "huawei", LastSeen: time.Now()}
	mustUpsertDevice(t, db, devA)
	mustUpsertDevice(t, db, devB)

	// Create a link via description matching so there is at least one link.
	iface := model.Interface{
		ID:          "pea-eth0",
		DeviceID:    "pe-a",
		Name:        "GigabitEthernet0/0/0",
		Type:        model.IfTypePhysical,
		Status:      "up",
		IPAddress:   "10.2.2.1",
		Mask:        "30",
		Description: "link to PE-B",
		LastUpdated: time.Now(),
	}
	if err := db.UpsertInterface(iface); err != nil {
		t.Fatalf("upsert interface: %v", err)
	}

	// Insert a config snapshot that mentions ospf and bgp.
	cs := model.ConfigSnapshot{
		DeviceID:   "pe-a",
		ConfigText: "ospf 1\n router-id 1.1.1.1\nbgp 65001\n peer 10.2.2.2\nmpls ldp\n",
		Format:     "hierarchical",
	}
	if _, err := db.InsertConfigSnapshot(cs); err != nil {
		t.Fatalf("insert config snapshot: %v", err)
	}

	links, err := plan.DiscoverLinks(db, "pe-a")
	if err != nil {
		t.Fatalf("DiscoverLinks: %v", err)
	}
	if len(links) == 0 {
		t.Fatal("expected at least one link")
	}

	for _, l := range links {
		if !containsStr(l.Protocols, "ospf") {
			t.Errorf("link %s→%s: expected 'ospf' in protocols, got %v", l.LocalInterface, l.PeerDevice, l.Protocols)
		}
		if !containsStr(l.Protocols, "bgp") {
			t.Errorf("link %s→%s: expected 'bgp' in protocols, got %v", l.LocalInterface, l.PeerDevice, l.Protocols)
		}
		if !containsStr(l.Protocols, "ldp") {
			t.Errorf("link %s→%s: expected 'ldp' in protocols, got %v", l.LocalInterface, l.PeerDevice, l.Protocols)
		}
	}
}

// TestContainsStr is a simple unit test for the containsStr helper, exercised
// indirectly via Sources/Protocols deduplication.
func TestContainsStr(t *testing.T) {
	db := openTestDB(t)

	devA := model.Device{ID: "dup-a", Hostname: "DupA", Vendor: "huawei", LastSeen: time.Now()}
	devB := model.Device{ID: "dup-b", Hostname: "DupB", Vendor: "huawei", LastSeen: time.Now()}
	mustUpsertDevice(t, db, devA)
	mustUpsertDevice(t, db, devB)

	// Both strategies will fire: same /30 subnet AND description contains hostname.
	iface := model.Interface{
		ID:          "dupa-eth0",
		DeviceID:    "dup-a",
		Name:        "GigabitEthernet0/0/0",
		Type:        model.IfTypePhysical,
		Status:      "up",
		IPAddress:   "10.3.3.1",
		Mask:        "30",
		Description: "to DupB ge0/0/0",
		LastUpdated: time.Now(),
	}
	ifaceB := model.Interface{
		ID:          "dupb-eth0",
		DeviceID:    "dup-b",
		Name:        "GigabitEthernet0/0/0",
		Type:        model.IfTypePhysical,
		Status:      "up",
		IPAddress:   "10.3.3.2",
		Mask:        "30",
		LastUpdated: time.Now(),
	}
	if err := db.UpsertInterface(iface); err != nil {
		t.Fatalf("upsert iface: %v", err)
	}
	if err := db.UpsertInterface(ifaceB); err != nil {
		t.Fatalf("upsert ifaceB: %v", err)
	}

	links, err := plan.DiscoverLinks(db, "dup-a")
	if err != nil {
		t.Fatalf("DiscoverLinks: %v", err)
	}

	found := findLink(links, "dup-a", "dup-b")
	if found == nil {
		t.Fatalf("no link found; got %v", links)
	}

	// Even though two strategies fire, each source should appear exactly once.
	seen := make(map[string]int)
	for _, s := range found.Sources {
		seen[s]++
	}
	for src, count := range seen {
		if count > 1 {
			t.Errorf("source %q duplicated %d times in Sources", src, count)
		}
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func mustUpsertDevice(t *testing.T, db *store.DB, d model.Device) {
	t.Helper()
	if err := db.UpsertDevice(d); err != nil {
		t.Fatalf("UpsertDevice %s: %v", d.ID, err)
	}
}

func findLink(links []plan.Link, localDevice, peerDevice string) *plan.Link {
	for i := range links {
		if links[i].LocalDevice == localDevice && links[i].PeerDevice == peerDevice {
			return &links[i]
		}
	}
	return nil
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
