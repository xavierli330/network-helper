package parser

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// InferredCommand represents a display command that a device should support,
// inferred from its running configuration.
type InferredCommand struct {
	// Command is the full display command to run, e.g. "display bgp peer".
	Command string `json:"command"`
	// Category groups the command, e.g. "bgp", "interface", "routing", "mpls", "policy".
	Category string `json:"category"`
	// Reason explains why this command was inferred from the config.
	Reason string `json:"reason"`
	// Priority: "high" (core visibility), "medium" (useful), "low" (optional/deep).
	Priority string `json:"priority"`
}

// InferCommands analyzes a device's running configuration and returns a list
// of display commands the device should support. This powers the self-check
// engine: by comparing inferred commands against ClassifyCommand coverage,
// we can detect gaps in parser support.
func InferCommands(vendor, configText string) []InferredCommand {
	if configText == "" {
		return nil
	}
	lower := strings.ToLower(configText)

	var cmds []InferredCommand

	switch vendor {
	case "huawei", "h3c":
		cmds = inferHuaweiH3C(lower, configText)
	case "cisco":
		cmds = inferCisco(lower, configText)
	case "juniper":
		cmds = inferJuniper(lower, configText)
	default:
		return nil
	}

	// Deduplicate by command string
	seen := make(map[string]bool)
	var result []InferredCommand
	for _, c := range cmds {
		if !seen[c.Command] {
			seen[c.Command] = true
			result = append(result, c)
		}
	}

	// Sort by priority (high first), then category, then command
	sort.Slice(result, func(i, j int) bool {
		pi, pj := priorityOrder(result[i].Priority), priorityOrder(result[j].Priority)
		if pi != pj {
			return pi < pj
		}
		if result[i].Category != result[j].Category {
			return result[i].Category < result[j].Category
		}
		return result[i].Command < result[j].Command
	})

	return result
}

func priorityOrder(p string) int {
	switch p {
	case "high":
		return 0
	case "medium":
		return 1
	case "low":
		return 2
	default:
		return 3
	}
}

// ── Huawei / H3C inference ──────────────────────────────────────────────────

var (
	hwIfaceRe       = regexp.MustCompile(`(?m)^interface\s+(\S+)`)
	hwBGPRe         = regexp.MustCompile(`(?mi)^bgp\s+\d+`)
	hwOSPFRe        = regexp.MustCompile(`(?mi)^ospf\s+\d+`)
	hwISISRe        = regexp.MustCompile(`(?mi)^isis\s+\d+`)
	hwMPLSRe        = regexp.MustCompile(`(?mi)^mpls\s*$|^mpls\s+lsr-id`)
	hwLDPRe         = regexp.MustCompile(`(?mi)^mpls\s+ldp`)
	hwRSVPRe        = regexp.MustCompile(`(?mi)^mpls\s+rsvp`)
	hwTERe          = regexp.MustCompile(`(?mi)^mpls\s+te`)
	hwSRRe          = regexp.MustCompile(`(?mi)^segment-routing`)
	hwVPNInstanceRe = regexp.MustCompile(`(?mi)^ip\s+vpn-instance\s+(\S+)`)
	hwRoutePolicyRe = regexp.MustCompile(`(?mi)^route-policy\s+(\S+)`)
	hwACLRe         = regexp.MustCompile(`(?mi)^acl\s+(number\s+)?\d+`)
	hwPrefixListRe  = regexp.MustCompile(`(?mi)^ip\s+ip-prefix\s+(\S+)`)
	hwCommFilterRe  = regexp.MustCompile(`(?mi)^ip\s+community-filter`)
	hwLLDPRe        = regexp.MustCompile(`(?mi)^lldp\s+enable`)
	hwBFDRe         = regexp.MustCompile(`(?mi)^bfd`)
	hwNTPRe         = regexp.MustCompile(`(?mi)^ntp-service`)
	hwTrafficPolRe  = regexp.MustCompile(`(?mi)^traffic-policy\s+(\S+)`)
	hwEthTrunkRe    = regexp.MustCompile(`(?mi)^interface\s+eth-trunk\s*\d+|^interface\s+bridge-aggregation`)
	hwVlanIfRe      = regexp.MustCompile(`(?mi)^interface\s+vlanif\s*\d+|^interface\s+vlan-interface`)
	hwStaticRouteRe = regexp.MustCompile(`(?mi)^ip\s+route-static`)
)

func inferHuaweiH3C(lower, configText string) []InferredCommand {
	var cmds []InferredCommand

	// Always-inferred commands: if we have a config, these are basic checks.
	cmds = append(cmds,
		InferredCommand{Command: "display current-configuration", Category: "config", Reason: "设备配置基线", Priority: "high"},
		InferredCommand{Command: "display version", Category: "system", Reason: "系统版本信息", Priority: "medium"},
	)

	// Interfaces — always present in any config
	if hwIfaceRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "display interface brief", Category: "interface", Reason: "配置中包含 interface 定义", Priority: "high"},
			InferredCommand{Command: "display ip interface brief", Category: "interface", Reason: "配置中包含 interface 定义", Priority: "high"},
		)
	}

	// Routing table — always useful
	cmds = append(cmds,
		InferredCommand{Command: "display ip routing-table", Category: "routing", Reason: "路由表是网络诊断的基础数据", Priority: "high"},
	)

	// BGP
	if hwBGPRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "display bgp peer", Category: "bgp", Reason: "配置中包含 BGP 进程", Priority: "high"},
			InferredCommand{Command: "display bgp network ipv4", Category: "bgp", Reason: "配置中包含 BGP 进程", Priority: "medium"},
		)
	}

	// OSPF
	if hwOSPFRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "display ospf peer", Category: "ospf", Reason: "配置中包含 OSPF 进程", Priority: "high"},
		)
	}

	// IS-IS
	if hwISISRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "display isis peer", Category: "isis", Reason: "配置中包含 IS-IS 进程", Priority: "high"},
		)
	}

	// MPLS
	if hwMPLSRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "display mpls lsp", Category: "mpls", Reason: "配置中启用了 MPLS", Priority: "high"},
			InferredCommand{Command: "display mpls forwarding-table", Category: "mpls", Reason: "配置中启用了 MPLS", Priority: "medium"},
		)
	}

	// MPLS LDP
	if hwLDPRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "display mpls ldp session", Category: "mpls", Reason: "配置中启用了 MPLS LDP", Priority: "high"},
			InferredCommand{Command: "display mpls ldp peer", Category: "mpls", Reason: "配置中启用了 MPLS LDP", Priority: "medium"},
		)
	}

	// RSVP
	if hwRSVPRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "display rsvp session", Category: "mpls", Reason: "配置中启用了 MPLS RSVP", Priority: "medium"},
		)
	}

	// MPLS TE
	if hwTERe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "display mpls te tunnel-interface", Category: "mpls", Reason: "配置中启用了 MPLS TE", Priority: "high"},
		)
	}

	// Segment Routing
	if hwSRRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "display segment-routing mapping-server ipv4", Category: "sr", Reason: "配置中启用了 Segment Routing", Priority: "high"},
			InferredCommand{Command: "display isis segment-routing prefix-sid", Category: "sr", Reason: "配置中启用了 Segment Routing", Priority: "medium"},
		)
	}

	// Route Policies
	policies := hwRoutePolicyRe.FindAllStringSubmatch(configText, -1)
	if len(policies) > 0 {
		cmds = append(cmds,
			InferredCommand{Command: "display route-policy", Category: "policy", Reason: fmt.Sprintf("配置中定义了 %d 条 route-policy", len(policies)), Priority: "medium"},
		)
	}

	// ACL
	if hwACLRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "display acl all", Category: "policy", Reason: "配置中定义了 ACL 规则", Priority: "medium"},
		)
	}

	// Prefix Lists
	if hwPrefixListRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "display ip ip-prefix", Category: "policy", Reason: "配置中定义了 ip-prefix", Priority: "low"},
		)
	}

	// Community Filter
	if hwCommFilterRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "display ip community-filter", Category: "policy", Reason: "配置中定义了 community-filter", Priority: "low"},
		)
	}

	// LLDP
	if hwLLDPRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "display lldp neighbor brief", Category: "neighbor", Reason: "配置中启用了 LLDP", Priority: "medium"},
		)
	}

	// VPN Instances
	vpns := hwVPNInstanceRe.FindAllStringSubmatch(configText, -1)
	if len(vpns) > 0 {
		cmds = append(cmds,
			InferredCommand{Command: "display ip vpn-instance", Category: "vpn", Reason: fmt.Sprintf("配置中定义了 %d 个 VPN 实例", len(vpns)), Priority: "medium"},
		)
		for _, m := range vpns {
			cmds = append(cmds,
				InferredCommand{
					Command:  fmt.Sprintf("display ip routing-table vpn-instance %s", m[1]),
					Category: "vpn",
					Reason:   fmt.Sprintf("VPN 实例 %s 的路由表", m[1]),
					Priority: "low",
				},
			)
		}
	}

	// BFD
	if hwBFDRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "display bfd session all", Category: "bfd", Reason: "配置中启用了 BFD", Priority: "medium"},
		)
	}

	// Link-Aggregation (Eth-Trunk / Bridge-Aggregation)
	if hwEthTrunkRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "display eth-trunk", Category: "interface", Reason: "配置中包含链路聚合接口", Priority: "medium"},
		)
	}

	// FIB
	cmds = append(cmds,
		InferredCommand{Command: "display fib", Category: "routing", Reason: "FIB 转发表检查", Priority: "low"},
	)

	// Static routes
	if hwStaticRouteRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "display ip routing-table protocol static", Category: "routing", Reason: "配置中包含静态路由", Priority: "low"},
		)
	}

	// Traffic Policy
	if hwTrafficPolRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "display traffic-policy applied-record", Category: "qos", Reason: "配置中定义了流量策略", Priority: "low"},
		)
	}

	// NTP
	if hwNTPRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "display ntp-service status", Category: "system", Reason: "配置中启用了 NTP", Priority: "low"},
		)
	}

	return cmds
}

// ── Cisco IOS-XR inference ──────────────────────────────────────────────────

var (
	ciscoBGPInferRe  = regexp.MustCompile(`(?mi)^router bgp\s+\d+`)
	ciscoOSPFInferRe = regexp.MustCompile(`(?mi)^router ospf\s+`)
	ciscoISISInferRe = regexp.MustCompile(`(?mi)^router isis\s+`)
	ciscoMPLSInferRe = regexp.MustCompile(`(?mi)^mpls\s+`)
	ciscoLDPInferRe  = regexp.MustCompile(`(?mi)^mpls\s+ldp`)
	ciscoRSVPInferRe = regexp.MustCompile(`(?mi)^rsvp`)
	ciscoTEInferRe   = regexp.MustCompile(`(?mi)^mpls\s+traffic-eng`)
	ciscoRPInferRe   = regexp.MustCompile(`(?mi)^route-policy\s+(\S+)`)
	ciscoVRFInferRe  = regexp.MustCompile(`(?mi)^vrf\s+(\S+)`)
	ciscoACLInferRe  = regexp.MustCompile(`(?mi)^ipv4\s+access-list`)
	ciscoPLInferRe   = regexp.MustCompile(`(?mi)^prefix-set\s+(\S+)`)
	ciscoBFDInferRe  = regexp.MustCompile(`(?mi)^bfd`)
	ciscoLLDPInferRe = regexp.MustCompile(`(?mi)^lldp`)
)

func inferCisco(lower, configText string) []InferredCommand {
	var cmds []InferredCommand

	cmds = append(cmds,
		InferredCommand{Command: "show running-config", Category: "config", Reason: "设备配置基线", Priority: "high"},
		InferredCommand{Command: "show version", Category: "system", Reason: "系统版本信息", Priority: "medium"},
		InferredCommand{Command: "show ip route", Category: "routing", Reason: "路由表是网络诊断的基础数据", Priority: "high"},
		InferredCommand{Command: "show interface brief", Category: "interface", Reason: "接口状态总览", Priority: "high"},
	)

	if ciscoBGPInferRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "show bgp summary", Category: "bgp", Reason: "配置中包含 BGP 进程", Priority: "high"},
			InferredCommand{Command: "show bgp ipv4 unicast summary", Category: "bgp", Reason: "配置中包含 BGP 进程", Priority: "medium"},
		)
	}

	if ciscoOSPFInferRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "show ospf neighbor", Category: "ospf", Reason: "配置中包含 OSPF 进程", Priority: "high"},
		)
	}

	if ciscoISISInferRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "show isis adjacency", Category: "isis", Reason: "配置中包含 IS-IS 进程", Priority: "high"},
		)
	}

	if ciscoMPLSInferRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "show mpls forwarding", Category: "mpls", Reason: "配置中启用了 MPLS", Priority: "high"},
		)
	}

	if ciscoLDPInferRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "show mpls ldp neighbor", Category: "mpls", Reason: "配置中启用了 MPLS LDP", Priority: "medium"},
		)
	}

	if ciscoRSVPInferRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "show rsvp session", Category: "mpls", Reason: "配置中启用了 RSVP", Priority: "medium"},
		)
	}

	if ciscoTEInferRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "show mpls traffic-eng tunnels", Category: "mpls", Reason: "配置中启用了 MPLS TE", Priority: "high"},
		)
	}

	policies := ciscoRPInferRe.FindAllStringSubmatch(configText, -1)
	if len(policies) > 0 {
		cmds = append(cmds,
			InferredCommand{Command: "show rpl route-policy", Category: "policy", Reason: fmt.Sprintf("配置中定义了 %d 条 route-policy", len(policies)), Priority: "medium"},
		)
	}

	vrfs := ciscoVRFInferRe.FindAllStringSubmatch(configText, -1)
	if len(vrfs) > 0 {
		cmds = append(cmds,
			InferredCommand{Command: "show vrf all", Category: "vpn", Reason: fmt.Sprintf("配置中定义了 %d 个 VRF", len(vrfs)), Priority: "medium"},
		)
	}

	if ciscoACLInferRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "show access-lists", Category: "policy", Reason: "配置中定义了 ACL", Priority: "medium"},
		)
	}

	if ciscoPLInferRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "show rpl prefix-set", Category: "policy", Reason: "配置中定义了 prefix-set", Priority: "low"},
		)
	}

	if ciscoBFDInferRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "show bfd session", Category: "bfd", Reason: "配置中启用了 BFD", Priority: "medium"},
		)
	}

	if ciscoLLDPInferRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "show lldp neighbors", Category: "neighbor", Reason: "配置中启用了 LLDP", Priority: "medium"},
		)
	}

	return cmds
}

// ── Juniper inference ───────────────────────────────────────────────────────

var (
	juniperBGPInferRe  = regexp.MustCompile(`(?m)protocols\s*\{[^}]*bgp`)
	juniperOSPFInferRe = regexp.MustCompile(`(?m)protocols\s*\{[^}]*ospf`)
	juniperISISInferRe = regexp.MustCompile(`(?m)protocols\s*\{[^}]*isis`)
	juniperMPLSInferRe = regexp.MustCompile(`(?m)protocols\s*\{[^}]*mpls`)
	juniperLDPInferRe  = regexp.MustCompile(`(?m)protocols\s*\{[^}]*ldp`)
	juniperRSVPInferRe = regexp.MustCompile(`(?m)protocols\s*\{[^}]*rsvp`)
	juniperPSInferRe   = regexp.MustCompile(`(?m)policy-statement\s+(\S+)`)
	juniperRIInferRe   = regexp.MustCompile(`(?m)routing-instances\s*\{`)
)

func inferJuniper(lower, configText string) []InferredCommand {
	var cmds []InferredCommand

	cmds = append(cmds,
		InferredCommand{Command: "show configuration", Category: "config", Reason: "设备配置基线", Priority: "high"},
		InferredCommand{Command: "show version", Category: "system", Reason: "系统版本信息", Priority: "medium"},
		InferredCommand{Command: "show route summary", Category: "routing", Reason: "路由表是网络诊断的基础数据", Priority: "high"},
		InferredCommand{Command: "show interface terse", Category: "interface", Reason: "接口状态总览", Priority: "high"},
	)

	if juniperBGPInferRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "show bgp summary", Category: "bgp", Reason: "配置中包含 BGP", Priority: "high"},
		)
	}

	if juniperOSPFInferRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "show ospf neighbor", Category: "ospf", Reason: "配置中包含 OSPF", Priority: "high"},
		)
	}

	if juniperISISInferRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "show isis adjacency", Category: "isis", Reason: "配置中包含 IS-IS", Priority: "high"},
		)
	}

	if juniperMPLSInferRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "show route table mpls.0", Category: "mpls", Reason: "配置中启用了 MPLS", Priority: "high"},
		)
	}

	if juniperLDPInferRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "show ldp neighbor", Category: "mpls", Reason: "配置中启用了 LDP", Priority: "medium"},
		)
	}

	if juniperRSVPInferRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "show rsvp session", Category: "mpls", Reason: "配置中启用了 RSVP", Priority: "medium"},
		)
	}

	policies := juniperPSInferRe.FindAllStringSubmatch(configText, -1)
	if len(policies) > 0 {
		cmds = append(cmds,
			InferredCommand{Command: "show policy", Category: "policy", Reason: fmt.Sprintf("配置中定义了 %d 条 policy-statement", len(policies)), Priority: "medium"},
		)
	}

	if juniperRIInferRe.MatchString(configText) {
		cmds = append(cmds,
			InferredCommand{Command: "show route instance", Category: "vpn", Reason: "配置中定义了 routing-instances", Priority: "medium"},
		)
	}

	return cmds
}
