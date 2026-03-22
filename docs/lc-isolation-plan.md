# 设备隔离变更方案

| 字段 | 值 |
|------|----|
| 目标设备 | CD-GX-0201-G17-H12516AF-LC-01 (huawei) |
| 生成时间 | 2026-03-22 13:25 |
| 互联设备 | 1 台 |
| 影响评估 | ⚠️ SPOF — 移除后 6 台设备受影响 |

## 互联关系

| 本端设备 | 本端接口 | 对端设备 | 对端接口 | 来源 | 协议 |
|----------|----------|----------|----------|------|------|
| cd-gx-0201-g17-h12516af-lc-01 | FortyGigE2/0/27 | cd-gx-0201-d04-h6800qt-la-01 |  | description |  |

## 阶段0: 方案规划

收集目标设备及所有互联对端的当前运行状态，作为变更前基线数据

**注意事项:**

- 确认操作窗口，通知相关团队
- 记录当前设备状态作为变更基线
- ⚠️ 该设备为单点故障节点 (SPOF)，移除后下游设备将失去连通性，请谨慎评估影响范围

### [cd-gx-0201-g17-h12516af-lc-01] Collect baseline state from target device

```
display interface brief
display ip routing-table statistics
display current-configuration
display lldp neighbor brief
display version
```

### [cd-gx-0201-d04-h6800qt-la-01] Collect baseline state from peer device cd-gx-0201-d04-h6800qt-la-01

```
display interface brief
display ip routing-table statistics
```

## 阶段1: 预检查

在执行变更前，确认目标设备所有协议邻居状态正常，不存在已有故障

**注意事项:**

- 确认所有 OSPF 邻居状态为 Full
- 确认所有 BGP 对等体状态为 Established
- 确认所有互联接口状态为 Up
- 如存在异常，需先处理现有故障再继续变更

### [cd-gx-0201-g17-h12516af-lc-01] Pre-check: verify current state on target device

```
display interface brief
display ip routing-table statistics
```

## 阶段2: 协议级隔离

通过调整路由协议参数（提高 OSPF cost、抑制 BGP 对等体、禁用 LDP），使流量在接口下线前平滑切走

**注意事项:**

- 执行后等待至少 60 秒，确保路由收敛完成
- 观察对端设备是否已重新选路，确认流量已切走后再进入下一阶段
- ⚠️ 未检测到协议信息（OSPF/BGP/LDP），阶段2将不会执行流量排干。阶段3的接口 shutdown 将是硬切！
- 建议: 先采集设备配置（display current-configuration）以获取协议信息

### [cd-gx-0201-g17-h12516af-lc-01] Protocol isolation: raise OSPF cost, suppress BGP peers, disable LDP

```
system-view
return
```

### [cd-gx-0201-g17-h12516af-lc-01] Verify protocol isolation took effect

```
display ospf peer brief
display bgp peer
```

## 阶段3: 接口级隔离

关闭目标设备所有互联接口，完成物理/逻辑层隔离

**注意事项:**

- 接口 shutdown 后确认对端接口状态变为 Down

### [cd-gx-0201-g17-h12516af-lc-01] Interface isolation: shut down all links to target device

```
system-view
interface FortyGigE2/0/27
shutdown
quit
return
```

## 阶段4: 变更后检查

在所有对端设备上确认邻居已消失，路由表已收敛，业务流量正常

**注意事项:**

- 确认对端设备 OSPF/BGP 邻居列表中目标设备已消失
- 确认对端路由表中无黑洞路由
- 联系业务团队确认流量无异常

### [cd-gx-0201-d04-h6800qt-la-01] Post-check: verify peer cd-gx-0201-d04-h6800qt-la-01 after isolation

```
display interface brief
display ip routing-table statistics
```

## 阶段5: 回退方案

如变更后出现异常，按以下步骤恢复：先重新上线接口，再恢复协议配置

**注意事项:**

- 回退后等待至少 60 秒，确保协议邻居重新建立、路由收敛完成
- 重新执行预检查命令，确认所有邻居状态恢复正常

### [cd-gx-0201-g17-h12516af-lc-01] Rollback: restore protocol settings

```
system-view
return
```

### [cd-gx-0201-g17-h12516af-lc-01] Rollback: re-enable interfaces

```
system-view
interface FortyGigE2/0/27
undo shutdown
quit
return
```

