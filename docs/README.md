# Network Helper 文档索引

---

## 目录结构

```
docs/
├── README.md                    # 本文档
├── FAQ.md                       # 常见问题
├── agent-guide.md               # Agent 使用指南
├── command-syntax-knowledge.md  # 命令语法知识库（供 LLM 使用）
├── configuration.md             # 配置说明
├── extension-guide.md           # 扩展开发指南
├── network-engineer-analysis.md # 网络工程师视角的解析逻辑分析
│
├── device/                      # 设备文档 & 知识库
│   ├── engineering-network-engineer.md   # 工程师视角设计
│   ├── network-knowledge-base.md          # 知识库总览
│   ├── extracted_text/                    # 原始设备文档提取文本（勿直接编辑）
│   │   ├── CISCO_ASR_*.txt                 # Cisco ASR 系列
│   │   ├── NE40E V800R025C00*.txt          # 华为 NE40E 系列
│   │   ├── Juniper_CLI.txt
│   │   └── H3C文档.txt
│   └── kb/                      # 知识库（按主题编号）
│       ├── kb-01-ip-routing.md
│       ├── kb-02-mpls.md
│       ├── kb-03-segment-routing.md
│       ├── kb-04-vpn.md
│       ├── kb-05-qos.md
│       ├── kb-06-security.md
│       ├── kb-07-reliability.md
│       ├── kb-08-monitoring.md
│       ├── kb-09-pcep.md
│       ├── kb-10-display-reference.md
│       ├── kb-11-troubleshooting.md
│       └── kb-12-cross-reference.md
│
├── superpowers/                 # Agent 能力规划 & 设计文档
│   ├── plans/                   # 实施计划（按编号排序）
│   │   ├── plan-01-core-foundation.md
│   │   ├── plan-02-parser-pipeline.md
│   │   ├── plan-03-vendors-watcher.md
│   │   ├── plan-04-graph-engine.md
│   │   ├── plan-05-knowledge-search.md
│   │   ├── plan-06-llm-export.md
│   │   ├── plan-07-igp-isolation.md
│   │   ├── plan-08-incremental-ingestion-fix.md
│   │   ├── plan-09-multi-session-data-freshness.md
│   │   ├── plan-10-plan-isolate.md
│   │   ├── plan-11-plan-isolate-v2.md
│   │   ├── plan-12-im-channel.md
│   │   ├── plan-13-mcp-server.md
│   │   ├── plan-14-field-introspection.md
│   │   └── plan-15-parser-rule-studio.md
│   └── specs/                   # 技术设计文档（按编号排序）
│       ├── spec-01-integration-testing-llm-optimization-design.md
│       ├── spec-02-network-helper-design.md
│       ├── spec-03-igp-isolation-design.md
│       ├── spec-04-incremental-ingestion-fix-design.md
│       ├── spec-05-multi-session-data-freshness-design.md
│       ├── spec-06-plan-isolate-design.md
│       ├── spec-07-plan-isolate-v2-design.md
│       ├── spec-08-plan-upgrade-design.md
│       ├── spec-09-agent-loop-design.md
│       ├── spec-10-im-channel-design.md
│       ├── spec-11-mcp-server-design.md
│       ├── spec-12-vector-memory-design.md
│       ├── spec-13-field-introspection-design.md
│       └── spec-14-parser-rule-studio-design.md
│
└── notes/                       # 临时工作记录（归档性质，勿当作规范文档）
    ├── OPTIMIZATION_COMPLETE.md
    ├── OPTIMIZATION_REPORT.md
    ├── lc-isolation-plan.md      # 旧版 LC 隔离计划（已废弃）
    ├── lc-isolation-plan-v2.md
    ├── phase-review-2026-03-24.md
    └── test-plan.md
```

---

## 设计原则

- **`device/kb/`**：由 `docs/command-syntax-knowledge.md` 通过 LLM 生成，供 Parser 理解命令输出结构时参考；可直接被 LLM RAG 检索
- **`superpowers/plans/`**：Feature 开发计划，按时间顺序编号（01 → 15）
- **`superpowers/specs/`**：对应 plans 的技术设计文档，`*-design.md` 后缀
- **`notes/`**：已废弃或一次性的工作记录，不作为开发规范依据
- 所有 Markdown 文件优先使用中文标题，方便 AI 阅读和检索
