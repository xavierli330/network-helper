# 第七章 网络可靠性

## 7.1 BFD

### 华为 NE40E

```
# 全局使能
bfd

# 静态BFD会话
bfd <session-name> bind peer-ip <peer-ip> [ vpn-instance <vpn> ] interface <interface> [ source-ip <src-ip> ]
  discriminator local <local-discr>
  discriminator remote <remote-discr>
  detect-multiplier <mult>                   # 默认3
  min-tx-interval <ms>                       # 最小发送间隔，默认1000ms
  min-rx-interval <ms>                       # 最小接收间隔，默认1000ms
  commit

# 动态BFD(协议联动自动创建)
interface <interface>
  ospf bfd enable / ospf bfd min-tx-interval <ms> min-rx-interval <ms> detect-multiplier <mult>
  isis bfd enable / isis bfd min-tx-interval <ms> min-rx-interval <ms> detect-multiplier <mult>
bgp <as-number>
  peer <ip> bfd enable / peer <ip> bfd min-tx-interval <ms> min-rx-interval <ms> detect-multiplier <mult>
mpls ldp
  bfd enable / bfd all-interface

# 静态路由联动BFD
ip route-static <dest> <mask> <nexthop> bfd enable

# VRRP联动BFD
interface <interface>
  vrrp vrid <vrid> track bfd-session <session-name> [ reduced <value> ]

# SBFD (Seamless BFD)
bfd
  sbfd local-discriminator <discriminator>

# BFD Echo模式
bfd <session-name> ...
  min-echo-rx-interval <interval>

# BFD for MPLS LSP
bfd <session-name> bind mpls-lsp peer-ip <dest-ip> [ nexthop <nexthop> ] interface <interface>
  discriminator local <local-discr>
  commit

# BFD for SR-TE Policy
segment-routing te
  policy <name>
    bfd enable
    bfd min-tx-interval <ms> min-rx-interval <ms> detect-multiplier <mult>
```

### H3C S12500X

```
bfd echo-source-ip <ip>
bfd <session-name> bind peer-ip <peer-ip> interface <interface>
  bfd detect-multiplier <mult>
  bfd min-transmit-interval <ms>
  bfd min-receive-interval <ms>
  bfd authentication-mode { simple | md5 | sha1 } <key>

interface <interface>
  ospf bfd enable / isis bfd enable
bgp <as-number>
  peer <ip> bfd enable
```

### Cisco IOS-XR BFD

```
# BFD全局/接口
bfd
  multipath include location <loc>
  multihop ttl-drop-threshold <value>

# BFD联动各协议(IOS-XR在各协议下配置)
router isis <instance>
  interface <interface>
    bfd minimum-interval <ms>
    bfd multiplier <mult>
    bfd fast-detect

router ospf <process>
  area <area-id>
    interface <interface>
      bfd minimum-interval <ms>
      bfd multiplier <mult>
      bfd fast-detect

router bgp <as>
  neighbor <ip>
    bfd minimum-interval <ms>
    bfd multiplier <mult>
    bfd fast-detect

router static
  address-family ipv4 unicast
    <prefix>/<len> <nexthop> bfd fast-detect [ minimum-interval <ms> ] [ multiplier <mult> ]

# SBFD (Seamless BFD)
bfd
  multihop ttl-drop-threshold <value>
sbfd
  local-discriminator ipv4-address <ip>
  remote-target ipv4-address <ip>
```

### Juniper Junos BFD

```
[edit protocols]
# OSPF BFD
ospf {
    area <area-id> {
        interface <interface> {
            bfd-liveness-detection {
                minimum-interval <ms>;                # 最小间隔(默认1000ms)
                minimum-receive-interval <ms>;        # 最小接收间隔
                multiplier <mult>;                    # 检测倍数(默认3)
                version { 0 | 1 | automatic };        # BFD版本
                no-issu-timer-negotiation;             # ISSU期间不协商
                dedicated-ukern-cpu;                   # 专用CPU处理BFD
            }
        }
    }
}

# IS-IS BFD
isis {
    interface <interface> {
        bfd-liveness-detection {
            minimum-interval <ms>;
            minimum-receive-interval <ms>;
            multiplier <mult>;
        }
    }
}

# BGP BFD
bgp {
    group <group-name> {
        bfd-liveness-detection {
            minimum-interval <ms>;
            multiplier <mult>;
        }
        neighbor <ip> {
            bfd-liveness-detection {
                minimum-interval <ms>;
                multiplier <mult>;
            }
        }
    }
}

# 静态路由BFD
[edit routing-options static]
route <prefix> {
    next-hop <ip>;
    bfd-liveness-detection {
        minimum-interval <ms>;
        multiplier <mult>;
        local-address <local-ip>;
    }
}
```

### Display/Show命令

```
华为: display bfd session all [ verbose ]      # BFD会话 ★
华为: display bfd statistics
H3C:  display bfd session [ verbose ]
H3C:  display bfd statistics

Cisco: show bfd session [ detail ]             # BFD会话 ★
Cisco: show bfd neighbors [ detail ]           # BFD邻居
Cisco: show bfd counters                        # BFD统计
Cisco: show bfd client [ detail ]              # BFD客户端(关联协议)

Juniper: show bfd session [ detail | extensive ]  # BFD会话 ★
Juniper: show bfd session address <ip>
Juniper: show bfd session summary
```

**BFD状态**: Up(正常) | Down(检测失败→物理/未配置/参数不匹配) | AdminDown(管理关闭)

**Juniper BFD回显示例**:
```
Address         State     Interface    Detect Time  Transmit    Multiplier
10.0.0.2        Up        ge-0/0/0     900ms        300ms       3
```

---

## 7.2 VRRP

### 华为 NE40E

```
# 标准VRRP
interface <interface>
  vrrp vrid <vrid> virtual-ip <virtual-ip>
  vrrp vrid <vrid> priority <priority>       # 默认100, Master通常配120+
  vrrp vrid <vrid> preempt-mode timer delay <seconds>  # 抢占延迟
  vrrp vrid <vrid> timer advertise <seconds>  # 通告间隔,默认1s
  vrrp vrid <vrid> authentication-mode { simple | md5 } <password>

# 联动Track(上行链路故障降优先级)
interface <interface>
  vrrp vrid <vrid> track interface <track-interface> [ reduced <value> ]
  vrrp vrid <vrid> track bfd-session <session-name> [ reduced <value> ]
  vrrp vrid <vrid> track nqa <admin-name> <test-name> [ reduced <value> ]

# 负载均衡VRRP(多备份组)
# 接口上配置多个VRID,不同VRID的Master分布在不同设备上
interface <interface>
  vrrp vrid 1 virtual-ip <vip1>
  vrrp vrid 1 priority 120                   # 设备A为VRID1的Master
  vrrp vrid 2 virtual-ip <vip2>
  vrrp vrid 2 priority 100                   # 设备A为VRID2的Backup
```

### H3C S12500X

```
interface <interface>
  vrrp vrid <vrid> virtual-ip <virtual-ip>
  vrrp vrid <vrid> priority <priority>       # 默认100
  vrrp vrid <vrid> preempt-mode timer delay <seconds>
  vrrp vrid <vrid> timer advertise <seconds>
  vrrp vrid <vrid> track interface <interface> [ reduced <value> ]
  vrrp vrid <vrid> track <track-id>
```

### Cisco IOS-XR VRRP/HSRP

```
# VRRP
router vrrp
  interface <interface>
    address-family ipv4
      vrrp <vrid> version 3
        address <virtual-ip>
        priority <priority>                         # 默认100
        preempt [ delay <seconds> ]
        timer <seconds>                             # 通告间隔
        track object <track-id> [ decrement <value> ]

# HSRP (Cisco特有，功能类似VRRP)
router hsrp
  interface <interface>
    address-family ipv4
      hsrp <group-id> version 2
        address <virtual-ip>
        priority <priority>
        preempt [ delay minimum <seconds> ]
        timers <hello> <hold>
        track object <track-id> [ decrement <value> ]
        bfd fast-detect [ peer ipv4 <ip> ]
```

### Juniper Junos VRRP

```
[edit interfaces <interface> unit <unit>]
family inet {
    address <ip>/<prefix-len> {
        vrrp-group <vrid> {
            virtual-address <virtual-ip>;
            priority <priority>;                    # 默认100
            preempt [ hold-time <seconds> ];
            advertise-interval <seconds>;           # 默认1秒
            accept-data;                            # 接受发往VIP的数据包
            authentication-type { simple <password> | md5 }
            track {
                interface <interface> {
                    priority-cost <cost>;            # 降低优先级的值
                    bandwidth-threshold <bw> {
                        priority-cost <cost>;
                    }
                }
                route <prefix>/<len> routing-instance <instance> {
                    priority-cost <cost>;
                }
            }
            # 快速切换
            fast-interval <ms>;                     # 毫秒级通告(默认配合BFD)
        }
    }
}

# VRRP全局配置
[edit protocols vrrp]
asymmetric-hold-time;                              # 非对称保持时间
delegate-processing;                               # 委托处理
version-3;                                         # 使用VRRPv3

# VRRP负载均衡(多组)
# 接口上配置多个vrrp-group，不同设备做不同组的Master
```

### Display/Show命令

```
华为: display vrrp [ brief | interface <if> [ vrid <vrid> ] ]  # VRRP状态 ★
H3C:  display vrrp [ verbose ]

Cisco: show vrrp [ interface <if> ] [ brief | detail ]         # VRRP状态 ★
Cisco: show hsrp [ interface <if> ] [ brief | detail ]         # HSRP状态
Cisco: show vrrp statistics
Cisco: show hsrp statistics

Juniper: show vrrp [ detail | extensive | summary ]            # VRRP状态 ★
Juniper: show vrrp track [ detail ]                            # VRRP Track
Juniper: show vrrp interface <interface>

# 关键字段:
# State: Master/Backup/Initialize (华为/Juniper) 或 Active/Standby/Init (Cisco HSRP)
# Priority: 当前优先级(联动降级后的实际值)
# Virtual IP: 虚拟IP地址
# Master Router: 当前Master的真实IP
```

**Juniper VRRP回显示例**:
```
Interface     State       Group  VR state   VR Mode   Timer    Type  Address
ge-0/0/0.0    up              1  master     Active    1.000    lcl   10.0.0.1
                                                                vip   10.0.0.254
```

**VRRP排障**:
- 双Master → 检查VLAN/认证不一致,VRRP通告被ACL过滤
- 切换慢 → 检查preempt-mode timer delay、BFD联动是否启用
- 不切换 → 检查track是否生效、优先级降低是否足够

---

## 7.3 MPLS OAM

### 华为 NE40E

```
# MPLS OAM全局
mpls oam

# LSP Ping/Traceroute (见2.1节)
ping lsp ip <dest-ip> <mask> [ -a <source-ip> ] [ -c <count> ] [ -s <size> ]
tracert lsp ip <dest-ip> <mask>

# MPLS-TP OAM (用于PWE3/MPLS-TP场景)
mpls-tp oam
  cc interval { 10ms | 100ms | 1s | 10s | 1min | 10min }

# CFM/Y.1731 (以太网OAM)
cfm enable
cfm md <md-name> level <level>
  ma <ma-name> vlan <vlan-id>
    mep <mep-id> interface <interface> direction { inward | outward }
    remote-mep <rmep-id>

# EFM (以太网链路OAM — 802.3ah)
interface <interface>
  efm enable
  efm trigger-if-down                        # EFM失败触发接口Down
```

### Display命令

```
华为: display mpls oam [ ingress | egress | transit ]
华为: display cfm ma
华为: display efm session [ interface <interface> ]
华为: display y1731 [ one-way-delay | two-way-delay | single-loss ]
```
