package plan

import "fmt"

// UpgradeParams holds the user-provided upgrade parameters.
type UpgradeParams struct {
	TargetVersion string // e.g. "V200R021C10SPC600"
	FirmwareFile  string // e.g. "NE40E-V800R021C10SPC600.cc"
}

// upgradeExecuteStep generates vendor-specific firmware upgrade commands.
func upgradeExecuteStep(deviceID string, params UpgradeParams, vendor string) DeviceCommand {
	var cmds []string
	switch vendor {
	case "huawei":
		cmds = []string{
			"system-view",
			fmt.Sprintf("startup system-software %s", params.FirmwareFile),
			"quit",
			"save",
			"# 确认保存: Y",
			"reboot",
			"# 确认重启: Y",
			"# 等待设备重启完成（预计 10-30 分钟，取决于设备型号）",
		}
	case "h3c":
		cmds = []string{
			fmt.Sprintf("boot-loader file flash:/%s slot 0 main", params.FirmwareFile),
			"# 确认: Y",
			"save force",
			"reboot",
			"# 确认重启: Y",
			"# 等待设备重启完成",
		}
	case "cisco":
		cmds = []string{
			fmt.Sprintf("install add source flash: %s", params.FirmwareFile),
			fmt.Sprintf("install activate %s", params.FirmwareFile),
			"install commit",
			"# 等待设备重启完成",
		}
	case "juniper":
		cmds = []string{
			fmt.Sprintf("request system software add /var/tmp/%s", params.FirmwareFile),
			"request system reboot",
			"# 确认: yes",
			"# 等待设备重启完成",
		}
	default:
		cmds = []string{
			fmt.Sprintf("# 请手动执行升级操作: 文件=%s 目标版本=%s", params.FirmwareFile, params.TargetVersion),
		}
	}
	return DeviceCommand{
		DeviceID: deviceID, Commands: cmds,
		Purpose: fmt.Sprintf("执行软件升级 — 目标版本: %s", params.TargetVersion),
	}
}

// upgradeVerifyStep generates post-upgrade verification commands.
func upgradeVerifyStep(deviceID string, params UpgradeParams, vendor string) DeviceCommand {
	var cmds []string
	switch vendor {
	case "huawei":
		cmds = []string{
			"display version",
			fmt.Sprintf("# 确认版本包含: %s", params.TargetVersion),
			"display device",
			"display alarm active",
		}
	case "h3c":
		cmds = []string{
			"display version",
			fmt.Sprintf("# 确认版本包含: %s", params.TargetVersion),
			"display device manuinfo",
		}
	case "cisco":
		cmds = []string{
			"show version",
			fmt.Sprintf("# 确认版本包含: %s", params.TargetVersion),
			"show platform",
			"admin show alarms brief",
		}
	case "juniper":
		cmds = []string{
			"show version",
			fmt.Sprintf("# 确认版本包含: %s", params.TargetVersion),
			"show chassis alarms",
			"show system alarms",
		}
	default:
		cmds = []string{
			"# 请手动验证版本",
		}
	}
	return DeviceCommand{
		DeviceID: deviceID, Commands: cmds,
		Purpose: fmt.Sprintf("升级验证 — 确认版本: %s", params.TargetVersion),
	}
}
