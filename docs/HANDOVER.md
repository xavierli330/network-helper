# nethelper 项目完整交接文档（详细版）

> **文档目的**：为后续维护者（包括能力相对低的 AI 模型）提供完整的项目上下文，确保任何人都能在不阅读全部源代码的情况下理解、维护和迭代本项目。
>
> **文档级别**：逐文件级别的详细交接，包含每个函数的职责、每张表的 DDL、每个 API 端点的参数。
>
> **最后更新**：2026-03-31
> **代码规模**：26,000+ 行 Go 代码，252 个 .go 文件，18 个内部模块，30+ 张数据库表

---

## 目录

1. [项目概述与痛点](#1-项目概述与痛点)
2. [架构设计全景](#2-架构设计全景)
3. [模块逐文件详解](#3-模块逐文件详解)
   - 3.1 [入口与 CLI](#31-入口与-cli)
   - 3.2 [Parser 解析层](#32-parser-解析层)
   - 3.3 [Pipeline DSL 引擎](#33-pipeline-dsl-引擎)
   - 3.4 [厂商解析器](#34-厂商解析器)
   - 3.5 [Store 存储层](#35-store-存储层)
   - 3.6 [Studio Web UI](#36-studio-web-ui)
   - 3.7 [MCP Server](#37-mcp-server)
   - 3.8 [Agent Loop](#38-agent-loop)
   - 3.9 [Discovery Engine](#39-discovery-engine)
   - 3.10 [其他模块](#310-其他模块)
4. [数据库完整 Schema](#4-数据库完整-schema)
5. [Rule Studio API 参考](#5-rule-studio-api-参考)
6. [DSL 引擎完整参考](#6-dsl-引擎完整参考)
7. [数据流追踪](#7-数据流追踪)
8. [待优化待落实功能](#8-待优化待落实功能)
9. [运维及部署手册](#9-运维及部署手册)
10. [常见 FAQ](#10-常见-faq)

---

## 1. 项目概述与痛点

### 1.1 一句话定位

**nethelper** 是一个面向运营商/ISP 网络工程师的智能运维工具，核心功能是：从终端日志文件中自动解析多厂商设备输出 → 存入 SQLite → 提供拓扑分析/变更方案/AI 排障/MCP Server/IM Bot 等能力。

### 1.2 解决的 7 大痛点

| # | 痛点 | 传统方式 | nethelper 方案 |
|---|------|---------|---------------|
| 1 | **日志散落** | SecureCRT/iTerm2 日志文件散落各处 | 自动解析 → SQLite 结构化存储 |
| 2 | **多厂商差异** | 华为 `display`/思科 `show`/Juniper `show` 各异 | 统一 `VendorParser` 接口，4 厂商 |
| 3 | **拓扑理解困难** | 人肉查邻居、画拓扑 | 内存图引擎自动推断，BFS/SPOF |
| 4 | **变更方案手写** | 每次变更手写，遗漏 BGP peer | 自动发现互联，分步隔离 |
| 5 | **经验无法积累** | 在聊天记录/脑子里 | 结构化笔记 + FTS5 + 向量语义搜索 |
| 6 | **LLM 缺乏数据** | 给 ChatGPT 凭空猜测 | Agent 先查 DB 真实数据再分析 |
| 7 | **团队协作低效** | 值班人手动查设备 | IM Bot 直接对话查数据 |

### 1.3 目标用户与协议栈

- **用户**：运营商/ISP 网络工程师
- **设备**：华为 NE40E、H3C H12508、Cisco ASR9000、Juniper MX960
- **协议**：OSPF、IS-IS、BGP、MPLS、LDP、RSVP、SR-MPLS、VPN（L3VPN）

### 1.4 四种使用方式

| 方式 | 命令 | 适用场景 | 需要 LLM |
|------|------|---------|----------|
| **CLI** | `nethelper show device` | 工程师直接查询 | 否 |
| **Agent Chat** | `nethelper agent chat` | 多轮排障对话 | 是 |
| **MCP Server** | `nethelper mcp serve` | Claude Code 调用 | 部分 |
| **IM Bot** | `nethelper channel start --feishu` | 飞书群里 @bot | 是 |

### 1.5 不配置 LLM 也能用的功能

日志解析、查询（show）、搜索（search）、配置 diff、拓扑分析（trace/check）、变更方案（plan）、数据导出（export）、Rule Studio 的手动编辑和测试。

---

## 2. 架构设计全景

### 2.1 目录结构与代码量

```
cmd/nethelper/main.go          12 行，极简入口
  └→ internal/cli/root.go      Cobra 命令 + PersistentPreRunE 依赖注入

internal/                       252 个 .go 文件
├── cli/           20+ Cobra 子命令
├── config/        YAML 配置加载
├── model/         统一数据模型（struct 定义）
├── parser/        多厂商解析器 + Pipeline + DSL 引擎
│   ├── huawei/    23 文件（含 7 个 Rule Studio 生成的）
│   ├── cisco/     7 文件
│   ├── h3c/       15 文件（含 4 个生成的）
│   ├── juniper/   7 文件
│   └── engine/    4 文件（Pipeline DSL 解释器 1284 行）
├── store/         33 文件（22 源码 + 11 测试）
├── studio/        6 .go + 3 静态文件（handlers.go 2609 行）
├── mcp/           7 文件（20 个 MCP tools）
├── agent/         6 文件（Agent Loop + REPL + 心跳）
├── discovery/     LLM 驱动规则生成
├── codegen/       Go 代码生成器
├── graph/         内存图引擎
├── plan/          变更方案引擎
├── llm/           LLM Provider + Embedding
├── memory/        向量记忆 + 知识库
├── channel/       IM 适配（飞书/Discord/Telegram/微信/QQ）
├── diff/          配置 diff
├── errors/        标准错误
└── watcher/       fsnotify 文件监控

desktop/           Wails v2 桌面应用（4 文件）
test/              集成测试（3 文件 + 4 厂商 testdata）
docs/              57 个 .md 文档
```

### 2.2 启动流程（关键！）

`cmd/nethelper/main.go` → `internal/cli/root.go` 的 `PersistentPreRunE`：

```
1. config.LoadFrom(cfgFile)           加载 ~/.nethelper/config.yaml
2. store.Open(cfg.DBPath)             打开 SQLite（WAL + FTS5 + 64MB 缓存）
3. registry.Register(huawei.New())    注册 4 个厂商解析器（顺序重要！）
   registry.Register(cisco.New())     先注册的提示符匹配优先级更高
   registry.Register(h3c.New())       华为在 H3C 之前（共享 <hostname>）
   registry.Register(juniper.New())
4. parser.NewPipeline(db, registry)   创建解析管道
5. llm.BuildFromConfig(cfg.LLM)      构建 LLM 路由器
```

4 个全局变量 `cfg`, `db`, `pipeline`, `llmRouter` 在所有 CLI 命令中共享。

### 2.3 核心数据管道

```
终端日志文件
  → parser.Split()              按提示符行拆分为 []CommandBlock（splitter.go）
  → detectRawConfig()           无提示符时检测裸配置（pipeline.go）
  → processBlocks()             核心处理流程（pipeline.go）
      ├── ClassifyCommand()     每个 block 分类命令类型
      ├── reclassifyH3C()       H3C/华为重分类（检测 "version 7." 签名）
      ├── 按 hostname 分组       每个设备一个 snapshot
      └── 逐 block 处理：
          ├── 大表(RIB/FIB/LFIB) → scratch pad（FIFO 200 条）
          ├── CmdUnknown → Collector → unknown_outputs 表
          └── 其他 → ParseOutput() → storeResult()
              ├── Interfaces → interfaces 表（UPSERT 时间戳保护）
              ├── RIB/FIB/LFIB → 各自表
              ├── Neighbors → protocol_neighbors（去重唯一索引）
              ├── Config → config_snapshots（SHA-256 哈希去重）
              │   ├── ExtractInterfacesFromConfig() → interfaces
              │   ├── ExtractBGPPeers() → bgp_peers
              │   ├── ExtractVRFInstances() → vrf_instances
              │   ├── ExtractRoutePolicies() → route_policies + route_policy_nodes
              │   └── CheckCoverage() → coverage_checks
              └── Generated(DSL) → scratch_entries（JSON rows）
```

### 2.4 关键接口（4 个核心接口）

```go
// 1. 厂商解析器 — 每个厂商实现一次
type VendorParser interface {
    Vendor() string
    DetectPrompt(line string) (hostname string, ok bool)
    ClassifyCommand(cmd string) model.CommandType
    ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error)
    SupportedCmdTypes() []model.CommandType          // Rule Studio 需要
    FieldSchema(cmdType model.CommandType) []FieldDef // Rule Studio 需要
}

// 2. LLM Provider — OpenAI 和 Anthropic 两种协议
type Provider interface {
    Name() string
    Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
    Supports(cap Capability) bool
}

// 3. IM 通道 — 每个平台实现一次
type Channel interface {
    Name() string
    Start(ctx context.Context, handler MessageHandler) error
    Stop() error
    SendText(chatID string, text string) error
}

// 4. 知识源 — 可插拔
type KnowledgeSource interface {
    Name() string
    Search(ctx context.Context, query string, topK int) ([]SearchResult, error)
}
```

### 2.5 CommandType 枚举

```go
CmdRIB       = "rib"           // display ip routing-table
CmdFIB       = "fib"           // display fib
CmdLFIB      = "lfib"          // display mpls lsp / forwarding
CmdInterface = "interface"     // display interface [brief]
CmdNeighbor  = "neighbor"      // display ospf/bgp/isis/ldp peer
CmdTunnel    = "tunnel"        // display mpls te tunnel
CmdSRMapping = "sr_mapping"    // display segment-routing
CmdConfig    = "config"        // display current-configuration
CmdConfigSet = "config_set"    // show configuration | display set (Juniper)
CmdUnknown   = "unknown"       // 未识别的命令
```

### 2.6 演进路线

```
Phase 0 (✅)     Phase 1 (✅)     Phase 2 (✅)     Phase 3 (✅)     Phase 4 (规划中)
CLI 工具         智能 CLI          Agent Loop       Network Agent    自动化执行
解析/查询        变更方案生成       LLM tool calling  IM/记忆/巡检     SSH 自动采集
                                                   Rule Studio      执行审批闸门
```

---

## 3. 模块逐文件详解

### 3.1 入口与 CLI

| 文件 | 行数 | 功能 |
|------|------|------|
| `cmd/nethelper/main.go` | 12 | 极简入口：`cli.SetVersion(version); cli.Execute()` |
| `internal/cli/root.go` | ~100 | Cobra root 命令 + `PersistentPreRunE` 依赖注入 |
| `internal/cli/show.go` | | `show device/interface/route/fib/label/neighbor/tunnel/topology` |
| `internal/cli/plan.go` | | `plan isolate/upgrade/cutover` |
| `internal/cli/agent.go` | | `agent chat` |
| `internal/cli/channel.go` | | `channel start --feishu/discord/telegram/wechat/qq` |
| `internal/cli/heartbeat.go` | | `heartbeat start` |
| `internal/cli/mcp.go` | | `mcp serve` |
| `internal/cli/rule.go` | | `rule studio` (启动 Rule Studio Web UI) |
| `internal/cli/watch.go` | | `watch ingest/start/stop/status` |
| `internal/cli/trace.go` | | `trace path/impact` |
| `internal/cli/diff.go` | | `diff config/route` |
| `internal/cli/search.go` | | `search config/log/command` |
| `internal/cli/knowledge.go` | | `knowledge search/sources` |
| `internal/cli/check.go` | | `check loop/spof` |
| `internal/cli/note.go` | | `note add/search` |
| `internal/cli/diagnose.go` | | `diagnose` (LLM 排障) |
| `internal/cli/explain.go` | | `explain` (LLM 配置解读) |
| `internal/cli/export.go` | | `export db/topology/report` |
| `internal/cli/config_cmd.go` | | `config llm` (查看 LLM 配置状态) |

### 3.2 Parser 解析层

**根目录文件**（`internal/parser/`）：

| 文件 | 行数 | 核心功能 |
|------|------|---------|
| `types.go` | 59 | `VendorParser` 接口 + `CommandBlock` 结构体 + `Registry`（厂商注册中心） |
| `pipeline.go` | 624 | **核心管道**：`Ingest()` / `IngestIncremental()` / `processBlocks()` / `storeResult()` / `detectRawConfig()` / `reclassifyH3C()` / `extractBGPPeers()` / `extractVRFInstances()` / `extractRoutePolicies()` |
| `splitter.go` | 223 | `Split()` / `SplitWithOffset()`：按提示符行拆分日志为 CommandBlock；时间戳提取/剥离 |
| `detector.go` | 77 | `DetectVendor()` / `DetectVendorWithHints()`：4 种提示符正则 + 3 层检测（hints > regex > empty）|
| `collector.go` | 569 | **5 层过滤链**：空输出 → 控制命令 → 帮助回显 → 错误输出 → 命令规范化；`NormaliseCommand()` 缩写展开（55+ 映射）；`StripArgs()` 参数占位符替换 |
| `config_coverage.go` | 133 | `CheckCoverage()` 自检引擎：从配置推断应支持命令 → 检查 ClassifyCommand 覆盖率 |
| `config_infer.go` | 475 | `InferCommands()`：从华为/H3C/Cisco/Juniper 配置推断 display/show 命令（25+ 种正则匹配）|
| `config_extract.go` | 350 | `ExtractInterfacesFromConfig()`：从配置提取接口定义（华为 # 分隔/Cisco ! 分隔/Juniper {} 层级）|
| `config_bgp_huawei.go` | 832 | 华为/H3C BGP 对等体 + VRF + 路由策略提取 |
| `config_bgp_cisco.go` | 995 | Cisco IOS-XR BGP 对等体 + VRF + 路由策略提取（支持缩进和扁平两种格式）|
| `config_bgp_juniper.go` | 594 | Juniper BGP 对等体 + VRF + 路由策略提取（大括号层级解析）|
| `log_analyzer.go` | 207 | `AnalyzeSessionLog()`：日志分析（只标记不过滤），用于 Rule Studio 批量导入 |
| `model_extract.go` | 86 | `ExtractModel()`：从主机名提取设备型号（华为/H3C/Cisco/Juniper 4 厂商模式）|
| `field.go` | 18 | FieldType/FieldDef 类型别名（从 model 包重导出）|
| `field_registry.go` | 86 | `FieldRegistry`：按 vendor+CommandType 索引 FieldDef，供 Studio API 使用 |

### 3.3 Pipeline DSL 引擎

**`internal/parser/engine/`**：

| 文件 | 行数 | 功能 |
|------|------|------|
| `pipeline.go` | 1284 | **DSL 解释器核心**：14 条指令，4 种执行模式 |
| `table.go` | 118 | `ParseTable()`：基于 schema 的固定列表格解析（支持列自动检测）|
| `pipeline_test.go` | 862 | 20+ 测试用例 |
| `table_test.go` | 191 | 6 个测试 |

**DSL 指令集**：

| 阶段 | 指令 | 参数 | 说明 |
|------|------|------|------|
| Trimming | `SKIP_UNTIL` | `<regex>` | 跳过行直到正则匹配（包含该行）|
| Trimming | `SKIP_LINES` | `<N>` | 跳过 N 行 |
| Trimming | `SKIP_BLANK` | | 跳过空白行 |
| Trimming | `STOP_AT` | `<regex>` | 遇到匹配行时停止 |
| Trimming | `FILTER` | `<regex>` | 仅保留匹配行 |
| Trimming | `REJECT` | `<regex>` | 丢弃匹配行 |
| Extraction | `SPLIT` | `$a $b $c` | 按空白拆分，最后变量获取剩余 |
| Extraction | `REGEX` | `<pattern>` | 命名分组 `(?P<name>...)` 提取 |
| Extraction | `REPLACE` | `<old> <new>` | 正则替换（new 可为空 `""`）|
| Post | `SET` | `$name <expr>` | 拼接/三元/REGEX_MATCH 赋值 |
| Post | `EMIT` | | 显式发射行 |
| Multi | `SECTION` | | 开启独立子管道 |
| Repeat | `REPEAT_FOR` | `<regex>` | 按正则分块，支持嵌套 |

**SET 表达式**：
- 简单赋值：`SET $x "literal"`
- 变量引用：`SET $x $other`
- 拼接：`SET $full $slot "/" $port`
- 三元：`SET $icon $status == up ? ✓ : ✗`
- REGEX_MATCH：`SET $clean (REGEX_MATCH($val, "pattern") ? "yes" : "no")`

**4 种执行模式（自动检测）**：
1. **Table mode**：有 SPLIT → 每行一条输出
2. **Record mode**：仅 REGEX → 整段 → 一条记录
3. **Regex Table mode**：FILTER + REGEX → 每匹配行一条
4. **Section mode**：有 SECTION → 各段独立运行后按行索引合并

### 3.4 厂商解析器

#### 华为（`internal/parser/huawei/`，23 文件）

**手写解析器**：
| 文件 | 函数 | 解析命令 |
|------|------|---------|
| `interface_brief.go` | `ParseInterfaceBrief()` | `display interface brief` |
| `routing_table.go` | `ParseRoutingTable()` | `display ip routing-table`（支持 VRF）|
| `ospf_peer.go` | `parseOspfPeer()` | `display ospf peer` |
| `ldp_session.go` | `parseLdpSession()` + `ParseNeighbor()` | `display mpls ldp session` |
| `mpls_lsp.go` | `ParseMplsLsp()` | `display mpls lsp` |

**Rule Studio 生成的解析器（7 个）**：
| 文件 | Rule ID | 命令 | DSL 方式 |
|------|---------|------|---------|
| `ip_inter_brief.go` | 4 | `display ip inter brief` | SKIP_UNTIL + SPLIT |
| `link_aggregation_verbose_bridge_aggregation_1.go` | 6 | `display link-aggregation verbose bridge-aggregation 1` | SECTION 双段 |
| `route_policy.go` | 12 | `display route-policy` | 嵌套 REPEAT_FOR |
| `route_policy_no_more.go` | 18 | `display route-policy {name} no-more` | STOP_AT + 嵌套 REPEAT_FOR |
| `acl.go` | 19 | `display acl {id}` | REPEAT_FOR + REGEX |
| `acl_all.go` | 20 | `display acl all` | REPEAT_FOR + FILTER + REGEX |
| `bgp_network_ipv4.go` | 24 | `display bgp network ipv4` | SKIP_UNTIL + SPLIT |

**生成文件**：`huawei_generated.go`（92 行）— 自动分派 `classifyGenerated()` / `parseGenerated()` / `generatedCmdTypes()` / `generatedFieldSchema()`

**提示符检测**：`<hostname>` 和 `[hostname]` 格式，拒绝含 `@$~/[] ` 的字符串，要求 hostname ≥ 3 字符

#### H3C（`internal/parser/h3c/`，15 文件）

- 与华为共享 `<hostname>` 提示符，通过 `reclassifyH3C()` 配置签名检测区分
- `display_interface.go`（6.47 KB）最复杂：自动检测 3 种格式（IP interface brief/interface brief/verbose）
- 4 个 Rule Studio 生成规则

#### Cisco（`internal/parser/cisco/`，7 文件）

- 支持标准 IOS/IOS-XE 提示符 (`Router#`) 和 IOS-XR (`RP/0/RP0/CPU0:hostname#`)
- 0 个 Rule Studio 规则

#### Juniper（`internal/parser/juniper/`，7 文件）

- 提示符：`user@hostname>` 和 `user@hostname#`
- 支持 `| display set` 区分 `CmdConfig` / `CmdConfigSet`
- 0 个 Rule Studio 规则

### 3.5 Store 存储层

**`internal/store/`**（33 文件）

**核心基础设施**：
- `db.go`（76 行）：`Open()` 设 5 个 PRAGMA（WAL/foreign_keys/synchronous=NORMAL/cache_size=-64000/temp_store=MEMORY），`migrate()` 容忍 `ALTER TABLE ADD COLUMN` 重复
- `migrations.go`（525 行）：80+ 条 SQL，30+ 张表，3 个 FTS5 虚拟表，多次去重迁移，pending_rules v1→v2 重建

**三级内存缓存**：
| 缓存 | 文件 | 功能 | 线程安全 |
|------|------|------|---------|
| `VendorHintCache` | `vendor_hints.go` | 主机名关键词 → 厂商映射 | `sync.RWMutex` |
| `PatternCache` | `pattern_store.go` | 命令前缀 → cmdType 分类 | `sync.RWMutex` |
| `RuntimeRegistry` | `runtime_rule_store.go` | 运行时 DSL 规则（4D 匹配）| `sync.RWMutex` |

**4D 匹配引擎**（`RuntimeRegistry.MatchWithContext()`）：
1. vendor（精确匹配）
2. model_pattern（正则匹配设备型号）
3. os_pattern（正则匹配 OS 版本）
4. DSL content patterns（至少一个正则匹配输出内容）
5. 特异性评分：model != ".*" → +2，os != ".*" → +1
6. 最高分优先，同分取最小 ID

**关键设计模式**：
- UPSERT 去重：几乎所有写入用 `ON CONFLICT ... DO UPDATE`
- 时间戳保护：`interfaces` 表只允许新数据覆盖旧数据
- SHA-256 哈希去重：`config_snapshots` 通过内容哈希避免存储相同配置
- FTS5 全文搜索：config/troubleshoot/commands 三个维度
- FIFO 驱逐：`scratch_entries` 保留最新 200 条

### 3.6 Studio Web UI

**`internal/studio/`**（6 个 .go + 3 个静态文件）

| 文件 | 行数 | 功能 |
|------|------|------|
| `server.go` | 162 | Server 结构体 + 路由注册（~50 个端点）|
| `handlers.go` | 2609 | 核心 handler + 全部 HTML 模板（内联在 Go 常量中）|
| `handlers_coverage.go` | 482 | 覆盖率自检页面 + API |
| `handlers_phase3.go` | 596 | Vendor Hints/Patterns/Field Schemas/Runtime Rules CRUD |
| `batch_import.go` | 555 | 批量导入（文件上传/粘贴/SSE 流式）|
| `server_test.go` | 145 | 路由和 API 测试 |
| `static/app.js` | 1883 | 前端 JS（Toast/SSE/Monaco Editor/键盘快捷键）|
| `static/style.css` | 1104 | 暗色主题 CSS 设计系统 |
| `static/htmx.min.js` | | 第三方库 |

**HTML 不是独立文件**——全部通过 Go `html/template` 从 `handlers.go` 中的 `const` 字符串模板动态生成。

**规则生命周期**：
```
unknown_outputs → [LLM Generate] → pending_rules (draft)
    → [人工测试/LLM改进] → pending_rules (testing)
    → [Approve] → runtime_rules (热激活，无需 go build)
                 或 codegen (Go 代码生成 + go build)
```

### 3.7 MCP Server

**`internal/mcp/`**（7 文件，20 个 tools）

| 文件 | Tools |
|------|-------|
| `tools_show.go` | show_devices, show_device, show_interfaces, show_neighbors, show_bgp_peers, show_topology, show_routes |
| `tools_analysis.go` | trace_path, trace_impact, check_spof, check_loop |
| `tools_plan.go` | plan_isolate, plan_upgrade, plan_cutover |
| `tools_search.go` | search_config, search_log, diff_config |
| `tools_write.go` | note_add, watch_ingest, diagnose |

`show_bgp_peers` 默认返回 per-group 摘要（减少 token 消耗），可通过 `peer_group` 参数展开具体组。

`diagnose` 调用 `gatherMCPContext()` 收集设备数据（接口/邻居/BGP peers/配置/路由），格式化为 Markdown 注入 LLM 系统提示。

### 3.8 Agent Loop

**`internal/agent/`**（6 文件）

**核心循环**（`loop.go`，457 行）：
```
Chat(ctx, userInput, onToolCall):
  1. compactContext()          上下文压缩
  2. injectMemory()           首次消息注入历史记忆+知识库（只一次）
  3. append user message
  4. for i := 0; i < 20; i++: // 最多 20 轮
       resp = router.Chat()
       if resp.ToolCalls > 0:
           for each tool: execute → append result
           continue
       else:
           return resp.Content  // 最终回答
```

**上下文压缩策略**：
1. 工具结果截断：超 2000 字符 → 保留头尾
2. 消息驱逐：总字符数超 50000 → 从第 2 条起逐条删除

**系统提示加载**：从 `~/.nethelper/` 的 `SOUL.md` + `IDENTITY.md` + `TOOLS.md` 拼接（缺失文件用内嵌默认值）

**其他文件**：
- `tools.go`（269 行）：Tool/Registry 结构体 + 10 个网络工具注册
- `heartbeat.go`（115 行）：定时巡检，异常时推送 IM
- `logger.go`（82 行）：JSONL 审计日志
- `repl.go`（63 行）：交互式 REPL

### 3.9 Discovery Engine

**`internal/discovery/engine.go`**（~1062 行）

LLM 驱动的自动规则生成：
1. 聚类：按 (vendor, command_norm) 分组
2. LLM 生成：发送样本输出 → LLM 返回 Pipeline DSL
3. **4 层自测试**：
   - L0: DSL 语法验证（`ValidatePipelineDSL()`）
   - L1: 无错执行（`ExecPipeline()` 不报错）
   - L2: 非空结果（至少 1 行）
   - L2.5: 行数启发式（防遗漏）
   - L3: 字段非空率 > 50%
   - L4: LLM 语义验证
4. 自动修复：`fixDSL()` 最多 2 次重试
5. 流式事件：`RunStream()` 发出 start/pending/success/failed/done

### 3.10 其他模块

| 模块 | 功能 |
|------|------|
| `internal/graph/` | 内存拓扑图：`BuildFromDB()` → BFS/DFS/SPOF/环路/影响分析 |
| `internal/plan/` | 变更方案：`BuildTopology()` + `GenerateIsolationPlanV2()/UpgradePlan()/CutoverPlan()` |
| `internal/llm/` | OpenAI + Anthropic 双协议 Provider，能力路由 Router |
| `internal/memory/` | 向量记忆 `Insert()/Search()` + 知识库聚合器 `Aggregator` |
| `internal/channel/` | IM 适配：飞书 WebSocket / Discord / Telegram / 微信 / QQ |
| `internal/codegen/` | Go 代码生成器：从 pending rule 生成 `*_generated.go` |
| `internal/watcher/` | fsnotify 文件监控 + 增量读取 |
| `internal/diff/` | 文本 diff 引擎 |
| `internal/config/` | YAML 配置加载 |
| `internal/model/` | 统一数据模型 struct 定义 |

---

## 4. 数据库完整 Schema

### 4.1 表一览（30+ 张）

| 分类 | 表名 | 主键 | 用途 |
|------|------|------|------|
| **网络** | `devices` | `id TEXT` | 设备主表 |
| | `interfaces` | `id TEXT` | 接口（UPSERT 时间戳保护）|
| | `snapshots` | `id INTEGER AUTOINCREMENT` | 快照 |
| **路由** | `rib_entries` | `id INTEGER` | RIB |
| | `fib_entries` | `id INTEGER` | FIB |
| | `lfib_entries` | `id INTEGER` | MPLS 标签表 |
| **协议** | `protocol_neighbors` | `id INTEGER` | 邻居（去重唯一索引）|
| | `mpls_te_tunnels` | `id INTEGER` | TE 隧道 |
| | `sr_mappings` | `id INTEGER` | SR SID |
| **BGP/VPN** | `bgp_peers` | `id INTEGER` | BGP 对等体（19 列）|
| | `vrf_instances` | `id INTEGER` | VPN/VRF |
| | `route_policies` | `id INTEGER` | 路由策略头 |
| | `route_policy_nodes` | `id INTEGER` | 策略节点/条目 |
| **配置** | `config_snapshots` | `id INTEGER` | 配置（SHA-256 去重）|
| **搜索** | `fts_config` | FTS5 | 配置全文搜索 |
| | `fts_troubleshoot` | FTS5 | 排障日志全文搜索 |
| | `fts_commands` | FTS5 | 命令参考全文搜索 |
| **记忆** | `memory_entries` | `id INTEGER` | 向量记忆 |
| | `conversations` | `id INTEGER` | 对话历史（per-user）|
| | `knowledge_cache` | `id INTEGER` | 知识库嵌入缓存 |
| **Rule Studio** | `unknown_outputs` | `id INTEGER` | 未知命令输出 |
| | `pending_rules` | `id INTEGER` | 规则草稿 |
| | `rule_test_cases` | `id INTEGER` | 测试用例 |
| | `runtime_rules` | `id INTEGER` | 运行时 DSL 规则 |
| **配置化** | `vendor_hostname_hints` | `id INTEGER` | 厂商提示 |
| | `classification_patterns` | `id INTEGER` | 分类模式 |
| | `field_schemas` | `id INTEGER` | 字段元数据 |
| | `coverage_checks` | `id INTEGER` | 覆盖率 |
| **其他** | `scratch_entries` | `id INTEGER` | 暂存（FIFO 200 条）|
| | `troubleshoot_logs` | `id INTEGER` | 排障笔记 |
| | `log_ingestions` | `id INTEGER` | 导入记录 |
| | `import_history` | `id INTEGER` | 批量导入历史 |
| | `command_references` | `id INTEGER` | 命令手册 |
| | `embedding_meta` | `rowid INTEGER` | 嵌入元数据 |

### 4.2 关键唯一索引

```sql
-- 邻居去重（5D）
UNIQUE INDEX idx_neighbors_dedup ON protocol_neighbors(device_id, protocol, remote_id, remote_address, snapshot_id)
-- BGP peers 去重（5D）
UNIQUE INDEX idx_bgp_peers_dedup ON bgp_peers(device_id, peer_ip, address_family, vrf, snapshot_id)
-- 运行时规则（4D）
UNIQUE INDEX idx_rr_vendor_model_os_cmd ON runtime_rules(vendor, model_pattern, os_pattern, command_pattern)
-- 分类模式
UNIQUE INDEX idx_cp_vendor_prefix ON classification_patterns(vendor, prefix)
-- 字段 schema
UNIQUE INDEX idx_fs_vendor_cmd_field ON field_schemas(vendor, cmd_type, field_name)
```

---

## 5. Rule Studio API 参考

### 5.1 页面路由

| 路径 | 方法 | 页面 |
|------|------|------|
| `/` | GET | Dashboard |
| `/rules` | GET | 规则列表（搜索/过滤）|
| `/rule/{id}` | GET/POST | 规则编辑器+沙盒 |
| `/test` | GET | Parser Tester |
| `/fields` | GET | Field Browser |
| `/compare` | GET | Cross-Vendor Compare |
| `/patterns` | GET | Command Patterns |
| `/unknown` | GET | Unknown Outputs |
| `/history` | GET | History |
| `/import` | GET | Batch Import |
| `/vendor-hints` | GET | Vendor Hints |
| `/coverage` | GET | Coverage 列表 |
| `/coverage/{device}` | GET | Coverage 详情 |

### 5.2 API 端点

| 路径 | 方法 | 功能 | 请求 | 响应 |
|------|------|------|------|------|
| `/api/rule/{id}/test` | POST | 测试 DSL | `input=<text>` | PipelineResult JSON |
| `/api/rule/{id}/testcase` | POST | 保存测试用例 | `input,expected,description` | `{id}` |
| `/api/rule/{id}/run-all-tests` | POST | 运行全部测试 | 无 | `{all_passed,results[]}` |
| `/api/rule/{id}/improve` | POST | LLM 改进 DSL | 无 | `{improved_dsl}` |
| `/api/rule/{id}/approve` | POST | 审批 | 无 | `{status,mode}` |
| `/api/rule/{id}/save-local` | POST | 保存到本地 | 无 | `{success,paths[]}` |
| `/api/rule/{id}/delete` | POST | 删除规则 | 无 | `{status}` |
| `/api/rule/{id}/regenerate` | POST | 重新生成 | 无 | `{rule_id,redirect}` |
| `/api/rule/{id}/save-patterns` | POST | 保存 4D 模式 | JSON `{model_pattern,os_pattern}` | `{success}` |
| `/api/discover` | GET | 运行 Discovery（SSE）| `?vendor=` | SSE 流 |
| `/api/import/analyze` | POST | 分析日志 | JSON/multipart | LogAnalysis |
| `/api/import/generate` | POST | 批量生成（SSE）| JSON | SSE 流 |
| `/api/import/manual` | POST | 手动添加 | JSON | `{status,pattern}` |
| `/api/unknown/{id}/generate` | POST | 单条生成（SSE）| 无 | SSE 流 |
| `/api/unknown/batch-generate` | POST | 批量生成（SSE）| JSON `{ids[]}` | SSE 流 |
| `/api/coverage/recheck` | POST | 重新检查 | `device_id` | 302 重定向 |
| `/api/coverage/ssh` | GET | 导出 SSH 脚本 | `?device_id=` | text/plain |
| `/api/coverage/boost` | POST | 一键 Boost（SSE）| `device_id` | SSE 流 |
| `/api/vendor-hints` | GET/POST | Vendor Hints CRUD | JSON | JSON |
| `/api/patterns` | GET/POST | Patterns CRUD | JSON | JSON |
| `/api/field-schemas` | GET/POST | Field Schemas CRUD | JSON | JSON |
| `/api/runtime-rules` | GET/POST | Runtime Rules CRUD | JSON | JSON |

---

## 6. DSL 引擎完整参考

详见 [3.3 节](#33-pipeline-dsl-引擎)。

**常见 DSL 模式示例**：

```
# 简单表格：display interface brief
SKIP_UNTIL ^Interface\s+PHY
SPLIT $interface $phy $protocol $in_util $out_util $in_errors $out_errors

# 多段合并：display link-aggregation verbose bridge-aggregation 1
SKIP_UNTIL ^Port\s+Status\s+Priority
SKIP_LINES 1
STOP_AT ^Remote:
REPLACE \{|\} ""
SPLIT $local_port $local_status $local_priority $local_oper_key $local_flag
SECTION
SKIP_UNTIL ^Actor\s+Partner\s+Priority
SKIP_LINES 1
STOP_AT ^<
REPLACE \{|\} ""
SPLIT $remote_port $remote_partner $remote_priority $remote_oper_key $remote_sys1 $remote_sys2 $remote_flag
SET $remote_systemid $remote_sys1 " " $remote_sys2

# 嵌套重复：display route-policy
REPEAT_FOR ^Route-policy:\s+(?P<route_policy_name>.+)$
REPEAT_FOR ^\s*(?P<action>permit|deny)\s*:\s*(?P<seq>\d+)\s*\(matched counts:\s*(?P<matched_counts>\d+)\)
REGEX ^\s*if-match\s+(?P<match_clause>.+)$
REGEX ^\s*apply\s+(?P<apply_clause>.+)$
```

---

## 7. 数据流追踪

### 7.1 从日志到数据库

```
日志文件 → pipeline.Ingest()
  → Split() → []CommandBlock{Hostname, Vendor, Command, Output, CapturedAt}
  → processBlocks()
    → ClassifyCommand() → block.CmdType
    → storeResult()
      → model.ParseResult.Interfaces → UpsertInterface() → interfaces 表
      → model.ParseResult.RIBEntries → InsertRIBEntries() → rib_entries 表
      → model.ParseResult.ConfigText → InsertConfigSnapshot() → config_snapshots 表
        → ExtractBGPPeers() → InsertBGPPeers() → bgp_peers 表
```

### 7.2 从未知命令到生成规则

```
pipeline 中 CmdUnknown → Collector.Collect()
  → 5 层过滤（空/控制/帮助/错误/规范化）
  → UpsertUnknownOutput() → unknown_outputs 表

Rule Studio → discovery.Engine.GenerateForUnknown()
  → 从 unknown_outputs 取样本
  → LLM 生成 DSL
  → 4 层自测试 + 自动修复
  → CreatePendingRule() → pending_rules 表

用户 Approve → UpsertRuntimeRule() → runtime_rules 表
  → RuntimeRegistry.Reload() → 内存热加载
```

### 7.3 从 Agent 消息到工具调用

```
用户消息 → Agent.Chat()
  → compactContext() → 压缩历史
  → injectMemory() → 注入向量记忆+知识库
  → router.Chat() → LLM 返回 ToolCalls
  → 执行每个 tool → 获取结果
  → 再次 router.Chat() → LLM 综合分析
  → 返回最终回答
```

---

## 8. 待优化待落实功能

### 8.1 高优先级

| 问题 | 影响 | 建议方案 | 相关文件 |
|------|------|---------|---------
| **SSH 自动采集** | 依赖人手动保存日志 | 实现 SSH/NETCONF 通道 | 新建 `internal/ssh/` |
| **LLM 流式输出** | 等待体验差 | streaming token | `internal/llm/openai.go` |
| **测试覆盖率不均** | agent/channel 覆盖不足 | 补充集成测试 | `test/integration/` |
| **handlers.go 过大** | 2609 行，难以维护 | 拆分 HTML 模板到独立文件 | `internal/studio/handlers.go` |

### 8.2 中优先级

| 问题 | 建议 |
|------|------|
| H3C/华为混淆 | 加强 vendor_hostname_hints + 设备型号启发式 |
| Context 摘要 | 实现 `enable_summary`（预留字段未实现）|
| IM 文件接收 | adapter 实现下载 + pipeline.Ingest |
| Peer group 角色推断 | 多维推断（AS 关系 + 拓扑位置 + 命名规则）|

### 8.3 已知技术债务

1. `internal/studio/handlers.go` 2609 行，HTML 模板内联
2. 向量搜索全表扫描 + 余弦相似度，>10 万条需优化
3. SQLite WASM 性能 10-20% 开销
4. `te` 缩写歧义：MPLS traffic-engineering vs ten-gigabitethernet
5. Rule Studio 生成测试中 `TODO: parse expected JSON and assert returned fields` 未实现

---

## 9. 运维及部署手册

### 9.1 环境要求

- **Go 1.22+**（编译）
- **无 CGo 依赖**（SQLite 通过 WASM `ncruces/go-sqlite3` + `tetratelabs/wazero`）
- **支持平台**：macOS、Linux

### 9.2 编译与安装

```bash
# 编译
./build.sh    # 或: go build -o nethelper ./cmd/nethelper

# 安装（交互式）
./install.sh

# 桌面版
./build-desktop.sh build
```

### 9.3 配置

**主配置**：`~/.nethelper/config.yaml`（最小配置）：
```yaml
watch_dirs:
  - ~/network-logs
llm:
  default: ollama
  providers:
    ollama:
      base_url: http://localhost:11434
      model: qwen2.5:14b
```

**完整配置参考**：`docs/configuration.md`（688 行）

**数据目录**：
```
~/.nethelper/
├── config.yaml          # 主配置
├── SOUL.md              # Agent 人格（首次启动自动创建）
├── IDENTITY.md          # Agent 身份
├── TOOLS.md             # 工具使用指南
├── nethelper.db         # SQLite（全部数据）
├── knowledge/           # 外挂知识库（*.md）
└── sessions/            # JSONL 审计日志
```

### 9.4 MCP 配置

`.mcp.json`：
```json
{
  "mcpServers": {
    "nethelper": {
      "command": "nethelper",
      "args": ["mcp", "serve"]
    }
  }
}
```

### 9.5 测试

```bash
go test ./...                                    # 单元测试
go test -tags=integration -v ./test/integration/ # 集成测试
go vet ./...                                     # 静态检查
```

### 9.6 数据备份与迁移

```bash
nethelper export db -o backup.db    # 备份
scp ~/.nethelper/ newhost:~/        # 迁移
```

---

## 10. 常见 FAQ

### Q1: 不配置 LLM 能用吗？
完全可以。LLM 只影响 `agent chat`、`channel start`、`heartbeat`、`mcp serve`(diagnose)、Rule Studio 的生成/改进。

### Q2: 华为和华三怎么区分？
`reclassifyH3C()` 检测 `version 7.`（H3C Comware 7）和 `mdc admin id`（H3C MDC）签名。还可通过 Vendor Hints 配置主机名关键词映射。

### Q3: Rule Studio 规则如何生效？
Pipeline/DSL 规则：Approve 后写入 `runtime_rules`，热重载到内存，**无需 go build**。传统 Go 代码：codegen 生成 `.go` 文件，需 `go build`。

### Q4: Pipeline DSL 怎么调试？
Rule Studio（`http://localhost:7070`）→ 编辑 DSL → 粘贴输出 → Run Parse → 保存测试用例 → Run All → Ask LLM to Improve。

### Q5: 新增厂商解析器需要改哪些文件？
1. `internal/parser/<vendor>/` — 实现 `VendorParser`
2. `internal/cli/root.go` — `registry.Register()`
3. 详见 `docs/extension-guide.md`

### Q6: 新增 MCP Tool 需要改哪些文件？
1. `internal/mcp/tools_*.go` — `s.AddTool()`
2. `internal/agent/tools.go` — `RegisterNethelperTools()` 中添加（如需 Agent 也能用）

### Q7: 数据库越来越大怎么办？
```sql
DELETE FROM rib_entries WHERE snapshot_id IN (
    SELECT id FROM snapshots WHERE captured_at < datetime('now', '-30 days')
);
```

### Q8: 关键依赖有哪些？
| 依赖 | 用途 |
|------|------|
| `spf13/cobra` | CLI 框架 |
| `ncruces/go-sqlite3` + `tetratelabs/wazero` | CGo-free SQLite（WASM）|
| `fsnotify/fsnotify` | 文件监控 |
| `mark3labs/mcp-go` | MCP 协议 |
| `larksuite/oapi-sdk-go/v3` | 飞书 SDK |
| `bwmarrin/discordgo` | Discord SDK |
| `wailsapp/wails/v2` | 桌面 GUI |

### Q9: 4D 规则匹配是什么？
Runtime Rules 按 (vendor, model_pattern, os_pattern, command_pattern) 做精确度评分匹配。model != ".*" → +2分，os != ".*" → +1分，最高分胜出。

### Q10: 项目最重要的 10 个文件？

| 文件 | 行数 | 说明 |
|------|------|------|
| `CLAUDE.md` | 129 | 项目工作指南 |
| `internal/cli/root.go` | ~100 | 启动+依赖注入 |
| `internal/parser/pipeline.go` | 624 | 核心解析管道 |
| `internal/parser/collector.go` | 569 | 未知命令收集 |
| `internal/parser/engine/pipeline.go` | 1284 | Pipeline DSL 引擎 |
| `internal/store/migrations.go` | 525 | 完整数据库 Schema |
| `internal/discovery/engine.go` | ~1062 | LLM 规则生成 |
| `internal/agent/loop.go` | 457 | Agent Loop |
| `internal/studio/server.go` | 162 | Rule Studio 路由 |
| `internal/studio/handlers.go` | 2609 | Rule Studio 处理器 |
