# nethelper 阶段性汇报 — 从 CLI 到 Network Agent

**日期：** 2026-03-24
**版本：** Phase 0-3 全部完成

---

## 一、项目概况

nethelper 是一个面向网络工程师的智能运维工具，从终端日志解析起步，经过四个阶段的演进，发展为具备 Agent Loop、IM 接入、向量记忆的网络运维数字助手。

### 数字概览

| 指标 | 数据 |
|------|------|
| Go 代码总量 | 26,247 行 |
| 包（packages）| 25 个 |
| Git commits | 168 个 |
| 数据库表 | 20+ 个（含 FTS5 虚拟表） |
| MCP Tools | 20 个 |
| IM 平台 | 5 个（飞书/Discord/Telegram/微信/QQ） |
| 支持厂商 | 4 个（华为 VRP/H3C Comware/Cisco IOS-XR/Juniper JUNOS） |

### 演进路径

```
Phase 0           Phase 1           Phase 2           Phase 3
CLI 工具    →    智能 CLI     →    Agent Loop   →    Network Agent
(✅)              (✅)              (✅)               (✅)

人执行命令       工具理解配置       agent 自主探索     可被调用的服务
人看结果         生成精确方案       人描述需求即可     IM 通讯/记忆/巡检
```

---

## 二、已完成能力矩阵

### Phase 0: CLI 工具

| 能力 | 实现 |
|------|------|
| 四厂商日志解析 | Huawei VRP、H3C Comware、Cisco IOS-XR、Juniper JUNOS，提示符自动识别 |
| 配置备份文件解析 | 无终端提示符的纯配置文件（NMS 采集的备份）自动检测和导入 |
| 增量采集 | fsnotify 文件监控 + 命令边界回溯（`SplitWithOffset`），不截断大配置 |
| 结构化存储 | SQLite WAL 模式，设备/接口/路由/邻居/BGP/VRF/隧道/标签全覆盖 |
| 全文搜索 | FTS5 索引覆盖配置、排障记录、命令手册 |
| 拓扑引擎 | 内存图（BFS 路径、DFS 环路、关节点 SPOF、影响分析） |
| 配置变更追踪 | 快照 + diff，SHA256 hash 去重 |
| LLM 增强 | OpenAI/Anthropic 双协议，多 provider 路由，排障建议 + 配置解读 |

### Phase 1: 智能 CLI

| 能力 | 实现 |
|------|------|
| `plan isolate` v2 | 多维度拓扑发现（BGP peers + 接口 description + 子网匹配 + 配置推断），按 peer group 分步，每步有检查点。LC 设备生成了包含 234 个 BGP peers 的精确方案 |
| `plan upgrade` | 8 阶段全流程（隔离→升级命令→重启等待→版本验证→恢复），4 厂商升级命令模板 |
| `plan cutover` | 7 阶段链路割接（配置新口→验证→切流量→关旧口→验证），IGP cost 操纵 |
| ISIS/OSPF/LDP 隔离 | ISIS set-overload、OSPF stub-router、LDP per-interface disable |
| 多 session 并发 | per-file mutex，UPSERT 去重，capture-time 条件更新，config hash 去重 |

### Phase 2: Agent Loop

| 能力 | 实现 |
|------|------|
| MCP Server | `nethelper mcp serve`，20 个 tools，stdio 传输，Claude Code 配置即用 |
| Agent Loop | LLM function calling（OpenAI + Anthropic tool calling 协议），while loop + tool 执行 + context 累积 |
| 交互式 REPL | `nethelper agent chat`，多轮对话，tool call 实时显示 |
| 经验归档 | system prompt 引导 agent 主动提议归档，结构化提取症状/命令/发现/结论 |
| 经验复用 | 每次对话自动先搜历史经验（search_log），参考后再推理 |

### Phase 3: Network Agent

| 能力 | 实现 |
|------|------|
| 向量记忆 | Ollama qwen3-embedding，SQLite BLOB 存储，Go 层余弦相似度搜索，per-session 注入 |
| IM 接入 | 飞书（WebSocket + 卡片消息 + PATCH 流式更新）、Discord（Gateway WS）、Telegram（长轮询）、微信（HTTP 桥接）、QQ（OneBot WS） |
| 权限分组 | admin/operator/viewer，tool 通配符匹配（`show_*`），per-user agent 隔离 |
| 心跳巡检 | cron 定时 + JSONL 审计日志 + 异常时 IM 告警推送（正常时静默） |
| Soul/Identity | SOUL.md + IDENTITY.md + TOOLS.md 配置文件驱动，无需重编译 |
| 外挂知识库 | 本地 .md 文件 embedding + HTTP API 多源聚合（iWiki/IMA 等），并行搜索 |
| 对话持久化 | conversations 表 UPSERT，session 重启/过期不丢历史 |
| Context Engine | token 预算（50K chars）+ tool result 裁剪（2K），可配置 |
| 消息去重 | 飞书事件重复投递防护（message_id 去重） |

---

## 三、架构设计

### 系统架构图

```
┌─────────────────────────────────────────────────────────┐
│                    nethelper                              │
│                                                           │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐               │
│  │ CLI      │  │ MCP      │  │ Agent    │               │
│  │ Commands │  │ Server   │  │ Loop     │               │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘               │
│       │              │              │                     │
│  ┌────┴──────────────┴──────────────┴─────┐              │
│  │          Internal Go API               │              │
│  │  store / graph / plan / parser / llm   │              │
│  └────────────────┬───────────────────────┘              │
│                   │                                       │
│  ┌────────────────┴───────────────────────┐              │
│  │           SQLite + FTS5                 │              │
│  │  devices, interfaces, bgp_peers,        │              │
│  │  config_snapshots, memory_entries,       │              │
│  │  conversations, knowledge_cache, ...     │              │
│  └─────────────────────────────────────────┘              │
│                                                           │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐               │
│  │ Channel  │  │ Memory   │  │ Heartbeat│               │
│  │ Router   │  │ Aggregator│ │ Patrol   │               │
│  └────┬─────┘  └────┬─────┘  └──────────┘               │
│       │              │                                    │
│  ┌────┴─────┐  ┌────┴─────┐                              │
│  │ IM       │  │ Knowledge│                              │
│  │ Adapters │  │ Sources  │                              │
│  │ 飞/D/T/W/Q│  │ local/HTTP│                             │
│  └──────────┘  └──────────┘                              │
└─────────────────────────────────────────────────────────┘
```

### 三层入口

| 入口 | 用户 | 交互方式 |
|------|------|---------|
| **CLI** | 网络工程师直接使用 | `nethelper show device` / `nethelper plan isolate` |
| **MCP Server** | Claude Code / 其他 agent | `nethelper mcp serve`（stdio JSON-RPC） |
| **Agent Chat** | 终端 REPL / IM | `nethelper agent chat` / `nethelper channel start` |

三个入口共享同一套内部 Go API（store/graph/plan/parser），不存在功能差异——同一个 `BuildTopology()` 函数被 CLI、MCP、Agent 三条路径调用。

### 模块化设计

| 模块 | 职责 | 扩展点 |
|------|------|--------|
| `parser/` | 厂商日志解析 | 新厂商：实现 `VendorParser` 接口 |
| `store/` | SQLite 数据层 | 新表：追加 migration |
| `graph/` | 拓扑分析算法 | 新算法：新增分析函数 |
| `plan/` | 变更方案生成 | 新场景：新增 `Generate*Plan()` + `commands_*.go` |
| `llm/` | LLM 提供者抽象 | 新协议：实现 `Provider` 接口 |
| `agent/` | Agent Loop + 工具注册 | 新工具：在 `RegisterNethelperTools()` 中 `.Register()` |
| `channel/` | IM 平台适配 | 新 IM：实现 `Channel` 接口 |
| `memory/` | 向量记忆 + 知识源 | 新知识源：实现 `KnowledgeSource` 接口 |
| `mcp/` | MCP tool 暴露 | 新 tool：在对应 `tools_*.go` 中 `AddTool()` |

### 关键接口

```go
// 厂商解析器
type VendorParser interface {
    Vendor() string
    DetectPrompt(line string) (hostname string, ok bool)
    ClassifyCommand(cmd string) model.CommandType
    ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error)
}

// IM 平台适配器
type Channel interface {
    Name() string
    Start(ctx context.Context, handler MessageHandler) error
    Stop() error
    SendText(chatID string, text string) error
}

// 流式卡片（可选扩展）
type StreamingChannel interface {
    Channel
    SendInitCard(chatID string, text string) (cardID string, err error)
    UpdateCard(chatID string, cardID string, text string) error
    FinalizeCard(cardID string, text string) error
}

// LLM 提供者
type Provider interface {
    Name() string
    Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
    Supports(cap Capability) bool
}

// 知识源
type KnowledgeSource interface {
    Name() string
    Search(ctx context.Context, query string, topK int) ([]SearchResult, error)
}
```

---

## 四、架构决策记录（ADR）

### ADR-1: Go + CGo-free SQLite

**决策：** 使用 Go 语言 + ncruces/go-sqlite3（WASM 实现，不依赖系统 CGo）。

**背景：** 需要单二进制分发、跨平台、不依赖外部数据库。

**权衡：**
- ✅ 单文件 `nethelper` 即可运行，`scp` 即可迁移
- ✅ 不需要安装 SQLite 开发库
- ⚠️ 无法使用 sqlite-vec 扩展（WASM 兼容性），向量搜索在 Go 层实现
- ⚠️ WASM 解释执行有 10-20% 性能开销，但对我们的数据量（万级）可忽略

### ADR-2: 向量搜索 Go 层实现

**决策：** 不用 sqlite-vec，在 Go 层全表扫描 + 余弦相似度计算。

**背景：** ncruces/go-sqlite3 是 WASM 实现，加载 C 扩展有兼容性风险。

**权衡：**
- ✅ 零外部依赖，100% 可移植
- ✅ 几千条记忆全表扫描 <10ms，完全够用
- ⚠️ 如果记忆超过 10 万条，需要考虑索引或分批加载

### ADR-3: LLM 双协议支持

**决策：** 同时支持 OpenAI 兼容协议和 Anthropic 原生协议，包括 tool calling。

**背景：** 用户可能使用 Kimi、Ollama、DeepSeek（OpenAI 兼容）或 Claude（Anthropic 协议）。

**实现：** `openai.go` 和 `anthropic.go` 分别处理请求/响应格式转换，统一为内部 `ChatRequest/ChatResponse` 类型。Tool calling 两端都支持。

### ADR-4: 配置分层

**决策：** 结构化参数走 YAML（`config.yaml`），人格化内容走 Markdown 文件（`SOUL.md` 等）。

**理由：**
- config.yaml 适合键值对、列表、嵌套结构（LLM 配置、channel 凭证、权限组）
- Markdown 文件适合长文本、可读性强的内容（人格定义、工具使用指南）
- 两者都可以热加载（重启 agent 即生效），不需要重新编译

### ADR-5: 飞书卡片方案

**决策：** 使用 inline interactive card JSON + Message.Patch 更新，而非 CardKit API。

**背景：** CardKit API 的 `card_id` 无法直接通过 `im.message.create` 发送（格式不兼容）。inline card JSON 是飞书消息 API 的原生支持格式。

**流程：** 发送 inline card（蓝色 header）→ 拿到 message_id → Patch 更新内容 → 最终 Patch 为绿色 header。

---

## 五、技术债务 & 已知局限

### 高优先级

| 问题 | 影响 | 建议 |
|------|------|------|
| **测试覆盖率不均** | agent/channel/mcp/memory 包 0% 覆盖 | 补充核心路径的集成测试 |
| **设备数据手动采集** | 依赖人在终端执行命令并保存日志 | 实现 SSH/NETCONF 自动采集通道 |
| **LLM 非流式输出** | agent 回复是一次性返回，用户等待体验差 | 实现 LLM streaming + 飞书 token-by-token 更新 |

### 中优先级

| 问题 | 影响 | 建议 |
|------|------|------|
| H3C/华为厂商混淆 | 共享提示符格式，靠配置内容区分 | 加入设备型号/hostname 启发式规则 |
| BGP 命令缺 AS 号 | `bgp` 进入需要 AS 号 | Link/PeerGroup 结构加 LocalAS 字段（已部分完成） |
| Peer group 角色推断粗糙 | QCDR 被分为 downlink 而非 uplink | 基于 AS 关系+拓扑位置+命名规则多维推断 |

### 低优先级

| 问题 | 影响 | 建议 |
|------|------|------|
| context 摘要模型未启用 | `enable_summary` 配置项预留但未实现 | 分层摘要 LLM 调用 |
| 文件接收未完整实现 | IM 收到文件只记日志 | adapter 实现下载 + pipeline.Ingest |

---

## 六、配置全景

```yaml
# ~/.nethelper/config.yaml

# 数据存储
data_dir: ~/.nethelper
db_path: ~/.nethelper/nethelper.db

# 日志监控
watch_dirs:
  - ~/Work/session_log/netdevice

# LLM 配置（多 provider 路由）
llm:
  default: anthropic
  providers:
    ollama: { base_url: "http://localhost:11434", model: "qwen2.5:14b" }
    anthropic: { api_key: "...", model: "k2p5", base_url: "https://api.kimi.com/coding/" }

# Embedding 向量模型
embedding:
  provider: ollama
  providers:
    ollama: { base_url: "http://localhost:11434", model: "qwen3-embedding" }

# IM 接入
channels:
  feishu: { app_id: "...", app_secret: "...", enabled: true }
  discord: { token: "...", enabled: false }
  telegram: { token: "...", enabled: false }
  wechat: { bridge_url: "...", token: "...", enabled: false }
  qq: { ws_url: "...", enabled: false }

# 权限分组
permissions:
  groups:
    admin: { users: ["feishu:ou_xxx"], tools: ["*"] }
    viewer: { users: ["*"], tools: ["show_*", "search_*"] }

# 心跳巡检
heartbeat:
  enabled: true
  interval: "30m"
  channel: "feishu"
  chat_id: "oc_xxx"

# Context 压缩
context:
  max_token_budget: 50000
  tool_result_max_len: 2000

# 外挂知识库
knowledge:
  sources:
    - type: http
      name: iwiki
      url: "https://iwiki.example.com/api"
      token: "xxx"
      enabled: false
```

```
~/.nethelper/
├── config.yaml          # 全局配置
├── SOUL.md              # Agent 人格/风格/边界
├── IDENTITY.md          # 名字/emoji/自我介绍
├── TOOLS.md             # 工具使用指南
├── knowledge/           # 外挂知识库（*.md）
├── sessions/            # JSONL 审计日志
└── nethelper.db         # SQLite 数据库
```

---

## 七、与 OpenClaw 的对标

| 维度 | OpenClaw | nethelper | 差距 |
|------|----------|-----------|------|
| Agent Loop | ✅ Pi agent + RPC | ✅ Go agent + tool calling | 平齐 |
| 多 IM | ✅ 20+ 平台 | ✅ 5 平台 | 平台数量少但架构可扩展 |
| 长期记忆 | ✅ Memory compaction | ✅ 向量 embedding | 缺乏自动 compaction |
| Soul/Identity | ✅ SOUL.md | ✅ SOUL.md | 平齐 |
| 心跳 | ✅ Cron + Heartbeat | ✅ cron + JSONL + IM | 平齐 |
| 外挂知识 | ✅ extraSearch | ✅ 多源聚合 | 平齐 |
| Worktree 隔离 | ✅ Git Worktree | ❌ 无（非代码场景） | N/A |
| 权限 | ✅ DM pairing + allowlist | ✅ 权限分组 + tool filter | 简化但够用 |
| 技术栈 | TypeScript/Node.js | Go | Go 更适合网络工具（性能+部署） |
| 领域 | 通用 AI 助手 | **网络运维专用** | 垂直优势 |

---

## 八、未来方向

### 短期（1-2 周）

| 方向 | 说明 | 价值 |
|------|------|------|
| **设备自动采集** | SSH/NETCONF 通道，agent 自主抓取配置和状态 | 从"辅助工具"到"自主 agent"的关键 |
| **MCP tools 细化** | 加 raw SQL query、config text、per-peer detail 等原子 tools | 让其他 agent 调用更灵活 |
| **测试补全** | agent/channel/mcp/memory 核心路径测试 | 质量保障 |

### 中期（1-2 月）

| 方向 | 说明 | 价值 |
|------|------|------|
| **LLM 流式输出** | streaming token + 飞书 CardKit 实时更新 | 用户体验 |
| **被动告警接收** | Webhook 接收 Zabbix/Prometheus 告警，agent 自动排障 | 从巡检到响应 |
| **多 agent 编排** | 网络 agent + 安全 agent + CMDB agent 协作 | 复杂场景 |
| **配置合规检查** | 基于规则/策略自动扫描配置偏差 | 安全运维 |

### 长期

| 方向 | 说明 |
|------|------|
| **自动化执行** | 从"生成方案"到"执行方案"（SSH 下发配置，需人工审批闸门） |
| **可视化拓扑** | Web UI 展示网络拓扑图，支持交互式探索 |
| **多租户** | 不同团队/项目使用独立的数据库和配置 |

---

## 九、总结

nethelper 在两天内完成了从 CLI 工具到 Network Agent 的四阶段演进。核心设计原则是**模块化 + 接口化**——每个新能力都通过实现标准接口（VendorParser、Channel、KnowledgeSource、Provider）接入，而非硬编码。

从技术路线看，这是 learn-claude-code 文章描述的 v0→v4 演化在网络运维领域的一次实践：

```
v0: CLI 工具（bash is all you need）
v1: Tool calling（Model as Agent）
v2: 经验积累（Explicit Todo → note_add）
v3: 并行处理（MCP Server → 被调用）
v4: 可扩展（Soul + Memory + Knowledge + Channel）
```

**当前最大的杠杆点是设备自动采集**——这是从"人给工具喂数据"到"工具自己获取数据"的转折点。做完之后，agent 才能真正闭环排障，而不是每次都要等人去设备上跑命令。
