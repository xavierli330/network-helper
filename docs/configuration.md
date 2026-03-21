# nethelper 配置指南

## 快速开始

```bash
# 1. 复制模板到默认位置
mkdir -p ~/.nethelper
cp configs/config.example.yaml ~/.nethelper/config.yaml

# 2. 编辑配置（至少设置 watch_dirs）
vim ~/.nethelper/config.yaml

# 3. 验证配置
nethelper config llm
```

## 配置文件位置

默认路径：`~/.nethelper/config.yaml`

可通过 `--config` 参数指定其他路径：

```bash
nethelper --config /path/to/config.yaml show device
```

## 配置项说明

### 数据存储

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `data_dir` | `~/.nethelper` | 数据目录，存放数据库、PID 文件、日志 |
| `db_path` | `~/.nethelper/nethelper.db` | SQLite 数据库路径 |

数据迁移只需复制 `nethelper.db` 和 `config.yaml` 两个文件。

### 日志监控目录

```yaml
watch_dirs:
  - ~/network-logs
  - /var/log/network
```

`nethelper watch start` 会监控这些目录。也可以用 `--dir` 参数临时指定：

```bash
nethelper watch start --dir ~/my-logs --dir /tmp/logs
```

### LLM 配置

LLM 是**可选的增强功能**。不配置 LLM，以下核心功能完全正常：

- ✅ 日志解析和监控（watch）
- ✅ 设备/接口/路由/标签查询（show）
- ✅ 拓扑分析和路径追踪（trace）
- ✅ FTS5 全文搜索（search）
- ✅ 配置差异对比（diff）
- ✅ 排障笔记管理（note add/list/search）
- ✅ 健康检查（check）
- ✅ 数据导出（export）

配置 LLM 后额外获得：

- 🤖 `nethelper diagnose` — AI 排障建议
- 🤖 `nethelper explain` — AI 配置/输出解读
- 🤖 `nethelper note extract` — AI 从日志自动提取排障经验

#### Provider 配置

所有 provider 都使用 **OpenAI 兼容 API** 格式。只要你的服务支持 `/v1/chat/completions` 接口，就能接入。

##### Ollama（推荐入门）

本地运行，免费，无隐私顾虑。

```bash
# 安装 Ollama (macOS)
brew install ollama

# 拉取模型
ollama pull qwen2.5:14b
```

```yaml
llm:
  default: ollama
  providers:
    ollama:
      base_url: http://localhost:11434
      model: qwen2.5:14b
```

推荐模型：
- `qwen2.5:14b` — 中英文均衡，网络知识好
- `qwen2.5:7b` — 更轻量
- `llama3.1:8b` — 英文为主

##### OpenAI

```yaml
llm:
  default: openai
  providers:
    openai:
      api_key: sk-proj-xxxxxxxxxxxx
      model: gpt-4o-mini       # 便宜且够用
      # model: gpt-4o          # 效果更好但更贵
```

API Key 也可以通过环境变量设置：

```yaml
    openai:
      api_key: ${OPENAI_API_KEY}    # 从环境变量读取
      model: gpt-4o-mini
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

##### 通义千问（阿里云 DashScope）

```yaml
llm:
  default: qwen
  providers:
    qwen:
      api_key: sk-xxxxxxxxxxxx
      model: qwen-plus
      base_url: https://dashscope.aliyuncs.com/compatible-mode/v1
```

##### 其他 OpenAI 兼容服务

任何提供 `/v1/chat/completions` 接口的服务都可以接入：

```yaml
llm:
  default: my-service
  providers:
    my-service:
      api_key: your-key
      model: your-model
      base_url: https://your-server.com/v1
```

#### 按能力路由

可以为不同任务指定不同的 provider/模型。比如：

- 提取（extract）用便宜的本地模型
- 分析（analyze）用最好的云端模型
- 解读（explain）用本地模型省钱

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
    extract: ollama       # 结构化提取 → 本地模型
    analyze: openai       # 排障分析 → GPT-4o
    explain: ollama       # 配置解读 → 本地模型
    parse: ollama         # 解析兜底 → 本地模型
```

四种能力：

| 能力 | 触发方式 | 说明 |
|------|---------|------|
| `extract` | `nethelper note extract <file>` | 从日志中提取排障经验 |
| `analyze` | `nethelper diagnose "描述"` | 分析问题、给出排障建议 |
| `explain` | `nethelper explain <text>` | 解读配置或命令输出 |
| `parse` | 自动（L1/L2 解析失败时） | 用 LLM 尝试解析未知输出格式 |

### Embedding 配置（预留）

当前版本使用 FTS5 关键词搜索。后续版本将支持 sqlite-vec 向量搜索，届时可配置 embedding provider：

```yaml
embedding:
  provider: ollama
  providers:
    ollama:
      base_url: http://localhost:11434
      model: nomic-embed-text    # 768 维
    openai:
      api_key: sk-proj-xxxxxxxxxxxx
      model: text-embedding-3-small  # 1536 维
```

## 数据目录结构

```
~/.nethelper/
├── config.yaml       # 配置文件
├── nethelper.db      # SQLite 数据库（所有数据）
├── watcher.pid       # Watcher 守护进程 PID 文件
└── logs/
    └── nethelper.log # 工具自身日志
```

## 常用操作

```bash
# 查看当前 LLM 配置状态
nethelper config llm

# 备份整个数据库
nethelper export db -o backup.db

# 导出网络拓扑为 DOT 图
nethelper export topology --format dot -o network.dot
dot -Tpng network.dot -o network.png    # 需要 graphviz

# 生成网络状态报告
nethelper export report -o report.md
```

## 故障排查

**"no LLM provider configured"**

运行 `nethelper config llm` 查看配置状态。确保：
1. `config.yaml` 中的 `llm.default` 指向一个已定义的 provider
2. provider 的 `base_url` 可达（Ollama 需要先启动）

**"database is locked"**

可能有多个进程同时写入。检查 `nethelper watch status`，确保只有一个 watcher 实例。

**"no devices found"**

还没有导入日志。使用 `nethelper watch ingest <file>` 手动导入，或 `nethelper watch start` 启动自动监控。
