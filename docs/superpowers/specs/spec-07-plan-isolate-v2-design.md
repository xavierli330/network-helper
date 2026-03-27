# Design: `plan isolate` v2 — 多维度互联发现 + 配置感知命令生成

**日期:** 2026-03-22
**状态:** Draft

## 问题

`plan isolate` v1 的命令生成器使用泛泛模板（"如果有 OSPF 就检查 OSPF 邻居"），不看设备实际配置。结果生成的方案对网络工程师没有可操作性——检查了不存在的协议，漏掉了实际运行的 BGP，命令粒度不够。

## 目标

重做 `plan isolate` 的两个核心组件：
1. **互联发现引擎** — 从设备实际数据（接口、BGP peers、配置、VRF）构建多维度互联视图
2. **命令生成器** — 消费完整互联视图，生成精确到每个 BGP peer、每个接口的具体命令，按 peer group 分批，每批后有检查点

## 设计

### 多维度互联发现

替换当前 `DiscoverLinks()` 的扁平 `[]Link` 输出为结构化的 `DeviceTopology`：

```go
// DeviceTopology 是一台设备的完整互联视图，整合四个层次的数据。
type DeviceTopology struct {
    DeviceID   string
    Hostname   string
    Vendor     string
    LocalAS    int        // BGP local AS (0 = no BGP)
    Protocols  []string   // 设备实际运行的协议 ["bgp"] / ["isis","ldp","bgp"] / []

    PeerGroups []PeerGroup   // BGP peer groups，从 bgp_peers 表聚合
    LAGs       []LAGBundle   // LAG 上联，从 interfaces 提取
    PhysicalLinks []PhysicalLink // 所有物理直连，从 interfaces 提取
    StaticRoutes  []StaticRoute  // 静态路由，从 config 提取
    VRFs       []VRFInfo     // VPN 实例，从 vrf_instances 表
}

type PeerGroup struct {
    Name     string          // "LA1", "QCDR", "XGWL"
    Type     string          // "external" / "internal"
    Role     string          // "downlink" / "uplink" / "management" (推断)
    Peers    []BGPPeerDetail
}

type BGPPeerDetail struct {
    PeerIP      string  // BGP peer address
    RemoteAS    int
    Description string  // 对端设备名（from config）
    Interface   string  // 关联物理接口名（通过 peer IP 和接口 IP 子网匹配）
}

type LAGBundle struct {
    Name        string   // "Route-Aggregation1"
    IP          string
    Mask        string
    Description string   // 对端设备名
    Members     []string // 成员物理口名
}

type PhysicalLink struct {
    Interface   string
    IP          string
    Mask        string
    Description string   // 对端设备名+对端接口
    PeerGroup   string   // 关联的 BGP peer group（如果 IP 匹配）
}

type StaticRoute struct {
    Prefix    string
    NextHop   string
    Interface string
    VRF       string
}

type VRFInfo struct {
    Name   string
    RD     string
}
```

**构建流程 `BuildTopology(db, deviceID) (DeviceTopology, error)`：**

1. 从 `devices` 表获取设备基本信息
2. 从 `bgp_peers` 表获取所有 BGP peers（最新 snapshot），聚合为 PeerGroups
3. 从 `interfaces` 表获取所有接口，识别 LAG（type=eth-trunk）和物理口
4. 交叉匹配：BGP peer IP ↔ 接口 IP 子网，建立 peer → interface 映射
5. 从 `config_snapshots` 提取静态路由（`ip route-static` 行）
6. 从 `vrf_instances` 表获取 VRF
7. 推断协议列表：bgp_peers 非空 → "bgp"；config 含 ospf → "ospf" 等
8. 推断 peer group 角色：
   - group type = internal → "management"
   - group name 含 "SDN"/"controller" → "management"
   - group 关联的接口是 LAG → "uplink"
   - 其他 external → "downlink"

### 配置感知的命令生成器

替换当前的 `CommandGenerator` 接口。新生成器直接消费 `DeviceTopology`：

```go
func GenerateIsolationPlanV2(topo DeviceTopology) Plan
```

**生成逻辑：**

#### 阶段0: 采集
基于 `topo.Protocols` 生成：
- 有 BGP → `display bgp peer`, `display bgp routing-table statistics`
- 有 OSPF → `display ospf peer brief` (Huawei) / `display ospf peer` (H3C)
- 有 ISIS → `display isis peer`
- 有 LDP → `display mpls ldp session`
- 通用: `display interface brief`, `display ip routing-table statistics`
- LAG 非空 → `display link-aggregation verbose`
- 管理 VRF → `display ip routing-table vpn-instance mgt_vrf`

#### 阶段1: 预检查
按 peer group 生成具体检查项：
```
✓ BGP peer group LA1: 2 peers 应全部 Established
✓ BGP peer group LA2~448: 220 peers 应全部 Established
...
✓ Route-Aggregation1/2: 状态应为 Up
```

#### 阶段2: 协议隔离（按 peer group 分 step）
排序规则：先 downlink → 再 uplink → 最后 management

每个 step：
1. 进入 BGP 视图：`system-view` → `bgp <local-AS>`
2. 逐个 peer ignore：`peer <IP> ignore  # <description>`
3. 退出：`quit` → `return`
4. **检查点**：`display bgp peer | include <peer-IP>` 确认 Idle

对于 OSPF/ISIS 场景（其他设备）：
- OSPF: `interface <X>` → `ospf cost 65535` → 检查 `display ospf peer brief`
- ISIS: `isis cost <max>` 或 `isis circuit-level` 调整

#### 阶段3: 接口隔离
- 先关 LAG 上联（Role=uplink 的接口）
- 再关物理下联
- 每组后检查 `display interface brief | include <interface>`

#### 阶段4: 变更后检查
基于 topo 中已知的对端设备（从 description 或 bgp_peers 获取），生成在对端设备上的检查命令。

#### 阶段5: 回退
逆序恢复：先恢复协议（undo peer ignore），再恢复接口（undo shutdown）。BGP 回退命令同样按 peer group 分 step，但顺序相反（先 management → uplink → downlink）。

### 命令语法适配

生成器内部根据 `topo.Vendor` 选择命令关键字：

| 操作 | Huawei VRP | H3C Comware | Cisco IOS-XR | Juniper |
|------|-----------|-------------|-------------|---------|
| BGP 抑制 | `peer X ignore` | `peer X ignore` | `neighbor X shutdown` | `deactivate protocols bgp group X` |
| OSPF cost | `ospf cost 65535` | `ospf cost 65535` | `router ospf X / cost 65535` | `set protocols ospf area X interface Y metric 65535` |
| 接口关闭 | `shutdown` | `shutdown` | `shutdown` | `disable` |
| BGP 进入 | `bgp <AS>` | `bgp <AS>` | `router bgp <AS>` | `edit protocols bgp` |

初始实现覆盖 Huawei VRP 和 H3C Comware。

### 代码结构

```
internal/plan/
├── plan.go              // Plan/Phase/DeviceCommand 数据模型（保留）
├── topology.go          // DeviceTopology 结构 + BuildTopology()
├── topology_test.go     // 互联发现测试
├── isolate_v2.go        // GenerateIsolationPlanV2()
├── isolate_v2_test.go   // 方案生成测试
├── commands_bgp.go      // BGP 隔离命令生成（按 vendor）
├── commands_iface.go    // 接口隔离命令生成
├── render.go            // 渲染器（保留，可能需微调适配新 Phase 结构）

# 保留但不再是主路径:
├── link.go              // 旧 DiscoverLinks（保留兼容）
├── command.go           // 旧 CommandGenerator 接口（保留兼容）
├── command_huawei.go    // 旧 Huawei 生成器（保留兼容）
├── command_h3c.go       // 旧 H3C 生成器（保留兼容）
├── isolate.go           // 旧 BuildIsolationPlan（保留兼容）
```

CLI 入口 `internal/cli/plan.go` 修改为调用 `BuildTopology` → `GenerateIsolationPlanV2`。

### 测试策略

- **BuildTopology 单元测试**：mock DB 数据，验证 PeerGroup 聚合、peer→interface 映射、角色推断
- **GenerateIsolationPlanV2 单元测试**：给定固定 DeviceTopology，验证每个 step 的命令正确性
- **集成测试**：用 LC 的真实 DB 数据端到端运行，验证输出包含全部 234 个 peer
- **黄金文件测试**：保存 LC 的预期输出，回归对比
