package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/xavierli/nethelper/internal/config"
	"github.com/xavierli/nethelper/internal/llm"
	"github.com/xavierli/nethelper/internal/memory"
	"github.com/xavierli/nethelper/internal/store"
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
- 你的回复会自动渲染为富文本（Markdown、卡片等），你不需要关心消息格式，正常用 Markdown 写回复即可
- 不要说"我无法发送卡片/图片/富文本"——系统会自动处理格式
- 工具返回的数据可能很长，总结关键信息给用户，不要原样输出`

// AgentOptions carries optional configuration for an Agent.
// Use the zero value for sensible defaults (no logging, default context limits).
type AgentOptions struct {
	// Logger, when non-nil, writes JSONL audit events for this agent's user.
	Logger *SessionLogger
	// UserKey is the stable identifier of the user who owns this session
	// (e.g. Feishu open_id, Discord user ID).  Used as the log file key.
	UserKey string
	// ContextCfg controls the multi-strategy context compression engine.
	// Zero value applies built-in defaults (50 000 char budget, 2 000 char tool cap).
	ContextCfg config.ContextConfig
}

// Agent orchestrates the LLM + tool calling loop.
type Agent struct {
	router         *llm.Router
	registry       *Registry
	embedder       llm.Embedder // optional: nil disables memory features
	db             *store.DB    // optional: nil disables memory features
	messages       []llm.Message
	sessionID      string
	memoryInjected bool // Issue #4: inject memory only once per session
	logger         *SessionLogger
	userKey        string
	contextCfg     config.ContextConfig
}

// New creates an Agent. embedder and db may be nil to disable vector memory.
// opts is variadic so callers that don't need options can call New(r, reg, e, db).
func New(router *llm.Router, registry *Registry, embedder llm.Embedder, db *store.DB, opts ...AgentOptions) *Agent {
	var opt AgentOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	// Apply defaults for zero-value context config fields.
	if opt.ContextCfg.MaxTokenBudget == 0 {
		opt.ContextCfg.MaxTokenBudget = 50000
	}
	if opt.ContextCfg.ToolResultMaxLen == 0 {
		opt.ContextCfg.ToolResultMaxLen = 2000
	}
	return &Agent{
		router:     router,
		registry:   registry,
		embedder:   embedder,
		db:         db,
		sessionID:  generateSessionID(),
		logger:     opt.Logger,
		userKey:    opt.UserKey,
		contextCfg: opt.ContextCfg,
		messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
		},
	}
}

// generateSessionID returns a simple nanosecond-based session identifier.
func generateSessionID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// compactContext applies a multi-strategy context compression pipeline.
//
// Strategy 1 — Tool result truncation: any tool message whose content exceeds
// cfg.ToolResultMaxLen is trimmed to keep the first and last halves joined by
// a truncation notice.  This prevents a single chatty tool from monopolising
// the context window.
//
// Strategy 2 — Token budget eviction: if the total character count of all
// messages still exceeds cfg.MaxTokenBudget, the oldest non-system messages
// (starting at index 1) are removed one at a time until the budget is met or
// only three messages remain (system + 1 user/assistant pair).
func (a *Agent) compactContext() {
	cfg := a.contextCfg

	// Strategy 1: truncate oversized tool results.
	for i := range a.messages {
		if a.messages[i].Role == "tool" && len(a.messages[i].Content) > cfg.ToolResultMaxLen {
			content := a.messages[i].Content
			half := cfg.ToolResultMaxLen / 2
			a.messages[i].Content = content[:half] + "\n...(truncated)...\n" + content[len(content)-half:]
		}
	}

	// Strategy 2: evict oldest messages when total length exceeds budget.
	totalLen := 0
	for _, m := range a.messages {
		totalLen += len(m.Content)
	}
	for totalLen > cfg.MaxTokenBudget && len(a.messages) > 3 {
		removed := a.messages[1]
		a.messages = append(a.messages[:1], a.messages[2:]...)
		totalLen -= len(removed.Content)
	}
}

// Chat sends a user message and runs the agent loop until LLM produces a final text response.
// It calls onToolCall for each tool invocation (for REPL display).
func (a *Agent) Chat(ctx context.Context, userInput string, onToolCall func(name string, args map[string]interface{})) (string, error) {
	// Issue #2: trim context if it has grown too large
	a.compactContext()

	// Log the incoming user message.
	a.logger.Log(a.userKey, SessionEvent{
		Type:    "user",
		Content: userInput,
	})

	// Issue #4: inject relevant memories only once per session (not every message)
	if !a.memoryInjected && a.embedder != nil && a.db != nil {
		a.memoryInjected = true
		embedCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		vec, err := a.embedder.Embed(embedCtx, userInput)
		cancel()
		if err == nil {
			memories, _ := memory.Search(a.db, vec, 3)
			if len(memories) > 0 {
				var sb strings.Builder
				sb.WriteString("## 相关历史记忆\n")
				for _, m := range memories {
					sb.WriteString(fmt.Sprintf("- [%s] %s\n", m.CreatedAt.Format("2006-01-02"), m.Content))
				}
				a.messages = append(a.messages, llm.Message{Role: "system", Content: sb.String()})
				a.logger.Log(a.userKey, SessionEvent{
					Type:    "memory",
					Content: sb.String(),
				})
			}
		}
		// Silently ignore embedding errors — memory is an enhancement, not critical
	}

	a.messages = append(a.messages, llm.Message{Role: "user", Content: userInput})

	maxIterations := 20 // safety limit
	for i := 0; i < maxIterations; i++ {
		resp, err := a.router.Chat(ctx, llm.CapAnalyze, llm.ChatRequest{
			Messages: a.messages,
			Tools:    a.registry.ToolDefs(),
		})
		if err != nil {
			a.logger.Log(a.userKey, SessionEvent{
				Type:    "error",
				Content: err.Error(),
			})
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

				// Log the tool call.
				a.logger.Log(a.userKey, SessionEvent{
					Type:     "tool_call",
					ToolName: tc.Name,
					ToolArgs: tc.Arguments,
				})

				tool, ok := a.registry.Get(tc.Name)
				var result string
				toolStart := time.Now()
				if !ok {
					result = fmt.Sprintf("Unknown tool: %s", tc.Name)
				} else {
					var execErr error
					result, execErr = tool.Handler(tc.Arguments)
					if execErr != nil {
						result = fmt.Sprintf("Error: %v", execErr)
					}
				}
				toolDuration := time.Since(toolStart).Milliseconds()

				// Truncate very long results to avoid context overflow
				if len(result) > 8000 {
					result = result[:8000] + "\n... (truncated)"
				}

				// Log the tool result.
				a.logger.Log(a.userKey, SessionEvent{
					Type:       "tool_result",
					ToolName:   tc.Name,
					Content:    result,
					DurationMs: toolDuration,
				})

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

		// Log the final assistant response.
		a.logger.Log(a.userKey, SessionEvent{
			Type:    "assistant",
			Content: resp.Content,
		})

		return resp.Content, nil
	}

	return "", fmt.Errorf("agent loop exceeded %d iterations", maxIterations)
}

// SaveMemory summarizes the current conversation with the LLM, embeds the
// summary, and stores it as a memory entry. It is a best-effort operation —
// any error is silently discarded.
func (a *Agent) SaveMemory(ctx context.Context) {
	if a.embedder == nil || a.db == nil {
		return
	}
	// Need at least system + user + assistant (3 messages) to be worth saving
	if len(a.messages) <= 2 {
		return
	}

	// Ask LLM to produce a concise summary of the conversation
	summary, err := a.router.Chat(ctx, llm.CapAnalyze, llm.ChatRequest{
		Messages: []llm.Message{
			{
				Role:    "system",
				Content: "用一两句话总结以下对话的关键内容（设备名、操作、结论）。只输出总结，不要其他内容。",
			},
			{
				Role:    "user",
				Content: formatConversation(a.messages),
			},
		},
	})
	if err != nil {
		return
	}

	embedCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	vec, err := a.embedder.Embed(embedCtx, summary.Content)
	cancel()
	if err != nil {
		return
	}

	_ = memory.Insert(a.db, "conversation", summary.Content, a.sessionID, vec)
}

// Reset clears conversation history (keeps system prompt).
func (a *Agent) Reset() {
	a.messages = a.messages[:1] // keep system prompt
	a.memoryInjected = false
}

// SaveConversation persists current messages to DB so they survive session expiry / process restart.
func (a *Agent) SaveConversation(userKey string) {
	if a.db == nil {
		return
	}
	data, _ := json.Marshal(a.messages)
	a.db.Exec(
		`INSERT INTO conversations (user_key, messages, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(user_key) DO UPDATE SET messages=excluded.messages, updated_at=CURRENT_TIMESTAMP`,
		userKey, string(data),
	)
}

// LoadConversation restores messages from DB, replacing the default system-only history.
func (a *Agent) LoadConversation(userKey string) {
	if a.db == nil {
		return
	}
	var data string
	err := a.db.QueryRow(`SELECT messages FROM conversations WHERE user_key = ?`, userKey).Scan(&data)
	if err != nil {
		return
	}
	var msgs []llm.Message
	if json.Unmarshal([]byte(data), &msgs) == nil && len(msgs) > 0 {
		a.messages = msgs
	}
}

// formatConversation renders the message history as plain text for the summarizer.
// It skips system messages and the injected memory block to keep the input clean.
func formatConversation(messages []llm.Message) string {
	var sb strings.Builder
	for _, m := range messages {
		switch m.Role {
		case "user":
			sb.WriteString("User: ")
			sb.WriteString(m.Content)
			sb.WriteString("\n")
		case "assistant":
			if m.Content != "" {
				sb.WriteString("Assistant: ")
				sb.WriteString(m.Content)
				sb.WriteString("\n")
			}
		}
	}
	return sb.String()
}
