# 第三章 Segment Routing

## 3.1 SR-MPLS BE

### 华为 NE40E

```
# SRGB全局标签块
segment-routing
  global-block <min-label> <max-label>      # 如 16000 23999

# SRLB本地标签块(Adj-SID)
segment-routing
  local-block <min-label> <max-label>

# IS-IS使能SR + Prefix SID
isis <process-id>
  segment-routing mpls
interface LoopBack <num>
  isis enable <process-id>
  prefix-sid { index <index> | absolute <label> } [ n-flag-clear ]

# OSPF使能SR + Prefix SID
ospf <process-id>
  segment-routing mpls
interface LoopBack <num>
  ospf enable <process-id>
  prefix-sid { index <index> | absolute <label> }

# Adjacency SID
isis <process-id>
  segment-routing auto-adj-sid disable       # 禁用自动Adj-SID
interface <interface>
  isis adjacency-sid { absolute <label> | index <index> }

# Anycast SID: 多节点Loopback配同一Prefix SID index
```

### H3C S12500X

```
segment-routing mpls
segment-routing
  global-block <min-label> <max-label>
isis <process-id>
  segment-routing mpls
interface LoopBack <num>
  isis enable <process-id>
  prefix-sid { index <index> | absolute <label> }

# 静态SR-MPLS
static-sr-mpls
  adjacency-sid <label> destination <ip> nexthop <nexthop> interface <if>
mpls te static-sr-mpls <tunnel-name>
  segment-list { <label> }
```

### Cisco IOS-XR (ASR9000)

```
# SRGB全局标签块
segment-routing
  global-block <min-label> <max-label>

# SRLB本地标签块
segment-routing
  local-block <min-label> <max-label>

# Mapping Server (非SR节点互操作)
segment-routing
  mapping-server
    prefix-sid-map
      address-family ipv4
        <prefix>/<len> <index> range <range>

# IS-IS使能SR + Prefix SID
router isis <instance>
  address-family ipv4 unicast
    metric-style wide
    segment-routing mpls
    segment-routing sr-prefer                     # SR优先于LDP
  interface Loopback <num>
    address-family ipv4 unicast
      prefix-sid [ index <index> | absolute <label> ] [ n-flag-clear ]

# OSPF使能SR + Prefix SID
router ospf <process>
  segment-routing mpls
  segment-routing forwarding mpls
  area <area-id>
    interface Loopback <num>
      prefix-sid [ index <index> | absolute <label> ]

# Adjacency SID
router isis <instance>
  interface <interface>
    address-family ipv4 unicast
      adjacency-sid { index <idx> | absolute <label> } [ protected ]
```

### Juniper Junos

```
# SR-MPLS 通过IS-IS使能
[edit protocols isis]
source-packet-routing {
    srgb start-label <min-label> index-range <range>;     # 如 start-label 16000 index-range 8000
    node-segment {
        ipv4-index <index>;                                # 节点SID索引
    }
    adjacency-segment {
        <interface-name> {
            label <label>;                                 # 手动Adj-SID
        }
    }
}

# SR-MPLS 通过OSPF使能
[edit protocols ospf]
area <area-id> {
    interface <interface> {
        source-packet-routing {
            node-segment {
                ipv4-index <index>;
            }
        }
    }
}

# SRGB (通过routing-options)
[edit routing-options]
source-packet-routing {
    srgb start-label <min> index-range <range>;
}

# Anycast SID: 多节点Loopback配相同index
```

### Display/Show命令

```
华为: display segment-routing prefix mpls [ ip-prefix <prefix> ]   # Prefix SID ★
华为: display segment-routing global-block                          # SRGB
华为: display segment-routing adjacency mpls                        # Adj-SID
华为: display isis segment-routing [ verbose ]                      # IS-IS SR能力
华为: display segment-routing traffic-statistics
H3C:  display segment-routing prefix mpls
H3C:  display segment-routing global-block

Cisco: show segment-routing mpls state
Cisco: show isis segment-routing label table                       # SR标签表 ★
Cisco: show segment-routing mpls connected-prefix-sid-map [ local | received ]
Cisco: show mpls forwarding labels <label>

Juniper: show isis spring label table                              # SR标签表 ★
Juniper: show isis spring node-segment                             # 节点段
Juniper: show route table mpls.0 label <label>
Juniper: show isis overview | match "segment"                      # SR全局状态
```

---

## 3.2 SR-MPLS TE Policy

### 华为 NE40E

```
# SR TE Policy
segment-routing te
  policy <name>
    color <color-value>                      # 颜色(流量导入标识)
    end-point <ip-address>
    binding-sid <label>                      # 可选Binding SID
    candidate-path preference <pref>         # 数值最大的优选
      segment-list <list-name>
      weight <weight>
    candidate-path preference <pref>
      dynamic
        metric-type { igp | te | latency }

# 段列表
segment-routing te
  segment-list <name>
    segment <seq> mpls label <label>
    segment <seq> mpls adjacency <adj-ip>
    segment <seq> mpls index <index>

# PCEP委托
segment-routing te
  pce-client
    connect-server <pce-ip>
    delegate sr-te-policy [ <name> ]

# ODN按需下发
segment-routing te
  on-demand color <color>
    dynamic
      metric-type { igp | te | latency }
    binding-sid-mode dynamic

# 流量导入方式
# 1.隧道策略: tunnel-policy <name> → tunnel select-seq sr-te-policy
# 2.BGP着色: apply extcommunity color <value>
# 3.Binding SID: ip route-static <dest> <mask> <binding-sid>

# 约束
segment-routing te
  policy <name>
    candidate-path preference <pref>
      constraints
        affinity include-any { <color> } / exclude-any { <color> }
        bandwidth <bw>
```

### Cisco IOS-XR SR-TE Policy

```
# SR TE Policy
segment-routing
  traffic-eng
    policy <name>
      color <color-value> end-point ipv4 <ip>
      binding-sid mpls <label>
      candidate-paths
        preference <pref>
          explicit segment-list <list-name>
            weight <weight>
        preference <pref>
          dynamic
            pcep
            metric type { igp | te | latency | hop-count }
            constraints
              affinity { include-any | include-all | exclude-any } name <color>
              bandwidth <bw-kbps>

    # 段列表
    segment-list <name>
      index <idx> mpls { label <label> | adjacency <ip> }

    # PCEP委托
    pcc
      source-address ipv4 <ip>
      pce address ipv4 <pce-ip> [ precedence <value> ]
      report-all

    # ODN按需下发 (On-Demand Nexthop)
    on-demand color <color>
      dynamic
        pcep
        metric type { igp | te | latency }
      binding-sid-mode dynamic

    # Performance Measurement
    policy <name>
      performance-measurement
        delay-measurement
          advertise-delay <threshold>

    # SBFD (Seamless BFD)
    segment-routing
      traffic-eng
        policy <name>
          candidate-paths preference <pref>
            constraints
              segments
                protection { protected-only | unprotected-only }
```

### Juniper Junos SR-TE

```
# SR-TE 源路由路径
[edit protocols source-packet-routing]
source-routing-path <path-name> {
    to <dest-ip>;
    binding-sid <label>;
    primary {
        <segment-list-name>;
    }
    secondary {
        <backup-segment-list-name>;
    }
    color <color-value>;
    sr-preference <value>;                         # 越大越优
}

segment-list <list-name> {
    <segment-name> {
        label <label>;
    }
}

# Spring Traffic Engineering (另一种配置方式)
[edit protocols spring-traffic-engineering]
tunnel <tunnel-name> {
    color <color>;
    end-point <ip>;
    primary {
        segment-list {
            <segment-name> label <label>;
        }
    }
}

# SR-TE PCE (通过PCEP)
[edit protocols pcep]
pce <pce-name> {
    local-address <ip>;
    destination-ipv4-address <pce-ip>;
    pce-type { active | stateful };
    delegation-cleanup-timeout <seconds>;
    lsp-provisioning;
    spring-capability;
}
```

### Display/Show命令

```
华为: display segment-routing te policy [ name <name> ] [ color <c> ] [ endpoint <ip> ]  ★
华为: display segment-routing te policy [ all ] [ detail ]
华为: display segment-routing te forwarding-table
华为: display segment-routing te policy statistics
# ★ 注意: Oper State=Up 仅表示候选路径存在，不代表路径可达

Cisco: show segment-routing traffic-eng policy [ name <name> ] [ color <c> endpoint ipv4 <ip> ]  ★
Cisco: show segment-routing traffic-eng policy [ all ] [ detail ]
Cisco: show segment-routing traffic-eng forwarding policy [ name <name> ]
Cisco: show pce lsp [ detail ]

Juniper: show spring-traffic-engineering lsp [ detail | extensive ]   ★
Juniper: show spring-traffic-engineering lsp name <name>
Juniper: show route programmed-by spring-te
Juniper: ping mpls segment-routing traffic-engineering [ spring-te <lsp-name> ]
Juniper: traceroute mpls segment-routing traffic-engineering [ spring-te <lsp-name> ]
```

---

## 3.3 SRv6

### 华为 NE40E

```
segment-routing ipv6
  encapsulation source-address <ipv6-address>
  locator <locator-name> <ipv6-prefix>/<prefix-len> [ static <static-len> ] [ args <args-len> ]

isis <process-id>
  ipv6 enable
  segment-routing ipv6 locator <locator-name>

# SRv6 TE Policy
segment-routing ipv6 te
  policy <name>
    color <color>
    end-point <ipv6-address>
    candidate-path preference <pref>
      segment-list <list-name>
segment-routing ipv6 te
  segment-list <name>
    segment <seq> srv6 sid <sid>

# SRv6 VPN
ip vpn-instance <vpn>
  ipv6-family
    segment-routing ipv6 locator <locator-name>
```

### Cisco IOS-XR SRv6

```
segment-routing
  srv6
    encapsulation source-address <ipv6-address>
    locators
      locator <locator-name>
        prefix <ipv6-prefix>/<prefix-len>
        micro-segment behavior unode psp-usd       # uSID行为

# IS-IS SRv6
router isis <instance>
  address-family ipv6 unicast
    segment-routing srv6
      locator <locator-name>

# SRv6 TE Policy
segment-routing
  traffic-eng
    policy <name>
      color <color> end-point ipv6 <ipv6>
      candidate-paths
        preference <pref>
          explicit segment-list <list-name>
    segment-list <name>
      index <idx> srv6 sid <ipv6-sid>

# SRv6 VPN
vrf <vrf-name>
  address-family ipv4 unicast
    segment-routing srv6
      locator <locator-name>
      alloc mode per-vrf
```

### Juniper Junos SRv6

```
# SRv6 全局配置
[edit routing-options]
source-packet-routing {
    srv6 {
        block <block-ipv6-prefix>/<len>;
        locator <locator-name> <locator-prefix>/<len> {
            function <function-prefix>/<len>;
        }
        no-reduced-srh;                            # 不使用缩减SRH
        transit-srh-insert;                        # 中转SRH插入
    }
}

# IS-IS SRv6
[edit protocols isis]
source-packet-routing {
    srv6 {
        locator <locator-name>;
    }
}

# BGP SRv6 VPN
[edit protocols bgp group <name>]
family inet-vpn unicast {
    srv6 {
        locator <locator-name> {
            end-dt4-sid;                           # L3VPN IPv4 Endpoint
            micro-dt4-sid;                         # Micro-SID L3VPN
        }
    }
}

# EVPN SRv6
[edit protocols evpn]
source-packet-routing {
    srv6 {
        locator <locator-name>;
    }
}
```

```
华为: display segment-routing ipv6 locator [ <name> ] [ verbose ]
华为: display segment-routing ipv6 sid [ <sid> ] [ verbose ]
华为: display segment-routing ipv6 te policy [ name <name> ]

Cisco: show segment-routing srv6 locator [ <name> ] [ detail ]
Cisco: show segment-routing srv6 sid [ <sid> ]
Cisco: show segment-routing srv6 manager

Juniper: show srv6 locator [ <name> ]
Juniper: show srv6 local-sids
Juniper: show route table <vrf>.inet.0 protocol bgp extensive   # 查看SRv6 VPN路由
```

---

## 3.4 Flex-Algo

### 华为 NE40E

```
segment-routing
  flex-algo <algo-id>                        # 128-255
    metric-type { igp | te | latency | delay }
    affinity include-any { <color> } / include-all / exclude-any
    priority <priority>

isis <process-id>
  flex-algo <algo-id>
    advertise-definition

interface LoopBack <num>
  prefix-sid algorithm <algo-id> { index <index> | absolute <label> }

segment-routing
  affinity-map <color-name> bit-position <bit>
interface <interface>
  segment-routing mpls te affinity <color-name>
```

### Cisco IOS-XR Flex-Algo

```
router isis <instance>
  flex-algo <algo-id>                              # 128-255
    metric-type { igp | te | delay }
    affinity { include-any | include-all | exclude-any } name <color>
    advertise-definition
    priority <priority>

  interface Loopback <num>
    address-family ipv4 unicast
      prefix-sid algorithm <algo-id> [ index <index> | absolute <label> ]

segment-routing
  affinity-map name <color-name> bit-position <bit>

interface <interface>
  affinity flex-algo <color-name>
```

### Juniper Junos Flex-Algo

```
[edit protocols isis]
source-packet-routing {
    flex-algorithm <algo-id> {
        definition {
            metric-type { igp | te-metric | delay };
            admin-group { include-any | include-all | exclude } <group>;
        }
        advertise;
    }
}

# Flex-Algo Prefix SID
interface <interface> {
    source-packet-routing {
        node-segment {
            ipv4-index <index>;
            algorithm <algo-id>;
        }
    }
}

# Strict ASLA-based Flex-Algo (OSPF)
[edit protocols ospf]
area <area-id> {
    interface <interface> {
        source-packet-routing {
            strict-asla-based-flex-algorithm;
        }
    }
}

# Sensor-based Stats (性能测量)
[edit protocols ospf]
area <area-id> {
    interface <interface> {
        source-packet-routing {
            sensor-based-stats { per-sid; }
        }
    }
}
```

```
华为: display segment-routing flex-algo [ <algo-id> ]
华为: display segment-routing prefix mpls flex-algo <algo-id>

Cisco: show isis flex-algo [ <algo-id> ]
Cisco: show segment-routing mpls connected-prefix-sid-map algorithm <algo-id>

Juniper: show isis spring flex-algorithm
Juniper: show ospf overview | match flex-algo
Juniper: show ospf spring flex-algorithm
```

---

## 3.5 TI-LFA FRR

### 华为 NE40E

```
# IS-IS TI-LFA
isis <process-id>
  frr
    loop-free-alternate [ level-1 | level-2 ]
    ti-lfa [ level-1 | level-2 ]
    ti-lfa node-protection [ level-1 | level-2 ]

# OSPF TI-LFA
ospf <process-id>
  frr
    loop-free-alternate
    ti-lfa / ti-lfa node-protection

# 接口排除
interface <interface>
  isis frr exclude / isis ti-lfa exclude

# Micro-Loop避免
isis <process-id>
  micro-loop-avoidance segment-routing [ rib-update-delay <seconds> ]
```

### Cisco IOS-XR TI-LFA

```
router isis <instance>
  address-family ipv4 unicast
    fast-reroute per-prefix
    fast-reroute per-prefix ti-lfa
    fast-reroute per-prefix tiebreaker { node-protecting | srlg-disjoint } index <idx>
    microloop avoidance segment-routing [ rib-update-delay <ms> ]

  interface <interface>
    address-family ipv4 unicast
      fast-reroute per-prefix
      fast-reroute per-prefix ti-lfa
      fast-reroute per-prefix exclude interface <interface>

router ospf <process>
  fast-reroute per-prefix
  fast-reroute per-prefix ti-lfa enable
  microloop avoidance segment-routing
```

### Juniper Junos TI-LFA

```
# IS-IS TI-LFA
[edit protocols isis]
backup-spf-options {
    use-post-convergence-lfa;                      # 启用TI-LFA
    use-source-packet-routing;                     # 使用SR标签栈保护
    node-link-degradation;
}

# OSPF TI-LFA
[edit protocols ospf]
backup-spf-options {
    use-post-convergence-lfa;
    use-source-packet-routing;
}

# 接口排除
[edit protocols isis]
interface <interface> {
    no-eligible-backup;                            # 排除作为备份
}
```

```
华为: display isis frr route [ <ip> <mask-len> ]
华为: display isis ti-lfa [ <ip> <mask-len> ]
华为: display ospf frr route

Cisco: show isis fast-reroute [ <prefix> ] [ detail ]
Cisco: show isis fast-reroute summary
Cisco: show ospf fast-reroute [ <prefix> ]

Juniper: show isis backup spf results
Juniper: show isis backup coverage
Juniper: show route <prefix> detail                # 查看备份路径
```
