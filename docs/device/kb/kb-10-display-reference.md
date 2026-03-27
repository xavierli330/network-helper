# 第十章 Show/Display命令速查与回显解读

> 按排障场景组织，标注 ★ 为高频使用命令
> 四厂商对照: 华为(display) / H3C(display) / Cisco IOS-XR(show) / Juniper Junos(show)

## 10.1 IGP状态验证

| 场景 | 华为 NE40E | H3C S12500X | Cisco IOS-XR (ASR9000) | Juniper Junos |
|:---|:---|:---|:---|:---|
| OSPF邻居 ★ | `display ospf peer [brief]` | `display ospf peer [verbose]` | `show ospf neighbor [detail]` | `show ospf neighbor [detail\|extensive]` |
| OSPF路由 | `display ospf routing` | `display ospf routing` | `show ospf route` | `show ospf route [detail]` |
| OSPF LSDB | `display ospf lsdb [type] [area <id>]` | `display ospf lsdb` | `show ospf database [type] [area <id>]` | `show ospf database [type] [area <id>] [extensive]` |
| OSPF接口 | `display ospf interface [<if>]` | `display ospf interface [<if>]` | `show ospf interface [<if>] [brief]` | `show ospf interface [<if>] [detail]` |
| OSPF错误 | `display ospf error` | - | `show ospf statistics` | `show ospf log` |
| IS-IS邻居 ★ | `display isis peer [verbose]` | `display isis peer [verbose]` | `show isis neighbors [detail]` | `show isis adjacency [detail\|extensive]` |
| IS-IS LSDB | `display isis lsdb [level] [verbose]` | `display isis lsdb [verbose]` | `show isis database [level] [detail]` | `show isis database [level] [detail\|extensive]` |
| IS-IS路由 | `display isis route` | `display isis route` | `show isis route [<prefix>]` | `show isis route [<prefix>] [detail]` |
| IS-IS SPF日志 | `display isis spf-log` | `display isis spf-log` | `show isis spf-log` | `show isis spf log [brief]` |
| IS-IS错误 | `display isis error` | - | `show isis statistics` | `show isis statistics` |
| 路由表 ★ | `display ip routing-table [verbose]` | `display ip routing-table [verbose]` | `show route [ipv4] [<prefix>] [detail]` | `show route [<prefix>] [detail\|extensive]` |
| FIB表 ★ | `display fib [<ip>]` | `display fib [<ip>]` | `show cef [<prefix>] [detail]` | `show route forwarding-table [destination <prefix>]` |

### OSPF邻居状态解读
| 状态 | 含义 | 排障方向 |
|:---|:---|:---|
| Down | 未收到Hello | 物理连通/ACL/认证 |
| Init | 单向Hello | 单向通信/认证不匹配 |
| ExStart | 主从协商 | **MTU不匹配(最常见)** |
| Full | 邻接建立 | **Full≠转发正常** |

### IS-IS邻居状态解读
| 状态 | 含义 | 排障方向 |
|:---|:---|:---|
| Initializing | 三次握手未完成 | 认证/级别/MTU不匹配 |
| Up | 正常 | 检查Level是否匹配预期 |

### 路由表回显字段
| 字段 | 说明 |
|:---|:---|
| Destination/Mask | 目的网段 |
| Proto | 路由来源(Static/OSPF/BGP/ISIS等) |
| Pre | 优先级(值越小越优) |
| Cost/Metric | 开销 |
| NextHop | 下一跳 |
| Interface | 出接口 |
| Flags: D | 已下发FIB(无D=仅RIB未转发) |
| Flags: R | 迭代路由(需递归解析下一跳) |

---

## 10.2 BGP排障

| 场景 | 华为 NE40E | H3C S12500X | Cisco IOS-XR | Juniper Junos |
|:---|:---|:---|:---|:---|
| BGP邻居 ★ | `display bgp peer [<ip>] [verbose]` | `display bgp peer [verbose]` | `show bgp neighbors [<ip>]` / `show bgp summary` | `show bgp neighbor [<ip>]` / `show bgp summary` |
| BGP路由 ★ | `display bgp routing-table [<ip>]` | `display bgp routing-table [<ip>]` | `show bgp [ipv4 unicast] [<prefix>] [detail]` | `show route protocol bgp [<prefix>] [detail]` |
| VPNv4路由 | `display bgp vpnv4 all routing-table` | `display bgp vpnv4 all routing-table` | `show bgp vpnv4 unicast [rd <rd>]` | `show route table bgp.l3vpn.0` |
| VPN实例路由 | `display bgp vpnv4 vpn-instance <vpn> routing-table` | `display bgp vpnv4 vpn-instance <vpn> routing-table` | `show bgp vrf <vrf> [<prefix>]` | `show route table <vrf>.inet.0 protocol bgp` |
| EVPN路由 | `display bgp evpn all routing-table [type {1-5}]` | `display bgp l2vpn evpn routing-table` | `show bgp l2vpn evpn [route-type {1-5}]` | `show route table <inst>.evpn.0` / `show evpn database` |
| 路由抑制 | `display bgp routing-table dampened` | `display bgp routing-table dampened` | `show bgp ipv4 unicast dampened-paths` | `show route damping suppressed` |

### BGP邻居状态解读
| 状态 | 含义 | 排障方向 |
|:---|:---|:---|
| Idle | 初始/重置 | ACL阻断/路由不可达/AS号错误/达最大前缀数 |
| Connect | TCP建立中 | 防火墙/ACL阻断TCP179 |
| Active | TCP失败重试 | **最常见问题状态**: 对端未配/ACL/路由不可达 |
| OpenSent | OPEN已发 | 版本/能力(AFI/SAFI)不匹配 |
| OpenConfirm | OPEN已确认 | 即将建立,极少卡此处 |
| Established | 会话建立 | 正常,确认AFI/SAFI协商 |

### BGP路由详细字段
| 字段 | 说明 |
|:---|:---|
| Network/Prefix | 路由前缀 |
| NextHop | 下一跳(0.0.0.0=本地始发) |
| LocPrf | Local Preference(默认100) |
| MED | 多出口鉴别(越小越优) |
| AS-Path | AS路径(为空=本AS始发) |
| Origin | i=IGP, e=EGP, ?=Incomplete |
| *> | *=有效, >=最优 |
| Community | 团体属性 |
| Ext-Community | 扩展团体(含RT) |
| Label | VPN标签 |

---

## 10.3 MPLS/标签域

| 场景 | 华为 NE40E | H3C S12500X | Cisco IOS-XR | Juniper Junos |
|:---|:---|:---|:---|:---|
| LDP会话 ★ | `display mpls ldp session [verbose]` | `display mpls ldp session [verbose]` | `show mpls ldp neighbor [detail]` | `show ldp session [detail\|extensive]` |
| LDP邻居 | `display mpls ldp peer [verbose]` | `display mpls ldp peer` | `show mpls ldp neighbor [<ip>]` | `show ldp neighbor [detail]` |
| 标签绑定 | `display mpls lsp [<ip> <mask>] [verbose]` | `display mpls lsp [<ip> <mask>]` | `show mpls ldp bindings [<prefix>/<len>]` | `show ldp database [<prefix>]` |
| LFIB(转发真相) ★ | `display mpls forwarding-table` | `display mpls forwarding-table` | `show mpls forwarding [prefix <prefix>/<len>]` | `show route table mpls.0` |
| 标签统计 | `display mpls lsp statistics` | `display mpls lsp statistics` | `show mpls label table [detail]` | `show ldp statistics` |
| TE隧道 ★ | `display mpls te tunnel-interface` | `display mpls te tunnel-interface` | `show mpls traffic-eng tunnels [detail]` | `show mpls lsp [detail\|extensive]` |
| RSVP会话 | `display rsvp session [detail]` | `display rsvp session` | `show rsvp session [detail]` | `show rsvp session [detail\|extensive]` |
| RSVP错误 | `display rsvp statistics` | `display rsvp statistics` | `show rsvp counters [summary]` | `show rsvp statistics` |
| LSP验证 ★ | `ping lsp ip <dest> <mask>` | `ping mpls lsp ip <dest> <mask>` | `ping mpls ipv4 <dest>/<mask>` | `ping mpls ldp <dest>/<mask>` |
| LSP追踪 ★ | `tracert lsp ip <dest> <mask>` | `tracert mpls lsp ip <dest> <mask>` | `traceroute mpls ipv4 <dest>/<mask>` | `traceroute mpls ldp <dest>/<mask>` |

### MPLS LSP回显字段
| 字段 | 说明 |
|:---|:---|
| FEC | 转发等价类(目的IP/掩码) |
| In-Label | 入标签 |
| Out-Label | 出标签(3=implicit-null/PHP, 0=explicit-null) |
| Next-Hop | 下一跳 |
| Out-Interface | 出接口 |
| Lsp Type | Ingress/Transit/Egress |

### LDP会话状态
| 状态 | 说明 |
|:---|:---|
| Operational | 正常 |
| Non Existent | 不存在(传输地址不可达/认证) |
| Initialized | 协商中(参数不匹配) |

---

## 10.4 SR/SR Policy

| 场景 | 华为 NE40E | Cisco IOS-XR | Juniper Junos |
|:---|:---|:---|:---|
| Prefix SID ★ | `display segment-routing prefix mpls` | `show isis segment-routing label table` | `show isis spring label table` |
| SRGB | `display segment-routing global-block` | `show segment-routing mpls state` | `show isis spring label table` |
| Adj-SID | `display segment-routing adjacency mpls` | `show isis adjacency detail` (含SID) | `show isis spring node-segment` |
| IS-IS SR | `display isis segment-routing [verbose]` | `show isis segment-routing label table` | `show isis overview \| match segment` |
| SR TE Policy ★ | `display segment-routing te policy` | `show segment-routing traffic-eng policy` | `show spring-traffic-engineering lsp` |
| SR TE转发表 | `display segment-routing te forwarding-table` | `show segment-routing traffic-eng forwarding` | `show route programmed-by spring-te` |
| SR流量统计 | `display segment-routing te policy statistics` | `show segment-routing traffic-eng policy detail` | `show spring-traffic-engineering lsp detail` |
| SRv6 Locator | `display segment-routing ipv6 locator` | `show segment-routing srv6 locator` | `show srv6 locator` |
| SRv6 SID | `display segment-routing ipv6 sid` | `show segment-routing srv6 sid` | `show srv6 local-sids` |
| Flex-Algo | `display segment-routing flex-algo` | `show isis flex-algo` | `show isis spring flex-algorithm` |
| FRR备份 | `display isis frr route` / `display isis ti-lfa` | `show isis fast-reroute` | `show isis backup spf results` |
| SR-TE Ping ★ | - | `ping mpls traffic-eng tunnel <n>` | `ping mpls segment-routing spring-te <lsp>` |
| SR-TE Trace ★ | - | `traceroute mpls traffic-eng tunnel <n>` | `traceroute mpls segment-routing spring-te <lsp>` |

---

## 10.5 VPN

| 场景 | 华为 NE40E | H3C S12500X | Cisco IOS-XR | Juniper Junos |
|:---|:---|:---|:---|:---|
| VPN/VRF实例 ★ | `display ip vpn-instance [verbose]` | `display ip vpn-instance` | `show vrf [<vrf>] [detail]` | `show route instance [<name>] [detail]` |
| VPN路由 ★ | `display ip routing-table vpn-instance <vpn>` | `display ip routing-table vpn-instance <vpn>` | `show route vrf <vrf>` | `show route table <vrf>.inet.0` |
| VPN FIB | `display fib vpn-instance <vpn>` | - | `show cef vrf <vrf>` | `show route forwarding-table table <vrf>` |
| 隧道策略 | `display tunnel-policy` | - | - (VRF直接关联隧道) | - (routing-instances直接关联) |
| 隧道信息 | `display tunnel-info [all] [destination <ip>]` | - | `show mpls forwarding` | `show route table mpls.0` |
| EVPN实例 | `display evpn vpn-instance [verbose]` | - | `show evpn evi [detail]` | `show evpn instance [extensive]` |
| EVPN路由 | `display bgp evpn all routing-table [type {1-5}]` | `display bgp l2vpn evpn routing-table` | `show bgp l2vpn evpn [route-type {1-5}]` | `show evpn database` / `show route table <inst>.evpn.0` |
| BD/BD信息 | `display bridge-domain` | - | `show l2vpn bridge-domain [detail]` | `show vpls connections [extensive]` |
| L2VPN伪线 | - | - | `show l2vpn xconnect [detail]` | `show l2vpn connections [extensive]` |

---

## 10.6 QoS/安全/可靠性

| 场景 | 华为 NE40E | H3C S12500X | Cisco IOS-XR | Juniper Junos |
|:---|:---|:---|:---|:---|
| QoS策略统计 ★ | `display traffic policy statistics interface <if>` | `display qos policy interface <if>` | `show policy-map interface <if>` | `show class-of-service interface <if>` |
| 队列统计 ★ | `display qos queue statistics interface <if>` | `display qos queue statistics interface <if>` | `show qos interface <if>` | `show class-of-service interface <if> queue` |
| ACL统计 | `display acl <number> [detail]` | `display acl <number> counter` | `show access-lists [<name>]` | `show firewall [filter <name>]` |
| CPU/RE防护 | `display cpu-defend statistics` | - | `show lpts pifib hardware police` | `show firewall filter protect-re` |
| URPF统计 | `display urpf statistics interface <if>` | - | `show cef drops` | `show route forwarding-table` |
| BFD会话 ★ | `display bfd session all [verbose]` | `display bfd session [verbose]` | `show bfd session [detail]` | `show bfd session [detail\|extensive]` |
| VRRP状态 ★ | `display vrrp [brief]` | `display vrrp [verbose]` | `show vrrp [brief\|detail]` / `show hsrp` | `show vrrp [detail\|extensive]` |
| NQA/SLA结果 | `display nqa results` | - | `show ipsla statistics [detail]` | `show services rpm probe-results` |
| 接口计数器 ★ | `display interface <if>` | `display interface <if>` | `show interface <if>` / `show controllers <if>` | `show interfaces <if> [detail\|extensive]` |
| 光模块 | `display transceiver [verbose]` | `display transceiver interface <if>` | `show controllers <if> phy` | `show interfaces diagnostics optics <if>` |
| NetFlow/采集 ★ | `display ip netstream [cache]` | - | `show flow monitor [cache]` | `show services accounting flow` |
| Telemetry ★ | `display telemetry subscription` | - | `show telemetry model-driven subscription` | `show analytics agent` |
| PCEP ★ | `display pce peer` | - | `show pce ipv4 peer` | `show path-computation-client active-pce` |

---

## 10.7 系统级诊断

| 场景 | 华为 NE40E | Cisco IOS-XR | Juniper Junos |
|:---|:---|:---|:---|
| 系统日志 ★ | `display logbuffer` | `show logging [last <n>]` | `show log messages [last <n>]` |
| 告警 | `display alarm [active]` | `show alarms [brief\|detail]` | `show system alarms` |
| CPU使用 | `display cpu-usage` | `show processes cpu [history]` | `show chassis routing-engine` |
| 内存使用 | `display memory-usage` | `show memory summary` | `show system memory` |
| 配置 | `display current-configuration` | `show running-config` | `show configuration [<hierarchy>]` |
| 硬件状态 | `display device` | `show platform [detail]` | `show chassis hardware [detail]` |
| 崩溃信息 | `display diagnostic-information` | `show context [all]` | `show system core-dumps` |
| CEF丢包 ★★★ | `display fib statistics` | `show cef drops` | `show pfe statistics traffic` |
