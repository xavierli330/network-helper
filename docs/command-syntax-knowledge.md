# 命令语法知识库 — Command Syntax Knowledge Base

> 本文档是 **命令参数分离 (stripArgs)** 的参考知识。
> 用于精确识别命令字符串中 "哪些词是参数、哪些词是关键字"。
>
> 维护者：网络工程师 + AI 协作
> 最后更新：2026-03-26

---

## 1. 总论：参数在命令中的位置规律

网络设备 CLI 命令的通用结构为：

```
<verb> <object-keyword> [<qualifier-keyword> <argument>]* [<modifier>] [<argument>]
```

**关键原则：参数不只在尾部，可以出现在命令的任意位置。**

参数的三种典型位置：

| 位置 | 说明 | 例子 |
|------|------|------|
| **尾部** | 最常见，指定 ID/IP/接口名 | `display vlan 100` |
| **中间** | qualifier-keyword 后跟参数，后面还有关键字 | `display bgp vpnv4 vpn-instance VPN_A peer 10.0.0.1` |
| **嵌入式** | 接口名作为中间对象 | `display interface GigabitEthernet0/0/1 brief` |

### 参数的识别特征

| 类型 | 正则/模式 | 说明 |
|------|-----------|------|
| 纯数字 | `^\d+$` | VLAN ID, AS号, 聚合组ID, 链路索引 |
| IPv4 地址 | `^\d+\.\d+\.\d+\.\d+(/\d+)?$` | Peer IP, 路由前缀 |
| IPv6 地址 | 包含 `:` | Peer IPv6 |
| 接口名 | `GigabitEthernet0/0/1`, `Eth-Trunk1` 等 | 物理/逻辑接口全名 |
| VPN/VRF 名 | 非保留关键字的自由文本 | `vpn-instance` 后面的参数 |
| 文件名/路径 | 包含 `.` 或 `/` | 如 `flash:/xxx.cfg` |
| MAC 地址 | `xxxx-xxxx-xxxx` (华为) 或 `xx:xx:xx:xx:xx:xx` | |

---

## 2. 华为 VRP (Huawei)

### 2.1 命令结构

华为 VRP 命令遵循 `display <对象> [限定符 <参数>]* [修饰符]` 结构。

常见动词：`display`, `ping`, `tracert`, `reset`, `debugging`

### 2.2 参数位置清单

#### `display interface`
```
display interface [<interface-type> <interface-number>] [brief]
```
- `<interface-type> <interface-number>` → 中间参数（可选）
- 例：`display interface GigabitEthernet0/0/1` — 接口名是参数
- 例：`display interface brief` — `brief` 是关键字（修饰符）

#### `display ip routing-table`
```
display ip routing-table [vpn-instance <name>] [<ip-prefix> [<mask-length>]] [verbose]
```
- `vpn-instance <name>` → 中间 keyword+argument 对
- `<ip-prefix>` → 中间/尾部参数
- 例：`display ip routing-table vpn-instance VPN_A 10.0.0.0 24 verbose`
  - 参数：`VPN_A`（VPN名）, `10.0.0.0`（IP）, `24`（掩码长度）
  - 关键字：`vpn-instance`, `verbose`

#### `display bgp`
```
display bgp [vpnv4|vpnv6] [vpn-instance <name>] peer [<ip-address>] [verbose]
display bgp [vpnv4|vpnv6] [vpn-instance <name>] routing-table [<ip-prefix> <mask>]
```
- `vpnv4`, `vpnv6` → 关键字（地址族）
- `vpn-instance <name>` → keyword+argument
- `peer <ip>` → keyword+argument
- 例：`display bgp vpnv4 vpn-instance CORP peer 10.0.0.1 verbose`
  - 参数：`CORP`, `10.0.0.1`
  - 关键字：`vpnv4`, `vpn-instance`, `peer`, `verbose`

#### `display ospf`
```
display ospf [<process-id>] peer [brief]
display ospf [<process-id>] routing
display ospf [<process-id>] lsdb [area <area-id>]
```
- `<process-id>` → 中间参数（纯数字，进程号）
- `area <area-id>` → keyword+argument
- 例：`display ospf 100 peer brief` — `100` 是参数

#### `display isis`
```
display isis [<process-id>] peer [verbose]
display isis [<process-id>] route
display isis [<process-id>] lsdb [level-1|level-2]
```

#### `display mpls`
```
display mpls lsp [verbose] [ingress|transit|egress]
display mpls ldp session [<peer-ip>] [verbose]
display mpls ldp peer [<peer-ip>]
display mpls te tunnel [interface Tunnel<id>]
display mpls forwarding-table [<prefix> <mask>]
```
- `<peer-ip>` → 中间参数
- `Tunnel<id>` → 接口名参数

#### `display link-aggregation`
```
display link-aggregation verbose [bridge-aggregation <id>]
display eth-trunk [<trunk-id>]
```
- `bridge-aggregation <id>` → keyword+argument
- `<trunk-id>` → 尾部纯数字参数

#### `display vlan`
```
display vlan [<vlan-id>] [verbose]
```

#### `display current-configuration`
```
display current-configuration [interface <interface-type> <interface-number>]
display current-configuration [configuration <section>]
display saved-configuration
```
- `interface <type> <number>` → keyword+argument+argument

#### `display arp`
```
display arp [interface <interface-name>]
display arp [<ip-address>]
display arp [vpn-instance <name>]
```

#### `display mac-address`
```
display mac-address [<mac-address>] [vlan <vlan-id>] [interface <interface-name>]
```

### 2.3 参数触发关键字 (Qualifier Keywords)

以下关键字后面紧跟参数值：

| 关键字 | 参数类型 | 示例 |
|--------|----------|------|
| `vpn-instance` | VPN 名称(string) | `vpn-instance CORP` |
| `peer` | IP 地址 | `peer 10.0.0.1` |
| `interface` | 接口名 | `interface GigabitEthernet0/0/1` |
| `vlan` | VLAN ID(number) | `vlan 100` |
| `area` | Area ID(number/ip) | `area 0.0.0.0` |
| `tunnel` | Tunnel ID(number) | `tunnel 100` |
| `bridge-aggregation` | BAGG ID(number) | `bridge-aggregation 1` |
| `eth-trunk` | Trunk ID(number) | `eth-trunk 1` |
| `acl` | ACL编号/名称 | `acl 3001` |
| `route-policy` | 策略名称 | `route-policy RP_EXPORT` |
| `community` | Community 值 | `community 65000:100` |

### 2.4 修饰符关键字 (Modifiers — 不是参数)

| 关键字 | 说明 |
|--------|------|
| `brief` | 简要模式 |
| `verbose` | 详细模式 |
| `detail` | 详细模式 |
| `summary` | 汇总模式 |
| `ingress` / `transit` / `egress` | MPLS 方向 |
| `level-1` / `level-2` | ISIS 级别 |
| `statistics` | 统计信息 |

---

## 3. H3C Comware

### 3.1 概述

H3C Comware 命令语法与华为 VRP 高度相似（都源自 Comware 平台），但有细微差异。

### 3.2 关键差异

| 特征 | 华为 VRP | H3C Comware |
|------|----------|-------------|
| 聚合接口 | `Eth-Trunk` | `Bridge-Aggregation`, `Route-Aggregation` |
| VLAN 接口 | `Vlanif` | `Vlan-interface` |
| 配置命令 | `display current-configuration` | `display current-configuration` (相同) |
| 聚合详情 | `display eth-trunk 1` | `display link-aggregation verbose bridge-aggregation 1` |

### 3.3 额外参数触发关键字

| 关键字 | 参数类型 | 示例 |
|--------|----------|------|
| `bridge-aggregation` | BAGG ID | `bridge-aggregation 1` |
| `route-aggregation` | RAGG ID | `route-aggregation 1` |
| `vlan-interface` | VLAN ID | `vlan-interface 100` |
| `service-instance` | 实例ID | `service-instance 10` |

---

## 4. Cisco IOS / IOS-XR

### 4.1 命令结构

Cisco 命令遵循 `show <object> [qualifier argument]* [modifier]` 结构。

### 4.2 参数位置清单

#### `show ip route` / `show route`
```
show ip route [vrf <name>] [<prefix>] [<mask>] [longer-prefixes]
show route [vrf <name>] [ipv4|ipv6] [<prefix>/<length>]
```
- `vrf <name>` → keyword+argument
- `<prefix>/<length>` → 尾部或中间参数

#### `show bgp`
```
show bgp [vpnv4|vpnv6] [unicast] [vrf <name>] summary
show bgp [vpnv4|vpnv6] [unicast] [vrf <name>] neighbors [<ip-address>] [advertised-routes|received-routes]
show bgp [ipv4|ipv6] unicast <prefix>/<length>
```
- `vrf <name>` → keyword+argument (中间)
- `neighbors <ip>` → keyword+argument
- `<prefix>/<length>` → 尾部参数

#### `show interface`
```
show interface [<interface-name>] [brief|status|description|accounting]
show ip interface [brief]
```
- `<interface-name>` → 中间参数
- 例：`show interface GigabitEthernet0/0/0 brief` — 接口名是中间参数

#### `show ospf` / `show ip ospf`
```
show ip ospf [<process-id>] neighbor [<interface-name>] [detail]
show ip ospf [<process-id>] database [router|network|summary|external] [<link-state-id>]
```

#### `show mpls`
```
show mpls forwarding-table [<prefix> <mask>] [detail]
show mpls ldp neighbor [<ip-address>] [detail]
show mpls traffic-eng tunnel [<tunnel-id>]
```

#### `show running-config`
```
show running-config [interface <interface-name>]
show running-config [router bgp <as-number>]
show running-config [| include <pattern>]
```

#### `show arp`
```
show arp [vrf <name>] [<ip-address>]
show arp [interface <interface-name>]
```

### 4.3 参数触发关键字

| 关键字 | 参数类型 | 示例 |
|--------|----------|------|
| `vrf` | VRF 名称 | `vrf MGMT` |
| `neighbor` / `neighbors` | IP 地址 | `neighbors 10.0.0.1` |
| `interface` | 接口名 | `interface GigabitEthernet0/0/0` |
| `vlan` | VLAN ID | `vlan 100` |
| `area` | Area ID | `area 0` |
| `tunnel` | Tunnel ID | `tunnel 100` |
| `port-channel` | PC ID | `port-channel 1` |
| `access-list` | ACL 名/编号 | `access-list 101` |
| `route-map` | 策略名 | `route-map RM_OUT` |
| `as-path` | AS 路径 | |

### 4.4 Cisco IOS-XR 特有

IOS-XR 的命令结构更接近 Juniper，有层次化视图：
```
show bgp vpnv4 unicast vrf VPN_A neighbors 10.0.0.1 advertised-routes
```
- 参数：`VPN_A`, `10.0.0.1`
- 关键字：`vpnv4`, `unicast`, `vrf`, `neighbors`, `advertised-routes`

---

## 5. Juniper JUNOS

### 5.1 命令结构

Juniper 命令结构：`show <object-path> [modifier]`

JUNOS 比较特殊 — 对象路径本身可以很长且分层。

### 5.2 参数位置清单

#### `show route`
```
show route [table <table-name>] [<prefix>] [detail|extensive|terse]
show route advertising-protocol bgp <neighbor-ip>
show route receive-protocol bgp <neighbor-ip>
show route instance <instance-name>
```
- `table <name>` → keyword+argument
- `<prefix>` → 中间/尾部参数
- `instance <name>` → keyword+argument
- `advertising-protocol bgp <ip>` → keyword+keyword+argument

#### `show bgp`
```
show bgp summary
show bgp neighbor [<ip-address>]
show bgp group [<group-name>]
```

#### `show interface`
```
show interface [<interface-name>] [terse|extensive|detail|descriptions]
show interface [<interface-name>] statistics
```
- `<interface-name>` 格式：`ge-0/0/0`, `xe-0/0/1`, `ae0`, `irb.100`, `lo0.0`

#### `show ospf`
```
show ospf [instance <name>] neighbor [<interface-name>] [detail|extensive]
show ospf [instance <name>] database [router|network|nssa|external] [area <area>]
show ospf [instance <name>] route
```

#### `show ldp`
```
show ldp session [<ip-address>] [detail|extensive]
show ldp neighbor [<ip-address>]
```

#### `show rsvp`
```
show rsvp session [<destination>] [detail|extensive]
show rsvp interface [<interface-name>]
```

#### `show configuration`
```
show configuration [<hierarchy-path>]
show configuration | display set
show configuration interfaces <interface-name>
show configuration routing-instances <instance-name>
show configuration protocols bgp group <group-name>
```

### 5.3 Juniper 参数触发关键字

| 关键字 | 参数类型 | 示例 |
|--------|----------|------|
| `table` | 路由表名 | `table inet.0` |
| `instance` | 路由实例名 | `instance VPN_A` |
| `neighbor` | IP 地址 | `neighbor 10.0.0.1` |
| `group` | BGP 组名 | `group IBGP` |
| `interface` | 接口名 | `interface ge-0/0/0` |
| `area` | Area ID | `area 0.0.0.0` |

### 5.4 Juniper 接口名格式

| 格式 | 说明 |
|------|------|
| `ge-X/Y/Z` | Gigabit Ethernet |
| `xe-X/Y/Z` | 10G Ethernet |
| `et-X/Y/Z` | 40G/100G Ethernet |
| `ae<N>` | Aggregated Ethernet |
| `irb.<unit>` | Integrated Routing & Bridging |
| `lo0.<unit>` | Loopback |
| `em0` / `fxp0` | Management |

---

## 6. 跨厂商参数分离规则（给 stripArgs 的实现指导）

### 6.1 全局规则

1. **所有纯数字** 在特定关键字后 → 参数（VLAN ID, AS 号, 进程 ID, 掩码长度等）
2. **IPv4/IPv6 地址** 在任何位置 → 参数
3. **接口名** 在任何位置 → 参数（匹配 interfaceNameRe）
4. **已知 qualifier-keyword 后面紧跟的一个词** → 参数

### 6.2 Qualifier Keywords 统一列表

以下关键字后面紧跟的一个 token 应被识别为参数：

```go
var qualifierKeywords = map[string]bool{
    // VRF/VPN
    "vpn-instance": true, "vrf": true, "instance": true,
    // Peer/Neighbor
    "peer": true, "neighbor": true, "neighbors": true,
    // Interface/Port
    "interface": true, "bridge-aggregation": true, "route-aggregation": true,
    "eth-trunk": true, "port-channel": true,
    // Area/Zone
    "area": true,
    // Routing Table
    "table": true,
    // BGP Group
    "group": true,
    // ACL/Policy
    "acl": true, "access-list": true, "route-policy": true, "route-map": true,
    // VLAN
    "vlan": true, "vlan-interface": true,
    // Tunnel
    "tunnel": true,
    // Service
    "service-instance": true,
}
```

### 6.3 扫描算法（建议替换当前的尾部剥离）

```
for i := 1; i < len(words); i++:
    word = words[i]
    if word 是 qualifierKeyword:
        if i+1 < len(words):
            words[i+1] = 分类占位符(words[i+1])  // {ip}, {id}, {interface}, {name}
            i++  // skip the argument
    else if word 匹配 IP:
        words[i] = "{ip}"
    else if word 匹配 接口名:
        words[i] = "{interface}"
    else if word 是纯数字 且 前一个词不是动词:
        // 可能是进程 ID、VLAN ID 等
        words[i] = "{id}"
```

### 6.4 占位符类型

| 占位符 | 匹配内容 |
|--------|----------|
| `{id}` | 纯数字（VLAN ID, 进程号, 聚合组 ID, 掩码长度等） |
| `{ip}` | IPv4/IPv6 地址（含可选 CIDR mask） |
| `{interface}` | 接口全名（GigabitEthernet0/0/1, ae0, ge-0/0/0 等） |
| `{name}` | qualifier-keyword 后跟的自由文本参数（VPN 名, VRF 名, 策略名等） |
| `{prefix}` | IP 前缀（含掩码） |
| `{mac}` | MAC 地址 |

---

## 7. 维护指南

### 添加新厂商

1. 在本文档新增一个 section
2. 列出其命令动词 + 对象关键字 + qualifier keywords
3. 在 `collector.go` 的 `qualifierKeywords` map 中添加新关键字
4. 运行测试确认不误杀

### 添加新命令类型

1. 在对应厂商 section 下新增命令语法
2. 标注哪些位置是参数
3. 如果有新的 qualifier keyword，添加到 §6.2 的列表中

### 常见陷阱

- `display vlan` 中 `vlan` 既是对象又可以是 qualifier keyword（跟数字时是参数）
- `brief` / `verbose` / `detail` 永远是修饰符，不是参数
- 某些命令关键字本身包含数字（如 `level-1`, `ipv4`），不要误识别为参数
- 接口名可能包含 `/`（如 `0/0/1`），需要完整匹配
- AS 号可能是 4 字节点分格式（如 `65000.1`），看起来像不完整 IP
