# 第十二章 跨特性信息关联分析

> 本章描述各网络特性之间的调用、依赖和联动关系，供排障工具进行关联分析

## 12.1 BGP 关联图

```
BGP ──┬── route-policy ──┬── if-match ip-prefix <name>          → IP前缀列表
      │                   ├── if-match as-path-filter <number>    → AS路径过滤器
      │                   ├── if-match community-filter <name>    → Community过滤器
      │                   ├── if-match extcommunity-filter <name> → RT过滤器(VPN)
      │                   ├── apply local-preference <value>      → LP属性
      │                   ├── apply community <value> [additive]  → 设置Community
      │                   ├── apply extcommunity rt <value>       → 设置RT(VPN)
      │                   ├── apply as-path <as> additive         → AS-Path Prepend
      │                   └── apply cost <value>                  → 设置MED
      │
      ├── filter-policy ──── acl <number>                          → ACL过滤
      │                   └── ip-prefix <name>                     → 前缀过滤
      │
      ├── peer ──── bfd enable                                     → BFD联动(快速检测)
      │          ├── password / keychain                           → 认证
      │          ├── valid-ttl-hops                                → GTSM安全
      │          └── connect-interface                             → 更新源
      │
      ├── ipv4-family vpnv4 ─── VPN ─── tunnel-policy             → 隧道策略(见12.4)
      │
      ├── l2vpn-family evpn ─── EVPN ─── bridge-domain            → BD/VNI
      │
      └── ipv4-family flow ─── Flow Specification                  → 流规则(安全)
```

### BGP排障关联链
```
BGP邻居不建立 → 检查:
  1. peer配置 → connect-interface可达?
  2. TCP 179 → ACL是否放行?
  3. GTSM → valid-ttl-hops是否匹配?
  4. 认证 → password/keychain是否一致?
  5. BFD → BFD震荡导致频繁reset?

BGP路由不通告 → 检查:
  1. route-policy export → 是否deny了该路由?
  2. ip-prefix → 前缀范围是否覆盖?
  3. as-path-filter → 是否过滤了AS路径?
  4. community-filter → 是否匹配了no-advertise?
  5. network/import-route → 是否宣告/引入?
```

---

## 12.2 OSPF/IS-IS 关联图

```
OSPF ──┬── route-policy(import-route时) ──── if-match / apply
       ├── filter-policy ──── acl / ip-prefix / route-policy
       ├── BFD ──── ospf bfd enable (接口级)
       ├── VRRP ──── 与VRRP同接口部署
       ├── SR ──── segment-routing mpls (OSPF使能SR)
       │         └── prefix-sid (Loopback接口)
       ├── FRR ──── loop-free-alternate / ti-lfa
       └── 被BGP引入 ──── bgp → import-route ospf [ route-policy ]

IS-IS ──┬── route-policy(import-route/路由泄露时)
        ├── filter-policy ──── acl / ip-prefix / route-policy
        ├── BFD ──── isis bfd enable (接口级)
        ├── SR ──── segment-routing mpls (IS-IS使能SR)
        │         ├── prefix-sid (Loopback)
        │         ├── adjacency-sid (接口)
        │         └── flex-algo
        ├── FRR ──── ti-lfa / ti-lfa node-protection
        ├── micro-loop-avoidance ──── segment-routing
        └── 被BGP引入 ──── bgp → import-route isis [ route-policy ]
```

---

## 12.3 VPN 关联图

```
L3VPN ──┬── ip vpn-instance ──── route-distinguisher (RD)
        │                     ├── vpn-target (RT import/export)
        │                     ├── tunnel-policy ──── 隧道选择(见12.4)
        │                     ├── route-limit ──── 路由数量限制
        │                     ├── import/export route-policy ──── 路由策略控制
        │                     └── segment-routing ipv6 locator ──── SRv6关联
        │
        ├── 接口 ──── ip binding vpn-instance ──── ★绑定后IP清除
        │
        ├── PE-CE BGP ──── bgp → ipv4-family vpn-instance
        │                  ├── peer + as-number (CE邻居)
        │                  ├── import-route (引入CE路由)
        │                  └── route-policy (CE方向过滤)
        │
        ├── PE间VPNv4 ──── bgp → ipv4-family vpnv4
        │                  ├── peer (PE/RR邻居)
        │                  └── reflect-client (RR反射)
        │
        └── RT交叉 ──── vpn-target import引入其他VPN的RT → VPN间路由泄露

EVPN ──┬── evpn vpn-instance ──── bd-mode / vpws-mode
       ├── bridge-domain ──── evpn binding → VXLAN VNI
       ├── IRB ──── Vbdif接口 → ip binding vpn-instance(L3VPN)
       └── BGP ──── l2vpn-family evpn → peer enable
```

### VPN排障关联链
```
VPN业务不通 → 完整检查链:
  1. VPN实例 → RD唯一? RT匹配(export↔import)?
  2. 接口绑定 → ip binding正确? IP配好?
  3. PE-CE BGP → Established? 路由引入?
  4. PE间VPNv4 → Established? RR反射? 路由传播?
  5. 隧道策略 → tunnel-policy绑定? 隧道类型匹配?
  6. 传输隧道 → LDP LSP/SR Policy/TE Tunnel到远端PE可达?
  7. 标签 → VPN标签+传输标签完整?
  8. FIB → display fib vpn-instance确认下发?
```

---

## 12.4 隧道策略 关联图

```
tunnel-policy ──── tunnel select-seq 优先级顺序:
  │
  ├── sr-te-policy ──── segment-routing te policy
  │                      ├── color + end-point
  │                      ├── candidate-path → segment-list (标签栈)
  │                      ├── 或 dynamic → metric-type (igp/te/latency)
  │                      ├── 或 pce-client (控制器下发)
  │                      └── 或 on-demand (ODN按需)
  │
  ├── cr-lsp ──── RSVP-TE隧道
  │               ├── explicit-path (显式路径)
  │               ├── signalled-bandwidth (信令带宽)
  │               ├── priority (抢占优先级)
  │               └── fast-reroute (FRR保护)
  │
  ├── ldp ──── LDP LSP (IGP最短路径)
  │            ├── transport-address (传输地址)
  │            └── igp-sync (IGP同步)
  │
  ├── sr-be ──── SR-MPLS BE (IGP最短路径+SR标签)
  │
  ├── srv6-te-policy ──── SRv6 TE Policy
  │
  └── srv6-be ──── SRv6 BE

# 消费者:
VPN ──── tunnel-policy <name> (在vpn-instance下绑定)
```

---

## 12.5 QoS 关联图

```
QoS MQC体系:
  traffic classifier ──── if-match acl <number>     → ACL(安全章节)
                      ├── if-match dscp/mpls-exp    → DSCP/EXP标记
                      └── if-match any
         ↓
  traffic behavior ──── car (CAR限速)
                    ├── remark dscp/mpls-exp/8021p  → 重标记(跨域QoS)
                    ├── redirect (策略路由)          → 关联路由策略
                    ├── queue af/ef/be/llq          → 队列调度
                    ├── queue wred                   → WRED随机早期丢弃
                    └── deny                         → 丢弃(类似ACL)
         ↓
  traffic policy ──── classifier + behavior 绑定
         ↓
  接口应用 ──── traffic-policy <name> inbound/outbound

# MPLS DiffServ:
  EXP值 ←→ 内部优先级映射 ←→ 队列调度
  入方向: 根据EXP/DSCP分类 → 映射本地优先级
  出方向: 根据本地优先级 → 映射出方向EXP/DSCP

# QoS与SR:
  SR TE Policy → 可关联特定QoS策略
  color → 可映射到不同QoS处理
```

---

## 12.6 VRRP 联动关联图

```
VRRP ──┬── track interface <if> reduced <value>
       │    → 上行接口Down时降低VRRP优先级 → 触发主备切换
       │
       ├── track bfd-session <session> reduced <value>
       │    → BFD检测到链路故障时降优先级 → 毫秒级切换
       │    → BFD ──── 物理链路/对端设备存活检测
       │
       ├── track nqa <admin> <test> reduced <value>
       │    → NQA探测目的不可达时降优先级
       │    → NQA ──── ICMP/TCP/UDP/LSP-Ping探测
       │
       └── VRRP本身 ──── 认证(simple/md5)
                      ├── preempt-mode timer delay
                      └── 同接口OSPF/IS-IS(网关冗余+路由冗余)
```

---

## 12.7 BFD 联动全景

```
BFD ──── 被以下协议/特性调用:
  │
  ├── OSPF ──── ospf bfd enable (接口级)
  │             → BFD Down → OSPF邻居立即Down → SPF重算
  │
  ├── IS-IS ──── isis bfd enable (接口级)
  │              → BFD Down → IS-IS邻居立即Down → SPF重算
  │
  ├── BGP ──── peer <ip> bfd enable
  │            → BFD Down → BGP会话立即Reset → 路由撤回
  │
  ├── LDP ──── bfd enable / bfd all-interface
  │            → BFD Down → LDP会话快速检测
  │
  ├── 静态路由 ──── ip route-static ... bfd enable
  │                 → BFD Down → 静态路由失效 → 切换备用路由
  │
  ├── VRRP ──── vrrp vrid <vrid> track bfd-session
  │             → BFD Down → VRRP降优先级 → 主备切换
  │
  ├── MPLS TE ──── mpls te bfd enable
  │                → BFD Down → TE隧道快速切换/FRR
  │
  ├── SR TE Policy ──── policy → bfd enable
  │                     → BFD Down → SR Policy候选路径切换
  │
  └── MPLS LSP ──── bfd bind mpls-lsp
                    → BFD Down → LSP故障检测
```

---

## 12.8 安全特性关联

```
ACL ──── 被以下特性调用:
  ├── 接口过滤 ──── traffic-filter acl <number>
  ├── QoS流分类 ──── traffic classifier → if-match acl <number>
  ├── BGP路由过滤 ──── peer filter-policy acl <number>
  ├── OSPF路由过滤 ──── filter-policy acl <number>
  ├── CPU防护 ──── cpu-defend → blacklist/whitelist acl <number>
  ├── URPF流模式 ──── traffic behavior → urpf
  ├── NAT ──── nat address-group → acl匹配
  └── 策略路由 ──── traffic classifier → if-match acl → redirect

CPU防护 ──┬── 协议CAR限速 → 保护各协议报文(BGP/OSPF/ISIS/LDP等)
          ├── 黑名单(ACL/prefix) → 直接丢弃攻击流量
          ├── 白名单(ACL/prefix) → 优先处理可信流量
          └── GTSM ──── BGP peer valid-ttl-hops → TTL安全

URPF ──── strict模式 → 源IP必须FIB存在+入接口匹配
       └── loose模式 → 源IP FIB存在即可(允许非对称路由)
       ★ strict模式+非对称路由 = 误丢包(关联路由策略/IGP分析)
```

---

## 12.9 监控特性关联

```
NQA ──── 被以下特性调用:
  ├── 静态路由 ──── track nqa → 路由切换
  ├── VRRP ──── track nqa → 优先级降级
  ├── Track对象 ──── nqa track <id> → 统一联动
  └── 独立使用 ──── SLA监控(时延/抖动/丢包)

Telemetry ──── 数据源:
  ├── 接口统计(ifm) → 流量/错误/丢弃
  ├── BGP路由(bgp) → 路由数/邻居状态
  ├── QoS统计(qos) → 队列深度/丢包
  └── 订阅模式: sensor-group → destination-group → subscription

IFIT ──── 关联:
  ├── ACL ──── 流匹配(match acl)
  ├── SR Policy ──── 隧道级检测
  └── 输出到 ──── Telemetry/采集器

NetStream ──── 关联:
  ├── ACL ──── 采样过滤
  └── 输出格式 ──── v5/v9/IPFIX → 采集器
```

---

## 12.10 完整关联矩阵(速查)

| 特性A | 调用/依赖 特性B | 关联方式 |
|:---|:---|:---|
| BGP | route-policy | peer route-policy import/export |
| BGP | ip-prefix | route-policy → if-match ip-prefix |
| BGP | as-path-filter | route-policy → if-match as-path-filter |
| BGP | community-filter | route-policy → if-match community-filter |
| BGP | ACL | peer filter-policy acl |
| BGP | BFD | peer bfd enable |
| BGP | Keychain/GTSM | peer keychain / peer valid-ttl-hops |
| OSPF/IS-IS | route-policy | import-route route-policy / filter-policy |
| OSPF/IS-IS | BFD | ospf/isis bfd enable (接口) |
| OSPF/IS-IS | SR | segment-routing mpls + prefix-sid |
| OSPF/IS-IS | TI-LFA | frr ti-lfa |
| VPN | tunnel-policy | vpn-instance → tunnel-policy |
| VPN | BGP VPNv4 | ipv4-family vpnv4 |
| VPN | RT | vpn-target import/export |
| VPN | route-policy | import/export route-policy |
| SR Policy | PCEP | pce-client → connect-server |
| SR Policy | BFD | policy → bfd enable |
| SR Policy | Flex-Algo | prefix-sid algorithm <algo-id> |
| QoS | ACL | traffic classifier → if-match acl |
| QoS | DSCP/EXP | classifier → if-match dscp/mpls-exp |
| VRRP | BFD | track bfd-session |
| VRRP | NQA | track nqa |
| VRRP | 接口状态 | track interface |
| 静态路由 | BFD | bfd enable |
| 静态路由 | NQA | track nqa |
| 静态路由 | Track | track bfd-session |
| CPU防护 | ACL | blacklist/whitelist acl |
| URPF | FIB/路由 | strict需入接口匹配 |
| NQA | Track | nqa track <id> |
| Telemetry | gRPC | grpc server |
| IFIT | ACL | match acl |
| NetStream | ACL | 采样过滤 |

---

## 12.10 四厂商命令映射关系

> 同一功能在不同厂商的命令入口对照，供跨厂商排障和迁移参考

### 12.10.1 路由协议命令映射

| 功能 | 华为 NE40E | H3C S12500X | Cisco IOS-XR | Juniper Junos |
|:---|:---|:---|:---|:---|
| OSPF进程 | `ospf <process-id>` | `ospf <process-id>` | `router ospf <process-id>` | `[edit protocols ospf]` |
| IS-IS进程 | `isis <process-id>` | `isis <process-id>` | `router isis <instance>` | `[edit protocols isis]` |
| BGP进程 | `bgp <as-number>` | `bgp <as-number>` | `router bgp <as-number>` | `[edit protocols bgp]` |
| BGP地址族 | `ipv4-family unicast` | `address-family ipv4` | `address-family ipv4 unicast` | `family inet { unicast; }` |
| VPNv4族 | `ipv4-family vpnv4` | `address-family vpnv4` | `address-family vpnv4 unicast` | `family inet-vpn { unicast; }` |
| EVPN族 | `l2vpn-family evpn` | `address-family l2vpn evpn` | `address-family l2vpn evpn` | `family evpn { signaling; }` |
| 路由策略 | `route-policy <name>` | `route-policy <name>` | `route-policy <name>` (RPL) | `policy-statement <name>` |
| 前缀列表 | `ip ip-prefix <name>` | `ip prefix-list <name>` | `prefix-set <name>` | `prefix-list <name>` |
| AS路径过滤 | `ip as-path-filter <n>` | `ip as-path <n>` | `as-path-set <name>` | `as-path <name>` |
| Community | `ip community-filter` | `ip community-list` | `community-set <name>` | `community <name>` |
| 静态路由 | `ip route-static` | `ip route-static` | `router static` | `[routing-options static]` |

### 12.10.2 MPLS/SR命令映射

| 功能 | 华为 NE40E | H3C S12500X | Cisco IOS-XR | Juniper Junos |
|:---|:---|:---|:---|:---|
| MPLS全局 | `mpls` + `mpls lsr-id` | `mpls enable` + `mpls lsr-id` | `mpls ldp / router-id` | `[protocols mpls]` |
| LDP接口 | 接口下`mpls ldp` | 接口下`mpls ldp enable` | `mpls ldp / interface` | `[protocols ldp] interface` |
| SRGB | `global-block <min> <max>` | `global-block <min> <max>` | `global-block <min> <max>` | `srgb start-label <min> index-range` |
| Prefix SID | `prefix-sid index <idx>` | `prefix-sid index <idx>` | `prefix-sid [index <idx>]` | `node-segment { ipv4-index; }` |
| SR-TE Policy | `segment-routing te / policy` | - | `traffic-eng / policy` | `source-routing-path` |
| SRv6 Locator | `segment-routing ipv6 / locator` | - | `srv6 / locator` | `srv6 / locator` |
| TE隧道 | `interface Tunnel / mpls te` | `interface Tunnel` | `interface tunnel-te` | `label-switched-path` |

### 12.10.3 VPN命令映射

| 功能 | 华为 NE40E | H3C S12500X | Cisco IOS-XR | Juniper Junos |
|:---|:---|:---|:---|:---|
| VPN实例 | `ip vpn-instance` | `ip vpn-instance` | `vrf <name>` | `routing-instances { instance-type vrf; }` |
| RD | `route-distinguisher` | `route-distinguisher` | `rd <rd>` | `route-distinguisher` |
| RT | `vpn-target <rt> both` | `vpn-target <rt> both` | `import/export route-target` | `vrf-target <rt>` |
| 接口绑定VPN | `ip binding vpn-instance` | `ip binding vpn-instance` | `vrf <name>` (接口下) | `interface` (instances下) |
| EVPN | `evpn vpn-instance bd-mode` | - | `evpn / evi <id>` | `[protocols evpn]` |

### 12.10.4 QoS命令映射

| 功能 | 华为 NE40E | H3C S12500X | Cisco IOS-XR | Juniper Junos |
|:---|:---|:---|:---|:---|
| 流分类 | `traffic classifier` | `traffic classifier` | `class-map` | `classifiers` |
| 行为/调度 | `traffic behavior` | `traffic behavior` | `policy-map / class` | `schedulers` |
| 策略 | `traffic policy` | `qos policy` | `policy-map` | `scheduler-maps` |
| 应用 | `traffic-policy inbound/outbound` | `qos apply policy` | `service-policy input/output` | `scheduler-map` (接口) |
| 监管 | `car cir` | `car cir` | `police rate` | `policer` (firewall) |

### 12.10.5 安全命令映射

| 功能 | 华为 NE40E | H3C S12500X | Cisco IOS-XR | Juniper Junos |
|:---|:---|:---|:---|:---|
| ACL | `acl <number>` | `acl {basic/advanced}` | `ipv4 access-list` | `firewall filter` |
| ACL应用 | `traffic-filter acl` | `packet-filter` | `access-group ingress/egress` | `filter input/output` |
| CPU防护 | `cpu-defend policy` | - | `lpts punt police` | `lo0 filter protect-re` |
| GTSM | `peer valid-ttl-hops` | - | `ttl-security` | firewall filter匹配TTL |

### 12.10.6 监控命令映射

| 功能 | 华为 NE40E | Cisco IOS-XR | Juniper Junos |
|:---|:---|:---|:---|
| 探测/SLA | `nqa test-instance` | `ipsla operation` | `services rpm` |
| NetFlow/采集 | `ip netstream` | `flow monitor-map / exporter-map / sampler-map` | `services flow-monitoring` + `forwarding-options sampling` |
| sFlow | `sflow` | `sflow` | `forwarding-options sampling` |
| Telemetry | `telemetry sensor-group/subscription` | `telemetry model-driven sensor-group/subscription` | `services analytics sensor/streaming-server` |
| PCEP | `pce pce-client connect-server` | `pcc pce address` | `[protocols pcep] pce <name>` |
