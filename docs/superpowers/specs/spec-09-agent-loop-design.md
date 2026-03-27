# Design: Agent Loop — LLM Tool Calling + Interactive REPL

**日期:** 2026-03-23
**状态:** Approved

## 问题

nethelper 的 LLM 层只支持纯文本聊天（send messages → get text）。要实现 agent loop（LLM 自主调用 nethelper 工具探索网络），需要先给 LLM 层加上 function/tool calling 能力。

## 目标

1. 扩展 LLM provider 支持 tool calling（OpenAI + Anthropic 协议）
2. 实现 `nethelper agent chat` — 交互式 REPL，LLM 自主调用 nethelper 工具

## 设计

### LLM Tool Calling 扩展

扩展 `ChatRequest` 和 `ChatResponse`：

```go
// 请求中附带 tool 定义
type ToolDef struct {
    Name        string                 `json:"name"`
    Description string                 `json:"description"`
    Parameters  map[string]interface{} `json:"parameters"` // JSON Schema
}

type ChatRequest struct {
    Messages    []Message  `json:"messages"`
    Tools       []ToolDef  `json:"tools,omitempty"`       // 新增
    Temperature float64    `json:"temperature,omitempty"`
    MaxTokens   int        `json:"max_tokens,omitempty"`
}

// 响应中可能包含 tool calls
type ToolCall struct {
    ID        string                 `json:"id"`
    Name      string                 `json:"name"`
    Arguments map[string]interface{} `json:"arguments"`
}

type ChatResponse struct {
    Content   string     `json:"content"`
    ToolCalls []ToolCall `json:"tool_calls,omitempty"` // 新增
    StopReason string   `json:"stop_reason,omitempty"` // "tool_use" / "end_turn"
}
```

**OpenAI provider 改动：**
- 请求：`tools` 字段映射为 OpenAI 的 `tools: [{type: "function", function: {...}}]`
- 响应：解析 `choices[0].message.tool_calls` 数组

**Anthropic provider 改动：**
- 请求：`tools` 字段映射为 Anthropic 的 `tools: [{name, description, input_schema}]`
- 响应：解析 `content` 数组中 `type: "tool_use"` 的 block

### Agent Loop

```go
func AgentLoop(ctx context.Context, router *llm.Router, tools []Tool, userInput string) {
    messages := []Message{systemPrompt, {Role: "user", Content: userInput}}

    for {
        resp, err := router.Chat(ctx, CapAnalyze, ChatRequest{
            Messages: messages,
            Tools:    toolDefs,
        })

        if len(resp.ToolCalls) > 0 {
            // 执行工具
            messages = append(messages, assistantMessage(resp))
            for _, tc := range resp.ToolCalls {
                result := executeTool(tc.Name, tc.Arguments)
                messages = append(messages, toolResultMessage(tc.ID, result))
            }
            continue // 再次调用 LLM
        }

        // 无 tool call → LLM 输出最终回答
        fmt.Println(resp.Content)
        break
    }
}
```

### Tool 注册

复用 MCP 的 tool handler 逻辑，但不走 MCP 协议。定义一个内部 Tool 接口：

```go
// internal/agent/tool.go
type Tool struct {
    Name        string
    Description string
    Parameters  map[string]interface{} // JSON Schema
    Handler     func(args map[string]interface{}) (string, error)
}
```

注册所有 MCP 中已有的 tools（show_devices, plan_isolate 等），handler 直接调用 store/plan 函数。

### CLI REPL

```
$ nethelper agent chat
nethelper> LC-01 要做板卡更换

[thinking] 调用 show_device("lc-01")...
[thinking] 调用 show_bgp_peers("lc-01")...
[thinking] 调用 plan_isolate("lc-01")...

LC-01 (H3C H12516AF, AS 65508) 有 234 个 BGP peers。
已生成 6 阶段隔离方案...

nethelper> 帮我记录一下这次变更

[thinking] 调用 note_add(...)...

已记录。

nethelper> exit
```

### 代码结构

```
internal/
├── llm/
│   ├── types.go          # 修改：加 ToolDef, ToolCall 字段
│   ├── openai.go         # 修改：支持 tools 请求/响应
│   └── anthropic.go      # 修改：支持 tools 请求/响应
├── agent/
│   ├── tools.go          # 新建：Tool 注册表 + 执行器
│   ├── loop.go           # 新建：agent loop 核心
│   └── repl.go           # 新建：REPL 交互
└── cli/
    └── agent.go          # 新建：nethelper agent chat 命令
```

### System Prompt

```
你是 nethelper 的网络运维助手。你可以调用工具来查询网络数据、分析拓扑、生成变更方案、记录排障经验。

当用户描述网络问题或变更需求时：
1. 先用 show_devices/show_topology 了解网络全貌
2. 用 show_bgp_peers/show_neighbors 查看具体设备的互联关系
3. 用 plan_isolate/plan_upgrade 生成变更方案
4. 用 search_log 搜索历史排障经验
5. 排障结束后用 note_add 归档经验

始终用中文回答。给出具体的、可操作的建议。
```
