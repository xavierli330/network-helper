# nethelper 扩展开发指南

本文档面向开发者，介绍如何扩展 nethelper 的各个能力模块。每个扩展点都有标准接口，实现接口后注册即可，无需修改核心代码。

## 目录

1. [新增厂商解析器](#1-新增厂商解析器-vendorparser)
2. [新增 IM 频道适配器](#2-新增-im-频道适配器-channel)
3. [新增知识源](#3-新增知识源-knowledgesource)
4. [新增 MCP Tool](#4-新增-mcp-tool)
5. [新增 Agent Tool](#5-新增-agent-tool)
6. [新增变更方案类型](#6-新增变更方案类型)

---

## 1. 新增厂商解析器（VendorParser）

适用场景：支持一个新的网络设备厂商（如 Nokia SR OS、Arista EOS）。

### 接口定义

```go
// internal/parser/types.go
type VendorParser interface {
    Vendor() string
    DetectPrompt(line string) (hostname string, ok bool)
    ClassifyCommand(cmd string) model.CommandType
    ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error)
}
```

| 方法 | 说明 |
|------|------|
| `Vendor()` | 返回厂商名称，如 `"nokia"` |
| `DetectPrompt(line)` | 判断一行文本是否是该厂商的命令行提示符，返回 hostname |
| `ClassifyCommand(cmd)` | 根据命令字符串识别命令类型（`CmdInterface`、`CmdRIB` 等）|
| `ParseOutput(cmdType, raw)` | 将原始命令输出解析为 `model.ParseResult` |

### 命令类型（model.CommandType）

```go
const (
    CmdInterface model.CommandType = "interface"
    CmdRIB                         = "rib"
    CmdFIB                         = "fib"
    CmdLFIB                        = "lfib"
    CmdNeighbor                    = "neighbor"
    CmdBGP                         = "bgp"
    CmdConfig                      = "config"
    CmdTunnel                      = "tunnel"
    CmdSRMapping                   = "sr_mapping"
    CmdUnknown                     = "unknown"
)
```

### 实现步骤

#### 第一步：创建包目录

```bash
mkdir -p internal/parser/nokia
```

#### 第二步：实现 VendorParser

```go
// internal/parser/nokia/parser.go
package nokia

import (
    "strings"
    "github.com/xavierli/nethelper/internal/model"
)

// Parser implements VendorParser for Nokia SR OS.
type Parser struct{}

// Vendor returns the vendor identifier.
func (p *Parser) Vendor() string { return "nokia" }

// DetectPrompt matches Nokia SR OS prompts.
// Typical prompt: "A:router-name#" or "B:router-name>"
func (p *Parser) DetectPrompt(line string) (hostname string, ok bool) {
    // Nokia SR OS prompts start with "A:" or "B:" followed by hostname and "#" or ">"
    line = strings.TrimSpace(line)
    if (strings.HasPrefix(line, "A:") || strings.HasPrefix(line, "B:")) &&
        (strings.HasSuffix(line, "#") || strings.HasSuffix(line, ">")) {
        // Extract hostname between "A:" and "#"
        inner := line[2 : len(line)-1]
        if inner != "" {
            return inner, true
        }
    }
    return "", false
}

// ClassifyCommand maps Nokia commands to CommandTypes.
func (p *Parser) ClassifyCommand(cmd string) model.CommandType {
    cmd = strings.ToLower(strings.TrimSpace(cmd))
    switch {
    case strings.Contains(cmd, "show router interface"):
        return model.CmdInterface
    case strings.Contains(cmd, "show router route-table"):
        return model.CmdRIB
    case strings.Contains(cmd, "show router ospf neighbor"):
        return model.CmdNeighbor
    case strings.Contains(cmd, "show router bgp neighbor"):
        return model.CmdNeighbor
    default:
        return model.CmdUnknown
    }
}

// ParseOutput parses the raw command output into a ParseResult.
func (p *Parser) ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error) {
    switch cmdType {
    case model.CmdInterface:
        return parseInterfaces(raw)
    case model.CmdRIB:
        return parseRoutes(raw)
    default:
        // Unknown output: return raw text for FTS5 scratch storage
        return model.ParseResult{RawText: raw}, nil
    }
}
```

#### 第三步：实现各解析函数

```go
// internal/parser/nokia/interfaces.go
package nokia

import (
    "bufio"
    "strings"
    "github.com/xavierli/nethelper/internal/model"
)

func parseInterfaces(raw string) (model.ParseResult, error) {
    var result model.ParseResult
    scanner := bufio.NewScanner(strings.NewReader(raw))
    // ... 解析逻辑
    return result, nil
}
```

#### 第四步：注册到 Root

在 `internal/cli/root.go` 的 `PersistentPreRunE` 中注册（注意顺序：先注册的提示符匹配优先级更高）：

```go
// internal/cli/root.go
import (
    // ... 其他 import
    "github.com/xavierli/nethelper/internal/parser/nokia"
)

// 在 PersistentPreRunE 中
registry.Register(&nokia.Parser{})
```

### 测试

在 `internal/parser/pipeline_test.go` 中添加测试用例：

```go
func TestNokiaInterface(t *testing.T) {
    // 准备一段 Nokia show router interface 的原始输出
    raw := `
A:router-01# show router interface
...`

    reg := parser.NewRegistry()
    reg.Register(&nokia.Parser{})

    blocks, err := parser.Split(raw, reg)
    require.NoError(t, err)
    require.NotEmpty(t, blocks)

    assert.Equal(t, "nokia", blocks[0].Vendor)
    assert.Equal(t, model.CmdInterface, blocks[0].CmdType)
}
```

```bash
go test ./internal/parser/...
```

---

## 2. 新增 IM 频道适配器（Channel）

适用场景：接入一个新的 IM 平台（如钉钉、Slack、企业微信）。

### 接口定义

```go
// internal/channel/types.go

type Channel interface {
    Name() string
    Start(ctx context.Context, handler MessageHandler) error
    Stop() error
    SendText(chatID string, text string) error
}

// 可选：支持流式卡片更新
type StreamingChannel interface {
    Channel
    SendInitCard(chatID string, text string) (cardID string, err error)
    UpdateCard(chatID string, cardID string, text string) error
    FinalizeCard(cardID string, text string) error
}

type MessageHandler func(msg InMessage)

type InMessage struct {
    Channel      string
    ChatID       string
    UserID       string
    UserName     string
    Text         string
    Files        []FileRef
    IsGroup      bool
    MentionedBot bool
    ReplyTo      string
}
```

### 实现步骤

#### 第一步：创建适配器包

```bash
mkdir -p internal/channel/dingtalk
```

#### 第二步：实现 Channel 接口

```go
// internal/channel/dingtalk/adapter.go
package dingtalk

import (
    "context"
    "github.com/xavierli/nethelper/internal/channel"
)

type Adapter struct {
    token  string
    stopCh chan struct{}
}

func New(token string) *Adapter {
    return &Adapter{
        token:  token,
        stopCh: make(chan struct{}),
    }
}

func (a *Adapter) Name() string { return "dingtalk" }

func (a *Adapter) Start(ctx context.Context, handler channel.MessageHandler) error {
    // 连接钉钉 WebSocket 或启动长轮询
    // 收到消息时，构造 InMessage 并调用 handler
    for {
        select {
        case <-ctx.Done():
            return nil
        case <-a.stopCh:
            return nil
        default:
            // 从钉钉接收消息
            // rawMsg := receiveDingTalkMessage()
            // msg := channel.InMessage{
            //     Channel:      "dingtalk",
            //     ChatID:       rawMsg.ConversationID,
            //     UserID:       rawMsg.SenderID,
            //     Text:         rawMsg.Text,
            //     IsGroup:      rawMsg.IsGroup,
            //     MentionedBot: rawMsg.AtMe,
            // }
            // handler(msg)
        }
    }
}

func (a *Adapter) Stop() error {
    close(a.stopCh)
    return nil
}

func (a *Adapter) SendText(chatID string, text string) error {
    // 调用钉钉发送消息 API
    return nil
}
```

**重要：** `InMessage.UserKey()` 方法返回 `<channel>:<userID>`，这是权限系统的用户标识。确保 `Channel` 字段填写正确的平台名称（与权限配置中的前缀一致）。

#### 第三步：在 config.go 添加配置结构

```go
// internal/config/config.go
type DingTalkChannelConfig struct {
    Token   string `yaml:"token"`
    Enabled bool   `yaml:"enabled"`
}

// 在 ChannelsConfig 中添加
type ChannelsConfig struct {
    // ... 已有字段
    DingTalk DingTalkChannelConfig `yaml:"dingtalk"`
}
```

#### 第四步：在 channel.go 注册

```go
// internal/cli/channel.go
import "github.com/xavierli/nethelper/internal/channel/dingtalk"

// 在 newChannelStartCmd() 中添加
if cfg.Channels.DingTalk.Enabled || dingtalkOnly {
    dc := cfg.Channels.DingTalk
    if dc.Token == "" {
        return fmt.Errorf("dingtalk: token required")
    }
    channels = append(channels, dingtalk.New(dc.Token))
}
```

#### 第五步：更新 heartbeat.go 的 createChannelByName

```go
func createChannelByName(name string) channel.Channel {
    switch name {
    case "feishu":
        // ... 已有
    case "dingtalk":
        dc := cfg.Channels.DingTalk
        if dc.Token != "" {
            return dingtalk.New(dc.Token)
        }
    }
    return nil
}
```

### 可选：实现流式卡片

如果平台支持消息编辑/更新（如飞书的 card patch），可以实现 `StreamingChannel` 接口，在 Agent 执行工具时实时更新消息内容。参考 `internal/channel/feishu/adapter.go` 的实现。

---

## 3. 新增知识源（KnowledgeSource）

适用场景：接入新的知识平台（如 Notion、语雀、自建 RAG 系统）。

### 接口定义

```go
// internal/memory/source.go
type KnowledgeSource interface {
    Name() string
    Search(ctx context.Context, query string, topK int) ([]SearchResult, error)
}

type SearchResult struct {
    Source  string  // 来源标识，如 "notion:page-123"
    Title   string  // 短标题
    Content string  // 相关文本内容
    Score   float64 // 相关度分数（0-1）
}
```

### 实现步骤

#### 第一步：实现 KnowledgeSource

```go
// internal/memory/source_notion.go
package memory

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
)

type NotionKnowledgeSource struct {
    name  string
    token string
}

func NewNotionKnowledgeSource(name, token string) KnowledgeSource {
    return &NotionKnowledgeSource{name: name, token: token}
}

func (s *NotionKnowledgeSource) Name() string { return s.name }

func (s *NotionKnowledgeSource) Search(ctx context.Context, query string, topK int) ([]SearchResult, error) {
    // 调用 Notion Search API
    reqURL := "https://api.notion.com/v1/search"

    body := map[string]interface{}{
        "query":       query,
        "page_size":   topK,
    }

    // ... 发起 HTTP 请求
    // ... 解析响应

    var results []SearchResult
    // for _, page := range resp.Results {
    //     results = append(results, SearchResult{
    //         Source:  "notion:" + page.ID,
    //         Title:   page.Title,
    //         Content: page.PlainText,
    //         Score:   0.7,
    //     })
    // }
    _ = reqURL
    _ = body
    return results, nil
}
```

#### 第二步：在 config.go 支持新类型（如需）

HTTP 类型的知识源可以直接用现有的 `http` type 配置：

```yaml
knowledge:
  sources:
    - type: http
      name: notion
      url: https://your-notion-proxy.example.com/search
      token: secret_xxx
      enabled: true
```

如果需要专有配置字段，在 `KnowledgeSourceConfig` 中追加字段。

#### 第三步：在 agent.go 中注册

在 `buildKnowledgeSources()` 函数中添加新类型的处理：

```go
// internal/cli/agent.go
func buildKnowledgeSources(cfg *config.Config) []memory.KnowledgeSource {
    var sources []memory.KnowledgeSource
    for _, sc := range cfg.Knowledge.Sources {
        if !sc.Enabled { continue }
        switch sc.Type {
        case "http":
            sources = append(sources, memory.NewHTTPKnowledgeSource(sc.Name, sc.URL, sc.Token))
        case "notion":
            sources = append(sources, memory.NewNotionKnowledgeSource(sc.Name, sc.Token))
        }
    }
    return sources
}
```

#### 测试

```go
func TestNotionKnowledgeSource(t *testing.T) {
    // 使用 httptest 模拟 Notion API
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode([]map[string]string{
            {"title": "BGP 排障", "content": "MTU 不一致是 BGP 建立失败的常见原因"},
        })
    }))
    defer server.Close()

    src := memory.NewHTTPKnowledgeSource("test", server.URL, "")
    results, err := src.Search(context.Background(), "BGP down", 3)
    require.NoError(t, err)
    assert.NotEmpty(t, results)
}
```

---

## 4. 新增 MCP Tool

适用场景：向 MCP Server 暴露新的工具，让 Claude Code 或其他 MCP 客户端调用。

### MCP 工具的组织

MCP 工具按功能分散在 `internal/mcp/tools_*.go` 文件中：

| 文件 | 包含工具 |
|------|---------|
| `tools_show.go` | `show_devices`、`show_device`、`show_interfaces` 等查询类工具 |
| `tools_analysis.go` | `check_loop`、`check_spof`、`trace_path`、`trace_impact`、`diff_config` |
| `tools_plan.go` | `plan_isolate`、`plan_upgrade`、`plan_cutover` |
| `tools_search.go` | `search_config`、`search_log` |
| `tools_write.go` | `note_add`、`diagnose`、`watch_ingest` |

### 实现步骤

#### 第一步：在合适的文件中添加工具

以在 `tools_show.go` 中添加 `show_config` 工具为例：

```go
// internal/mcp/tools_show.go（在 registerShowTools 函数末尾添加）

s.AddTool(mcpmcp.NewTool("show_config",
    mcpmcp.WithDescription("Show the latest configuration snapshot for a device"),
    mcpmcp.WithString("device_id",
        mcpmcp.Required(),
        mcpmcp.Description("The device ID whose config to retrieve"),
    ),
), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
    id := req.GetString("device_id", "")
    if id == "" {
        return mcpmcp.NewToolResultError("device_id is required"), nil
    }

    snapshot, err := db.GetLatestConfig(id)
    if err != nil {
        return mcpmcp.NewToolResultError(err.Error()), nil
    }

    return mcpmcp.NewToolResultText(snapshot.Config), nil
})
```

#### 第二步：如果需要新文件，在 server.go 中调用

```go
// internal/mcp/server.go
func NewServer(db *store.DB, pipeline *parser.Pipeline, llmRouter *llm.Router) *mcpserver.MCPServer {
    s := mcpserver.NewMCPServer("nethelper", "1.0.0",
        mcpserver.WithToolCapabilities(true),
    )
    registerShowTools(s, db)
    registerAnalysisTools(s, db)
    registerPlanTools(s, db)
    registerSearchTools(s, db)
    registerWriteTools(s, db, pipeline, llmRouter)
    registerMyNewTools(s, db)  // 新增
    return s
}
```

### 参数类型示例

```go
// 字符串参数（必填）
mcpmcp.WithString("device_id", mcpmcp.Required(), mcpmcp.Description("Device ID"))

// 字符串参数（可选）
mcpmcp.WithString("filter", mcpmcp.Description("Optional filter string"))

// 数字参数
mcpmcp.WithNumber("limit", mcpmcp.Description("Max results"), mcpmcp.DefaultNumber(10))

// 布尔参数
mcpmcp.WithBool("verbose", mcpmcp.Description("Show detailed output"))
```

---

## 5. 新增 Agent Tool

适用场景：让 Agent（`agent chat` / `channel start`）能够调用新的工具。

与 MCP Tool 的区别：Agent Tool 注册在 `agent.Registry`，供 `agent chat` 和 `channel start` 的 tool calling 使用；MCP Tool 注册在 MCP Server，供 Claude Code 等外部 MCP 客户端使用。两者可以共存，提供相同功能的工具通常需要在两处都注册。

### 接口定义

```go
// internal/agent/tools.go
type Tool struct {
    Name        string
    Description string
    Parameters  map[string]interface{} // JSON Schema
    Handler     func(args map[string]interface{}) (string, error)
}
```

### 实现步骤

在 `internal/agent/tools.go` 的 `RegisterNethelperTools` 函数末尾添加：

```go
// internal/agent/tools.go（在 RegisterNethelperTools 末尾）

// show_config — 查看设备最新配置
reg.Register(Tool{
    Name:        "show_config",
    Description: "Show the latest configuration snapshot for a device",
    Parameters: obj(map[string]interface{}{
        "device_id": strProp("Device ID"),
    }, []string{"device_id"}),
    Handler: func(args map[string]interface{}) (string, error) {
        id, _ := args["device_id"].(string)
        snapshot, err := db.GetLatestConfig(id)
        if err != nil {
            return "", err
        }
        // 截断避免 context 溢出
        config := snapshot.Config
        if len(config) > 3000 {
            config = config[:3000] + "\n...(truncated, use CLI for full output)"
        }
        return config, nil
    },
})
```

### 辅助函数

`RegisterNethelperTools` 内已定义的辅助函数：

```go
// 构造 JSON Schema object
obj := func(props map[string]interface{}, required []string) map[string]interface{} {
    schema := map[string]interface{}{"type": "object", "properties": props}
    if len(required) > 0 {
        schema["required"] = required
    }
    return schema
}

// 字符串属性
strProp := func(desc string) map[string]interface{} {
    return map[string]interface{}{"type": "string", "description": desc}
}
```

### 工具命名规范

为了与权限系统的通配符配合，工具名采用 `<动词>_<名词>` 格式：

| 前缀 | 类型 | 示例 |
|------|------|------|
| `show_` | 只读查询 | `show_devices`、`show_config` |
| `search_` | 搜索 | `search_log`、`search_config` |
| `plan_` | 方案生成 | `plan_isolate`、`plan_upgrade` |
| `note_` | 笔记操作 | `note_add` |
| `watch_` | 数据导入 | `watch_ingest` |

---

## 6. 新增变更方案类型

适用场景：支持新的变更场景（如 VLAN 割接、VRF 迁移、链路聚合扩容）。

### 方案生成架构

```
BuildTopology(db, deviceID)        → DeviceTopology（设备拓扑信息）
    ↓
GenerateXxxPlan(topo, params)      → Plan（方案结构）
    ↓
RenderMarkdown(plan)               → string（Markdown 输出）
```

### 实现步骤

#### 第一步：创建命令模板文件

```go
// internal/plan/commands_vlan.go
package plan

// vlanCommands 定义各厂商的 VLAN 操作命令
var vlanCommands = map[string]map[string]string{
    "huawei": {
        "create_vlan":    "vlan %d\n quit",
        "add_to_trunk":   "interface %s\n port trunk allow-pass vlan %d",
        "verify":         "display vlan %d",
    },
    "cisco": {
        "create_vlan":    "vlan %d\n name %s",
        "add_to_trunk":   "interface %s\n switchport trunk allowed vlan add %d",
        "verify":         "show vlan id %d",
    },
}
```

#### 第二步：定义方案参数结构

```go
// internal/plan/plan_vlan.go
package plan

type VLANMigrationParams struct {
    OldVLAN int
    NewVLAN int
    VLANName string
}
```

#### 第三步：实现 GenerateXxxPlan 函数

```go
// internal/plan/plan_vlan.go

// GenerateVLANMigrationPlan 生成 VLAN 割接变更方案。
func GenerateVLANMigrationPlan(topo *DeviceTopology, params VLANMigrationParams) *Plan {
    vendor := topo.Vendor
    cmds, ok := vlanCommands[vendor]
    if !ok {
        cmds = vlanCommands["huawei"] // fallback
    }
    _ = cmds

    plan := &Plan{
        Title:    fmt.Sprintf("VLAN 割接方案 — %s (VLAN %d → %d)", topo.Hostname, params.OldVLAN, params.NewVLAN),
        DeviceID: topo.DeviceID,
        Vendor:   vendor,
        Phases: []Phase{
            {
                Name:        "第一步：预检",
                Description: "确认当前 VLAN 流量，记录基线",
                Commands: []string{
                    fmt.Sprintf("display vlan %d", params.OldVLAN),
                    "display interface brief",
                },
                Checkpoints: []string{
                    "确认 VLAN 成员接口列表",
                    "记录当前流量（可选）",
                },
            },
            {
                Name:        "第二步：创建新 VLAN",
                Commands: []string{
                    fmt.Sprintf("vlan %d", params.NewVLAN),
                    fmt.Sprintf(" name %s", params.VLANName),
                    " quit",
                },
            },
            // ... 更多阶段
        },
    }
    return plan
}
```

#### 第四步：在 CLI 中添加命令

在 `internal/cli/plan.go` 中添加子命令：

```go
// internal/cli/plan.go

func newPlanVLANCmd() *cobra.Command {
    var oldVLAN, newVLAN int
    var vlanName string

    cmd := &cobra.Command{
        Use:   "vlan <device-id>",
        Short: "Generate a VLAN migration plan",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            deviceID := args[0]
            topo, err := plan.BuildTopology(db, deviceID)
            if err != nil {
                return fmt.Errorf("failed to build topology: %w", err)
            }
            p := plan.GenerateVLANMigrationPlan(topo, plan.VLANMigrationParams{
                OldVLAN:  oldVLAN,
                NewVLAN:  newVLAN,
                VLANName: vlanName,
            })
            fmt.Println(plan.RenderMarkdown(p))
            return nil
        },
    }

    cmd.Flags().IntVar(&oldVLAN, "old-vlan", 0, "Source VLAN ID")
    cmd.Flags().IntVar(&newVLAN, "new-vlan", 0, "Target VLAN ID")
    cmd.Flags().StringVar(&vlanName, "name", "", "New VLAN name")
    cmd.MarkFlagRequired("old-vlan")
    cmd.MarkFlagRequired("new-vlan")

    return cmd
}

// 在 newPlanCmd() 中注册
func newPlanCmd() *cobra.Command {
    cmd := &cobra.Command{Use: "plan", Short: "Generate change plans"}
    cmd.AddCommand(
        newPlanIsolateCmd(),
        newPlanUpgradeCmd(),
        newPlanCutoverCmd(),
        newPlanVLANCmd(),  // 新增
    )
    return cmd
}
```

#### 第五步：在 MCP 和 Agent 中注册

**MCP Tool：**

```go
// internal/mcp/tools_plan.go（在 registerPlanTools 中添加）
s.AddTool(mcpmcp.NewTool("plan_vlan_migration",
    mcpmcp.WithDescription("Generate a VLAN migration plan"),
    mcpmcp.WithString("device_id", mcpmcp.Required(), mcpmcp.Description("Device ID")),
    mcpmcp.WithNumber("old_vlan", mcpmcp.Required(), mcpmcp.Description("Source VLAN ID")),
    mcpmcp.WithNumber("new_vlan", mcpmcp.Required(), mcpmcp.Description("Target VLAN ID")),
), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
    id := req.GetString("device_id", "")
    oldVLAN := int(req.GetNumber("old_vlan", 0))
    newVLAN := int(req.GetNumber("new_vlan", 0))

    topo, err := plan.BuildTopology(db, id)
    if err != nil {
        return mcpmcp.NewToolResultError(err.Error()), nil
    }
    p := plan.GenerateVLANMigrationPlan(topo, plan.VLANMigrationParams{
        OldVLAN: oldVLAN,
        NewVLAN: newVLAN,
    })
    return mcpmcp.NewToolResultText(plan.RenderMarkdown(p)), nil
})
```

**Agent Tool：**

```go
// internal/agent/tools.go（在 RegisterNethelperTools 中添加）
reg.Register(Tool{
    Name:        "plan_vlan_migration",
    Description: "Generate a VLAN migration plan",
    Parameters: obj(map[string]interface{}{
        "device_id": strProp("Device ID"),
        "old_vlan":  map[string]interface{}{"type": "integer", "description": "Source VLAN ID"},
        "new_vlan":  map[string]interface{}{"type": "integer", "description": "Target VLAN ID"},
    }, []string{"device_id", "old_vlan", "new_vlan"}),
    Handler: func(args map[string]interface{}) (string, error) {
        id := getStr(args, "device_id")
        oldVLAN := int(getNum(args, "old_vlan"))
        newVLAN := int(getNum(args, "new_vlan"))
        topo, err := plan.BuildTopology(db, id)
        if err != nil {
            return "", err
        }
        p := plan.GenerateVLANMigrationPlan(topo, plan.VLANMigrationParams{
            OldVLAN: oldVLAN,
            NewVLAN: newVLAN,
        })
        return plan.RenderMarkdown(p), nil
    },
})
```

#### 测试

```go
func TestVLANMigrationPlan(t *testing.T) {
    topo := &plan.DeviceTopology{
        DeviceID: "sw-01",
        Hostname: "sw-01",
        Vendor:   "huawei",
    }
    p := plan.GenerateVLANMigrationPlan(topo, plan.VLANMigrationParams{
        OldVLAN:  100,
        NewVLAN:  200,
        VLANName: "new-vlan",
    })
    assert.NotEmpty(t, p.Phases)

    md := plan.RenderMarkdown(p)
    assert.Contains(t, md, "VLAN 割接方案")
    assert.Contains(t, md, "VLAN 200")
}
```

---

## 通用开发规范

### 错误处理

- 解析器的 `ParseOutput` 遇到无法解析的内容时，返回 `model.ParseResult{RawText: raw}` 而非 error，这样原文仍会被 FTS5 索引
- Agent Tool 的 Handler 遇到错误时，返回描述性的错误字符串，Agent 会将其告知用户
- MCP Tool 遇到错误时，返回 `mcpmcp.NewToolResultError(msg)` 而非 error

### 测试

```bash
# 运行所有测试
go test ./...

# 运行单个包的测试
go test ./internal/parser/...
go test ./internal/plan/...

# 带详细输出
go test -v ./internal/parser/...

# 类型检查
go vet ./...
```

### 构建验证

```bash
go build -o nethelper ./cmd/nethelper
./nethelper version
```
