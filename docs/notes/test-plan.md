# nethelper 测试验收计划

> 项目状态：60 commits, 101 Go files, 13 packages, 全部单元测试通过
>
> 本文档覆盖所有功能的端到端验收测试。逐步测试，发现问题记录到 Issues 部分。

---

## 测试前准备

### 环境检查

```bash
# 确认安装
nethelper version
# 期望: nethelper v0.1.0

# 确认配置
nethelper config llm
# 期望: 显示 LLM provider 状态

# 确认数据库位置
ls -la ~/.nethelper/nethelper.db
```

### 准备测试日志文件

需要准备 4 种厂商的真实日志文件（或模拟日志）：

| 文件 | 内容要求 |
|------|---------|
| `huawei-test.log` | 华为设备回显，包含 `<hostname>dis cur`、`dis interface brief`、`dis ospf peer`、`dis bgp peer` |
| `cisco-test.log` | 思科设备回显，包含 `hostname#show run`、`show ip route`、`show ip ospf neighbor` |
| `h3c-test.log` | 华三设备回显，包含 `<hostname>display interface brief`、`display ip routing-table` |
| `juniper-test.log` | Juniper 回显，包含 `user@hostname> show route`、`show interfaces terse` |
| `timestamped.log` | 带时间戳前缀的日志（如 SecureCRT 格式：`2026-03-21-13-11-26: <hostname>dis cur`） |

如果暂时没有所有厂商的日志，可以先用华为日志（`teg_20260321131104.log`）完成华为相关测试。

---

## 第一部分：日志导入与解析

### T1.1 手动导入 — 华为日志（带时间戳）

```bash
# 清空数据库重新测试
rm -f ~/.nethelper/nethelper.db

# 导入带时间戳的华为日志
nethelper watch ingest ~/Work/session_log/netdevice/teg_20260321131104.log
```

**验收标准：**
- [ ] 输出显示 `Devices: 1`（只识别出真实设备，跳板机 shell 不被误识别）
- [ ] `Blocks parsed` > 0，`Failed: 0`
- [ ] 不应出现 `xavierli@` 或 `MNET-TEG` 等跳板机主机名

### T1.2 验证设备识别

```bash
nethelper show device
```

**验收标准：**
- [ ] 只显示一台设备
- [ ] Vendor 为 `huawei`（不是 `h3c`）
- [ ] Hostname 正确

### T1.3 验证配置存储

```bash
# 搜索配置中的关键词
nethelper search config "sysname"
nethelper search config "bgp"
nethelper search config "interface Eth-Trunk"
nethelper search config "ospf"
```

**验收标准：**
- [ ] `sysname` 搜索能找到设备名配置
- [ ] `bgp` 搜索能找到 BGP 配置段
- [ ] 配置内容不包含时间戳前缀（`2026-03-21-xx-xx-xx:` 已被剥离）

### T1.4 验证暂存区

```bash
nethelper show scratch
nethelper show scratch --id 1  # 查看具体内容
```

**验收标准：**
- [ ] 路由表输出被存到暂存区（category=route）
- [ ] 暂存区显示命令名、数据大小、时间
- [ ] `--id N` 能查看完整内容

### T1.5 验证邻居解析

```bash
nethelper show neighbor --device <device-id>
```

**验收标准：**
- [ ] 如果日志中包含 `dis ospf peer` / `dis bgp peer` 输出，邻居应被解析
- [ ] 邻居的 protocol、remote_id、state 字段正确
- [ ] 如果日志中没有邻居命令输出，显示 "No neighbors found"（不报错）

### T1.6 验证接口解析

```bash
nethelper show interface --device <device-id>
```

**验收标准：**
- [ ] 如果日志中包含 `dis interface brief`，接口应被解析
- [ ] 接口类型推断正确（physical/loopback/eth-trunk/vlanif）
- [ ] 状态正确（up/down/admin-down）

---

## 第二部分：多厂商解析（有对应日志时测试）

### T2.1 思科日志导入

```bash
nethelper watch ingest cisco-test.log
nethelper show device  # 应多出一台 cisco 设备
nethelper show route --device <cisco-device-id>
```

**验收标准：**
- [ ] Vendor 识别为 `cisco`
- [ ] `hostname#` 提示符正确匹配
- [ ] `show ip route` 输出被存到暂存区
- [ ] `show ip ospf neighbor` 被解析为邻居

### T2.2 华三日志导入

```bash
nethelper watch ingest h3c-test.log
```

**验收标准：**
- [ ] `<hostname>` 提示符正确匹配
- [ ] 注意：当前版本华为先于华三匹配，所以 H3C 设备可能被标记为 `huawei`
- [ ] 解析结果仍然正确（两者命令格式相似）

### T2.3 Juniper 日志导入

```bash
nethelper watch ingest juniper-test.log
```

**验收标准：**
- [ ] Vendor 识别为 `juniper`
- [ ] `user@hostname>` 提示符正确匹配
- [ ] `show route` 多行格式（prefix 一行 + next-hop 续行）被正确解析

---

## 第三部分：Watcher 实时监控

### T3.1 启动监控

```bash
# 确保监控目录存在
mkdir -p ~/network-logs

# 启动（前台运行，Ctrl+C 停止）
nethelper watch start --dir ~/network-logs
```

**验收标准：**
- [ ] 显示 "Watching directories: ..."
- [ ] 显示 "Press Ctrl+C to stop."

### T3.2 实时文件检测

在另一个终端：

```bash
# 复制一个日志文件到监控目录
cp ~/Work/session_log/netdevice/teg_20260321131104.log ~/network-logs/test1.log
```

**验收标准：**
- [ ] watcher 终端在几秒内显示 `[HH:MM:SS] Ingested test1.log (...)`
- [ ] 显示解析统计（devices, parsed）

### T3.3 增量读取

```bash
# 向同一个文件追加内容
echo "<TEST-DEVICE>display version" >> ~/network-logs/test1.log
echo "Huawei version V200R020" >> ~/network-logs/test1.log
```

**验收标准：**
- [ ] watcher 只处理新增内容（`new=XX bytes`，不是整个文件大小）

### T3.4 优雅停止

在 watcher 终端按 `Ctrl+C`

**验收标准：**
- [ ] 显示 "Stopping watcher..."
- [ ] 显示 "Watcher stopped."
- [ ] PID 文件被清理（`ls ~/.nethelper/watcher.pid` 应不存在）

### T3.5 状态检查

```bash
nethelper watch status
# 停止后应显示: Watcher is not running.

# 再次启动后
nethelper watch start --dir ~/network-logs &
nethelper watch status
# 应显示: Watcher is running (PID xxxx)

# 停止
nethelper watch stop
```

---

## 第四部分：拓扑分析

> 需要至少导入 2+ 台设备的日志，且设备间有邻居关系

### T4.1 拓扑概览

```bash
nethelper show topology
```

**验收标准：**
- [ ] 显示设备数、接口数、子网数
- [ ] 每台设备的 PEERS 列正确反映邻居数

### T4.2 路径追踪

```bash
nethelper trace path --from <device1> --to <device2>
nethelper trace path --from <device1> --to <device2> --all
```

**验收标准：**
- [ ] 显示最短路径（节点序列）
- [ ] `--all` 显示所有路径
- [ ] 如果两台设备无连接，显示 "no path from..."

### T4.3 故障影响分析

```bash
nethelper trace impact --node <device-id>
```

**验收标准：**
- [ ] 显示移除该设备后受影响的设备列表
- [ ] 如果该设备不是单点故障，显示 "no impact"

### T4.4 环路检测

```bash
nethelper check loop
```

**验收标准：**
- [ ] 正常网络应显示 "No loops detected."
- [ ] 不应 panic 或报错

### T4.5 单点故障检测

```bash
nethelper check spof
```

**验收标准：**
- [ ] 列出单点故障设备（如果有）
- [ ] 全冗余网络显示 "No single points of failure found."

---

## 第五部分：知识系统

### T5.1 排障笔记 — 增删查

```bash
# 添加
nethelper note add \
  --symptom "OSPF 邻居反复震荡" \
  --finding "MTU 不匹配，一端 1500 另一端 9000" \
  --resolution "统一 MTU 为 1500" \
  --tags "ospf,mtu,flap"

# 列表
nethelper note list

# 搜索
nethelper note search "MTU"
nethelper note search "OSPF"
nethelper note search "不存在的关键词"
```

**验收标准：**
- [ ] `note add` 返回 "Note #N created."
- [ ] `note list` 显示添加的笔记（最新优先）
- [ ] `note search "MTU"` 匹配到（FTS5 全文搜索）
- [ ] 搜索不存在的词返回 "No matching notes found."

### T5.2 全文搜索

```bash
nethelper search config "bgp"
nethelper search config "ospf cost"
nethelper search log "OSPF"
nethelper search command "routing"
```

**验收标准：**
- [ ] `search config` 搜索配置快照内容
- [ ] `search log` 搜索排障笔记
- [ ] `search command` 搜索命令手册（需要先有 command_references 数据）
- [ ] 无匹配时显示 "No matching ... found."

### T5.3 配置差异对比

```bash
# 需要同一设备有 2 次以上配置导入
# 导入两次不同版本的配置：
nethelper watch ingest old-config.log
# 修改配置后再导入
nethelper watch ingest new-config.log

nethelper diff config --device <device-id>
```

**验收标准：**
- [ ] 显示 unified diff 格式（`-` 删除、`+` 新增）
- [ ] 只有一个快照时显示 "Need at least 2 config snapshots to diff."

---

## 第六部分：LLM 智能功能

> 需要配置 LLM provider（config.yaml 中的 llm 部分）

### T6.1 AI 配置解读

```bash
# 基于设备数据回答
nethelper explain "这台设备有哪些 bgp 邻居"
nethelper explain "这台设备的 OSPF 区域怎么划分的"
nethelper explain --device <id> "接口 Eth-Trunk1 的配置是什么"

# 基于文件内容解读
nethelper explain --file /path/to/some-output.txt "解释这段输出"
```

**验收标准：**
- [ ] LLM 回答引用了实际设备数据（IP、接口名、AS 号等）
- [ ] 不是泛泛而谈的通用回答
- [ ] `--device` 聚焦到特定设备
- [ ] `--file` 模式能读取文件内容

### T6.2 AI 排障建议

```bash
nethelper diagnose "OSPF 邻居 down"
nethelper diagnose --device <id> "BGP 邻居震荡"
```

**验收标准：**
- [ ] 如果有历史排障笔记匹配，先显示 "📋 相关排障历史"
- [ ] LLM 分析引用了设备的实际配置和邻居数据
- [ ] 建议的排查命令与设备厂商匹配（华为用 display，思科用 show）

### T6.3 AI 经验提取

```bash
nethelper note extract ~/Work/session_log/netdevice/teg_20260321131104.log
```

**验收标准：**
- [ ] LLM 从日志中提取 symptom/findings/resolution/tags
- [ ] 输出 JSON 格式的结构化信息
- [ ] 提示用户如何用 `note add` 保存

### T6.4 无 LLM 降级

```bash
# 临时禁用 LLM（把 config 里 default 注释掉）
nethelper diagnose "OSPF 问题"
nethelper explain "查看配置"
```

**验收标准：**
- [ ] `diagnose` 退回到 FTS5 搜索，显示匹配的历史笔记
- [ ] `explain` 提示 "No LLM provider configured" 并给出配置指引
- [ ] 不报错不 panic

---

## 第七部分：导出功能

### T7.1 数据库备份

```bash
nethelper export db -o /tmp/backup.db
ls -la /tmp/backup.db
```

**验收标准：**
- [ ] 生成的文件大小 > 0
- [ ] 文件可以用 `sqlite3` 打开查询

### T7.2 拓扑导出

```bash
# DOT 格式
nethelper export topology --format dot -o /tmp/network.dot
cat /tmp/network.dot

# JSON 格式
nethelper export topology --format json -o /tmp/network.json
cat /tmp/network.json

# 如果安装了 graphviz，生成图片
dot -Tpng /tmp/network.dot -o /tmp/network.png
```

**验收标准：**
- [ ] DOT 文件包含 `digraph network {`，设备节点和边
- [ ] JSON 文件包含 `nodes` 和 `edges` 数组
- [ ] graphviz 能成功渲染图片

### T7.3 网络报告

```bash
nethelper export report -o /tmp/report.md
cat /tmp/report.md
```

**验收标准：**
- [ ] Markdown 格式，包含设备汇总表
- [ ] 包含网络统计（设备数、接口数、子网数）
- [ ] 如果有单点故障，在报告中列出

---

## 第八部分：边界情况与健壮性

### T8.1 空数据库

```bash
rm -f ~/.nethelper/nethelper.db
nethelper show device          # 应显示空提示
nethelper show topology        # 应显示 0 设备
nethelper check loop           # 不应报错
nethelper search config "test" # 应显示空结果
```

### T8.2 非法输入

```bash
nethelper watch ingest /nonexistent/file     # 应报文件不存在错误
nethelper show route --device nonexistent    # 应报找不到设备/快照
nethelper explain                             # 无参数应报错
nethelper trace path                          # 缺少 --from --to 应报错
```

### T8.3 大文件处理

```bash
# 导入大型日志文件（>1MB）
nethelper watch ingest <large-log-file>
```

**验收标准：**
- [ ] 不 OOM
- [ ] 解析时间合理（<30 秒）
- [ ] 暂存区大条目正确存储

### T8.4 重复导入

```bash
# 同一个文件导入两次
nethelper watch ingest test.log
nethelper watch ingest test.log
```

**验收标准：**
- [ ] 第二次导入不应 panic
- [ ] 设备数据被更新（upsert）
- [ ] 配置快照会新增一条（可以用 diff 对比）

### T8.5 混合厂商日志

```bash
# 一个文件包含多个厂商的设备输出（跳板机场景）
nethelper watch ingest mixed-vendors.log
nethelper show device
```

**验收标准：**
- [ ] 每个厂商的设备分别识别
- [ ] 命令输出归属正确的设备

---

## 已知限制（当前版本）

以下是已知的限制，不算测试失败：

| 限制 | 说明 | 计划修复 |
|------|------|---------|
| 华为/华三无法自动区分 | 两者共用 `<hostname>` 提示符，当前按注册顺序匹配（华为优先） | 后续支持配置文件绑定设备→厂商 |
| 路由表不做结构化存储 | 太大，存暂存区原文 | 可支持特定前缀的结构化查询 |
| Embedding 向量搜索未启用 | sqlite-vec 集成未完成 | 后续 Plan |
| `note extract --save` 不自动保存 | 需要手动 `note add` | 后续添加 `--save` 标志 |
| `diff snapshot` 未实现 | Spec 中有但未做 | 后续补充 |
| `check label` 未实现 | RIB↔LFIB↔FIB 一致性检查 | 后续补充 |
| `trace label` 未实现 | 标签栈沿途变化追踪 | 后续补充 |
| `check stale` 未实现 | 过期/不一致数据检测 | 后续补充 |
| `--llm` 临时覆盖标志 | Spec 中有但未实现 | 后续补充 |
| Watcher 不支持递归子目录 | 只监控直接指定的目录 | 后续增加 fsnotify 递归监控 |
| 环境变量 `${ENV_VAR}` 展开 | config.yaml 中不支持 | 后续补充 |

---

## 测试记录模板

每次测试后，在下方记录结果：

```
### 测试日期: YYYY-MM-DD
测试人:
测试日志文件:

| 测试项 | 结果 | 备注 |
|--------|------|------|
| T1.1 华为日志导入 | ✅/❌ | |
| T1.2 设备识别 | ✅/❌ | |
| T1.3 配置存储 | ✅/❌ | |
| ... | | |
```
