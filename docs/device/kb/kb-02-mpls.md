# 第二章 MPLS技术

## 2.1 MPLS基础与LDP

### 华为 NE40E

```
# 全局使能
mpls lsr-id <ip-address>          # LSR ID = Loopback地址
mpls                               # 使能MPLS
mpls ldp                           # 使能LDP

# 接口使能
interface <interface>
  mpls                             # 接口使能MPLS
  mpls ldp                         # 接口使能LDP

# LDP参数
mpls ldp
  transport-address <ip>           # 传输地址=Loopback
  md5-password [ cipher ] <password> <peer-ip>
  label advertise { explicit-null | implicit-null }
  outbound peer <peer-lsr-id> fec ip-prefix <prefix-name>  # 标签过滤
  graceful-restart
  session-protection duration <seconds>
  igp-sync enable                  # LDP-IGP同步
```

### H3C S12500X

```
mpls lsr-id <ip-address>
mpls enable
interface <interface>
  mpls enable
  mpls ldp enable
mpls ldp
  transport-address <ip>
  md5-password { cipher | simple } <password> <peer-lsr-id>
  graceful-restart
```

### Cisco IOS-XR (ASR9000)

```
# 全局使能
mpls ldp
  router-id <ip-address>                  # LDP Router ID = Loopback

# 接口使能
mpls ldp
  interface <interface>

# LDP参数
mpls ldp
  address-family ipv4
    label local allocate for <prefix-acl>    # 标签过滤
  neighbor <peer-ip> password [ encrypted ] <password>
  session protection [ duration <seconds> ]
  igp sync delay { on-session-up <seconds> | on-proc-restart <seconds> }
  graceful-restart
  discovery transport-address <ip>            # 传输地址

# Targeted LDP (远端LDP)
mpls ldp
  address-family ipv4
    discovery targeted-hello accept
  neighbor <peer-ip> targeted
```

### Juniper Junos

```
[edit protocols ldp]
interface <interface>;
transport-address <ip>;                        # 传输地址(默认=Router ID)
session <peer-ip> {
    authentication-key <password>;
}
targeted-hello <peer-ip> {                     # 远端LDP
    local-address <local-ip>;
}
graceful-restart;
igp-synchronization holddown-interval <seconds>;

# 标签过滤
import <policy-name>;
export <policy-name>;
```

### Display/Show命令

```
华为: display mpls ldp session [ <peer-id> ] [ verbose ]      # LDP会话 ★
华为: display mpls ldp peer [ verbose ]                        # LDP邻居
华为: display mpls lsp [ <ip> <mask> ] [ verbose ]             # LSP表
华为: display mpls forwarding-table [ <ip> <mask-len> ]        # LFIB(转发真相) ★
华为: display mpls lsp statistics                              # 标签资源统计

H3C:  display mpls ldp session [ verbose ]
H3C:  display mpls lsp [ <ip> <mask> ]
H3C:  display mpls forwarding-table

Cisco: show mpls ldp neighbor [ <ip> ] [ detail ]             # LDP邻居/会话 ★
Cisco: show mpls ldp bindings [ <prefix>/<len> ]              # 标签绑定
Cisco: show mpls forwarding [ prefix <prefix>/<len> ]         # LFIB(转发真相) ★
Cisco: show mpls label table [ detail ]                       # 标签表
Cisco: show mpls interfaces                                    # MPLS接口

Juniper: show ldp session [ detail | extensive ]               # LDP会话 ★
Juniper: show ldp neighbor [ detail ]                          # LDP邻居
Juniper: show ldp database [ <prefix> ]                        # 标签绑定数据库
Juniper: show route table mpls.0                               # LFIB ★
Juniper: show ldp interface                                    # LDP接口
Juniper: show ldp traffic-statistics                           # LDP流量统计

# LSP验证
华为: ping lsp ip <dest-ip> <mask> [ -a <source-ip> ]
华为: tracert lsp ip <dest-ip> <mask>
H3C:  ping mpls lsp ip <dest-ip> <mask>
H3C:  tracert mpls lsp ip <dest-ip> <mask>
Cisco: ping mpls ipv4 <dest-ip>/<mask-len> [ source <src-ip> ]
Cisco: traceroute mpls ipv4 <dest-ip>/<mask-len>
Juniper: ping mpls ldp <dest-ip>/<mask-len> [ source <src-ip> ]
Juniper: traceroute mpls ldp <dest-ip>/<mask-len>

# 二分法定位 ★
华为: ping lsp ip <dest> <mask> -h <ttl> -c 100
Cisco: ping mpls ipv4 <dest>/<mask> ttl <ttl> repeat <count>
Juniper: ping mpls ldp <dest>/<mask> ttl <ttl> count <count>
```

**LDP会话状态**: Operational(正常) | Non Existent(不存在,检查传输地址/认证) | Initialized(协商中,参数不匹配)

**MPLS LSP回显字段**:
| 字段 | 说明 |
|:---|:---|
| FEC | 转发等价类(目的IP/掩码) |
| In-Label | 入标签(与上游出标签对应) |
| Out-Label | 出标签(3=implicit-null/PHP, 0=explicit-null) |
| Next-Hop | 下一跳 |
| Out-Interface | 出接口 |

---

## 2.2 RSVP-TE隧道

### 华为 NE40E

```
# 全局
mpls
mpls te                            # 使能TE
mpls te cspf                       # 使能CSPF算路

# 接口
interface <interface>
  mpls
  mpls te
  mpls te bandwidth max-reservable-bandwidth <bw>

# TE隧道
interface Tunnel <number>
  ip address unnumbered interface Loopback <num>
  tunnel-protocol mpls te
  destination <dest-ip>
  mpls te tunnel-id <id>
  mpls te signalled-bandwidth <bw>
  mpls te affinity-property <affinity> mask <mask>
  mpls te path [ explicit-path <path-name> ]
  mpls te path secondary [ explicit-path <path-name> ]
  mpls te record-route [ label ]
  mpls te fast-reroute [ bandwidth <bw> ] [ node-protection ] [ bandwidth-protection ]
  mpls te auto-bandwidth
  mpls te priority <setup-priority> <hold-priority>   # 0最高,7最低
  statistic enable

# 显式路径
explicit-path <name>
  next hop <ip> [ strict | loose ]

# RSVP认证/Hello
mpls rsvp-te
  authentication { cipher | simple } <password>
  hello enable
  hello lost <count>                # 默认3
```

### H3C

```
mpls te / mpls te cspf
interface Tunnel <number>
  tunnel-protocol mpls te
  destination <dest-ip>
  mpls te signalled-bandwidth <bw>
  mpls te path explicit-path <name>
  mpls te fast-reroute
  mpls te priority <setup> <hold>
  mpls te record-route
```

### Cisco IOS-XR RSVP-TE

```
# 全局
mpls traffic-eng
rsvp
  interface <interface>
    bandwidth <bw-kbps>                    # 可预留带宽

# TE隧道
interface tunnel-te <number>
  ipv4 unnumbered <loopback>
  destination <dest-ip>
  signalled-bandwidth <bw-kbps>
  affinity <affinity> mask <mask>          # 亲和属性
  path-option <preference> { explicit name <path-name> | dynamic }
  path-option <preference> explicit name <path-name>     # 显式路径
  path-option <preference> dynamic                       # 动态CSPF
  record-route
  fast-reroute [ protect { node | bandwidth } ]
  auto-bw [ bw-limit { min <min> max <max> } ] [ adjustment-threshold <pct> ] [ collect-bw ]
  priority <setup-priority> <hold-priority>              # 0最高,7最低
  logging events { lsp-status | lsp-setups }

# 显式路径
explicit-path name <name>
  index <idx> next-address [ strict | loose ] ipv4 unicast <ip>

# RSVP认证
rsvp
  interface <interface>
    authentication
      key-source key-chain <name>

# MPLS TE GMPLS UNI
mpls traffic-eng
  gmpls optical-uni
    controller <controller-name>
```

### Juniper Junos RSVP/MPLS TE

```
[edit protocols mpls]
interface <interface>;
label-switched-path <lsp-name> {
    to <dest-ip>;
    bandwidth <bw>;
    priority <setup-priority> <hold-priority>;
    admin-group { include-any | include-all | exclude } <group-name>;
    primary <path-name> {
        bandwidth <bw>;
    }
    secondary <path-name> {
        standby;
    }
    fast-reroute;                                  # 链路保护FRR
    fast-reroute { node-protection; }              # 节点保护FRR
    record;                                        # 记录路由
    auto-bandwidth {
        minimum-bandwidth <bw>;
        maximum-bandwidth <bw>;
        adjust-interval <seconds>;
        adjust-threshold <percent>;
    }
    # MPLS OAM
    oam {
        bfd-liveness-detection {
            minimum-interval <ms>;
            multiplier <mult>;
        }
    }
}

# 显式路径
path <path-name> {
    <ip> { strict; }
    <ip> { loose; }
}

# 管理组定义
admin-groups {
    <group-name> <bit>;
}

# Bypass隧道(手动)
label-switched-path <bypass-name> {
    to <protected-nexthop>;
    bypass;
}

# 出口保护
egress-protection { ... }

[edit protocols rsvp]
interface <interface> {
    bandwidth <bw>;
    link-protection;                               # 链路保护
    authentication-key <password>;
    aggregate;                                     # 聚合预留
    subscription <percent>;                        # 带宽超订
}
```

### Display/Show命令

```
华为: display mpls te tunnel-interface [ <tunnel> ]           # TE隧道 ★
华为: display rsvp session [ <tunnel> ] [ detail ]            # RSVP会话
华为: display rsvp statistics                                  # RSVP错误
华为: display mpls te cspf destination <dest> [ bandwidth <bw> ]

Cisco: show mpls traffic-eng tunnels [ name <name> ] [ detail ]  # TE隧道 ★
Cisco: show rsvp session [ detail ]                              # RSVP会话
Cisco: show rsvp counters [ summary ]                            # RSVP统计
Cisco: show mpls traffic-eng topology [ <ip> ]                   # TE拓扑
Cisco: show mpls traffic-eng fast-reroute database               # FRR数据库

Juniper: show mpls lsp [ name <name> ] [ detail | extensive ]    # LSP隧道 ★
Juniper: show rsvp session [ detail | extensive ]                # RSVP会话
Juniper: show rsvp interface [ <interface> ]                     # RSVP接口
Juniper: show rsvp statistics                                    # RSVP统计
Juniper: show mpls interface                                     # MPLS接口
Juniper: show mpls label [ usage ]                               # 标签使用

# LSP Ping/Traceroute (RSVP-TE)
Cisco: ping mpls traffic-eng tunnel <number>
Cisco: traceroute mpls traffic-eng tunnel <number>
Juniper: ping mpls rsvp <lsp-name>
Juniper: traceroute mpls rsvp <lsp-name>
```

---

## 2.3 静态CR-LSP

### 华为 NE40E

```
# Ingress
static-cr-lsp ingress <name> destination <dest-ip> <mask> nexthop <nexthop> out-label <label>
# Transit
static-cr-lsp transit <name> incoming-interface <if> in-label <label> nexthop <nexthop> out-label <label>
# Egress
static-cr-lsp egress <name> incoming-interface <if> in-label <label>
```

---

## 2.4 MPLS TE FRR

### 华为 NE40E

```
# 隧道启用FRR
interface Tunnel <number>
  mpls te fast-reroute [ bandwidth <bw> ] [ hop-limit <limit> ]
  mpls te fast-reroute node-protection
  mpls te fast-reroute bandwidth-protection

# 手动Bypass隧道
interface Tunnel <number>
  tunnel-protocol mpls te
  destination <protected-next-hop>
  mpls te bypass-tunnel

# 自动FRR
mpls te
  auto-frr

# FRR联动BFD
mpls te
  bfd enable
  bfd min-tx-interval <ms> min-rx-interval <ms> detect-multiplier <mult>
```

### Cisco IOS-XR MPLS TE FRR

```
# 隧道启用FRR
interface tunnel-te <number>
  fast-reroute
  fast-reroute protect { node | bandwidth }

# 自动Bypass
mpls traffic-eng
  auto-tunnel backup

# MPLS Forwarding (IOS-XR特有)
mpls forwarding
  label-security interface <interface> rpf          # 入标签安全

# MPLS Static (IOS-XR)
mpls static
  interface <interface>
  address-family ipv4 unicast
    local-label <label> allocate
      forward
        path 1 nexthop <interface> <nexthop-ip> out-label { <label> | pop }

# MPLS OAM (IOS-XR)
mpls oam
  echo disable-vendor-extension
  echo revision 4                                   # RFC8029
  dpm                                               # Data Plane Monitoring
    interval <minutes>
    pps <pps>

# MPLS Performance Measurement
performance-measurement
  interface <interface>
    delay-measurement
      advertise-delay <threshold-ms>
  lsp <tunnel-name>
    delay-measurement
```

### Juniper Junos MPLS TE FRR (补充)

```
# FRR (在MPLS LSP层级配置)
label-switched-path <name> {
    fast-reroute;                                   # 链路保护
    fast-reroute {
        node-protection;                            # 节点保护
        bandwidth <bw>;                             # 保护带宽
        hop-limit <hops>;
    }
}

# 手动Bypass
label-switched-path <bypass-name> {
    to <protected-nh>;
    bypass;
}

# RSVP链路保护
[edit protocols rsvp]
interface <interface> {
    link-protection;
    bandwidth <bw>;
}
```

```
华为: display mpls te fast-reroute [ interface <if> ]
华为: display mpls te bypass-tunnel
Cisco: show mpls traffic-eng fast-reroute database [ interface <if> ]
Cisco: show mpls traffic-eng tunnels [ backup ]
Juniper: show mpls lsp bypass [ detail ]
Juniper: show rsvp session bypass [ detail ]
```
