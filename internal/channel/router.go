package channel

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/xavierli/nethelper/internal/agent"
	"github.com/xavierli/nethelper/internal/llm"
	"github.com/xavierli/nethelper/internal/parser"
	"github.com/xavierli/nethelper/internal/store"
)

type Router struct {
	db          *store.DB
	pipeline    *parser.Pipeline
	llmRouter   *llm.Router
	embedder    llm.Embedder
	permissions *PermissionConfig
	mu          sync.Mutex
	sessions    map[string]*userSession
}

type userSession struct {
	agent      *agent.Agent
	group      *PermissionGroup
	lastActive time.Time
}

func NewRouter(db *store.DB, pipeline *parser.Pipeline, llmRouter *llm.Router, embedder llm.Embedder, perms *PermissionConfig) *Router {
	r := &Router{
		db:          db,
		pipeline:    pipeline,
		llmRouter:   llmRouter,
		embedder:    embedder,
		permissions: perms,
		sessions:    make(map[string]*userSession),
	}
	go r.cleanupLoop()
	return r
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

	r.mu.Lock()
	sess, exists := r.sessions[userKey]
	if !exists || time.Since(sess.lastActive) > 30*time.Minute {
		reg := agent.NewRegistry()
		agent.RegisterNethelperTools(reg, r.db, r.pipeline)
		filtered := filterTools(reg, group)
		ag := agent.New(r.llmRouter, filtered, r.embedder, r.db)
		sess = &userSession{agent: ag, group: group}
		r.sessions[userKey] = sess
	}
	sess.lastActive = time.Now()
	r.mu.Unlock()

	response, err := sess.agent.Chat(ctx, msg.Text, func(name string, args map[string]interface{}) {
		log.Printf("[%s] tool: %s", userKey, name)
	})
	if err != nil {
		return fmt.Sprintf("❌ 错误: %v", err)
	}
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
				sess.agent.SaveMemory(context.Background())
				delete(r.sessions, key)
			}
		}
		r.mu.Unlock()
	}
}
