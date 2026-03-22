package plan

import (
	"fmt"
	"time"
)

// GenerateUpgradePlan creates an 8-phase upgrade plan.
// It reuses isolation phases and adds upgrade-specific phases in the middle.
func GenerateUpgradePlan(topo DeviceTopology, params UpgradeParams) Plan {
	p := Plan{
		TargetDevice:   topo.DeviceID,
		TargetHostname: topo.Hostname,
		TargetVendor:   topo.Vendor,
		IsSPOF:         topo.IsSPOF,
		ImpactDevices:  topo.ImpactDevices,
		GeneratedAt:    time.Now(),
	}

	// Phase 0: collection — reuse phase0Collection, append version collection
	phase0 := phase0Collection(topo)
	phase0.Number = 0
	// display version is already appended in phase0Collection, so no duplication needed

	// Phase 1: pre-check — reuse phase1PreCheck, add upgrade-specific notes
	phase1 := phase1PreCheck(topo)
	phase1.Number = 1
	phase1.Notes = append(phase1.Notes,
		fmt.Sprintf("✓ 确认当前版本，目标版本: %s", params.TargetVersion),
		"✓ 确认磁盘空间充足（dir flash:/）",
		fmt.Sprintf("✓ 确认固件文件 %s 已上传到设备", params.FirmwareFile),
	)

	// Phase 2: protocol isolation
	phase2 := phase2ProtocolIsolation(topo)
	phase2.Number = 2

	// Phase 3: interface isolation
	phase3 := phase3IfaceIsolation(topo)
	phase3.Number = 3

	// Phase 4: upgrade execution
	phase4 := Phase{
		Number:      4,
		Name:        "升级执行",
		Description: fmt.Sprintf("执行软件升级至 %s，设备将重启", params.TargetVersion),
		Steps: []DeviceCommand{
			upgradeExecuteStep(topo.DeviceID, params, topo.Vendor),
		},
		Notes: []string{
			"⚠️ 升级过程中设备将重启，所有会话中断",
			"等待设备重启完成后再继续下一阶段",
			"如升级失败，设备可能自动回退到旧版本",
		},
	}

	// Phase 5: upgrade verification
	phase5 := Phase{
		Number:      5,
		Name:        "升级验证",
		Description: "确认设备已成功升级到目标版本，硬件状态正常",
		Steps: []DeviceCommand{
			upgradeVerifyStep(topo.DeviceID, params, topo.Vendor),
		},
		Notes: []string{
			fmt.Sprintf("确认 display version 输出包含 %s", params.TargetVersion),
			"确认无硬件告警",
			"如版本不正确，不要继续恢复流量——先排查升级失败原因",
		},
	}

	// Phase 6: traffic recovery — reuse phase5Rollback (reverse of isolation)
	phase6 := phase5Rollback(topo)
	phase6.Number = 6
	phase6.Name = "恢复流量"
	phase6.Description = "升级成功后，按逆序恢复协议和接口"

	// Phase 7: post-check
	phase7 := phase4PostCheck(topo)
	phase7.Number = 7

	p.Phases = []Phase{phase0, phase1, phase2, phase3, phase4, phase5, phase6, phase7}
	return p
}
