package parser

import (
	"encoding/json"
	"testing"

	"github.com/xavierli/nethelper/internal/model"
)

const ciscoFullBGPConfig = `
router bgp 45090
 bgp router-id 3.3.3.3
 address-family ipv4 unicast
 !
 address-family vpnv4 unicast
 !
 neighbor-group CU-eRR
  remote-as 45090
  update-source Loopback0
  address-family ipv4 unicast
   route-policy rp_CU_eRR_V4_in in
   route-policy rp_CU_eRR_V4_out out
   soft-reconfiguration inbound always
  !
  address-family vpnv4 unicast
   route-policy rp_CU_eRR_VPNv4_in in
   route-policy rp_CU_eRR_VPNv4_out out
  !
 !
 neighbor 10.200.0.3
  use neighbor-group CU-eRR
  description SZ-BH-0701-K03-MX480-TRR-01
 !
 neighbor 10.200.0.23
  use neighbor-group CU-eRR
  description TJ-DQ-M302-G01-MX480-TRR-01
 !
 vrf CAP
  rd 45090:11088
  address-family ipv4 unicast
   import route-policy RTN-IN
   import route-target
    45090:1004
    45090:1008
   !
   export route-target
    45090:1004
   !
  !
  neighbor 12.194.120.125
   remote-as 4134
   description CT_V4_GZ_YS_01
   address-family ipv4 unicast
    route-policy rp_isp_in in
    route-policy rp_isp_out out
   !
  !
 !
!
`

func TestExtractBGPPeersCisco_GlobalNeighbors(t *testing.T) {
	peers := ExtractBGPPeersCisco(ciscoFullBGPConfig)
	if len(peers) == 0 {
		t.Fatal("expected peers, got 0")
	}

	// Collect peers by IP
	byIP := make(map[string][]string) // ip -> list of AFs
	for _, p := range peers {
		byIP[p.PeerIP] = append(byIP[p.PeerIP], p.AddressFamily)
	}

	// 10.200.0.3 should have 2 AFs from group inheritance
	if afs, ok := byIP["10.200.0.3"]; !ok {
		t.Error("missing peer 10.200.0.3")
	} else if len(afs) != 2 {
		t.Errorf("10.200.0.3: expected 2 AFs, got %d: %v", len(afs), afs)
	}

	// Same for 10.200.0.23
	if afs, ok := byIP["10.200.0.23"]; !ok {
		t.Error("missing peer 10.200.0.23")
	} else if len(afs) != 2 {
		t.Errorf("10.200.0.23: expected 2 AFs, got %d: %v", len(afs), afs)
	}
}

func TestExtractBGPPeersCisco_GroupInheritance(t *testing.T) {
	peers := ExtractBGPPeersCisco(ciscoFullBGPConfig)

	for _, p := range peers {
		if p.PeerIP == "10.200.0.3" && p.AddressFamily == "ipv4 unicast" {
			if p.LocalAS != 45090 {
				t.Errorf("LocalAS: got %d, want 45090", p.LocalAS)
			}
			if p.RemoteAS != 45090 {
				t.Errorf("RemoteAS: got %d, want 45090", p.RemoteAS)
			}
			if p.PeerGroup != "CU-eRR" {
				t.Errorf("PeerGroup: got %q, want CU-eRR", p.PeerGroup)
			}
			if p.UpdateSource != "Loopback0" {
				t.Errorf("UpdateSource: got %q, want Loopback0", p.UpdateSource)
			}
			if p.ImportPolicy != "rp_CU_eRR_V4_in" {
				t.Errorf("ImportPolicy: got %q, want rp_CU_eRR_V4_in", p.ImportPolicy)
			}
			if p.ExportPolicy != "rp_CU_eRR_V4_out" {
				t.Errorf("ExportPolicy: got %q, want rp_CU_eRR_V4_out", p.ExportPolicy)
			}
			if p.SoftReconfig != 1 {
				t.Errorf("SoftReconfig: got %d, want 1", p.SoftReconfig)
			}
			if p.Description != "SZ-BH-0701-K03-MX480-TRR-01" {
				t.Errorf("Description: got %q", p.Description)
			}
			return
		}
	}
	t.Error("peer 10.200.0.3 with AF ipv4 unicast not found")
}

func TestExtractBGPPeersCisco_VPNv4Inheritance(t *testing.T) {
	peers := ExtractBGPPeersCisco(ciscoFullBGPConfig)

	for _, p := range peers {
		if p.PeerIP == "10.200.0.23" && p.AddressFamily == "vpnv4 unicast" {
			if p.ImportPolicy != "rp_CU_eRR_VPNv4_in" {
				t.Errorf("ImportPolicy: got %q, want rp_CU_eRR_VPNv4_in", p.ImportPolicy)
			}
			if p.ExportPolicy != "rp_CU_eRR_VPNv4_out" {
				t.Errorf("ExportPolicy: got %q, want rp_CU_eRR_VPNv4_out", p.ExportPolicy)
			}
			return
		}
	}
	t.Error("peer 10.200.0.23 with AF vpnv4 unicast not found")
}

func TestExtractBGPPeersCisco_VRFNeighbor(t *testing.T) {
	peers := ExtractBGPPeersCisco(ciscoFullBGPConfig)

	for _, p := range peers {
		if p.PeerIP == "12.194.120.125" {
			if p.VRF != "CAP" {
				t.Errorf("VRF: got %q, want CAP", p.VRF)
			}
			if p.RemoteAS != 4134 {
				t.Errorf("RemoteAS: got %d, want 4134", p.RemoteAS)
			}
			if p.Description != "CT_V4_GZ_YS_01" {
				t.Errorf("Description: got %q", p.Description)
			}
			if p.ImportPolicy != "rp_isp_in" {
				t.Errorf("ImportPolicy: got %q, want rp_isp_in", p.ImportPolicy)
			}
			if p.ExportPolicy != "rp_isp_out" {
				t.Errorf("ExportPolicy: got %q, want rp_isp_out", p.ExportPolicy)
			}
			return
		}
	}
	t.Error("VRF peer 12.194.120.125 not found")
}

func TestExtractBGPPeersCisco_EmptyConfig(t *testing.T) {
	peers := ExtractBGPPeersCisco("")
	if peers != nil {
		t.Errorf("expected nil for empty config, got %d peers", len(peers))
	}
}

func TestExtractBGPPeersCisco_DirectNeighborNoGroup(t *testing.T) {
	config := `
router bgp 65000
 neighbor 10.0.0.1
  remote-as 65001
  description DirectPeer
  address-family ipv4 unicast
   route-policy ALLOW_ALL in
   route-policy DENY_DEFAULT out
  !
 !
!
`
	peers := ExtractBGPPeersCisco(config)
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(peers))
	}
	p := peers[0]
	if p.PeerIP != "10.0.0.1" {
		t.Errorf("PeerIP: got %q", p.PeerIP)
	}
	if p.RemoteAS != 65001 {
		t.Errorf("RemoteAS: got %d", p.RemoteAS)
	}
	if p.ImportPolicy != "ALLOW_ALL" {
		t.Errorf("ImportPolicy: got %q", p.ImportPolicy)
	}
	if p.ExportPolicy != "DENY_DEFAULT" {
		t.Errorf("ExportPolicy: got %q", p.ExportPolicy)
	}
}

func TestExtractBGPPeersCisco_ShutdownNeighbor(t *testing.T) {
	config := `
router bgp 65000
 neighbor 10.0.0.2
  remote-as 65002
  shutdown
 !
!
`
	peers := ExtractBGPPeersCisco(config)
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(peers))
	}
	if peers[0].Shutdown != 1 {
		t.Error("expected Shutdown=1")
	}
	if peers[0].Enabled != 0 {
		t.Error("expected Enabled=0")
	}
}

// ---------- VRF Tests ----------

const ciscoVRFConfig = `
vrf CAP
 address-family ipv4 unicast
  import route-policy RTN-IN
  import route-target
   45090:1004
   45090:1008
  !
  export route-target
   45090:1004
  !
 !
!
vrf MGMT
 address-family ipv4 unicast
  import route-target
   65000:100
  !
  export route-target
   65000:100
  !
 !
!
`

func TestExtractVRFInstancesCisco_Standalone(t *testing.T) {
	vrfs := ExtractVRFInstancesCisco(ciscoVRFConfig)
	if len(vrfs) != 2 {
		t.Fatalf("expected 2 VRFs, got %d", len(vrfs))
	}

	// Find CAP
	var cap *model.VRFInstance
	for i := range vrfs {
		if vrfs[i].VRFName == "CAP" {
			cap = &vrfs[i]
			break
		}
	}
	if cap == nil {
		t.Fatal("VRF CAP not found")
	}
	if cap.ImportPolicy != "RTN-IN" {
		t.Errorf("ImportPolicy: got %q, want RTN-IN", cap.ImportPolicy)
	}

	var importRTs []string
	if err := json.Unmarshal([]byte(cap.ImportRT), &importRTs); err != nil {
		t.Fatalf("ImportRT JSON: %v", err)
	}
	if len(importRTs) != 2 || importRTs[0] != "45090:1004" || importRTs[1] != "45090:1008" {
		t.Errorf("ImportRT: got %v", importRTs)
	}

	var exportRTs []string
	if err := json.Unmarshal([]byte(cap.ExportRT), &exportRTs); err != nil {
		t.Fatalf("ExportRT JSON: %v", err)
	}
	if len(exportRTs) != 1 || exportRTs[0] != "45090:1004" {
		t.Errorf("ExportRT: got %v", exportRTs)
	}
}

func TestExtractVRFInstancesCisco_WithBGPBlock(t *testing.T) {
	// Config has both standalone vrf and BGP vrf sub-block with RD
	config := `
vrf CAP
 address-family ipv4 unicast
  import route-target
   45090:1004
  !
  export route-target
   45090:1004
  !
 !
!
router bgp 45090
 vrf CAP
  rd 45090:11088
  address-family ipv4 unicast
   import route-policy RTN-IN
  !
 !
!
`
	vrfs := ExtractVRFInstancesCisco(config)
	if len(vrfs) != 1 {
		t.Fatalf("expected 1 VRF, got %d", len(vrfs))
	}
	if vrfs[0].RD != "45090:11088" {
		t.Errorf("RD: got %q, want 45090:11088", vrfs[0].RD)
	}
	if vrfs[0].ImportPolicy != "RTN-IN" {
		t.Errorf("ImportPolicy: got %q, want RTN-IN", vrfs[0].ImportPolicy)
	}
}

func TestExtractVRFInstancesCisco_BGPOnly(t *testing.T) {
	// VRF only defined inside router bgp
	config := `
router bgp 45090
 vrf INTERNET
  rd 45090:999
  address-family ipv4 unicast
   import route-target
    45090:999
   !
   export route-target
    45090:999
   !
  !
 !
!
`
	vrfs := ExtractVRFInstancesCisco(config)
	if len(vrfs) != 1 {
		t.Fatalf("expected 1 VRF, got %d", len(vrfs))
	}
	if vrfs[0].VRFName != "INTERNET" {
		t.Errorf("VRFName: got %q", vrfs[0].VRFName)
	}
	if vrfs[0].RD != "45090:999" {
		t.Errorf("RD: got %q", vrfs[0].RD)
	}
}

func TestExtractVRFInstancesCisco_EmptyConfig(t *testing.T) {
	vrfs := ExtractVRFInstancesCisco("")
	if len(vrfs) != 0 {
		t.Errorf("expected 0 VRFs from empty config, got %d", len(vrfs))
	}
}

// ---------- Route-Policy Tests ----------

const ciscoRoutePolicyConfig = `
route-policy rp_CU_eRR_V4_in
  if community matches-any (45090:3, 65535:3) then
    set local-preference 299
  elseif community matches-any (45090:1, 65535:1) then
    set local-preference 290
  else
    pass
  endif
end-policy
!
route-policy rp_isp_in
  apply rp_ISP_DENY_DEFAULT_V4
  apply rp_ISP_FILTER_BOGONS
  if destination in pl_ISP_GZ then
    set local-preference 350
  endif
  pass
end-policy
!
route-policy DENY_ALL
  drop
end-policy
!
`

func TestExtractRoutePoliciesCisco_Count(t *testing.T) {
	policies := ExtractRoutePoliciesCisco(ciscoRoutePolicyConfig)
	if len(policies) != 3 {
		t.Fatalf("expected 3 policies, got %d", len(policies))
	}
}

func TestExtractRoutePoliciesCisco_PolicyDetails(t *testing.T) {
	policies := ExtractRoutePoliciesCisco(ciscoRoutePolicyConfig)

	// Find rp_CU_eRR_V4_in
	var p1 *model.RoutePolicy
	for i := range policies {
		if policies[i].PolicyName == "rp_CU_eRR_V4_in" {
			p1 = &policies[i]
			break
		}
	}
	if p1 == nil {
		t.Fatal("rp_CU_eRR_V4_in not found")
	}
	if p1.VendorType != "route-policy" {
		t.Errorf("VendorType: got %q", p1.VendorType)
	}
	if !containsStr(p1.RawText, "set local-preference 299") {
		t.Error("RawText missing 'set local-preference 299'")
	}
	if !containsStr(p1.RawText, "end-policy") {
		t.Error("RawText missing 'end-policy'")
	}
	if len(p1.Nodes) != 3 {
		t.Fatalf("expected 3 nodes (if/elseif/else), got %d", len(p1.Nodes))
	}
	// First node: if community matches-any → set local-preference 299
	if p1.Nodes[0].MatchClauses == "[]" {
		t.Error("Nodes[0].MatchClauses should not be empty")
	}
	if !containsStr(p1.Nodes[0].ApplyClauses, "local-preference") {
		t.Errorf("Nodes[0] should set local-preference, got %q", p1.Nodes[0].ApplyClauses)
	}
	// Last node: else → pass → permit
	if p1.Nodes[2].Action != "permit" {
		t.Errorf("Nodes[2] Action: got %q, want permit", p1.Nodes[2].Action)
	}
}

func TestExtractRoutePoliciesCisco_ApplyClauses(t *testing.T) {
	policies := ExtractRoutePoliciesCisco(ciscoRoutePolicyConfig)

	var p2 *model.RoutePolicy
	for i := range policies {
		if policies[i].PolicyName == "rp_isp_in" {
			p2 = &policies[i]
			break
		}
	}
	if p2 == nil {
		t.Fatal("rp_isp_in not found")
	}

	if len(p2.Nodes) < 2 {
		t.Fatalf("expected at least 2 nodes, got %d", len(p2.Nodes))
	}

	// First node should have the apply clauses (pre-conditional block)
	if !containsStr(p2.Nodes[0].ApplyClauses, "rp_ISP_DENY_DEFAULT_V4") {
		t.Errorf("Nodes[0] ApplyClauses missing rp_ISP_DENY_DEFAULT_V4, got %q", p2.Nodes[0].ApplyClauses)
	}
	if !containsStr(p2.Nodes[0].ApplyClauses, "rp_ISP_FILTER_BOGONS") {
		t.Errorf("Nodes[0] ApplyClauses missing rp_ISP_FILTER_BOGONS, got %q", p2.Nodes[0].ApplyClauses)
	}

	// Second node should have destination match clause
	if !containsStr(p2.Nodes[1].MatchClauses, "destination-prefix-set") {
		t.Errorf("Nodes[1] MatchClauses missing destination match, got %q", p2.Nodes[1].MatchClauses)
	}
}

func TestExtractRoutePoliciesCisco_DenyPolicy(t *testing.T) {
	policies := ExtractRoutePoliciesCisco(ciscoRoutePolicyConfig)

	var p3 *model.RoutePolicy
	for i := range policies {
		if policies[i].PolicyName == "DENY_ALL" {
			p3 = &policies[i]
			break
		}
	}
	if p3 == nil {
		t.Fatal("DENY_ALL not found")
	}
	if p3.Nodes[0].Action != "deny" {
		t.Errorf("Action: got %q, want deny", p3.Nodes[0].Action)
	}
}

func TestExtractRoutePoliciesCisco_EmptyConfig(t *testing.T) {
	policies := ExtractRoutePoliciesCisco("")
	if len(policies) != 0 {
		t.Errorf("expected 0 policies from empty config, got %d", len(policies))
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && findSubstring(s, sub))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
