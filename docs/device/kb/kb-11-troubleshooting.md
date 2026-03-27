# 第十一章 排障思路与方法论

## 11.1 故障域分类

```yaml
fault_domains:
  ip_domain:
    symptoms:
      - "设备A无法到达设备B(公网IP)"
      - "非对称路径——正向OK反向丢包"
      - "流量走次优路径"
      - "路由震荡/频繁收敛"
    investigation_order:
      - IGP邻居状态(Full ≠ 转发OK)
      - 路由表 + 递归下一跳解析
      - FIB/CEF一致性(vs RIB)
      - 接口物理状态 + 错误计数器
      - 双向traceroute对比
      - 疑似段流量统计

  label_domain:
    symptoms:
      - "VPN流量黑洞——标签路径存在但不转发"
      - "部分VPN可达——某些FEC无标签"
      - "LSP建立但流量丢弃"
    investigation_order:
      - LDP/RSVP会话状态
      - 特定FEC的标签绑定(上下游)
      - LFIB条目存在性和正确性
      - MPLS LSP Ping/Traceroute端到端
      - 标签资源利用率(接近耗尽?)
      - PHP行为(倒数第二跳弹出)
      - VPN标签 vs 传输标签区分

  silent_drop:
    symptoms:
      - "无告警无日志但业务降级"
      - "间歇性丢包——无法复现"
      - "Ping正常但应用失败"
    root_cause_matrix:
      physical_layer: [CRC错误(光模块老化), 对齐错误(线缆问题), 光功率临界]
      label_layer: [标签栈MTU超限(静默丢弃), LFIB过期(下一跳不可达), 标签耗尽]
      queue_layer: [尾丢弃低于告警阈值, 微秒级突发被5秒监控间隔平滑, QoS队列跨域不匹配]
      forwarding_layer: [FIB/RIB不一致, ACL静默丢弃(无log), uRPF在非对称路径上丢弃]

  te_tunnel:
    symptoms:
      - "TE隧道Oper Down但Admin Up"
      - "TE隧道震荡"
      - "TE隧道Up但流量不走隧道"
    five_layer_method:
      - "Layer 1 Physical: 端口状态/光功率/CRC"
      - "Layer 2 Link: VLAN匹配/LAG状态/协议状态"
      - "Layer 3 Protocol: RSVP会话/PathErr-ResvErr/TED同步"
      - "Layer 4 Forwarding: LSP完整性/标签栈正确性/LFIB"
      - "Layer 5 Service: 隧道策略绑定/流量匹配/BFD联动"

  convergence:
    symptoms:
      - "链路故障后路由不一致持续数秒"
      - "SR Policy切换8-12秒(应<50ms)"
      - "PE重启后VPN路由恢复需2分钟"
    focus:
      - "IS-IS: LSP生成(ms) → flooding(常是瓶颈!) → SPF(ms)"
      - "BGP: scan interval / add-path / RR拓扑 / dampening"
      - "FRR: BFD检测 → 预计算备份 → 回切延迟"
      - "PCE: CSPF计算时间 / 并发策略重算队列"
```

---

## 11.2 IP域五步法

```
Step 1 → SCOPE: 定义故障边界
  端到端中断 or 部分丢包?
  单向 or 双向?
  单前缀 or 多前缀?
  → 输出: 故障范围声明

Step 2 → SEGMENT: 确定故障域
  IP域(IGP/路由) or 标签域(MPLS/SR)?
  用MPLS LSP Ping独立测试标签路径
  → 输出: 故障域确定

Step 3 → TRACE: 逐跳走转发路径
  每跳验证: RIB → FIB → 接口 → 物理
  检查双向(正向路径可能与反向不同)
  → 输出: 疑似故障段

Step 4 → COMPARE: 双向路径分析
  正向traceroute vs 反向traceroute
  正向计数器 vs 反向计数器
  不对称 = 可能的根因区域
  → 输出: 方向性故障定位

Step 5 → PROVE: 流统计 + 定向抓包
  在疑似段部署双向流统计
  需要时: 疑似跳入口和出口抓包
  关联: 跳N发送数 vs 跳N+1接收数
  → 输出: 数据证据确认根因
```

---

## 11.3 TE隧道五层法

```
Layer 1 → PHYSICAL: 端口Up/Down? 光功率? CRC/FCS错误? → 有错误: 换光模块/线缆
Layer 2 → LINK: VLAN标签匹配? LAG成员健康? → 不匹配: 修L2配置
Layer 3 → PROTOCOL: RSVP会话? PathErr/ResvErr? TED同步? → PathErr: 查中间节点配置变更
Layer 4 → FORWARDING: LSP端到端? 标签栈正确? LFIB? → 标签缺失: 查LDP/RSVP标签分发
Layer 5 → SERVICE: 流量匹配隧道? tunnel-policy绑定? BFD联动? → 流量绕过: 查策略绑定和FIB
```

---

## 11.4 静默丢包二分法

```
Step 1 → DIRECTION: 哪个方向丢包?
  用SNMP/Telemetry计数器(零登录)
  或: MPLS LSP Ping/Traceroute(单条命令)

Step 2 → BISECT: 二分搜索缩小范围
  8跳路径示例:
    探测TTL=4 → 丢包? → 问题在前半段
    探测TTL=2 → 正常? → 问题在跳2-4之间
    探测TTL=3 → 丢包! → 问题在跳3
  3次探测代替8次顺序检查

Step 3 → PRIORITIZE: 高风险设备优先
  ① 最近变更的设备
  ② 历史故障设备
  ③ 最高利用率链路
  ④ 接近寿命终止的光模块
  ⑤ 跨厂商互联点

Step 4 → DEEP DIVE: 物理→队列→标签→ACL
  Physical: CRC/光功率/对齐错误
  Queue: 尾丢弃/WRED/缓冲区利用率
  Label: LFIB一致性/标签栈深度vs MTU
  ACL/Policy: 静默deny/uRPF丢弃

Step 5 → VERIFY: 负载下验证修复
  修复 → 流统计确认零丢包 → 持续≥5分钟
  大包Ping测试(ping -s 1400+)排除MTU问题
```

---

## 11.5 常见故障快速定位表

| 现象 | 华为首查命令 | Cisco IOS-XR首查命令 | Juniper Junos首查命令 | 可能根因 |
|:---|:---|:---|:---|:---|
| OSPF邻居卡ExStart | `display ospf peer` | `show ospf neighbor detail` | `show ospf neighbor extensive` | MTU不匹配 |
| BGP卡Active | `display bgp peer <ip> verbose` | `show bgp neighbors <ip>` | `show bgp neighbor <ip>` | TCP179被阻/路由不可达/对端未配 |
| VPN路由缺失 | `display bgp vpnv4 vpn-instance <vpn> routing-table` | `show bgp vrf <vrf>` | `show route table <vrf>.inet.0` | RT不匹配/RR未反射/传输隧道断 |
| 标签路径黑洞 | `ping lsp ip <dest> <mask>` + `display mpls lsp verbose` | `ping mpls ipv4 <dest>/<mask>` + `show mpls forwarding` | `ping mpls ldp <dest>/<mask>` + `show route table mpls.0` | LFIB过期/标签耗尽/PHP异常 |
| SR Policy Oper Down | `display segment-routing te policy name <name>` | `show segment-routing traffic-eng policy name <name>` | `show spring-traffic-engineering lsp name <name>` | 段列表标签不可达/约束不满足 |
| BFD频繁震荡 | `display bfd session all verbose` | `show bfd session detail` | `show bfd session extensive` | 光功率临界/微秒级丢包/定时器过激进 |
| VRRP双Master | `display vrrp` | `show vrrp detail` / `show hsrp detail` | `show vrrp detail` | VLAN不通/认证不匹配/ACL过滤通告 |
| QoS丢包 | `display qos queue statistics interface <if>` | `show policy-map interface <if>` | `show class-of-service interface <if> queue` | 队列拥塞/CAR限速/整形不足 |
| 接口有丢包无告警 | `display interface <if> \| include CRC\|Error\|Drop` | `show interface <if>` + `show controllers <if>` | `show interfaces <if> extensive` | CRC(光模块老化)/尾丢弃/ACL静默deny |
| 路由震荡 | `display isis spf-log` / `display bgp routing-table dampened` | `show isis spf-log` / `show bgp dampened-paths` | `show isis spf log` / `show route damping suppressed` | 链路不稳定/dampening触发/计时器不当 |
| 静默丢包(无告警无日志) | `display fib statistics` + `display urpf statistics` | **`show cef drops` ★★★** + `show lpts pifib hardware police` | `show pfe statistics traffic` + `show firewall log` | FIB不一致/ACL静默deny/uRPF/标签MTU |
| EVPN MAC学习异常 | `display evpn mac routing-table` | `show l2vpn forwarding bridge-domain <bd> mac-address` | `show evpn arp-table` + `show evpn database` | MAC移动超限/duplicate-mac/BD绑定错误 |

---

## 11.6 Cisco IOS-XR 特有排障工具

```
# CEF丢包分析 ★★★ (静默丢包排障第一命令)
show cef drops
# 输出各类CEF丢包原因: adjacency/no-route/null/punt-unreachable/rpf

# LPTS分析 (控制面报文异常)
show lpts pifib hardware police            # 查看协议限速是否触发
show lpts pifib hardware entry brief       # 查看LPTS条目
show lpts ifib entries brief

# 进程CPU/内存
show processes cpu [ history ] [ location <loc> ]
show memory summary [ location <loc> ]
show watchdog memory-state

# 接口深度诊断
show controllers <if> [ stats | internal | phy ]
show controllers optics <if>               # 光模块诊断

# EEM触发告警
show event manager policy [ registered ]

# 在线诊断
show diagnostic result [ module <id> ] [ detail ]

# 一致性检查
show cef inconsistency [ records ]
show bgp convergence
show isis database detail | include Metric  # 检查链路开销
```

## 11.7 Juniper Junos 特有排障工具

```
# 转发平面分析
show pfe statistics traffic                # PFE丢包统计 ★
show pfe statistics ip                     # IP丢包分类
show route forwarding-table                # 转发表(硬件真相) ★

# 防火墙日志分析
show firewall log [ detail ]               # 防火墙丢包日志 ★
show firewall [ filter <name> ]            # 过滤器命中计数

# 接口深度诊断
show interfaces <if> extensive             # 完整接口统计(含各类错误) ★
show interfaces diagnostics optics <if>    # 光模块诊断

# 路由策略测试 ★
test policy <policy-name> <prefix>         # 在线测试路由是否被策略匹配
# 这是Juniper独有的强大排障工具，无需实际应用策略即可验证效果

# Commit确认(安全变更) ★
commit confirmed <minutes>                 # 自动回退的安全变更
# 如果<minutes>内未执行commit，配置自动回退

# 配置对比
show | compare                             # 候选配置与运行配置对比
show | compare rollback <n>                # 与历史版本对比

# 系统日志
show log messages [ last <n> ]
show log messages | match <pattern>

# 核心转储
show system core-dumps
request system core-dump ...

# 流量路径追踪
traceroute monitor <ip>                    # 持续traceroute监控
```

---

## 11.8 收敛等待参考

| 协议事件 | 典型收敛时间 | 观察指标 |
|:---|:---|:---|
| IGP SPF (IS-IS/OSPF) | 1-5s (+ flooding延迟大网络可达30s) | LSA/LSP数量稳定, SPF日志 |
| BGP路由更新 | 30-60s (scan interval) | UPDATE消息率回基线, 路由数稳定 |
| LDP标签分发 | 10-20s | 标签表数量稳定 (`display mpls lsp statistics` / `show mpls label table` / `show ldp statistics`) |
| RSVP-TE路径建立 | 5-30s | Path/Resv交换完成, 隧道Oper Up |
| BFD会话建立 | 3×检测间隔(如3×300ms=900ms) | 会话Up |
| FIB硬件编程 | 控制面收敛后1-10s | FIB与RIB一致 (`display fib` / `show cef` / `show route forwarding-table`) |

---

## 11.9 变更安全框架

```
FOR EACH 原子变更步骤:
  BASELINE → 记录基线(路由/标签/计数器/流量)
  EXECUTE  → 执行一步变更
  VERIFY   → 验证本步效果
  CONVERGE → 等待协议收敛(参照11.8)
  STEADY   → 确认稳态(≥5分钟观察)
  PROCEED  → 正常则继续 / 异常则FREEZE

IF 异常:
  FREEZE → 停止所有后续操作
  REVERSE → 回退最后一步(非全部)
  CONVERGE → 等待收敛
  CONFIRM → 对比基线确认恢复
  REASSESS → 分析原因后再决定

逐步升级干预手段:
  route-policy(精准绕行) → LP/MED(影响选路) → IGP cost(影响拓扑)
  → shutdown peer(邻居隔离) → shutdown interface(物理隔离)

Juniper特有安全变更工具:
  commit confirmed <minutes>    → 自动回退保护，最安全的变更方式
  show | compare               → 提交前对比变更
  rollback <n>                 → 回退到历史版本
```
