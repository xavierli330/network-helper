# Design: OSPF/ISIS/LDP 隔离命令生成

**日期:** 2026-03-22
**状态:** Approved

## 问题

当前 `plan isolate` v2 只能对 BGP 设备生成协议隔离命令。对于运行 ISIS/LDP/OSPF 的设备（如 QCDR），阶段2只输出"未检测到协议"警告。

## 目标

在 `BuildTopology` 和 `GenerateIsolationPlanV2` 中增加 ISIS、OSPF、LDP 的隔离/回退命令生成。

## 设计

### BuildTopology 增强

当前 `BuildTopology` 通过配置关键字检测协议（`topo.Protocols`），但不提取协议的接口级细节。需要增加：

**ISIS 信息提取：**
- 从配置中提取 `isis <process-id>`、`is-level`、`network-entity`
- 识别哪些接口启用了 ISIS（`isis enable <process>`）

**OSPF 信息提取：**
- 从配置中提取 `ospf <process-id> router-id <id>`
- 识别哪些接口启用了 OSPF（`ospf enable <process> area <area>`）
- 或从 `network` 语句推断

**LDP 信息提取：**
- 检测全局 `mpls ldp` 配置
- 识别哪些接口启用了 `mpls ldp`

新增到 `DeviceTopology`：

```go
type IGPInfo struct {
    Protocol  string   // "isis" / "ospf"
    ProcessID string   // "1" / "100"
    Interfaces []string // 启用了该协议的接口名
}

type DeviceTopology struct {
    // ... existing fields ...
    IGPs []IGPInfo  // ISIS/OSPF 实例
    HasLDP bool     // 是否运行 LDP
    LDPInterfaces []string // 启用 LDP 的接口
}
```

### 命令生成

新增 `internal/plan/commands_igp.go`：

**ISIS 隔离（华为/H3C）：**
```
system-view
isis <process-id>
  set-overload        # 通告 overload bit，其他设备绕行
quit
return
```
检查点：`display isis peer` 确认邻居仍在但 overload 生效

**ISIS 回退：**
```
system-view
isis <process-id>
  undo set-overload
quit
return
```

**OSPF 隔离（华为/H3C）：**
```
system-view
ospf <process-id>
  stub-router         # 通告 max metric，让其他设备绕行
quit
return
```

**LDP 隔离：**
在每个 LDP 接口上 `undo mpls ldp`。

**隔离顺序更新：**
```
Phase 2 顺序:
1. ISIS set-overload（如果有 ISIS）
2. OSPF stub-router（如果有 OSPF）
3. 等待 IGP 收敛（60s）
4. LDP disable per-interface（如果有 LDP）
5. BGP per-peer-group ignore（已有）
```

### 代码结构

| 文件 | 改动 |
|------|------|
| `internal/plan/topology.go` | 添加 IGPInfo/HasLDP 字段 + 从配置提取 |
| `internal/plan/commands_igp.go` | 新建：ISIS/OSPF/LDP 隔离/检查/回退命令 |
| `internal/plan/commands_igp_test.go` | 新建：测试 |
| `internal/plan/isolate_v2.go` | Phase 2 加入 IGP 步骤（在 BGP 之前） |
