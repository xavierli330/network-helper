# IM Channel System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let nethelper agent receive messages from IM platforms (Feishu first) and respond via per-user agent instances with permission-based tool access.

**Architecture:** Channel Adapter interface abstracts IM platforms. Message Router handles user identification, permission checking, and per-user Agent lifecycle. Feishu adapter uses official Go SDK WebSocket mode (no public IP needed). Config extended with `channels` and `permissions` sections.

**Tech Stack:** Go, `github.com/larksuite/oapi-sdk-go/v3` (Feishu SDK with WebSocket), existing agent/llm/store packages.

**Spec:** `docs/superpowers/specs/2026-03-23-im-channel-design.md`

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/channel/types.go` | Create | Channel interface, InMessage, OutMessage, FileRef types |
| `internal/channel/permissions.go` | Create | Permission group config, tool matching, user lookup |
| `internal/channel/router.go` | Create | Message Router: user→agent routing, session management |
| `internal/channel/feishu/adapter.go` | Create | Feishu WebSocket adapter using oapi-sdk-go |
| `internal/config/config.go` | Modify | Add ChannelConfig, PermissionConfig types |
| `internal/cli/channel.go` | Create | `nethelper channel start` CLI command |
| `internal/cli/root.go` | Modify | Register channel command |
| `go.mod` | Modify | Add larksuite/oapi-sdk-go dependency |

---

## Task 1: Channel types + permission system

**Files:**
- Create: `internal/channel/types.go`
- Create: `internal/channel/permissions.go`

These are pure data types with no external dependencies — can be built and tested independently.

- [ ] **Step 1: Create types.go**

```go
// internal/channel/types.go
package channel

import "context"

// FileRef represents a file or image attached to a message.
type FileRef struct {
    Name string // original filename
    URL  string // download URL or local path
    Type string // "file" / "image"
}

// InMessage is a normalized incoming message from any IM platform.
type InMessage struct {
    Channel      string    // "feishu", "discord", "telegram"
    ChatID       string    // platform chat/conversation ID
    UserID       string    // platform user ID
    UserName     string    // display name
    Text         string    // message text content
    Files        []FileRef // attached files/images
    IsGroup      bool      // true if from a group chat
    MentionedBot bool      // true if bot was @mentioned
    ReplyTo      string    // message ID being replied to (if any)
}

// UserKey returns a globally unique user identifier: "channel:user_id"
func (m InMessage) UserKey() string {
    return m.Channel + ":" + m.UserID
}

// MessageHandler is the callback for incoming messages.
type MessageHandler func(msg InMessage)

// Channel abstracts an IM platform connection.
type Channel interface {
    Name() string
    Start(ctx context.Context, handler MessageHandler) error
    Stop() error
    SendText(chatID string, text string) error
}
```

- [ ] **Step 2: Create permissions.go**

```go
// internal/channel/permissions.go
package channel

import "strings"

// PermissionGroup defines a set of users and their allowed tools.
type PermissionGroup struct {
    Name  string
    Users []string // user keys like "feishu:ou_xxx" or "*" for default
    Tools []string // tool patterns like "show_*", "*", or exact names
}

// PermissionConfig holds all permission groups.
type PermissionConfig struct {
    Groups []PermissionGroup
}

// Resolve finds the permission group for a user key.
// Returns the most specific matching group (first match wins, "*" is fallback).
func (pc *PermissionConfig) Resolve(userKey string) *PermissionGroup {
    var fallback *PermissionGroup
    for i := range pc.Groups {
        g := &pc.Groups[i]
        for _, u := range g.Users {
            if u == userKey {
                return g
            }
            if u == "*" {
                fallback = g
            }
        }
    }
    return fallback // nil if no "*" group defined
}

// ToolAllowed checks if a tool name is permitted by the group's tool patterns.
func (g *PermissionGroup) ToolAllowed(toolName string) bool {
    for _, pattern := range g.Tools {
        if pattern == "*" {
            return true
        }
        if pattern == toolName {
            return true
        }
        // Wildcard prefix match: "show_*" matches "show_devices"
        if strings.HasSuffix(pattern, "*") {
            prefix := strings.TrimSuffix(pattern, "*")
            if strings.HasPrefix(toolName, prefix) {
                return true
            }
        }
    }
    return false
}
```

- [ ] **Step 3: Build + commit**

Run: `go build ./internal/channel/`
Commit: `feat(channel): add Channel interface, message types, and permission system`

---

## Task 2: Config extension

**Files:**
- Modify: `internal/config/config.go`

- [ ] **Step 1: Read config.go, add new types**

Add to the Config struct:

```go
type FeishuChannelConfig struct {
    AppID     string `yaml:"app_id"`
    AppSecret string `yaml:"app_secret"`
    Enabled   bool   `yaml:"enabled"`
}

type ChannelsConfig struct {
    Feishu FeishuChannelConfig `yaml:"feishu"`
    // Discord, Telegram, WeChat will be added later
}

type PermGroupConfig struct {
    Users []string `yaml:"users"`
    Tools []string `yaml:"tools"`
}

type PermissionsConfig struct {
    Groups map[string]PermGroupConfig `yaml:"groups"`
}

type Config struct {
    // ... existing fields ...
    Channels    ChannelsConfig    `yaml:"channels"`
    Permissions PermissionsConfig `yaml:"permissions"`
}
```

- [ ] **Step 2: Build + commit**

Commit: `feat(config): add channels and permissions config sections`

---

## Task 3: Message Router

**Files:**
- Create: `internal/channel/router.go`

The router manages per-user Agent instances, checks permissions, and dispatches messages.

- [ ] **Step 1: Implement router**

```go
// internal/channel/router.go
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

// Router receives messages from channels, resolves permissions,
// manages per-user Agent instances, and dispatches responses.
type Router struct {
    db          *store.DB
    pipeline    *parser.Pipeline
    llmRouter   *llm.Router
    embedder    llm.Embedder
    permissions *PermissionConfig

    mu       sync.Mutex
    sessions map[string]*userSession // keyed by UserKey
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
    // Start session cleanup goroutine
    go r.cleanupLoop()
    return r
}

// Handle processes an incoming message and returns the response text.
func (r *Router) Handle(ctx context.Context, msg InMessage) string {
    userKey := msg.UserKey()

    // Skip group messages unless bot is mentioned
    if msg.IsGroup && !msg.MentionedBot {
        return ""
    }

    // Resolve permissions
    group := r.permissions.Resolve(userKey)
    if group == nil {
        return "⚠️ 你没有权限使用此 bot。请联系管理员。"
    }

    // Get or create per-user agent
    r.mu.Lock()
    sess, exists := r.sessions[userKey]
    if !exists || time.Since(sess.lastActive) > 30*time.Minute {
        // Create new agent with filtered tool registry
        reg := agent.NewRegistry()
        agent.RegisterNethelperTools(reg, r.db, r.pipeline)
        filteredReg := r.filterTools(reg, group)
        ag := agent.New(r.llmRouter, filteredReg, r.embedder, r.db)
        sess = &userSession{agent: ag, group: group}
        r.sessions[userKey] = sess
    }
    sess.lastActive = time.Now()
    r.mu.Unlock()

    // Run agent
    response, err := sess.agent.Chat(ctx, msg.Text, func(name string, args map[string]interface{}) {
        log.Printf("[%s] tool: %s(%v)", userKey, name, args)
    })
    if err != nil {
        return fmt.Sprintf("❌ Agent 错误: %v", err)
    }
    return response
}

// filterTools creates a new registry with only the tools allowed by the permission group.
func (r *Router) filterTools(full *agent.Registry, group *PermissionGroup) *agent.Registry {
    filtered := agent.NewRegistry()
    for _, name := range full.Names() {
        if group.ToolAllowed(name) {
            tool, _ := full.Get(name)
            filtered.Register(tool)
        }
    }
    return filtered
}

// cleanupLoop removes idle sessions every 5 minutes.
func (r *Router) cleanupLoop() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()
    for range ticker.C {
        r.mu.Lock()
        for key, sess := range r.sessions {
            if time.Since(sess.lastActive) > 30*time.Minute {
                // Save memory before cleanup
                sess.agent.SaveMemory(context.Background())
                delete(r.sessions, key)
            }
        }
        r.mu.Unlock()
    }
}
```

**IMPORTANT:** This uses `full.Names()` method on agent.Registry — you'll need to add it if it doesn't exist:

```go
// In internal/agent/tools.go, add:
func (r *Registry) Names() []string {
    return r.order
}
```

- [ ] **Step 2: Add Names() to agent.Registry if needed**
- [ ] **Step 3: Build + commit**

Commit: `feat(channel): add Message Router with per-user agent sessions and permission filtering`

---

## Task 4: Feishu WebSocket Adapter

**Files:**
- Modify: `go.mod` (add larksuite/oapi-sdk-go)
- Create: `internal/channel/feishu/adapter.go`

- [ ] **Step 1: Add Feishu SDK dependency**

Run: `go get github.com/larksuite/oapi-sdk-go/v3@latest`

- [ ] **Step 2: Implement Feishu adapter**

```go
// internal/channel/feishu/adapter.go
package feishu

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"

    lark "github.com/larksuite/oapi-sdk-go/v3"
    larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
    "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
    larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
    larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

    "github.com/xavierli/nethelper/internal/channel"
)

// Adapter connects to Feishu via WebSocket and relays messages.
type Adapter struct {
    appID     string
    appSecret string
    client    *lark.Client
    wsClient  *larkws.Client
    handler   channel.MessageHandler
    ctx       context.Context
    cancel    context.CancelFunc
}

func New(appID, appSecret string) *Adapter {
    return &Adapter{
        appID:     appID,
        appSecret: appSecret,
        client:    lark.NewClient(appID, appSecret),
    }
}

func (a *Adapter) Name() string { return "feishu" }

func (a *Adapter) Start(ctx context.Context, handler channel.MessageHandler) error {
    a.handler = handler
    a.ctx, a.cancel = context.WithCancel(ctx)

    // Create event dispatcher for message events
    eventDispatcher := dispatcher.NewEventDispatcher("", "").
        OnP2MessageReceiveV1(a.onMessage)

    // Create WebSocket client (long connection, no public IP needed)
    a.wsClient = larkws.NewClient(a.appID, a.appSecret,
        larkws.WithEventHandler(eventDispatcher),
        larkws.WithLogLevel(larkcore.LogLevelInfo),
    )

    return a.wsClient.Start(a.ctx)
}

func (a *Adapter) Stop() error {
    if a.cancel != nil {
        a.cancel()
    }
    return nil
}

func (a *Adapter) SendText(chatID string, text string) error {
    // Feishu message content format
    content, _ := json.Marshal(map[string]string{"text": text})
    req := larkim.NewCreateMessageReqBuilder().
        ReceiveIdType(larkim.ReceiveIdTypeChatId).
        Body(larkim.NewCreateMessageReqBodyBuilder().
            ReceiveId(chatID).
            MsgType(larkim.MsgTypeText).
            Content(string(content)).
            Build()).
        Build()

    _, err := a.client.Im.Message.Create(a.ctx, req)
    return err
}

func (a *Adapter) onMessage(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
    msg := event.Event.Message
    sender := event.Event.Sender

    // Parse text content
    var textContent struct {
        Text string `json:"text"`
    }
    if msg.Content != nil {
        json.Unmarshal([]byte(*msg.Content), &textContent)
    }

    // Clean up @mentions from text
    text := textContent.Text
    // Feishu @mentions look like @_user_1 in text
    // Remove them for cleaner agent input

    // Build normalized message
    inMsg := channel.InMessage{
        Channel:      "feishu",
        ChatID:       deref(msg.ChatId),
        UserID:       deref(sender.SenderId.OpenId),
        UserName:     "", // would need extra API call to get name
        Text:         strings.TrimSpace(text),
        IsGroup:      deref(msg.ChatType) == "group",
        MentionedBot: containsMention(msg),
    }

    if inMsg.Text == "" {
        return nil // skip empty messages
    }

    if a.handler != nil {
        a.handler(inMsg)
    }
    return nil
}

func deref(s *string) string {
    if s == nil { return "" }
    return *s
}

func containsMention(msg *larkim.EventMessage) bool {
    if msg.Mentions == nil { return false }
    for _, m := range msg.Mentions {
        if m.Id != nil && m.Key != nil {
            return true // any mention counts for now
        }
    }
    return false
}
```

**NOTE for implementor:** The exact Feishu SDK API may differ slightly from what's shown above. Read the SDK source code in the Go module cache (`~/go/pkg/mod/github.com/larksuite/oapi-sdk-go@*/`) to confirm:
- The `dispatcher.NewEventDispatcher()` constructor params
- The `OnP2MessageReceiveV1` handler signature
- The `larkws.NewClient` constructor and options
- The `larkim.NewCreateMessageReqBuilder` API
- The event struct field names and types

- [ ] **Step 3: Build + commit**

Commit: `feat(channel): add Feishu WebSocket adapter`

---

## Task 5: CLI command + wiring

**Files:**
- Create: `internal/cli/channel.go`
- Modify: `internal/cli/root.go`

- [ ] **Step 1: Create channel CLI**

```go
// internal/cli/channel.go
package cli

import (
    "context"
    "fmt"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/spf13/cobra"
    "github.com/xavierli/nethelper/internal/channel"
    "github.com/xavierli/nethelper/internal/channel/feishu"
    "github.com/xavierli/nethelper/internal/llm"
)

func newChannelCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "channel",
        Short: "IM channel management",
    }
    cmd.AddCommand(newChannelStartCmd())
    return cmd
}

func newChannelStartCmd() *cobra.Command {
    var feishuOnly bool
    cmd := &cobra.Command{
        Use:   "start",
        Short: "Start IM channel connections",
        Long:  "Connect to configured IM platforms and start receiving messages.",
        RunE: func(cmd *cobra.Command, args []string) error {
            if llmRouter == nil {
                return fmt.Errorf("LLM not configured — agent requires LLM for responses")
            }

            // Build permission config from cfg
            perms := buildPermissions()

            // Build embedder
            embedder := llm.BuildEmbedder(cfg.Embedding)

            // Create router
            router := channel.NewRouter(db, pipeline, llmRouter, embedder, perms)

            // Start channels
            ctx, cancel := context.WithCancel(context.Background())
            defer cancel()

            var channels []channel.Channel

            if cfg.Channels.Feishu.Enabled || feishuOnly {
                fc := cfg.Channels.Feishu
                if fc.AppID == "" || fc.AppSecret == "" {
                    return fmt.Errorf("feishu channel: app_id and app_secret required in config")
                }
                ch := feishu.New(fc.AppID, fc.AppSecret)
                channels = append(channels, ch)
            }

            if len(channels) == 0 {
                return fmt.Errorf("no channels configured — add channels section to config.yaml")
            }

            // Start each channel with the router as message handler
            for _, ch := range channels {
                go func(c channel.Channel) {
                    log.Printf("Starting channel: %s", c.Name())
                    handler := func(msg channel.InMessage) {
                        response := router.Handle(ctx, msg)
                        if response != "" {
                            if err := c.SendText(msg.ChatID, response); err != nil {
                                log.Printf("[%s] send error: %v", c.Name(), err)
                            }
                        }
                    }
                    if err := c.Start(ctx, handler); err != nil {
                        log.Printf("[%s] error: %v", c.Name(), err)
                    }
                }(ch)
            }

            fmt.Printf("Channels started: %d\n", len(channels))
            fmt.Println("Press Ctrl+C to stop.")

            // Wait for signal
            sigCh := make(chan os.Signal, 1)
            signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
            <-sigCh

            fmt.Println("\nStopping channels...")
            for _, ch := range channels {
                ch.Stop()
            }
            return nil
        },
    }
    cmd.Flags().BoolVar(&feishuOnly, "feishu", false, "Start only Feishu channel")
    return cmd
}

// buildPermissions converts config to channel.PermissionConfig.
func buildPermissions() *channel.PermissionConfig {
    pc := &channel.PermissionConfig{}
    for name, group := range cfg.Permissions.Groups {
        pc.Groups = append(pc.Groups, channel.PermissionGroup{
            Name:  name,
            Users: group.Users,
            Tools: group.Tools,
        })
    }
    // Default: if no groups defined, allow all users read-only
    if len(pc.Groups) == 0 {
        pc.Groups = append(pc.Groups, channel.PermissionGroup{
            Name:  "default",
            Users: []string{"*"},
            Tools: []string{"show_*", "search_*"},
        })
    }
    return pc
}
```

- [ ] **Step 2: Register in root.go**

Add `root.AddCommand(newChannelCmd())`.

- [ ] **Step 3: Build + smoke test**

Run: `go build ./cmd/nethelper && ./nethelper channel start --help`

- [ ] **Step 4: Commit**

Commit: `feat(cli): add channel start command for IM connections`

---

## Task 6: Integration test + config example

- [ ] **Step 1: Add example channel config to config.yaml template**

Add to `~/.nethelper/config.yaml`:

```yaml
channels:
  feishu:
    app_id: ""      # 飞书开放平台 App ID
    app_secret: ""  # 飞书开放平台 App Secret
    enabled: false

permissions:
  groups:
    admin:
      users: []     # e.g. ["feishu:ou_xxx"]
      tools: ["*"]
    operator:
      users: []
      tools: ["show_*", "plan_*", "search_*", "trace_*", "check_*", "note_*"]
    viewer:
      users: ["*"]
      tools: ["show_devices", "show_topology", "show_interfaces", "search_config"]
```

- [ ] **Step 2: Full test suite**

Run: `go test ./... && go vet ./...`

- [ ] **Step 3: Update README**

Add IM section to README, update Phase 3 progress.

- [ ] **Step 4: Commit**

Commit: `feat(channel): complete Feishu IM integration + update README`
