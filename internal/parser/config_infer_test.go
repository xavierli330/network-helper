package parser

import (
	"strings"
	"testing"
)

func TestInferCommands_HuaweiBGP(t *testing.T) {
	config := `
sysname CoreRouter
#
bgp 65001
 router-id 10.0.0.1
 peer 10.0.0.2 as-number 65002
 peer 10.0.0.3 as-number 65003
#
interface GigabitEthernet0/0/0
 ip address 10.0.0.1 255.255.255.252
#
ospf 1
 area 0.0.0.0
  network 10.0.0.0 0.0.0.3
#
`
	cmds := InferCommands("huawei", config)
	if len(cmds) == 0 {
		t.Fatal("expected non-empty inferred commands")
	}

	// Should include BGP, OSPF, interface and routing commands
	found := map[string]bool{}
	for _, c := range cmds {
		found[c.Command] = true
	}

	expect := []string{
		"display bgp peer",
		"display ospf peer",
		"display interface brief",
		"display ip routing-table",
		"display current-configuration",
	}
	for _, e := range expect {
		if !found[e] {
			t.Errorf("expected command %q not found in inferred list", e)
		}
	}
}

func TestInferCommands_HuaweiMPLS(t *testing.T) {
	config := `
sysname PE1
#
mpls lsr-id 10.0.0.1
mpls
#
mpls ldp
#
mpls te
#
segment-routing
#
`
	cmds := InferCommands("huawei", config)
	found := map[string]bool{}
	for _, c := range cmds {
		found[c.Command] = true
	}

	expect := []string{
		"display mpls lsp",
		"display mpls ldp session",
		"display mpls te tunnel-interface",
		"display segment-routing mapping-server ipv4",
	}
	for _, e := range expect {
		if !found[e] {
			t.Errorf("expected command %q not found", e)
		}
	}
}

func TestInferCommands_HuaweiRoutePolicy(t *testing.T) {
	config := `
sysname PolicyRouter
#
route-policy RP-IN permit node 10
 if-match ip-prefix PL-IN
 apply local-preference 200
#
route-policy RP-OUT permit node 10
 if-match ip-prefix PL-OUT
#
acl number 3000
 rule 10 permit source 10.0.0.0 0.0.0.255
#
ip ip-prefix PL-IN permit 10.0.0.0 24
#
`
	cmds := InferCommands("huawei", config)
	found := map[string]bool{}
	for _, c := range cmds {
		found[c.Command] = true
	}

	if !found["display route-policy"] {
		t.Error("expected display route-policy")
	}
	if !found["display acl all"] {
		t.Error("expected display acl all")
	}
	if !found["display ip ip-prefix"] {
		t.Error("expected display ip ip-prefix")
	}
}

func TestInferCommands_EmptyConfig(t *testing.T) {
	cmds := InferCommands("huawei", "")
	if cmds != nil {
		t.Errorf("expected nil for empty config, got %d commands", len(cmds))
	}
}

func TestInferCommands_UnknownVendor(t *testing.T) {
	cmds := InferCommands("unknown", "some config")
	if cmds != nil {
		t.Errorf("expected nil for unknown vendor, got %d commands", len(cmds))
	}
}

func TestInferCommands_Cisco(t *testing.T) {
	config := `
router bgp 45090
 neighbor 10.0.0.1
  remote-as 65001
!
router ospf 1
!
interface GigabitEthernet0/0/0
 ipv4 address 10.0.0.1 255.255.255.252
!
`
	cmds := InferCommands("cisco", config)
	if len(cmds) == 0 {
		t.Fatal("expected non-empty inferred commands for cisco")
	}
	found := map[string]bool{}
	for _, c := range cmds {
		found[c.Command] = true
	}
	if !found["show bgp summary"] {
		t.Error("expected show bgp summary")
	}
	if !found["show ospf neighbor"] {
		t.Error("expected show ospf neighbor")
	}
}

func TestInferCommands_PriorityOrder(t *testing.T) {
	config := `
sysname TestRouter
#
bgp 65001
#
interface GigabitEthernet0/0/0
#
`
	cmds := InferCommands("huawei", config)
	if len(cmds) == 0 {
		t.Fatal("expected commands")
	}
	// First commands should be high priority
	if cmds[0].Priority != "high" {
		t.Errorf("first command should be high priority, got %q", cmds[0].Priority)
	}
	// Ensure no medium/low appears before high
	seenNonHigh := false
	for _, c := range cmds {
		if c.Priority != "high" {
			seenNonHigh = true
		}
		if seenNonHigh && c.Priority == "high" {
			t.Error("high priority command appeared after non-high priority")
			break
		}
	}
}

func TestCheckCoverage_HuaweiBasic(t *testing.T) {
	// We can't easily build a full Registry in a unit test without importing
	// the vendor packages, but we can test the InferCommands + GenerateSSHScript
	// integration independently.
	config := `
sysname TestRouter
#
bgp 65001
#
interface GigabitEthernet0/0/0
#
`
	cmds := InferCommands("huawei", config)
	if len(cmds) == 0 {
		t.Fatal("expected commands")
	}

	// Build a mock coverage report
	report := &CoverageReport{
		DeviceID: "test-router",
		Vendor:   "huawei",
	}
	for _, ic := range cmds {
		status := CoverageCovered
		if strings.HasPrefix(ic.Command, "display version") {
			status = CoverageUncovered
		}
		report.Items = append(report.Items, CoverageItem{
			Command:  ic.Command,
			Category: ic.Category,
			Status:   status,
			Priority: ic.Priority,
		})
	}
	report.TotalCount = len(report.Items)
	for _, item := range report.Items {
		if item.Status == CoverageCovered {
			report.CoveredCount++
		}
	}

	script := GenerateSSHScript(report)
	if script == "" {
		t.Error("expected non-empty SSH script")
	}
	if !strings.Contains(script, "nethelper Self-Check") {
		t.Error("SSH script should contain header")
	}
}

func TestGenerateSSHScript_AllCovered(t *testing.T) {
	report := &CoverageReport{
		Items: []CoverageItem{
			{Command: "display bgp peer", Status: CoverageCovered},
		},
	}
	script := GenerateSSHScript(report)
	if !strings.Contains(script, "All inferred commands are covered") {
		t.Error("expected all-covered message")
	}
}

func TestGenerateSSHScript_Nil(t *testing.T) {
	script := GenerateSSHScript(nil)
	if script != "" {
		t.Error("expected empty string for nil report")
	}
}

func TestInferCommands_VPNInstances(t *testing.T) {
	config := `
sysname VPNPE
#
ip vpn-instance CUST-A
 ipv4-family
  route-distinguisher 65001:100
#
ip vpn-instance CUST-B
 ipv4-family
  route-distinguisher 65001:200
#
`
	cmds := InferCommands("huawei", config)
	found := map[string]bool{}
	for _, c := range cmds {
		found[c.Command] = true
	}
	if !found["display ip vpn-instance"] {
		t.Error("expected display ip vpn-instance")
	}
	if !found["display ip routing-table vpn-instance CUST-A"] {
		t.Error("expected VPN instance A routing table command")
	}
	if !found["display ip routing-table vpn-instance CUST-B"] {
		t.Error("expected VPN instance B routing table command")
	}
}
