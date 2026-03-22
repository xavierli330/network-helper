package plan

import "time"

// PlanInput holds all inputs needed to build a device isolation plan.
type PlanInput struct {
	TargetDevice   string
	TargetHostname string
	TargetVendor   string
	Links          []Link
	IsSPOF         bool
	ImpactDevices  []string
}

// BuildIsolationPlan orchestrates the construction of a 6-phase isolation Plan
// for the target device, using the appropriate vendor CommandGenerator.
func BuildIsolationPlan(input PlanInput) Plan {
	gen := GeneratorForVendor(input.TargetVendor)

	phases := []Phase{
		buildPhase0(gen, input),
		buildPhase1(gen, input),
		buildPhase2(gen, input),
		buildPhase3(gen, input),
		buildPhase4(gen, input),
		buildPhase5(gen, input),
	}

	return Plan{
		TargetDevice:   input.TargetDevice,
		TargetHostname: input.TargetHostname,
		TargetVendor:   input.TargetVendor,
		Links:          input.Links,
		IsSPOF:         input.IsSPOF,
		ImpactDevices:  input.ImpactDevices,
		Phases:         phases,
		GeneratedAt:    time.Now(),
	}
}

// buildPhase0 constructs Phase 0 — 方案规划 (Plan preparation / state collection).
func buildPhase0(gen CommandGenerator, input PlanInput) Phase {
	notes := []string{
		"确认操作窗口，通知相关团队",
		"记录当前设备状态作为变更基线",
	}
	if input.IsSPOF {
		notes = append(notes, "⚠️ 该设备为单点故障节点 (SPOF)，移除后下游设备将失去连通性，请谨慎评估影响范围")
	}

	return Phase{
		Number:      0,
		Name:        "方案规划",
		Description: "收集目标设备及所有互联对端的当前运行状态，作为变更前基线数据",
		Steps:       gen.CollectionCommands(input.TargetDevice, input.Links),
		Notes:       notes,
	}
}

// buildPhase1 constructs Phase 1 — 预检查 (Pre-change verification).
func buildPhase1(gen CommandGenerator, input PlanInput) Phase {
	return Phase{
		Number:      1,
		Name:        "预检查",
		Description: "在执行变更前，确认目标设备所有协议邻居状态正常，不存在已有故障",
		Steps:       gen.PreCheckCommands(input.TargetDevice, input.Links),
		Notes: []string{
			"确认所有 OSPF 邻居状态为 Full",
			"确认所有 BGP 对等体状态为 Established",
			"确认所有互联接口状态为 Up",
			"如存在异常，需先处理现有故障再继续变更",
		},
	}
}

// buildPhase2 constructs Phase 2 — 协议级隔离 (Protocol-level isolation).
func buildPhase2(gen CommandGenerator, input PlanInput) Phase {
	phase2 := Phase{
		Number:      2,
		Name:        "协议级隔离",
		Description: "通过调整路由协议参数（提高 OSPF cost、抑制 BGP 对等体、禁用 LDP），使流量在接口下线前平滑切走",
		Steps:       gen.ProtocolIsolateCommands(input.TargetDevice, input.Links),
		Notes: []string{
			"执行后等待至少 60 秒，确保路由收敛完成",
			"观察对端设备是否已重新选路，确认流量已切走后再进入下一阶段",
		},
	}

	protocols := collectProtocols(input.Links)
	if len(protocols) == 0 {
		phase2.Notes = append(phase2.Notes, "⚠️ 未检测到协议信息（OSPF/BGP/LDP），阶段2将不会执行流量排干。阶段3的接口 shutdown 将是硬切！")
		phase2.Notes = append(phase2.Notes, "建议: 先采集设备配置（display current-configuration）以获取协议信息")
	}

	return phase2
}

// buildPhase3 constructs Phase 3 — 接口级隔离 (Interface-level isolation).
func buildPhase3(gen CommandGenerator, input PlanInput) Phase {
	return Phase{
		Number:      3,
		Name:        "接口级隔离",
		Description: "关闭目标设备所有互联接口，完成物理/逻辑层隔离",
		Steps:       gen.InterfaceIsolateCommands(input.TargetDevice, input.Links),
		Notes: []string{
			"接口 shutdown 后确认对端接口状态变为 Down",
		},
	}
}

// buildPhase4 constructs Phase 4 — 变更后检查 (Post-change verification).
func buildPhase4(gen CommandGenerator, input PlanInput) Phase {
	return Phase{
		Number:      4,
		Name:        "变更后检查",
		Description: "在所有对端设备上确认邻居已消失，路由表已收敛，业务流量正常",
		Steps:       gen.PostCheckCommands(input.TargetDevice, input.Links),
		Notes: []string{
			"确认对端设备 OSPF/BGP 邻居列表中目标设备已消失",
			"确认对端路由表中无黑洞路由",
			"联系业务团队确认流量无异常",
		},
	}
}

// buildPhase5 constructs Phase 5 — 回退方案 (Rollback procedure).
func buildPhase5(gen CommandGenerator, input PlanInput) Phase {
	return Phase{
		Number:      5,
		Name:        "回退方案",
		Description: "如变更后出现异常，按以下步骤恢复：先重新上线接口，再恢复协议配置",
		Steps:       gen.RollbackCommands(input.TargetDevice, input.Links),
		Notes: []string{
			"回退后等待至少 60 秒，确保协议邻居重新建立、路由收敛完成",
			"重新执行预检查命令，确认所有邻居状态恢复正常",
		},
	}
}
