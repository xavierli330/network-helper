package channel

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/xavierli/nethelper/internal/agent"
	"github.com/xavierli/nethelper/internal/config"
	"github.com/xavierli/nethelper/internal/llm"
	"github.com/xavierli/nethelper/internal/parser"
	"github.com/xavierli/nethelper/internal/store"
)

// maxIMMessageLen is the character threshold above which a response is truncated
// for IM delivery. Long outputs (e.g. plan_isolate) would otherwise be split
// into dozens of messages by IM platforms.
const maxIMMessageLen = 3000

type Router struct {
	db            *store.DB
	pipeline      *parser.Pipeline
	llmRouter     *llm.Router
	embedder      llm.Embedder
	permissions   *PermissionConfig
	mu            sync.Mutex
	sessions      map[string]*userSession
	sessionLogger *agent.SessionLogger
	contextCfg    config.ContextConfig
}

// userSession holds per-user state. mu serialises concurrent messages from the
// same user so that agent.messages is never accessed by two goroutines at once
// (Issue #1: DATA RACE fix).
type userSession struct {
	agent      *agent.Agent
	group      *PermissionGroup
	lastActive time.Time
	mu         sync.Mutex // serialize messages for this user
}

func NewRouter(db *store.DB, pipeline *parser.Pipeline, llmRouter *llm.Router, embedder llm.Embedder, perms *PermissionConfig, opts ...RouterOptions) *Router {
	var opt RouterOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	r := &Router{
		db:            db,
		pipeline:      pipeline,
		llmRouter:     llmRouter,
		embedder:      embedder,
		permissions:   perms,
		sessions:      make(map[string]*userSession),
		sessionLogger: opt.SessionLogger,
		contextCfg:    opt.ContextCfg,
	}
	go r.cleanupLoop()
	return r
}

// RouterOptions carries optional configuration for the channel Router.
type RouterOptions struct {
	// SessionLogger, when non-nil, records JSONL audit events for every user session.
	SessionLogger *agent.SessionLogger
	// ContextCfg controls agent context compression. Zero value uses agent defaults.
	ContextCfg config.ContextConfig
}

func (r *Router) Handle(ctx context.Context, msg InMessage) string {
	userKey := msg.UserKey()
	if msg.IsGroup && !msg.MentionedBot {
		return ""
	}

	group := r.permissions.Resolve(userKey)
	if group == nil {
		return "⚠️ 你没有权限使用此 bot。"
	}

	// Retrieve or create session under the router lock, then release immediately
	// so concurrent users don't block each other.
	r.mu.Lock()
	sess, exists := r.sessions[userKey]
	if !exists || time.Since(sess.lastActive) > 30*time.Minute {
		// Save the expiring session's conversation before replacing it
		if exists {
			sess.agent.SaveConversation(userKey)
		}
		reg := agent.NewRegistry()
		agent.RegisterNethelperTools(reg, r.db, r.pipeline)
		filtered := filterTools(reg, group)
		ag := agent.New(r.llmRouter, filtered, r.embedder, r.db, agent.AgentOptions{
			Logger:     r.sessionLogger,
			UserKey:    userKey,
			ContextCfg: r.contextCfg,
		})
		// Issue #3: restore conversation history so context survives restarts
		ag.LoadConversation(userKey)
		sess = &userSession{agent: ag, group: group}
		r.sessions[userKey] = sess
	}
	r.mu.Unlock()

	// Issue #1: lock the per-user session mutex so that two messages from the
	// same user are processed sequentially, eliminating the data race on
	// agent.messages.
	sess.mu.Lock()
	defer sess.mu.Unlock()
	sess.lastActive = time.Now()

	// Issue #6: handle attached files before processing the text message.
	// Full download + ingest will be added when channel adapters expose a
	// download API; for now we acknowledge receipt and log the reference.
	for _, f := range msg.Files {
		if f.Type == "file" {
			log.Printf("[%s] received file: %s (%s)", userKey, f.Name, f.URL)
			return fmt.Sprintf("[收到文件: %s — 文件处理功能开发中]", f.Name)
		}
	}

	response, err := sess.agent.Chat(ctx, msg.Text, func(name string, args map[string]interface{}) {
		log.Printf("[%s] tool: %s", userKey, name)
	})
	if err != nil {
		return fmt.Sprintf("❌ 错误: %v", err)
	}

	// Issue #5: truncate very long responses so they don't explode into dozens
	// of IM messages. Suggest the CLI for the full output.
	if len(response) > maxIMMessageLen {
		response = response[:maxIMMessageLen] + "\n\n... (输出过长，已截断。完整内容请使用 `nethelper agent chat` 或 `nethelper plan isolate` 查看)"
	}

	// Issue #3: persist conversation after every reply so state survives a
	// process restart between turns.
	sess.agent.SaveConversation(userKey)

	return response
}

func filterTools(full *agent.Registry, group *PermissionGroup) *agent.Registry {
	filtered := agent.NewRegistry()
	for _, name := range full.Names() {
		if group.ToolAllowed(name) {
			tool, _ := full.Get(name)
			filtered.Register(tool)
		}
	}
	return filtered
}

func (r *Router) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		r.mu.Lock()
		for key, sess := range r.sessions {
			if time.Since(sess.lastActive) > 30*time.Minute {
				// Issue #3: persist conversation before evicting the session
				sess.agent.SaveConversation(key)
				sess.agent.SaveMemory(context.Background())
				delete(r.sessions, key)
			}
		}
		r.mu.Unlock()
	}
}
