# Network Helper — 设计规格文档

> **项目名称**: nethelper
> **日期**: 2026-03-21
> **语言**: Go
> **目标用户**: 多厂商环境的网络工程师（运营商级别，涉及 MPLS/SR）

## 1. 项目概述

nethelper 是一个 CLI 工具，帮助网络工程师提升排障效率和设备配置理解效率。它通过监控终端日志文件，自动解析多厂商设备的命令回显，构建网络知识图谱，并通过 LLM 增强提供智能排障建议。

### 核心特性

- **多厂商支持** — 华为 VRP、思科 IOS/IOS-XE、华三 Comware、Juniper JUNOS
- **自动日志监控** — 后台守护进程监控日志目录，增量解析新内容
- **控制平面 + 数据平面** — RIB、FIB、LFIB 全覆盖，支持 MPLS/LDP/RSVP/SR
- **内存图引擎** — 拓扑分析、路径追踪、影响范围评估
- **LLM 热插拔** — 按能力路由到不同模型，无 LLM 可降级
- **向量语义搜索** — FTS5 + Embedding 双引擎混合搜索
- **可迁移记忆** — 单 SQLite 文件，复制即迁移

## 2. 整体架构

系统分为 5 个核心模块：

```
日志文件 → Watcher → Parser → Store（SQLite + 图）→ CLI 查询
                                  ↑
                              LLM / Embedding（增强层）
```

| 模块 | 职责 |
|------|------|
| **Watcher** | 后台守护进程，fsnotify 监控日志目录，检测新内容触发解析 |
| **Parser** | 识别厂商类型，将原始回显解析为结构化数据。每个厂商一个解析插件 |
| **Store** | SQLite 持久化 + 内存图引擎。启动时从 SQLite 加载图到内存 |
| **Knowledge** | 记忆管理：配置变更追踪、排障经验积累、命令手册存储 |
| **CLI** | 用户交互入口，查询拓扑、搜索历史、分析配置差异 |

## 3. 数据模型

### 3.1 设备与接口

**devices 设备表**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT PK | 设备唯一标识 |
| hostname | TEXT | 主机名 |
| vendor | TEXT | 厂商 (huawei/cisco/h3c/juniper) |
| model | TEXT | 设备型号 |
| os_version | TEXT | 系统版本 |
| mgmt_ip | TEXT | 管理 IP |
| router_id | TEXT | Router-ID |
| mpls_lsr_id | TEXT | LSR-ID |
| last_seen | TIMESTAMP | 最后活跃时间 |

**interfaces 接口表（统一物理 + 虚拟）**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT PK | 接口唯一标识 |
| device_id | TEXT FK | 所属设备 |
| name | TEXT | 接口名 (GE0/0/1, Tunnel1, Eth-Trunk0…) |
| type | TEXT | 接口类型（见枚举） |
| status | TEXT | 状态 (up/down/admin-down) |
| ip_address | TEXT | IP 地址 |
| mask | TEXT | 掩码 |
| vlan | INTEGER | VLAN |
| bandwidth | TEXT | 带宽 |
| description | TEXT | 描述 |
| parent_id | TEXT FK | 父接口（成员 → 聚合） |
| last_updated | TIMESTAMP | 最后更新时间 |

**接口类型枚举**: physical, loopback, vlanif, eth-trunk, tunnel-te, tunnel-sr, tunnel-gre, nve, null, sub-interface

### 3.2 控制平面

**rib_entries 路由表 (RIB)**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | |
| device_id | TEXT FK | |
| vrf | TEXT | VRF 名（默认 "default"） |
| prefix | TEXT | 目的前缀 |
| mask_len | INTEGER | 掩码长度 |
| protocol | TEXT | 协议 (ospf/bgp/isis/static/direct) |
| next_hop | TEXT | 下一跳 |
| outgoing_interface | TEXT | 出接口 |
| preference | INTEGER | 优先级 |
| metric | INTEGER | 度量值 |
| tag | INTEGER | 标签 |
| snapshot_id | INTEGER FK | 快照 ID |

**protocol_neighbors 协议邻居表**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | |
| device_id | TEXT FK | |
| protocol | TEXT | 协议 (ospf/bgp/isis/ldp/rsvp/lldp/cdp) |
| local_id | TEXT | 本端 Router-ID/LSR-ID |
| remote_id | TEXT | 对端 |
| local_interface | TEXT | 本端接口 |
| remote_address | TEXT | 对端地址 |
| state | TEXT | 状态 (full/established/up/down) |
| area_id | TEXT | OSPF/ISIS area |
| as_number | INTEGER | BGP AS 号 |
| uptime | TEXT | 邻居持续时间 |
| snapshot_id | INTEGER FK | 快照 ID |

**mpls_te_tunnels TE 隧道状态表**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | |
| device_id | TEXT FK | |
| tunnel_name | TEXT | 隧道名 |
| type | TEXT | 类型 (rsvp-te/sr-te/sr-policy) |
| destination | TEXT | 目的地址 |
| state | TEXT | 状态 (up/down) |
| signaled_bw | TEXT | 信令带宽 |
| explicit_path | TEXT | JSON: 显式路径跳列表 |
| actual_path | TEXT | JSON: 实际路径/ERO |
| binding_sid | INTEGER | Binding SID |
| snapshot_id | INTEGER FK | 快照 ID |

**sr_mappings SR 标签映射表**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | |
| device_id | TEXT FK | |
| prefix | TEXT | 前缀 |
| sid_index | INTEGER | SID 索引 |
| sid_label | INTEGER | SID 标签 (SRGB base + index) |
| algorithm | INTEGER | 算法 (0=SPF, 1=strict) |
| flags | TEXT | 标志位 |
| source | TEXT | 来源协议 (isis/ospf) |
| snapshot_id | INTEGER FK | 快照 ID |

### 3.3 数据平面

**fib_entries 转发表 (FIB)**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | |
| device_id | TEXT FK | |
| vrf | TEXT | VRF |
| prefix | TEXT | 目的前缀 |
| mask_len | INTEGER | 掩码长度 |
| next_hop | TEXT | 下一跳 |
| outgoing_interface | TEXT | 出接口 |
| label_action | TEXT | 标签动作 (push/swap/pop/none) |
| out_label | TEXT | 出标签（或标签栈 JSON） |
| tunnel_id | TEXT | 递归到哪个隧道 |
| snapshot_id | INTEGER FK | 快照 ID |

**lfib_entries 标签转发表 (LFIB)**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | |
| device_id | TEXT FK | |
| in_label | INTEGER | 入标签 |
| action | TEXT | 动作 (push/swap/pop/php) |
| out_label | TEXT | 出标签（或标签栈） |
| next_hop | TEXT | 下一跳 |
| outgoing_interface | TEXT | 出接口 |
| fec | TEXT | 绑定的 FEC (prefix/TE/VPN) |
| protocol | TEXT | 协议 (ldp/rsvp/sr/bgp-lu) |
| snapshot_id | INTEGER FK | 快照 ID |

### 3.4 知识/记忆表

**snapshots 快照表**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | |
| device_id | TEXT FK | |
| captured_at | TIMESTAMP | 采集时间 |
| source_file | TEXT | 来源日志文件 |
| commands | TEXT | JSON: 包含哪些命令的输出 |

**config_snapshots 配置快照表**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | |
| device_id | TEXT FK | |
| config_text | TEXT | 完整配置 |
| diff_from_prev | TEXT | 与上一版差异 |
| captured_at | TIMESTAMP | |
| source_file | TEXT | 来源日志文件 |

**troubleshoot_logs 排障记录表**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | |
| device_id | TEXT FK | 可为空 |
| symptom | TEXT | 问题现象 |
| commands_used | TEXT | 执行了哪些命令 |
| findings | TEXT | 发现了什么 |
| resolution | TEXT | 怎么解决的 |
| tags | TEXT | 标签 (ospf,neighbor,flap) |
| created_at | TIMESTAMP | |

**command_references 命令手册表**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | |
| vendor | TEXT | 厂商 |
| command | TEXT | 命令 |
| description | TEXT | 描述 |
| example_output | TEXT | 示例输出 |
| parse_hint | TEXT | 解析关键字段提示 |
| source_url | TEXT | 来源 URL |

**log_ingestions 日志摄入记录表**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | |
| file_path | TEXT | 文件路径 |
| file_hash | TEXT | 文件哈希（避免重复） |
| last_offset | INTEGER | 增量读取位置 |
| processed_at | TIMESTAMP | |

### 3.5 向量存储

使用 sqlite-vec 扩展，向量数据在同一个 .db 文件中。

**vec_embeddings** — sqlite-vec 虚拟表，存储向量

**embedding_meta 元数据映射表**

| 字段 | 类型 | 说明 |
|------|------|------|
| rowid | INTEGER PK | 对应 vec_embeddings 行 |
| source_type | TEXT | 来源类型 (troubleshoot/config/command/raw) |
| source_id | INTEGER | 来源记录 ID |
| chunk_text | TEXT | 文本块内容 |
| model_name | TEXT | 生成模型名 |
| created_at | TIMESTAMP | |

### 3.6 内存图模型

**节点类型**: Device, Interface, Subnet, VLAN, VRF, RoutingInstance, LSP, SRPolicy

**边类型**:
- HAS_INTERFACE — Device → Interface
- MEMBER_OF — Physical Interface → Eth-Trunk
- CONNECTS_TO — Interface ↔ Interface (L2)
- IN_SUBNET — Interface → Subnet
- IN_VRF — Interface → VRF
- PEER — Device ↔ Device (协议邻居，属性含协议类型)
- ROUTES_VIA — Device → Interface (路由下一跳)
- LSP_HOP — Device → Device (MPLS 路径)
- LABEL_BIND — LSP → Interface (标签绑定)
- TUNNELS_THROUGH — SRPolicy → LSP

**图查询能力**: 端到端路径追踪、隧道依赖分析、标签一致性校验、单点故障检测、环路检测

## 4. 多厂商解析器

### 4.1 解析流水线

```
原始日志 → ① Splitter（按提示符切分命令块）
         → ② Detector（识别厂商 + 命令类型）
         → ③ Parser（厂商特定解析器提取字段）
         → ④ 统一模型（写入 SQLite + 更新图）
```

### 4.2 Splitter — 提示符识别

| 厂商 | 提示符模式 |
|------|-----------|
| 华为 | `<hostname>` 或 `[hostname]` |
| 思科 | `hostname#` 或 `hostname(config)#` |
| 华三 | `<hostname>` 或 `[hostname]` |
| Juniper | `user@hostname>` 或 `user@hostname#` |

从提示符同时提取 hostname，用于关联设备。

### 4.3 VendorParser 接口

```go
type VendorParser interface {
    Vendor() string
    DetectPrompt(line string) (hostname string, ok bool)
    ClassifyCommand(cmd string) CommandType
    ParseOutput(cmdType CommandType, raw string) (ParseResult, error)
}
```

ParseResult 是厂商无关的统一输出结构，包含 Interfaces、RIBEntries、FIBEntries、LFIBEntries、Neighbors、Tunnels、SRMappings 等切片，以及 RawText 原文。

### 4.4 三层解析策略

| 层级 | 策略 | 说明 |
|------|------|------|
| L1 | 内置正则模板 | 覆盖最常见命令输出格式，开箱即用 |
| L2 | 用户自定义模板 | 遇到无法解析的输出时，用户定义提取规则存入 command_references |
| L2.5 | LLM 兜底解析 | L1/L2 都失败时调用 LLM 提取结构化数据，成功后自动生成正则模板 |
| L3 | 原文保留 + 全文索引 | 完全无法解析时存原文，建 FTS5 索引 |

关键原则：**原文永远保留**，解析失败不阻塞，增量解析。

## 5. CLI 命令设计

```
nethelper
├── watch        # 文件监控
│   ├── start       启动守护进程，监控指定目录
│   ├── stop        停止监控
│   ├── status      查看监控状态
│   └── ingest      手动导入日志文件
├── show         # 查询
│   ├── device      查看设备信息
│   ├── interface   查看接口信息
│   ├── route       查询路由 (RIB)
│   ├── fib         查询转发表 (FIB)
│   ├── label       查询标签表 (LFIB)
│   ├── neighbor    查看协议邻居
│   ├── tunnel      查看 TE/SR 隧道
│   └── topology    查看拓扑概览
├── trace        # 路径分析
│   ├── path        A→B 端到端路径追踪（控制+数据平面）
│   ├── label       追踪标签栈沿途变化
│   └── impact      链路/设备故障影响范围分析
├── diff         # 差异对比
│   ├── config      配置变更对比
│   ├── route       路由表变化对比
│   └── snapshot    任意两个快照对比
├── search       # 全文搜索
│   ├── config      搜索配置内容
│   ├── log         搜索排障记录
│   └── command     搜索命令手册
├── note         # 排障笔记
│   ├── add         记录排障经验
│   ├── extract     LLM 从日志自动提取经验
│   ├── list        列出笔记
│   └── search      搜索笔记
├── check        # 健康检查
│   ├── label       RIB↔LFIB↔FIB 标签一致性
│   ├── loop        环路检测
│   ├── spof        单点故障检测
│   └── stale       过期/不一致数据检测
├── diagnose     # LLM 排障建议
├── explain      # LLM 配置/输出解读
├── config       # 配置管理
│   └── llm         LLM/Embedding provider 配置
└── export       # 导出/迁移
    ├── db          导出完整 SQLite 数据库
    ├── topology    导出拓扑 (JSON/DOT/SVG)
    └── report      生成网络状态报告 (Markdown)
```

## 6. LLM 集成

### 6.1 Provider 抽象

```go
type LLMProvider interface {
    Name() string
    Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
    Supports(capability Capability) bool
}
```

能力类型: CapExtract（结构化提取）、CapAnalyze（排障分析）、CapExplain（配置解读）、CapParse（解析兜底）、CapEmbed（向量嵌入）

内置 Provider: openai、anthropic、ollama、openai-compat（兼容任何 OpenAI API 的服务）

### 6.2 按能力路由

配置文件中可为不同能力指定不同 provider/model，也可用 `--llm` 标志临时覆盖。

### 6.3 四大能力

| 能力 | 触发方式 | 工作流 |
|------|---------|--------|
| 排障日志 → 结构化经验 | `nethelper note extract <日志>` | LLM 提取 symptom/findings/resolution → 用户确认 → 存入 troubleshoot_logs |
| 相似问题推荐 + 排障建议 | `nethelper diagnose "描述"` | FTS5 + 向量搜索历史笔记 → 结合当前网络状态 → LLM 分析建议 |
| 配置/输出解读 | `--explain` 标志或 `nethelper explain` | 获取输出 + command_references 上下文 → LLM 中文解释 |
| 解析器兜底 (L2.5) | 自动（L1/L2 失败时） | LLM 提取 JSON → 成功后自动生成正则模板存入 command_references |

### 6.4 无 LLM 降级

| 能力 | 降级行为 |
|------|---------|
| 经验提取 | 退回手动 `note add` |
| 排障建议 | 退回 FTS5 关键词搜索 |
| 配置解读 | 展示原文 + command_references 字段说明 |
| 解析兜底 | L3 原文保留 + FTS5 索引 |

核心承诺：**没有 LLM，所有核心功能（监控、查询、diff、check）照常工作。LLM 是增强层，不是依赖层。**

## 7. Embedding 向量搜索

### 7.1 双搜索引擎

FTS5 关键词搜索 + sqlite-vec 向量语义搜索，用 RRF (Reciprocal Rank Fusion) 合并排名。

### 7.2 EmbeddingProvider 接口

```go
type EmbeddingProvider interface {
    Name() string
    Dimensions() int
    Embed(ctx context.Context, texts []string) ([][]float32, error)
}
```

内置 Provider: openai (text-embedding-3-small)、ollama (nomic-embed-text / bge-m3)、openai-compat

### 7.3 向量化策略

| 数据源 | 向量化内容 | 用途 |
|--------|-----------|------|
| troubleshoot_logs | symptom + findings + resolution 拼接 | 语义搜索排障经验 |
| config_snapshots | 按配置段落分块 | 语义搜索配置 |
| command_references | command + description | 查命令 |
| raw_outputs (L3) | 未解析的原始输出 | 兜底搜索 |

### 7.4 模型切换

切换 embedding provider 后自动检测维度变化，后台增量重建向量索引。重建期间 FTS5 兜底。

## 8. Watcher 守护进程

- **fsnotify** 监控指定目录，检测 Write/Create 事件
- **去抖动** — 500ms 窗口合并事件，避免读到半截行
- **增量读取** — log_ingestions.last_offset 记录文件读取位置
- **文件轮转** — 检测 inode 变化，处理日志文件被重建的情况
- **递归监控** — 支持子目录，按目录名/文件名推断设备归属
- **并发安全** — 多文件同时变化时，解析任务排队串行写入 SQLite

## 9. 记忆系统

四层记忆积累机制：

| 层级 | 触发方式 | 内容 |
|------|---------|------|
| 自动 | 日志摄入时 | 网络状态（设备/接口/邻居/路由/标签），快照保留历史 |
| 半自动 | 检测到新配置时 | 配置变更追踪，自动 diff，用户可加注释 |
| 手动 | `note add` | 排障笔记，结构化存储 |
| 可扩展 | 遇到新命令格式时 | 用户定义解析规则存入 command_references |

迁移方案：复制 `~/.nethelper/nethelper.db` + `config.yaml` 即完成全部数据迁移。

## 10. 项目结构

```
nethelper/
├── cmd/nethelper/main.go
├── internal/
│   ├── cli/          # CLI 命令定义 (Cobra)
│   ├── watcher/      # 文件监控守护进程
│   ├── parser/       # 多厂商解析器
│   │   ├── huawei/
│   │   ├── cisco/
│   │   ├── h3c/
│   │   └── juniper/
│   ├── store/        # SQLite + 内存图 + 搜索
│   ├── llm/          # LLM Provider 层
│   ├── embedding/    # Embedding Provider 层
│   ├── graph/        # 图分析算法
│   └── model/        # 统一数据模型
├── config/config.go
├── go.mod
└── go.sum
```

### 核心依赖

| 库 | 用途 |
|----|------|
| cobra | CLI 框架 |
| fsnotify | 文件系统监控 |
| modernc.org/sqlite | 纯 Go SQLite（无 CGo） |
| sqlite-vec | 向量搜索扩展 |
| lipgloss / bubbletea | 终端 UI 美化（可选） |
| yaml.v3 | 配置文件解析 |
| slog | 结构化日志（标准库） |

### 数据目录

```
~/.nethelper/
├── config.yaml       # 配置文件
├── nethelper.db      # SQLite 数据库（全部数据）
├── watcher.pid       # 守护进程 PID
└── logs/nethelper.log
```
