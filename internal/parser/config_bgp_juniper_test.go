package parser

import (
	"testing"
)

const juniperBGPConfig = `
protocols {
    bgp {
        group toTJ-TRR {
            type internal;
            local-address 10.200.1.86;
            family inet {
                unicast;
            }
            family inet-vpn {
                unicast;
            }
            family inet6-vpn {
                unicast;
            }
            vpn-apply-export;
            neighbor 10.200.0.4 {
                description TJ-DQ-M302-G01-MX480-TRR-01;
                import RR-IN;
                export RR-OUT;
            }
            neighbor 41.7.6.251 {
                description TJ-DQ-M302-G01-MX480-TRR-02;
            }
        }
        group toSZ-TRR {
            type external;
            local-address 10.200.1.86;
            peer-as 65001;
            family inet {
                unicast;
            }
            neighbor 10.200.0.9 {
                description SZ-BH-0701-M04-MX480-QCRR-01;
                import RR-IN;
                export RR-OUT;
            }
        }
    }
}
`

func TestExtractBGPPeersJuniper(t *testing.T) {
	peers := ExtractBGPPeersJuniper(juniperBGPConfig)
	if len(peers) == 0 {
		t.Fatal("expected BGP peers, got none")
	}

	// Group toTJ-TRR has 3 families × 2 neighbors = 6 peers.
	// Group toSZ-TRR has 1 family × 1 neighbor = 1 peer.
	// Total = 7
	if len(peers) != 7 {
		t.Fatalf("expected 7 peers, got %d", len(peers))
	}

	// Check first neighbor in first group.
	found := false
	for _, p := range peers {
		if p.PeerIP == "10.200.0.4" && p.AddressFamily == "ipv4-unicast" {
			found = true
			if p.PeerGroup != "toTJ-TRR" {
				t.Errorf("expected group toTJ-TRR, got %s", p.PeerGroup)
			}
			if p.Description != "TJ-DQ-M302-G01-MX480-TRR-01" {
				t.Errorf("unexpected description: %s", p.Description)
			}
			if p.ImportPolicy != "RR-IN" {
				t.Errorf("expected import RR-IN, got %s", p.ImportPolicy)
			}
			if p.ExportPolicy != "RR-OUT" {
				t.Errorf("expected export RR-OUT, got %s", p.ExportPolicy)
			}
			if p.UpdateSource != "10.200.1.86" {
				t.Errorf("expected update-source 10.200.1.86, got %s", p.UpdateSource)
			}
			break
		}
	}
	if !found {
		t.Error("did not find peer 10.200.0.4 with ipv4-unicast")
	}

	// Check external peer inherits peer-as from group.
	for _, p := range peers {
		if p.PeerIP == "10.200.0.9" {
			if p.RemoteAS != 65001 {
				t.Errorf("expected remote-as 65001, got %d", p.RemoteAS)
			}
			break
		}
	}

	// Check second neighbor inherits group-level import/export (empty since
	// neighbor 41.7.6.251 has no import/export).
	for _, p := range peers {
		if p.PeerIP == "41.7.6.251" && p.AddressFamily == "vpnv4" {
			if p.ImportPolicy != "" {
				t.Errorf("expected empty import policy for 41.7.6.251, got %s", p.ImportPolicy)
			}
			break
		}
	}
}

const juniperVRFConfig = `
routing-instances {
    CAP {
        instance-type vrf;
        route-distinguisher 45090:10110;
        vrf-import CAP-ISP-VRF-import;
        vrf-export CAP-ISP-VRF-export;
        vrf-table-label;
        interface xe-0/0/2.0;
        interface xe-0/0/3.0;
        protocols {
            bgp {
                group ISP-V4 {
                    type external;
                    peer-as 4134;
                    family inet {
                        unicast;
                    }
                    neighbor 135.28.160.195 {
                        description CT_V4_SZ_BH_01;
                        import ct-bgp-in;
                        export ct-bgp-out;
                    }
                }
            }
        }
    }
    MGMT {
        instance-type vrf;
        route-distinguisher 45090:20220;
        vrf-import MGMT-import;
        vrf-export MGMT-export;
        vrf-table-label;
    }
}
`

func TestExtractVRFInstancesJuniper(t *testing.T) {
	vrfs := ExtractVRFInstancesJuniper(juniperVRFConfig)
	if len(vrfs) != 2 {
		t.Fatalf("expected 2 VRFs, got %d", len(vrfs))
	}

	var cap *struct{ vrf int }
	for i, v := range vrfs {
		if v.VRFName == "CAP" {
			idx := i
			cap = &struct{ vrf int }{vrf: idx}
			if v.RD != "45090:10110" {
				t.Errorf("CAP RD: expected 45090:10110, got %s", v.RD)
			}
			if v.ImportPolicy != "CAP-ISP-VRF-import" {
				t.Errorf("CAP import: expected CAP-ISP-VRF-import, got %s", v.ImportPolicy)
			}
			if v.ExportPolicy != "CAP-ISP-VRF-export" {
				t.Errorf("CAP export: expected CAP-ISP-VRF-export, got %s", v.ExportPolicy)
			}
		}
	}
	if cap == nil {
		t.Error("VRF CAP not found")
	}
}

func TestExtractVRFBGPPeersJuniper(t *testing.T) {
	peers := ExtractVRFBGPPeersJuniper(juniperVRFConfig)
	if len(peers) != 1 {
		t.Fatalf("expected 1 VRF BGP peer, got %d", len(peers))
	}
	p := peers[0]
	if p.VRF != "CAP" {
		t.Errorf("expected VRF CAP, got %s", p.VRF)
	}
	if p.PeerIP != "135.28.160.195" {
		t.Errorf("expected peer 135.28.160.195, got %s", p.PeerIP)
	}
	if p.RemoteAS != 4134 {
		t.Errorf("expected remote-as 4134, got %d", p.RemoteAS)
	}
	if p.ImportPolicy != "ct-bgp-in" {
		t.Errorf("expected import ct-bgp-in, got %s", p.ImportPolicy)
	}
	if p.Description != "CT_V4_SZ_BH_01" {
		t.Errorf("expected description CT_V4_SZ_BH_01, got %s", p.Description)
	}
}

const juniperPolicyConfig = `
policy-options {
    policy-statement RR-IN {
        term P_high-priority-route {
            from {
                community [ 65535:3 45090:3 ];
            }
            then {
                local-preference 300;
                accept;
            }
        }
        term P_mid-priority-route {
            from {
                community [ 65535:1 45090:1 ];
            }
            then {
                local-preference 200;
                accept;
            }
        }
        then accept;
    }
    policy-statement DENY-ALL {
        then reject;
    }
    community target:45090:1004 members target:45090:1004;
}
`

func TestExtractRoutePoliciesJuniper(t *testing.T) {
	policies := ExtractRoutePoliciesJuniper(juniperPolicyConfig)
	if len(policies) != 2 {
		t.Fatalf("expected 2 policies, got %d", len(policies))
	}

	// Find RR-IN.
	var rrIn *int
	for i, pol := range policies {
		if pol.PolicyName == "RR-IN" {
			idx := i
			rrIn = &idx
		}
	}
	if rrIn == nil {
		t.Fatal("policy RR-IN not found")
	}

	pol := policies[*rrIn]
	if pol.VendorType != "policy-statement" {
		t.Errorf("expected vendor_type policy-statement, got %s", pol.VendorType)
	}

	// RR-IN should have 2 terms + 1 default = 3 nodes.
	if len(pol.Nodes) != 3 {
		t.Fatalf("expected 3 nodes in RR-IN, got %d", len(pol.Nodes))
	}

	// First term.
	n0 := pol.Nodes[0]
	if n0.TermName != "P_high-priority-route" {
		t.Errorf("term 0 name: %s", n0.TermName)
	}
	if n0.Action != "accept" {
		t.Errorf("term 0 action: %s", n0.Action)
	}
	if n0.MatchClauses == "" {
		t.Error("term 0 match_clauses is empty")
	}
	if n0.ApplyClauses == "" {
		t.Error("term 0 apply_clauses is empty")
	}

	// Default action.
	nDefault := pol.Nodes[2]
	if nDefault.TermName != "_default" {
		t.Errorf("default node term_name: %s", nDefault.TermName)
	}
	if nDefault.Action != "accept" {
		t.Errorf("default action: %s", nDefault.Action)
	}

	// DENY-ALL policy.
	var denyAll *int
	for i, pol := range policies {
		if pol.PolicyName == "DENY-ALL" {
			idx := i
			denyAll = &idx
		}
	}
	if denyAll == nil {
		t.Fatal("policy DENY-ALL not found")
	}
	dp := policies[*denyAll]
	if len(dp.Nodes) != 1 {
		t.Fatalf("DENY-ALL expected 1 node, got %d", len(dp.Nodes))
	}
	if dp.Nodes[0].Action != "reject" {
		t.Errorf("DENY-ALL action: %s", dp.Nodes[0].Action)
	}
}

func TestExtractJuniperBlock(t *testing.T) {
	config := `
protocols {
    bgp {
        group test {
            type internal;
        }
    }
}
`
	block := extractJuniperBlock(config, "protocols")
	if block == "" {
		t.Fatal("extractJuniperBlock returned empty for 'protocols'")
	}
	if !containsString(block, "bgp") {
		t.Error("protocols block should contain 'bgp'")
	}

	inner := extractJuniperBlock(block, "bgp")
	if inner == "" {
		t.Fatal("extractJuniperBlock returned empty for 'bgp'")
	}
	if !containsString(inner, "group test") {
		t.Error("bgp block should contain 'group test'")
	}
}

func containsString(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && contains(s, substr)
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
