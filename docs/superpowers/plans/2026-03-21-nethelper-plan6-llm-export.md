# Nethelper Plan 6: LLM Integration + Export

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add LLM provider abstraction with OpenAI-compatible API support, implement `diagnose`, `explain`, `note extract`, and `config llm` commands, and build the `export db/topology/report` commands.

**Architecture:** The LLM package defines a `Provider` interface with `Chat()` and `Supports()`. An OpenAI-compatible implementation handles OpenAI, Anthropic (via compatible endpoints), Ollama, and any OpenAI-API-compatible service. A `Router` dispatches requests to the right provider based on capability type. CLI commands use the router to get LLM responses. All LLM features gracefully degrade when no provider is configured. Export commands serialize data from the store to files (SQLite copy, DOT graph, Markdown report).

**Tech Stack:** Go 1.24+, net/http (stdlib for API calls), encoding/json, existing store/graph/config packages

**Spec:** `docs/superpowers/specs/2026-03-21-network-helper-design.md` (Sections 6, 7, CLI §5)

**Depends on:** Plan 1-5 (all prior work)

**Note:** Embedding + sqlite-vec vector search is deferred to a follow-up plan. This plan implements the `EmbeddingProvider` interface and config, but vector storage/search requires sqlite-vec integration which needs separate evaluation. FTS5 remains the primary search engine.

---

## File Structure

```
internal/
├── llm/
│   ├── provider.go                # Provider interface, ChatRequest/Response, Capability enum
│   ├── openai.go                  # OpenAI-compatible provider (works with OpenAI/Ollama/DeepSeek/etc.)
│   ├── router.go                  # Router: dispatches to provider by capability
│   ├── provider_test.go           # Tests with mock provider
│   └── router_test.go            # Router tests
├── cli/
│   ├── diagnose.go                # New: diagnose command (LLM-powered troubleshoot advisor)
│   ├── explain.go                 # New: explain command (LLM-powered config/output interpreter)
│   ├── export.go                  # New: export db/topology/report
│   ├── llmconfig.go               # New: config llm command
│   ├── note.go                    # Modify: add note extract subcommand
│   └── root.go                    # Modify: register new commands, init LLM router
```

---

### Task 1: LLM Provider Interface + Types

**Files:**
- Create: `internal/llm/provider.go`
- Create: `internal/llm/provider_test.go`

- [ ] **Step 1: Write test**

```go
// internal/llm/provider_test.go
package llm

import (
	"context"
	"testing"
)

type mockProvider struct {
	name     string
	response string
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	return ChatResponse{Content: m.response}, nil
}
func (m *mockProvider) Supports(cap Capability) bool { return true }

func TestCapabilityString(t *testing.T) {
	tests := []struct{ cap Capability; want string }{
		{CapExtract, "extract"},
		{CapAnalyze, "analyze"},
		{CapExplain, "explain"},
		{CapParse, "parse"},
	}
	for _, tt := range tests {
		if string(tt.cap) != tt.want { t.Errorf("got %q, want %q", tt.cap, tt.want) }
	}
}

func TestChatRequestMessages(t *testing.T) {
	req := ChatRequest{
		Messages: []Message{
			{Role: "system", Content: "You are a network engineer."},
			{Role: "user", Content: "What is OSPF?"},
		},
	}
	if len(req.Messages) != 2 { t.Errorf("expected 2, got %d", len(req.Messages)) }
}

func TestMockProviderChat(t *testing.T) {
	p := &mockProvider{name: "mock", response: "test response"}
	resp, err := p.Chat(context.Background(), ChatRequest{})
	if err != nil { t.Fatalf("error: %v", err) }
	if resp.Content != "test response" { t.Errorf("got: %s", resp.Content) }
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/llm/ -v`
Expected: FAIL

- [ ] **Step 3: Implement provider.go**

```go
// internal/llm/provider.go
package llm

import "context"

// Capability represents what an LLM provider can do.
type Capability string

const (
	CapExtract Capability = "extract" // structured extraction from logs
	CapAnalyze Capability = "analyze" // troubleshooting analysis
	CapExplain Capability = "explain" // config/output explanation
	CapParse   Capability = "parse"   // parser fallback (L2.5)
)

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`    // "system", "user", "assistant"
	Content string `json:"content"`
}

// ChatRequest is the input to a chat completion.
type ChatRequest struct {
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

// ChatResponse is the output from a chat completion.
type ChatResponse struct {
	Content string `json:"content"`
}

// Provider is the interface for LLM backends.
type Provider interface {
	Name() string
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
	Supports(cap Capability) bool
}
```

- [ ] **Step 4: Run test — verify PASS**

Run: `go test ./internal/llm/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/llm/provider.go internal/llm/provider_test.go
git commit -m "feat: add LLM Provider interface with Capability types"
```

---

### Task 2: OpenAI-Compatible Provider

**Files:**
- Create: `internal/llm/openai.go`

- [ ] **Step 1: Add test to provider_test.go**

```go
func TestOpenAIProviderCreation(t *testing.T) {
	p := NewOpenAIProvider("test-key", "gpt-4o-mini", "")
	if p.Name() != "openai" { t.Errorf("name: %s", p.Name()) }
	if !p.Supports(CapAnalyze) { t.Error("should support analyze") }
}

func TestOpenAIProviderCustomName(t *testing.T) {
	p := NewOpenAICompatProvider("deepseek", "sk-xxx", "deepseek-chat", "https://api.deepseek.com")
	if p.Name() != "deepseek" { t.Errorf("name: %s", p.Name()) }
}

func TestOpenAIProviderOllama(t *testing.T) {
	p := NewOpenAICompatProvider("ollama", "", "qwen2.5:14b", "http://localhost:11434")
	if p.Name() != "ollama" { t.Errorf("name: %s", p.Name()) }
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/llm/ -v -run TestOpenAI`
Expected: FAIL

- [ ] **Step 3: Implement openai.go**

```go
// internal/llm/openai.go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

// OpenAIProvider implements Provider for any OpenAI-compatible API.
// Works with: OpenAI, Ollama (/v1 endpoint), DeepSeek, Groq, Together, etc.
type OpenAIProvider struct {
	name    string
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewOpenAIProvider creates a provider for the official OpenAI API.
func NewOpenAIProvider(apiKey, model, baseURL string) *OpenAIProvider {
	if baseURL == "" { baseURL = defaultOpenAIBaseURL }
	if model == "" { model = "gpt-4o-mini" }
	return &OpenAIProvider{name: "openai", apiKey: apiKey, model: model, baseURL: baseURL, client: &http.Client{}}
}

// NewOpenAICompatProvider creates a provider for any OpenAI-compatible API.
func NewOpenAICompatProvider(name, apiKey, model, baseURL string) *OpenAIProvider {
	if baseURL == "" { baseURL = defaultOpenAIBaseURL }
	// Ollama uses /v1 path
	if name == "ollama" && baseURL == "http://localhost:11434" {
		baseURL = "http://localhost:11434/v1"
	}
	return &OpenAIProvider{name: name, apiKey: apiKey, model: model, baseURL: baseURL, client: &http.Client{}}
}

func (p *OpenAIProvider) Name() string { return p.name }

func (p *OpenAIProvider) Supports(cap Capability) bool {
	return true // OpenAI-compatible APIs support all capabilities
}

type openAIRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *OpenAIProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	apiReq := openAIRequest{
		Model:       p.model,
		Messages:    req.Messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}
	if apiReq.Temperature == 0 { apiReq.Temperature = 0.3 }
	if apiReq.MaxTokens == 0 { apiReq.MaxTokens = 2048 }

	body, err := json.Marshal(apiReq)
	if err != nil { return ChatResponse{}, fmt.Errorf("marshal request: %w", err) }

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil { return ChatResponse{}, fmt.Errorf("create request: %w", err) }

	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil { return ChatResponse{}, fmt.Errorf("HTTP request: %w", err) }
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil { return ChatResponse{}, fmt.Errorf("read response: %w", err) }

	var apiResp openAIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return ChatResponse{}, fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Error != nil {
		return ChatResponse{}, fmt.Errorf("API error: %s", apiResp.Error.Message)
	}

	if len(apiResp.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("no choices in response")
	}

	return ChatResponse{Content: apiResp.Choices[0].Message.Content}, nil
}
```

- [ ] **Step 4: Run test — verify PASS**

Run: `go test ./internal/llm/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/llm/openai.go internal/llm/provider_test.go
git commit -m "feat: add OpenAI-compatible LLM provider (OpenAI/Ollama/DeepSeek/etc.)"
```

---

### Task 3: LLM Router — Capability-Based Dispatch

**Files:**
- Create: `internal/llm/router.go`
- Create: `internal/llm/router_test.go`

- [ ] **Step 1: Write test**

```go
// internal/llm/router_test.go
package llm

import (
	"context"
	"testing"
)

func TestRouterDispatch(t *testing.T) {
	r := NewRouter()
	r.SetDefault(&mockProvider{name: "default", response: "default answer"})
	r.SetForCapability(CapAnalyze, &mockProvider{name: "analyzer", response: "analysis result"})

	// Default provider for extract
	resp, err := r.Chat(context.Background(), CapExtract, ChatRequest{})
	if err != nil { t.Fatalf("error: %v", err) }
	if resp.Content != "default answer" { t.Errorf("got: %s", resp.Content) }

	// Specific provider for analyze
	resp, err = r.Chat(context.Background(), CapAnalyze, ChatRequest{})
	if err != nil { t.Fatalf("error: %v", err) }
	if resp.Content != "analysis result" { t.Errorf("got: %s", resp.Content) }
}

func TestRouterNoProvider(t *testing.T) {
	r := NewRouter()
	_, err := r.Chat(context.Background(), CapExtract, ChatRequest{})
	if err == nil { t.Error("expected error with no provider") }
}

func TestRouterAvailable(t *testing.T) {
	r := NewRouter()
	if r.Available() { t.Error("should not be available with no providers") }

	r.SetDefault(&mockProvider{name: "test"})
	if !r.Available() { t.Error("should be available with default provider") }
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/llm/ -v -run TestRouter`
Expected: FAIL

- [ ] **Step 3: Implement router.go**

```go
// internal/llm/router.go
package llm

import (
	"context"
	"fmt"

	"github.com/xavierli/nethelper/internal/config"
)

// Router dispatches LLM requests to the appropriate provider based on capability.
type Router struct {
	defaultProvider Provider
	capProviders    map[Capability]Provider
}

func NewRouter() *Router {
	return &Router{capProviders: make(map[Capability]Provider)}
}

func (r *Router) SetDefault(p Provider) {
	r.defaultProvider = p
}

func (r *Router) SetForCapability(cap Capability, p Provider) {
	r.capProviders[cap] = p
}

// Available returns true if at least one provider is configured.
func (r *Router) Available() bool {
	return r.defaultProvider != nil || len(r.capProviders) > 0
}

// Chat routes a request to the appropriate provider for the given capability.
func (r *Router) Chat(ctx context.Context, cap Capability, req ChatRequest) (ChatResponse, error) {
	p := r.providerFor(cap)
	if p == nil {
		return ChatResponse{}, fmt.Errorf("no LLM provider configured (run 'nethelper config llm' to set up)")
	}
	return p.Chat(ctx, req)
}

func (r *Router) providerFor(cap Capability) Provider {
	if p, ok := r.capProviders[cap]; ok && p.Supports(cap) {
		return p
	}
	if r.defaultProvider != nil {
		return r.defaultProvider
	}
	return nil
}

// BuildFromConfig creates a Router from the application config.
func BuildFromConfig(cfg config.LLMConfig) *Router {
	router := NewRouter()

	providers := make(map[string]Provider)
	for name, pc := range cfg.Providers {
		p := NewOpenAICompatProvider(name, pc.APIKey, pc.Model, pc.BaseURL)
		providers[name] = p
	}

	if def, ok := providers[cfg.Default]; ok {
		router.SetDefault(def)
	}

	// Map routing config to capabilities
	capMap := map[string]Capability{
		"extract": CapExtract,
		"analyze": CapAnalyze,
		"explain": CapExplain,
		"parse":   CapParse,
	}
	for capName, providerName := range cfg.Routing {
		cap, ok := capMap[capName]
		if !ok { continue }
		if p, ok := providers[providerName]; ok {
			router.SetForCapability(cap, p)
		}
	}

	return router
}
```

- [ ] **Step 4: Run test — verify PASS**

Run: `go test ./internal/llm/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/llm/router.go internal/llm/router_test.go
git commit -m "feat: add LLM router with capability-based dispatch"
```

---

### Task 4: CLI — diagnose + explain + note extract

**Files:**
- Create: `internal/cli/diagnose.go`
- Create: `internal/cli/explain.go`
- Modify: `internal/cli/note.go` (add extract subcommand)
- Modify: `internal/cli/root.go` (init LLM router, register commands)

- [ ] **Step 1: Update root.go — init LLM router**

Add import `"github.com/xavierli/nethelper/internal/llm"` and add `var llmRouter *llm.Router` to the var block.

In `PersistentPreRunE`, after the pipeline init, add:

```go
		// Initialize LLM router from config
		llmRouter = llm.BuildFromConfig(cfg.LLM)
```

Add command registrations:

```go
	root.AddCommand(newDiagnoseCmd())
	root.AddCommand(newExplainCmd())
	root.AddCommand(newConfigCmd())
	root.AddCommand(newExportCmd())
```

- [ ] **Step 2: Implement diagnose.go**

```go
// internal/cli/diagnose.go
package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/llm"
)

func newDiagnoseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diagnose <description>",
		Short: "Get AI-powered troubleshooting advice",
		Long:  "Searches historical troubleshooting notes and uses LLM to provide contextual advice. Falls back to FTS5 search without LLM.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]

			// Always search history first (works without LLM)
			logs, err := db.SearchTroubleshootLogs(query)
			if err != nil { logs = nil } // degrade gracefully

			if len(logs) > 0 {
				fmt.Println("📋 Related troubleshooting history:\n")
				for _, l := range logs {
					fmt.Printf("  [%s] %s\n", l.Tags, l.Symptom)
					if l.Resolution != "" { fmt.Printf("    → %s\n", l.Resolution) }
				}
				fmt.Println()
			}

			// If LLM available, get AI analysis
			if !llmRouter.Available() {
				if len(logs) == 0 {
					fmt.Println("No matching history found. Configure an LLM provider for AI-powered diagnosis:")
					fmt.Println("  nethelper config llm --help")
				}
				return nil
			}

			fmt.Println("🤖 AI Analysis:\n")

			// Build context from history
			var historyCtx string
			if len(logs) > 0 {
				var parts []string
				for _, l := range logs {
					parts = append(parts, fmt.Sprintf("- Symptom: %s / Finding: %s / Resolution: %s", l.Symptom, l.Findings, l.Resolution))
				}
				historyCtx = "Related past issues:\n" + strings.Join(parts, "\n")
			}

			systemPrompt := `You are a senior network engineer assistant. Analyze the problem description and provide:
1. Possible root causes (ranked by likelihood)
2. Recommended troubleshooting commands to run
3. If similar past issues are provided, reference them in your analysis
Reply in the same language as the user's input.`

			userMsg := fmt.Sprintf("Problem: %s", query)
			if historyCtx != "" { userMsg += "\n\n" + historyCtx }

			resp, err := llmRouter.Chat(context.Background(), llm.CapAnalyze, llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "system", Content: systemPrompt},
					{Role: "user", Content: userMsg},
				},
			})
			if err != nil {
				fmt.Printf("LLM error: %v\n", err)
				return nil // degrade gracefully
			}

			fmt.Println(resp.Content)
			return nil
		},
	}
}
```

- [ ] **Step 3: Implement explain.go**

```go
// internal/cli/explain.go
package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/llm"
)

func newExplainCmd() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "explain [text-or-command]",
		Short: "AI-powered explanation of config or command output",
		Long:  "Explain network device configuration or command output. Reads from argument, --file, or stdin.",
		RunE: func(cmd *cobra.Command, args []string) error {
			var text string
			if file != "" {
				data, err := os.ReadFile(file)
				if err != nil { return fmt.Errorf("read file: %w", err) }
				text = string(data)
			} else if len(args) > 0 {
				text = args[0]
			} else {
				return fmt.Errorf("provide text as argument or use --file")
			}

			if !llmRouter.Available() {
				fmt.Println("No LLM provider configured. Showing raw text:")
				fmt.Println(text)
				fmt.Println("\nConfigure an LLM for AI explanations: nethelper config llm --help")
				return nil
			}

			systemPrompt := `You are a senior network engineer. Explain the following network device configuration or command output.
Focus on:
1. What each section/line does
2. Key values and what they mean
3. Any potential issues or notable configurations
Reply in the same language as the user's input. Be concise and practical.`

			resp, err := llmRouter.Chat(context.Background(), llm.CapExplain, llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "system", Content: systemPrompt},
					{Role: "user", Content: text},
				},
			})
			if err != nil { return fmt.Errorf("LLM error: %w", err) }

			fmt.Println(resp.Content)
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "read from file instead of argument")
	return cmd
}
```

- [ ] **Step 4: Add note extract to note.go**

Add this function and register it in `newNoteCmd`:

```go
func newNoteExtractCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "extract <file>",
		Short: "Extract troubleshooting experience from log file using AI",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil { return fmt.Errorf("read file: %w", err) }

			if !llmRouter.Available() {
				fmt.Println("No LLM provider configured. Use 'nethelper note add' to manually record notes.")
				return nil
			}

			systemPrompt := `Analyze this network device troubleshooting log and extract structured information.
Return a JSON object with these fields:
- symptom: problem description
- commands_used: commands executed during troubleshooting
- findings: what was discovered
- resolution: how it was fixed (if apparent)
- tags: comma-separated relevant tags (e.g., ospf,mtu,flap)
Reply only with the JSON object, no markdown.`

			resp, err := llmRouter.Chat(context.Background(), llm.CapExtract, llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "system", Content: systemPrompt},
					{Role: "user", Content: string(data)},
				},
			})
			if err != nil { return fmt.Errorf("LLM error: %w", err) }

			fmt.Println("Extracted troubleshooting note:")
			fmt.Println(resp.Content)
			fmt.Println("\nTo save this note, use: nethelper note add --symptom '...' --finding '...' --resolution '...' --tags '...'")
			return nil
		},
	}
}
```

In `newNoteCmd`, add: `note.AddCommand(newNoteExtractCmd())`

Also add import for `"os"` if not already present in note.go, and make `llmRouter` accessible (it's already a package-level var in root.go).

- [ ] **Step 5: Build and verify**

Run: `go build ./cmd/nethelper`
Run: `./nethelper diagnose --help`
Run: `./nethelper explain --help`
Run: `./nethelper note extract --help`

- [ ] **Step 6: Commit**

```bash
git add internal/cli/diagnose.go internal/cli/explain.go internal/cli/note.go internal/cli/root.go
git commit -m "feat: add diagnose, explain, and note extract commands with LLM integration"
```

---

### Task 5: CLI — config llm

**Files:**
- Create: `internal/cli/llmconfig.go`

- [ ] **Step 1: Implement llmconfig.go**

```go
// internal/cli/llmconfig.go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	config := &cobra.Command{
		Use:   "config",
		Short: "Configuration management",
	}
	config.AddCommand(newConfigLLMCmd())
	return config
}

func newConfigLLMCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "llm",
		Short: "Show LLM provider configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("LLM Configuration:")
			fmt.Printf("  Config file: %s\n\n", cfgFile)

			if cfg.LLM.Default != "" {
				fmt.Printf("  Default provider: %s\n", cfg.LLM.Default)
			} else {
				fmt.Println("  Default provider: (none)")
			}

			if len(cfg.LLM.Providers) > 0 {
				fmt.Println("\n  Providers:")
				for name, pc := range cfg.LLM.Providers {
					model := pc.Model
					if model == "" { model = "(default)" }
					baseURL := pc.BaseURL
					if baseURL == "" { baseURL = "https://api.openai.com/v1" }
					hasKey := "no"
					if pc.APIKey != "" { hasKey = "yes" }
					fmt.Printf("    %s: model=%s base_url=%s api_key=%s\n", name, model, baseURL, hasKey)
				}
			} else {
				fmt.Println("\n  No providers configured.")
			}

			if len(cfg.LLM.Routing) > 0 {
				fmt.Println("\n  Capability routing:")
				for cap, provider := range cfg.LLM.Routing {
					fmt.Printf("    %s → %s\n", cap, provider)
				}
			}

			if !llmRouter.Available() {
				fmt.Println("\n  ⚠ No LLM provider is active. To configure, edit:")
				fmt.Printf("    %s\n\n", cfgFile)
				fmt.Println("  Example config:")
				fmt.Println("    llm:")
				fmt.Println("      default: ollama")
				fmt.Println("      providers:")
				fmt.Println("        ollama:")
				fmt.Println("          base_url: http://localhost:11434")
				fmt.Println("          model: qwen2.5:14b")
			} else {
				fmt.Println("\n  ✓ LLM provider is active.")
			}

			return nil
		},
	}
}
```

- [ ] **Step 2: Build and verify**

Run: `go build ./cmd/nethelper && ./nethelper config llm`
Expected: shows config with "No providers configured" and example

- [ ] **Step 3: Commit**

```bash
git add internal/cli/llmconfig.go
git commit -m "feat: add config llm command to show LLM provider status"
```

---

### Task 6: CLI — export db/topology/report

**Files:**
- Create: `internal/cli/export.go`

- [ ] **Step 1: Implement export.go**

```go
// internal/cli/export.go
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/graph"
)

func newExportCmd() *cobra.Command {
	export := &cobra.Command{
		Use:   "export",
		Short: "Export data for backup or migration",
	}
	export.AddCommand(newExportDBCmd())
	export.AddCommand(newExportTopologyCmd())
	export.AddCommand(newExportReportCmd())
	return export
}

func newExportDBCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Export SQLite database (copy)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if output == "" {
				output = fmt.Sprintf("nethelper-backup-%s.db", time.Now().Format("20060102-150405"))
			}

			src, err := os.Open(db.Path())
			if err != nil { return fmt.Errorf("open source: %w", err) }
			defer src.Close()

			dst, err := os.Create(output)
			if err != nil { return fmt.Errorf("create output: %w", err) }
			defer dst.Close()

			n, err := io.Copy(dst, src)
			if err != nil { return fmt.Errorf("copy: %w", err) }

			fmt.Printf("Exported database to %s (%d bytes)\n", output, n)
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "output file path")
	return cmd
}

func newExportTopologyCmd() *cobra.Command {
	var format, output string
	cmd := &cobra.Command{
		Use:   "topology",
		Short: "Export network topology",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := graph.BuildFromDB(db)
			if err != nil { return fmt.Errorf("build graph: %w", err) }

			var content string
			switch format {
			case "dot":
				content = exportDOT(g)
			case "json":
				content = exportJSON(g)
			default:
				content = exportDOT(g)
			}

			if output != "" {
				if err := os.WriteFile(output, []byte(content), 0644); err != nil {
					return fmt.Errorf("write file: %w", err)
				}
				fmt.Printf("Exported topology to %s (%s format)\n", output, format)
			} else {
				fmt.Print(content)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "dot", "output format (dot, json)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "output file path")
	return cmd
}

func exportDOT(g *graph.Graph) string {
	var sb strings.Builder
	sb.WriteString("digraph network {\n")
	sb.WriteString("  rankdir=LR;\n")
	sb.WriteString("  node [shape=box];\n\n")

	// Device nodes
	for _, n := range g.NodesByType(graph.NodeTypeDevice) {
		label := n.Props["hostname"]
		if label == "" { label = n.ID }
		sb.WriteString(fmt.Sprintf("  %q [label=%q shape=box style=filled fillcolor=lightblue];\n",
			n.ID, label))
	}
	sb.WriteString("\n")

	// PEER edges only (device-to-device)
	seen := make(map[string]bool)
	for _, n := range g.NodesByType(graph.NodeTypeDevice) {
		for _, e := range g.NeighborsByType(n.ID, graph.EdgePeer) {
			key := n.ID + "-" + e.To
			reverseKey := e.To + "-" + n.ID
			if seen[key] || seen[reverseKey] { continue }
			seen[key] = true
			label := e.Props["protocol"]
			sb.WriteString(fmt.Sprintf("  %q -> %q [label=%q dir=none];\n", n.ID, e.To, label))
		}
	}

	sb.WriteString("}\n")
	return sb.String()
}

func exportJSON(g *graph.Graph) string {
	type jsonNode struct {
		ID    string            `json:"id"`
		Type  string            `json:"type"`
		Props map[string]string `json:"props"`
	}
	type jsonEdge struct {
		From  string            `json:"from"`
		To    string            `json:"to"`
		Type  string            `json:"type"`
		Props map[string]string `json:"props,omitempty"`
	}
	type jsonGraph struct {
		Nodes []jsonNode `json:"nodes"`
		Edges []jsonEdge `json:"edges"`
	}

	var jg jsonGraph
	for _, n := range g.NodesByType(graph.NodeTypeDevice) {
		jg.Nodes = append(jg.Nodes, jsonNode{ID: n.ID, Type: string(n.Type), Props: n.Props})
	}
	for _, n := range g.NodesByType(graph.NodeTypeDevice) {
		for _, e := range g.NeighborsByType(n.ID, graph.EdgePeer) {
			jg.Edges = append(jg.Edges, jsonEdge{From: n.ID, To: e.To, Type: string(e.Type), Props: e.Props})
		}
	}

	data, _ := json.MarshalIndent(jg, "", "  ")
	return string(data) + "\n"
}

func newExportReportCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate network status report (Markdown)",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := graph.BuildFromDB(db)
			if err != nil { return fmt.Errorf("build graph: %w", err) }

			devices, _ := db.ListDevices()
			var sb strings.Builder

			sb.WriteString("# Network Status Report\n\n")
			sb.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))

			// Summary
			sb.WriteString("## Summary\n\n")
			sb.WriteString(fmt.Sprintf("- **Devices:** %d\n", len(devices)))
			sb.WriteString(fmt.Sprintf("- **Interfaces:** %d\n", len(g.NodesByType(graph.NodeTypeInterface))))
			sb.WriteString(fmt.Sprintf("- **Subnets:** %d\n", len(g.NodesByType(graph.NodeTypeSubnet))))
			sb.WriteString(fmt.Sprintf("- **Graph edges:** %d\n\n", g.EdgeCount()))

			// Devices
			sb.WriteString("## Devices\n\n")
			sb.WriteString("| Hostname | Vendor | Model | Mgmt IP | Router-ID | Last Seen |\n")
			sb.WriteString("|----------|--------|-------|---------|-----------|-----------|\n")
			for _, d := range devices {
				sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
					d.Hostname, d.Vendor, d.Model, d.MgmtIP, d.RouterID, d.LastSeen.Format("2006-01-02 15:04")))
			}
			sb.WriteString("\n")

			// SPOFs
			spofs := graph.FindSPOF(g, graph.NodeTypeDevice)
			if len(spofs) > 0 {
				sb.WriteString("## ⚠ Single Points of Failure\n\n")
				for _, s := range spofs {
					n, _ := g.GetNode(s)
					hostname := s
					if n != nil && n.Props["hostname"] != "" { hostname = n.Props["hostname"] }
					sb.WriteString(fmt.Sprintf("- %s\n", hostname))
				}
				sb.WriteString("\n")
			}

			content := sb.String()
			if output != "" {
				if err := os.WriteFile(output, []byte(content), 0644); err != nil {
					return fmt.Errorf("write file: %w", err)
				}
				fmt.Printf("Report exported to %s\n", output)
			} else {
				fmt.Print(content)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "output file path")
	return cmd
}
```

- [ ] **Step 2: Build and verify**

Run: `go build ./cmd/nethelper`
Run: `./nethelper export --help` → shows db/topology/report
Run: `./nethelper export topology --help` → shows format/output flags

- [ ] **Step 3: Run full test suite**

Run: `go test ./... -timeout 60s`
Expected: all PASS

- [ ] **Step 4: Commit**

```bash
git add internal/cli/export.go
git commit -m "feat: add export db/topology/report commands"
```

- [ ] **Step 5: Clean up**

Run: `rm -f nethelper`
