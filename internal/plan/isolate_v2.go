package plan

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// GenerateIsolationPlanV2 produces a 6-phase isolation Plan from a DeviceTopology.
func GenerateIsolationPlanV2(topo DeviceTopology) Plan {
	plan := Plan{
		TargetDevice:   topo.DeviceID,
		TargetHostname: topo.Hostname,
		TargetVendor:   topo.Vendor,
		IsSPOF:         topo.IsSPOF,
		ImpactDevices:  topo.ImpactDevices,
		GeneratedAt:    time.Now(),
	}

	plan.Phases = []Phase{
		phase0Collection(topo),
		phase1PreCheck(topo),
		phase2ProtocolIsolation(topo),
		phase3IfaceIsolation(topo),
		phase4PostCheck(topo),
		phase5Rollback(topo),
	}

	return plan
}

// phase0Collection builds Phase 0: 采集 — collect device state before any changes.
func phase0Collection(topo DeviceTopology) Phase {
	p := Phase{
		Number:      0,
		Name:        "采集",
		Description: "变更前采集设备状态，建立基线",
	}

	cmds := []string{
		"display interface brief",
		"display ip routing-table statistics",
	}

	for _, proto := range topo.Protocols {
		switch proto {
		case "bgp":
			cmds = append(cmds,
				"display bgp peer",
				"display bgp routing-table statistics",
			)
		case "ospf":
			if strings.ToLower(topo.Vendor) == "h3c" {
				cmds = append(cmds, "display ospf peer")
			} else {
				cmds = append(cmds, "display ospf peer brief")
			}
		case "isis":
			cmds = append(cmds, "display isis peer")
		case "ldp":
			cmds = append(cmds, "display mpls ldp session")
		}
	}

	if len(topo.LAGs) > 0 {
		cmds = append(cmds, "display link-aggregation verbose")
	}

	cmds = append(cmds,
		"display current-configuration",
		"display version",
	)

	p.Steps = []DeviceCommand{
		{
			DeviceID:   topo.DeviceID,
			DeviceHost: topo.Hostname,
			Vendor:     topo.Vendor,
			Commands:   cmds,
			Purpose:    "采集设备当前状态",
		},
	}

	if topo.IsSPOF {
		p.Notes = append(p.Notes,
			"⚠️  [SPOF] 该设备是单点故障节点，隔离将直接影响以下设备: "+strings.Join(topo.ImpactDevices, ", "),
		)
	}

	return p
}

// phase1PreCheck builds Phase 1: 预检查 — list expected peer counts and LAG state.
func phase1PreCheck(topo DeviceTopology) Phase {
	p := Phase{
		Number:      1,
		Name:        "预检查",
		Description: "确认变更前各 BGP 邻居组及 LAG 状态符合预期",
	}

	for _, pg := range topo.PeerGroups {
		p.Notes = append(p.Notes,
			fmt.Sprintf("[ ] BGP 组 %-20s  角色=%-10s  期望 peers=%d", pg.Name, string(pg.Role), len(pg.Peers)),
		)
	}

	for _, lag := range topo.LAGs {
		memberInfo := strings.Join(lag.Members, ", ")
		if memberInfo == "" {
			memberInfo = "(无成员)"
		}
		p.Notes = append(p.Notes,
			fmt.Sprintf("[ ] LAG %-15s  IP=%-18s  成员: %s", lag.Name, lag.IP+"/"+lag.Mask, memberInfo),
		)
	}

	return p
}

// phase2ProtocolIsolation builds Phase 2: 协议级隔离 — IGP overload, LDP disable, then BGP peer ignore.
func phase2ProtocolIsolation(topo DeviceTopology) Phase {
	p := Phase{
		Number:      2,
		Name:        "协议级隔离",
		Description: "按 IGP → LDP → BGP 顺序执行协议级隔离",
	}

	hasBGP := false
	for _, proto := range topo.Protocols {
		if proto == "bgp" {
			hasBGP = true
			break
		}
	}

	// 1. IGP isolation (ISIS overload / OSPF stub-router)
	for _, igp := range topo.IGPs {
		switch igp.Protocol {
		case "isis":
			isolate := isisIsolateStep(topo.DeviceID, igp, topo.Vendor)
			isolate.DeviceHost = topo.Hostname
			checkpoint := isisCheckpoint(topo.DeviceID, igp, topo.Vendor)
			checkpoint.DeviceHost = topo.Hostname
			p.Steps = append(p.Steps, isolate, checkpoint)
		case "ospf":
			isolate := ospfIsolateStep(topo.DeviceID, igp, topo.Vendor)
			isolate.DeviceHost = topo.Hostname
			checkpoint := ospfCheckpoint(topo.DeviceID, igp, topo.Vendor)
			checkpoint.DeviceHost = topo.Hostname
			p.Steps = append(p.Steps, isolate, checkpoint)
		}
	}

	// 2. Wait for IGP convergence
	if len(topo.IGPs) > 0 {
		p.Steps = append(p.Steps, DeviceCommand{
			DeviceID:   topo.DeviceID,
			DeviceHost: topo.Hostname,
			Vendor:     topo.Vendor,
			Commands:   []string{"# 等待 IGP 收敛（建议至少 60 秒）"},
			Purpose:    "等待 IGP 路由收敛",
		})
	}

	// 3. LDP disable
	if topo.HasLDP && len(topo.LDPInterfaces) > 0 {
		ldpIsolate := ldpIsolateSteps(topo.DeviceID, topo.LDPInterfaces, topo.Vendor)
		ldpIsolate.DeviceHost = topo.Hostname
		ldpCheck := ldpCheckpoint(topo.DeviceID, topo.Vendor)
		ldpCheck.DeviceHost = topo.Hostname
		p.Steps = append(p.Steps, ldpIsolate, ldpCheck)
	}

	// 4. BGP (existing code)
	if !hasBGP || len(topo.PeerGroups) == 0 {
		if len(p.Steps) == 0 {
			p.Notes = append(p.Notes,
				"⚠️  未检测到 BGP 协议或无 peer 组，跳过协议级隔离。请直接执行接口级硬切，风险较高，需人工确认。",
			)
		}
		return p
	}

	sorted := sortPeerGroups(topo.PeerGroups)

	for _, pg := range sorted {
		isolate := bgpIsolateStep(topo.DeviceID, topo.LocalAS, pg, topo.Vendor)
		isolate.DeviceHost = topo.Hostname
		checkpoint := bgpCheckpoint(topo.DeviceID, pg, topo.Vendor)
		checkpoint.DeviceHost = topo.Hostname

		p.Steps = append(p.Steps, isolate, checkpoint)
	}

	return p
}

// phase3IfaceIsolation builds Phase 3: 接口级隔离 — shut LAGs then physical links.
func phase3IfaceIsolation(topo DeviceTopology) Phase {
	p := Phase{
		Number:      3,
		Name:        "接口级隔离",
		Description: "关闭 LAG 上联接口，再关闭物理下联接口",
	}

	cmds := ifaceIsolateCommands(topo.DeviceID, topo.LAGs, topo.PhysicalLinks, topo.Vendor)
	for i := range cmds {
		cmds[i].DeviceHost = topo.Hostname
	}
	p.Steps = append(p.Steps, cmds...)

	// Verification step
	p.Steps = append(p.Steps, DeviceCommand{
		DeviceID:   topo.DeviceID,
		DeviceHost: topo.Hostname,
		Vendor:     topo.Vendor,
		Commands: []string{
			"display interface brief",
			"display ip routing-table statistics",
		},
		Purpose: "验证接口隔离后设备状态",
	})

	return p
}

// phase4PostCheck builds Phase 4: 变更后检查 — verify device is fully disconnected.
func phase4PostCheck(topo DeviceTopology) Phase {
	p := Phase{
		Number:      4,
		Name:        "变更后检查",
		Description: "确认设备已完全隔离，各协议邻居已中断",
	}

	cmds := []string{
		"display interface brief",
		"display ip routing-table statistics",
	}

	for _, proto := range topo.Protocols {
		switch proto {
		case "bgp":
			cmds = append(cmds,
				"display bgp peer",
				"display bgp peer | include Established",
			)
		case "ospf":
			if strings.ToLower(topo.Vendor) == "h3c" {
				cmds = append(cmds, "display ospf peer")
			} else {
				cmds = append(cmds, "display ospf peer brief")
			}
		case "isis":
			cmds = append(cmds, "display isis peer")
		case "ldp":
			cmds = append(cmds, "display mpls ldp session")
		}
	}

	p.Steps = []DeviceCommand{
		{
			DeviceID:   topo.DeviceID,
			DeviceHost: topo.Hostname,
			Vendor:     topo.Vendor,
			Commands:   cmds,
			Purpose:    "确认设备已完全隔离",
		},
	}

	return p
}

// phase5Rollback builds Phase 5: 回退方案 — reverse BGP (management→uplink→downlink), then LDP, then IGP, then interfaces.
func phase5Rollback(topo DeviceTopology) Phase {
	p := Phase{
		Number:      5,
		Name:        "回退方案",
		Description: "按 management → uplink → downlink 顺序撤销 BGP 隔离，再恢复 LDP/IGP，最后恢复接口",
	}

	hasBGP := false
	for _, proto := range topo.Protocols {
		if proto == "bgp" {
			hasBGP = true
			break
		}
	}

	if hasBGP && len(topo.PeerGroups) > 0 {
		// Reverse of phase 2: management first, then uplink, then downlink
		sorted := sortPeerGroups(topo.PeerGroups)
		// Reverse the sorted slice
		for left, right := 0, len(sorted)-1; left < right; left, right = left+1, right-1 {
			sorted[left], sorted[right] = sorted[right], sorted[left]
		}

		for _, pg := range sorted {
			rollback := bgpRollbackStep(topo.DeviceID, topo.LocalAS, pg, topo.Vendor)
			rollback.DeviceHost = topo.Hostname
			p.Steps = append(p.Steps, rollback)
		}
	}

	// LDP restore
	if topo.HasLDP && len(topo.LDPInterfaces) > 0 {
		ldpRestore := ldpRollbackSteps(topo.DeviceID, topo.LDPInterfaces, topo.Vendor)
		ldpRestore.DeviceHost = topo.Hostname
		p.Steps = append(p.Steps, ldpRestore)
	}

	// IGP restore (reverse order)
	for i := len(topo.IGPs) - 1; i >= 0; i-- {
		igp := topo.IGPs[i]
		switch igp.Protocol {
		case "isis":
			rollback := isisRollbackStep(topo.DeviceID, igp, topo.Vendor)
			rollback.DeviceHost = topo.Hostname
			p.Steps = append(p.Steps, rollback)
		case "ospf":
			rollback := ospfRollbackStep(topo.DeviceID, igp, topo.Vendor)
			rollback.DeviceHost = topo.Hostname
			p.Steps = append(p.Steps, rollback)
		}
	}

	// Interface rollback
	rollbackCmds := ifaceRollbackCommands(topo.DeviceID, topo.LAGs, topo.PhysicalLinks, topo.Vendor)
	for i := range rollbackCmds {
		rollbackCmds[i].DeviceHost = topo.Hostname
	}
	p.Steps = append(p.Steps, rollbackCmds...)

	return p
}

// sortPeerGroups returns a stable-sorted copy ordered by role: downlink < uplink < management.
func sortPeerGroups(groups []PeerGroup) []PeerGroup {
	sorted := make([]PeerGroup, len(groups))
	copy(sorted, groups)
	sort.SliceStable(sorted, func(i, j int) bool {
		return roleOrder(sorted[i].Role) < roleOrder(sorted[j].Role)
	})
	return sorted
}

// roleOrder maps a PeerGroupRole to a numeric priority for sorting.
func roleOrder(r PeerGroupRole) int {
	switch r {
	case RoleDownlink:
		return 0
	case RoleUplink:
		return 1
	case RoleManagement:
		return 2
	default:
		return 1
	}
}
