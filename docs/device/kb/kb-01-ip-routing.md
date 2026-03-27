# 第一章 IP路由协议

## 1.1 静态路由

### 华为 NE40E 配置命令

```
# 基本静态路由
ip route-static <ip-address> { <mask> | <mask-length> } <nexthop-address> [ preference <pref> ] [ tag <tag> ] [ description <text> ]

# 指定出接口+下一跳
ip route-static <ip-address> { <mask> | <mask-length> } <interface-name> <nexthop-address>

# VPN实例静态路由
ip route-static vpn-instance <vpn-name> <dest> { <mask> | <mask-length> } <nexthop>

# 跨VPN静态路由
ip route-static vpn-instance <src-vpn> <dest> <mask> vpn-instance <dst-vpn> <nexthop>

# 联动BFD
ip route-static bfd <peer-ip> local-address <local-ip>
ip route-static <ip-address> <mask-length> <nexthop> bfd enable

# 联动NQA
ip route-static <ip-address> <mask-length> <interface> <nexthop> track nqa <admin> <test>

# 联动Track/EFM
ip route-static <ip-address> <mask-length> <nexthop> track bfd-session <session-name>
ip route-static <ip-address> <mask-length> <interface> <nexthop> track efm-state <interface>

# 黑洞路由
ip route-static <ip-address> <mask-length> NULL0

# 静态路由FRR
ip route-static frr
ip route-static <ip> <mask> <if> <nexthop> preference <pref> track bfd-session <session>
ip route-static <ip> <mask> <backup-if> <backup-nexthop>

# 删除
undo ip route-static <ip-address> { <mask> | <mask-length> } [ <nexthop> ]
```

**关键参数默认值**: preference=60, tag=无, bfd=关闭

### H3C S12500X 配置命令

```
ip route-static <dest> { <mask> | <mask-length> } { <nexthop> | <interface> [ <nexthop> ] } [ preference <pref> ] [ tag <tag> ] [ description <text> ]
ip route-static vpn-instance <vpn> <dest> <mask> <nexthop> [ preference <pref> ]
ip route-static <dest> <mask> <nexthop> bfd control-packet destination <dest-ip> source <src-ip>
ip route-static <dest> <mask> <nexthop> track <track-id>
ip route-static <dest> <mask> NULL 0
```

### Cisco IOS-XR (ASR9000) 配置命令

```
# 基本静态路由
router static
  address-family ipv4 unicast
    <prefix>/<mask-len> <nexthop-ip> [ <distance> ] [ description <text> ] [ tag <tag> ] [ track <object> ]
    <prefix>/<mask-len> <interface> <nexthop-ip>

# VRF静态路由
router static
  vrf <vrf-name>
    address-family ipv4 unicast
      <prefix>/<mask-len> <nexthop-ip>
      <prefix>/<mask-len> vrf <dest-vrf> <nexthop-ip>

# 黑洞路由
router static
  address-family ipv4 unicast
    <prefix>/<mask-len> Null0

# BFD联动
router static
  address-family ipv4 unicast
    <prefix>/<mask-len> <nexthop-ip> bfd fast-detect [ minimum-interval <ms> ] [ multiplier <mult> ]
```

**关键差异**: IOS-XR使用`router static`全局块，`address-family`区分IPv4/IPv6

### Juniper Junos 配置命令

```
# 基本静态路由
[edit routing-options]
static {
    route <prefix> {
        next-hop <nexthop-ip>;
        preference <pref>;          # 默认5
        tag <tag>;
        no-readvertise;
        retain;                     # 重启后保留
    }
}

# 多下一跳/ECMP
route <prefix> {
    next-hop [ <nexthop1> <nexthop2> ];
}

# 限定下一跳(Qualified Next-Hop，浮动静态)
route <prefix> {
    next-hop <primary-nh>;
    qualified-next-hop <backup-nh> {
        preference <higher-pref>;
    }
}

# BFD联动
route <prefix> {
    next-hop <nexthop-ip>;
    bfd-liveness-detection {
        minimum-interval <ms>;
        multiplier <mult>;
        local-address <local-ip>;
    }
}

# LSP下一跳(RSVP-TE/SR-TE)
route <prefix> {
    lsp-next-hop <lsp-name>;
    spring-te-lsp-next-hop <spring-te-lsp>;
}

# 黑洞路由
route <prefix> discard;

# VRF静态路由
[edit routing-instances <vrf-name> routing-options]
static {
    route <prefix> next-hop <nexthop>;
}
```

### Display/Show命令

```
# 路由表
华为: display ip routing-table [ <ip-address> [ <mask> ] [ verbose ] ]
H3C:  display ip routing-table [ <ip-address> [ verbose ] ]
Cisco: show route [ ipv4 ] [ <prefix> ] [ detail ]
Juniper: show route [ <prefix> ] [ detail | extensive | table <table> ]

# 静态路由
华为/H3C: display ip routing-table protocol static
Cisco: show route static
Juniper: show route protocol static

# VPN/VRF路由表
华为: display ip routing-table vpn-instance <vpn-name>
H3C:  display ip routing-table vpn-instance <vpn-name>
Cisco: show route vrf <vrf-name>
Juniper: show route table <instance-name>.inet.0

# FIB（转发平面真相）★
华为: display fib [ <ip-address> [ <mask-length> ] ]
H3C:  display fib [ <ip-address> ]
Cisco: show cef [ <prefix> ] [ detail ] / show cef vrf <vrf> [ <prefix> ]
Juniper: show route forwarding-table [ destination <prefix> ] [ table default ]
```

**路由表回显字段**:
| 字段 | 说明 | 排障关注 |
|:---|:---|:---|
| Proto | 路由来源(Static/OSPF/BGP) | 确认学习方式 |
| Pre | 优先级(越小越优) | 判断优选逻辑 |
| NextHop | 下一跳 | 确认转发方向 |
| Interface | 出接口 | 确认物理出口 |
| Flags: D | 已下发FIB | 无D=仅在RIB，未转发 |

---

## 1.2 OSPF

### 华为 NE40E 配置命令

```
# 进程与区域
ospf [ <process-id> ] [ router-id <router-id> ]
  area <area-id>
    network <ip-address> <wildcard-mask>
    stub [ no-summary ]                     # Totally Stub: no-summary
    nssa [ no-summary ] [ no-import-route ]

# 接口参数
interface <interface>
  ospf cost <cost>                          # 默认根据带宽自动计算
  ospf dr-priority <priority>               # DR优先级，默认1，0不参选
  ospf timer hello <interval>               # 默认10s
  ospf timer dead <interval>                # 默认40s(4×Hello)
  ospf authentication-mode { simple | md5 | hmac-md5 | hmac-sha256 } <key-id> [ cipher ] <password>
  ospf bfd enable                           # BFD联动
  ospf bfd min-tx-interval <ms> min-rx-interval <ms> detect-multiplier <mult>

# 路由汇总
area <area-id>
  abr-summary <ip> <mask> [ not-advertise ]       # ABR区域间汇总
asbr-summary <ip> <mask> [ not-advertise ]          # ASBR外部路由汇总

# 引入外部路由
import-route { static | direct | bgp | isis | rip } [ cost <cost> ] [ type <type> ] [ tag <tag> ] [ route-policy <name> ]

# 收敛优化
bandwidth-reference <value>                 # 参考带宽，建议100000(100G网络)
spf-schedule-interval <max> [ <init> <incr> ]
lsa-generation-interval <max> [ <init> <incr> ]

# GR平滑重启
graceful-restart

# Silent接口
silent-interface <interface>

# 过载标志(维护引流)
stub-router [ on-startup [ <seconds> ] ]
```

### H3C S12500X 配置命令

```
ospf [ <process-id> ] [ router-id <router-id> ]
  area <area-id>
    network <ip-address> <wildcard-mask>
    stub [ no-summary ] / nssa [ no-summary ]
  import-route { static | direct | bgp | isis } [ cost <cost> ] [ type <type> ] [ route-policy <name> ]

interface <interface>
  ospf cost <cost>
  ospf dr-priority <priority>
  ospf timer hello <interval> / ospf timer dead <interval>
  ospf authentication-mode { simple | md5 | hmac-md5 | hmac-sha256 } <key-id> [ cipher ] <password>
  ospf bfd enable
```

### Cisco IOS-XR (ASR9000) OSPF

```
router ospf <process-id>
  router-id <router-id>
  area <area-id>
    interface <interface>
      cost <cost>
      priority <priority>                        # DR优先级
      hello-interval <seconds>                   # 默认10s
      dead-interval <seconds>                    # 默认40s
      authentication message-digest
        message-digest-key <key-id> md5 [ encrypted ] <password>
      bfd minimum-interval <ms> / bfd multiplier <mult> / bfd fast-detect
      network { broadcast | non-broadcast | point-to-point | point-to-multipoint }
      mtu-ignore                                 # 忽略MTU检查
    stub [ no-summary ]
    nssa [ no-summary ] [ no-redistribution ]

  # 路由汇总
  area <area-id>
    range <prefix>/<mask-len> [ not-advertise ]            # ABR汇总
  summary-prefix <prefix>/<mask-len> [ not-advertise ]     # ASBR汇总

  # 引入外部路由
  redistribute { static | connected | bgp <as> | isis <tag> | rip } [ route-policy <name> ] [ metric <cost> ] [ metric-type <type> ] [ tag <tag> ]

  # 收敛优化
  auto-cost reference-bandwidth <mbps>           # 参考带宽
  timers throttle spf <init-ms> <incr-ms> <max-ms>
  timers throttle lsa all <init-ms> <incr-ms> <max-ms>

  # GR平滑重启
  nsf { cisco | ietf }

  # 过载标志(Stub Router)
  max-metric router-lsa [ on-startup <seconds> ] [ summary-lsa ] [ external-lsa ]

  # OSPFv3 (IPv6)
  router ospfv3 <process-id>
    area <area-id>
      interface <interface>
```

### Juniper Junos OSPF

```
[edit protocols ospf]
area <area-id> {
    interface <interface> {
        metric <cost>;
        priority <priority>;
        hello-interval <seconds>;
        dead-interval <seconds>;
        interface-type { p2p | p2mp | nbma };
        authentication {
            md5 <key-id> key <password>;
        }
        bfd-liveness-detection {
            minimum-interval <ms>;
            multiplier <mult>;
        }
    }
    stub [ default-metric <metric> ] [ no-summaries ];
    nssa {
        default-lsa { default-metric <metric>; no-summaries; }
        area-range <prefix>;
    }
    area-range <prefix>/<mask-len> [ restrict ];          # ABR区域间汇总(restrict=不通告)
}

# 引入外部路由
export <policy-name>;                                      # OSPF使用路由策略(policy-statement)引入

# SR使能
area <area-id> {
    interface <interface> {
        source-packet-routing {
            node-segment {
                ipv4-index <index>;
            }
        }
    }
}

# 过载
overload [ timeout <seconds> ];

# GR
graceful-restart;
```

### Display/Show命令

```
# OSPF邻居 ★排障首选
华为: display ospf peer [ brief ]
H3C:  display ospf peer [ verbose ]
Cisco: show ospf neighbor [ detail ]
Juniper: show ospf neighbor [ detail | extensive ]

# OSPF路由表
华为: display ospf routing
H3C:  display ospf routing
Cisco: show ospf route
Juniper: show ospf route [ detail ]

# OSPF LSDB
华为: display ospf lsdb [ router | network | summary | asbr | external | nssa ] [ area <area-id> ]
Cisco: show ospf database [ router | network | summary | asbr-summary | external | nssa-external ] [ area <area-id> ]
Juniper: show ospf database [ router | network | netsummary | asbrsummary | extern | nssa ] [ area <area-id> ] [ detail | extensive ]

# OSPF接口
华为/H3C: display ospf interface [ <interface> ]
Cisco: show ospf interface [ <interface> ] [ brief ]
Juniper: show ospf interface [ <interface> ] [ detail ]

# OSPF错误统计
华为: display ospf error
Cisco: show ospf statistics [ neighbor <ip> ]

# OSPF Flex-Algo / SR相关
Juniper: show ospf overview | show ospf database opaque-area
```

**OSPF邻居状态解读**:
| 状态 | 含义 | 排障方向 |
|:---|:---|:---|
| Down | 未收到Hello | 物理连通/ACL/认证 |
| Init | 单向Hello | 单向通信/MTU/认证不匹配 |
| ExStart | 主从协商 | **MTU不匹配(最常见卡此处)** |
| Exchange | DBD交换 | MTU/LSA过多超时 |
| Full | 邻接建立 | **Full ≠ 转发正常** |

---

## 1.3 IS-IS

### 华为 NE40E 配置命令

```
isis [ <process-id> ]
  network-entity <net>                      # 如 49.0001.0010.0100.1001.00
  is-level { level-1 | level-2 | level-1-2 }
  cost-style { narrow | wide | compatible }  # 建议wide

# 接口
interface <interface>
  isis enable [ <process-id> ]
  isis cost <cost> [ level-1 | level-2 ]
  isis circuit-type { p2p | broadcast }
  isis circuit-level { level-1 | level-2 | level-1-2 }
  isis bfd enable

# 认证
area-authentication-mode { simple | md5 | hmac-md5 | keychain } <password>
domain-authentication-mode { simple | md5 | hmac-md5 | keychain } <password>

# 路由泄露(L2→L1)
import-route isis level-2 into level-1 [ filter-policy route-policy <name> ]

# 引入外部路由
import-route { static | direct | ospf | bgp } [ level-1 | level-2 ] [ cost <cost> ] [ route-policy <name> ]

# SR使能
segment-routing mpls
segment-routing global-block <min-label> <max-label>

# 过载标志
set-overload [ on-startup [ <seconds> ] ]

# 收敛定时器
timer spf <max> <init> <incr>
timer lsp-generation <max> <init> <incr>

# GR
graceful-restart
```

### H3C 关键命令差异

```
isis [ <process-id> ]
  network-entity <net>
  cost-style { narrow | wide | compatible }
  segment-routing mpls
  segment-routing global-block <min> <max>

interface <interface>
  isis enable [ <process-id> ]
  isis cost <cost>
  isis bfd enable
```

### Cisco IOS-XR IS-IS

```
router isis <instance-tag>
  net <net>                                      # 如 49.0001.0010.0100.1001.00
  is-type { level-1 | level-2-only | level-1-2 }
  address-family ipv4 unicast
    metric-style wide                            # 建议wide
    segment-routing mpls
    segment-routing sr-prefer                    # SR优先于LDP

  interface <interface>
    circuit-type { level-1 | level-2-only | level-1-2 }
    point-to-point                               # P2P模式
    address-family ipv4 unicast
      metric <cost> [ level { 1 | 2 } ]
    bfd minimum-interval <ms> / bfd multiplier <mult> / bfd fast-detect
    hello-interval <seconds> [ level { 1 | 2 } ]
    hello-multiplier <mult> [ level { 1 | 2 } ]

  # 认证
  lsp-password { text | hmac-md5 | keychain } <password> [ level { 1 | 2 } ]

  # 路由泄露
  address-family ipv4 unicast
    propagate level 2 into level 1 route-policy <name>

  # 引入外部路由
  address-family ipv4 unicast
    redistribute { static | connected | ospf <process> | bgp <as> } [ route-policy <name> ] [ metric <cost> ] [ level { 1 | 2 } ]

  # SR使能
  interface Loopback <num>
    address-family ipv4 unicast
      prefix-sid [ index <index> | absolute <label> ]

  # 过载
  set-overload-bit [ on-startup <seconds> ]

  # 收敛
  lsp-gen-interval <max-ms> [ <init-ms> <incr-ms> ] [ level { 1 | 2 } ]
  spf-interval <max-ms> [ <init-ms> <incr-ms> ] [ level { 1 | 2 } ]

  # NSF (GR)
  nsf { cisco | ietf }
```

### Juniper Junos IS-IS

```
[edit protocols isis]
interface <interface> {
    level 1 { metric <cost>; disable; }
    level 2 { metric <cost>; }
    point-to-point;
    bfd-liveness-detection {
        minimum-interval <ms>;
        multiplier <mult>;
    }
}

level 1 { authentication-key <key>; authentication-type md5; wide-metrics-only; }
level 2 { authentication-key <key>; authentication-type md5; wide-metrics-only; }

# 路由泄露
level 1 {
    export <leak-policy>;                         # L2→L1用策略
}

# SR使能
source-packet-routing {
    srgb start-label <min> index-range <range>;
    node-segment {
        ipv4-index <index>;
    }
    adjacency-segment { ... }
    flex-algorithm <algo-id> { ... }
}

# 过载
overload [ timeout <seconds> ];

# 收敛
spf-delay <ms>;
lsp-lifetime <seconds>;

# IS-IS延迟测量 (用于Flex-Algo latency metric)
interface <interface> {
    delay-measurement;
}

# Flood Reflector (大规模网络)
flood-reflector { cluster <id>; }

# GR
graceful-restart;
```

### Display/Show命令

```
# IS-IS邻居 ★
华为/H3C: display isis peer [ verbose ]
Cisco: show isis neighbors [ detail ]
Juniper: show isis adjacency [ detail | extensive ]

# IS-IS LSDB
华为/H3C: display isis lsdb [ level-1 | level-2 ] [ verbose ]
Cisco: show isis database [ level { 1 | 2 } ] [ detail ] [ <system-id> ]
Juniper: show isis database [ level 1 | level 2 ] [ detail | extensive ] [ <system-id> ]

# IS-IS路由
华为: display isis route [ <ip-address> <mask-length> ]
Cisco: show isis route [ <prefix> ]
Juniper: show isis route [ <prefix> ] [ detail ]

# IS-IS SPF日志（排查收敛）
华为: display isis spf-log
Cisco: show isis spf-log
Juniper: show isis spf log [ brief ]

# IS-IS错误
华为: display isis error
Cisco: show isis statistics

# IS-IS SR能力
Cisco: show isis segment-routing label table
Juniper: show isis spring label table
Juniper: show isis spring node-segment
```

**IS-IS邻居状态**: Initializing(三次握手未完成) → Up(正常)

---

## 1.4 BGP

### 华为 NE40E 配置命令

```
bgp <as-number>
  router-id <router-id>
  peer <ip> as-number <as-number>
  peer <ip> connect-interface <interface>
  peer <ip> ebgp-max-hop <hop>
  peer <ip> password [ cipher ] <password>
  peer <ip> bfd enable
  peer <ip> bfd min-tx-interval <ms> min-rx-interval <ms> detect-multiplier <mult>

# IPv4单播地址族
  ipv4-family unicast
    peer <ip> enable
    peer <ip> route-policy <name> { import | export }
    peer <ip> filter-policy { acl <number> | ip-prefix <name> } { import | export }
    peer <ip> as-path-filter <number> { import | export }
    network <ip-address> [ mask { <mask> | <mask-length> } ]
    import-route { static | direct | ospf | isis } [ route-policy <name> ]
    peer <ip> advertise-community
    peer <ip> advertise-ext-community

# VPNv4地址族
  ipv4-family vpnv4
    peer <ip> enable
    peer <ip> route-policy <name> { import | export }

# EVPN地址族
  l2vpn-family evpn
    peer <ip> enable

# 路由反射器
  peer <ip> reflect-client
  reflector cluster-id <id>

# 选路控制
  peer <ip> preferred-value <value>           # 华为私有，最高优先
  default local-preference <value>             # 默认100
  bestroute compare-med                        # 跨AS比较MED
  bestroute as-path-ignore                     # 忽略AS-Path(慎用)

# 收敛
  timer keepalive <keepalive> hold <holdtime>  # 默认60/180秒
  graceful-restart
  peer <ip> advertise add-path { best <number> | all }

# 路由抑制
  dampening [ <half-life> <reuse> <suppress> <max-suppress-time> ]

# BGP Flow Specification
  ipv4-family flow
    peer <ip> enable
```

### H3C 关键差异

```
bgp <as-number>
  router-id <router-id>
  peer <ip> as-number <as-number>
  peer <ip> connect-interface <interface>
  peer <ip> bfd enable

  address-family ipv4 [ unicast ]       # H3C用address-family
    peer <ip> enable
    peer <ip> route-policy <name> { import | export }
    network <ip-address> [ mask-length | mask <mask> ]

  address-family vpnv4
    peer <ip> enable

  address-family l2vpn evpn
    peer <ip> enable
```

### Cisco IOS-XR BGP

```
router bgp <as-number>
  bgp router-id <router-id>
  address-family ipv4 unicast
    network <prefix>/<mask-len>
    redistribute { static | connected | ospf <process> | isis <tag> } [ route-policy <name> ]
    allocate-label all                            # VPNv4标签分配

  # 邻居
  neighbor <ip>
    remote-as <as-number>
    update-source <interface>
    ebgp-multihop <hops>
    password [ encrypted ] <password>
    bfd minimum-interval <ms> / bfd multiplier <mult> / bfd fast-detect
    timers <keepalive> <holdtime>                  # 默认60/180秒
    graceful-restart

    address-family ipv4 unicast
      route-policy <name> { in | out }
      prefix-policy <name>                        # 前缀过滤
      soft-reconfiguration inbound always
      send-community-ebgp / send-extended-community-ebgp
      next-hop-self
      as-override                                 # AS覆盖(Hub-Spoke VPN)

  # VPNv4地址族
  neighbor <ip>
    address-family vpnv4 unicast
      route-policy <name> { in | out }

  # EVPN地址族
  neighbor <ip>
    address-family l2vpn evpn

  # 路由反射器
  neighbor <ip>
    address-family { ipv4 unicast | vpnv4 unicast | l2vpn evpn }
      route-reflector-client
  bgp cluster-id <id>

  # 选路控制
  bgp bestpath as-path ignore                     # 忽略AS-Path(慎用)
  bgp bestpath med always                         # 跨AS比较MED

  # 路由抑制
  address-family ipv4 unicast
    bgp dampening [ <half-life> <reuse> <suppress> <max-suppress-time> ]

  # RPL路由策略语言 (IOS-XR特有)
  route-policy <name>
    if destination in <prefix-set> then
      set local-preference <value>
    endif
  end-policy

  prefix-set <name>
    <prefix>/<len>,
    <prefix>/<len> le <le-len> ge <ge-len>
  end-set

  as-path-set <name>
    ios-regex '^65001$',
    ios-regex '_65001_'
  end-set

  community-set <name>
    65001:100,
    no-export
  end-set

  extcommunity-set rt <name>
    <as>:<value>
  end-set
```

### Juniper Junos BGP

```
[edit protocols bgp]
group <group-name> {
    type { internal | external };
    local-address <ip>;
    peer-as <as-number>;
    hold-time <seconds>;
    authentication-key <password>;
    bfd-liveness-detection {
        minimum-interval <ms>;
        multiplier <mult>;
    }
    multihop { ttl <value>; }

    family inet {
        unicast;
        labeled-unicast;                          # 标签单播(用于Inter-AS Option C)
    }
    family inet-vpn {
        unicast;                                   # VPNv4
    }
    family evpn {
        signaling;                                 # EVPN
    }
    family inet6 {
        unicast;
        labeled-unicast;
    }

    import <policy-name>;                          # 入方向策略
    export <policy-name>;                          # 出方向策略

    neighbor <ip> {
        peer-as <as-number>;                       # 覆盖组级别配置
    }

    # SRv6相关
    family inet {
        unicast {
            extended-nexthop;
        }
    }
    family inet-vpn {
        unicast {
            srv6 {
                locator <locator-name> {
                    end-dt4-sid;
                    micro-dt4-sid;
                }
            }
        }
    }

    # 路由反射器
    cluster <cluster-id>;
}

# GR
graceful-restart;

# Entropy Label (负载均衡)
group <group-name> {
    entropy-label { ... }
}

# Add-Path
group <group-name> {
    family inet unicast {
        add-path { send { path-count <n>; } receive; }
    }
}
```

### Display/Show命令

```
# BGP邻居 ★
华为/H3C: display bgp peer [ <ip> ] [ verbose ]
Cisco: show bgp [ ipv4 unicast ] neighbors [ <ip> ]
Cisco: show bgp [ ipv4 unicast ] summary                 # 邻居摘要
Juniper: show bgp neighbor [ <ip> ]
Juniper: show bgp summary

# BGP路由表
华为/H3C: display bgp routing-table [ <ip-address> ]
Cisco: show bgp [ ipv4 unicast ] [ <prefix> ] [ detail ]
Juniper: show route protocol bgp [ <prefix> ] [ detail | extensive ]
Juniper: show bgp group [ <group-name> ]

# VPNv4路由
华为: display bgp vpnv4 all routing-table
华为: display bgp vpnv4 vpn-instance <vpn> routing-table [ <ip> ]
Cisco: show bgp vpnv4 unicast [ rd <rd> ] [ <prefix> ]
Cisco: show bgp vrf <vrf-name> [ ipv4 unicast ] [ <prefix> ]
Juniper: show route table <vrf>.inet.0 protocol bgp

# EVPN路由
华为: display bgp evpn all routing-table [ type { 1 | 2 | 3 | 4 | 5 } ]
H3C:  display bgp l2vpn evpn routing-table
Cisco: show bgp l2vpn evpn [ rd <rd> ] [ route-type { 1 | 2 | 3 | 4 | 5 } ]
Juniper: show evpn database [ instance <name> ]
Juniper: show route table <evpn-instance>.evpn.0

# 路由抑制
华为/H3C: display bgp routing-table dampened
Cisco: show bgp ipv4 unicast dampened-paths
```

**BGP邻居状态解读**:
| 状态 | 含义 | 排障方向 |
|:---|:---|:---|
| Idle | 初始/重置 | ACL阻断/路由不可达/AS号错 |
| Active | TCP连接失败重试 | 对端未配/ACL/路由不可达 |
| OpenSent | OPEN已发送 | 版本/能力不匹配 |
| Established | 会话建立 | 正常，确认AFI/SAFI协商 |

**BGP选路优先级(华为)**:
1. Preferred Value(越大越优) → 2. Local Preference(越大越优,默认100) → 3. 本地生成>学习 → 4. AS-Path最短 → 5. Origin(IGP>EGP>Incomplete) → 6. MED最小 → 7. EBGP>IBGP → 8. IGP Metric到下一跳最小 → 9. Cluster-List最短 → 10. Router-ID最小 → 11. Peer IP最小

**BGP选路优先级(Juniper)**: Local Preference → AS-Path → Origin → MED → EBGP>IBGP → IGP Metric → Router-ID → Peer IP → Cluster-List

**BGP选路优先级(Cisco IOS-XR)**: Weight(Cisco私有) → Local Preference → 本地始发 → AS-Path → Origin → MED → EBGP>IBGP → IGP Metric → Router-ID → Neighbor IP

---

## 1.5 路由策略与策略路由

### 华为 NE40E — Route-Policy

```
route-policy <name> { permit | deny } node <node-number>
  # ---- if-match 匹配条件 ----
  if-match acl <acl-number>
  if-match ip-prefix <prefix-name>                    # ★最常用
  if-match ip next-hop ip-prefix <prefix-name>
  if-match as-path-filter <number>
  if-match community-filter { <number> | <name> } [ whole-match ]
  if-match extcommunity-filter <name>
  if-match route-type { external-type1 | external-type2 | internal }
  if-match interface <interface>
  if-match cost <cost>
  if-match tag <tag>
  if-match origin { egp | igp | incomplete }

  # ---- apply 动作 ----
  apply cost <cost>                                   # 设MED
  apply local-preference <value>                       # 设LP
  apply origin { egp <as> | igp | incomplete }
  apply community { <community> | well-known } [ additive ]
  apply extcommunity rt <rt> [ additive ]
  apply preferred-value <value>
  apply tag <tag>
  apply ip-address next-hop <ip>
  apply as-path <as-number> [ additive ]               # AS-Path Prepend
```

### 华为 NE40E — 前缀列表/过滤器

```
# IP前缀列表
ip ip-prefix <name> [ index <idx> ] { permit | deny } <ip> <mask-len> [ greater-equal <ge> ] [ less-equal <le> ]
# 示例: ip ip-prefix test index 10 permit 10.0.0.0 8 greater-equal 8 less-equal 24

# AS-Path过滤器
ip as-path-filter <number> { permit | deny } <regex>
# 示例: ip as-path-filter 1 permit ^65001$ (来自AS65001)
#        ip as-path-filter 2 permit _65001_ (经过AS65001)
#        ip as-path-filter 3 permit ^$ (本地始发)

# Community过滤器
ip community-filter basic <name> { permit | deny } { <community> | internet | no-advertise | no-export }
ip community-filter advanced <name> { permit | deny } <regex>

# Extcommunity过滤器
ip extcommunity-filter basic <name> { permit | deny } rt <rt-value>
```

### H3C 关键差异

```
route-policy <name> { permit | deny } node <seq>
  if-match ip address { acl <number> | prefix-list <name> }  # H3C用prefix-list
  if-match as-path <number>
  if-match community-list { basic | advanced } <name>

ip prefix-list <name> [ index <idx> ] { permit | deny } <ip> <mask-len> [ ge <ge> ] [ le <le> ]
ip as-path <number> { permit | deny } <regex>
ip community-list { basic | advanced } <name> { permit | deny } { <community> | regex }
```

### Cisco IOS-XR — RPL路由策略语言

```
# RPL (Routing Policy Language) — IOS-XR特有结构化策略
route-policy <name>
  # 条件匹配
  if destination in <prefix-set> then
    set local-preference <value>
    done
  elseif as-path in <as-path-set> then
    set med <value>
    done
  elseif community matches-any <community-set> then
    drop
  else
    pass
  endif
end-policy

# 前缀集(Prefix Set)
prefix-set <name>
  10.0.0.0/8 le 24,
  172.16.0.0/12 le 32,
  192.168.0.0/16
end-set

# AS路径集
as-path-set <name>
  ios-regex '^65001$',             # 来自AS65001
  ios-regex '_65001_',             # 经过AS65001
  ios-regex '^$'                   # 本地始发
end-set

# Community集
community-set <name>
  65001:100,
  65001:200,
  no-export,
  no-advertise
end-set

# Extended Community集
extcommunity-set rt <name>
  100:1,
  200:1
end-set

# 应用
router bgp <as>
  neighbor <ip>
    address-family ipv4 unicast
      route-policy <name> { in | out }
router ospf <process>
  redistribute bgp <as> route-policy <name>
```

### Juniper Junos — Policy Statement

```
[edit policy-options]
# 路由策略
policy-statement <name> {
    term <term-name> {
        from {
            protocol [ ospf isis bgp static direct ];
            route-filter <prefix>/<len> [ exact | orlonger | longer | upto /<len> | prefix-length-range /<min>-/<max> ];
            prefix-list <list-name>;
            prefix-list-filter <list-name> [ orlonger | longer | exact ];
            as-path <as-path-name>;
            community <community-name>;
            neighbor <ip>;
            area <area-id>;
            metric <value>;
            local-preference <value>;
            next-hop <ip>;
            interface <interface>;
            tag <value>;
        }
        then {
            accept;
            reject;
            local-preference <value>;
            metric <value>;                        # 设MED
            origin { igp | egp | incomplete };
            community { add | delete | set } <community-name>;
            as-path-prepend "<as> <as>";
            next-hop <ip>;
            next-hop self;
            preference <value>;
            tag <value>;
            forwarding-class <class>;
            cos-next-hop-map <map>;
        }
    }
    # 多个term按顺序匹配，默认拒绝(BGP)/接受(IGP export)
}

# 前缀列表
prefix-list <name> {
    <prefix>/<len>;
    apply-path "protocols bgp group <*> neighbor <*>";  # 动态前缀列表
}

# AS路径正则
as-path <name> "<regex>";
  # 示例: as-path my-path "65001";         来自AS65001
  # 示例: as-path transit "65001 .* 65002"; 经AS65001到65002
  # 最长65536字符

# Community定义
community <name> members [ "<as>:<value>" ];
  # 标准社区: "65001:100"
  # 扩展社区: "target:100:1" (RT)
  # 大社区: "65001:1:100"
  # invert-match: 匹配不含此community的路由

# 应用
[edit protocols bgp group <name>]
import <policy-name>;
export <policy-name>;

[edit protocols ospf]
export <policy-name>;                              # OSPF通过export引入外部路由
```

### Display/Show命令

```
华为: display route-policy [ <name> ]
华为: display ip ip-prefix [ <name> ]
华为: display ip as-path-filter [ <number> ]
华为: display ip community-filter [ <name> ]
H3C:  display route-policy [ <name> ]
H3C:  display ip prefix-list [ <name> ]
Cisco: show rpl route-policy [ <name> ] [ detail ]
Cisco: show rpl prefix-set [ <name> ]
Cisco: show rpl as-path-set [ <name> ]
Cisco: show rpl community-set [ <name> ]
Cisco: show rpl extcommunity-set rt [ <name> ]
Juniper: show policy <policy-name>
Juniper: show configuration policy-options
Juniper: test policy <policy-name> <prefix>        # ★策略测试
```
