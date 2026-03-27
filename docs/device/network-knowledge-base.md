# 🌐 网络知识库 — 华为 NE40E & H3C S12500X & Cisco ASR9000 & Juniper Junos

> **适用设备**:
> - 华为 NE40E V800R025C00SPC500
> - H3C S12500X-AF / S12500-X / S9800 (Release 2710+)
> - Cisco ASR9000 系列 (IOS-XR)
> - Juniper MX/PTX 系列 (Junos OS)
>
> **用途**: 供网络排障分析工具调用的结构化知识库，覆盖配置命令、回显解读、排障思路、信息关联
>
> **文档来源**:
> - 华为: 26份NE40E配置指南 (IP路由/MPLS/SR/VPN/QoS/安全/可靠性/监控/路径控制)
> - H3C: 完整命令参考手册(5401页) — OSPF/IS-IS/BGP/MPLS/ACL/QoS/VRRP/BFD/EVPN等
> - Cisco: 19份ASR9000 IOS-XR命令参考 — 路由(BGP/OSPF/IS-IS/静态/RPL)/MPLS(LDP/TE/Static/OAM)/SR(SR-MPLS/SRv6/Flex-Algo/PCE)/VPN(L2VPN/VPLS/EVPN/VXLAN)/QoS/安全(AAA/IPSec/MACsec/LPTS)/监控(NetFlow/EEM/IP SLA)/IP地址服务(ACL/ARP/CEF/DHCP/HSRP/VRRP)
> - Juniper: Junos OS CLI命令参考(32859页) — 路由(OSPF/IS-IS/BGP/静态)/MPLS(LSP/LDP/RSVP)/SR(SR-MPLS/SRv6/SR-TE/Flex-Algo)/路由策略(policy-statement/prefix-list/as-path/community)/VPN(L3VPN/L2VPN/VPLS/EVPN)/安全(firewall filter)/QoS(CoS/schedulers/drop-profiles/rewrite-rules)/可靠性(BFD/VRRP)/监控(flow-monitoring/JTI telemetry)

---

## 📑 知识库文件索引

| 文件 | 内容 | 覆盖厂商 |
|:---|:---|:---|
| [01-ip-routing.md](./kb/01-ip-routing.md) | IP路由协议：静态路由、OSPF、IS-IS、BGP、路由策略 | 华为/H3C/Cisco/Juniper |
| [02-mpls.md](./kb/02-mpls.md) | MPLS技术：LDP、RSVP-TE、静态CR-LSP、TE FRR、MPLS OAM | 华为/H3C/Cisco/Juniper |
| [03-segment-routing.md](./kb/03-segment-routing.md) | Segment Routing：SR-MPLS BE、SR TE Policy、SRv6、Flex-Algo、TI-LFA | 华为/H3C/Cisco/Juniper |
| [04-vpn.md](./kb/04-vpn.md) | VPN技术：L3VPN、跨域VPN、EVPN/L2VPN/VPLS、VXLAN、隧道策略 | 华为/H3C/Cisco/Juniper |
| [05-qos.md](./kb/05-qos.md) | QoS：MQC/MQC(Cisco)/CoS(Juniper)体系、流量监管与整形、HQoS | 华为/H3C/Cisco/Juniper |
| [06-security.md](./kb/06-security.md) | 安全：ACL/Firewall Filter、本机防攻击/LPTS/RE Filter、URPF、AAA/IPSec/MACsec | 华为/H3C/Cisco/Juniper |
| [07-reliability.md](./kb/07-reliability.md) | 网络可靠性：BFD、VRRP/HSRP、MPLS OAM | 华为/H3C/Cisco/Juniper |
| [08-monitoring.md](./kb/08-monitoring.md) | 系统监控：NQA/IP SLA、NetStream/NetFlow/Jflow、Telemetry/JTI、EEM、IFIT | 华为/H3C/Cisco/Juniper |
| [09-pcep.md](./kb/09-pcep.md) | 路径控制：PCEP (PCC/PCE Server) | 华为/Cisco/Juniper |
| [10-display-reference.md](./kb/10-display-reference.md) | **四厂商Show/Display命令对照表**与回显解读 | 华为/H3C/Cisco/Juniper |
| [11-troubleshooting.md](./kb/11-troubleshooting.md) | 排障思路与方法论、**四厂商排障命令对照**、Cisco/Juniper特有排障工具 | 华为/H3C/Cisco/Juniper |
| [12-cross-reference.md](./kb/12-cross-reference.md) | 跨特性信息关联分析、**四厂商命令映射关系表** | 华为/H3C/Cisco/Juniper |

---

## 🔑 使用说明

### 查询规则
1. **按协议查命令** → 打开对应章节文件，查找协议配置段(每个协议按华为/H3C/Cisco/Juniper分节)
2. **按回显排障** → 打开 `10-display-reference.md`，查找四厂商对应show/display命令和字段解读
3. **按故障现象排障** → 打开 `11-troubleshooting.md`，匹配故障域和症状，查看四厂商首查命令
4. **查跨特性依赖** → 打开 `12-cross-reference.md`，查找特性关联图和四厂商命令映射
5. **跨厂商命令转换** → 打开 `12-cross-reference.md` 12.10节，查找同功能在不同厂商的命令入口

### 厂商标记约定
- `华为:` = 华为 NE40E V800R025C00SPC500 命令语法
- `H3C:` = H3C S12500X 系列命令语法
- `Cisco:` = Cisco ASR9000 IOS-XR 命令语法
- `Juniper:` = Juniper Junos OS 命令语法 (MX/PTX系列)
- 无标记 = 通用概念

### 命令语法约定
- `< >` = 必填参数
- `[ ]` = 可选参数
- `{ A | B }` = 从A或B中选择
- `★` = 排障高频使用命令
- `★★★` = 关键排障命令(如Cisco `show cef drops`)

### 厂商特有概念对照
| 概念 | 华为 | H3C | Cisco IOS-XR | Juniper Junos |
|:---|:---|:---|:---|:---|
| 路由策略框架 | route-policy + if-match/apply | route-policy + if-match/apply | RPL(route-policy + if/set) | policy-statement(term/from/then) |
| QoS框架 | MQC(classifier+behavior+policy) | MQC(classifier+behavior+policy) | MQC(class-map+policy-map) | CoS(classifiers+schedulers+scheduler-maps) |
| ACL框架 | acl(数字编号:2000-5999) | acl(basic/advanced) | access-list(命名) | firewall filter(term-based) |
| 控制面防护 | cpu-defend policy | - | LPTS (自动) | lo0 firewall filter |
| VPN实例 | ip vpn-instance | ip vpn-instance | vrf | routing-instances |
| 网关冗余 | VRRP | VRRP | VRRP + HSRP(Cisco特有) | VRRP |
| 配置模型 | 命令行(扁平) | 命令行(扁平) | 命令行(层级) | 层级配置+commit模型 |
