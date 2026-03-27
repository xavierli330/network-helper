# Design: nethelper MCP Server

**日期:** 2026-03-23
**状态:** Approved

## 问题

nethelper 的所有能力（查询、分析、变更方案生成）都锁在 CLI 中，只能人工交互使用。要进入 Phase 2（Agent Loop），第一步是让 agent 能够调用 nethelper——通过 MCP（Model Context Protocol）把 nethelper 暴露为 tool provider。

## 目标

实现 `nethelper mcp serve` 命令，启动一个 MCP server（stdio 传输），将 nethelper 的核心功能暴露为 19 个 MCP tools。完成后在 Claude Code 中配置即可直接使用。

## 设计

### 入口

```bash
nethelper mcp serve
```

启动后通过 stdin/stdout 与调用方通信（JSON-RPC 2.0，换行分隔）。日志输出到 stderr。

### MCP SDK

使用官方 Go SDK `github.com/modelcontextprotocol/go-sdk` 或社区成熟实现 `github.com/mark3labs/mcp-go`。两者都支持 stdio transport。选择标准：哪个 API 更简洁、文档更好。

### Tool 列表

所有 tool 返回 JSON 格式（不是 CLI 表格），便于 LLM 解析。

**查询类（只读）：**

| Tool Name | 参数 | 返回 |
|-----------|------|------|
| `show_devices` | — | `[{id, hostname, vendor, os_version, mgmt_ip, last_seen}]` |
| `show_device` | `device_id` | `{id, hostname, vendor, ...}` |
| `show_interfaces` | `device_id` | `[{name, type, status, ip, description}]` |
| `show_routes` | `device_id`, `prefix?`, `protocol?` | `[{prefix, next_hop, protocol, metric}]` |
| `show_neighbors` | `device_id` | `[{protocol, remote_id, state, interface}]` |
| `show_topology` | — | `{devices: N, interfaces: N, subnets: N, peers: [{device, peer_count}]}` |
| `show_bgp_peers` | `device_id` | `[{peer_ip, remote_as, peer_group, description, state?}]` |
| `trace_path` | `from`, `to` | `{hops: [{node, type, details}]}` |
| `trace_impact` | `node_id` | `{affected: [device_ids]}` |
| `check_spof` | — | `{spofs: [device_ids]}` |
| `check_loop` | — | `{loops: [[node_ids]]}` |
| `diff_config` | `device_id` | `{current, previous, diff}` |
| `search_config` | `query` | `[{device_id, snippet, score}]` |
| `search_log` | `query` | `[{symptom, resolution, tags}]` |

**变更方案类：**

| Tool Name | 参数 | 返回 |
|-----------|------|------|
| `plan_isolate` | `device_id` | 变更方案文本（Markdown） |
| `plan_upgrade` | `device_id`, `version`, `file` | 升级方案文本 |
| `plan_cutover` | `device_id`, `old_interface`, `new_interface` | 割接方案文本 |

**AI 分析类：**

| Tool Name | 参数 | 返回 |
|-----------|------|------|
| `diagnose` | `description`, `device_id?` | AI 排障建议文本 |

**写入类：**

| Tool Name | 参数 | 返回 |
|-----------|------|------|
| `note_add` | `device_id`, `symptom`, `commands_used`, `findings`, `resolution`, `tags` | `{id, status}` |
| `watch_ingest` | `file_path` | `{devices_found, blocks_parsed}` |

### 代码结构

```
internal/mcp/
├── server.go           // MCP server 初始化 + tool 注册
├── tools_show.go       // show_* 系列 tool handlers
├── tools_trace.go      // trace_* + check_* handlers
├── tools_plan.go       // plan_* handlers
├── tools_search.go     // search_* + diff_* handlers
├── tools_write.go      // note_add + watch_ingest handlers
└── tools_ai.go         // diagnose handler

internal/cli/
└── mcp.go              // CLI 入口: nethelper mcp serve
```

### Tool Handler 模式

每个 tool handler 直接调用 store/graph/plan 包的 Go 函数，不走 CLI 解析。返回 JSON：

```go
func handleShowDevices(db *store.DB) mcp.ToolHandler {
    return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        devices, err := db.ListDevices()
        if err != nil {
            return nil, err
        }
        data, _ := json.Marshal(devices)
        return mcp.NewTextResult(string(data)), nil
    }
}
```

### Claude Code 集成

```json
{
  "mcpServers": {
    "nethelper": {
      "command": "/path/to/nethelper",
      "args": ["mcp", "serve"],
      "env": {}
    }
  }
}
```

### 全局状态

MCP server 启动时执行与 CLI 相同的初始化（`config.LoadFrom` → `store.Open` → parser registry），共享 `db` 连接。plan 相关 tool 需要 `graph.BuildFromDB`（按需构建）。LLM 相关 tool（diagnose）需要 `llm.BuildFromConfig`。

### 测试

- 单元测试：每个 tool handler 独立测试（mock DB / 使用 temp DB）
- 集成测试：启动 MCP server 进程，通过 stdio 发 JSON-RPC 请求验证响应
- 端到端：在 Claude Code 中手动验证（"帮我查 LC-01 的接口"）
