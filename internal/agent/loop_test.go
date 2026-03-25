package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/xavierli/nethelper/internal/config"
	"github.com/xavierli/nethelper/internal/llm"
)

type mockProvider struct {
	name      string
	responses []llm.ChatResponse
	callCount int
	err       error
}

func (m *mockProvider) Name() string {
	if m.name != "" {
		return m.name
	}
	return "mock"
}

func (m *mockProvider) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	if m.err != nil {
		return llm.ChatResponse{}, m.err
	}
	if m.callCount < len(m.responses) {
		resp := m.responses[m.callCount]
		m.callCount++
		return resp, nil
	}
	return llm.ChatResponse{Content: "default response"}, nil
}

func (m *mockProvider) Supports(cap llm.Capability) bool {
	return true
}

type mockLogger struct{}

func (m *mockLogger) Log(userKey string, event SessionEvent) {}
func (m *mockLogger) Save(userKey string) error              { return nil }

func TestNew(t *testing.T) {
	router := llm.NewRouter()
	router.SetDefault(&mockProvider{})

	registry := NewRegistry()

	t.Run("basic creation", func(t *testing.T) {
		agent := New(router, registry, nil, nil)
		if agent == nil {
			t.Fatal("expected non-nil agent")
		}
		if agent.sessionID == "" {
			t.Error("session ID not generated")
		}
		if len(agent.messages) != 1 {
			t.Errorf("expected 1 system message, got %d", len(agent.messages))
		}
	})

	t.Run("with options", func(t *testing.T) {
		opt := AgentOptions{
			UserKey: "test-user",
			ContextCfg: config.ContextConfig{
				MaxTokenBudget:   10000,
				ToolResultMaxLen: 1000,
			},
		}
		agent := New(router, registry, nil, nil, opt)

		if agent.userKey != "test-user" {
			t.Errorf("expected userKey 'test-user', got %s", agent.userKey)
		}
		if agent.contextCfg.MaxTokenBudget != 10000 {
			t.Errorf("expected MaxTokenBudget 10000, got %d", agent.contextCfg.MaxTokenBudget)
		}
	})
}

func TestAgent_Chat_SimpleResponse(t *testing.T) {
	router := llm.NewRouter()
	router.SetDefault(&mockProvider{
		responses: []llm.ChatResponse{
			{Content: "Hello, I can help you with network troubleshooting."},
		},
	})

	registry := NewRegistry()
	agent := New(router, registry, nil, nil)

	ctx := context.Background()
	response, err := agent.Chat(ctx, "Hi", nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if response == "" {
		t.Error("expected non-empty response")
	}
}

func TestAgent_Chat_WithToolCall(t *testing.T) {
	toolCalled := false

	router := llm.NewRouter()
	router.SetDefault(&mockProvider{
		responses: []llm.ChatResponse{
			{
				ToolCalls: []llm.ToolCall{
					{
						ID:        "call_1",
						Name:      "test_tool",
						Arguments: map[string]interface{}{"param": "value"},
					},
				},
			},
			{Content: "Tool executed successfully."},
		},
	})

	registry := NewRegistry()
	registry.Register(Tool{
		Name:        "test_tool",
		Description: "A test tool",
		Handler: func(args map[string]interface{}) (string, error) {
			toolCalled = true
			return "Tool result", nil
		},
	})

	agent := New(router, registry, nil, nil)

	ctx := context.Background()
	_, err := agent.Chat(ctx, "Use the tool", nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !toolCalled {
		t.Error("tool was not called")
	}
}

func TestAgent_Chat_LLMError(t *testing.T) {
	router := llm.NewRouter()
	router.SetDefault(&mockProvider{
		err: errors.New("LLM service unavailable"),
	})

	registry := NewRegistry()
	agent := New(router, registry, nil, nil)

	ctx := context.Background()
	_, err := agent.Chat(ctx, "Hi", nil)

	if err == nil {
		t.Error("expected error when LLM fails")
	}
}

func TestCompactContext_TruncateStrategy(t *testing.T) {
	router := llm.NewRouter()
	router.SetDefault(&mockProvider{})

	registry := NewRegistry()
	agent := New(router, registry, nil, nil, AgentOptions{
		ContextCfg: config.ContextConfig{
			MaxTokenBudget:   1000,
			ToolResultMaxLen: 100,
		},
	})

	longContent := make([]byte, 5000)
	for i := range longContent {
		longContent[i] = 'a'
	}
	agent.messages = append(agent.messages, llm.Message{
		Role:    "user",
		Content: string(longContent),
	})

	agent.compactContext()

	if len(agent.messages[1].Content) > 5000 {
		t.Error("message not truncated")
	}
}

func TestCompactContext_EvictStrategy(t *testing.T) {
	router := llm.NewRouter()
	router.SetDefault(&mockProvider{})

	registry := NewRegistry()
	agent := New(router, registry, nil, nil, AgentOptions{
		ContextCfg: config.ContextConfig{
			MaxTokenBudget:   200,
			ToolResultMaxLen: 100,
		},
	})

	agent.messages = []llm.Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "message 1"},
		{Role: "assistant", Content: "response 1"},
		{Role: "user", Content: "message 2"},
		{Role: "assistant", Content: "response 2"},
		{Role: "user", Content: "message 3"},
		{Role: "assistant", Content: "response 3"},
	}

	agent.compactContext()

	if len(agent.messages) < 3 {
		t.Errorf("expected at least 3 messages, got %d", len(agent.messages))
	}

	if agent.messages[0].Role != "system" {
		t.Error("first message should be system role")
	}
}

func TestGenerateSessionID(t *testing.T) {
	id1 := generateSessionID()

	if id1 == "" {
		t.Error("session ID should not be empty")
	}

	time.Sleep(time.Microsecond)
	id2 := generateSessionID()
	if id1 == id2 {
		t.Error("session IDs should be unique when called with time gap")
	}
}

func TestReadFileOrDefault(t *testing.T) {
	content := "test content"
	result := readFileOrDefault("/nonexistent/path", content)
	if result != content {
		t.Error("should return fallback for nonexistent file")
	}
}

func TestWriteIfNotExist(t *testing.T) {
	t.Run("write new file", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := tmpDir + "/test.md"

		err := writeIfNotExist(path, "test content")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content := readFileOrDefault(path, "fallback")
		if content != "test content" {
			t.Error("file content mismatch")
		}
	})

	t.Run("skip existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := tmpDir + "/existing.md"

		writeIfNotExist(path, "original")
		err := writeIfNotExist(path, "new content")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content := readFileOrDefault(path, "fallback")
		if content != "original" {
			t.Error("existing file should not be overwritten")
		}
	})
}

func TestEnsureDefaultFiles(t *testing.T) {
	tmpDir := t.TempDir()

	err := EnsureDefaultFiles(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	files := []string{"SOUL.md", "IDENTITY.md", "TOOLS.md"}
	for _, file := range files {
		content := readFileOrDefault(tmpDir+"/"+file, "")
		if content == "" {
			t.Errorf("%s was not created", file)
		}
	}
}

func TestLoadSystemPrompt(t *testing.T) {
	t.Run("with default files", func(t *testing.T) {
		tmpDir := t.TempDir()
		EnsureDefaultFiles(tmpDir)

		prompt := LoadSystemPrompt(tmpDir)
		if prompt == "" {
			t.Error("system prompt should not be empty")
		}
	})

	t.Run("with empty dir", func(t *testing.T) {
		prompt := LoadSystemPrompt("")
		if prompt == "" {
			t.Error("should return built-in defaults for empty path")
		}
	})
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"this is a long string", 10, "this is a …"},
		{"", 10, ""},
		{"exact", 5, "exact"},
	}

	for _, tt := range tests {
		result := truncate(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncate(%q, %d) = %q, expected %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()

	t.Run("register and get", func(t *testing.T) {
		tool := Tool{
			Name:        "test",
			Description: "test tool",
			Handler:     func(args map[string]interface{}) (string, error) { return "ok", nil },
		}
		r.Register(tool)

		got, ok := r.Get("test")
		if !ok {
			t.Error("should find registered tool")
		}
		if got.Name != "test" {
			t.Error("tool name mismatch")
		}
	})

	t.Run("names order", func(t *testing.T) {
		r2 := NewRegistry()
		r2.Register(Tool{Name: "first"})
		r2.Register(Tool{Name: "second"})
		r2.Register(Tool{Name: "third"})

		names := r2.Names()
		if len(names) != 3 {
			t.Errorf("expected 3 names, got %d", len(names))
		}
		if names[0] != "first" || names[1] != "second" || names[2] != "third" {
			t.Error("names not in registration order")
		}
	})
}
