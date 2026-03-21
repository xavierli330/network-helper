# nethelper

网络工程师的排障效率工具 —— 解析多厂商设备日志，构建网络拓扑，追踪配置变更，积累排障经验。

## 特性

- **四厂商解析** — 华为 VRP、思科 IOS、华三 Comware、Juniper JUNOS
- **自动监控** — 后台监控日志目录，新内容自动解析入库
- **控制平面 + 数据平面** — RIB、FIB、LFIB 全覆盖，支持 MPLS/LDP/RSVP/SR
- **内存图引擎** — 拓扑分析、端到端路径追踪、故障影响评估、单点故障检测
- **FTS5 全文搜索** — 跨配置、排障记录、命令手册搜索
- **配置变更追踪** — 自动 diff，随时对比历史版本
- **排障笔记** — 结构化记录排障经验，越用越好
- **LLM 增强**（可选）— AI 排障建议、配置解读、日志经验提取
- **单文件迁移** — 所有数据在一个 SQLite 文件里，复制即迁移

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

详见 [配置指南](docs/configuration.md)。

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
├── store/        SQLite 存储层 + FTS5
└── watcher/      fsnotify 文件监控
```

## 许可证

MIT
