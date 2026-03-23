package agent

import (
	"context"
	"fmt"

	"github.com/xavierli/nethelper/internal/llm"
)

const systemPrompt = `你是 nethelper 网络运维助手。你可以调用工具来查询网络数据、分析拓扑、生成变更方案、记录排障经验。

## 工作流程

当用户描述网络问题或变更需求时，按以下顺序工作：

### 1. 先搜历史经验（每次对话开始必做）
收到用户问题后，第一步调用 search_log 搜索是否有类似的历史排障记录。
如果找到相关经验，先告诉用户"找到了类似的历史记录"并参考其中的解决方案。
如果没有找到，继续下一步。

### 2. 收集信息
- show_devices 了解网络全貌
- show_device / show_interfaces / show_bgp_peers / show_neighbors 查看具体设备

### 3. 分析和行动
- plan_isolate / plan_upgrade 生成变更方案
- 给出具体、可操作的建议

### 4. 归档经验（每次排障结束主动提议）
当一个排障或变更讨论告一段落时，主动询问用户是否需要记录本次经验。
如果用户同意，调用 note_add 归档，提取：
- symptom: 问题症状或变更目标
- commands_used: 过程中使用的关键命令
- findings: 发现的关键信息
- resolution: 最终结论或解决方案
- tags: 相关标签（设备名、协议、故障类型等）

## 原则
- 用中文回答
- 给出具体、可操作的建议
- 不要凭空猜测——用工具查到的数据说话
- 工具返回的数据可能很长，总结关键信息给用户，不要原样输出`

// Agent orchestrates the LLM + tool calling loop.
type Agent struct {
	router   *llm.Router
	registry *Registry
	messages []llm.Message
}

func New(router *llm.Router, registry *Registry) *Agent {
	return &Agent{
		router:   router,
		registry: registry,
		messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
		},
	}
}

// Chat sends a user message and runs the agent loop until LLM produces a final text response.
// It calls onToolCall for each tool invocation (for REPL display).
func (a *Agent) Chat(ctx context.Context, userInput string, onToolCall func(name string, args map[string]interface{})) (string, error) {
	a.messages = append(a.messages, llm.Message{Role: "user", Content: userInput})

	maxIterations := 20 // safety limit
	for i := 0; i < maxIterations; i++ {
		resp, err := a.router.Chat(ctx, llm.CapAnalyze, llm.ChatRequest{
			Messages: a.messages,
			Tools:    a.registry.ToolDefs(),
		})
		if err != nil {
			return "", fmt.Errorf("LLM error: %w", err)
		}

		if len(resp.ToolCalls) > 0 {
			// Assistant message with tool calls
			a.messages = append(a.messages, llm.Message{
				Role:      "assistant",
				Content:   resp.Content,
				ToolCalls: resp.ToolCalls,
			})

			// Execute each tool
			for _, tc := range resp.ToolCalls {
				if onToolCall != nil {
					onToolCall(tc.Name, tc.Arguments)
				}

				tool, ok := a.registry.Get(tc.Name)
				var result string
				if !ok {
					result = fmt.Sprintf("Unknown tool: %s", tc.Name)
				} else {
					var execErr error
					result, execErr = tool.Handler(tc.Arguments)
					if execErr != nil {
						result = fmt.Sprintf("Error: %v", execErr)
					}
				}

				// Truncate very long results to avoid context overflow
				if len(result) > 8000 {
					result = result[:8000] + "\n... (truncated)"
				}

				a.messages = append(a.messages, llm.Message{
					Role:       "tool",
					Content:    result,
					ToolCallID: tc.ID,
					Name:       tc.Name,
				})
			}
			continue // loop back to LLM
		}

		// No tool calls → final answer
		a.messages = append(a.messages, llm.Message{Role: "assistant", Content: resp.Content})
		return resp.Content, nil
	}

	return "", fmt.Errorf("agent loop exceeded %d iterations", maxIterations)
}

// Reset clears conversation history (keeps system prompt).
func (a *Agent) Reset() {
	a.messages = a.messages[:1] // keep system prompt
}
