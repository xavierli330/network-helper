package plan

import (
	"fmt"
	"strings"
)

// isisIsolateStep builds a DeviceCommand that sets ISIS overload on a given process.
// This causes ISIS to advertise maximum metric, draining traffic before maintenance.
func isisIsolateStep(deviceID string, igp IGPInfo, vendor string) DeviceCommand {
	var cmds []string
	switch strings.ToLower(vendor) {
	case "cisco":
		cmds = []string{
			fmt.Sprintf("router isis %s", igp.ProcessID),
			"set-overload-bit",
		}
	default: // huawei, h3c
		cmds = []string{
			"system-view",
			fmt.Sprintf("isis %s", igp.ProcessID),
			"set-overload",
			"quit",
			"return",
		}
	}
	return DeviceCommand{
		DeviceID: deviceID,
		Vendor:   vendor,
		Commands: cmds,
		Purpose:  fmt.Sprintf("ISIS 隔离 — set-overload (进程 %s)", igp.ProcessID),
	}
}

// isisCheckpoint builds a read-only DeviceCommand to verify ISIS overload has taken effect.
func isisCheckpoint(deviceID string, igp IGPInfo, vendor string) DeviceCommand {
	var cmds []string
	switch strings.ToLower(vendor) {
	case "cisco":
		cmds = []string{"show isis neighbors"}
	default: // huawei, h3c
		cmds = []string{"display isis peer"}
	}
	return DeviceCommand{
		DeviceID: deviceID,
		Vendor:   vendor,
		Commands: cmds,
		Purpose:  fmt.Sprintf(">>> 检查点: ISIS 进程 %s overload 生效 <<<", igp.ProcessID),
	}
}

// isisRollbackStep builds a DeviceCommand that removes ISIS overload from a given process.
func isisRollbackStep(deviceID string, igp IGPInfo, vendor string) DeviceCommand {
	var cmds []string
	switch strings.ToLower(vendor) {
	case "cisco":
		cmds = []string{
			fmt.Sprintf("router isis %s", igp.ProcessID),
			"no set-overload-bit",
		}
	default: // huawei, h3c
		cmds = []string{
			"system-view",
			fmt.Sprintf("isis %s", igp.ProcessID),
			"undo set-overload",
			"quit",
			"return",
		}
	}
	return DeviceCommand{
		DeviceID: deviceID,
		Vendor:   vendor,
		Commands: cmds,
		Purpose:  fmt.Sprintf("ISIS 回退 — undo set-overload (进程 %s)", igp.ProcessID),
	}
}

// ospfIsolateStep builds a DeviceCommand that configures OSPF stub-router (max-metric) on a given process.
func ospfIsolateStep(deviceID string, igp IGPInfo, vendor string) DeviceCommand {
	var cmds []string
	switch strings.ToLower(vendor) {
	case "cisco":
		cmds = []string{
			fmt.Sprintf("router ospf %s", igp.ProcessID),
			"max-metric router-lsa",
		}
	default: // huawei, h3c
		cmds = []string{
			"system-view",
			fmt.Sprintf("ospf %s", igp.ProcessID),
			"stub-router",
			"quit",
			"return",
		}
	}
	return DeviceCommand{
		DeviceID: deviceID,
		Vendor:   vendor,
		Commands: cmds,
		Purpose:  fmt.Sprintf("OSPF 隔离 — stub-router (进程 %s)", igp.ProcessID),
	}
}

// ospfCheckpoint builds a read-only DeviceCommand to verify OSPF stub-router has taken effect.
func ospfCheckpoint(deviceID string, igp IGPInfo, vendor string) DeviceCommand {
	var cmds []string
	switch strings.ToLower(vendor) {
	case "cisco":
		cmds = []string{"show ip ospf neighbor"}
	case "h3c":
		cmds = []string{"display ospf peer"}
	default: // huawei
		cmds = []string{"display ospf peer brief"}
	}
	return DeviceCommand{
		DeviceID: deviceID,
		Vendor:   vendor,
		Commands: cmds,
		Purpose:  fmt.Sprintf(">>> 检查点: OSPF 进程 %s stub-router 生效 <<<", igp.ProcessID),
	}
}

// ospfRollbackStep builds a DeviceCommand that removes OSPF stub-router from a given process.
func ospfRollbackStep(deviceID string, igp IGPInfo, vendor string) DeviceCommand {
	var cmds []string
	switch strings.ToLower(vendor) {
	case "cisco":
		cmds = []string{
			fmt.Sprintf("router ospf %s", igp.ProcessID),
			"no max-metric router-lsa",
		}
	default: // huawei, h3c
		cmds = []string{
			"system-view",
			fmt.Sprintf("ospf %s", igp.ProcessID),
			"undo stub-router",
			"quit",
			"return",
		}
	}
	return DeviceCommand{
		DeviceID: deviceID,
		Vendor:   vendor,
		Commands: cmds,
		Purpose:  fmt.Sprintf("OSPF 回退 — undo stub-router (进程 %s)", igp.ProcessID),
	}
}

// ldpIsolateSteps builds a DeviceCommand that disables MPLS LDP on each of the given interfaces.
func ldpIsolateSteps(deviceID string, interfaces []string, vendor string) DeviceCommand {
	cmds := []string{"system-view"}
	for _, iface := range interfaces {
		cmds = append(cmds,
			fmt.Sprintf("interface %s", iface),
			"undo mpls ldp",
			"quit",
		)
	}
	cmds = append(cmds, "return")
	return DeviceCommand{
		DeviceID: deviceID,
		Vendor:   vendor,
		Commands: cmds,
		Purpose:  fmt.Sprintf("LDP 隔离 — 禁用 %d 个接口的 MPLS LDP", len(interfaces)),
	}
}

// ldpCheckpoint builds a read-only DeviceCommand to verify LDP sessions have been cleared.
func ldpCheckpoint(deviceID string, vendor string) DeviceCommand {
	return DeviceCommand{
		DeviceID: deviceID,
		Vendor:   vendor,
		Commands: []string{"display mpls ldp session"},
		Purpose:  ">>> 检查点: LDP 会话已中断 <<<",
	}
}

// ldpRollbackSteps builds a DeviceCommand that re-enables MPLS LDP on each of the given interfaces.
func ldpRollbackSteps(deviceID string, interfaces []string, vendor string) DeviceCommand {
	cmds := []string{"system-view"}
	for _, iface := range interfaces {
		cmds = append(cmds,
			fmt.Sprintf("interface %s", iface),
			"mpls ldp",
			"quit",
		)
	}
	cmds = append(cmds, "return")
	return DeviceCommand{
		DeviceID: deviceID,
		Vendor:   vendor,
		Commands: cmds,
		Purpose:  fmt.Sprintf("LDP 回退 — 恢复 %d 个接口的 MPLS LDP", len(interfaces)),
	}
}
