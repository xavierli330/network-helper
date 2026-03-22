package plan

import (
	"fmt"
	"strings"
	"time"
)

// CutoverParams holds the old and new interface names for the link migration.
type CutoverParams struct {
	OldInterface string // e.g. "Eth-Trunk1"
	NewInterface string // e.g. "Eth-Trunk200"
}

// GenerateCutoverPlan produces a 7-phase link-migration Plan from a DeviceTopology
// and a CutoverParams describing which interface to move traffic away from and which
// interface to move it onto.
func GenerateCutoverPlan(topo DeviceTopology, params CutoverParams) Plan {
	// Find the old interface in the topology so later phases can copy its config.
	var oldLink *PhysicalLink
	for i, l := range topo.PhysicalLinks {
		if strings.EqualFold(l.Interface, params.OldInterface) {
			oldLink = &topo.PhysicalLinks[i]
			break
		}
	}
	var oldLAG *LAGBundle
	for i, l := range topo.LAGs {
		if strings.EqualFold(l.Name, params.OldInterface) {
			oldLAG = &topo.LAGs[i]
			break
		}
	}

	p := Plan{
		TargetDevice:   topo.DeviceID,
		TargetHostname: topo.Hostname,
		TargetVendor:   topo.Vendor,
		GeneratedAt:    time.Now(),
		IsSPOF:         topo.IsSPOF,
		ImpactDevices:  topo.ImpactDevices,
	}

	p.Phases = []Phase{
		cutoverPhase0Collect(topo, params),
		cutoverPhase1PreCheck(topo, params, oldLink, oldLAG),
		cutoverPhase2ConfigNew(topo, params, oldLink, oldLAG),
		cutoverPhase3VerifyNew(topo, params),
		cutoverPhase4SwitchTraffic(topo, params),
		cutoverPhase5ShutOld(topo, params),
		cutoverPhase6PostCheck(topo, params),
	}
	return p
}

// cutoverPhase0Collect — Phase 0: 采集
// Displays the current state of both the old and new interfaces and shows the
// running-config for the old interface so it can be used as a reference.
func cutoverPhase0Collect(topo DeviceTopology, params CutoverParams) Phase {
	p := Phase{
		Number:      0,
		Name:        "采集",
		Description: "变更前采集旧口与新口状态，建立基线",
	}

	cmds := []string{
		fmt.Sprintf("display interface %s", params.OldInterface),
		fmt.Sprintf("display interface %s", params.NewInterface),
		fmt.Sprintf("display current-configuration interface %s", params.OldInterface),
		"display ip routing-table statistics",
	}

	for _, proto := range topo.Protocols {
		switch proto {
		case "isis":
			cmds = append(cmds, "display isis peer")
		case "ospf":
			if strings.ToLower(topo.Vendor) == "h3c" {
				cmds = append(cmds, "display ospf peer")
			} else {
				cmds = append(cmds, "display ospf peer brief")
			}
		case "ldp":
			cmds = append(cmds, "display mpls ldp session")
		case "bgp":
			cmds = append(cmds, "display bgp peer")
		}
	}

	p.Steps = []DeviceCommand{
		{
			DeviceID:   topo.DeviceID,
			DeviceHost: topo.Hostname,
			Vendor:     topo.Vendor,
			Commands:   cmds,
			Purpose:    fmt.Sprintf("采集 %s（旧口）和 %s（新口）当前状态", params.OldInterface, params.NewInterface),
		},
	}

	if topo.IsSPOF {
		p.Notes = append(p.Notes,
			"⚠️  [SPOF] 该设备是单点故障节点，切流将直接影响以下设备: "+strings.Join(topo.ImpactDevices, ", "),
		)
	}

	return p
}

// cutoverPhase1PreCheck — Phase 1: 预检查
// Verifies that the old interface is operationally Up with traffic, and that
// the new interface is physically available (Up or at least present).
func cutoverPhase1PreCheck(topo DeviceTopology, params CutoverParams, oldLink *PhysicalLink, oldLAG *LAGBundle) Phase {
	p := Phase{
		Number:      1,
		Name:        "预检查",
		Description: "确认旧口有流量、新口物理可用，满足切换前提条件",
	}

	p.Notes = append(p.Notes,
		fmt.Sprintf("[ ] 旧口 %-20s  当前状态应为 Up，且有流量通过", params.OldInterface),
		fmt.Sprintf("[ ] 新口 %-20s  物理状态应为 Up（或至少 Present/Down-but-reachable）", params.NewInterface),
	)

	// Report what we found for the old interface in the topology database.
	if oldLink != nil {
		p.Notes = append(p.Notes,
			fmt.Sprintf("[ ] 旧口拓扑信息  IP=%s/%s  描述=%q", oldLink.IP, oldLink.Mask, oldLink.Description),
		)
	} else if oldLAG != nil {
		memberStr := strings.Join(oldLAG.Members, ", ")
		if memberStr == "" {
			memberStr = "(无成员)"
		}
		p.Notes = append(p.Notes,
			fmt.Sprintf("[ ] 旧口拓扑信息  LAG IP=%s/%s  成员=%s  描述=%q",
				oldLAG.IP, oldLAG.Mask, memberStr, oldLAG.Description),
		)
	} else {
		p.Notes = append(p.Notes,
			fmt.Sprintf("⚠️  数据库中未找到接口 %s 的拓扑信息，请手工确认接口配置", params.OldInterface),
		)
	}

	// IGP protocol notes
	for _, igp := range topo.IGPs {
		for _, iface := range igp.Interfaces {
			if strings.EqualFold(iface, params.OldInterface) {
				p.Notes = append(p.Notes,
					fmt.Sprintf("[ ] 旧口已加入 %s 进程 %s，新口配置时需同步添加", igp.Protocol, igp.ProcessID),
				)
			}
		}
	}

	// LDP note
	if topo.HasLDP {
		for _, iface := range topo.LDPInterfaces {
			if strings.EqualFold(iface, params.OldInterface) {
				p.Notes = append(p.Notes,
					"[ ] 旧口启用了 MPLS LDP，新口配置时需同步开启",
				)
				break
			}
		}
	}

	p.Steps = []DeviceCommand{
		{
			DeviceID:   topo.DeviceID,
			DeviceHost: topo.Hostname,
			Vendor:     topo.Vendor,
			Commands: []string{
				fmt.Sprintf("display interface %s", params.OldInterface),
				fmt.Sprintf("display interface %s", params.NewInterface),
			},
			Purpose: "预检查: 确认旧口/新口接口状态",
		},
	}

	return p
}

// cutoverPhase2ConfigNew — Phase 2: 配置新接口
// Generates CLI commands to configure the new interface with the same IP,
// description, and protocol settings as the old interface.
func cutoverPhase2ConfigNew(topo DeviceTopology, params CutoverParams, oldLink *PhysicalLink, oldLAG *LAGBundle) Phase {
	p := Phase{
		Number:      2,
		Name:        "配置新接口",
		Description: fmt.Sprintf("将 %s 的 IP / 协议配置迁移到 %s", params.OldInterface, params.NewInterface),
	}

	// Determine IP, mask, description from old interface info.
	var ip, mask, desc string
	if oldLink != nil {
		ip = oldLink.IP
		mask = oldLink.Mask
		desc = oldLink.Description
	} else if oldLAG != nil {
		ip = oldLAG.IP
		mask = oldLAG.Mask
		desc = oldLAG.Description
	}

	// --- Determine which IGP processes are on the old interface ---
	type igpOnIface struct {
		proto     string
		processID string
	}
	var igpsOnOld []igpOnIface
	for _, igp := range topo.IGPs {
		for _, iface := range igp.Interfaces {
			if strings.EqualFold(iface, params.OldInterface) {
				igpsOnOld = append(igpsOnOld, igpOnIface{igp.Protocol, igp.ProcessID})
				break
			}
		}
	}

	// Check if LDP is on old interface
	ldpOnOld := false
	if topo.HasLDP {
		for _, iface := range topo.LDPInterfaces {
			if strings.EqualFold(iface, params.OldInterface) {
				ldpOnOld = true
				break
			}
		}
	}

	vendor := strings.ToLower(topo.Vendor)

	var cmds []string

	if ip == "" && desc == "" && len(igpsOnOld) == 0 && !ldpOnOld {
		// No config info available — produce a comment-only placeholder block.
		cmds = []string{
			fmt.Sprintf("# 未在数据库中找到接口 %s 的配置信息", params.OldInterface),
			"# 请手工执行以下操作（以 Huawei/H3C 为例）：",
			"system-view",
			fmt.Sprintf("interface %s", params.NewInterface),
			fmt.Sprintf("  # description <复制自 %s>", params.OldInterface),
			fmt.Sprintf("  # ip address <IP> <MASK>  （复制自 %s）", params.OldInterface),
			"  # isis enable <进程号>  （如适用）",
			"  # ospf enable <进程号> area <区域>  （如适用）",
			"  # mpls ldp  （如适用）",
			"quit",
			"return",
		}
	} else {
		switch vendor {
		case "cisco":
			cmds = append(cmds,
				fmt.Sprintf("interface %s", params.NewInterface),
			)
			if desc != "" {
				cmds = append(cmds, fmt.Sprintf(" description %s", desc))
			}
			if ip != "" {
				// Convert prefix-length mask to dotted-decimal for IOS if needed
				cmds = append(cmds, fmt.Sprintf(" ip address %s %s", ip, prefixToMask(mask)))
			} else {
				cmds = append(cmds, fmt.Sprintf(" # ip address <复制自 %s>", params.OldInterface))
			}
			for _, igp := range igpsOnOld {
				switch igp.proto {
				case "isis":
					cmds = append(cmds, fmt.Sprintf(" ip router isis %s", igp.processID))
				case "ospf":
					cmds = append(cmds, fmt.Sprintf(" ip ospf %s area 0", igp.processID))
				}
			}
			if ldpOnOld {
				cmds = append(cmds, " mpls ip")
			}
		default: // huawei, h3c
			cmds = append(cmds,
				"system-view",
				fmt.Sprintf("interface %s", params.NewInterface),
			)
			if desc != "" {
				cmds = append(cmds, fmt.Sprintf(" description %s", desc))
			}
			if ip != "" {
				cmds = append(cmds, fmt.Sprintf(" ip address %s %s", ip, mask))
			} else {
				cmds = append(cmds, fmt.Sprintf(" # ip address <复制自 %s>", params.OldInterface))
			}
			for _, igp := range igpsOnOld {
				switch igp.proto {
				case "isis":
					cmds = append(cmds, fmt.Sprintf(" isis enable %s", igp.processID))
				case "ospf":
					cmds = append(cmds, fmt.Sprintf(" ospf enable %s area 0", igp.processID))
				}
			}
			if ldpOnOld {
				cmds = append(cmds, " mpls ldp")
			}
			cmds = append(cmds, "quit", "return")
		}
	}

	p.Steps = []DeviceCommand{
		{
			DeviceID:   topo.DeviceID,
			DeviceHost: topo.Hostname,
			Vendor:     topo.Vendor,
			Commands:   cmds,
			Purpose:    fmt.Sprintf("配置新接口 %s（迁移自 %s）", params.NewInterface, params.OldInterface),
		},
	}

	p.Notes = append(p.Notes,
		"⚠️  请确认新口 IP 与旧口相同且对端已接受，避免 IP 冲突",
		"完成后执行 Phase 3 验证新口协议正常建立，再继续 Phase 4 切流",
	)

	return p
}

// cutoverPhase3VerifyNew — Phase 3: 验证新口
// Verifies that the new interface is operationally Up, can ping its peer, and
// that any IGP/LDP sessions have re-established over the new interface.
func cutoverPhase3VerifyNew(topo DeviceTopology, params CutoverParams) Phase {
	p := Phase{
		Number:      3,
		Name:        "验证新口",
		Description: fmt.Sprintf("确认 %s 已 Up、可达，协议邻居在新口建立", params.NewInterface),
	}

	cmds := []string{
		fmt.Sprintf("display interface %s", params.NewInterface),
		fmt.Sprintf("ping -a <新口IP> <对端IP>  # 请替换为实际 IP"),
	}

	for _, proto := range topo.Protocols {
		switch proto {
		case "isis":
			cmds = append(cmds, "display isis peer")
		case "ospf":
			if strings.ToLower(topo.Vendor) == "h3c" {
				cmds = append(cmds, "display ospf peer")
			} else {
				cmds = append(cmds, "display ospf peer brief")
			}
		case "ldp":
			cmds = append(cmds, "display mpls ldp session")
		case "bgp":
			cmds = append(cmds, "display bgp peer")
		}
	}

	p.Steps = []DeviceCommand{
		{
			DeviceID:   topo.DeviceID,
			DeviceHost: topo.Hostname,
			Vendor:     topo.Vendor,
			Commands:   cmds,
			Purpose:    fmt.Sprintf("验证新口 %s Up 且协议正常", params.NewInterface),
		},
	}

	p.Notes = append(p.Notes,
		fmt.Sprintf("[ ] %s 接口状态为 Up", params.NewInterface),
		"[ ] ping 对端成功（无丢包）",
	)
	for _, igp := range topo.IGPs {
		for _, iface := range igp.Interfaces {
			if strings.EqualFold(iface, params.OldInterface) {
				p.Notes = append(p.Notes,
					fmt.Sprintf("[ ] %s 进程 %s 邻居在新口已建立（Full/Up）", igp.Protocol, igp.ProcessID),
				)
				break
			}
		}
	}
	p.Notes = append(p.Notes,
		"⚠️  如新口协议未建立，请勿继续执行 Phase 4，先排查原因",
	)

	return p
}

// cutoverPhase4SwitchTraffic — Phase 4: 切流量
// Raises the IGP cost on the old interface to its maximum value, forcing IGP to
// prefer the new interface. Verifies the routing table shifts to the new path.
func cutoverPhase4SwitchTraffic(topo DeviceTopology, params CutoverParams) Phase {
	p := Phase{
		Number:      4,
		Name:        "切流量",
		Description: fmt.Sprintf("调大 %s 的 IGP cost，将流量切到 %s", params.OldInterface, params.NewInterface),
	}

	vendor := strings.ToLower(topo.Vendor)

	// Determine which IGP processes are on the old interface.
	type igpOnIface struct {
		proto     string
		processID string
	}
	var igpsOnOld []igpOnIface
	for _, igp := range topo.IGPs {
		for _, iface := range igp.Interfaces {
			if strings.EqualFold(iface, params.OldInterface) {
				igpsOnOld = append(igpsOnOld, igpOnIface{igp.Protocol, igp.ProcessID})
				break
			}
		}
	}

	var configCmds []string
	if len(igpsOnOld) == 0 {
		// No IGP info — generate a commented template
		switch vendor {
		case "cisco":
			configCmds = []string{
				fmt.Sprintf("interface %s", params.OldInterface),
				" # isis metric 65535  (如使用 ISIS)",
				" # ip ospf cost 65535  (如使用 OSPF)",
			}
		default:
			configCmds = []string{
				"system-view",
				fmt.Sprintf("interface %s", params.OldInterface),
				" # isis cost 65535  (如使用 ISIS)",
				" # ospf cost 65535  (如使用 OSPF)",
				"quit",
				"return",
			}
		}
	} else {
		switch vendor {
		case "cisco":
			configCmds = append(configCmds, fmt.Sprintf("interface %s", params.OldInterface))
			for _, igp := range igpsOnOld {
				switch igp.proto {
				case "isis":
					configCmds = append(configCmds, " isis metric 65535")
				case "ospf":
					configCmds = append(configCmds, " ip ospf cost 65535")
				}
			}
		default: // huawei, h3c
			configCmds = append(configCmds, "system-view", fmt.Sprintf("interface %s", params.OldInterface))
			for _, igp := range igpsOnOld {
				switch igp.proto {
				case "isis":
					configCmds = append(configCmds, " isis cost 65535")
				case "ospf":
					configCmds = append(configCmds, " ospf cost 65535")
				}
			}
			configCmds = append(configCmds, "quit", "return")
		}
	}

	// Verification commands to confirm traffic has shifted
	verifyCmds := []string{
		"display ip routing-table",
		fmt.Sprintf("display interface %s", params.OldInterface),
		fmt.Sprintf("display interface %s", params.NewInterface),
	}

	p.Steps = []DeviceCommand{
		{
			DeviceID:   topo.DeviceID,
			DeviceHost: topo.Hostname,
			Vendor:     topo.Vendor,
			Commands:   configCmds,
			Purpose:    fmt.Sprintf("调大旧口 %s IGP cost 至 65535，引导流量切到新口", params.OldInterface),
		},
		{
			DeviceID:   topo.DeviceID,
			DeviceHost: topo.Hostname,
			Vendor:     topo.Vendor,
			Commands:   verifyCmds,
			Purpose:    "验证路由表已切换到新口路径",
		},
	}

	p.Notes = append(p.Notes,
		"等待 IGP 收敛（建议 30-60 秒）后再检查路由表",
		fmt.Sprintf("[ ] display ip routing-table 中相关路由下一跳已改为 %s 方向", params.NewInterface),
		fmt.Sprintf("[ ] %s 的流量统计（rate/pps）明显下降", params.OldInterface),
		fmt.Sprintf("[ ] %s 的流量统计明显上升", params.NewInterface),
	)

	return p
}

// cutoverPhase5ShutOld — Phase 5: 关旧口
// Shuts down the old interface once traffic has been confirmed to move to the new one.
func cutoverPhase5ShutOld(topo DeviceTopology, params CutoverParams) Phase {
	p := Phase{
		Number:      5,
		Name:        "关旧口",
		Description: fmt.Sprintf("确认流量已切走后，shutdown %s", params.OldInterface),
	}

	vendor := strings.ToLower(topo.Vendor)

	var cmds []string
	switch vendor {
	case "cisco":
		cmds = []string{
			fmt.Sprintf("interface %s", params.OldInterface),
			" shutdown",
		}
	default: // huawei, h3c
		cmds = []string{
			"system-view",
			fmt.Sprintf("interface %s", params.OldInterface),
			" shutdown",
			"quit",
			"return",
		}
	}

	p.Steps = []DeviceCommand{
		{
			DeviceID:   topo.DeviceID,
			DeviceHost: topo.Hostname,
			Vendor:     topo.Vendor,
			Commands:   cmds,
			Purpose:    fmt.Sprintf("shutdown 旧口 %s", params.OldInterface),
		},
	}

	p.Notes = append(p.Notes,
		fmt.Sprintf("⚠️  执行前请确认 %s 上的流量已降为 0 或接近 0", params.OldInterface),
		"shutdown 后立即检查业务是否正常（ping、telnet 等），如有异常立即回退",
	)

	return p
}

// cutoverPhase6PostCheck — Phase 6: 变更后检查
// Confirms that traffic is now carried on the new interface, the old interface is
// administratively down, and all protocols remain healthy.
// Rollback instructions are embedded as notes.
func cutoverPhase6PostCheck(topo DeviceTopology, params CutoverParams) Phase {
	p := Phase{
		Number:      6,
		Name:        "变更后检查",
		Description: "确认新口承载流量、旧口已关闭，各协议邻居状态正常",
	}

	cmds := []string{
		fmt.Sprintf("display interface %s", params.NewInterface),
		fmt.Sprintf("display interface %s", params.OldInterface),
		"display ip routing-table statistics",
	}

	for _, proto := range topo.Protocols {
		switch proto {
		case "isis":
			cmds = append(cmds, "display isis peer")
		case "ospf":
			if strings.ToLower(topo.Vendor) == "h3c" {
				cmds = append(cmds, "display ospf peer")
			} else {
				cmds = append(cmds, "display ospf peer brief")
			}
		case "ldp":
			cmds = append(cmds, "display mpls ldp session")
		case "bgp":
			cmds = append(cmds, "display bgp peer")
		}
	}

	p.Steps = []DeviceCommand{
		{
			DeviceID:   topo.DeviceID,
			DeviceHost: topo.Hostname,
			Vendor:     topo.Vendor,
			Commands:   cmds,
			Purpose:    "变更后检查：新口正常、旧口已关闭、协议健康",
		},
	}

	p.Notes = append(p.Notes,
		fmt.Sprintf("[ ] %s 状态 Up，流量统计正常", params.NewInterface),
		fmt.Sprintf("[ ] %s 状态 Administratively Down", params.OldInterface),
		"[ ] 所有 IGP/LDP/BGP 邻居状态正常（Full/Established）",
		"[ ] 业务端到端连通性验证通过",
		"",
		"── 回退方案 ──────────────────────────────────────────",
		fmt.Sprintf("如需回退："),
		fmt.Sprintf("  1. undo shutdown %s（恢复旧口）", params.OldInterface),
		fmt.Sprintf("  2. 恢复 %s 上的 IGP cost（undo isis cost / undo ospf cost）", params.OldInterface),
		fmt.Sprintf("  3. 确认流量回切到 %s 后，shutdown %s", params.OldInterface, params.NewInterface),
	)

	return p
}

// prefixToMask converts a numeric prefix length (e.g. "30") to a dotted-decimal
// subnet mask (e.g. "255.255.255.252") for use in Cisco IOS syntax.
// If the input is already in dotted-decimal form it is returned unchanged.
func prefixToMask(mask string) string {
	if strings.Contains(mask, ".") {
		return mask // already dotted-decimal
	}
	maskLen := 0
	for _, ch := range mask {
		if ch < '0' || ch > '9' {
			return mask
		}
		maskLen = maskLen*10 + int(ch-'0')
	}
	if maskLen < 0 || maskLen > 32 {
		return mask
	}
	var m [4]uint8
	remaining := maskLen
	for i := 0; i < 4; i++ {
		if remaining >= 8 {
			m[i] = 0xFF
			remaining -= 8
		} else if remaining > 0 {
			m[i] = uint8(0xFF << (8 - remaining))
			remaining = 0
		}
	}
	return fmt.Sprintf("%d.%d.%d.%d", m[0], m[1], m[2], m[3])
}
