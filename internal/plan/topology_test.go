package plan_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/plan"
	"github.com/xavierli/nethelper/internal/store"
)

func setupTestDB(t *testing.T) *store.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// insertDevice inserts a device and returns its ID.
func insertDevice(t *testing.T, db *store.DB, id, hostname, vendor string) model.Device {
	t.Helper()
	dev := model.Device{
		ID:       id,
		Hostname: hostname,
		Vendor:   vendor,
		LastSeen: time.Now(),
	}
	if err := db.UpsertDevice(dev); err != nil {
		t.Fatalf("upsert device: %v", err)
	}
	return dev
}

// insertSnapshot inserts a snapshot and returns its ID.
func insertSnapshot(t *testing.T, db *store.DB, deviceID string) int {
	t.Helper()
	snapID, err := db.CreateSnapshot(model.Snapshot{
		DeviceID:   deviceID,
		SourceFile: "test.log",
		CapturedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	return snapID
}

// TestBuildTopology_BGPPeerGroups verifies that BGP peers are correctly grouped
// into PeerGroups with correct counts and LocalAS.
func TestBuildTopology_BGPPeerGroups(t *testing.T) {
	db := setupTestDB(t)

	insertDevice(t, db, "pe1", "PE1", "huawei")
	snapID := insertSnapshot(t, db, "pe1")

	peers := []model.BGPPeer{
		// Group "rr-clients" — 2 peers, internal
		{DeviceID: "pe1", PeerGroup: "rr-clients", LocalAS: 65000, PeerIP: "10.0.0.1", RemoteAS: 65000, SnapshotID: snapID},
		{DeviceID: "pe1", PeerGroup: "rr-clients", LocalAS: 65000, PeerIP: "10.0.0.2", RemoteAS: 65000, SnapshotID: snapID},
		// Group "upstream" — 1 peer, external
		{DeviceID: "pe1", PeerGroup: "upstream", LocalAS: 65000, PeerIP: "192.168.1.1", RemoteAS: 65100, SnapshotID: snapID},
		// Group "customer" — 1 peer, external
		{DeviceID: "pe1", PeerGroup: "customer", LocalAS: 65000, PeerIP: "172.16.0.1", RemoteAS: 64512, SnapshotID: snapID},
	}
	if err := db.InsertBGPPeers(peers); err != nil {
		t.Fatalf("insert peers: %v", err)
	}

	topo, err := plan.BuildTopology(db, "pe1")
	if err != nil {
		t.Fatalf("BuildTopology: %v", err)
	}

	if len(topo.PeerGroups) != 3 {
		t.Errorf("want 3 PeerGroups, got %d", len(topo.PeerGroups))
	}
	if topo.LocalAS != 65000 {
		t.Errorf("want LocalAS=65000, got %d", topo.LocalAS)
	}

	// Find rr-clients group
	var rrGroup *plan.PeerGroup
	for i := range topo.PeerGroups {
		if topo.PeerGroups[i].Name == "rr-clients" {
			rrGroup = &topo.PeerGroups[i]
		}
	}
	if rrGroup == nil {
		t.Fatal("rr-clients PeerGroup not found")
	}
	if len(rrGroup.Peers) != 2 {
		t.Errorf("want 2 peers in rr-clients, got %d", len(rrGroup.Peers))
	}
	if rrGroup.Type != "internal" {
		t.Errorf("want rr-clients type=internal, got %q", rrGroup.Type)
	}
	if rrGroup.LocalAS != 65000 {
		t.Errorf("want rr-clients LocalAS=65000, got %d", rrGroup.LocalAS)
	}

	// Verify upstream is external
	var upGroup *plan.PeerGroup
	for i := range topo.PeerGroups {
		if topo.PeerGroups[i].Name == "upstream" {
			upGroup = &topo.PeerGroups[i]
		}
	}
	if upGroup == nil {
		t.Fatal("upstream PeerGroup not found")
	}
	if upGroup.Type != "external" {
		t.Errorf("want upstream type=external, got %q", upGroup.Type)
	}
}

// TestBuildTopology_PeerInterfaceMapping verifies that a BGP peer IP on the same /30
// as a local interface gets its Interface field set.
func TestBuildTopology_PeerInterfaceMapping(t *testing.T) {
	db := setupTestDB(t)

	insertDevice(t, db, "pe2", "PE2", "huawei")
	snapID := insertSnapshot(t, db, "pe2")

	// Local interface: 10.0.0.1/30, subnet 10.0.0.0/30
	iface := model.Interface{
		ID:          "pe2:GE0/0/0",
		DeviceID:    "pe2",
		Name:        "GigabitEthernet0/0/0",
		Type:        model.IfTypePhysical,
		Status:      "up",
		IPAddress:   "10.0.0.1",
		Mask:        "30",
		LastUpdated: time.Now(),
	}
	if err := db.UpsertInterface(iface); err != nil {
		t.Fatalf("upsert interface: %v", err)
	}

	// BGP peer on same /30: 10.0.0.2
	peers := []model.BGPPeer{
		{DeviceID: "pe2", PeerGroup: "upstream", LocalAS: 65000, PeerIP: "10.0.0.2", RemoteAS: 65100, SnapshotID: snapID},
	}
	if err := db.InsertBGPPeers(peers); err != nil {
		t.Fatalf("insert peers: %v", err)
	}

	topo, err := plan.BuildTopology(db, "pe2")
	if err != nil {
		t.Fatalf("BuildTopology: %v", err)
	}

	if len(topo.PeerGroups) == 0 {
		t.Fatal("expected at least one PeerGroup")
	}
	found := false
	for _, pg := range topo.PeerGroups {
		for _, p := range pg.Peers {
			if p.PeerIP == "10.0.0.2" {
				if p.Interface != "GigabitEthernet0/0/0" {
					t.Errorf("want Interface=GigabitEthernet0/0/0, got %q", p.Interface)
				}
				found = true
			}
		}
	}
	if !found {
		t.Error("peer 10.0.0.2 not found in any PeerGroup")
	}
}

// TestBuildTopology_LAGDetection verifies that an eth-trunk with 2 member interfaces
// is represented as a LAGBundle with the correct members.
func TestBuildTopology_LAGDetection(t *testing.T) {
	db := setupTestDB(t)

	insertDevice(t, db, "pe3", "PE3", "huawei")

	// The LAG (eth-trunk) interface
	trunk := model.Interface{
		ID:          "pe3:Eth-Trunk10",
		DeviceID:    "pe3",
		Name:        "Eth-Trunk10",
		Type:        model.IfTypeEthTrunk,
		Status:      "up",
		IPAddress:   "10.1.1.1",
		Mask:        "30",
		Description: "Uplink to core",
		LastUpdated: time.Now(),
	}
	if err := db.UpsertInterface(trunk); err != nil {
		t.Fatalf("upsert trunk: %v", err)
	}

	// Two physical member interfaces with ParentID pointing to the trunk
	members := []model.Interface{
		{
			ID:          "pe3:GE0/0/1",
			DeviceID:    "pe3",
			Name:        "GigabitEthernet0/0/1",
			Type:        model.IfTypePhysical,
			Status:      "up",
			ParentID:    "pe3:Eth-Trunk10",
			LastUpdated: time.Now(),
		},
		{
			ID:          "pe3:GE0/0/2",
			DeviceID:    "pe3",
			Name:        "GigabitEthernet0/0/2",
			Type:        model.IfTypePhysical,
			Status:      "up",
			ParentID:    "pe3:Eth-Trunk10",
			LastUpdated: time.Now(),
		},
	}
	for _, m := range members {
		if err := db.UpsertInterface(m); err != nil {
			t.Fatalf("upsert member: %v", err)
		}
	}

	topo, err := plan.BuildTopology(db, "pe3")
	if err != nil {
		t.Fatalf("BuildTopology: %v", err)
	}

	if len(topo.LAGs) != 1 {
		t.Fatalf("want 1 LAG, got %d", len(topo.LAGs))
	}
	lag := topo.LAGs[0]
	if lag.Name != "Eth-Trunk10" {
		t.Errorf("want LAG name=Eth-Trunk10, got %q", lag.Name)
	}
	if lag.IP != "10.1.1.1" {
		t.Errorf("want LAG IP=10.1.1.1, got %q", lag.IP)
	}
	if len(lag.Members) != 2 {
		t.Errorf("want 2 LAG members, got %d: %v", len(lag.Members), lag.Members)
	}

	// Members should not appear in PhysicalLinks (they have ParentID set)
	for _, pl := range topo.PhysicalLinks {
		if pl.Interface == "GigabitEthernet0/0/1" || pl.Interface == "GigabitEthernet0/0/2" {
			t.Errorf("trunk member %q should not appear in PhysicalLinks", pl.Interface)
		}
	}
}

// TestBuildTopology_ProtocolDetection verifies that protocols are detected from
// both config text keywords and from the presence of BGP peers.
func TestBuildTopology_ProtocolDetection(t *testing.T) {
	db := setupTestDB(t)

	insertDevice(t, db, "pe4", "PE4", "huawei")
	snapID := insertSnapshot(t, db, "pe4")

	// Insert a BGP peer (triggers "bgp" protocol detection)
	peers := []model.BGPPeer{
		{DeviceID: "pe4", PeerGroup: "rr", LocalAS: 65000, PeerIP: "1.1.1.1", RemoteAS: 65000, SnapshotID: snapID},
	}
	if err := db.InsertBGPPeers(peers); err != nil {
		t.Fatalf("insert peers: %v", err)
	}

	// Insert a config snapshot containing "isis" keyword
	_, err := db.InsertConfigSnapshot(model.ConfigSnapshot{
		DeviceID:   "pe4",
		ConfigText: "# IS-IS config\nisis 1\n is-name PE4\n network-entity 49.0001.0000.0000.0004.00\ninterface GE0/0/0\n isis enable 1\n",
		Format:     "hierarchical",
		CapturedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("insert config: %v", err)
	}

	topo, err := plan.BuildTopology(db, "pe4")
	if err != nil {
		t.Fatalf("BuildTopology: %v", err)
	}

	protoSet := make(map[string]bool)
	for _, p := range topo.Protocols {
		protoSet[p] = true
	}

	if !protoSet["bgp"] {
		t.Error("expected 'bgp' in Protocols")
	}
	if !protoSet["isis"] {
		t.Error("expected 'isis' in Protocols")
	}
}

// TestBuildTopology_NoBGP verifies that a device with no snapshots returns
// LocalAS=0, empty PeerGroups, and no error.
func TestBuildTopology_NoBGP(t *testing.T) {
	db := setupTestDB(t)

	insertDevice(t, db, "pe5", "PE5", "cisco")

	topo, err := plan.BuildTopology(db, "pe5")
	if err != nil {
		t.Fatalf("BuildTopology: %v", err)
	}

	if topo.LocalAS != 0 {
		t.Errorf("want LocalAS=0, got %d", topo.LocalAS)
	}
	if len(topo.PeerGroups) != 0 {
		t.Errorf("want empty PeerGroups, got %v", topo.PeerGroups)
	}
	if topo.DeviceID != "pe5" {
		t.Errorf("want DeviceID=pe5, got %q", topo.DeviceID)
	}
	if topo.Hostname != "PE5" {
		t.Errorf("want Hostname=PE5, got %q", topo.Hostname)
	}
	if topo.Vendor != "cisco" {
		t.Errorf("want Vendor=cisco, got %q", topo.Vendor)
	}
}

// TestBuildTopology_IGPDetection verifies that ISIS processes and LDP are
// extracted from a Huawei-style # delimited config.
func TestBuildTopology_IGPDetection(t *testing.T) {
	db := setupTestDB(t)
	insertDevice(t, db, "pe6", "PE6", "huawei")

	configText := `#
isis 1
 is-level level-2
 network-entity 49.0001.0000.0000.0006.00
#
interface Eth-Trunk1
 ip address 10.0.0.1 255.255.255.252
 isis enable 1
 isis circuit-type p2p
 mpls ldp
#
interface Eth-Trunk2
 ip address 10.0.0.5 255.255.255.252
 isis enable 1
 mpls ldp
#
interface LoopBack0
 ip address 1.1.1.6 255.255.255.255
 isis enable 1
#
`
	_, err := db.InsertConfigSnapshot(model.ConfigSnapshot{
		DeviceID:   "pe6",
		ConfigText: configText,
		Format:     "hierarchical",
		CapturedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("insert config: %v", err)
	}

	topo, err := plan.BuildTopology(db, "pe6")
	if err != nil {
		t.Fatalf("BuildTopology: %v", err)
	}

	// Should have exactly 1 IGP entry: isis process 1
	if len(topo.IGPs) != 1 {
		t.Fatalf("want 1 IGP entry, got %d: %+v", len(topo.IGPs), topo.IGPs)
	}
	igp := topo.IGPs[0]
	if igp.Protocol != "isis" {
		t.Errorf("want Protocol=isis, got %q", igp.Protocol)
	}
	if igp.ProcessID != "1" {
		t.Errorf("want ProcessID=1, got %q", igp.ProcessID)
	}
	if len(igp.Interfaces) < 2 {
		t.Errorf("want at least 2 ISIS interfaces, got %d: %v", len(igp.Interfaces), igp.Interfaces)
	}

	// LDP should be detected on 2 interfaces
	if !topo.HasLDP {
		t.Error("want HasLDP=true")
	}
	if len(topo.LDPInterfaces) < 2 {
		t.Errorf("want at least 2 LDP interfaces, got %d: %v", len(topo.LDPInterfaces), topo.LDPInterfaces)
	}

	// Protocols slice should include "isis" and "ldp"
	protoSet := make(map[string]bool)
	for _, p := range topo.Protocols {
		protoSet[p] = true
	}
	if !protoSet["isis"] {
		t.Error("want 'isis' in Protocols")
	}
	if !protoSet["ldp"] {
		t.Error("want 'ldp' in Protocols")
	}
}

// TestBuildTopology_OSPFDetection verifies that OSPF processes and enabled
// interfaces are extracted from a Huawei-style # delimited config.
func TestBuildTopology_OSPFDetection(t *testing.T) {
	db := setupTestDB(t)
	insertDevice(t, db, "pe7", "PE7", "huawei")

	configText := `#
ospf 100
 area 0.0.0.0
#
interface GigabitEthernet0/0/0
 ip address 192.168.1.1 255.255.255.252
 ospf enable 100 area 0.0.0.0
#
interface GigabitEthernet0/0/1
 ip address 192.168.2.1 255.255.255.252
 ospf enable 100 area 0.0.0.0
#
interface LoopBack0
 ip address 2.2.2.7 255.255.255.255
#
`
	_, err := db.InsertConfigSnapshot(model.ConfigSnapshot{
		DeviceID:   "pe7",
		ConfigText: configText,
		Format:     "hierarchical",
		CapturedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("insert config: %v", err)
	}

	topo, err := plan.BuildTopology(db, "pe7")
	if err != nil {
		t.Fatalf("BuildTopology: %v", err)
	}

	// Should have exactly 1 IGP entry: ospf process 100
	if len(topo.IGPs) != 1 {
		t.Fatalf("want 1 IGP entry, got %d: %+v", len(topo.IGPs), topo.IGPs)
	}
	igp := topo.IGPs[0]
	if igp.Protocol != "ospf" {
		t.Errorf("want Protocol=ospf, got %q", igp.Protocol)
	}
	if igp.ProcessID != "100" {
		t.Errorf("want ProcessID=100, got %q", igp.ProcessID)
	}
	if len(igp.Interfaces) != 2 {
		t.Errorf("want 2 OSPF interfaces, got %d: %v", len(igp.Interfaces), igp.Interfaces)
	}

	// No LDP in this config
	if topo.HasLDP {
		t.Error("want HasLDP=false")
	}

	// Protocols slice should include "ospf" but not "ldp"
	protoSet := make(map[string]bool)
	for _, p := range topo.Protocols {
		protoSet[p] = true
	}
	if !protoSet["ospf"] {
		t.Error("want 'ospf' in Protocols")
	}
	if protoSet["ldp"] {
		t.Error("want no 'ldp' in Protocols")
	}
}
