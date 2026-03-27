# 第六章 安全

## 6.1 ACL

### 华为 NE40E

```
# 基本ACL (2000-2999)
acl [ number ] <acl-number> [ name <acl-name> ] [ match-order { auto | config } ]
  rule [ <rule-id> ] { permit | deny } [ source <source-addr> <wildcard> | any ]
  rule [ <rule-id> ] { permit | deny } source <source-addr> <wildcard> [ time-range <name> ]

# 高级ACL (3000-3999)
acl [ number ] <acl-number> [ name <acl-name> ]
  rule [ <rule-id> ] { permit | deny } { ip | tcp | udp | icmp | <protocol-number> }
    [ source <source-addr> <wildcard> ]
    [ destination <dest-addr> <wildcard> ]
    [ source-port { eq | gt | lt | range } <port> ]
    [ destination-port { eq | gt | lt | range } <port> ]
    [ dscp <dscp> ]
    [ tcp-flag { syn | ack | fin | rst | established } ]
    [ time-range <name> ]
    [ logging ]

# 二层ACL (4000-4999)
acl [ number ] <acl-number>
  rule { permit | deny } [ source-mac <mac> <mask> ] [ dest-mac <mac> <mask> ] [ type <code> <mask> ]

# 用户自定义ACL (5000-5999)
acl [ number ] <acl-number>
  rule { permit | deny } <rule-string>       # 基于报文偏移

# ACL应用到接口
interface <interface>
  traffic-filter { inbound | outbound } acl <acl-number>

# ACL应用到BGP/OSPF路由过滤
bgp <as>
  ipv4-family unicast
    peer <ip> filter-policy acl <number> { import | export }
ospf <process>
  filter-policy acl <number> { import | export }
```

### H3C S12500X

```
acl basic <acl-number> [ name <name> ]
  rule [ <id> ] { permit | deny } [ source <addr> <wildcard> | any ]
acl advanced <acl-number> [ name <name> ]
  rule [ <id> ] { permit | deny } { ip | tcp | udp | icmp | <protocol> }
    [ source <addr> <wildcard> ] [ destination <addr> <wildcard> ]
    [ source-port { eq | gt | lt | range } <port> ]
    [ destination-port { eq | gt | lt | range } <port> ]
    [ dscp <dscp> ] [ logging ]

# 应用
interface <interface>
  packet-filter <acl-number> { inbound | outbound }
```

### Cisco IOS-XR ACL

```
# IPv4 ACL
ipv4 access-list <acl-name>
  <seq> { permit | deny } { ipv4 | tcp | udp | icmp | <protocol-number> }
    [ <source-addr> <wildcard> | host <ip> | any ]
    [ <dest-addr> <wildcard> | host <ip> | any ]
    [ eq <port> | gt <port> | lt <port> | range <start> <end> ]
    [ dscp <dscp> ]
    [ established ]                                 # TCP已建立连接
    [ fragments ]                                   # IP分片
    [ log | log-input ]
    [ counter <counter-name> ]

# IPv6 ACL
ipv6 access-list <acl-name>
  <seq> { permit | deny } { ipv6 | tcp | udp | icmp } ...

# Object-Group ACL
object-group network ipv4 <group-name>
  <prefix>/<len>
  host <ip>
  range <start-ip> <end-ip>
object-group port <group-name>
  eq <port>
  range <start-port> <end-port>
ipv4 access-list <name>
  <seq> permit tcp net-group <src-group> port-group <port-group> any

# ACL应用到接口
interface <interface>
  ipv4 access-group <acl-name> { ingress | egress }

# ACL应用到BGP/OSPF
router bgp <as>
  neighbor <ip>
    address-family ipv4 unicast
      prefix-policy <prefix-set>
router ospf <process>
  distribute-list route-policy <name> in
```

### Juniper Junos 防火墙过滤器 (Firewall Filter)

```
[edit firewall]
family inet {
    filter <filter-name> {
        term <term-name> {
            from {
                # 匹配条件
                source-address { <prefix>/<len>; }
                destination-address { <prefix>/<len>; }
                source-port <port>;
                destination-port <port>;
                protocol [ tcp udp icmp ospf ];
                dscp <dscp>;
                ip-options <option>;
                fragment-flags <flags>;
                tcp-flags <flags>;                  # syn ack fin rst
                tcp-established;                    # 已建立TCP
                prefix-list <list-name>;
                source-prefix-list <list-name>;
                destination-prefix-list <list-name>;
                interface-group <group>;
                forwarding-class <class>;
                loss-priority { low | medium-low | medium-high | high };
                packet-length <range>;
            }
            then {
                # 动作
                accept;
                discard;
                reject [ <reject-type> ];           # 发送ICMP拒绝
                count <counter-name>;
                log;
                syslog;
                policer <policer-name>;
                forwarding-class <class>;
                loss-priority <level>;
                next term;                          # 继续匹配下一term
                routing-instance <instance>;
            }
        }
    }
}

# 应用到接口
[edit interfaces <interface> unit <unit>]
family inet {
    filter {
        input <filter-name>;
        output <filter-name>;
    }
}

# 回环(lo0)过滤器 — 控制面保护 ★
[edit interfaces lo0 unit 0]
family inet {
    filter {
        input protect-re;                           # RE保护过滤器
    }
}
```

### Display/Show命令

```
华为: display acl [ <acl-number> ] [ all ]            # ACL配置
华为: display acl <acl-number> [ detail ]              # 含match计数
H3C:  display acl [ <acl-number> ]
H3C:  display acl <acl-number> counter
华为: display traffic-filter applied-record             # 接口ACL应用

Cisco: show access-lists [ ipv4 ] [ <acl-name> ] [ hardware { ingress | egress } [ interface <if> ] [ detail ] ]  ★
Cisco: show access-lists ipv4 <name> [ usage | pfilter ] [ location <loc> ]

Juniper: show firewall [ filter <name> ]              # 防火墙过滤器统计 ★
Juniper: show firewall log [ detail ]                 # 防火墙日志
Juniper: show firewall policer [ <name> ]             # Policer统计
Juniper: show firewall counter [ filter <name> ]
```

---

## 6.2 本机防攻击

### 华为 NE40E

```
# CPU-Defend
cpu-defend policy <name>
  car { bgp | ospf | isis | ldp | rsvp | bfd | vrrp | ssh | telnet | snmp | icmp | arp | dhcp | ntp } cir <cir-pps> [ cbs <cbs> ]
  blacklist { acl <acl-number> | ip-prefix <prefix-name> }
  whitelist { acl <acl-number> | ip-prefix <prefix-name> }
cpu-defend-policy <name> global

# GTSM
bgp <as-number>
  peer <ip> valid-ttl-hops <hops>            # 仅接受TTL=255的BGP报文

# Keychain
keychain <name> mode periodic
  key-id <id>
    key-string [ cipher ] <password>
    send-lifetime <start> <end>
    receive-lifetime <start> <end>
    algorithm { md5 | sha-256 | hmac-sha-256 }
bgp <as-number>
  peer <ip> keychain <keychain-name>

# ARP安全
arp speed-limit source-ip <ip> maximum <max>
arp speed-limit source-mac <mac> maximum <max>
arp-miss speed-limit source-ip maximum <max>

# DHCP Snooping
dhcp snooping enable
interface <interface>
  dhcp snooping trusted
```

### Cisco IOS-XR 安全特性

```
# AAA/RADIUS/TACACS+
aaa authentication login <list-name> { group { radius | tacacs+ | <server-group> } | local }
aaa authorization exec <list-name> { group { radius | tacacs+ } | local }
aaa accounting exec <list-name> start-stop { group { radius | tacacs+ } }
radius-server host <ip> auth-port <port> acct-port <port> key [ encrypted ] <password>
tacacs-server host <ip> port <port> key [ encrypted ] <password>

# MPP (Management Plane Protection)
control-plane
  management-plane
    inband
      interface <interface>
        allow { ssh | telnet | snmp | http | netconf } [ peer address ipv4 <prefix>/<len> ]
    out-of-band
      interface <interface>
        allow { ssh | snmp }

# LPTS (Local Packet Transport Services) — IOS-XR特有 ★
# 控制面报文速率限制(自动保护)
lpts punt police
  protocol { bgp | ospf | isis | ldp | rsvp | bfd | icmp | arp | ssh | snmp | ntp }
    rate <pps>
lpts flow ipv4 filter
  location <loc>

# SSH/SSL
ssh server v2
ssh server dscp <dscp>
ssh server logging
ssh server rate-limit <attempts> [ timeout <seconds> ]
ssh server algorithms key-exchange { diffie-hellman-group14-sha1 | ecdh-sha2-nistp256 }

# IPSec
crypto isakmp policy <priority>
  encryption { aes 256 | aes 128 | 3des }
  hash { sha256 | sha1 | md5 }
  authentication { pre-share | rsa-sig }
  group <dh-group>
  lifetime <seconds>
ipsec transform-set <name> { esp-aes 256 | esp-3des } { esp-sha256-hmac | esp-sha-hmac }

# Keychain (TCP-AO/BGP认证)
key chain <name>
  key <key-id>
    key-string [ encrypted ] <password>
    accept-lifetime <start> <end>
    send-lifetime <start> <end>
    cryptographic-algorithm { hmac-md5 | hmac-sha1-12 | hmac-sha-256 | aes-128-cmac }

# MACsec
macsec-policy <name>
  security-policy { should-secure | must-secure }
  cipher-suite { gcm-aes-128 | gcm-aes-256 }
  window-size <size>
interface <interface>
  macsec psk-keychain <keychain-name> policy <name>

# FIPS模式
crypto fips-mode

# PKI
crypto ca trustpoint <name>
  enrollment url <url>
  subject-name <dn>
  crl optional
```

### Juniper Junos 安全(补充)

```
# 控制面保护 (lo0 filter) — 等同华为CPU-Defend
# 在lo0接口应用filter，按协议限速
[edit firewall family inet filter protect-re]
term allow-bgp {
    from {
        source-prefix-list trusted-bgp-peers;
        protocol tcp;
        destination-port bgp;
    }
    then {
        policer bgp-policer;
        accept;
    }
}
term allow-ospf { from { protocol ospf; } then { policer ospf-policer; accept; } }
term allow-bfd { from { protocol udp; destination-port [ 3784 3785 ]; } then accept; }
term allow-ssh { from { source-prefix-list mgmt-hosts; protocol tcp; destination-port ssh; } then accept; }
term allow-icmp { from { protocol icmp; } then { policer icmp-policer; accept; } }
term deny-all { then { count denied-to-re; log; discard; } }

# GTSM (Generalized TTL Security Mechanism)
[edit protocols bgp group <group> neighbor <ip>]
multihop { ttl <value>; }
# 或通过firewall filter匹配TTL

# Keychain
[edit security]
authentication-key-chains {
    key-chain <name> {
        key <key-id> {
            secret <password>;
            start-time <datetime>;
            algorithm { md5 | hmac-sha-1 | ao };
        }
    }
}
```

### Display/Show命令

```
华为: display cpu-defend policy [ <name> ]
华为: display cpu-defend statistics [ all | <protocol> ]
华为: display arp speed-limit

Cisco: show lpts pifib hardware police [ location <loc> ]    # LPTS限速 ★
Cisco: show lpts pifib hardware entry [ brief ]              # LPTS条目
Cisco: show ssh [ session details ]
Cisco: show crypto session [ detail ]                         # IPSec会话
Cisco: show crypto ipsec sa [ detail ]
Cisco: show macsec mka session [ interface <if> ]

Juniper: show firewall filter protect-re                      # RE保护统计 ★
Juniper: show system login                                     # 登录用户
Juniper: show security ipsec security-associations             # IPSec SA
Juniper: show security pki ca-certificate
```

---

## 6.3 URPF

### 华为 NE40E

```
# 接口模式
interface <interface>
  ip urpf { strict | loose }
  # strict: 源IP必须匹配FIB且入接口匹配(防欺骗,注意非对称路由)
  # loose:  源IP仅需FIB有路由(允许非对称路由)
  ip urpf loose allow-default                # 允许默认路由匹配

# VPN URPF
interface <interface>
  ip urpf { strict | loose } vpn-instance <vpn>

# 流模式URPF
traffic behavior <name>
  urpf { strict | loose }
```

```
华为: display urpf interface <interface>
华为: display urpf statistics interface <interface>
Cisco: show cef ipv4 <prefix> [ detail ]                     # 查看uRPF状态
Juniper: show route forwarding-table [ destination <prefix> ] # 验证源路由是否存在
```

**URPF排障**: strict模式+非对称路由=误丢包 → 用loose; `display urpf statistics` / `show cef drops` 检查drop是否增长

---

## 6.4 Cisco IOS-XR IP地址与服务 (补充)

```
# HSRP (热备路由协议 — Cisco版VRRP)
router hsrp
  interface <interface>
    address-family ipv4
      hsrp <group-id> version 2
        address <virtual-ip>
        priority <priority>                         # 默认100
        preempt [ delay minimum <seconds> ]
        timers <hello> <hold>
        track object <track-id> [ decrement <value> ]
        bfd fast-detect [ peer ipv4 <ip> interface <if> ]
        authentication <password>

# VRRP (IOS-XR)
router vrrp
  interface <interface>
    address-family ipv4
      vrrp <vrid> version 3
        address <virtual-ip>
        priority <priority>
        preempt [ delay <seconds> ]
        timer <seconds>
        track object <track-id> [ decrement <value> ]

# ARP
arp <ip> <mac> arpa [ alias ]
arp timeout <seconds>
interface <interface>
  arp timeout <seconds>
  arp learning { disable | local }

# DHCP
dhcp ipv4
  profile <profile-name> { server | relay | proxy }
    relay information option
    helper-address vrf <vrf> <ip>

# CEF (Cisco Express Forwarding) ★
show cef [ <prefix> ] [ detail ]                    # CEF转发表
show cef drops                                       # CEF丢包 ★★★ (排障静默丢包关键命令)
show cef inconsistency [ records ]
show cef vrf <vrf> [ <prefix> ] [ detail ]

# LPTS (Local Packet Transport Services) ★
show lpts pifib hardware police                      # 协议限速统计
show lpts pifib hardware entry brief                 # LPTS条目
show lpts ifib entries [ brief ]

# Prefix-list
prefix-set <name>
  <prefix>/<len>,
  <prefix>/<len> le <le> ge <ge>
end-set
```
