# nethelper Agent 使用指南

本文档介绍 nethelper 的 Agent 相关功能：终端对话（`agent chat`）、MCP Server（`mcp serve`）、IM 频道接入（`channel start`）、心跳巡检（`heartbeat start`）、向量记忆和知识库。

## 前置条件

所有 Agent 功能都需要 LLM 配置。请先确保 `~/.nethelper/config.yaml` 中配置了 `llm` 段：

```bash
nethelper config llm    # 检查 LLM 配置状态
```

如果看到 `no LLM provider configured`，参考 [configuration.md](./configuration.md#llm-llm-配置) 配置 LLM。

---

## 一、nethelper agent chat — 终端 REPL

`agent chat` 启动一个交互式终端对话，你用自然语言描述需求，Agent 自动调用工具查数据、生成方案。

```bash
nethelper agent chat
```

### 示例对话

```
nethelper> LC-01 要做板卡更换，帮我准备变更方案

  [tool] show_devices()
  [tool] show_device(device_id=LC-01)
  [tool] show_interfaces(device_id=LC-01)
  [tool] show_bgp_peers(device_id=LC-01)
  [tool] plan_isolate(device_id=LC-01)

## 🔧 LC-01 设备隔离方案

### 第一步：IGP 隔离
...（完整方案）...
```

### REPL 内置命令

| 命令 | 说明 |
|------|------|
| `exit` / `quit` | 退出（自动保存本次对话记忆） |
| `/reset` | 清空对话历史，保留 system prompt，开始新话题 |

### 工具调用显示

每次 Agent 调用工具时，终端会实时显示：

```
  [tool] show_bgp_peers(device_id=LC-01)
```

格式为 `[tool] <工具名>(<参数>)`。

### 对话自动保存

退出时（`exit` 或 Ctrl+D），Agent 会：
1. 用 LLM 总结本次对话要点
2. 向量化摘要并存入 `memory_entries` 表
3. 下次对话开始时，相关记忆自动注入到上下文中

---

## 二、nethelper mcp serve — MCP Server

`mcp serve` 以 stdio 模式启动 MCP Server，让 Claude Code 或其他支持 MCP 的 AI 工具直接调用 nethelper 的 20 个工具。

```bash
nethelper mcp serve
```

### 在 Claude Code 中配置

在项目根目录创建 `.mcp.json`，或在 Claude Code 设置文件（`~/.claude/settings.json`）中添加：

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

配置后重启 Claude Code，即可在对话中直接使用 nethelper 的工具（工具列表见下方）。

### MCP 工具列表

| 工具名 | 说明 |
|--------|------|
| `show_devices` | 列出所有设备 |
| `show_device` | 查询单个设备详情 |
| `show_interfaces` | 查询设备接口列表 |
| `show_neighbors` | 查询 LLDP/CDP 邻居 |
| `show_bgp_peers` | 查询 BGP peer 组摘要（或指定组的详细列表）|
| `show_topology` | 查询网络拓扑（含 SPOF 状态）|
| `show_routes` | 查询路由表（最近 10 条）|
| `check_loop` | 检测路由环路 |
| `check_spof` | 检测单点故障 |
| `trace_path` | 端到端路径追踪 |
| `trace_impact` | 模拟设备故障的影响范围 |
| `diff_config` | 配置变更 diff |
| `plan_isolate` | 生成设备隔离变更方案 |
| `plan_upgrade` | 生成设备升级变更方案 |
| `plan_cutover` | 生成链路割接变更方案 |
| `search_config` | FTS5 全文搜索配置内容 |
| `search_log` | FTS5 全文搜索排障笔记 |
| `note_add` | 记录排障经验 |
| `diagnose` | AI 排障建议 |
| `watch_ingest` | 导入日志文件 |

### 使用示例

在 Claude Code 对话框中：

```
帮我查一下 LC-01 的 BGP 邻居，看看有哪些 peer group
```

Claude Code 会自动调用 `show_bgp_peers(device_id="LC-01")` 并展示结果。

---

## 三、nethelper channel start — IM 频道接入

`channel start` 连接 IM 平台，将收到的消息转发给 Agent 处理，让团队成员直接在 IM 里与 nethelper 对话。

```bash
nethelper channel start
```

支持 `--feishu`、`--discord`、`--telegram`、`--wechat`、`--qq` 参数强制启动单个平台（忽略 config 中的 `enabled` 标志）：

```bash
nethelper channel start --feishu    # 仅启动飞书
nethelper channel start --telegram  # 仅启动 Telegram
```

收到 SIGINT（Ctrl+C）或 SIGTERM 时优雅停止。

### 飞书（Feishu）

#### 1. 创建飞书应用

1. 登录 [飞书开放平台](https://open.feishu.cn/) → 「开发者后台」→「创建应用」
2. 选择「自建应用」，填写应用名称（如 `nethelper`）
3. 记录 **App ID** 和 **App Secret**（在「凭证与基础信息」页面）

#### 2. 开启 WebSocket 长连接

1. 进入应用详情 → 「事件订阅」
2. 将「订阅方式」改为 **长连接**（WebSocket）
3. 无需配置公网回调地址，nethelper 主动建立连接

#### 3. 开通权限

在「权限管理」中开通以下权限：
- `im:message:receive_v1` — 接收消息
- `im:message` — 发送消息

#### 4. 发布应用

「版本管理与发布」→ 提交审核 → 发布（企业自建应用通常可直接发布）。

#### 5. 配置 nethelper

```yaml
channels:
  feishu:
    app_id: cli_xxxxxxxxxxxxxxxx
    app_secret: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
    enabled: true
```

#### 6. 使用

将 bot 加入飞书群：@机器人 发消息。私聊机器人直接发消息。

```
@nethelper LC-01 有告警，BGP 邻居有 down 的，帮我看一下
```

飞书频道支持**流式卡片**：收到消息后立即推送「⏳ 收到，正在思考...」卡片，每次工具调用时更新卡片内容，最终结果替换卡片（蓝色 header → 绿色 header）。

---

### Discord

#### 1. 创建 Bot

1. 前往 [Discord Developer Portal](https://discord.com/developers/applications) → New Application
2. 进入 Bot 页面 → Add Bot → 记录 **Token**
3. 在 Bot 页面开启「Message Content Intent」（需要才能读到消息内容）
4. 使用 OAuth2 URL Generator 生成邀请链接，勾选 `bot` + `Send Messages` + `Read Messages`，邀请 bot 进服务器

#### 2. 配置 nethelper

```yaml
channels:
  discord:
    token: MTxxxxxxx.xxxxxx.xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
    enabled: true
```

#### 3. 使用

在 Discord 频道中 @ bot 或发私信：

```
@nethelper 帮我查一下 LC-01 的拓扑
```

---

### Telegram

#### 1. 创建 Bot

1. 在 Telegram 中搜索 `@BotFather`，发送 `/newbot`
2. 按提示设置 bot 名称和用户名，记录 **Bot Token**（格式：`1234567890:AAxxxxx`）
3. 可选：通过 `@BotFather` 设置 bot 命令列表（`/setcommands`）

#### 2. 配置 nethelper

```yaml
channels:
  telegram:
    token: 1234567890:AAxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
    enabled: true
```

#### 3. 使用

直接私聊 bot，或将 bot 加入群组后发消息（需要 @ 机器人）：

```
帮我生成 LC-01 的隔离方案
```

Telegram 使用**长轮询**（long polling）接收消息，无需公网地址。

---

### 微信（WeChat）

微信没有开放的 bot API，需要借助第三方 WeChat HTTP Bridge（如 WeChatPadLocal、Gewechat 等）。

#### 前提

部署一个 WeChat HTTP Bridge 服务，监听 HTTP 端口（例如 `http://localhost:9000`），具体部署方式参考所用 bridge 的文档。

#### 配置 nethelper

```yaml
channels:
  wechat:
    bridge_url: http://localhost:9000
    token: your-bridge-token
    enabled: true
```

nethelper 连接桥接服务后，即可通过微信收发消息。

---

### QQ

QQ 使用 [go-cqhttp](https://github.com/Mrs4s/go-cqhttp) 或其他 OneBot v11 协议实现。

#### 1. 部署 go-cqhttp

```bash
# 下载 go-cqhttp
wget https://github.com/Mrs4s/go-cqhttp/releases/latest/download/go-cqhttp_linux_amd64.tar.gz
tar -xf go-cqhttp_linux_amd64.tar.gz

# 首次运行生成配置
./go-cqhttp
# 选择 ws-reverse 模式（反向 WebSocket）
```

在 go-cqhttp 的 `config.yml` 中配置正向 WS：

```yaml
servers:
  - ws:
      host: 0.0.0.0
      port: 6700
```

#### 2. 配置 nethelper

```yaml
channels:
  qq:
    ws_url: ws://localhost:6700
    enabled: true
```

---

### 群组消息处理

在群组中，bot **只响应 @ 自己的消息**（`MentionedBot: true`）。私聊消息全部响应。

### 权限控制

所有 IM 请求都经过权限检查。用户的权限由 `permissions.groups` 配置决定：

```yaml
permissions:
  groups:
    admin:
      users: ["feishu:ou_abc123"]
      tools: ["*"]
    viewer:
      users: ["*"]
      tools: ["show_*", "search_*"]
```

没有匹配到任何权限组的用户会收到：「⚠️ 你没有权限使用此 bot。」

---

## 四、nethelper heartbeat start — 心跳巡检

`heartbeat start` 按固定间隔自动运行一个 Agent，检查网络状态，异常时向 IM 推送告警。

```bash
nethelper heartbeat start
```

**前提：** `config.yaml` 中 `heartbeat.enabled: true`。

### 配置示例

```yaml
heartbeat:
  enabled: true
  interval: 30m
  prompt: "检查所有设备的网络拓扑状态，查找单点故障(SPOF)和异常。如有变化或异常，给出简要报告。如果一切正常，只需说'巡检正常，无异常'。"
  channel: feishu
  chat_id: oc_xxxxxxxxxxxxxxxxxxxxxxxx
```

### 告警推送逻辑

- Agent 每隔 `interval` 运行一次，执行 `prompt` 中的巡检指令
- Agent 判断网络**正常**时，**不发送**任何消息（静默）
- Agent 发现**异常**时，调用告警函数向 `chat_id` 发送告警消息
- 每次巡检结果写入 JSONL 审计日志（`~/.nethelper/sessions/heartbeat_YYYY-MM-DD.jsonl`）

### 与 channel start 联动

如果同时运行 `channel start` 和 `heartbeat start`，两者共用同一个 IM 连接（如飞书），不需要分别建立连接。实际上，`channel start` 在检测到 `heartbeat.enabled: true` 时会自动启动心跳巡检：

```bash
# 一条命令同时启动 IM 接入 + 心跳巡检
nethelper channel start
```

独立运行心跳（不需要 IM 接入时）：

```bash
nethelper heartbeat start
```

---

## 五、向量记忆

向量记忆让 Agent 在跨会话中保持「记忆」，不会每次都从零开始。

### 工作原理

```
第一次对话
用户: "LC-01 的 BGP neighbor down 了，原因是 MTU 不一致"
...（排障过程）...
用户: exit
  → Agent 用 LLM 总结对话要点
  → 向量化摘要
  → 存入 SQLite memory_entries 表

第二次对话（一周后）
用户: "LC-02 的 BGP 邻居也 down 了"
  → Agent 向量化这句话
  → 搜索 memory_entries，找到 LC-01 案例（余弦相似度 > 0.3）
  → 自动注入到 system context：
    ## 相关历史记忆
    - [2026-03-17] LC-01 BGP neighbor down 原因是 MTU 不一致，通过修改接口 MTU 解决
  → Agent 优先参考历史经验
```

### 前提条件

必须配置 `embedding` 才能使用向量记忆：

```yaml
embedding:
  provider: ollama
  providers:
    ollama:
      base_url: http://localhost:11434
      model: qwen3-embedding
```

### 记忆注入时机

每次对话的**第一条消息**时注入（一个会话只注入一次，避免重复注入干扰）。

### 查看存储的记忆

```bash
sqlite3 ~/.nethelper/nethelper.db "
SELECT id, source, content, created_at
FROM memory_entries
ORDER BY created_at DESC
LIMIT 20;"
```

### 记忆保存触发

- `agent chat`：用户输入 `exit` 或 `quit` 时自动保存
- `channel start`：用户会话过期（30 分钟不活跃）时自动保存

---

## 六、知识库

知识库是外挂的业务专有知识，Agent 在每次回答时会语义搜索知识库，找到相关段落后注入到上下文。

### 本地知识库（推荐）

将 `.md` 文件放入 `~/.nethelper/knowledge/` 目录即可：

```bash
# 创建目录（首次启动时自动创建）
mkdir -p ~/.nethelper/knowledge

# 添加知识文件
cat > ~/.nethelper/knowledge/bgp-runbook.md << 'EOF'
# BGP 故障处理手册

## BGP 邻居 Down

### 症状
BGP neighbor state 变为 Idle 或 Active。

### 排查步骤
1. 检查路由可达性：`ping <peer_ip> source <local_ip>`
2. 检查接口 MTU 一致性：`display interface <if>`
3. 检查 AS number 是否匹配：`display bgp peer`
4. 查看 BGP Error Notify：`display bgp error`

### 常见原因
- MTU 不一致（最常见）：确保两端 MTU 相同
- Authentication mismatch：核对 md5 密码
- AS 号配错：核对 peer remote-as
EOF
```

**注意：** 本地知识库依赖 `embedding` 配置。如果未配置 embedding，知识库文件不会被加载。

### HTTP 知识库

适合接入企业内网知识平台（iWiki、Confluence、自建 API 等）：

```yaml
knowledge:
  sources:
    - type: http
      name: iwiki
      url: https://iwiki.example.com/api/search
      token: Bearer_your_token_here
      enabled: true
```

nethelper 向该 URL 发送 `GET ?q=<query>&top_k=3` 请求，期望返回：

```json
[
  {"title": "BGP MTU 问题排查", "content": "...相关内容..."},
  {"title": "BGP 认证配置", "content": "...相关内容..."}
]
```

### 知识来源标识

搜索结果中会显示来源标识，便于追溯：

```
## 相关知识库
**[local:bgp-runbook.md]** BGP 故障处理手册
MTU 不一致（最常见）：确保两端 MTU 相同...

**[iwiki:page-1234]** BGP 邻居 Down 排查指南
...
```

---

## 七、JSONL 会话日志

所有 Agent 对话（无论是 `agent chat` 还是 `channel start`）都会生成 JSONL 审计日志，存放在 `~/.nethelper/sessions/` 目录。

### 文件命名

```
<sanitized_user_key>_<date>.jsonl
```

例如：
- `repl_2026-03-24.jsonl` — `agent chat` 的日志（user key 为 `repl`）
- `feishu_ou_abc123_2026-03-24.jsonl` — 飞书用户 `ou_abc123` 的日志
- `heartbeat_2026-03-24.jsonl` — 心跳巡检的日志

### 日志格式

每行一个 JSON 对象：

```jsonl
{"ts":"2026-03-24T10:00:01Z","user":"repl","type":"user","content":"LC-01 BGP 邻居有告警"}
{"ts":"2026-03-24T10:00:02Z","user":"repl","type":"tool_call","tool":"show_bgp_peers","args":{"device_id":"LC-01"}}
{"ts":"2026-03-24T10:00:03Z","user":"repl","type":"tool_result","tool":"show_bgp_peers","content":"[{...}]","duration_ms":42}
{"ts":"2026-03-24T10:00:05Z","user":"repl","type":"assistant","content":"检查了 LC-01 的 BGP peers..."}
{"ts":"2026-03-24T10:00:01Z","user":"repl","type":"memory","content":"## 相关历史记忆\n..."}
```

事件类型：

| type | 说明 |
|------|------|
| `user` | 用户消息 |
| `assistant` | Agent 回复 |
| `tool_call` | 工具调用（含参数）|
| `tool_result` | 工具返回（含执行耗时）|
| `memory` | 注入的历史记忆 |
| `error` | 错误信息 |

### 查看日志

```bash
# 查看今天的 agent chat 日志
cat ~/.nethelper/sessions/repl_$(date +%Y-%m-%d).jsonl | jq .

# 查看工具调用耗时
cat ~/.nethelper/sessions/repl_*.jsonl | jq 'select(.type=="tool_result") | {tool, duration_ms}'

# 统计今天调用了哪些工具
cat ~/.nethelper/sessions/repl_$(date +%Y-%m-%d).jsonl | jq 'select(.type=="tool_call") | .tool' | sort | uniq -c
```
