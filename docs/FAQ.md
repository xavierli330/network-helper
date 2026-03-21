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

### Q: 推荐什么 LLM？

| 场景 | 推荐 |
|------|------|
| 本地免费、无隐私顾虑 | Ollama + qwen2.5:14b |
| 效果最好 | OpenAI gpt-4o / Anthropic Claude |
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

API Key 存在 `~/.nethelper/config.yaml` 中，权限默认 644。建议：

```bash
chmod 600 ~/.nethelper/config.yaml
```

也可以使用环境变量（当前版本暂不支持 `${ENV_VAR}` 语法展开，后续版本会加）。

---

## 排障分析

### Q: `trace path` 的路径是怎么算出来的？

基于内存图引擎的 BFS 最短路径。图节点来自设备和接口数据，边来自协议邻居关系（OSPF/BGP/ISIS/LDP peer）和子网连接。

注意：路径分析依赖已导入的数据质量——如果只导入了部分设备的日志，图可能不完整。

### Q: `check spof` 是怎么检测单点故障的？

暴力法：逐个模拟移除每个设备节点，检查剩余网络是否仍然连通。如果移除某个设备后网络断开，该设备就是单点故障。

### Q: 能检测 MPLS 标签一致性吗？

当前版本存储了 RIB/FIB/LFIB 数据，但 `check label` 命令尚未实现（spec 中有规划）。你可以通过 `show route`、`show fib`、`show label` 手动对比。

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
