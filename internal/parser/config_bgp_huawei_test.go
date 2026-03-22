package parser

import (
	"encoding/json"
	"sort"
	"testing"
)

const huaweiBGPConfig = `
#
sysname PE-Router-01
#
bgp 45090
 router-id 10.200.0.35
 group toRR internal
 peer toRR connect-interface LoopBack0
 peer 10.200.0.1 as-number 45090
 peer 10.200.0.1 group toRR
 peer 10.200.0.1 description SZ-BA-0401-K03-MX480-GRR-01
 peer 10.200.0.2 as-number 45090
 peer 10.200.0.2 group toRR
 peer 10.200.0.2 description SZ-BA-0402-K04-NE40E-GRR-02
#
 ipv4-family unicast
  peer toRR enable
  peer 10.200.0.1 enable
  peer 10.200.0.1 group toRR
  undo peer 10.200.0.2 enable
#
 ipv4-family vpnv4
  peer toRR enable
  peer toRR route-policy RR-IN import
  peer toRR route-policy RR-OUT export
  peer toRR advertise-community
  peer 10.200.0.1 enable
  peer 10.200.0.1 group toRR
  peer 10.200.0.2 enable
  peer 10.200.0.2 group toRR
#
 ipv4-family vpn-instance GLOBAL
  group ebgp external
  peer 10.200.152.2 as-number 65028
  peer 10.200.152.2 group ebgp
  peer 10.200.152.2 description CD-GX-0402-C15-H12508-MAN-01
  peer ebgp route-policy ebgp-export export
  peer ebgp advertise-community
  peer ebgp enable
#
 ipv4-family vpn-instance MGT
  peer 10.200.152.74 as-number 65028
  peer 10.200.152.74 group ebgp
  peer 10.200.152.74 bfd enable
  peer 10.200.152.74 enable
  peer 10.200.152.74 route-policy MGT-ebgp-export export
#
interface LoopBack0
 ip address 10.200.0.35 255.255.255.255
#
`

func TestExtractBGPPeersHuawei(t *testing.T) {
	peers := ExtractBGPPeersHuawei(huaweiBGPConfig)
	if len(peers) == 0 {
		t.Fatal("expected peers, got none")
	}

	// Build lookup by (ip, af, vrf)
	type key struct{ ip, af, vrf string }
	m := make(map[key]int)
	for i, p := range peers {
		m[key{p.PeerIP, p.AddressFamily, p.VRF}] = i
	}

	// --- Global IPv4 unicast ---
	// 10.200.0.1 should be enabled in ipv4-unicast
	idx, ok := m[key{"10.200.0.1", "ipv4-unicast", ""}]
	if !ok {
		t.Fatal("missing 10.200.0.1 ipv4-unicast peer")
	}
	p := peers[idx]
	if p.LocalAS != 45090 {
		t.Errorf("LocalAS = %d, want 45090", p.LocalAS)
	}
	if p.RemoteAS != 45090 {
		t.Errorf("RemoteAS = %d, want 45090", p.RemoteAS)
	}
	if p.PeerGroup != "toRR" {
		t.Errorf("PeerGroup = %q, want toRR", p.PeerGroup)
	}
	if p.Description != "SZ-BA-0401-K03-MX480-GRR-01" {
		t.Errorf("Description = %q", p.Description)
	}
	if p.UpdateSource != "LoopBack0" {
		t.Errorf("UpdateSource = %q, want LoopBack0", p.UpdateSource)
	}
	if p.Enabled != 1 {
		t.Errorf("Enabled = %d, want 1", p.Enabled)
	}

	// 10.200.0.2 should NOT be in ipv4-unicast (undo peer enable)
	if _, ok := m[key{"10.200.0.2", "ipv4-unicast", ""}]; ok {
		t.Error("10.200.0.2 should be disabled in ipv4-unicast, should not appear")
	}

	// --- VPNv4 ---
	idx, ok = m[key{"10.200.0.1", "vpnv4", ""}]
	if !ok {
		t.Fatal("missing 10.200.0.1 vpnv4 peer")
	}
	p = peers[idx]
	if p.ImportPolicy != "RR-IN" {
		t.Errorf("ImportPolicy = %q, want RR-IN", p.ImportPolicy)
	}
	if p.ExportPolicy != "RR-OUT" {
		t.Errorf("ExportPolicy = %q, want RR-OUT", p.ExportPolicy)
	}
	if p.AdvertiseCommunity != 1 {
		t.Errorf("AdvertiseCommunity = %d, want 1", p.AdvertiseCommunity)
	}

	// 10.200.0.2 should be in vpnv4 (enabled explicitly)
	idx, ok = m[key{"10.200.0.2", "vpnv4", ""}]
	if !ok {
		t.Fatal("missing 10.200.0.2 vpnv4 peer")
	}
	p = peers[idx]
	if p.ImportPolicy != "RR-IN" {
		t.Errorf("10.200.0.2 vpnv4 ImportPolicy = %q, want RR-IN (inherited from group toRR)", p.ImportPolicy)
	}

	// --- VPN-instance GLOBAL ---
	idx, ok = m[key{"10.200.152.2", "ipv4-unicast", "GLOBAL"}]
	if !ok {
		t.Fatal("missing 10.200.152.2 in VRF GLOBAL")
	}
	p = peers[idx]
	if p.RemoteAS != 65028 {
		t.Errorf("RemoteAS = %d, want 65028", p.RemoteAS)
	}
	if p.VRF != "GLOBAL" {
		t.Errorf("VRF = %q, want GLOBAL", p.VRF)
	}
	if p.ExportPolicy != "ebgp-export" {
		t.Errorf("ExportPolicy = %q, want ebgp-export", p.ExportPolicy)
	}
	if p.AdvertiseCommunity != 1 {
		t.Errorf("AdvertiseCommunity = %d, want 1", p.AdvertiseCommunity)
	}

	// --- VPN-instance MGT ---
	idx, ok = m[key{"10.200.152.74", "ipv4-unicast", "MGT"}]
	if !ok {
		t.Fatal("missing 10.200.152.74 in VRF MGT")
	}
	p = peers[idx]
	if p.RemoteAS != 65028 {
		t.Errorf("RemoteAS = %d, want 65028", p.RemoteAS)
	}
	if p.BFDEnabled != 1 {
		t.Errorf("BFDEnabled = %d, want 1", p.BFDEnabled)
	}
	if p.ExportPolicy != "MGT-ebgp-export" {
		t.Errorf("ExportPolicy = %q, want MGT-ebgp-export", p.ExportPolicy)
	}
}

const huaweiVRFConfig = `
#
ip vpn-instance GLOBAL
 ipv4-family
  route-distinguisher 45090:10031
  vpn-target 45090:1003 45090:1008 import-extcommunity
  vpn-target 45090:1003 export-extcommunity
  tnl-policy OPEN-tnl-policy
#
ip vpn-instance MGT
 ipv4-family
  route-distinguisher 45090:20001
  vpn-target 45090:2001 import-extcommunity
  vpn-target 45090:2001 export-extcommunity
#
interface LoopBack0
 ip address 10.0.0.1 255.255.255.255
#
`

func TestExtractVRFInstancesHuawei(t *testing.T) {
	vrfs := ExtractVRFInstancesHuawei(huaweiVRFConfig)
	if len(vrfs) != 2 {
		t.Fatalf("expected 2 VRFs, got %d", len(vrfs))
	}

	// Sort by name for deterministic checks
	sort.Slice(vrfs, func(i, j int) bool { return vrfs[i].VRFName < vrfs[j].VRFName })

	// GLOBAL
	v := vrfs[0]
	if v.VRFName != "GLOBAL" {
		t.Errorf("VRFName = %q, want GLOBAL", v.VRFName)
	}
	if v.RD != "45090:10031" {
		t.Errorf("RD = %q, want 45090:10031", v.RD)
	}
	if v.TunnelPolicy != "OPEN-tnl-policy" {
		t.Errorf("TunnelPolicy = %q, want OPEN-tnl-policy", v.TunnelPolicy)
	}

	var importRTs []string
	if err := json.Unmarshal([]byte(v.ImportRT), &importRTs); err != nil {
		t.Fatalf("bad ImportRT JSON: %v", err)
	}
	if len(importRTs) != 2 || importRTs[0] != "45090:1003" || importRTs[1] != "45090:1008" {
		t.Errorf("ImportRT = %v, want [45090:1003, 45090:1008]", importRTs)
	}

	var exportRTs []string
	if err := json.Unmarshal([]byte(v.ExportRT), &exportRTs); err != nil {
		t.Fatalf("bad ExportRT JSON: %v", err)
	}
	if len(exportRTs) != 1 || exportRTs[0] != "45090:1003" {
		t.Errorf("ExportRT = %v, want [45090:1003]", exportRTs)
	}

	// MGT
	v = vrfs[1]
	if v.VRFName != "MGT" {
		t.Errorf("VRFName = %q, want MGT", v.VRFName)
	}
	if v.RD != "45090:20001" {
		t.Errorf("RD = %q, want 45090:20001", v.RD)
	}
}

const huaweiRoutePolicyConfig = `
#
route-policy RR-IN permit node 10
 if-match community-filter BR1-LP
 apply local-preference 200
#
route-policy RR-IN permit node 20
 if-match community-filter BR2-LP
 apply local-preference 280
#
route-policy RR-IN permit node 1000
#
route-policy RR-OUT deny node 10
 if-match ip-prefix DENY-LIST
#
route-policy RR-OUT permit node 20
#
interface LoopBack0
 ip address 10.0.0.1 255.255.255.255
#
`

func TestExtractRoutePoliciesHuawei(t *testing.T) {
	policies := ExtractRoutePoliciesHuawei(huaweiRoutePolicyConfig)
	if len(policies) != 2 {
		t.Fatalf("expected 2 policies, got %d", len(policies))
	}

	// Sort by name for deterministic order
	sort.Slice(policies, func(i, j int) bool { return policies[i].PolicyName < policies[j].PolicyName })

	// RR-IN: 3 nodes
	rrin := policies[0]
	if rrin.PolicyName != "RR-IN" {
		t.Errorf("PolicyName = %q, want RR-IN", rrin.PolicyName)
	}
	if rrin.VendorType != "route-policy" {
		t.Errorf("VendorType = %q, want route-policy", rrin.VendorType)
	}
	if len(rrin.Nodes) != 3 {
		t.Fatalf("RR-IN nodes = %d, want 3", len(rrin.Nodes))
	}

	// Node 10
	n := rrin.Nodes[0]
	if n.Sequence != 10 {
		t.Errorf("node seq = %d, want 10", n.Sequence)
	}
	if n.Action != "permit" {
		t.Errorf("node action = %q, want permit", n.Action)
	}

	var matchClauses []map[string]string
	if err := json.Unmarshal([]byte(n.MatchClauses), &matchClauses); err != nil {
		t.Fatalf("bad MatchClauses JSON: %v", err)
	}
	if len(matchClauses) != 1 || matchClauses[0]["type"] != "community-filter" || matchClauses[0]["value"] != "BR1-LP" {
		t.Errorf("MatchClauses = %v", matchClauses)
	}

	var applyClauses []map[string]string
	if err := json.Unmarshal([]byte(n.ApplyClauses), &applyClauses); err != nil {
		t.Fatalf("bad ApplyClauses JSON: %v", err)
	}
	if len(applyClauses) != 1 || applyClauses[0]["type"] != "local-preference" || applyClauses[0]["value"] != "200" {
		t.Errorf("ApplyClauses = %v", applyClauses)
	}

	// Node 1000 (empty permit)
	n = rrin.Nodes[2]
	if n.Sequence != 1000 {
		t.Errorf("node seq = %d, want 1000", n.Sequence)
	}
	if n.Action != "permit" {
		t.Errorf("node action = %q, want permit", n.Action)
	}
	var emptyMatch []map[string]string
	json.Unmarshal([]byte(n.MatchClauses), &emptyMatch)
	if len(emptyMatch) != 0 {
		t.Errorf("node 1000 should have no match clauses, got %v", emptyMatch)
	}

	// RR-OUT: 2 nodes
	rrout := policies[1]
	if rrout.PolicyName != "RR-OUT" {
		t.Errorf("PolicyName = %q, want RR-OUT", rrout.PolicyName)
	}
	if len(rrout.Nodes) != 2 {
		t.Fatalf("RR-OUT nodes = %d, want 2", len(rrout.Nodes))
	}
	if rrout.Nodes[0].Action != "deny" {
		t.Errorf("RR-OUT node 10 action = %q, want deny", rrout.Nodes[0].Action)
	}

	// Check raw text contains both nodes
	if !containsSubstring(rrout.RawText, "route-policy RR-OUT deny node 10") {
		t.Errorf("RawText missing RR-OUT node 10 header")
	}
	if !containsSubstring(rrout.RawText, "route-policy RR-OUT permit node 20") {
		t.Errorf("RawText missing RR-OUT node 20 header")
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && stringContains(s, sub))
}

func stringContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
