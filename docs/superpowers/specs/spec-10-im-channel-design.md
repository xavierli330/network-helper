# Design: IM Channel 系统 — 多平台接入 + 权限分组

**日期:** 2026-03-23
**状态:** Approved

## 目标

让 nethelper agent 能通过 IM 平台（飞书/Discord/Telegram/微信）接收消息并回复，支持多用户权限分组和会话隔离。飞书作为第一个实现验证架构。

## 架构

```
IM 平台 ←WebSocket→ Channel Adapter ── Message Router ── Agent(per-user)
                                          ├─ 用户识别
                                          ├─ 权限检查
                                          ├─ 会话隔离
                                          └─ 记忆注入
```

### Channel 接口

```go
type Channel interface {
    Name() string
    Start(ctx context.Context, handler MessageHandler) error
    Stop() error
    SendText(chatID string, text string) error
    SendFile(chatID string, path string) error
}

type MessageHandler func(msg InMessage)

type InMessage struct {
    Channel     string
    ChatID      string   // 平台聊天 ID
    UserID      string   // 平台用户 ID
    UserName    string
    Text        string
    Files       []FileRef  // 文件引用（URL + 名称）
    IsGroup     bool
    MentionedBot bool
    ReplyTo     string   // 被回复消息 ID
}

type FileRef struct {
    Name string
    URL  string
    Type string  // "file" / "image"
}
```

### Message Router

收到消息后：
1. 构造 user key: `"feishu:ou_xxx"` 或 `"telegram:123456"`
2. 查权限组 → 确定可用 tools
3. 获取或创建 per-user Agent 实例
4. 调用 `agent.Chat()` → 拿到回复
5. 通过 Channel 发回

群消息只在 @bot 时响应。DM 直接响应。

### 权限分组

```yaml
channels:
  feishu:
    app_id: "cli_xxx"
    app_secret: "xxx"
    enabled: true

permissions:
  groups:
    admin:
      users: ["feishu:ou_xxx"]
      tools: ["*"]
    operator:
      users: ["feishu:ou_yyy"]
      tools: ["show_*", "plan_*", "search_*", "trace_*", "check_*", "note_*"]
    viewer:
      users: ["*"]  # 默认
      tools: ["show_devices", "show_topology", "show_interfaces", "search_config"]
```

Wildcard 匹配：`show_*` 匹配所有 `show_` 开头的 tools。

### Per-User Agent 隔离

```go
type UserSession struct {
    Agent     *agent.Agent
    UserKey   string        // "feishu:ou_xxx"
    UserName  string
    Group     string        // "admin" / "operator" / "viewer"
    LastActive time.Time
}
```

每个 `(channel:user_id)` 有独立的 Agent 实例，独立的消息历史和记忆空间。idle 超过 30 分钟自动清理（节省内存），下次消息重新创建。

### 文件处理

收到文件/图片时：
1. Channel Adapter 下载到临时目录
2. 如果是配置文件（.txt/.conf/.cfg）→ 调用 `pipeline.Ingest()` 导入
3. 如果是图片 → 后续支持 LLM 视觉分析（当前版本跳过）

### 飞书 Adapter

使用 `github.com/larksuite/oapi-sdk-go/v3`：
- WebSocket 长连接（`ws` 包），不需要公网 IP
- `EventDispatcher` 处理 `im.message.receive_v1` 事件
- 通过 `lark.Client` 发送回复消息
- 用 `open_id` 识别用户

### 代码结构

```
internal/channel/
├── types.go          // Channel 接口 + InMessage + FileRef
├── router.go         // Message Router + 权限检查 + agent 管理
├── permissions.go    // 权限配置解析 + tool 匹配
├── feishu/
│   └── adapter.go    // 飞书 WebSocket adapter
internal/cli/
└── channel.go        // nethelper channel start 命令
internal/config/
└── config.go         // 修改：加 channels + permissions 配置段
```

### CLI

```bash
nethelper channel start           # 启动所有已配置的 channels
nethelper channel start --feishu  # 只启动飞书
```
