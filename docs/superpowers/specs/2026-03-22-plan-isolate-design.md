# Design: `nethelper plan isolate` — 设备隔离变更方案生成

**日期:** 2026-03-22
**状态:** Draft

## 问题

网络工程师需要对核心设备执行维护隔离（如硬件更换、版本升级），目前 nethelper 能分析拓扑影响（`trace impact`、`check spof`），但无法生成结构化的变更方案——包含预检查命令、隔离步骤、验证命令和回退方案。

工程师当前需要手动：
1. 逐台查看配置，梳理互联关系
2. 手写变更方案文档
3. 人工组织预检查和变更后检查命令

这个过程耗时、易遗漏，且难以标准化。

## 目标

新增 `nethelper plan isolate <device>` 命令，基于已采集的配置和拓扑数据，自动生成五阶段分阶段隔离变更方案。

## 用户场景

以本次实际场景为例：
- **目标设备:** LC（CD-GX-0201-G17-H12516AF-LC-01，H3C Comware 7）
- **互联设备:** LA（CD-GX-0201-D04-H6800QT-LA-01，Huawei）、QCDR（CD-GX-0201-H10-HW12816-QCDR-01，Huawei VRP）
- **拓扑角色:** LC 是 SPOF，连接 LA 和 QCDR，移除后影响 6 台设备
- **隔离类型:** 分阶段隔离（先协议级排干流量，再接口级断开）

## 设计

### 命令接口

两步交互模式：

```bash
# 第一步：生成方案 + 采集命令清单
nethelper plan isolate cd-gx-0201-g17-h12516af-lc-01

# 第二步：采集完数据后，生成含预检查结果的完整方案
nethelper plan isolate cd-gx-0201-g17-h12516af-lc-01 --check
```

可选参数：
- `--format markdown|text` — 输出格式（默认 text）
- `--check` — 启用预检查，要求已采集动态数据
- `--output <file>` — 输出到文件

### 五阶段变更流程

#### 阶段 0: 方案规划（Plan）

纯静态分析，基于配置和接口数据推断互联关系，输出需要人工在设备上执行的采集命令清单。

**输入:** SQLite 中的配置快照、接口信息
**输出:**
- 互联关系表（本端设备/接口 ↔ 对端设备/接口）
- 推断出的协议关系（OSPF/BGP/LDP/MPLS）
- 需要人工采集的命令清单（按设备分组）
- 影响评估（哪些设备受影响，是否为 SPOF）

示例输出：
```
=== 设备隔离方案 — 阶段0: 规划 ===
目标设备: CD-GX-0201-G17-H12516AF-LC-01 (H3C Comware 7)
互联设备: 2 台
影响评估: SPOF — 移除后 6 台设备受影响

互联关系:
  LC-01 FortyGigE2/0/27  <-->  LA-01  FG1/0/50   (来源: description)
  LC-01 FortyGigE2/0/35  <-->  QCDR-01 ...        (来源: subnet)

推断协议: OSPF (配置中发现 ospf 进程), BGP (发现 bgp peer 配置)

>>> 请在以下设备上执行命令并灌入 nethelper: <<<

[CD-GX-0201-G17-H12516AF-LC-01]
  display ospf peer brief
  display bgp peer
  display mpls ldp session
  display ip routing-table statistics
  display interface brief

[CD-GX-0201-D04-H6800QT-LA-01]
  display ospf peer brief
  display bgp peer
  display interface FortyGigE1/0/50

[CD-GX-0201-H10-HW12816-QCDR-01]
  display ospf peer brief
  display bgp peer
  ...
```

#### 阶段 1: 预检查（Pre-check）

在工程师采集完数据（`watch ingest`）后运行，基于完整数据验证基线状态。

**输入:** 新采集的邻居、路由数据
**输出:**
- 基线状态汇总（邻居数、路由数、接口状态）
- 是否可以安全隔离的判断
- 已知风险告警

#### 阶段 2: 协议级隔离（排干流量）

生成排干流量的配置命令，按协议分组。

**命令生成逻辑:**
- OSPF: 将互联接口 cost 调至 65535
- BGP: shutdown 指向目标设备的 peer
- LDP: 关闭 LDP session（如适用）
- 每组命令后附带中间验证命令

#### 阶段 3: 接口级隔离

生成 shutdown 互联接口的命令。

#### 阶段 4: 变更后检查（Post-check）

在邻居设备上执行检查，确认目标设备已脱离网络：
- 邻居表中不再出现目标设备
- 路由已收敛到备用路径
- 与阶段1基线对比

### 互联关系推断引擎

核心组件，负责从静态数据推断设备间的连接关系。

**推断策略（按优先级）：**

1. **已知设备名匹配**（接口 description）
   - 从数据库加载所有已知设备 hostname
   - 在每个接口的 description 中做子串匹配（忽略大小写）
   - 匹配上则认为该接口连接到对应设备
   - 不硬编码 description 格式，适应各种命名风格

2. **子网匹配**（接口 IP）
   - 两个设备的接口 IP 在同一 /30 或 /31 → 互联
   - 复用现有 graph 包的子网逻辑

3. **配置推断**（配置快照）
   - 解析 OSPF 配置段（`router ospf` / `ospf 1`），提取 network 语句覆盖的接口
   - 解析 BGP 配置段，提取 peer IP，与已知接口 IP 交叉匹配
   - 解析 MPLS/LDP 配置段

**数据结构:**

```go
// internal/plan/link.go
type Link struct {
    LocalDevice    string   // 本端设备 ID
    LocalInterface string   // 本端接口名
    LocalIP        string   // 本端 IP
    PeerDevice     string   // 对端设备 ID
    PeerInterface  string   // 对端接口名（可能为空）
    PeerIP         string   // 对端 IP（可能为空）
    Protocols      []string // 推断出的协议 ["ospf", "bgp", "ldp"]
    Source         string   // 推断来源 "description" / "subnet" / "config"
}
```

### 命令生成器

按厂商生成具体 CLI 命令。

```go
// internal/plan/command.go
type CommandGenerator interface {
    PreCheckCommands(links []Link) []DeviceCommand
    ProtocolIsolateCommands(links []Link) []DeviceCommand
    InterfaceIsolateCommands(links []Link) []DeviceCommand
    PostCheckCommands(links []Link) []DeviceCommand
    CollectionCommands(links []Link) []DeviceCommand  // 阶段0的采集命令
}
```

初始实现覆盖 Huawei VRP 和 H3C Comware（本次场景涉及的两个厂商）。Cisco IOS 和 Juniper 作为后续扩展。

### 代码结构

```
internal/plan/
├── link.go          // Link 结构 + DiscoverLinks() 互联推断
├── isolate.go       // GenerateIsolationPlan() 方案生成主逻辑
├── command.go       // CommandGenerator 接口
├── command_huawei.go // Huawei VRP 命令模板
├── command_h3c.go   // H3C Comware 命令模板
├── render.go        // 渲染为 text/markdown
└── plan.go          // Plan/Phase/Step 数据模型

internal/cli/
└── plan.go          // CLI 入口: plan isolate 子命令
```

### 错误处理

- 目标设备不存在 → 错误退出，提示 `show device`
- 无互联关系推断出 → 警告，建议补充数据
- `--check` 但缺少动态数据 → 提示需要先执行阶段0的采集命令
- 不支持的厂商 → 跳过该设备的命令生成，输出警告

### 测试策略

- **单元测试:** 互联推断引擎（mock 数据库数据，验证 Link 输出）
- **单元测试:** 命令生成器（给定 Link 列表，验证输出命令正确性）
- **集成测试:** 用 LC/LA/QCDR 真实数据端到端运行 `plan isolate`
- **网络工程师审查:** 输出方案的专业性由网络工程师角色验证

## 三角色协作流程

本功能通过模拟三角色迭代开发：

1. **网络工程师** — 运行现有命令发现功能缺口，提出需求，验收变更方案的专业性
2. **开发** — 实现 `internal/plan/` 包和 CLI 命令
3. **测试** — 编写测试用例，用真实数据验证输出

迭代流程：
- Round 1: 网络工程师出需求 → 开发实现 → 测试验证
- Round 2: 测试反馈问题 → 开发修复 → 网络工程师复审
- Round 3: 三方对齐，网络工程师使用工具生成最终 LC 隔离方案

## 后续扩展方向

- `plan migrate` — 设备迁移方案
- `plan upgrade` — 版本升级方案
- Plan 引擎持久化（存数据库，状态跟踪）
- LLM 辅助生成更智能的方案建议
- 回退方案自动生成
