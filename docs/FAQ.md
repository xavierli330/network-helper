# FAQ — 常见问题

## 安装

### Q: 需要什么环境？

Go 1.22+ 即可。无需 CGo，无需额外数据库服务。编译后是单个可执行文件。

```bash
go version    # 确认 Go 版本
./install.sh  # 一键安装
```

### Q: 支持哪些操作系统？

macOS 和 Linux。Windows 未测试但理论上可行（watcher 的 signal 处理可能需要适配）。

### Q: 安装后 `nethelper` 命令找不到？

安装脚本会自动将安装目录添加到 shell 配置，但需要重新加载：

```bash
source ~/.zshrc   # zsh
source ~/.bashrc  # bash
```

或者直接打开一个新的终端窗口。

### Q: 如何卸载？

```bash
rm $(which nethelper)         # 删除可执行文件
rm -rf ~/.nethelper           # 删除数据（慎重！）
```

---

## 日志导入

### Q: 支持什么格式的日志文件？

普通文本文件，包含设备的终端回显。典型场景：SecureCRT/iTerm2/PuTTY 的会话日志。

文件中需要包含**设备提示符**（如 `<HUAWEI>`, `Router#`, `admin@MX204>`），nethelper 靠提示符识别设备和切分命令。

### Q: 一个文件里可以包含多个设备的输出吗？

可以。nethelper 按提示符自动识别不同设备，每个设备的数据独立存储。

### Q: 一个文件里可以包含多条命令的输出吗？

可以，这也是最常见的用法。一次终端会话通常执行多条命令，全部输出保存到一个文件。

### Q: 日志里有些命令解析不了怎么办？

不影响。nethelper 采用三层解析策略：
1. **L1 正则匹配** — 内置模板覆盖常用命令
2. **L2.5 LLM 兜底** — 配置了 LLM 时自动尝试
3. **L3 原文保留** — 完全无法解析的内容存原文，建 FTS5 索引，至少可以搜索到

### Q: 同一个文件导入两次会重复吗？

`watch ingest` 会记录文件的处理位置（offset），watcher 模式下只读取新增内容。但手动 `ingest` 同一文件会重新处理。

### Q: 华为和华三的提示符一样（`<hostname>`），怎么区分？

当前版本默认将 `<hostname>` 提示符的输出交给华为解析器处理。华为和华三的命令输出格式非常相似，大部分情况下不影响解析结果。后续版本将支持在配置文件中手动绑定设备与厂商的对应关系。

---

## 数据与存储

### Q: 数据存在哪里？

`~/.nethelper/nethelper.db`（SQLite 单文件）。可以通过 `config.yaml` 的 `db_path` 字段自定义。

### Q: 如何备份？

```bash
nethelper export db -o backup-$(date +%Y%m%d).db
```

或者直接复制 `.db` 文件。

### Q: 如何迁移到另一台机器？

复制两个文件即可：

```bash
scp ~/.nethelper/nethelper.db  newhost:~/.nethelper/
scp ~/.nethelper/config.yaml   newhost:~/.nethelper/
```

### Q: 数据库会越来越大吗？

会。每次日志导入都会创建快照（RIB/FIB/LFIB/邻居等）。如果只关心最新状态不需要历史，可以定期清理旧快照（当前版本暂未提供自动清理命令）。

### Q: 可以用其他工具查看 SQLite 数据吗？

当然。推荐 [DB Browser for SQLite](https://sqlitebrowser.org/) 或命令行 `sqlite3`：

```bash
sqlite3 ~/.nethelper/nethelper.db "SELECT * FROM devices;"
```

---

## Watcher 监控

### Q: `watch start` 和 `watch ingest` 有什么区别？

- `watch ingest <file>` — 一次性导入一个文件
- `watch start` — 后台持续监控目录，有新文件或文件变化时自动导入

### Q: Watcher 是后台进程吗？

`watch start` 在前台运行（Ctrl+C 停止）。如果需要后台运行：

```bash
nethelper watch start &           # 简单后台
nohup nethelper watch start &     # 不随终端关闭
```

### Q: 可以监控多个目录吗？

可以。在 `config.yaml` 中配置多个目录：

```yaml
watch_dirs:
  - ~/network-logs/huawei
  - ~/network-logs/cisco
  - /var/log/network
```

或用 `--dir` 参数：

```bash
nethelper watch start --dir ~/logs1 --dir ~/logs2
```

### Q: Watcher 会重复处理文件吗？

不会。Watcher 使用增量读取——记录每个文件的已处理位置（offset），只读取新增内容。

---

## LLM 相关

### Q: 不配置 LLM 能用吗？

完全可以。LLM 只是增强层，影响以下三个命令：
- `nethelper diagnose` — 退回到 FTS5 关键词搜索
- `nethelper explain` — 显示原文
- `nethelper note extract` — 需要手动 `note add`

所有其他功能（解析、查询、搜索、diff、trace、check、export）完全不依赖 LLM。

`agent chat`、`channel start`、`heartbeat start`、`mcp serve` 需要 LLM 配置才能使用。

### Q: 推荐什么 LLM？

| 场景 | 推荐 |
|------|------|
| 本地免费、无隐私顾虑 | Ollama + qwen2.5:14b |
| 效果最好 | Anthropic Claude / OpenAI gpt-4o |
| 性价比 | DeepSeek deepseek-chat |
| 中文优化 | 通义千问 qwen-plus |

### Q: 可以同时用多个 LLM 吗？

可以。通过能力路由，不同任务用不同模型：

```yaml
llm:
  default: ollama
  providers:
    ollama:
      base_url: http://localhost:11434
      model: qwen2.5:14b
    openai:
      api_key: sk-xxx
      model: gpt-4o
  routing:
    extract: ollama     # 提取用本地模型（省钱）
    analyze: openai     # 分析用 GPT-4o（效果好）
    explain: ollama     # 解读用本地模型
```

### Q: API Key 安全吗？

API Key 存在 `~/.nethelper/config.yaml` 中。建议：

```bash
chmod 600 ~/.nethelper/config.yaml
```

---

## 排障分析

### Q: `trace path` 的路径是怎么算出来的？

基于内存图引擎的 BFS 最短路径。图节点来自设备和接口数据，边来自协议邻居关系（OSPF/BGP/ISIS/LDP peer）和子网连接。

注意：路径分析依赖已导入的数据质量——如果只导入了部分设备的日志，图可能不完整。

### Q: `check spof` 是怎么检测单点故障的？

暴力法：逐个模拟移除每个设备节点，检查剩余网络是否仍然连通。如果移除某个设备后网络断开，该设备就是单点故障。

### Q: 能检测 MPLS 标签一致性吗？

当前版本存储了 RIB/FIB/LFIB 数据，但 `check label` 命令尚未实现。你可以通过 `show route`、`show fib`、`show label` 手动对比。

---

## 搜索

### Q: 搜索支持中文吗？

FTS5 支持中文搜索，但分词依赖 SQLite 的默认 tokenizer（unicode61），对中文的分词效果有限。建议搜索时使用关键词而非长句。

### Q: 搜索能跨厂商吗？

能。所有厂商的数据存入统一模型后，搜索不区分厂商。`search config "ospf cost"` 会匹配所有厂商的配置。

---

## 导出

### Q: DOT 格式怎么用？

DOT 是 Graphviz 的图描述语言。导出后可以生成图片：

```bash
nethelper export topology --format dot -o network.dot

# 安装 graphviz
brew install graphviz   # macOS
apt install graphviz    # Ubuntu

# 生成图片
dot -Tpng network.dot -o network.png
dot -Tsvg network.dot -o network.svg
```

### Q: 报告包含什么内容？

`nethelper export report` 生成 Markdown 格式的网络状态报告，包含：
- 设备汇总表（主机名、厂商、型号、管理 IP、Router-ID）
- 网络统计（设备数、接口数、子网数）
- 单点故障列表（如有）

---

## Agent 对话（agent chat）

### Q: "LLM not configured" 是什么错误？

需要在 `config.yaml` 中配置 `llm` 段：

```yaml
llm:
  default: ollama
  providers:
    ollama:
      base_url: http://localhost:11434
      model: qwen2.5:14b
```

先运行 `nethelper config llm` 确认配置状态。如果使用 Ollama，确保 `ollama serve` 已启动且模型已拉取（`ollama pull qwen2.5:14b`）。

### Q: Agent 调用了工具但回答不准确？

几个常见原因：

1. **数据不足：** 如果数据库中没有相关设备的数据，agent 无法凭空生成答案。先用 `nethelper watch ingest <file>` 导入设备日志。

2. **模型能力不足：** 较小的本地模型（7b 以下）可能无法有效使用 tool calling。尝试换用更大的模型或云端模型。

3. **上下文太长：** 多轮对话后 context 可能溢出。在 REPL 中输入 `/reset` 清空对话历史。

### Q: tool calling 不工作，Agent 只用文字回答不调用工具？

这通常是模型不支持 function calling/tool use。确认：

1. 你的模型支持 tool calling（Ollama：qwen2.5、llama3.1 等；云端：GPT-3.5+、Claude-3+）
2. Ollama 版本是否足够新：`ollama --version`（建议 0.5.0+）
3. 尝试换用支持 tool calling 的模型：`ollama pull qwen2.5:14b`

### Q: Agent 回答速度很慢？

本地模型推理速度受 CPU/GPU 性能影响。建议：
- 使用 GPU 加速：Ollama 自动检测 GPU，安装好驱动即可
- 换用更小的模型：`qwen2.5:7b` 比 `qwen2.5:14b` 快一倍
- 使用云端模型（DeepSeek 性价比较高）

### Q: REPL 中如何开始新话题？

输入 `/reset` 清空对话历史（保留 system prompt）。

### Q: 退出后记忆会丢失吗？

输入 `exit` 退出时，Agent 会自动总结对话并保存为向量记忆（需要配置 `embedding`）。下次对话时，相关记忆会自动注入。

如果直接 Ctrl+C 强制退出，记忆可能不会保存。建议用 `exit` 命令正常退出。

---

## IM 接入（channel start）

### Q: 飞书连接失败，日志显示 401？

检查 `app_id` 和 `app_secret` 是否正确。注意：
- App ID 格式：`cli_` 开头
- 从「凭证与基础信息」页面获取，不是「企业自建应用」的 App Key
- 确保应用已发布（未发布的应用无法使用 API）

### Q: 飞书连接成功，但收不到消息？

检查以下几点：
1. 「事件订阅」→「订阅方式」是否已改为**长连接**（WebSocket）
2. 权限：`im:message:receive_v1` 是否已开通
3. 应用是否已加入群组（群机器人需要先邀请加入）
4. 是否 @ 了机器人（群消息需要 @ 才响应）

### Q: 飞书消息重复回复？

飞书 WebSocket 服务器在网络抖动时可能重复投递事件。nethelper 内部有 `message_id` 去重机制，相同消息只处理一次。如果仍然出现重复，可能是有多个 `channel start` 进程在运行，检查并关闭重复进程。

### Q: Telegram bot 无法接收消息？

1. 检查 token 是否正确（`1234567890:AA...` 格式）
2. 确认 bot 没有其他进程在 polling（同一 bot 只能有一个进程长轮询）
3. 在 Telegram 中直接给 bot 发消息（私聊），确认基础通信正常

### Q: 用户收到 "⚠️ 你没有权限使用此 bot"？

该用户没有匹配到任何权限组。检查 `permissions.groups` 配置：

1. 获取用户的标识：在 JSONL 日志中搜索 `"user"` 字段
2. 将用户添加到合适的权限组：

```yaml
permissions:
  groups:
    operator:
      users:
        - feishu:ou_xxxxxxxxxxxxxxxx   # 添加这个用户
      tools:
        - "show_*"
        - "search_*"
```

如果希望所有用户都能使用，设置 `users: ["*"]`：

```yaml
permissions:
  groups:
    default:
      users: ["*"]
      tools: ["show_*", "search_*"]
```

### Q: 机器人回复被截断了？

IM 回复有 3000 字符限制（防止长方案被拆成几十条消息）。完整内容可以：

1. 使用 `nethelper agent chat` 终端对话（无字符限制）
2. 使用 `nethelper plan isolate <device-id>` 直接运行 CLI 命令

### Q: 多个用户同时发消息会互相干扰吗？

不会。每个用户有独立的 Agent 会话（`userSession`），并通过 per-user mutex 保证同一用户的消息按顺序处理。不同用户的会话完全隔离。

---

## 向量记忆（Memory）

### Q: 记忆功能不生效（每次对话都不记得之前的内容）？

向量记忆需要配置 `embedding`：

```yaml
embedding:
  provider: ollama
  providers:
    ollama:
      base_url: http://localhost:11434
      model: qwen3-embedding
```

确认 embedding 模型已拉取：`ollama pull qwen3-embedding`。

### Q: 记忆被污染了怎么清理？

直接在 SQLite 中删除记忆条目：

```bash
sqlite3 ~/.nethelper/nethelper.db

# 查看所有记忆
SELECT id, source, substr(content, 1, 100), created_at FROM memory_entries;

# 删除特定记忆（按 ID）
DELETE FROM memory_entries WHERE id = 5;

# 清空所有记忆（谨慎！）
DELETE FROM memory_entries;
```

### Q: 记忆搜索到的内容不相关？

向量相似度基于 embedding 模型的语义理解。不相关结果通常有两个原因：
1. **embedding 模型不匹配：** 记忆是用 A 模型向量化的，现在用 B 模型搜索，两个向量空间不兼容。更换模型后需要清空记忆并重新积累
2. **记忆内容太笼统：** 总结过于简短或抽象，语义不够具体。可以手动删除低质量记忆

### Q: 知识库文件添加后不生效？

1. 检查文件是否放在正确目录（`~/.nethelper/knowledge/` 或 config 中 `path` 指定的目录）
2. 检查文件格式是否为 `.md`
3. 检查 embedding 是否已配置
4. **重启 agent chat 或 channel start**（知识库在启动时加载，不会热更新）

### Q: 知识库很大，搜索会不会很慢？

当前实现对所有 knowledge_cache 条目做全表扫描 + 余弦相似度计算。几千条文档 <10ms，通常不成问题。如果知识库超过数万条，需要考虑优化（分批加载或外置向量数据库）。

---

## 变更方案（plan）

### Q: `plan isolate` 生成的方案不够具体，命令不完整？

方案质量取决于数据库中的设备信息完整度。建议：

1. 先导入设备的完整配置（`display current-configuration` 输出）
2. 导入 BGP peer 信息（`display bgp peer`）
3. 导入接口信息（`display interface brief`）

数据越完整，生成的方案越详细。可以用 `nethelper show topology --device <id>` 预览 nethelper 看到的拓扑信息。

### Q: 方案中没有检测到 OSPF/ISIS 隔离步骤？

方案引擎通过分析配置内容推断运行的协议。如果没有检测到 IGP：
1. 确认已导入设备的完整配置快照（含 `ospf` 或 `isis` 配置段）
2. 检查 `nethelper show neighbor --device <id>` 是否有 OSPF/ISIS 邻居记录
3. 如果协议是在 VRF 下运行，确认 VRF 配置也已导入

### Q: `plan upgrade` 的升级命令是哪个厂商的？

方案引擎根据设备的 `vendor` 字段（`huawei`/`cisco`/`h3c`/`juniper`）自动选择对应的升级命令模板。查看设备厂商：

```bash
nethelper show device <device-id>
```

---

## 心跳巡检（heartbeat）

### Q: 心跳启动后，每次都推送消息吗？

不是。心跳的设计原则是**异常时告警，正常时静默**。Agent 在巡检时如果判断一切正常（回复类似"巡检正常，无异常"），则不推送 IM 消息，只写 JSONL 日志。

只有发现异常（SPOF 变化、邻居 Down 等）时才推送告警。

你可以自定义 `heartbeat.prompt` 调整巡检行为，比如"每次都发简报"：

```yaml
heartbeat:
  prompt: "检查所有设备状态，无论是否正常，都给出一句话巡检摘要。"
```

### Q: 心跳日志在哪里？

在 `~/.nethelper/sessions/heartbeat_<date>.jsonl`，格式与 agent chat 日志相同。
