package model

// CoreCommandRelations defines the core command correlation relationships
// derived from network engineering analysis (Section 3).
var CoreCommandRelations = []CommandRelation{
	// ── BGP 关联 ──
	{
		SourceCmd:    "display bgp peer",
		TargetCmd:    "display bgp peer {ip} verbose",
		RelationType: "context",
		FieldMapping: map[string]string{"Peer": "{ip}"},
		Description:  "BGP概览到邻居详情",
	},
	{
		SourceCmd:    "display bgp peer",
		TargetCmd:    "display interface {interface}",
		RelationType: "reference",
		FieldMapping: map[string]string{"Interface": "{interface}"},
		Description:  "BGP邻居引用接口",
	},
	// ── 路由 关联 ──
	{
		SourceCmd:    "display ip routing-table",
		TargetCmd:    "display arp",
		RelationType: "reference",
		FieldMapping: map[string]string{"NextHop": "{ip}"},
		Description:  "路由下一跳到ARP检查",
	},
	{
		SourceCmd:    "display ip routing-table",
		TargetCmd:    "display interface {interface}",
		RelationType: "reference",
		FieldMapping: map[string]string{"Interface": "{interface}"},
		Description:  "路由出接口到接口状态",
	},
	// ── VPN 关联 ──
	{
		SourceCmd:    "display ip vpn-instance",
		TargetCmd:    "display ip routing-table vpn-instance {name}",
		RelationType: "context",
		FieldMapping: map[string]string{"VPN-Instance": "{name}"},
		Description:  "VPN实例到VPN路由",
	},
	{
		SourceCmd:    "display ip vpn-instance",
		TargetCmd:    "display bgp vpnv4 vpn-instance {name} peer",
		RelationType: "context",
		FieldMapping: map[string]string{"VPN-Instance": "{name}"},
		Description:  "VPN实例到VPNv4邻居",
	},
	// ── MPLS 关联 ──
	{
		SourceCmd:    "display mpls ldp session",
		TargetCmd:    "display mpls ldp session {ip} verbose",
		RelationType: "context",
		FieldMapping: map[string]string{"PeerLDPID": "{ip}"},
		Description:  "LDP会话概览到详情",
	},
	{
		SourceCmd:    "display mpls forwarding-table",
		TargetCmd:    "display ip routing-table",
		RelationType: "reference",
		FieldMapping: map[string]string{"FEC": "{prefix}"},
		Description:  "LFIB到RIB一致性",
	},
	// ── 接口 关联 ──
	{
		SourceCmd:    "display interface brief",
		TargetCmd:    "display interface {interface}",
		RelationType: "context",
		FieldMapping: map[string]string{"Interface": "{interface}"},
		Description:  "接口概览到详情",
	},
}

// CoreTroubleshootScenarios defines the core troubleshooting command chains
// derived from network engineering analysis (Section 4).
var CoreTroubleshootScenarios = []TroubleshootScenario{
	{
		ID: "bgp-peer-down", Name: "BGP邻居不通", Domain: "ip",
		Triggers: []string{"BGP peer Idle", "BGP peer Active", "BGP session not established"},
		Steps: []TroubleshootStep{
			{
				Order: 1, Purpose: "查BGP概览",
				Commands: map[string]string{
					"huawei":  "display bgp peer",
					"cisco":   "show bgp summary",
					"juniper": "show bgp summary",
				},
			},
			{
				Order: 2, Purpose: "邻居详情", ParamFrom: "Peer",
				Commands: map[string]string{
					"huawei":  "display bgp peer {ip} verbose",
					"cisco":   "show bgp neighbors {ip}",
					"juniper": "show bgp neighbor {ip}",
				},
			},
			{
				Order: 3, Purpose: "检查路由", ParamFrom: "PeerIP",
				Commands: map[string]string{
					"huawei":  "display ip routing-table {peer-ip}",
					"cisco":   "show route {peer-ip}",
					"juniper": "show route {peer-ip}",
				},
			},
			{
				Order: 4, Purpose: "连通性测试",
				Commands: map[string]string{
					"huawei":  "ping -a {src} {peer-ip}",
					"cisco":   "ping {peer-ip} source {src}",
					"juniper": "ping {peer-ip} source {src}",
				},
			},
		},
	},
	{
		ID: "link-fault", Name: "链路故障", Domain: "ip",
		Triggers: []string{"interface down", "packet loss", "CRC errors"},
		Steps: []TroubleshootStep{
			{
				Order: 1, Purpose: "接口概览",
				Commands: map[string]string{
					"huawei":  "display interface brief",
					"cisco":   "show interface brief",
					"juniper": "show interfaces terse",
				},
			},
			{
				Order: 2, Purpose: "接口详情", ParamFrom: "Interface",
				Commands: map[string]string{
					"huawei":  "display interface {iface}",
					"cisco":   "show interface {iface}",
					"juniper": "show interfaces {iface} extensive",
				},
			},
			{
				Order: 3, Purpose: "光模块", ParamFrom: "Interface",
				Commands: map[string]string{
					"huawei":  "display transceiver interface {iface}",
					"cisco":   "show controllers {iface}",
					"juniper": "show interfaces diagnostics optics {iface}",
				},
			},
			{
				Order: 4, Purpose: "ARP检查", ParamFrom: "Interface",
				Commands: map[string]string{
					"huawei":  "display arp interface {iface}",
					"cisco":   "show arp",
					"juniper": "show arp interface {iface}",
				},
			},
		},
	},
	{
		ID: "vpn-service-fault", Name: "VPN服务异常", Domain: "vpn",
		Triggers: []string{"VPN unreachable", "VPN route missing", "VPN traffic blackhole"},
		Steps: []TroubleshootStep{
			{
				Order: 1, Purpose: "VRF检查",
				Commands: map[string]string{
					"huawei":  "display ip vpn-instance {name}",
					"cisco":   "show vrf {name} detail",
					"juniper": "show route instance {name}",
				},
			},
			{
				Order: 2, Purpose: "VRF路由", ParamFrom: "VPN-Instance",
				Commands: map[string]string{
					"huawei":  "display ip routing-table vpn-instance {name}",
					"cisco":   "show route vrf {name}",
					"juniper": "show route table {name}.inet.0",
				},
			},
			{
				Order: 3, Purpose: "PE-CE BGP", ParamFrom: "VPN-Instance",
				Commands: map[string]string{
					"huawei":  "display bgp vpnv4 vpn-instance {name} peer",
					"cisco":   "show bgp vpnv4 unicast vrf {name} summary",
					"juniper": "show bgp summary",
				},
			},
			{
				Order: 4, Purpose: "VPNv4路由", ParamFrom: "VPN-Instance",
				Commands: map[string]string{
					"huawei":  "display bgp vpnv4 vpn-instance {name} routing-table",
					"cisco":   "show bgp vpnv4 unicast vrf {name}",
					"juniper": "show route table {name}.inet.0 protocol bgp",
				},
			},
			{
				Order: 5, Purpose: "传输隧道",
				Commands: map[string]string{
					"huawei":  "display mpls lsp",
					"cisco":   "show mpls forwarding",
					"juniper": "show route table mpls.0",
				},
			},
		},
	},
	{
		ID: "mpls-lsp-broken", Name: "MPLS LSP断裂", Domain: "label",
		Triggers: []string{"LSP down", "label blackhole", "VPN transport failure"},
		Steps: []TroubleshootStep{
			{
				Order: 1, Purpose: "LSP状态",
				Commands: map[string]string{
					"huawei":  "display mpls lsp verbose",
					"cisco":   "show mpls forwarding",
					"juniper": "show route table mpls.0",
				},
			},
			{
				Order: 2, Purpose: "LFIB",
				Commands: map[string]string{
					"huawei":  "display mpls forwarding-table {prefix}",
					"cisco":   "show mpls forwarding-table {prefix}",
					"juniper": "show route table mpls.0 label {label}",
				},
			},
			{
				Order: 3, Purpose: "LDP会话",
				Commands: map[string]string{
					"huawei":  "display mpls ldp session",
					"cisco":   "show mpls ldp neighbor",
					"juniper": "show ldp session",
				},
			},
			{
				Order: 4, Purpose: "IGP路由",
				Commands: map[string]string{
					"huawei":  "display ip routing-table {loopback}",
					"cisco":   "show route {loopback}",
					"juniper": "show route {loopback}",
				},
			},
		},
	},
}
