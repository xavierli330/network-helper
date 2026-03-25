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
- **parse-time 计算**：派生字段在 ParseOutput 时计算，结果直接入库，查询侧零额外开销
- **开发者写 Go**：不引入表达式引擎，code generator 生成骨架 + `TODO` 注释，开发者填一行 Go
- **FieldSchema() 加入接口**：编译时强制所有 vendor parser 实现，不会漂移

## Data Structures

### FieldDef

```go
// internal/parser/field.go
type FieldDef struct {
    Name        string   // snake_case，如 "phy_status"
    Type        string   // "string" | "int" | "float" | "bool"
    Description string   // 人读说明
    Example     string   // 示例值，如 "up"
    Derived     bool     // 是否派生字段
    DerivedFrom []string // 依赖的源字段名，如 ["in_bytes", "bandwidth_kbps"]
}
```

### VendorParser 接口新增方法

```go
// internal/parser/types.go（现有接口）
type VendorParser interface {
    Vendor() string
    DetectPrompt(line string) (string, bool)
    ClassifyCommand(cmd string) model.CommandType
    ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error)
    FieldSchema(cmdType model.CommandType) []FieldDef  // ← 新增
}
```

`FieldSchema` 返回 `cmdType` 对应的字段列表；未知 `cmdType` 返回 `nil`（不是 error）。

### FieldRegistry

```go
// internal/parser/field_registry.go
type FieldRegistry struct {
    // vendor → commandNorm → []FieldDef
    index map[string]map[string][]FieldDef
}

// BuildFieldRegistry 遍历 registry 中所有 parser，收集所有 cmdType 的 FieldDef
func BuildFieldRegistry(reg *Registry) *FieldRegistry

// Fields 查询单个命令的字段列表，commandNorm 为 normalised 命令字符串
func (r *FieldRegistry) Fields(vendor, commandNorm string) []FieldDef

// Vendors 返回所有已注册 vendor
func (r *FieldRegistry) Vendors() []string

// Commands 返回某 vendor 已注册的所有命令（normalised）
func (r *FieldRegistry) Commands(vendor string) []string
```

`commandNorm` 使用与 collector 相同的 normalise 逻辑（lowercase + strip display/show prefix）。

**Registry 初始化：** 在 `root.go` `PersistentPreRunE` 中，pipeline 创建后立即 build：

```go
fieldRegistry = parser.BuildFieldRegistry(registry)
```

## Hand-Written Parser Implementation

每个 vendor parser（huawei/cisco/h3c/juniper）在各自的 `<vendor>.go` 中实现 `FieldSchema()`。

示例（huawei `display interface brief`）：

```go
func (p *Parser) FieldSchema(cmdType model.CommandType) []FieldDef {
    switch cmdType {
    case model.CmdInterface:
        return []parser.FieldDef{
            {Name: "name",        Type: "string", Description: "接口名称",     Example: "GigabitEthernet0/0/0"},
            {Name: "phy_status",  Type: "string", Description: "物理状态",     Example: "up"},
            {Name: "proto_status",Type: "string", Description: "协议状态",     Example: "up"},
            {Name: "ip_address",  Type: "string", Description: "IP 地址",      Example: "10.0.0.1"},
            {Name: "mask",        Type: "string", Description: "子网掩码",      Example: "255.255.255.0"},
            {Name: "bandwidth",   Type: "string", Description: "带宽配置",      Example: "1000M"},
            {Name: "description", Type: "string", Description: "接口描述",      Example: "to-PE1"},
        }
    case model.CmdNeighbor:
        return []parser.FieldDef{
            {Name: "protocol",       Type: "string", Description: "邻居协议",   Example: "ospf"},
            {Name: "remote_id",      Type: "string", Description: "对端 ID",   Example: "10.0.0.2"},
            {Name: "remote_address", Type: "string", Description: "对端地址",   Example: "10.0.0.2"},
            {Name: "state",          Type: "string", Description: "邻居状态",   Example: "Full"},
            {Name: "uptime",         Type: "string", Description: "建立时长",   Example: "2d3h"},
        }
    // ... 其他 cmdType
    }
    return nil
}
```

对于 Rule Studio 生成的 parser，`FieldSchema()` 由 code generator 从 `schema_yaml` 自动生成，不需要手写。

## Rule Studio: `schema_yaml` Derived Fields Extension

在现有 `schema_yaml` 中新增 `derived` 块：

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

`derived` 字段为可选，缺省时与现有行为完全兼容。

## Code Generator Changes

### GenerateParserFile 扩展

对含 `derived` 的规则，在 `ParseTable` 调用之后插入派生字段骨架：

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

    return model.ParseResult{Type: model.CommandType("generated:huawei:..."), RawText: raw}, nil
}
```

开发者只需找到 `// TODO` 注释填入一行 Go 表达式。`_ = row` 占位保证未实现时也能编译。

### FieldSchema() 自动生成

```go
func (p *Parser) FieldSchema(cmdType model.CommandType) []parser.FieldDef {
    switch cmdType {
    case model.CommandType("generated:huawei:traffic_policy_statistics_interface"):
        return []parser.FieldDef{
            {Name: "interface",      Type: "string", Description: "", Example: ""},
            {Name: "in_bytes",       Type: "int",    Description: "", Example: ""},
            {Name: "bandwidth_kbps", Type: "int",    Description: "", Example: ""},
            {Name: "util_pct",       Type: "float",  Description: "入方向利用率百分比", Example: "3.14",
             Derived: true, DerivedFrom: []string{"in_bytes", "bandwidth_kbps"}},
        }
    }
    return nil
}
```

`Description` 和 `Example` 从 `schema_yaml` 的 `columns` 条目读取（如有）；`derived` 字段从 `derived` 块读取。

## CLI: `nethelper rule fields`

新增 `internal/cli/fields.go`，挂在 `newRuleCmd()` 下：

```
nethelper rule fields <vendor> [command]

# 列出某命令所有字段
nethelper rule fields huawei "display interface brief"

# 列出某 vendor 所有已注册命令
nethelper rule fields huawei

# 列出所有 vendor
nethelper rule fields
```

输出格式：

```
Vendor: huawei  Command: display interface brief
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

新增 API 端点：

```
GET /api/fields?vendor=huawei&command=display+interface+brief
→ {"fields": [{"name":"phy_status","type":"string",...}, ...]}

GET /api/fields?vendor=huawei
→ {"commands": ["display interface brief", "display bgp peer", ...]}

GET /api/fields
→ {"vendors": ["huawei", "cisco", "h3c", "juniper"]}
```

## File Map

### New Files

| File | Responsibility |
|------|----------------|
| `internal/parser/field.go` | `FieldDef` struct 定义 |
| `internal/parser/field_registry.go` | `FieldRegistry` + `BuildFieldRegistry` |
| `internal/parser/field_registry_test.go` | Unit tests |
| `internal/cli/fields.go` | `nethelper rule fields` CLI 命令 |
| `internal/cli/fields_test.go` | CLI 命令测试 |

### Modified Files

| File | Change |
|------|--------|
| `internal/parser/types.go` | `VendorParser` 接口新增 `FieldSchema()` |
| `internal/parser/huawei/huawei.go` | 实现 `FieldSchema()` |
| `internal/parser/cisco/cisco.go` | 实现 `FieldSchema()` |
| `internal/parser/h3c/h3c.go` | 实现 `FieldSchema()` |
| `internal/parser/juniper/juniper.go` | 实现 `FieldSchema()` |
| `internal/cli/root.go` | 注册 `fieldRegistry` 全局变量 + 注入 CLI |
| `internal/codegen/generator.go` | `schema_yaml` 解析 `derived` 块；生成派生骨架 + `FieldSchema()` |
| `internal/studio/handlers.go` | 新增 `/api/fields` 端点 + 字段浏览侧边栏 HTML |

### Deferred

- 内置表达式求值引擎（用户明确选择 Go 实现）
- 跨命令字段 join / 关联查询
- 字段变更历史追踪（字段增删 diff）
- `FieldDef` 持久化到 DB（目前仅内存 registry）
