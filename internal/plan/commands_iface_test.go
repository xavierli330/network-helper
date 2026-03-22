package plan

import (
	"strings"
	"testing"
)

func TestIfaceIsolateCommands_LAGFirst(t *testing.T) {
	lags := []LAGBundle{
		{Name: "Eth-Trunk1", Description: "uplink-to-core"},
	}
	links := []PhysicalLink{
		{Interface: "GigabitEthernet0/0/1", Description: "server-rack-01"},
		{Interface: "GigabitEthernet0/0/2", Description: "server-rack-02"},
	}

	cmds := ifaceIsolateCommands("dev-01", lags, links, "huawei")

	if len(cmds) != 2 {
		t.Fatalf("expected 2 DeviceCommands, got %d", len(cmds))
	}

	// First command must be LAG
	if !strings.Contains(cmds[0].Purpose, "LAG 上联") {
		t.Errorf("first command should be LAG uplink, got purpose: %s", cmds[0].Purpose)
	}

	// Second command must be physical
	if !strings.Contains(cmds[1].Purpose, "物理下联") {
		t.Errorf("second command should be physical downlink, got purpose: %s", cmds[1].Purpose)
	}

	// Check LAG command content
	lagCmds := cmds[0].Commands
	if lagCmds[0] != "system-view" {
		t.Errorf("expected system-view first, got %q", lagCmds[0])
	}
	if lagCmds[len(lagCmds)-1] != "return" {
		t.Errorf("expected return last, got %q", lagCmds[len(lagCmds)-1])
	}
	if !containsLine(lagCmds, "interface Eth-Trunk1") {
		t.Error("expected 'interface Eth-Trunk1' in LAG commands")
	}
	if !containsLine(lagCmds, "shutdown") {
		t.Error("expected 'shutdown' in LAG commands")
	}

	// Check physical command has both interfaces
	physCmds := cmds[1].Commands
	if !containsLinePrefix(physCmds, "interface GigabitEthernet0/0/1") {
		t.Error("expected GigabitEthernet0/0/1 in physical commands")
	}
	if !containsLinePrefix(physCmds, "interface GigabitEthernet0/0/2") {
		t.Error("expected GigabitEthernet0/0/2 in physical commands")
	}
}

func TestIfaceIsolateCommands_NoLAG(t *testing.T) {
	links := []PhysicalLink{
		{Interface: "GigabitEthernet0/0/3", Description: "downlink-a"},
		{Interface: "GigabitEthernet0/0/4", Description: "downlink-b"},
	}

	cmds := ifaceIsolateCommands("dev-02", nil, links, "huawei")

	if len(cmds) != 1 {
		t.Fatalf("expected 1 DeviceCommand with no LAGs, got %d", len(cmds))
	}

	if !strings.Contains(cmds[0].Purpose, "物理下联") {
		t.Errorf("expected physical downlink command, got purpose: %s", cmds[0].Purpose)
	}
	if !strings.Contains(cmds[0].Purpose, "2") {
		t.Errorf("expected count 2 in purpose, got: %s", cmds[0].Purpose)
	}
}

func TestIfaceIsolateCommands_NoPhysical(t *testing.T) {
	lags := []LAGBundle{
		{Name: "Eth-Trunk10", Description: "core-uplink"},
	}

	cmds := ifaceIsolateCommands("dev-03", lags, nil, "huawei")

	if len(cmds) != 1 {
		t.Fatalf("expected 1 DeviceCommand with no physical links, got %d", len(cmds))
	}

	if !strings.Contains(cmds[0].Purpose, "LAG 上联") {
		t.Errorf("expected LAG uplink command, got purpose: %s", cmds[0].Purpose)
	}
	if !strings.Contains(cmds[0].Purpose, "1") {
		t.Errorf("expected count 1 in purpose, got: %s", cmds[0].Purpose)
	}
}

func TestIfaceRollbackCommands_ReverseOrder(t *testing.T) {
	lags := []LAGBundle{
		{Name: "Eth-Trunk1"},
	}
	links := []PhysicalLink{
		{Interface: "GigabitEthernet0/0/1"},
		{Interface: "GigabitEthernet0/0/2"},
	}

	cmds := ifaceRollbackCommands("dev-01", lags, links, "huawei")

	if len(cmds) != 2 {
		t.Fatalf("expected 2 DeviceCommands, got %d", len(cmds))
	}

	// Physical downlinks must come FIRST in rollback (reverse of isolate)
	if !strings.Contains(cmds[0].Purpose, "物理下联") {
		t.Errorf("first rollback command should be physical downlink, got: %s", cmds[0].Purpose)
	}

	// LAG uplinks must come SECOND in rollback
	if !strings.Contains(cmds[1].Purpose, "LAG 上联") {
		t.Errorf("second rollback command should be LAG uplink, got: %s", cmds[1].Purpose)
	}

	// Verify undo shutdown is used (not shutdown)
	physCmds := cmds[0].Commands
	if !containsLine(physCmds, "undo shutdown") {
		t.Error("expected 'undo shutdown' in physical rollback commands")
	}
	if containsLine(physCmds, "shutdown") {
		t.Error("rollback must not contain bare 'shutdown'")
	}

	lagCmds := cmds[1].Commands
	if !containsLine(lagCmds, "undo shutdown") {
		t.Error("expected 'undo shutdown' in LAG rollback commands")
	}

	// Verify purpose says 恢复 not 隔离
	if !strings.Contains(cmds[0].Purpose, "接口恢复") {
		t.Errorf("rollback purpose should say 接口恢复, got: %s", cmds[0].Purpose)
	}
	if !strings.Contains(cmds[1].Purpose, "接口恢复") {
		t.Errorf("rollback purpose should say 接口恢复, got: %s", cmds[1].Purpose)
	}
}

func TestIfaceIsolateCommands_WithDescription(t *testing.T) {
	links := []PhysicalLink{
		{Interface: "GigabitEthernet0/0/5", Description: "to-server-42"},
		{Interface: "GigabitEthernet0/0/6", Description: ""},
	}

	cmds := ifaceIsolateCommands("dev-04", nil, links, "huawei")

	if len(cmds) != 1 {
		t.Fatalf("expected 1 DeviceCommand, got %d", len(cmds))
	}

	physCmds := cmds[0].Commands

	// Interface with description should include the comment
	found := false
	for _, c := range physCmds {
		if strings.HasPrefix(c, "interface GigabitEthernet0/0/5") && strings.Contains(c, "# to-server-42") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'interface GigabitEthernet0/0/5  # to-server-42' in commands, got: %v", physCmds)
	}

	// Interface without description should have no comment
	for _, c := range physCmds {
		if strings.HasPrefix(c, "interface GigabitEthernet0/0/6") && strings.Contains(c, "#") {
			t.Errorf("interface without description should not have '#' comment, got: %q", c)
		}
	}
}

// containsLine checks whether lines contains an exact match of target.
func containsLine(lines []string, target string) bool {
	for _, l := range lines {
		if l == target {
			return true
		}
	}
	return false
}

// containsLinePrefix checks whether any line has target as a prefix.
func containsLinePrefix(lines []string, prefix string) bool {
	for _, l := range lines {
		if strings.HasPrefix(l, prefix) {
			return true
		}
	}
	return false
}
