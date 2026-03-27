# Field Introspection & Derived Fields Design

## Goal

给解析层加一套「字段目录」机制：工程师可以查询任意命令的解析结果有哪些字段、字段类型和示例值；开发者可以在 Rule Studio `schema_yaml` 里声明派生字段，code generator 自动生成调用骨架，开发者填入 Go 表达式即可。

## Background

当前痛点：
1. **不知道已有哪些字段** — 手写 parser 的字段只存在于 Go struct，没有可查询的元数据
2. **无法基于已有字段做二次计算** — 如利用率 = 流量 / 带宽，只能手写完整 parser
3. **Rule Studio 规则 schema 和 parser 代码分离** — 审批规则后不知道实际暴露什么字段

## Architecture

```
手写 parser          Rule Studio 生成 parser
FieldSchema()  ←→   code generator 从 schema_yaml 生成
     ↓                        ↓
         FieldRegistry（启动时 build）
                  ↓
     CLI: nethelper rule fields        UI: Rule Studio 字段浏览器
```

**关键设计决策：**
- **parse-time 计算**：派生字段在 ParseOutput 时计算，结果通过 `model.ParseResult.Rows` 传递给 pipeline，pipeline 写入 scratch pad；查询侧零额外开销
- **开发者写 Go**：不引入表达式引擎，code generator 生成骨架 + `TODO` 注释，开发者填一行 Go
- **FieldSchema() + SupportedCmdTypes() 加入接口**：编译时强制所有 vendor parser 实现，不会漂移
- **FieldRegistry 以 CommandType 为 key**：避免 commandNorm → CommandType 多对一冲突；CLI 层先调 ClassifyCommand 再查 registry

## Data Structures

### FieldType 和 FieldDef

```go
// internal/parser/field.go

// FieldType 是字段类型的枚举，限制合法值集合。
type FieldType string

const (
    FieldTypeString FieldType = "string"
    FieldTypeInt    FieldType = "int"
    FieldTypeFloat  FieldType = "float"
    FieldTypeBool   FieldType = "bool"
)

type FieldDef struct {
    Name        string    // snake_case，如 "phy_status"
    Type        FieldType // 枚举，见上方常量
    Description string    // 人读说明
    Example     string    // 示例值，如 "up"
    Derived     bool      // 是否派生字段
    DerivedFrom []string  // 依赖的源字段名，如 ["in_bytes", "bandwidth_kbps"]
}
```

使用 `FieldType` 而非裸 `string`，hand-written parser 在编译期即可发现拼写错误；schema_yaml 解析时显式验证 `FieldType` 合法性。

### model.ParseResult 新增 Rows 字段

```go
// internal/model/parse_result.go（现有结构，新增一行）
type ParseResult struct {
    // ... 现有字段 ...
    Rows []map[string]string `json:"rows,omitempty"` // ← 新增：供生成 parser 传递通用行数据
}
```

同时更新 `IsEmpty()` 以包含 `Rows`：

```go
func (pr ParseResult) IsEmpty() bool {
    return len(pr.Interfaces) == 0 &&
        len(pr.RIBEntries) == 0 &&
        len(pr.FIBEntries) == 0 &&
        len(pr.LFIBEntries) == 0 &&
        len(pr.Neighbors) == 0 &&
        len(pr.Tunnels) == 0 &&
        len(pr.SRMappings) == 0 &&
        len(pr.Rows) == 0  // ← 新增
}
```

Pipeline 在 `storeResult()` 中：若 `result.Rows` 非空，将其 JSON 序列化后写入 `scratch_entries`（category = `"generated"`），与现有大表路由逻辑一致。

### VendorParser 接口新增两个方法

```go
// internal/parser/types.go（现有接口）
type VendorParser interface {
    Vendor() string
    DetectPrompt(line string) (string, bool)
    ClassifyCommand(cmd string) model.CommandType
    ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error)
    FieldSchema(cmdType model.CommandType) []FieldDef        // ← 新增：返回该 cmdType 的字段列表
    SupportedCmdTypes() []model.CommandType                  // ← 新增：返回该 parser 支持的所有 cmdType
}
```

- `FieldSchema`：未知 `cmdType` 返回 `nil`（不是 error）。
- `SupportedCmdTypes`：供 `BuildFieldRegistry` 遍历；包含手写常量（`CmdInterface`、`CmdNeighbor` 等）和生成 parser 的动态类型字符串（如 `"generated:huawei:traffic_policy_statistics_interface"`）。

**所有四个 vendor parser 的编译期接口检查必须升级为：**

```go
// 在 huawei.go / cisco.go / h3c.go / juniper.go 顶部
var _ parser.VendorParser = (*Parser)(nil)  // 确保整个接口被满足
```

（现有检查仅验证 `Vendor()`，升级后在编译期即可发现缺少 `FieldSchema` 或 `SupportedCmdTypes` 的情况。）

### FieldRegistry

```go
// internal/parser/field_registry.go
type FieldRegistry struct {
    // vendor → CommandType → []FieldDef
    // key 使用 model.CommandType（string alias），不是 commandNorm
    index map[string]map[model.CommandType][]FieldDef
}

// BuildFieldRegistry 遍历 registry 中所有 parser：
//   1. 调用 p.SupportedCmdTypes() 获得完整 cmdType 列表
//   2. 对每个 cmdType 调用 p.FieldSchema(ct) 收集字段
func BuildFieldRegistry(reg *Registry) *FieldRegistry

// Fields 查询单个 CommandType 的字段列表
func (r *FieldRegistry) Fields(vendor string, cmdType model.CommandType) []FieldDef

// Vendors 返回所有已注册 vendor
func (r *FieldRegistry) Vendors() []string

// CmdTypes 返回某 vendor 已注册的所有 CommandType
func (r *FieldRegistry) CmdTypes(vendor string) []model.CommandType
```

**CLI 层职责**：调用 `ClassifyCommand(rawInput)` 将用户输入的命令字符串转为 `CommandType`，再调用 `fieldRegistry.Fields(vendor, cmdType)` 查询。

**Registry 初始化：** 在 `root.go` 中，`registry` 变量需升级为 **package-level var**（与 `pipeline`、`db` 一致），这样 `fields.go` 的 CLI 命令才能调用 `registry.Get(vendor).ClassifyCommand(rawInput)` 做命令字符串→CommandType 的转换。

```go
// root.go：package-level vars（新增 registry 和 fieldRegistry）
var (
    cfg          *config.Config
    db           *store.DB
    pipeline     *parser.Pipeline
    llmRouter    *llm.Router
    registry     *parser.Registry      // ← 升级为 package-level（原为 PersistentPreRunE 局部变量）
    fieldRegistry *parser.FieldRegistry // ← 新增
)
```

在 `PersistentPreRunE` 中赋值（已有 registry 初始化代码，改为赋给 package-level var）：

```go
registry = parser.NewRegistry()
registry.Register(huawei.New())
// ...
pipeline = parser.NewPipelineWithCollector(db, registry, parser.NewCollector(db))
fieldRegistry = parser.BuildFieldRegistry(registry)
```

## Hand-Written Parser Implementation

每个 vendor parser（huawei/cisco/h3c/juniper）在各自的 `<vendor>.go` 中实现 `FieldSchema()` 和 `SupportedCmdTypes()`。

**前提**：`newRuleCmd()` 已在 `internal/cli/rule.go` 中实现（Parser Rule Studio 计划已落地），`nethelper rule fields` 作为子命令挂入该 parent command。如果 Rule Studio 计划尚未合并，实现者需先确保 `rule.go` 中存在 `newRuleCmd()`。

示例（huawei）：

```go
// internal/parser/huawei/huawei.go

// compile-time interface check
var _ parser.VendorParser = (*Parser)(nil)

func (p *Parser) SupportedCmdTypes() []model.CommandType {
    base := []model.CommandType{
        model.CmdInterface,
        model.CmdNeighbor,
        model.CmdRIB,
        model.CmdFIB,
        model.CmdLFIB,
        model.CmdTunnel,
        model.CmdSRMapping,
        model.CmdConfig,
        model.CmdConfigSet, // ← 不要遗漏
        // 生成 parser 的动态类型由 huawei_generated.go 的 generatedCmdTypes() 提供
    }
    return append(base, generatedCmdTypes()...)
}

func (p *Parser) FieldSchema(cmdType model.CommandType) []parser.FieldDef {
    switch cmdType {
    case model.CmdInterface:
        return []parser.FieldDef{
            {Name: "name",         Type: parser.FieldTypeString, Description: "接口名称", Example: "GigabitEthernet0/0/0"},
            {Name: "phy_status",   Type: parser.FieldTypeString, Description: "物理状态", Example: "up"},
            {Name: "proto_status", Type: parser.FieldTypeString, Description: "协议状态", Example: "up"},
            {Name: "ip_address",   Type: parser.FieldTypeString, Description: "IP 地址",  Example: "10.0.0.1"},
            {Name: "mask",         Type: parser.FieldTypeString, Description: "子网掩码", Example: "255.255.255.0"},
            {Name: "bandwidth",    Type: parser.FieldTypeString, Description: "带宽配置", Example: "1000M"},
            {Name: "description",  Type: parser.FieldTypeString, Description: "接口描述", Example: "to-PE1"},
        }
    case model.CmdNeighbor:
        return []parser.FieldDef{
            {Name: "protocol",       Type: parser.FieldTypeString, Description: "邻居协议", Example: "ospf"},
            {Name: "remote_id",      Type: parser.FieldTypeString, Description: "对端 ID",  Example: "10.0.0.2"},
            {Name: "remote_address", Type: parser.FieldTypeString, Description: "对端地址", Example: "10.0.0.2"},
            {Name: "state",          Type: parser.FieldTypeString, Description: "邻居状态", Example: "Full"},
            {Name: "uptime",         Type: parser.FieldTypeString, Description: "建立时长", Example: "2d3h"},
        }
    // ... 其他 cmdType
    default:
        // 委托生成 parser
        return generatedFieldSchema(cmdType)
    }
}
```

### 生成 Parser 的 SupportedCmdTypes 整合

`huawei_generated.go`（以及各 vendor 对应文件）新增两个辅助函数，供主 Parser 调用：

```go
// internal/parser/huawei/huawei_generated.go
// 注意：本文件需 import parser 父包（用于 parser.FieldDef 类型）。
// package huawei → import parser 是合法的子包→父包引用，不构成循环。
// 同理适用于 cisco_generated.go、h3c_generated.go、juniper_generated.go。

import (
    "github.com/xavierli/nethelper/internal/model"
    "github.com/xavierli/nethelper/internal/parser"
)

// generatedCmdTypes 返回所有已生成规则的 CommandType 列表
func generatedCmdTypes() []model.CommandType {
    return []model.CommandType{
        // GENERATED CMDTYPES — do not edit this comment
    }
}

// generatedFieldSchema 返回生成规则对应的 FieldDef 列表
func generatedFieldSchema(cmdType model.CommandType) []parser.FieldDef {
    switch cmdType {
    // GENERATED FIELD CASES — do not edit this comment
    }
    return nil
}
```

`PatchGeneratedFile` 在 approve 时同时 patch **四处** sentinel：
- `// GENERATED CASES`（分类函数，已有）
- `// GENERATED PARSE CASES`（解析函数，已有）
- `// GENERATED CMDTYPES`（cmdType 枚举，新增）
- `// GENERATED FIELD CASES`（FieldSchema，新增）

## Rule Studio: `schema_yaml` Derived Fields Extension

在现有 `schema_yaml` 中新增可选 `derived` 块：

```yaml
header_pattern: "Interface\\s+InOctets\\s+Bandwidth"
skip_lines: 0
columns:
  - name: interface
    index: 0
    type: string
  - name: in_bytes
    index: 1
    type: int
  - name: bandwidth_kbps
    index: 2
    type: int
derived:
  - name: util_pct
    type: float
    description: "入方向利用率百分比"
    from: ["in_bytes", "bandwidth_kbps"]
    example: "3.14"
```

`derived` 字段为可选，缺省时与现有行为完全兼容。Schema 解析时执行以下校验，违反任何一条均返回 error：
- `type` 只接受 `FieldType` 枚举值（`string` | `int` | `float` | `bool`）
- `from` 中的每个字段名必须存在于 `columns[*].name` 列表中（引用不存在的列为 schema 错误）

## Code Generator Changes

### GenerateParserFile 扩展

对含 `derived` 的规则，在 `ParseTable` 调用之后插入派生字段骨架，并将 `tableResult.Rows` 赋给 `model.ParseResult.Rows`：

```go
// ParseHuaweiTrafficPolicyStatisticsInterface — generated
func ParseHuaweiTrafficPolicyStatisticsInterface(raw string) (model.ParseResult, error) {
    schema := engine.TableSchema{ ... }
    tableResult, err := engine.ParseTable(schema, raw)
    if err != nil {
        return model.ParseResult{RawText: raw}, err
    }

    // Derived fields — implement each TODO below
    for i := range tableResult.Rows {
        row := tableResult.Rows[i]
        // derived: util_pct (float) from [in_bytes, bandwidth_kbps]
        // TODO: row["util_pct"] = fmt.Sprintf("%f", ...)
        _ = row
    }

    // Rows 非空时 pipeline 自动写入 scratch pad（category="generated"）
    return model.ParseResult{
        Type:    model.CommandType("generated:huawei:traffic_policy_statistics_interface"),
        RawText: raw,
        Rows:    tableResult.Rows,
    }, nil
}
```

开发者只需找到 `// TODO` 注释填入一行 Go 表达式。`_ = row` 占位保证未实现时也能编译。

### FieldSchema() 自动生成

`FieldSchema` 通过 sentinel `// GENERATED FIELD CASES` 注入到 `generatedFieldSchema()` 函数中：

```go
// huawei_generated.go — 由 PatchGeneratedFile 追加 case

func generatedFieldSchema(cmdType model.CommandType) []parser.FieldDef {
    switch cmdType {
    // GENERATED FIELD CASES — do not edit this comment
    case model.CommandType("generated:huawei:traffic_policy_statistics_interface"):
        return []parser.FieldDef{
            {Name: "interface",      Type: parser.FieldTypeString, Description: "", Example: ""},
            {Name: "in_bytes",       Type: parser.FieldTypeInt,    Description: "", Example: ""},
            {Name: "bandwidth_kbps", Type: parser.FieldTypeInt,    Description: "", Example: ""},
            {Name: "util_pct",       Type: parser.FieldTypeFloat,  Description: "入方向利用率百分比", Example: "3.14",
             Derived: true, DerivedFrom: []string{"in_bytes", "bandwidth_kbps"}},
        }
    }
    return nil
}
```

`Description` 和 `Example` 从 `schema_yaml` 的 `columns` 条目读取（如有）；`derived` 字段从 `derived` 块读取。

## Pipeline: Rows 路由

`internal/parser/pipeline.go` 的 `storeResult()` 新增路由分支（在现有 `isBulkTableCommand` 检查之前或之后均可，两者互斥）：

```go
if len(result.Rows) > 0 {
    // 生成 parser 的通用行数据：JSON 序列化后写入 scratch pad
    // 注意：storeResult() 中实际参数名为 pr，此处用 result 做说明，实现时统一。
    rowsJSON, err := json.Marshal(result.Rows)
    if err != nil {
        rowsJSON = []byte("[]")
    }
    p.db.InsertScratch(model.ScratchEntry{ // nolint: errcheck — 与现有大表路由一致，忽略 error
        DeviceID: deviceID,
        Category: "generated",
        Query:    string(result.Type), // CommandType 是 string alias，直接转换
        Content:  string(rowsJSON),
    })
    return nil
}
```

说明：
- `model.ScratchEntry.Content` 是单个 `string`；所有行作为 JSON 数组存入一条记录（与现有大表路由的 `RawText` 模式一致）。
- `string(result.Type)` 直接转换 `CommandType`（底层为 `string`）；无需 `.String()` 方法。
- 需在 `pipeline.go` 的 import 中添加 `"encoding/json"`。

## CLI: `nethelper rule fields`

新增 `internal/cli/fields.go`，挂在 `newRuleCmd()`（已存在于 `internal/cli/rule.go`）下：

```
nethelper rule fields <vendor> [command]

# 列出某命令所有字段（command 为用户可读命令字符串，CLI 内部调用 ClassifyCommand 转为 CommandType）
nethelper rule fields huawei "display interface brief"

# 列出某 vendor 所有已注册 CommandType
nethelper rule fields huawei

# 列出所有 vendor
nethelper rule fields
```

输出格式：

```
Vendor: huawei  Command: display interface brief  (CommandType: interface)
Field             Type    Derived  From                     Description
─────────────────────────────────────────────────────────────────────
name              string  no       —                        接口名称
phy_status        string  no       —                        物理状态
util_pct          float   yes      in_bytes,bandwidth_kbps  入方向利用率
```

## Rule Studio UI: 字段浏览器

在现有编辑器页面右侧新增侧边栏（HTMX，lazy load）：

- 顶部：vendor 下拉 + 命令搜索框（`hx-get` 触发）
- 主体：字段列表，每行显示名称、类型、是否派生、描述、示例值
- 点击字段名 → 复制到剪贴板（`navigator.clipboard.writeText`）

新增 API 端点（vendor 参数必填；command 为原始命令字符串，服务端调用 `ClassifyCommand` 转换）：

```
GET /api/fields?vendor=huawei&command=display+interface+brief
→ {"cmdType":"interface","fields": [{"name":"phy_status","type":"string",...}, ...]}

GET /api/fields?vendor=huawei
→ {"cmdTypes": ["interface", "neighbor", "generated:huawei:traffic_policy_statistics_interface", ...]}

GET /api/fields
→ {"vendors": ["huawei", "cisco", "h3c", "juniper"]}
```

## File Map

### New Files

| File | Responsibility |
|------|----------------|
| `internal/parser/field.go` | `FieldType` 常量 + `FieldDef` struct 定义 |
| `internal/parser/field_registry.go` | `FieldRegistry`（以 `CommandType` 为 key）+ `BuildFieldRegistry` |
| `internal/parser/field_registry_test.go` | Unit tests |
| `internal/cli/fields.go` | `nethelper rule fields` CLI 命令 |
| `internal/cli/fields_test.go` | CLI 命令测试 |

### Modified Files

| File | Change |
|------|--------|
| `internal/model/parse_result.go` | 新增 `Rows []map[string]string` 字段；`IsEmpty()` 添加 `len(pr.Rows) == 0` |
| `internal/parser/types.go` | `VendorParser` 接口新增 `FieldSchema()` + `SupportedCmdTypes()` |
| `internal/parser/pipeline.go` | `storeResult()` 新增 Rows → scratch pad 路由；import 添加 `encoding/json` |
| `internal/parser/huawei/huawei.go` | 实现 `FieldSchema()` + `SupportedCmdTypes()`（含 `CmdConfigSet`）；升级接口检查为 `var _ parser.VendorParser = (*Parser)(nil)` |
| `internal/parser/cisco/cisco.go` | 同上 |
| `internal/parser/h3c/h3c.go` | 同上 |
| `internal/parser/juniper/juniper.go` | 同上 |
| `internal/parser/huawei/huawei_generated.go` | 新增 `generatedCmdTypes()` + `generatedFieldSchema()` + 两个新 sentinel（`// GENERATED CMDTYPES`、`// GENERATED FIELD CASES`）；添加 `import "github.com/xavierli/nethelper/internal/parser"` |
| `internal/parser/cisco/cisco_generated.go` | 同上 |
| `internal/parser/h3c/h3c_generated.go` | 同上 |
| `internal/parser/juniper/juniper_generated.go` | 同上 |
| `internal/cli/root.go` | `registry` 升级为 package-level var；新增 `fieldRegistry *parser.FieldRegistry` package-level var；`PersistentPreRunE` 赋值并调用 `BuildFieldRegistry` |
| `internal/codegen/generator.go` | `schema_yaml` 解析 `derived` 块（含 `from` 引用校验）；生成派生骨架 + `Rows` return；`PatchGeneratedFile` 新增两处 sentinel patch（`// GENERATED CMDTYPES`、`// GENERATED FIELD CASES`）；`FieldType` 枚举校验 |
| `internal/studio/server.go` | `Server` struct 新增 `fieldReg *parser.FieldRegistry` 字段；`NewServer()` 新增同名参数；`registerRoutes()` 传入 handlers 并注册 `/api/fields` 路由 |
| `internal/studio/handlers.go` | `handlers` struct 新增 `fieldReg *parser.FieldRegistry` 字段；实现 `/api/fields` 端点 + 字段浏览侧边栏 HTML |
| `internal/cli/rule.go` | `rule studio` 子命令调用 `studio.NewServer(...)` 时传入 `fieldRegistry`（package-level var）作为新参数 |

### Deferred

- 内置表达式求值引擎（用户明确选择 Go 实现）
- 跨命令字段 join / 关联查询
- 字段变更历史追踪（字段增删 diff）
- `FieldDef` 持久化到 DB（目前仅内存 registry）
