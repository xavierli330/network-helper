# 第八章 系统监控

## 8.1 NQA

### 华为 NE40E

```
# NQA测试例
nqa test-instance <admin-name> <test-name>
  test-type { icmp | jitter | tcp | udp | lsp-ping | lsp-jitter | lsp-trace | trace | path-jitter | dns | ftp | http }
  destination-address ipv4 <ip-address>
  source-address ipv4 <ip-address>
  frequency <seconds>                        # 测试频率
  timeout <ms>                               # 超时时间
  probe-count <count>                        # 探测次数
  interval <ms>                              # 探测间隔

  # ICMP测试
  test-type icmp
  destination-address ipv4 <ip>
  packet-size <size>
  ttl <ttl>

  # Jitter测试(时延抖动)
  test-type jitter
  destination-address ipv4 <ip>
  destination-port <port>
  probe-count <count>
  interval <ms>

  # LSP Ping测试
  test-type lsp-ping
  lsp-nexthop <ip-address>
  lsp-masklen <mask-length>

  # VPN实例NQA
  test-type icmp
  destination-address ipv4 <ip> vpn-instance <vpn-name>

  start now                                  # 立即启动

# NQA联动静态路由
ip route-static <dest> <mask> <if> <nexthop> track nqa <admin> <test>

# NQA联动Track
nqa track <track-id> test-instance <admin> <test> [ reaction-time <ms> ]
```

### Cisco IOS-XR IP SLA

```
# IP SLA测试例
ipsla
  operation <id>
    type { icmp echo | icmp path-echo | icmp path-jitter | udp echo | udp jitter | tcp connect | mpls lsp ping | mpls lsp trace }
    destination address <ip>
    source address <ip>
    frequency <seconds>
    timeout <ms>
    packet count <count>
    packet interval <ms>

    # ICMP Echo
    type icmp echo
      destination address <ip>
      packet size <size>

    # UDP Jitter
    type udp jitter
      destination address <ip>
      destination port <port>
      packet count <count>
      packet interval <ms>

    # MPLS LSP Ping
    type mpls lsp ping
      target traffic-eng tunnel <number>

    # VRF
    vrf <vrf-name>

  schedule operation <id>
    start-time now
    life { forever | <seconds> }

# IP SLA联动Track
track <id>
  type rtr <ipsla-id> reachability
  delay { up <seconds> | down <seconds> }

# TWAMP (Two-Way Active Measurement Protocol)
ipsla
  responder
    type udp ipv4 address <ip> port <port>
  operation <id>
    type udp echo
      destination address <ip>
      destination port <port>
```

### Cisco IOS-XR EEM (Embedded Event Manager)

```
event manager
  action <name>
    type script
      script <filename>
    type syslog
      pattern <regex>
  policy <name>
    event syslog pattern <regex>
    action <action-name>
      cli <command>
      syslog msg <message>

# EEM Applet示例
event manager policy <name>
  event syslog pattern "%OSPF-5-ADJCHG"
  action 1.0 cli command "show ospf neighbor"
  action 2.0 syslog msg "OSPF neighbor change detected"
```

### Cisco IOS-XR系统监控

```
# 告警管理
logging events level { emergencies | alerts | critical | errors | warnings | notifications | informational | debugging }
logging events threshold <count> <window-seconds>
logging correlator rule <name>
  type { stateful | nonstateful }
  reissue-nonbistate
  timeout <seconds>
  rootcause { <msg-category> <msg-group> <msg-code> }
  nonrootcause { <msg-category> <msg-group> <msg-code> }

# Logging Services
logging <ip> [ port <port> ] [ severity <level> ] [ vrf <vrf> ]
logging source-interface <interface>
logging hostnameprefix <hostname>
logging archive { device <device> | severity <level> | frequency { daily | weekly } | size <kb> }

# Performance Management
performance-mgmt
  resources memory
    poll-interval <seconds>
    threshold { used-percent | available }
  statistics interface { generic-counters | data-rates | basic-counters }
    poll-interval <seconds>

# Diagnostics
diagnostic monitor threshold <module> <interval>
show diagnostic result [ module <id> ] [ detail ]
show platform
show processes [ cpu | memory ]
show controllers <interface>
```

### Display/Show命令

```
华为: display nqa results [ <admin> <test> ]           # NQA结果 ★
华为: display nqa statistics [ <admin> <test> ]        # NQA统计
华为: display nqa-server status                         # NQA服务端状态

Cisco: show ipsla statistics [ <id> ] [ detail ]       # IP SLA结果 ★
Cisco: show ipsla operation [ <id> ]                   # IP SLA配置
Cisco: show ipsla reaction-trigger [ <id> ]
Cisco: show track [ <id> ]                             # Track状态
Cisco: show logging [ last <n> ]                       # 系统日志 ★
Cisco: show logging events buffer [ all-locations ]
Cisco: show logging correlator [ rule <name> ]
Cisco: show alarms [ brief | detail ]                  # 告警 ★
Cisco: show context [ all ]                             # 崩溃信息
Cisco: show event manager policy [ registered | available ]
Cisco: show processes cpu [ history ] [ location <loc> ]
Cisco: show memory summary [ location <loc> ]
Cisco: show platform [ detail ]
```

---

## 8.2 NetStream

### 华为 NE40E

```
# 原始流(Flow)采集
interface <interface>
  ip netstream { inbound | outbound }                   # 接口使能采集

# NetStream采样
interface <interface>
  ip netstream sampler { fix-packets <num> | fix-time <ms> | random-packets <num> }

# 聚合流
ip netstream aggregation { as | protocol-port | source-prefix | destination-prefix | prefix | tos-as }
  active-timeout <minutes>
  inactive-timeout <seconds>
  enable

# 流输出(导出到采集器)
ip netstream export version { 5 | 9 | ipfix }
ip netstream export source <ip-address>
ip netstream export host <collector-ip> <port> [ vpn-instance <vpn> ]

# 灵活流(Flexible Flow)
flow-template <template-name>
  match { source-address | destination-address | source-port | destination-port | protocol | dscp | input-interface | output-interface }
  collect { bytes | packets | timestamp }
```

### Cisco IOS-XR NetFlow

```
# Flow Exporter Map
flow exporter-map <name>
  version { v9 | ipfix }
  destination <collector-ip> port <port>
  source <source-interface>
  transport udp <port>
  dscp <dscp>
  template { data timeout <seconds> | options timeout <seconds> }

# Flow Monitor Map
flow monitor-map <name>
  record { ipv4 | ipv6 | mpls } [ { peer-as | origin-as } ]
  exporter <exporter-map-name>
  cache { entries <count> | timeout { active <seconds> | inactive <seconds> | update <seconds> } }

# Flow Sampler Map
sampler-map <name>
  random 1 out-of <rate>                          # 采样率(如1/1000)

# 接口应用
interface <interface>
  flow { ipv4 | ipv6 | mpls } monitor <monitor-map> sampler <sampler-map> { ingress | egress }

# sFlow
sflow agent { ipv4 | ipv6 } <ip>
sflow collector <collector-ip> port <port>
interface <interface>
  sflow { ingress | egress } { sample-rate <rate> }
```

### Juniper Junos Flow Monitoring

```
# NetFlow v9/IPFIX模板
[edit services flow-monitoring]
version9 {
    template <template-name> {
        { ipv4-template | ipv6-template | mpls-template | mpls-ipv4-template };
        flow-active-timeout <seconds>;
        flow-inactive-timeout <seconds>;
        option-refresh-rate { packets <count>; seconds <seconds>; }
    }
}
version-ipfix {
    template <template-name> {
        { ipv4-template | ipv6-template | mpls-template };
        flow-active-timeout <seconds>;
        flow-inactive-timeout <seconds>;
        nexthop-learning { enable; }
    }
}

# Inline Jflow (高性能内联采样)
[edit services flow-monitoring]
version9 {
    template <template-name> {
        flow-active-timeout <seconds>;
        flow-inactive-timeout <seconds>;
        ipv4-template;
    }
}

[edit chassis]
fpc <slot> {
    sampling-instance <instance-name>;
    inline-services {
        flex-flow-spec;
    }
}

[edit forwarding-options]
sampling {
    instance <instance-name> {
        input {
            rate <rate>;                             # 采样率
            run-length <length>;
            maximum-packet-length <bytes>;
        }
        family inet {
            output {
                flow-server <collector-ip> {
                    port <port>;
                    version9 {
                        template <template-name>;
                    }
                    source-address <source-ip>;
                }
                inline-jflow {
                    source-address <source-ip>;
                    flow-export-rate <pps>;
                }
            }
        }
    }
}

# 接口应用采样
[edit interfaces <interface> unit <unit>]
family inet {
    sampling {
        input;
        output;
    }
}
```

### Display/Show命令

```
华为: display ip netstream [ interface <if> ] [ cache ]
华为: display ip netstream export
华为: display ip netstream statistics

Cisco: show flow exporter [ <name> ]                  # 导出器状态
Cisco: show flow monitor [ <name> ] [ cache [ summary ] ]  # 流缓存 ★
Cisco: show sampler [ <name> ]                         # 采样器
Cisco: show sflow statistics

Juniper: show services accounting flow [ detail ]      # 流统计 ★
Juniper: show services accounting status               # 采集状态
Juniper: show services accounting errors               # 采集错误
Juniper: show services accounting memory               # 内存使用
Juniper: show services accounting flow-table [ detail ] # 流表
```

---

## 8.3 Telemetry

### 华为 NE40E

```
# gRPC Telemetry (Dial-In/Dial-Out)
telemetry
  sensor-group <name>
    sensor-path <yang-path>                  # 如: huawei-ifm:ifm/interfaces/interface
  destination-group <name>
    ipv4-address <ip> port <port> protocol grpc no-tls
  subscription <name>
    sensor-group <sensor-group-name>
    destination-group <dest-group-name>
    sample-interval <ms>                     # 采样间隔

# YANG Push
telemetry
  subscription <name>
    sensor-group <name>
    encoding { json | gpb }
    protocol { grpc | udp }

# OpenConfig Telemetry
grpc
  grpc server enable
  grpc server port <port>
```

### Cisco IOS-XR Telemetry

```
# Model-Driven Telemetry (MDT)
telemetry model-driven
  sensor-group <group-name>
    sensor-path <yang-path>                         # 如: Cisco-IOS-XR-infra-statsd-oper:infra-statistics
  destination-group <group-name>
    address-family ipv4 <ip> port <port>
      encoding { self-describing-gpb | gpb | json }
      protocol { grpc | tcp | udp }
  subscription <sub-name>
    sensor-group-id <group-name> sample-interval <ms>
    destination-id <group-name>
    source-interface <interface>

# gRPC Server (Dial-In模式)
grpc
  port <port>
  no-tls
  address-family { ipv4 | ipv6 | dual }
```

### Juniper Junos JTI (Juniper Telemetry Interface)

```
# gRPC传感器 (Junos Telemetry Interface)
[edit services analytics]
sensor <sensor-name> {
    server-name <server-name>;
    export-name <export-profile>;
    resource <yang-resource>;                       # 如 /junos/system/linecard/interface/
    resource-filter <filter>;
    polling-interval <seconds>;
    reporting-rate <seconds>;
}

streaming-server <server-name> {
    remote-address <ip>;
    remote-port <port>;
    tls { ... }
}

export-profile <profile-name> {
    reporting-rate <seconds>;
    format { gpb | json };
    transport { grpc | udp };
    local-address <ip>;
    local-port <port>;
}

# OpenConfig Telemetry (gRPC Dial-Out)
[edit system services extension-service request-response grpc]
clear-text {
    port <port>;
}
```

### Display/Show命令

```
华为: display telemetry subscription [ <name> ]
华为: display telemetry sensor-path [ <name> ]
华为: display grpc status

Cisco: show telemetry model-driven subscription [ <name> ]    # 订阅状态 ★
Cisco: show telemetry model-driven sensor-group [ <name> ]
Cisco: show telemetry model-driven destination [ <name> ]
Cisco: show grpc status
Cisco: show grpc trace all

Juniper: show analytics agent                                  # JTI代理状态 ★
Juniper: show analytics dialout statistics                     # 拨出统计
Juniper: show analytics streaming-server                       # 流式服务器
Juniper: show agent sensors                                    # 传感器列表
```

---

## 8.4 IFIT

### 华为 NE40E

```
# IFIT (In-band Flow Information Telemetry)
# 应用级质量检测
ifit
  flow <flow-name>
    match { acl <acl-number> | dscp <dscp> | source-address <ip> | destination-address <ip> }
    period <seconds>                         # 统计周期

# 隧道级质量检测
ifit
  tunnel <tunnel-name>
    period <seconds>

# IFIT端到端模式
ifit
  ingress-node                               # 入节点
  egress-node                                # 出节点
  transit-node                               # 中间节点
```

### Display命令

```
华为: display ifit flow [ <flow-name> ] statistics
华为: display ifit tunnel statistics
```

---

## 8.5 IP FPM (Flow Performance Measurement)

### 华为 NE40E

```
# 端到端性能统计
ip fpm
  flow <flow-name>
    match acl <acl-number>
    measure { delay | loss | both }
    period <seconds>

# 逐点性能统计
ip fpm
  flow <flow-name>
    per-hop enable
```

### Display命令

```
华为: display ip fpm statistics [ flow <name> ]
```

---

## 8.6 sFlow / TWAMP

### 华为 NE40E

```
# sFlow
sflow agent ip <ip-address>
sflow collector <collector-id> ip <collector-ip> port <port>
interface <interface>
  sflow sampling-rate <rate> [ inbound | outbound ]
  sflow counter interval <seconds>

# TWAMP (Two-Way Active Measurement Protocol)
twamp-light
  reflector <name>
    ip-address <local-ip>
    port <port>
    vpn-instance <vpn>

twamp-light
  sender <name>
    target-address <reflector-ip>
    target-port <port>
    count <count>
    interval <ms>
```

### Display命令

```
华为: display sflow [ statistics ]
华为: display twamp-light sender [ <name> ] statistics
华为: display twamp-light reflector [ <name> ]
```
