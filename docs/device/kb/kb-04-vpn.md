# 第四章 VPN技术

## 4.1 BGP/MPLS IP VPN (L3VPN)

### 华为 NE40E

```
# VPN实例
ip vpn-instance <vpn-name>
  ipv4-family
    route-distinguisher <rd>                 # <IP>:<num> 或 <AS>:<num>
    vpn-target <rt> { import-extcommunity | export-extcommunity | both }
    tunnel-policy <policy-name>              # 绑定隧道策略
    import route-policy <name>               # VPN引入策略
    export route-policy <name>               # VPN导出策略
    route-limit <max> [ <percentage> ] [ alert-only ]

# 接口绑定 ★绑VPN后接口IP清除，需重配
interface <interface>
  ip binding vpn-instance <vpn-name>
  ip address <ip> <mask>

# PE-CE BGP邻居
bgp <as-number>
  ipv4-family vpn-instance <vpn-name>
    peer <ce-ip> as-number <ce-as>
    peer <ce-ip> enable
    import-route { static | direct | ospf | isis } [ route-policy <name> ]
    network <ip> <mask-len>

# PE间VPNv4
bgp <as-number>
  ipv4-family vpnv4
    peer <pe-ip> enable
    peer <pe-ip> next-hop-local
    peer <pe-ip> reflect-client              # RR上
```

### H3C S12500X

```
ip vpn-instance <vpn-name>
  route-distinguisher <rd>
  vpn-target <rt> { import-extcommunity | export-extcommunity | both }
interface <interface>
  ip binding vpn-instance <vpn-name>
  ip address <ip> <mask>
bgp <as-number>
  address-family vpn-instance <vpn-name>
    peer <ce-ip> as-number <ce-as>
    import-route { static | direct | ospf | isis } [ route-policy <name> ]
  address-family vpnv4
    peer <pe-ip> enable
```

### Cisco IOS-XR L3VPN

```
# VRF定义
vrf <vrf-name>
  address-family ipv4 unicast
    import route-target <rt>
    export route-target <rt>
    import route-policy <name>
    export route-policy <name>
    maximum prefix <max> [ <threshold-pct> ] [ warning-only ]

# 接口绑定
interface <interface>
  vrf <vrf-name>
  ipv4 address <ip> <mask>

# PE-CE BGP
router bgp <as-number>
  vrf <vrf-name>
    rd <rd>
    address-family ipv4 unicast
      redistribute { static | connected | ospf <process> } [ route-policy <name> ]
      network <prefix>/<len>
    neighbor <ce-ip>
      remote-as <ce-as>
      address-family ipv4 unicast
        route-policy <name> { in | out }
        as-override                                # Hub-Spoke场景
        send-community-ebgp / send-extended-community-ebgp

# PE间VPNv4
router bgp <as-number>
  neighbor <pe-ip>
    address-family vpnv4 unicast
      route-reflector-client                       # RR上
      next-hop-self
```

### Juniper Junos L3VPN

```
[edit routing-instances]
<vrf-name> {
    instance-type vrf;
    interface <interface>;
    route-distinguisher <rd>;
    vrf-target <rt>;                               # import + export
    vrf-import <import-policy>;                    # 精细RT控制
    vrf-export <export-policy>;
    vrf-table-label;                               # 自动分配VPN标签
    routing-options {
        static {
            route <prefix> next-hop <nh>;
        }
        auto-export;                               # VPN间路由泄露
    }
    protocols {
        bgp {
            group <ce-group> {
                type external;
                peer-as <ce-as>;
                neighbor <ce-ip>;
                import <policy>;
                export <policy>;
                as-override;                       # Hub-Spoke
            }
        }
        ospf {
            area <area-id> {
                interface <interface>;
            }
            export <policy>;
        }
    }
}

# PE间VPNv4
[edit protocols bgp group <ibgp-group>]
family inet-vpn {
    unicast;
}
neighbor <pe-ip>;
cluster <cluster-id>;                              # RR上
```

### Display/Show命令

```
华为: display ip vpn-instance [ <name> ] [ verbose ]          # VPN实例 ★
华为: display ip routing-table vpn-instance <vpn>              # VPN路由
华为: display bgp vpnv4 all routing-table                     # VPNv4全局
华为: display bgp vpnv4 vpn-instance <vpn> routing-table [ <ip> ]
华为: display fib vpn-instance <vpn> [ <ip> ]                 # VPN FIB
华为: display tunnel-policy [ <name> ]                         # 隧道策略
华为: display tunnel-info [ all ] [ destination <ip> ]         # 隧道信息

Cisco: show vrf [ <vrf-name> ] [ detail ]                     # VRF实例 ★
Cisco: show route vrf <vrf-name> [ <prefix> ]                 # VPN路由
Cisco: show bgp vpnv4 unicast [ rd <rd> ]                     # VPNv4全局
Cisco: show bgp vrf <vrf-name> [ <prefix> ] [ detail ]
Cisco: show cef vrf <vrf-name> [ <prefix> ]                   # VPN FIB

Juniper: show route instance [ <name> ] [ detail ]            # 路由实例 ★
Juniper: show route table <vrf>.inet.0 [ <prefix> ]           # VPN路由
Juniper: show route table bgp.l3vpn.0                         # VPNv4全局
Juniper: show route forwarding-table table <vrf>              # VPN FIB
```

**L3VPN排障检查链**:
1. VPN实例 → RD/RT匹配? (`display ip vpn-instance verbose` / `show vrf detail` / `show route instance detail`)
2. PE-CE BGP → Established? (`display bgp vpnv4 vpn-instance <vpn> peer` / `show bgp vrf <vrf> summary`)
3. VPN路由 → 本地/远端路由存在? (`display bgp vpnv4 vpn-instance <vpn> routing-table` / `show bgp vrf <vrf>`)
4. 传输隧道 → LSP/SR到远端PE可达? (`display mpls lsp` / `show mpls forwarding` / `show route table mpls.0`)
5. 隧道策略 → 绑定正确? (`display tunnel-policy`)
6. VPN FIB → 已下发转发? (`display fib vpn-instance` / `show cef vrf` / `show route forwarding-table table <vrf>`)

---

## 4.2 跨域VPN

### Option A (VRF-to-VRF)
```
# ASBR上创建VPN实例,通过子接口互联
ip vpn-instance <vpn-name>
  route-distinguisher <rd>
  vpn-target <rt> both
interface <sub-interface>
  ip binding vpn-instance <vpn-name>
  ip address <ip> <mask>
bgp <as-number>
  ipv4-family vpn-instance <vpn-name>
    peer <asbr2-ip> as-number <remote-as>
```

### Option B (MP-EBGP between ASBRs)
```
# ASBR配置VPNv4 EBGP
bgp <as-number>
  ipv4-family vpnv4
    peer <asbr2-ip> as-number <remote-as>
    peer <asbr2-ip> enable
    peer <asbr2-ip> label-route-capability    # ★ 标签重分配
interface <asbr-interconnect>
  mpls
```

### Option C (Multi-Hop MP-EBGP + 标签路由)
```
# 1.ASBR间交换标签路由
bgp <as-number>
  ipv4-family unicast
    peer <asbr2-ip> as-number <remote-as>
    peer <asbr2-ip> label-route-capability

# 2.PE间直接建立VPNv4邻居
bgp <as-number>
  ipv4-family vpnv4
    peer <remote-pe-ip> as-number <remote-as>
    peer <remote-pe-ip> ebgp-max-hop <hops>
    peer <remote-pe-ip> connect-interface LoopBack <num>
```

---

## 4.3 EVPN VPLS/VPWS

### 华为 NE40E

```
# EVPN VPLS
evpn vpn-instance <name> bd-mode
  route-distinguisher <rd>
  vpn-target <rt> { import-extcommunity | export-extcommunity | both }
bridge-domain <bd-id>
  evpn binding vpn-instance <name>
interface <interface>
  l2 binding bridge-domain <bd-id>
bgp <as-number>
  l2vpn-family evpn
    peer <ip> enable
    peer <ip> reflect-client                 # RR

# EVPN VPWS
evpn vpn-instance <name> vpws-mode
  route-distinguisher <rd>
  vpn-target <rt> both
  local-service-id <local-id> remote-service-id <remote-id>

# EVPN IRB
bridge-domain <bd-id>
  vxlan vni <vni-id>
  evpn binding vpn-instance <name>
interface Vbdif <bd-id>
  ip binding vpn-instance <l3vpn>
  ip address <ip> <mask>
  mac-address <mac>
  arp collect host enable
```

### H3C

```
bgp <as-number>
  address-family l2vpn evpn
    peer <ip> enable
```

### Cisco IOS-XR EVPN/L2VPN

```
# L2VPN Xconnect (P2P伪线)
l2vpn
  xconnect group <group-name>
    p2p <xconnect-name>
      interface <ac-interface>
      neighbor ipv4 <pe-ip> pw-id <pw-id>
        pw-class <class-name>

# VPLS (Bridge Domain)
l2vpn
  bridge group <group-name>
    bridge-domain <bd-name>
      interface <interface>
      vfi <vfi-name>
        neighbor <pe-ip> pw-id <pw-id>
        autodiscovery bgp
          signaling-type bgp
          rd <rd>
          route-target <rt>

# EVPN
evpn
  evi <evi-id>
    advertise-mac
    bgp route-target <rt>
l2vpn
  bridge group <group>
    bridge-domain <bd>
      interface <interface>
      evi <evi-id>

# EVPN VPWS
evpn
  evi <evi-id>
    vpws
      local-service-id <id> remote-service-id <id>

# VXLAN/NVE
interface nve <id>
  member vni <vni-id>
    host-reachability protocol bgp-evpn
    ingress-replication protocol bgp-evpn

# L2VPN ACL
l2vpn
  bridge group <group>
    bridge-domain <bd>
      mac { limit { maximum <max> } | secure }

# GRE隧道
interface tunnel-ip <number>
  tunnel mode gre ipv4
  tunnel source <ip>
  tunnel destination <ip>
  ipv4 address <ip> <mask>
```

### Juniper Junos EVPN/VPN

```
# L2VPN (Kompella/Martini)
[edit protocols l2vpn]
encapsulation-type { ethernet | ethernet-vlan };
site <site-name> {
    site-identifier <id>;
    interface <interface> {
        remote-site-id <id>;
    }
}
control-word;                                      # 控制字

# VPLS
[edit protocols vpls]
site <site-name> {
    site-identifier <id>;
    interface <interface>;
}
vpls-id <id>;
connectivity-type { ce | irb };
bum-hashing;                                       # BUM流量哈希
mesh-group <name> {
    neighbor <pe-ip>;
}

# EVPN
[edit protocols evpn]
encapsulation vxlan;
extended-vni-list [ <vni-id> ];
default-gateway { advertise | do-not-advertise };
duplicate-mac-detection {
    detection-threshold <threshold>;
    detection-window <seconds>;
    auto-recovery-time <seconds>;
}
# EVPN assisted-replication
assisted-replication {
    replicator;
    leaf;
}
# EVPN SRv6
source-packet-routing {
    srv6 {
        locator <locator-name>;
    }
}

# EVPN路由实例
[edit routing-instances]
<evpn-name> {
    instance-type { evpn | virtual-switch };
    vlan-id <id>;
    interface <interface>;
    route-distinguisher <rd>;
    vrf-target <rt>;
    protocols {
        evpn;
    }
}

# L2VPN Show命令
Juniper: show l2vpn connections [ extensive ]
Juniper: show vpls connections [ extensive ]
```

### Display/Show命令

```
华为: display evpn vpn-instance [ name <name> ] [ verbose ]
华为: display bgp evpn all routing-table [ type { 1 | 2 | 3 | 4 | 5 } ]
# Type 1=Ethernet Auto-Discovery, Type 2=MAC/IP(★最常用)
# Type 3=Inclusive Multicast, Type 4=Ethernet Segment, Type 5=IP Prefix
华为: display evpn mac routing-table
华为: display bridge-domain [ <bd-id> ]
H3C:  display bgp l2vpn evpn routing-table

Cisco: show evpn evi [ <evi-id> ] [ detail ]                  # EVPN实例 ★
Cisco: show bgp l2vpn evpn [ rd <rd> ] [ route-type { 1-5 } ]
Cisco: show l2vpn bridge-domain [ <bd-name> ] [ detail ]      # BD信息
Cisco: show l2vpn xconnect [ group <name> ] [ detail ]        # P2P伪线
Cisco: show evpn ethernet-segment [ detail ]
Cisco: show l2vpn forwarding bridge-domain <bd> mac-address    # MAC表

Juniper: show evpn database [ instance <name> ]                # EVPN数据库 ★
Juniper: show evpn arp-table [ instance <name> ]               # EVPN ARP
Juniper: show evpn instance [ <name> ] [ extensive ]
Juniper: show route table <instance>.evpn.0 [ detail ]
Juniper: show l2vpn connections [ extensive ]
Juniper: show vpls connections [ extensive ]
Juniper: show vpls mac-table [ instance <name> ]
```

---

## 4.4 隧道策略

### 华为 NE40E

```
tunnel-policy <name>
  tunnel select-seq { cr-lsp | ldp | sr-te-policy | gre | sr-be | srv6-te-policy | srv6-be } [ load-balance-number <num> ]

# 示例: 优先SR-TE,备份LDP
tunnel-policy prefer-sr
  tunnel select-seq sr-te-policy ldp load-balance-number 1

# VPN绑定
ip vpn-instance <vpn>
  ipv4-family
    tunnel-policy <name>
```

```
华为: display tunnel-policy [ <name> ]
华为: display tunnel-info [ all | { sr-te-policy | ldp | cr-lsp } ] [ destination <ip> ]
```
