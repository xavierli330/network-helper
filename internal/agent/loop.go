package agent

import (
	"context"
	"fmt"

	"github.com/xavierli/nethelper/internal/llm"
)

const systemPrompt = `你是 nethelper 网络运维助手。你可以调用工具来查询网络数据、分析拓扑、生成变更方案、记录排障经验。

当用户描述网络问题或变更需求时：
1. 先了解网络全貌（show_devices）
2. 查看具体设备的互联关系（show_bgp_peers, show_neighbors）
3. 生成变更方案（plan_isolate, plan_upgrade）
4. 搜索历史排障经验（search_log）
5. 排障结束后归档经验（note_add）

用中文回答。给出具体、可操作的建议。`

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
