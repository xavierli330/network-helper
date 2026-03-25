# nethelper

网络工程师的智能运维工具 —— 从日志解析到 Agent 闭环。

```
终端日志 → 自动解析 → 拓扑理解 → 变更方案生成 → Agent 自主排障
```

## 特性一览

- 🔍 **四厂商解析** — 华为 VRP、思科 IOS-XR、华三 Comware、Juniper JUNOS
- 📊 **拓扑引擎** — 配置推断互联关系，SPOF 检测，路径追踪，影响分析
- 📋 **变更方案** — 设备隔离 / 软件升级 / 链路割接，精确到每个 BGP peer
- 🤖 **Agent Loop** — LLM tool calling，多轮排障对话，经验自动归档/复用
- 🔌 **MCP Server** — 20 个 tools，Claude Code / 其他 agent 直接调用
- 💬 **IM 接入** — 飞书 / Discord / Telegram / 微信 / QQ，权限分组
- 🧠 **向量记忆** — 跨会话长期记忆 + 外挂知识库（本地 .md / HTTP API）
- ⏰ **心跳巡检** — 定时自动检查网络状态，异常推送 IM 告警
- 🎭 **可配置人格** — SOUL.md / IDENTITY.md / TOOLS.md 驱动 agent 行为

## 安装

```bash
git clone https://github.com/xavierli/nethelper.git
cd nethelper
go build -o nethelper ./cmd/nethelper
```

**前置条件：** Go 1.22+

## 快速开始

```bash
# 1. 导入设备日志
nethelper watch ingest ~/network-logs/session.log

# 2. 查看设备
nethelper show device

# 3. 生成隔离方案
nethelper plan isolate qcdr-01

# 4. 启动 AI 助手对话
nethelper agent chat

# 5. 启动飞书 IM bot
nethelper channel start --feishu

# 6. 在 Claude Code 中使用（配置 .mcp.json 后自动可用）
# "帮我查一下 LC-01 的 BGP 邻居"
```

## 命令一览

```
nethelper
├── show          查询网络数据
│   ├── device        设备信息
│   ├── interface     接口信息
│   ├── route         路由表 (RIB)
│   ├── fib           转发表 (FIB)
│   ├── label         标签表 (LFIB)
│   ├── neighbor      协议邻居 (OSPF/BGP/ISIS/LDP/RSVP)
│   ├── tunnel        TE/SR 隧道
│   └── topology      拓扑概览（配置推断）
│
├── plan          变更方案生成
│   ├── isolate       设备隔离（per-peer-group 分步 + 检查点）
│   ├── upgrade       软件升级（隔离→升级→验证→恢复，8 阶段）
│   └── cutover       链路割接（配新口→切流量→关旧口，7 阶段）
│
├── agent         AI 助手
│   └── chat          交互式排障对话（LLM + tool calling）
│
├── channel       IM 接入
│   └── start         启动 IM bot（--feishu / --discord / --telegram / --wechat / --qq）
│
├── heartbeat     定时巡检
│   └── start         启动心跳巡检（cron + 日志 + 告警）
│
├── mcp           MCP Server
│   └── serve         启动 MCP server（stdio，供 Claude Code 等调用）
│
├── watch         日志监控
│   ├── ingest        手动导入日志文件
│   ├── start         启动实时监控
│   ├── stop          停止监控
│   └── status        监控状态
│
├── trace         路径分析
│   ├── path          端到端路径追踪
│   └── impact        故障影响范围
│
├── diff          变更对比
│   ├── config        配置差异
│   └── route         路由表差异
│
├── search        全文搜索
│   ├── config        搜索配置
│   ├── log           搜索排障记录
│   └── command       搜索命令手册
│
├── knowledge     知识库查询（无 LLM，可追溯）
│   ├── search        直接搜索知识源
│   └── sources       列出配置的知识源
│
├── check         健康检查
│   ├── loop          环路检测
│   └── spof          单点故障检测
│
├── note          排障笔记
│   ├── add           记录经验
│   └── search        搜索笔记
│
├── diagnose      AI 排障建议
├── explain       AI 配置解读
│
└── export        数据导出
    ├── db            备份数据库
    ├── topology      导出拓扑
    └── report        生成报告
```

## 工作原理

```
                    ┌─────────────┐
                    │  IM 平台     │
                    │ 飞书/Discord │
                    └──────┬──────┘
                           │
┌──────────┐   ┌───────────┴───────────┐   ┌──────────┐
│ 终端日志  │──→│      nethelper        │←──│ Claude   │
│ 配置备份  │   │                       │   │ Code     │
└──────────┘   │  Parser → SQLite      │   │ (MCP)    │
               │  Graph Engine         │   └──────────┘
               │  Plan Generator       │
               │  Agent Loop + LLM     │
               │  Vector Memory        │
               └───────────┬───────────┘
                           │
                    ┌──────┴──────┐
                    │   SQLite    │
                    │ 设备/配置/   │
                    │ 拓扑/记忆    │
                    └─────────────┘
```

**三种使用方式：**

| 方式 | 命令 | 适用场景 |
|------|------|---------|
| **CLI** | `nethelper show device` | 网络工程师直接查询 |
| **Agent Chat** | `nethelper agent chat` | 多轮排障对话 |
| **MCP Server** | `nethelper mcp serve` | 被 Claude Code / 其他 agent 调用 |
| **IM Bot** | `nethelper channel start` | 飞书/Discord 群里 @bot 提问 |

## 支持的厂商

| 厂商 | 日志解析 | 配置备份解析 | 变更方案 |
|------|---------|-------------|---------|
| **华为 VRP** | ✅ | ✅ | ✅ BGP ignore + ISIS overload + OSPF stub-router |
| **H3C Comware** | ✅ | ✅ | ✅ 同上 |
| **Cisco IOS-XR** | ✅ | — | ✅ neighbor shutdown + max-metric |
| **Juniper JUNOS** | ✅ | — | ✅ deactivate + disable |

未识别的命令输出保存原文 + FTS5 索引，不丢失数据。

## 配置

详见 [配置指南](docs/configuration.md)。

**最小配置：**

```yaml
# ~/.nethelper/config.yaml
watch_dirs:
  - ~/Work/session_log

llm:
  default: ollama
  providers:
    ollama:
      base_url: http://localhost:11434
      model: qwen2.5:14b
```

**Agent 人格配置：**

```
~/.nethelper/
├── SOUL.md              # 人格/风格/边界
├── IDENTITY.md          # 名字/emoji
├── TOOLS.md             # 工具使用指南
└── knowledge/           # 外挂知识库 (*.md)
```

编辑这些文件即可自定义 agent 行为，无需重新编译。

## 知识库查询（可追溯、无 LLM）

对于生产环境，需要**数据可追溯、可验证**，避免 LLM 幻觉：

```bash
# 直接查询 IMA 知识库（不经过 LLM）
nethelper knowledge search "骨干网排障" --source ima

# 查询本地知识库
nethelper knowledge search "MPLS配置" --source local

# 查询所有源
nethelper knowledge search "BGP故障" --all

# 查看配置的知识源
nethelper knowledge sources
```

**特点：**
- 直接返回原始搜索结果
- 显示数据来源（IMA/本地/HTTP）
- 显示相关性分数
- 无 LLM 生成或改写

配置知识源（`~/.nethelper/config.yaml`）：
```yaml
knowledge:
  sources:
    - type: ima
      name: ima-network-kb
      enabled: true
      client_id: "your_client_id"
      api_key: "your_api_key"
      kb_id: "your_kb_id"
    - type: http
      name: company-wiki
      enabled: true
      url: "https://wiki.example.com/api"
      token: "optional_token"
```

## 文档

| 文档 | 内容 |
|------|------|
| [配置指南](docs/configuration.md) | 完整配置参考（YAML + .md 文件） |
| [Agent 使用指南](docs/agent-guide.md) | Agent Chat / MCP / IM / 心跳 / 记忆 / 知识库 |
| [扩展开发指南](docs/extension-guide.md) | 新厂商 / 新 IM / 新知识源 / 新 MCP tool / 新方案类型 |
| [常见问题](docs/FAQ.md) | Agent / IM / 记忆 / 方案 / 巡检 |
| [阶段性回顾](docs/phase-review-2026-03-24.md) | 架构决策 / 技术债务 / OpenClaw 对标 / 路线图 |

## 项目结构

```
internal/
├── agent/        Agent Loop + REPL + 工具注册 + 心跳巡检
├── channel/      IM 平台适配（飞书/Discord/Telegram/微信/QQ）
├── cli/          CLI 命令 (Cobra)
├── config/       配置加载 (YAML)
├── diff/         文本差异引擎
├── graph/        内存图引擎 + 拓扑分析算法
├── llm/          LLM Provider + Embedding + Tool Calling
├── mcp/          MCP Server + 20 个 Tool Handlers
├── memory/       向量记忆 + 知识库搜索（本地/HTTP）
├── model/        统一数据模型
├── parser/       多厂商解析器
│   ├── huawei/
│   ├── cisco/
│   ├── h3c/
│   └── juniper/
├── plan/         变更方案引擎（拓扑发现 + 命令生成）
├── store/        SQLite + FTS5 + 向量存储
└── watcher/      fsnotify 文件监控
```

## 数据存储

所有数据在一个 SQLite 文件中：`~/.nethelper/nethelper.db`

```bash
# 备份
nethelper export db -o backup.db

# 迁移
scp ~/.nethelper/ user@newhost:~/
```

## 演进路线

```
Phase 0 (✅)     Phase 1 (✅)     Phase 2 (✅)     Phase 3 (✅)     Phase 4 (规划中)
CLI 工具         智能 CLI          Agent Loop       Network Agent    自动化执行
解析/查询        变更方案生成       LLM tool calling  IM/记忆/巡检     SSH 自动采集
```

详见 [阶段性回顾](docs/phase-review-2026-03-24.md)。

## 许可证

MIT
