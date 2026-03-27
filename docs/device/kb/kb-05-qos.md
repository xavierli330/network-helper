# 第五章 QoS服务质量

## 5.1 MQC体系

> MQC: traffic classifier → traffic behavior → traffic policy → 应用到接口

### 华为 NE40E

```
# ============ Step 1: 流分类 ============
traffic classifier <name> [ operator { or | and } ]   # 默认or
  if-match acl <acl-number>
  if-match dscp <dscp-value>                    # 0-63
  if-match mpls-exp <exp-value>                 # 0-7
  if-match vlan-id <vlan-id>
  if-match 8021p <value>                        # 802.1p CoS 0-7
  if-match ip-precedence <precedence>           # 0-7
  if-match source-address <ip> <wildcard>
  if-match destination-address <ip> <wildcard>
  if-match protocol { ip | tcp | udp | icmp }
  if-match any                                  # 所有流量

# ============ Step 2: 流行为 ============
traffic behavior <name>
  # 流量监管CAR
  car cir <cir-kbps> [ cbs <cbs-bytes> ] [ pir <pir-kbps> ] [ pbs <pbs-bytes> ] [ green pass | yellow { pass | discard } | red discard ]
  # 流量整形
  queue shaping [ shaping-percentage <pct> ] [ cir <cir-kbps> [ pir <pir-kbps> ] ]
  # 重标记
  remark dscp <dscp>
  remark mpls-exp <exp>
  remark local-precedence <value>               # 内部队列映射
  remark 8021p <cos>
  # 丢弃
  deny
  # 重定向(策略路由)
  redirect ip-nexthop <ip>
  redirect interface <interface>
  # 队列调度
  queue { af | ef | be | llq } [ bandwidth <bw-kbps> | bandwidth-percentage <pct> ]
  queue wred { dscp | ip-precedence }
  # 统计
  statistic enable

# ============ Step 3: 流策略 ============
traffic policy <name>
  classifier <classifier-name> behavior <behavior-name>

# ============ Step 4: 应用 ============
interface <interface>
  traffic-policy <policy-name> { inbound | outbound }
# 全局
traffic-policy <policy-name> global { inbound | outbound }
```

### H3C S12500X

```
traffic classifier <name> [ operator { or | and } ]
  if-match acl <acl> / if-match dscp <dscp> / if-match mpls-exp <exp> / if-match any
traffic behavior <name>
  car cir <cir-kbps> [ cbs <cbs> ] [ ebs <ebs> ] [ green pass | yellow pass | red discard ]
  remark dscp <dscp> / remark dot1p <cos>
  filter deny
  redirect interface <interface>
  accounting
qos policy <name>
  classifier <name> behavior <name>
interface <interface>
  qos apply policy <name> { inbound | outbound }
```

### Cisco IOS-XR (ASR9000) MQC

```
# ============ Step 1: 流分类 (Class-Map) ============
class-map [ type qos ] [ match-any | match-all ] <name>
  match dscp <dscp-value>                           # 0-63 或名称(ef/af11/cs1等)
  match mpls experimental topmost <exp>             # MPLS EXP 0-7
  match access-group [ ipv4 | ipv6 ] <acl-name>
  match protocol <protocol>
  match precedence <value>
  match cos <value>                                 # 802.1p
  match qos-group <group>
  match source-address ipv4 <prefix>/<len>
  match destination-address ipv4 <prefix>/<len>
  match [ not ] <condition>                         # 逻辑非

# ============ Step 2: 策略映射 (Policy-Map) ============
policy-map <name>
  class <class-name>
    # 流量监管 (Police)
    police rate <bps> [ burst <bytes> ] [ peak-rate <bps> peak-burst <bytes> ]
      conform-action { transmit | set dscp <dscp> | set mpls experimental topmost <exp> }
      exceed-action { drop | set dscp <dscp> | set precedence <value> }
      violate-action { drop | set dscp <dscp> }
    # 或百分比
    police rate percent <pct>

    # 重标记
    set dscp <dscp>
    set mpls experimental topmost <exp>
    set qos-group <group>
    set cos <value>
    set precedence <value>

    # 队列调度
    priority [ level { 1 | 2 } ]                    # 严格优先级(LLQ)
    bandwidth { <kbps> | percent <pct> }             # 最小保证带宽
    bandwidth remaining { percent <pct> }            # 剩余带宽比例
    queue-limit <packets> [ <unit> ]                 # 队列深度

    # 拥塞避免 (WRED)
    random-detect dscp <dscp> <min-thresh> <max-thresh> <mark-prob>
    random-detect precedence <prec> <min-thresh> <max-thresh> <mark-prob>
    random-detect exponential-weighting-constant <exp>

    # 整形
    shape average { <bps> | percent <pct> }

  class class-default
    bandwidth remaining percent <pct>
    queue-limit <packets>

# ============ Step 3: 应用 ============
interface <interface>
  service-policy { input | output } <policy-name>

# H-QoS (分层QoS)
policy-map <parent-policy>
  class class-default
    service-policy <child-policy>
    shape average <bps>

# Link Bundle QoS
bundle-ether <id>
  service-policy output <policy>
  bundle maximum-active links <num>

# ANCP (Access Node Control Protocol)
ancp
  sender name <name>
    peer address ipv4 <ip>
```

### Juniper Junos CoS (Class-of-Service)

```
# ============ 转发类定义 ============
[edit class-of-service]
forwarding-classes {
    class <class-name> queue-num <0-7> {
        priority { low | medium-low | medium-high | high };
    }
}

# ============ 调度器 ============
schedulers {
    <scheduler-name> {
        transmit-rate { <bps> | percent <pct> | remainder };
        shaping-rate { <bps> | percent <pct> };
        priority { low | medium-low | medium-high | high | strict-high };
        buffer-size { percent <pct> | remainder | temporal <usec> };
        excess-rate { <bps> | percent <pct> | proportional };
        drop-profile-map {
            loss-priority { low | medium-low | medium-high | high } protocol any {
                drop-profile <profile-name>;
            }
        }
    }
}

# ============ 调度器映射 ============
scheduler-maps {
    <map-name> {
        forwarding-class <class-name> scheduler <scheduler-name>;
    }
}

# ============ 丢弃配置 (WRED) ============
drop-profiles {
    <profile-name> {
        fill-level <percent> drop-probability <percent>;
        # 或插值模式
        interpolate {
            fill-level [ <pct1> <pct2> ];
            drop-probability [ <pct1> <pct2> ];
        }
    }
}

# ============ 重写规则 ============
rewrite-rules {
    dscp <rule-name> {
        forwarding-class <class> {
            loss-priority { low | high } code-point <dscp>;
        }
    }
    ieee-802.1 <rule-name> {
        forwarding-class <class> {
            loss-priority low code-point <cos>;
        }
    }
    exp <rule-name> {
        forwarding-class <class> {
            loss-priority low code-point <exp>;
        }
    }
}

# ============ 分类器 (BA Classifier) ============
classifiers {
    dscp <classifier-name> {
        forwarding-class <class> {
            loss-priority { low | high } code-points <dscp>;
        }
    }
    exp <classifier-name> { ... }
    ieee-802.1 <classifier-name> { ... }
}

# ============ 应用到接口 ============
[edit class-of-service]
interfaces {
    <interface> {
        scheduler-map <map-name>;
        unit <unit> {
            classifiers dscp <classifier-name>;
            rewrite-rules dscp <rule-name>;
            forwarding-class-map { ... }
        }
        shaping-rate <bps>;                         # 端口整形
    }
}

# ============ 流量监管 (Policer) ============
[edit firewall]
policer <policer-name> {
    if-exceeding {
        bandwidth-limit <bps>;
        burst-size-limit <bytes>;
    }
    then {
        discard;
        # 或 loss-priority high;
        # 或 forwarding-class <class>;
    }
}
# 在防火墙过滤器中引用policer
filter <name> {
    term <term> {
        then {
            policer <policer-name>;
            accept;
        }
    }
}
```

### Display/Show命令

```
# QoS策略统计 ★
华为: display traffic policy statistics interface <if> { inbound | outbound } [ verbose ]
H3C:  display qos policy interface <if> { inbound | outbound }
Cisco: show policy-map interface <if> [ input | output ]      ★
Juniper: show class-of-service interface <if> [ comprehensive ]  ★

# 队列统计
华为: display qos queue statistics interface <if>
H3C:  display qos queue statistics interface <if>
Cisco: show qos interface <if> [ input | output ]
Juniper: show class-of-service interface <if> [ queue ] [ detail ]

# 策略定义查看
华为: display traffic classifier [ user-defined ]
华为: display traffic behavior [ user-defined ]
华为: display traffic policy [ user-defined ] [ <name> ]
Cisco: show class-map [ <name> ]
Cisco: show policy-map [ <name> ]
Juniper: show class-of-service scheduler-map [ <name> ]
Juniper: show class-of-service drop-profile [ <name> ]
Juniper: show class-of-service forwarding-class
Juniper: show class-of-service classifier [ <name> ]
Juniper: show class-of-service rewrite-rule [ <name> ]

# Policer统计
Juniper: show policer [ <name> ]
Juniper: show firewall policer [ <name> ]
```

**队列统计关键字段**:
| 字段 | 排障关注 |
|:---|:---|
| Passed Packets/Bytes | 确认流量分类正确 |
| Dropped Packets/Bytes | **非零=拥塞** |
| Queue Length | 持续高=拥塞 |
| Tail Drop / WRED Drop | 区分拥塞类型 |

---

## 5.2 流量监管与整形

### 华为 NE40E

```
# 接口级CAR
interface <interface>
  qos car inbound cir <cir-kbps> [ cbs <cbs> ] [ green pass | red discard ]
  qos car outbound cir <cir-kbps>

# 端口整形/限速
interface <interface>
  qos lr outbound cir <cir-kbps> [ cbs <cbs> ]
  port shaping <shaping-kbps>
```

---

## 5.3 HQoS

### 华为 NE40E

```
# 用户级调度
user-queue <name>
  cir <cir-kbps>
  pir <pir-kbps>
# 用户组
user-group <name>
  cir <cir-kbps>
  pir <pir-kbps>
# HQoS Profile
hqos-profile <name>
  user-queue <name>
  user-group <name>
# 应用
interface <interface>
  hqos-profile <name> outbound
```
