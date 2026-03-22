package plan

import "fmt"

// ifaceIsolateCommands builds DeviceCommands to shut down LAG uplinks first,
// then physical downlinks. Commands with no entries are omitted.
func ifaceIsolateCommands(deviceID string, lags []LAGBundle, links []PhysicalLink, vendor string) []DeviceCommand {
	var cmds []DeviceCommand

	// First: shut LAG uplinks
	if len(lags) > 0 {
		lines := []string{"system-view"}
		for _, lag := range lags {
			lines = append(lines, fmt.Sprintf("interface %s", lag.Name))
			lines = append(lines, "shutdown")
			lines = append(lines, "quit")
		}
		lines = append(lines, "return")

		cmds = append(cmds, DeviceCommand{
			DeviceID: deviceID,
			Vendor:   vendor,
			Commands: lines,
			Purpose:  fmt.Sprintf("接口隔离 — 关闭 %d 个 LAG 上联", len(lags)),
		})
	}

	// Second: shut physical downlinks
	if len(links) > 0 {
		lines := []string{"system-view"}
		for _, link := range links {
			ifLine := fmt.Sprintf("interface %s", link.Interface)
			if link.Description != "" {
				ifLine += fmt.Sprintf("  # %s", link.Description)
			}
			lines = append(lines, ifLine)
			lines = append(lines, "shutdown")
			lines = append(lines, "quit")
		}
		lines = append(lines, "return")

		cmds = append(cmds, DeviceCommand{
			DeviceID: deviceID,
			Vendor:   vendor,
			Commands: lines,
			Purpose:  fmt.Sprintf("接口隔离 — 关闭 %d 个物理下联", len(links)),
		})
	}

	return cmds
}

// ifaceRollbackCommands builds DeviceCommands to restore interfaces in reverse
// order: physical downlinks first (undo shutdown), then LAG uplinks.
func ifaceRollbackCommands(deviceID string, lags []LAGBundle, links []PhysicalLink, vendor string) []DeviceCommand {
	var cmds []DeviceCommand

	// First: restore physical downlinks
	if len(links) > 0 {
		lines := []string{"system-view"}
		for _, link := range links {
			lines = append(lines, fmt.Sprintf("interface %s", link.Interface))
			lines = append(lines, "undo shutdown")
			lines = append(lines, "quit")
		}
		lines = append(lines, "return")

		cmds = append(cmds, DeviceCommand{
			DeviceID: deviceID,
			Vendor:   vendor,
			Commands: lines,
			Purpose:  fmt.Sprintf("接口恢复 — 开启 %d 个物理下联", len(links)),
		})
	}

	// Second: restore LAG uplinks
	if len(lags) > 0 {
		lines := []string{"system-view"}
		for _, lag := range lags {
			lines = append(lines, fmt.Sprintf("interface %s", lag.Name))
			lines = append(lines, "undo shutdown")
			lines = append(lines, "quit")
		}
		lines = append(lines, "return")

		cmds = append(cmds, DeviceCommand{
			DeviceID: deviceID,
			Vendor:   vendor,
			Commands: lines,
			Purpose:  fmt.Sprintf("接口恢复 — 开启 %d 个 LAG 上联", len(lags)),
		})
	}

	return cmds
}
