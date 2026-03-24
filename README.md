# nethelper

网络工程师的智能运维工具 —— 从日志解析到 Agent 闭环。

```
终端日志 → 自动解析 → 拓扑理解 → 变更方案生成 → Agent 自主排障
```

## 演进路线

```
Phase 0 (✅)          Phase 1 (✅)          Phase 2 (✅)          Phase 3 (✅)
CLI 工具              智能 CLI               Agent Loop            Network Agent
─────────────── → ─────────────── → ─────────────── → ───────────────

人执行命令            工具理解配置            agent 自主探索         可被调用的服务
人看结果              生成精确方案            人描述需求即可         IM 通讯 / 经验复用
人做判断              人审批执行              agent 闭环执行         多 agent 协作
```

### Phase 0: CLI 工具 ✅

解析多厂商日志 → SQLite 存储 → CLI 查询 / 搜索 / 拓扑分析 / LLM 增强。

- 四厂商解析（华为 VRP、思科 IOS、华三 Comware、Juniper JUNOS）
- 控制平面 + 数据平面（RIB / FIB / LFIB，MPLS / LDP / RSVP / SR）
- 内存图引擎（拓扑分析、路径追踪、影响评估、SPOF 检测）
- FTS5 全文搜索、配置变更追踪、排障笔记
- LLM 增强（可选）：AI 排障建议、配置解读

### Phase 1: 智能 CLI ✅ 完成

让 CLI 不只是"展示数据"，而是"理解数据并生成可操作的方案"。

| 特性 | 状态 | 说明 |
|------|------|------|
| `plan isolate` v2 | ✅ | 多维度拓扑发现（BGP peers + 接口 + 配置 + VRF），按 peer group 分步隔离，每步有检查点 |
| 增量采集修复 | ✅ | 命令边界回溯，不再截断大配置 |
| 多 session 并发 watch | ✅ | Per-file mutex，多台设备同时 log 互不干扰 |
| 数据时效覆盖 | ✅ | UPSERT 去重 + capture-time 条件更新 + config hash 去重 |
| `plan upgrade` | ✅ | 隔离 → 升级（含厂商命令）→ 验证 → 恢复，8 阶段全流程 |
| `plan cutover` | ✅ | 链路割接：配置新口 → 切流量 → 关旧口，7 阶段 |
| OSPF/ISIS 隔离支持 | ✅ | ISIS set-overload + OSPF stub-router + LDP per-interface disable |

### Phase 2: Agent Loop ✅ 完成

从"人执行 CLI"变成"agent 自己调用 CLI"。人只描述需求，agent loop 驱动探索。

| 特性 | 状态 | 说明 |
|------|------|------|
| MCP Server | ✅ | `nethelper mcp serve` — 20 个 MCP tools，Claude Code 直接调用 |
| Agent Loop 核心 | ✅ | `nethelper agent chat` — LLM tool calling + 交互式 REPL |
| 排障对话 | ✅ | agent chat 多轮对话，自动调用工具查数据验证假设 |
| 经验归档 | ✅ | agent 排障结束后主动提议归档（system prompt 引导） |
| 经验复用 | ✅ | agent 收到问题后自动先搜历史经验（system prompt 引导） |

```
用户: "LC-01 要做板卡更换，帮我准备变更方案"

Agent Loop:
  → nethelper show device LC-01              # 了解设备
  → nethelper show interface --device LC-01  # 看接口
  → nethelper plan isolate LC-01             # 生成方案
  → 发现数据不够 → 提示用户采集更多信息
  → 用户采集后 → nethelper plan isolate LC-01 --check
  → 输出完整方案 + 预检查结果
```

| 特性 | 说明 |
|------|------|
| Agent Loop 核心 | LLM 决策 + tool calling（nethelper CLI 作为 tools） |
| 排障对话 | 和 agent 聊排障 case，agent 调用 nethelper 查数据验证假设 |
| 经验归档 | 排障聊透彻后，自动提取 → `note add` 结构化存储（症状、命令、发现、结论） |
| 经验复用 | 下次遇到类似问题，先 `search log` 找历史经验再推理 |
| MCP Server | nethelper 作为 MCP tool provider，可被 Claude Code / 其他 agent 调用 |

### Phase 3: Network Agent（OpenClaw 形态）✅ 完成

从单机 agent 变成可联网、可协作、有记忆的网络运维数字同事。

| 特性 | 状态 | 说明 |
|------|------|------|
| 长期记忆 | ✅ | 向量 embedding + 余弦搜索，跨会话记忆自动注入 |
| IM 通讯 | ✅ | 飞书/Discord/Telegram/微信/QQ 五平台，Channel 接口 + 权限分组 |
| 心跳巡检 | ✅ | cron 定时巡检 + JSONL 日志 + 可选 IM 告警推送 |
| 多 Agent 协作 | ✅ | MCP Server 可被任意 agent 调用 |
| Soul/Identity | ✅ | SOUL.md / IDENTITY.md / TOOLS.md 配置文件驱动 |
| 外挂知识库 | ✅ | ~/.nethelper/knowledge/*.md 自动 embedding + 语义搜索 |
| 被调用 | 作为其他 agent 的 tool（"帮我查一下 LC-01 的 BGP 邻居"） |

### 跨阶段技术决策

**设备数据自动采集（待做）：** 当前依赖人工在终端执行命令、保存日志。后续应建立系统级通道（SSH/NETCONF/API）让 agent 自主采集设备配置和状态数据，而不是等人手动提供。这是从"辅助工具"到"自主 agent"的关键基础设施。

**多 session 并发：** Watcher 已支持多文件独立 offset 跟踪。需要增强的是同一设备来自不同文件的数据合并策略。

**数据时效覆盖：** 接口/邻居等状态数据使用 UPSERT 语义（`ON CONFLICT DO UPDATE`），自然支持覆盖。需要增强的是重复命令的变化检测与标记。

**经验固化闭环：** `diagnose`（对话排障）→ 确认根因 → `note add`（结构化归档）→ 下次 `diagnose` 先搜历史经验。CLI 阶段就能实现大部分，Agent Loop 阶段自动化这个闭环。

---

## 安装

```bash
git clone https://github.com/xavierli/nethelper.git
cd nethelper
./install.sh
```

安装向导会引导你设置数据目录、监控目录和 LLM 配置。

**前置条件：** Go 1.22+

## 快速开始

```bash
# 1. 导入一个设备日志文件
nethelper watch ingest ~/network-logs/core-switch.log

# 2. 查看解析到的设备
nethelper show device

# 3. 查看路由表
nethelper show route --device core-sw01

# 4. 启动自动监控（终端回显保存到监控目录即自动解析）
nethelper watch start

# 5. 生成设备隔离变更方案
nethelper plan isolate core-sw01
```

## 命令一览

```
nethelper
├── show        查询网络数据
│   ├── device      设备信息
│   ├── interface   接口信息
│   ├── route       路由表 (RIB)
│   ├── fib         转发表 (FIB)
│   ├── label       标签表 (LFIB)
│   ├── neighbor    协议邻居 (OSPF/BGP/ISIS/LDP/RSVP)
│   ├── tunnel      TE/SR 隧道
│   └── topology    拓扑概览
│
├── watch       日志监控
│   ├── ingest      手动导入日志文件
│   ├── start       启动实时监控
│   ├── stop        停止监控
│   └── status      监控状态
│
├── plan        变更方案生成
│   ├── isolate     设备隔离方案（多维度拓扑发现 + 按 peer group 分步 + 检查点）
│   ├── upgrade     设备升级方案（隔离 + 升级 + 验证 + 恢复，--version + --file）
│   └── cutover     链路割接方案（配置新口 + 切流量 + 关旧口，--old + --new）
│
├── trace       路径分析
│   ├── path        端到端路径追踪 (--from A --to B)
│   └── impact      故障影响范围 (--node X)
│
├── diff        变更对比
│   ├── config      配置差异
│   └── route       路由表差异
│
├── search      全文搜索
│   ├── config      搜索配置内容
│   ├── log         搜索排障记录
│   └── command     搜索命令手册
│
├── note        排障笔记
│   ├── add         记录排障经验
│   ├── list        列出笔记
│   ├── search      搜索笔记
│   └── extract     AI 从日志提取经验 (需 LLM)
│
├── check       健康检查
│   ├── loop        环路检测
│   └── spof        单点故障检测
│
├── diagnose    AI 排障建议 (需 LLM)
├── explain     AI 配置解读 (需 LLM)
│
├── config      配置管理
│   └── llm         查看 LLM 配置
│
└── export      导出
    ├── db          备份数据库
    ├── topology    导出拓扑 (DOT/JSON)
    └── report      生成网络报告 (Markdown)
```

## 工作原理

```
终端日志文件 → Watcher 监控 → Parser 解析 → SQLite 存储 → CLI 查询
                                                ↑
                                          图引擎 + LLM（增强）
```

1. 你在终端上操作设备，终端软件（SecureCRT、iTerm2 等）把回显保存到日志文件
2. nethelper 监控日志目录，检测到新内容自动解析
3. 解析器识别厂商和命令类型，提取结构化数据（设备、接口、路由、邻居、标签等）
4. 数据存入 SQLite，同时构建内存图用于拓扑分析
5. 你通过 CLI 查询、搜索、对比、分析

## 支持的命令输出

| 厂商 | 解析支持 |
|------|---------|
| **华为** | `display interface brief`, `display ip routing-table`, `display ospf peer`, `display mpls ldp session`, `display mpls lsp` |
| **思科** | `show ip interface brief`, `show ip route`, `show ip ospf neighbor`, `show mpls forwarding-table` |
| **华三** | `display interface brief`, `display ip routing-table`, `display ospf peer` |
| **Juniper** | `show interfaces terse`, `show route`, `show ospf neighbor` |

未识别的命令输出会保存原文并建立 FTS5 索引，不丢失数据。

## LLM 配置（可选）

LLM 是可选的增强功能。不配置也不影响核心功能。

```yaml
# ~/.nethelper/config.yaml
llm:
  default: ollama
  providers:
    ollama:
      base_url: http://localhost:11434
      model: qwen2.5:14b
```

支持任何 OpenAI 兼容 API：Ollama、OpenAI、DeepSeek、通义千问、Kimi 等。

## 数据存储

所有数据在一个 SQLite 文件中：`~/.nethelper/nethelper.db`

```bash
# 备份
nethelper export db -o backup.db

# 迁移到另一台机器
scp ~/.nethelper/nethelper.db user@newhost:~/.nethelper/
scp ~/.nethelper/config.yaml user@newhost:~/.nethelper/
```

## 项目结构

```
internal/
├── cli/          CLI 命令 (Cobra)
├── config/       配置加载 (YAML)
├── diff/         文本差异引擎
├── graph/        内存图引擎 + 分析算法
├── llm/          LLM Provider 抽象层
├── model/        统一数据模型
├── parser/       多厂商解析器
│   ├── huawei/
│   ├── cisco/
│   ├── h3c/
│   └── juniper/
├── plan/         变更方案引擎（拓扑发现 + 命令生成）
├── store/        SQLite 存储层 + FTS5
└── watcher/      fsnotify 文件监控
```

## 许可证

MIT
