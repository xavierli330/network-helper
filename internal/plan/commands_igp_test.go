package plan

import (
	"strings"
	"testing"
)

func TestISISIsolateStep_Huawei(t *testing.T) {
	igp := IGPInfo{Protocol: "isis", ProcessID: "1", Interfaces: []string{"Eth-Trunk1"}}
	cmd := isisIsolateStep("dev-01", igp, "huawei")

	joined := strings.Join(cmd.Commands, "\n")
	if !strings.Contains(joined, "isis 1") {
		t.Errorf("expected 'isis 1' in commands, got:\n%s", joined)
	}
	if !strings.Contains(joined, "set-overload") {
		t.Errorf("expected 'set-overload' in commands, got:\n%s", joined)
	}
	if !strings.Contains(cmd.Purpose, "进程 1") {
		t.Errorf("expected process ID in purpose, got: %q", cmd.Purpose)
	}
}

func TestISISIsolateStep_Cisco(t *testing.T) {
	igp := IGPInfo{Protocol: "isis", ProcessID: "CORE", Interfaces: []string{"GigabitEthernet0/0"}}
	cmd := isisIsolateStep("dev-02", igp, "cisco")

	joined := strings.Join(cmd.Commands, "\n")
	if !strings.Contains(joined, "router isis CORE") {
		t.Errorf("expected 'router isis CORE' in commands, got:\n%s", joined)
	}
	if !strings.Contains(joined, "set-overload-bit") {
		t.Errorf("expected 'set-overload-bit' in commands, got:\n%s", joined)
	}
}

func TestISISRollbackStep(t *testing.T) {
	igp := IGPInfo{Protocol: "isis", ProcessID: "1"}
	cmd := isisRollbackStep("dev-01", igp, "huawei")

	joined := strings.Join(cmd.Commands, "\n")
	if !strings.Contains(joined, "undo set-overload") {
		t.Errorf("expected 'undo set-overload' in rollback commands, got:\n%s", joined)
	}
	if !strings.Contains(joined, "isis 1") {
		t.Errorf("expected 'isis 1' in rollback commands, got:\n%s", joined)
	}
}

func TestISISRollbackStep_Cisco(t *testing.T) {
	igp := IGPInfo{Protocol: "isis", ProcessID: "CORE"}
	cmd := isisRollbackStep("dev-02", igp, "cisco")

	joined := strings.Join(cmd.Commands, "\n")
	if !strings.Contains(joined, "no set-overload-bit") {
		t.Errorf("expected 'no set-overload-bit' in cisco rollback, got:\n%s", joined)
	}
}

func TestISISCheckpoint_Huawei(t *testing.T) {
	igp := IGPInfo{Protocol: "isis", ProcessID: "1"}
	cmd := isisCheckpoint("dev-01", igp, "huawei")

	joined := strings.Join(cmd.Commands, "\n")
	if !strings.Contains(joined, "display isis peer") {
		t.Errorf("expected 'display isis peer' in checkpoint, got:\n%s", joined)
	}
}

func TestISISCheckpoint_Cisco(t *testing.T) {
	igp := IGPInfo{Protocol: "isis", ProcessID: "CORE"}
	cmd := isisCheckpoint("dev-02", igp, "cisco")

	joined := strings.Join(cmd.Commands, "\n")
	if !strings.Contains(joined, "show isis neighbors") {
		t.Errorf("expected 'show isis neighbors' in cisco checkpoint, got:\n%s", joined)
	}
}

func TestOSPFIsolateStep_Huawei(t *testing.T) {
	igp := IGPInfo{Protocol: "ospf", ProcessID: "100"}
	cmd := ospfIsolateStep("dev-01", igp, "huawei")

	joined := strings.Join(cmd.Commands, "\n")
	if !strings.Contains(joined, "ospf 100") {
		t.Errorf("expected 'ospf 100' in commands, got:\n%s", joined)
	}
	if !strings.Contains(joined, "stub-router") {
		t.Errorf("expected 'stub-router' in commands, got:\n%s", joined)
	}
	if !strings.Contains(cmd.Purpose, "进程 100") {
		t.Errorf("expected process ID in purpose, got: %q", cmd.Purpose)
	}
}

func TestOSPFIsolateStep_H3C(t *testing.T) {
	igp := IGPInfo{Protocol: "ospf", ProcessID: "1"}
	// Checkpoint for H3C should use "display ospf peer" (not brief)
	cmd := ospfCheckpoint("dev-h3c", igp, "h3c")

	joined := strings.Join(cmd.Commands, "\n")
	if !strings.Contains(joined, "display ospf peer") {
		t.Errorf("expected 'display ospf peer' for H3C, got:\n%s", joined)
	}
	if strings.Contains(joined, "brief") {
		t.Errorf("H3C ospf checkpoint should NOT use 'brief', got:\n%s", joined)
	}
}

func TestOSPFCheckpoint_Huawei_UsesBrief(t *testing.T) {
	igp := IGPInfo{Protocol: "ospf", ProcessID: "100"}
	cmd := ospfCheckpoint("dev-01", igp, "huawei")

	joined := strings.Join(cmd.Commands, "\n")
	if !strings.Contains(joined, "display ospf peer brief") {
		t.Errorf("expected 'display ospf peer brief' for Huawei, got:\n%s", joined)
	}
}

func TestOSPFRollbackStep_Huawei(t *testing.T) {
	igp := IGPInfo{Protocol: "ospf", ProcessID: "100"}
	cmd := ospfRollbackStep("dev-01", igp, "huawei")

	joined := strings.Join(cmd.Commands, "\n")
	if !strings.Contains(joined, "undo stub-router") {
		t.Errorf("expected 'undo stub-router' in rollback, got:\n%s", joined)
	}
}

func TestOSPFRollbackStep_Cisco(t *testing.T) {
	igp := IGPInfo{Protocol: "ospf", ProcessID: "1"}
	cmd := ospfRollbackStep("dev-02", igp, "cisco")

	joined := strings.Join(cmd.Commands, "\n")
	if !strings.Contains(joined, "no max-metric router-lsa") {
		t.Errorf("expected 'no max-metric router-lsa' in cisco rollback, got:\n%s", joined)
	}
}

func TestLDPIsolateSteps(t *testing.T) {
	ifaces := []string{"Eth-Trunk1", "GigabitEthernet0/0/1"}
	cmd := ldpIsolateSteps("dev-01", ifaces, "huawei")

	joined := strings.Join(cmd.Commands, "\n")
	for _, iface := range ifaces {
		if !strings.Contains(joined, "interface "+iface) {
			t.Errorf("expected 'interface %s' in LDP isolate commands, got:\n%s", iface, joined)
		}
		if !strings.Contains(joined, "undo mpls ldp") {
			t.Errorf("expected 'undo mpls ldp' in LDP isolate commands, got:\n%s", joined)
		}
	}
	if !strings.Contains(cmd.Purpose, "2") {
		t.Errorf("expected interface count '2' in purpose, got: %q", cmd.Purpose)
	}
}

func TestLDPRollbackSteps(t *testing.T) {
	ifaces := []string{"Eth-Trunk1", "GigabitEthernet0/0/1"}
	cmd := ldpRollbackSteps("dev-01", ifaces, "huawei")

	joined := strings.Join(cmd.Commands, "\n")
	for _, iface := range ifaces {
		if !strings.Contains(joined, "interface "+iface) {
			t.Errorf("expected 'interface %s' in LDP rollback commands, got:\n%s", iface, joined)
		}
	}
	// mpls ldp should be restored (without "undo" prefix)
	if !strings.Contains(joined, "\nmpls ldp") {
		t.Errorf("expected 'mpls ldp' (restore) in rollback commands, got:\n%s", joined)
	}
	// Should NOT have "undo mpls ldp"
	if strings.Contains(joined, "undo mpls ldp") {
		t.Errorf("rollback should NOT contain 'undo mpls ldp', got:\n%s", joined)
	}
}

func TestLDPCheckpoint(t *testing.T) {
	cmd := ldpCheckpoint("dev-01", "huawei")

	joined := strings.Join(cmd.Commands, "\n")
	if !strings.Contains(joined, "display mpls ldp session") {
		t.Errorf("expected 'display mpls ldp session' in LDP checkpoint, got:\n%s", joined)
	}
}
