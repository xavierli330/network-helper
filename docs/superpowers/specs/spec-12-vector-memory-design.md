# Design: 向量记忆系统 — Agent 长期记忆

**日期:** 2026-03-23
**状态:** Approved

## 问题

agent chat 退出后所有对话历史丢失。下次进来，agent 不知道之前聊过什么、网络拓扑的认知、用户偏好。

## 目标

让 agent 具备跨会话的长期记忆——基于向量 embedding 的语义搜索记忆系统。

## 设计

### 存储

`memory_entries` 表（新增到 SQLite）：

```sql
CREATE TABLE memory_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    category TEXT NOT NULL DEFAULT 'conversation',  -- conversation/insight/preference
    content TEXT NOT NULL,                            -- 记忆文本
    embedding BLOB,                                   -- float32[] 序列化
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    session_id TEXT NOT NULL DEFAULT ''               -- 关联会话 ID
)
```

### Embedding 提供者

新增 `internal/llm/embedding.go`：

```go
type EmbeddingProvider interface {
    Embed(ctx context.Context, text string) ([]float32, error)
}

// OllamaEmbedder calls POST /api/embeddings
type OllamaEmbedder struct {
    BaseURL string
    Model   string
}
```

调用 Ollama embedding API（`POST /api/embeddings`），返回 `[]float32`。

配置来源：`config.yaml` 的 `embedding` 段（已配置为 `qwen3-embedding`）。

### 向量搜索

`internal/memory/search.go`：

Go 层遍历所有 memory_entries，计算余弦相似度，返回 Top-K。

```go
func cosineSimilarity(a, b []float32) float64
func Search(db, queryVec []float32, topK int) []MemoryEntry
```

数据量预期：几百到几千条记忆，全表扫描 + Go 计算完全可行（<10ms）。

### 记忆写入

每次对话结束时：
1. Agent 用 LLM 生成对话摘要（1-2 句话总结关键内容）
2. 摘要 → embedding → 存入 memory_entries（category="conversation"）

触发时机：用户输入 `exit` / `quit` 或 `/reset` 时自动执行。

### 记忆读取

每次 `Chat()` 调用时：
1. 用户输入 → embedding
2. 向量搜索 memory_entries → Top-3 最相关记忆
3. 将记忆文本追加到 system prompt 末尾：

```
## 相关历史记忆
- [2026-03-22] 讨论了 LC-01 板卡更换，生成了隔离方案，234 个 BGP peers
- [2026-03-20] QCDR-01 出现 ISIS 邻居抖动，原因是 Eth-Trunk1 成员口 CRC 错误
```

### 代码结构

```
internal/
├── llm/
│   └── embedding.go      # EmbeddingProvider + OllamaEmbedder
├── memory/
│   ├── store.go           # MemoryEntry + DB 操作（Insert/List/Search）
│   └── search.go          # 余弦相似度 + Top-K 搜索
├── agent/
│   └── loop.go            # 修改：Chat 开始时注入记忆，结束时保存摘要
└── store/
    └── migrations.go      # 新增 memory_entries 表
```
