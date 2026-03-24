# nethelper 配置完整参考

## 概览

nethelper 的配置分为两层：

| 层次 | 文件 | 用途 |
|------|------|------|
| 结构化配置 | `~/.nethelper/config.yaml` | 连接凭证、路由策略、权限组、功能开关 |
| 人格配置 | `~/.nethelper/SOUL.md` / `IDENTITY.md` / `TOOLS.md` | Agent 行为风格，Markdown 自由编写 |
| 知识库 | `~/.nethelper/knowledge/*.md` | 外挂业务知识，Markdown 格式 |

配置文件默认路径：`~/.nethelper/config.yaml`。可用 `--config` 参数指定其他路径：

```bash
nethelper --config /etc/nethelper/config.yaml show device
```

---

## 快速开始

```bash
mkdir -p ~/.nethelper
cat > ~/.nethelper/config.yaml << 'EOF'
data_dir: ~/.nethelper
watch_dirs:
  - ~/network-logs

llm:
  default: ollama
  providers:
    ollama:
      base_url: http://localhost:11434
      model: qwen2.5:14b
EOF
```

---

## 完整配置示例

```yaml
# ~/.nethelper/config.yaml

# ───────────── 数据存储 ─────────────
data_dir: ~/.nethelper
db_path: ~/.nethelper/nethelper.db

# ───────────── 日志监控目录 ─────────────
watch_dirs:
  - ~/Work/session_log/netdevice

# ───────────── LLM 配置 ─────────────
llm:
  default: anthropic
  providers:
    ollama:
      base_url: http://localhost:11434
      model: qwen2.5:14b
    anthropic:
      api_key: sk-ant-xxxxxxxx
      model: claude-3-5-sonnet-20241022
    deepseek:
      api_key: sk-xxxxxxxx
      model: deepseek-chat
      base_url: https://api.deepseek.com/v1
  routing:
    extract: ollama
    analyze: anthropic
    explain: ollama

# ───────────── Embedding 向量模型 ─────────────
embedding:
  provider: ollama
  providers:
    ollama:
      base_url: http://localhost:11434
      model: qwen3-embedding

# ───────────── IM 接入 ─────────────
channels:
  feishu:
    app_id: cli_xxxxxxxxxxxxxxxx
    app_secret: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
    enabled: true
  discord:
    token: MTxxxxxxx.xxxxxx.xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
    enabled: false
  telegram:
    token: 1234567890:AAxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
    enabled: false
  wechat:
    bridge_url: http://localhost:9000
    token: your-wechat-token
    enabled: false
  qq:
    ws_url: ws://localhost:6700
    enabled: false

# ───────────── 权限分组 ─────────────
permissions:
  groups:
    admin:
      users:
        - feishu:ou_xxxxxxxxxxxxxxxxxxxxxxxx
      tools:
        - "*"
    operator:
      users:
        - feishu:ou_yyyyyyyyyyyyyyyyyyyyyyyy
        - discord:123456789012345678
      tools:
        - "show_*"
        - "search_*"
        - "plan_*"
        - "note_add"
    viewer:
      users:
        - "*"
      tools:
        - "show_*"
        - "search_*"

# ───────────── 心跳巡检 ─────────────
heartbeat:
  enabled: true
  interval: 30m
  prompt: "检查所有设备的网络拓扑状态，查找单点故障(SPOF)和异常。如有变化或异常，给出简要报告。如果一切正常，只需说'巡检正常，无异常'。"
  channel: feishu
  chat_id: oc_xxxxxxxxxxxxxxxxxxxxxxxx

# ───────────── Context 压缩 ─────────────
context:
  max_token_budget: 50000
  tool_result_max_len: 2000
  enable_summary: false

# ───────────── 外挂知识库 ─────────────
knowledge:
  sources:
    - type: local
      name: local-kb
      path: ~/.nethelper/knowledge
      enabled: true
    - type: http
      name: iwiki
      url: https://iwiki.example.com/api/search
      token: Bearer_xxxxxxxx
      enabled: false
```

---

## 配置项详解

### data_dir / db_path

```yaml
data_dir: ~/.nethelper    # Agent 配置文件（SOUL.md 等）和会话日志的根目录
db_path: ~/.nethelper/nethelper.db    # SQLite 数据库文件路径
```

- `data_dir` 下会自动创建 `SOUL.md`、`IDENTITY.md`、`TOOLS.md`、`knowledge/`、`sessions/` 等子目录
- `db_path` 单独配置，允许数据库放在不同位置（如挂载的 NAS）
- 数据迁移只需复制 `nethelper.db` 和 `config.yaml` 两个文件

### watch_dirs

```yaml
watch_dirs:
  - ~/network-logs/huawei
  - ~/network-logs/cisco
  - /var/log/network
```

`nethelper watch start` 监控这些目录。每个目录中的 `.log`、`.txt` 文件被扫描为终端日志。

也可通过 `--dir` 参数临时指定（不修改 config.yaml）：

```bash
nethelper watch start --dir ~/tmp-logs
```

---

### llm（LLM 配置）

LLM 是**可选的增强功能**。不配置 LLM，日志解析、拓扑分析、FTS5 搜索等核心功能完全正常。配置 LLM 后才可使用 `agent chat`、`mcp serve`、`channel start`、`heartbeat start`、`diagnose`、`explain`、`note extract`。

#### providers（提供者）

每个 provider 支持以下字段：

| 字段 | 说明 |
|------|------|
| `api_key` | API 密钥 |
| `model` | 模型名称 |
| `base_url` | API 端点（OpenAI 兼容格式）|

**协议自动识别：** provider 名称或 `base_url` 包含 `anthropic` 或 `kimi` 时，自动使用 Anthropic 协议（`POST /v1/messages` + `x-api-key`）；其他均走 OpenAI 协议（`POST /v1/chat/completions` + `Authorization: Bearer`）。

##### Ollama（本地，推荐入门）

```yaml
llm:
  default: ollama
  providers:
    ollama:
      base_url: http://localhost:11434
      model: qwen2.5:14b
```

推荐模型：`qwen2.5:14b`（中英文均衡）、`qwen2.5:7b`（轻量）。

先安装并拉取模型：

```bash
brew install ollama      # macOS
ollama pull qwen2.5:14b
ollama serve             # 启动服务（默认 11434 端口）
```

##### OpenAI

```yaml
llm:
  default: openai
  providers:
    openai:
      api_key: sk-proj-xxxxxxxxxxxx
      model: gpt-4o-mini
```

##### Anthropic（Claude）

```yaml
llm:
  default: anthropic
  providers:
    anthropic:
      api_key: sk-ant-xxxxxxxxxxxx
      model: claude-3-5-sonnet-20241022
```

##### DeepSeek

```yaml
llm:
  default: deepseek
  providers:
    deepseek:
      api_key: sk-xxxxxxxxxxxx
      model: deepseek-chat
      base_url: https://api.deepseek.com/v1
```

##### 通义千问（阿里云）

```yaml
llm:
  default: qwen
  providers:
    qwen:
      api_key: sk-xxxxxxxxxxxx
      model: qwen-plus
      base_url: https://dashscope.aliyuncs.com/compatible-mode/v1
```

##### Kimi（Moonshot）

```yaml
llm:
  default: kimi
  providers:
    kimi:
      api_key: sk-xxxxxxxxxxxx
      model: moonshot-v1-8k
      base_url: https://api.moonshot.cn/v1
```

> **注意：** base_url 含 `kimi` 时自动使用 Anthropic 协议。若使用标准 OpenAI 协议的 Moonshot，将 `base_url` 改为不含 kimi 的 URL。

#### routing（能力路由）

不同任务可以路由到不同 provider，实现成本与质量的平衡：

```yaml
llm:
  default: ollama
  providers:
    ollama:
      base_url: http://localhost:11434
      model: qwen2.5:14b
    openai:
      api_key: sk-proj-xxxxxxxxxxxx
      model: gpt-4o
  routing:
    extract: ollama      # 从日志提取排障经验（note extract）→ 本地省钱
    analyze: openai      # AI 排障分析（diagnose / agent chat）→ 最强模型
    explain: ollama      # 配置解读（explain）→ 本地省钱
```

四种能力标识：

| 能力 | 触发场景 |
|------|---------|
| `extract` | `nethelper note extract <file>` |
| `analyze` | `nethelper diagnose` / `agent chat` / `channel start` |
| `explain` | `nethelper explain` |
| `parse` | 解析器兜底（内部使用）|

未在 routing 中指定的能力，回落到 `default` provider。

---

### embedding（向量 Embedding）

向量记忆和知识库语义搜索依赖 embedding 模型。不配置时，向量功能被禁用（但 FTS5 关键词搜索仍然可用）。

```yaml
embedding:
  provider: ollama
  providers:
    ollama:
      base_url: http://localhost:11434
      model: qwen3-embedding     # 或 nomic-embed-text
```

拉取 embedding 模型：

```bash
ollama pull qwen3-embedding
```

配置 embedding 后，以下能力自动激活：
- `agent chat` / `channel start` 会在每次对话开始时注入相关历史记忆
- `~/.nethelper/knowledge/*.md` 文件会被自动向量化，纳入语义搜索

---

### channels（IM 接入）

每个平台都有独立的配置块。`enabled: true` 才会在 `nethelper channel start` 时启动。

#### 飞书（Feishu）

```yaml
channels:
  feishu:
    app_id: cli_xxxxxxxxxxxxxxxx
    app_secret: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
    enabled: true
```

| 字段 | 说明 |
|------|------|
| `app_id` | 飞书开放平台应用的 App ID |
| `app_secret` | 飞书开放平台应用的 App Secret |
| `enabled` | 是否启动时连接 |

飞书应用创建详见 [agent-guide.md](./agent-guide.md#飞书-feishu)。

#### Discord

```yaml
channels:
  discord:
    token: MTxxxxxxx.xxxxxx.xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
    enabled: false
```

| 字段 | 说明 |
|------|------|
| `token` | Discord Bot Token（从 Discord Developer Portal 获取）|
| `enabled` | 是否启动时连接 |

#### Telegram

```yaml
channels:
  telegram:
    token: 1234567890:AAxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
    enabled: false
```

| 字段 | 说明 |
|------|------|
| `token` | Telegram Bot Token（从 BotFather 获取）|
| `enabled` | 是否启动时连接 |

#### 微信（WeChat）

微信需要桥接服务（第三方 WeChat HTTP Bridge）：

```yaml
channels:
  wechat:
    bridge_url: http://localhost:9000
    token: your-wechat-token
    enabled: false
```

| 字段 | 说明 |
|------|------|
| `bridge_url` | 微信桥接服务的 HTTP 地址 |
| `token` | 桥接服务的鉴权 token |
| `enabled` | 是否启动时连接 |

#### QQ

QQ 需要 go-cqhttp 等 OneBot 协议实现：

```yaml
channels:
  qq:
    ws_url: ws://localhost:6700
    enabled: false
```

| 字段 | 说明 |
|------|------|
| `ws_url` | OneBot WebSocket 服务地址（go-cqhttp 默认 ws://localhost:6700）|
| `enabled` | 是否启动时连接 |

---

### permissions（权限分组）

权限分组控制哪些用户可以让 bot 调用哪些工具。适用于 `channel start` 场景。

```yaml
permissions:
  groups:
    admin:
      users:
        - feishu:ou_xxxxxxxxxxxxxxxxxxxxxxxx   # 飞书 open_id
        - discord:123456789012345678            # Discord user ID
      tools:
        - "*"                                  # 允许所有工具
    operator:
      users:
        - feishu:ou_yyyyyyyyyyyyyyyyyyyyyyyy
      tools:
        - "show_*"        # 通配符：show_devices, show_device, show_interfaces...
        - "search_*"
        - "plan_*"
        - "note_add"
    viewer:
      users:
        - "*"              # 通配符：未匹配到其他组的所有用户
      tools:
        - "show_*"
        - "search_*"
```

**用户标识格式：** `<platform>:<platform_user_id>`

| 平台 | 用户标识格式 | 示例 |
|------|------------|------|
| 飞书 | `feishu:<open_id>` | `feishu:ou_abc123` |
| Discord | `discord:<user_id>` | `discord:123456789` |
| Telegram | `telegram:<user_id>` | `telegram:987654321` |
| 微信 | `wechat:<wxid>` | `wechat:wxid_abc123` |
| QQ | `qq:<qq_number>` | `qq:12345678` |

**工具通配符规则：**
- `*` — 匹配任意工具
- `show_*` — 匹配所有以 `show_` 开头的工具
- `plan_isolate` — 精确匹配单个工具名

**不配置 permissions 时的默认行为：** 所有用户只能调用 `show_*` 和 `search_*` 工具。

---

### heartbeat（心跳巡检）

定时驱动 Agent 自主巡检网络状态，异常时向 IM 推送告警（正常时静默）。

```yaml
heartbeat:
  enabled: true
  interval: 30m          # 支持: 15m, 30m, 1h, 2h 等
  prompt: "检查所有设备的网络拓扑状态，查找单点故障(SPOF)和异常。如有变化或异常，给出简要报告。如果一切正常，只需说'巡检正常，无异常'。"
  channel: feishu        # 告警推送目标 IM（目前仅支持 feishu）
  chat_id: oc_xxxxxxxxxxxxxxxxxxxxxxxx    # 推送到的会话 ID（群 ID 或个人 chat ID）
```

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `enabled` | `false` | 是否启用 |
| `interval` | `30m` | 巡检间隔，最小 1 分钟 |
| `prompt` | 内置巡检 prompt | 每次巡检时发给 Agent 的指令 |
| `channel` | `""` | 告警推送平台，留空则不推送 |
| `chat_id` | `""` | 推送目标会话 ID，`channel` 和 `chat_id` 必须同时设置 |

**启动方式一：** `nethelper heartbeat start`（独立运行）

**启动方式二：** 在 `channel start` 时自动共启（推荐，`channel` 和 `heartbeat` 共用同一个 IM 连接）

---

### context（Context 压缩）

控制 Agent 对话历史的内存管理策略，防止长对话撑爆 LLM 上下文窗口。

```yaml
context:
  max_token_budget: 50000       # 总字符预算（~12K tokens），超出时驱逐最老的消息
  tool_result_max_len: 2000     # 单条工具返回的最大字符数，超出截断
  enable_summary: false         # 预留：滚动摘要（暂未实现）
```

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `max_token_budget` | `50000` | 所有消息的总字符数上限（以 4 字符/token 粗算约 12K tokens）|
| `tool_result_max_len` | `2000` | 单个工具结果保留的最大字符数，超出部分替换为截断提示 |
| `enable_summary` | `false` | 预留字段，当前不生效 |

**压缩策略（按顺序）：**
1. 工具结果截断：超过 `tool_result_max_len` 的工具消息，保留头部 + 尾部，中间替换为 `...(truncated)...`
2. 消息驱逐：总字符数超过 `max_token_budget` 时，从第 2 条消息起逐条删除最老的消息，直到预算满足或仅剩 system + 1 轮对话

---

### knowledge（外挂知识库）

外挂知识库让 Agent 在对话时能搜索到业务专有知识（如网络规范、操作手册、历史 case）。

```yaml
knowledge:
  sources:
    - type: local
      name: local-kb
      path: ~/.nethelper/knowledge    # 可省略，默认为 <data_dir>/knowledge
      enabled: true
    - type: http
      name: iwiki
      url: https://iwiki.example.com/api/search
      token: Bearer_xxxxxxxx
      enabled: false
    - type: http
      name: confluence
      url: https://confluence.example.com/rest/api/search
      token: Basic_base64encoded
      enabled: false
```

#### local 类型

扫描指定目录下的所有 `.md` 文件，在 Agent 启动时向量化并加载到内存。查询时通过余弦相似度搜索最相关的段落。

| 字段 | 说明 |
|------|------|
| `type` | `local` |
| `name` | 来源标识（显示在搜索结果中）|
| `path` | Markdown 文件目录，默认 `<data_dir>/knowledge` |
| `enabled` | 是否启用 |

**前提：** 必须配置 `embedding` 才能向量化本地知识库。

#### http 类型

调用外部 HTTP API 搜索知识（iWiki、Confluence、自建知识库等）。

| 字段 | 说明 |
|------|------|
| `type` | `http` |
| `name` | 来源标识 |
| `url` | API 端点（GET 请求，参数为 `q=<query>&top_k=<n>`）|
| `token` | Bearer Token（可选，放入 `Authorization` header）|
| `enabled` | 是否启用 |

HTTP 知识源的查询格式为：`GET <url>?q=<query>&top_k=<n>`，响应格式为 JSON 数组：

```json
[
  {"title": "文章标题", "content": "相关段落内容"}
]
```

---

## Markdown 配置文件

除了 `config.yaml`，以下 Markdown 文件控制 Agent 的行为风格：

### ~/.nethelper/SOUL.md

定义 Agent 的人格、价值观和行为边界。首次运行时自动创建默认文件，可自由编辑。

```markdown
# Soul

你是一个专业的网络运维助手，名叫 nethelper。你帮助网络工程师排障、生成变更方案、积累运维经验。

## 风格
- 用中文回答
- 专业但友好
- 给出具体、可操作的建议
- 不要凭空猜测——用工具查到的数据说话

## 边界
- 你只处理网络运维相关的问题
- 不要编造设备数据——如果工具没返回，就说"数据不足"
```

### ~/.nethelper/IDENTITY.md

定义 Agent 的名称、emoji 和自我介绍：

```markdown
# Identity

- 名字: nethelper
- emoji: 🔧
- 自我介绍: 我是 nethelper，你的网络运维助手。我可以查询设备、分析拓扑、生成变更方案、记录排障经验。
```

### ~/.nethelper/TOOLS.md

指导 Agent 如何使用工具、按什么顺序工作：

```markdown
# 工具使用指南

## 工作流程
1. 先搜历史经验（search_log）
2. 收集信息（show_devices, show_interfaces, show_bgp_peers）
3. 分析和行动（plan_isolate, plan_upgrade）
4. 归档经验（note_add）
```

**修改生效：** 重启 `nethelper agent chat` 或 `nethelper channel start` 即可。

---

### ~/.nethelper/knowledge/*.md

放在这个目录下的任何 `.md` 文件都会被自动加载为本地知识库：

```
~/.nethelper/knowledge/
├── bgp-runbook.md          # BGP 故障处理手册
├── ospf-design.md          # OSPF 设计规范
├── maintenance-window.md   # 变更窗口规定
└── device-inventory.md     # 设备台账说明
```

**写法建议：** 每个文件关注一个主题，使用 `##` 标题分节，内容具体（避免泛泛而谈）。Agent 通过语义搜索找到相关段落，内容越具体，相关性越准确。

---

## 数据目录完整结构

```
~/.nethelper/
├── config.yaml          # 主配置文件
├── SOUL.md              # Agent 人格定义（首次启动自动创建）
├── IDENTITY.md          # Agent 身份信息（首次启动自动创建）
├── TOOLS.md             # 工具使用指南（首次启动自动创建）
├── nethelper.db         # SQLite 数据库（所有解析数据 + 记忆 + 对话历史）
├── knowledge/           # 外挂知识库 Markdown 文件
│   └── *.md
└── sessions/            # JSONL 审计日志（每用户每天一个文件）
    ├── repl_2026-03-24.jsonl
    └── feishu_ou_xxx_2026-03-24.jsonl
```

---

## 常用操作

```bash
# 查看当前 LLM 配置
nethelper config llm

# 备份数据库
nethelper export db -o backup-$(date +%Y%m%d).db

# 迁移到另一台机器
scp ~/.nethelper/nethelper.db  newhost:~/.nethelper/
scp ~/.nethelper/config.yaml   newhost:~/.nethelper/

# 保护 config.yaml（含 API Key）
chmod 600 ~/.nethelper/config.yaml
```
