package plan

import (
	"strings"
	"testing"
	"time"
)

// newTestPlan creates a minimal Plan with 1 phase and 1 link for use in render tests.
func newTestPlan() Plan {
	link := Link{
		LocalDevice:    "lc-01",
		LocalInterface: "GigabitEthernet0/0/0",
		LocalIP:        "10.0.0.1",
		PeerDevice:     "lc-02",
		PeerInterface:  "GigabitEthernet0/0/1",
		PeerIP:         "10.0.0.2",
		Protocols:      []string{"ospf"},
		Sources:        []string{"config"},
	}

	phase := Phase{
		Number:      0,
		Name:        "方案规划",
		Description: "收集目标设备当前运行状态",
		Steps: []DeviceCommand{
			{
				DeviceID: "lc-01",
				Vendor:   "huawei",
				Commands: []string{
					"display interface brief",
					"display ospf peer brief",
				},
				Purpose: "Collect baseline state from target device",
			},
		},
		Notes: []string{
			"⚠️ SPOF — 移除后 2 台设备受影响",
		},
	}

	return Plan{
		TargetDevice:   "lc-01",
		TargetHostname: "LC-01",
		TargetVendor:   "huawei",
		Links:          []Link{link},
		IsSPOF:         true,
		ImpactDevices:  []string{"lc-03", "lc-04"},
		Phases:         []Phase{phase},
		GeneratedAt:    time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC),
	}
}

func TestRenderText(t *testing.T) {
	p := newTestPlan()
	out := RenderText(p)

	checks := []struct {
		label string
		want  string
	}{
		{"target hostname", "LC-01"},
		{"phase name", "方案规划"},
		{"ospf command", "display ospf peer brief"},
		{"SPOF flag", "SPOF"},
		{"link local device", "lc-01"},
		{"link peer device", "lc-02"},
		{"phase header", "─── 阶段0:"},
	}

	for _, c := range checks {
		if !strings.Contains(out, c.want) {
			t.Errorf("RenderText missing %s: want substring %q\nOutput:\n%s", c.label, c.want, out)
		}
	}
}

func TestRenderMarkdown(t *testing.T) {
	p := newTestPlan()
	out := RenderMarkdown(p)

	checks := []struct {
		label string
		want  string
	}{
		{"markdown heading", "# "},
		{"target hostname", "LC-01"},
		{"code fence", "```"},
		{"SPOF flag", "SPOF"},
		{"phase section", "## 阶段0:"},
		{"link in table", "lc-01"},
	}

	for _, c := range checks {
		if !strings.Contains(out, c.want) {
			t.Errorf("RenderMarkdown missing %s: want substring %q\nOutput:\n%s", c.label, c.want, out)
		}
	}
}

func TestRenderText_NoSPOF(t *testing.T) {
	p := newTestPlan()
	p.IsSPOF = false
	p.ImpactDevices = nil
	// Remove the SPOF note from the phase to simulate a non-SPOF plan
	p.Phases[0].Notes = []string{"确认操作窗口"}

	out := RenderText(p)

	if !strings.Contains(out, "LC-01") {
		t.Error("RenderText should still contain target hostname")
	}

	// When IsSPOF is false, the impact assessment line should not appear
	if strings.Contains(out, "影响评估") {
		t.Error("RenderText should not contain impact assessment when IsSPOF=false")
	}
}

func TestRenderMarkdown_NoSPOF(t *testing.T) {
	p := newTestPlan()
	p.IsSPOF = false
	p.ImpactDevices = nil
	p.Phases[0].Notes = nil

	out := RenderMarkdown(p)

	if !strings.Contains(out, "LC-01") {
		t.Error("RenderMarkdown should still contain target hostname")
	}
	if strings.Contains(out, "影响评估") {
		t.Error("RenderMarkdown should not contain impact assessment when IsSPOF=false")
	}
}
