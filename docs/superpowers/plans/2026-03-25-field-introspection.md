# Field Introspection & Derived Fields Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a queryable field-directory to the parser layer so engineers can ask "what fields does this command produce?" and Rule Studio rules can declare derived fields computed from parsed columns.

**Architecture:** `VendorParser` gains two new interface methods (`FieldSchema` and `SupportedCmdTypes`); a `FieldRegistry` (keyed by `model.CommandType`) is built at startup from every registered parser; `model.ParseResult` gets a generic `Rows` field so generated parsers can pass table data through the pipeline to the scratch pad. The `nethelper rule fields` CLI command and a `/api/fields` Studio endpoint expose the registry to users.

**Tech Stack:** Go 1.22, SQLite (via `github.com/ncruces/go-sqlite3`), Cobra CLI, HTMX (already embedded in `internal/studio/static/`), `encoding/json` (stdlib).

---

## File Structure

### New files
| File | Responsibility |
|------|---------------|
| `internal/parser/field.go` | `FieldType` string enum + `FieldDef` struct |
| `internal/parser/field_registry.go` | `FieldRegistry` type, `BuildFieldRegistry`, `Fields`, `CmdTypes`, `Vendors` |
| `internal/parser/field_registry_test.go` | Unit tests for registry build + lookup |
| `internal/cli/fields.go` | `nethelper rule fields` Cobra command |
| `internal/cli/fields_test.go` | CLI command tests |

### Modified files
| File | What changes |
|------|-------------|
| `internal/model/parse_result.go` | Add `Rows []map[string]string` to `ParseResult`; update `IsEmpty()` |
| `internal/parser/types.go` | Add `FieldSchema(model.CommandType) []FieldDef` and `SupportedCmdTypes() []model.CommandType` to `VendorParser` interface |
| `internal/parser/pipeline.go` | Add Rows → scratch pad branch in `storeResult()`; add `"encoding/json"` import |
| `internal/parser/huawei/huawei.go` | Implement `FieldSchema()` + `SupportedCmdTypes()`; upgrade compile-time guard |
| `internal/parser/cisco/cisco.go` | Same as above |
| `internal/parser/h3c/h3c.go` | Same as above |
| `internal/parser/juniper/juniper.go` | Same as above |
| `internal/parser/huawei/huawei_generated.go` | Add `generatedCmdTypes()`, `generatedFieldSchema()`, two new sentinels, `parser` import |
| `internal/parser/cisco/cisco_generated.go` | Same as above |
| `internal/parser/h3c/h3c_generated.go` | Same as above |
| `internal/parser/juniper/juniper_generated.go` | Same as above |
| `internal/cli/root.go` | Promote `registry` to package-level var; add `fieldRegistry *parser.FieldRegistry`; call `BuildFieldRegistry` |
| `internal/cli/rule.go` | Pass `fieldRegistry` to `studio.NewServer()` |
| `internal/codegen/generator.go` | Parse `derived` block in schema_yaml; generate Rows return; patch two new sentinels |
| `internal/studio/server.go` | Add `fieldReg *parser.FieldRegistry` to `Server` and `NewServer()`; register `/api/fields` route |
| `internal/studio/handlers.go` | Add `fieldReg` to `handlers` struct; implement `/api/fields` handler + sidebar HTML |

---

## Task 1: FieldType + FieldDef + model.ParseResult.Rows

**Files:**
- Create: `internal/parser/field.go`
- Modify: `internal/model/parse_result.go` (lines 23–44)
- Test: `internal/parser/field_registry_test.go` (created in Task 2, but the types are tested there)

- [ ] **Step 1: Create `internal/parser/field.go`**

```go
package parser

// FieldType is the set of legal field value kinds.
type FieldType string

const (
	FieldTypeString FieldType = "string"
	FieldTypeInt    FieldType = "int"
	FieldTypeFloat  FieldType = "float"
	FieldTypeBool   FieldType = "bool"
)

// FieldDef describes one output field produced by a parsed command.
type FieldDef struct {
	Name        string    // snake_case identifier, e.g. "phy_status"
	Type        FieldType // one of the FieldType constants
	Description string    // human-readable description
	Example     string    // representative value, e.g. "up"
	Derived     bool      // true if computed from other fields
	DerivedFrom []string  // source field names for derived fields
}
```

- [ ] **Step 2: Add `Rows` to `ParseResult` and update `IsEmpty()`**

Open `internal/model/parse_result.go`. The current `ParseResult` struct ends at line 34 and `IsEmpty()` is at lines 36–44. Make these exact changes:

```go
// Add Rows as the last field of ParseResult (after RawText, line 33):
Rows []map[string]string `json:"rows,omitempty"` // generic row data from generated parsers

// Update IsEmpty() to include Rows:
func (pr ParseResult) IsEmpty() bool {
	return len(pr.Interfaces) == 0 &&
		len(pr.RIBEntries) == 0 &&
		len(pr.FIBEntries) == 0 &&
		len(pr.LFIBEntries) == 0 &&
		len(pr.Neighbors) == 0 &&
		len(pr.Tunnels) == 0 &&
		len(pr.SRMappings) == 0 &&
		len(pr.Rows) == 0
}
```

- [ ] **Step 3: Run tests to confirm nothing broke**

```bash
go test ./internal/model/... ./internal/parser/...
```

Expected: all existing tests pass (no new tests yet for `Rows` — those come in Tasks 2 and 3).

- [ ] **Step 4: Commit**

```bash
git add internal/parser/field.go internal/model/parse_result.go
git commit -m "feat(parser): add FieldDef types and ParseResult.Rows field"
```

---

## Task 2: VendorParser interface + FieldRegistry

**Files:**
- Modify: `internal/parser/types.go` (lines 9–14)
- Create: `internal/parser/field_registry.go`
- Create: `internal/parser/field_registry_test.go`

- [ ] **Step 1: Add two methods to `VendorParser` interface**

Open `internal/parser/types.go`. The current interface is:

```go
type VendorParser interface {
    Vendor() string
    DetectPrompt(line string) (hostname string, ok bool)
    ClassifyCommand(cmd string) model.CommandType
    ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error)
}
```

Replace it with:

```go
type VendorParser interface {
	Vendor() string
	DetectPrompt(line string) (hostname string, ok bool)
	ClassifyCommand(cmd string) model.CommandType
	ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error)
	// SupportedCmdTypes returns all CommandType values this parser handles,
	// including any dynamically registered generated types.
	SupportedCmdTypes() []model.CommandType
	// FieldSchema returns the field definitions for the given CommandType.
	// Returns nil (not an error) for unknown types.
	FieldSchema(cmdType model.CommandType) []FieldDef
}
```

- [ ] **Step 2: Verify the build breaks on the four vendor parsers**

```bash
go build ./... 2>&1 | grep "does not implement"
```

Expected: four errors like `*huawei.Parser does not implement parser.VendorParser (missing FieldSchema method)`.
This confirms the interface guard is working. Do NOT fix yet — that's Tasks 3–6.

- [ ] **Step 3: Write the failing registry test**

Create `internal/parser/field_registry_test.go`:

```go
package parser_test

import (
	"testing"

	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/parser"
)

// stubParser implements VendorParser for testing only.
type stubParser struct {
	vendor string
	types  []model.CommandType
	schema map[model.CommandType][]parser.FieldDef
}

func (s *stubParser) Vendor() string                { return s.vendor }
func (s *stubParser) DetectPrompt(string) (string, bool) { return "", false }
func (s *stubParser) ClassifyCommand(string) model.CommandType { return model.CmdUnknown }
func (s *stubParser) ParseOutput(model.CommandType, string) (model.ParseResult, error) {
	return model.ParseResult{}, nil
}
func (s *stubParser) SupportedCmdTypes() []model.CommandType { return s.types }
func (s *stubParser) FieldSchema(ct model.CommandType) []parser.FieldDef {
	return s.schema[ct]
}

func TestBuildFieldRegistry(t *testing.T) {
	reg := parser.NewRegistry()
	reg.Register(&stubParser{
		vendor: "test",
		types:  []model.CommandType{model.CmdInterface, model.CmdNeighbor},
		schema: map[model.CommandType][]parser.FieldDef{
			model.CmdInterface: {
				{Name: "name", Type: parser.FieldTypeString, Description: "interface name", Example: "GE0/0/0"},
			},
		},
	})

	fr := parser.BuildFieldRegistry(reg)

	vendors := fr.Vendors()
	if len(vendors) != 1 || vendors[0] != "test" {
		t.Fatalf("expected [test], got %v", vendors)
	}

	types := fr.CmdTypes("test")
	if len(types) != 2 {
		t.Fatalf("expected 2 cmd types, got %d", len(types))
	}

	fields := fr.Fields("test", model.CmdInterface)
	if len(fields) != 1 || fields[0].Name != "name" {
		t.Fatalf("unexpected fields: %+v", fields)
	}

	// Unknown vendor → empty, not panic
	if fr.Fields("unknown", model.CmdInterface) != nil {
		t.Fatal("expected nil for unknown vendor")
	}

	// Known vendor, unknown cmdType → nil
	if fr.Fields("test", model.CmdRIB) != nil {
		t.Fatal("expected nil for unknown cmdType")
	}
}
```

- [ ] **Step 4: Run the test — it will fail to compile (FieldRegistry not yet defined)**

```bash
go test ./internal/parser/... 2>&1 | head -20
```

Expected: compile error `undefined: parser.BuildFieldRegistry`.

- [ ] **Step 5: Create `internal/parser/field_registry.go`**

```go
package parser

import "github.com/xavierli/nethelper/internal/model"

// FieldRegistry is an in-memory index of field definitions,
// keyed by vendor → CommandType.
type FieldRegistry struct {
	index map[string]map[model.CommandType][]FieldDef
	reg   *Registry // kept for ClassifyCommand lookups in the Studio API
}

// BuildFieldRegistry iterates every registered VendorParser, calls
// SupportedCmdTypes() to get the full list, then collects FieldDef
// slices via FieldSchema().
func BuildFieldRegistry(reg *Registry) *FieldRegistry {
	fr := &FieldRegistry{
		index: make(map[string]map[model.CommandType][]FieldDef),
		reg:   reg,
	}
	for _, p := range reg.Parsers() {
		vendor := p.Vendor()
		fr.index[vendor] = make(map[model.CommandType][]FieldDef)
		for _, ct := range p.SupportedCmdTypes() {
			if defs := p.FieldSchema(ct); len(defs) > 0 {
				fr.index[vendor][ct] = defs
			}
		}
	}
	return fr
}

// Fields returns the FieldDef list for the given vendor + CommandType.
// Returns nil if the vendor or CommandType is not registered.
func (r *FieldRegistry) Fields(vendor string, cmdType model.CommandType) []FieldDef {
	if m, ok := r.index[vendor]; ok {
		return m[cmdType]
	}
	return nil
}

// Vendors returns all vendor names in insertion order.
func (r *FieldRegistry) Vendors() []string {
	vendors := make([]string, 0, len(r.index))
	for v := range r.index {
		vendors = append(vendors, v)
	}
	return vendors
}

// CmdTypes returns all CommandType values registered for the given vendor.
func (r *FieldRegistry) CmdTypes(vendor string) []model.CommandType {
	m, ok := r.index[vendor]
	if !ok {
		return nil
	}
	types := make([]model.CommandType, 0, len(m))
	for ct := range m {
		types = append(types, ct)
	}
	return types
}

// ClassifyCommand resolves a raw command string to a CommandType for the given vendor.
// Returns empty string ("") if the vendor is unknown.
func (r *FieldRegistry) ClassifyCommand(vendor, rawCmd string) model.CommandType {
	p, ok := r.reg.Get(vendor)
	if !ok {
		return ""
	}
	return p.ClassifyCommand(rawCmd)
}
```

**Note:** `Registry.Parsers()` already exists in `internal/parser/types.go` (line 46) — no new method needed. `FieldRegistry` also stores `reg *Registry` for the `ClassifyCommand` method used by the Studio API handler.

- [ ] **Step 6: Run the test — now fails because stubParser satisfies interface but vendor parsers still don't**

```bash
go test ./internal/parser/ -run TestBuildFieldRegistry -v
```

Expected: PASS (the stub satisfies the interface; the real vendor parsers are in sub-packages and not compiled here).

- [ ] **Step 7: Run full build to see interface errors**

```bash
go build ./... 2>&1 | grep -c "does not implement"
```

Expected: 4 (one per vendor parser). This is expected — fixed in Tasks 3–6.

- [ ] **Step 8: Commit**

```bash
git add internal/parser/types.go internal/parser/field_registry.go internal/parser/field_registry_test.go
git commit -m "feat(parser): add FieldRegistry and expand VendorParser interface"
```

---

## Task 3: Huawei parser — FieldSchema + SupportedCmdTypes + generated stubs

**Files:**
- Modify: `internal/parser/huawei/huawei.go`
- Modify: `internal/parser/huawei/huawei_generated.go`
- Test: `internal/parser/huawei/generated_test.go` (already exists — extend)

- [ ] **Step 1: Update `huawei_generated.go` — add two new functions and import**

Open `internal/parser/huawei/huawei_generated.go` (currently 24 lines). Replace the entire file with:

```go
// Code generated by nethelper rule-studio. DO NOT EDIT.
// Manual additions belong in huawei.go, not here.

package huawei

import (
	"strings"

	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/parser"
)

func classifyGenerated(cmd string) model.CommandType {
	switch {
	// GENERATED CASES — do not edit this comment
	}
	_ = strings.ToLower // keep import used
	return model.CmdUnknown
}

func parseGenerated(cmdType model.CommandType, raw string) (model.ParseResult, error) {
	switch cmdType {
	// GENERATED PARSE CASES — do not edit this comment
	}
	return model.ParseResult{Type: cmdType, RawText: raw}, nil
}

// generatedCmdTypes returns CommandType values for all approved Rule Studio rules.
func generatedCmdTypes() []model.CommandType {
	return []model.CommandType{
		// GENERATED CMDTYPES — do not edit this comment
	}
}

// generatedFieldSchema returns FieldDef slices for Rule Studio generated parsers.
func generatedFieldSchema(cmdType model.CommandType) []parser.FieldDef {
	switch cmdType {
	// GENERATED FIELD CASES — do not edit this comment
	}
	return nil
}
```

- [ ] **Step 2: Implement `SupportedCmdTypes()` and `FieldSchema()` in `huawei.go`**

Open `internal/parser/huawei/huawei.go`. The current compile-time guard on line 13 is:

```go
var _ interface{ Vendor() string } = (*Parser)(nil)
```

**Replace** that line with:

```go
var _ parser.VendorParser = (*Parser)(nil)
```

Then add these two methods after `ParseOutput` (after line 110):

```go
// SupportedCmdTypes returns all CommandType values the Huawei parser handles.
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
		model.CmdConfigSet,
	}
	return append(base, generatedCmdTypes()...)
}

// FieldSchema returns field definitions for the given CommandType.
func (p *Parser) FieldSchema(cmdType model.CommandType) []parser.FieldDef {
	switch cmdType {
	case model.CmdInterface:
		return []parser.FieldDef{
			{Name: "name",         Type: parser.FieldTypeString, Description: "接口名称", Example: "GigabitEthernet0/0/0"},
			{Name: "phy_status",   Type: parser.FieldTypeString, Description: "物理状态", Example: "up"},
			{Name: "proto_status", Type: parser.FieldTypeString, Description: "协议状态", Example: "up"},
			{Name: "ip_address",   Type: parser.FieldTypeString, Description: "IP 地址", Example: "10.0.0.1"},
			{Name: "mask",         Type: parser.FieldTypeString, Description: "子网掩码", Example: "255.255.255.0"},
			{Name: "bandwidth",    Type: parser.FieldTypeString, Description: "带宽配置", Example: "1000M"},
			{Name: "description",  Type: parser.FieldTypeString, Description: "接口描述", Example: "to-PE1"},
		}
	case model.CmdNeighbor:
		return []parser.FieldDef{
			{Name: "protocol",       Type: parser.FieldTypeString, Description: "邻居协议", Example: "ospf"},
			{Name: "remote_id",      Type: parser.FieldTypeString, Description: "对端 ID", Example: "10.0.0.2"},
			{Name: "remote_address", Type: parser.FieldTypeString, Description: "对端地址", Example: "10.0.0.2"},
			{Name: "state",          Type: parser.FieldTypeString, Description: "邻居状态", Example: "Full"},
			{Name: "uptime",         Type: parser.FieldTypeString, Description: "建立时长", Example: "2d3h"},
		}
	case model.CmdRIB:
		return []parser.FieldDef{
			{Name: "prefix",    Type: parser.FieldTypeString, Description: "目的前缀", Example: "10.0.0.0"},
			{Name: "mask_len",  Type: parser.FieldTypeInt,    Description: "前缀长度", Example: "24"},
			{Name: "protocol",  Type: parser.FieldTypeString, Description: "路由协议", Example: "ospf"},
			{Name: "next_hop",  Type: parser.FieldTypeString, Description: "下一跳", Example: "10.1.0.1"},
			{Name: "interface", Type: parser.FieldTypeString, Description: "出接口", Example: "GE0/0/1"},
			{Name: "preference",Type: parser.FieldTypeInt,    Description: "路由优先级", Example: "10"},
			{Name: "metric",    Type: parser.FieldTypeInt,    Description: "路由开销", Example: "1"},
		}
	case model.CmdConfig, model.CmdConfigSet:
		return []parser.FieldDef{
			{Name: "config_text", Type: parser.FieldTypeString, Description: "设备配置文本", Example: "sysname Router1"},
		}
	default:
		return generatedFieldSchema(cmdType)
	}
}
```

- [ ] **Step 3: Add import for `parser` parent package to `huawei.go`**

Check the existing imports in `huawei.go` (currently imports `model`, `parser` types are referenced via package path). The import block needs:

```go
import (
    "strings"

    "github.com/xavierli/nethelper/internal/model"
    "github.com/xavierli/nethelper/internal/parser"
)
```

**Note:** The sub-package (`internal/parser/huawei`) importing its parent (`internal/parser`) is valid Go — it is NOT circular because the parent `internal/parser` package does NOT import any vendor sub-packages. If `huawei.go` already imports `parser` (it references `parser.VendorParser` in the guard), you just need to ensure the import path is there.

- [ ] **Step 4: Build the huawei package**

```bash
go build ./internal/parser/huawei/...
```

Expected: no errors.

- [ ] **Step 5: Run huawei tests**

```bash
go test ./internal/parser/huawei/... -v
```

Expected: all existing tests pass. The generated_test.go file tests classifyGenerated/parseGenerated — these should still pass.

- [ ] **Step 6: Commit**

```bash
git add internal/parser/huawei/huawei.go internal/parser/huawei/huawei_generated.go
git commit -m "feat(parser/huawei): implement FieldSchema, SupportedCmdTypes, add generated stubs"
```

---

## Task 4: Cisco, H3C, Juniper — FieldSchema + SupportedCmdTypes + generated stubs

**Files:**
- Modify: `internal/parser/cisco/cisco.go`
- Modify: `internal/parser/cisco/cisco_generated.go`
- Modify: `internal/parser/h3c/h3c.go`
- Modify: `internal/parser/h3c/h3c_generated.go`
- Modify: `internal/parser/juniper/juniper.go`
- Modify: `internal/parser/juniper/juniper_generated.go`

The pattern is identical to Task 3. Apply to all three remaining vendors.

- [ ] **Step 1: Update `cisco_generated.go`**

Same template as `huawei_generated.go` from Task 3, but `package cisco`:

```go
// Code generated by nethelper rule-studio. DO NOT EDIT.

package cisco

import (
	"strings"

	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/parser"
)

func classifyGenerated(cmd string) model.CommandType {
	switch {
	// GENERATED CASES — do not edit this comment
	}
	_ = strings.ToLower
	return model.CmdUnknown
}

func parseGenerated(cmdType model.CommandType, raw string) (model.ParseResult, error) {
	switch cmdType {
	// GENERATED PARSE CASES — do not edit this comment
	}
	return model.ParseResult{Type: cmdType, RawText: raw}, nil
}

func generatedCmdTypes() []model.CommandType {
	return []model.CommandType{
		// GENERATED CMDTYPES — do not edit this comment
	}
}

func generatedFieldSchema(cmdType model.CommandType) []parser.FieldDef {
	switch cmdType {
	// GENERATED FIELD CASES — do not edit this comment
	}
	return nil
}
```

- [ ] **Step 2: Update `h3c_generated.go`** — same template, `package h3c`

- [ ] **Step 3: Update `juniper_generated.go`** — same template, `package juniper`

- [ ] **Step 4: Add `SupportedCmdTypes()` + `FieldSchema()` to `cisco.go`**

Upgrade compile-time guard to `var _ parser.VendorParser = (*Parser)(nil)`.

Add these methods (look at the existing `ParseOutput` to find appropriate `cmdType` cases for this vendor):

```go
func (p *Parser) SupportedCmdTypes() []model.CommandType {
	base := []model.CommandType{
		model.CmdInterface,
		model.CmdNeighbor,
		model.CmdRIB,
		model.CmdConfig,
		model.CmdConfigSet,
	}
	return append(base, generatedCmdTypes()...)
}

func (p *Parser) FieldSchema(cmdType model.CommandType) []parser.FieldDef {
	switch cmdType {
	case model.CmdInterface:
		return []parser.FieldDef{
			{Name: "name",        Type: parser.FieldTypeString, Description: "Interface name", Example: "GigabitEthernet0/0"},
			{Name: "status",      Type: parser.FieldTypeString, Description: "Interface status", Example: "up"},
			{Name: "protocol",    Type: parser.FieldTypeString, Description: "Protocol status", Example: "up"},
			{Name: "ip_address",  Type: parser.FieldTypeString, Description: "IP address", Example: "10.0.0.1"},
			{Name: "description", Type: parser.FieldTypeString, Description: "Interface description", Example: "to-PE1"},
		}
	case model.CmdNeighbor:
		return []parser.FieldDef{
			{Name: "protocol",       Type: parser.FieldTypeString, Description: "Protocol", Example: "ospf"},
			{Name: "remote_id",      Type: parser.FieldTypeString, Description: "Neighbor ID", Example: "10.0.0.2"},
			{Name: "remote_address", Type: parser.FieldTypeString, Description: "Neighbor address", Example: "10.0.0.2"},
			{Name: "state",          Type: parser.FieldTypeString, Description: "Neighbor state", Example: "Full"},
			{Name: "uptime",         Type: parser.FieldTypeString, Description: "Uptime", Example: "2d3h"},
		}
	default:
		return generatedFieldSchema(cmdType)
	}
}
```

- [ ] **Step 5: Add `SupportedCmdTypes()` + `FieldSchema()` to `h3c.go`**

Upgrade compile-time guard: replace existing guard (e.g. `var _ interface{ Vendor() string } = (*Parser)(nil)`) with:

```go
var _ parser.VendorParser = (*Parser)(nil)
```

Add the import `"github.com/xavierli/nethelper/internal/parser"` if not present.

Append after `ParseOutput`:

```go
func (p *Parser) SupportedCmdTypes() []model.CommandType {
	base := []model.CommandType{
		model.CmdInterface,
		model.CmdNeighbor,
		model.CmdRIB,
		model.CmdFIB,
		model.CmdLFIB,
		model.CmdConfig,
	}
	return append(base, generatedCmdTypes()...)
}

func (p *Parser) FieldSchema(cmdType model.CommandType) []parser.FieldDef {
	switch cmdType {
	case model.CmdInterface:
		return []parser.FieldDef{
			{Name: "name",         Type: parser.FieldTypeString, Description: "接口名称", Example: "GigabitEthernet0/0/0"},
			{Name: "phy_status",   Type: parser.FieldTypeString, Description: "物理状态", Example: "UP"},
			{Name: "proto_status", Type: parser.FieldTypeString, Description: "协议状态", Example: "UP"},
			{Name: "ip_address",   Type: parser.FieldTypeString, Description: "IP 地址",  Example: "10.0.0.1"},
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
	default:
		return generatedFieldSchema(cmdType)
	}
}
```

- [ ] **Step 6: Add `SupportedCmdTypes()` + `FieldSchema()` to `juniper.go`**

Upgrade compile-time guard to `var _ parser.VendorParser = (*Parser)(nil)`.

Add import `"github.com/xavierli/nethelper/internal/parser"` if not present.

Append after `ParseOutput`:

```go
func (p *Parser) SupportedCmdTypes() []model.CommandType {
	base := []model.CommandType{
		model.CmdInterface,
		model.CmdNeighbor,
		model.CmdRIB,
		model.CmdLFIB,
		model.CmdTunnel,
		model.CmdConfig,
		model.CmdConfigSet,
	}
	return append(base, generatedCmdTypes()...)
}

func (p *Parser) FieldSchema(cmdType model.CommandType) []parser.FieldDef {
	switch cmdType {
	case model.CmdInterface:
		return []parser.FieldDef{
			{Name: "name",        Type: parser.FieldTypeString, Description: "Interface name", Example: "ge-0/0/0"},
			{Name: "phy_status",  Type: parser.FieldTypeString, Description: "Physical state",  Example: "up"},
			{Name: "proto_status",Type: parser.FieldTypeString, Description: "Protocol state",  Example: "up"},
			{Name: "description", Type: parser.FieldTypeString, Description: "Description",     Example: "to-PE1"},
		}
	case model.CmdNeighbor:
		return []parser.FieldDef{
			{Name: "protocol",       Type: parser.FieldTypeString, Description: "Protocol",        Example: "ospf"},
			{Name: "remote_id",      Type: parser.FieldTypeString, Description: "Neighbor ID",     Example: "10.0.0.2"},
			{Name: "remote_address", Type: parser.FieldTypeString, Description: "Neighbor address",Example: "10.0.0.2"},
			{Name: "state",          Type: parser.FieldTypeString, Description: "State",           Example: "Full"},
		}
	default:
		return generatedFieldSchema(cmdType)
	}
}
```

- [ ] **Step 7: Build all vendor sub-packages**

```bash
go build ./internal/parser/...
```

Expected: no errors.

- [ ] **Step 8: Run all parser tests**

```bash
go test ./internal/parser/...
```

Expected: all pass.

- [ ] **Step 9: Commit**

```bash
git add internal/parser/cisco/ internal/parser/h3c/ internal/parser/juniper/
git commit -m "feat(parser): implement FieldSchema+SupportedCmdTypes for cisco, h3c, juniper"
```

---

## Task 5: Pipeline Rows routing + root.go wiring

**Files:**
- Modify: `internal/parser/pipeline.go` (around line 198 — start of `storeResult`)
- Modify: `internal/cli/root.go` (lines 18–26, 55–59)

- [ ] **Step 1: Add Rows branch to `storeResult()` in `pipeline.go`**

Find `storeResult()` at line 198. The **actual** signature is:

```go
func (p *Pipeline) storeResult(deviceID string, snapID int, pr model.ParseResult, capturedAt time.Time, vendor string) error {
```

Add the following block as the **first check** inside `storeResult()` (before the existing `isBulkTableCommand` check). Use `pr` (the actual parameter name), not `result`:

```go
// Generated parsers return row data via Rows; route to scratch pad.
if len(pr.Rows) > 0 {
    rowsJSON, err := json.Marshal(pr.Rows)
    if err != nil {
        rowsJSON = []byte("[]")
    }
    _, _ = p.db.InsertScratch(model.ScratchEntry{ // InsertScratch returns (int, error); both ignored per bulk-table pattern
        DeviceID: deviceID,
        Category: "generated",
        Query:    string(pr.Type),
        Content:  string(rowsJSON),
    })
    return nil
}
```

Add `"encoding/json"` to the import block of `pipeline.go`. Check if it's already there first with `grep -n "encoding/json" internal/parser/pipeline.go`.

- [ ] **Step 2: Promote `registry` to package-level var in `root.go`**

Currently `root.go` has these package-level vars (lines 18–26):

```go
var (
    cfg       *config.Config
    db        *store.DB
    pipeline  *parser.Pipeline
    llmRouter *llm.Router
    version   = "dev"
)
```

Replace with:

```go
var (
    cfg           *config.Config
    db            *store.DB
    pipeline      *parser.Pipeline
    llmRouter     *llm.Router
    registry      *parser.Registry
    fieldRegistry *parser.FieldRegistry
    version       = "dev"
)
```

- [ ] **Step 3: Update `PersistentPreRunE` to assign the package-level `registry` and build `fieldRegistry`**

Currently lines 55–60 look like:

```go
registry := parser.NewRegistry()
registry.Register(huawei.New())
registry.Register(cisco.New())
registry.Register(h3c.New())
registry.Register(juniper.New())
pipeline = parser.NewPipelineWithCollector(db, registry, parser.NewCollector(db))
```

Change `registry :=` to `registry =` (remove `:` to assign the package-level var), then add `fieldRegistry` initialization after pipeline:

```go
registry = parser.NewRegistry()
registry.Register(huawei.New())
registry.Register(cisco.New())
registry.Register(h3c.New())
registry.Register(juniper.New())
pipeline = parser.NewPipelineWithCollector(db, registry, parser.NewCollector(db))
fieldRegistry = parser.BuildFieldRegistry(registry)
```

- [ ] **Step 4: Build and test**

```bash
go build ./... && go test ./internal/parser/... ./internal/cli/...
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/parser/pipeline.go internal/cli/root.go
git commit -m "feat(pipeline): route ParseResult.Rows to scratch pad; wire FieldRegistry at startup"
```

---

## Task 6: `nethelper rule fields` CLI command

**Files:**
- Create: `internal/cli/fields.go`
- Create: `internal/cli/fields_test.go`
- Modify: `internal/cli/rule.go` (add `newRuleFieldsCmd()` to parent)

- [ ] **Step 1: Write the failing CLI test**

Create `internal/cli/fields_test.go`:

```go
package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/parser"
)

func TestRuleFieldsCmd_NoArgs(t *testing.T) {
	// Build a minimal FieldRegistry with one vendor
	reg := parser.NewRegistry()
	reg.Register(&stubVendorParser{vendor: "testvendor"})
	fr := parser.BuildFieldRegistry(reg)

	var buf bytes.Buffer
	cmd := newRuleFieldsCmd(fr, reg)
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "testvendor") {
		t.Errorf("expected testvendor in output, got: %s", out)
	}
}

func TestRuleFieldsCmd_VendorOnly(t *testing.T) {
	reg := parser.NewRegistry()
	reg.Register(&stubVendorParser{
		vendor: "testvendor",
		types:  []model.CommandType{model.CmdInterface},
	})
	fr := parser.BuildFieldRegistry(reg)

	var buf bytes.Buffer
	cmd := newRuleFieldsCmd(fr, reg)
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"testvendor"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "interface") {
		t.Errorf("expected 'interface' in output, got: %s", out)
	}
}

func TestRuleFieldsCmd_VendorAndCommand(t *testing.T) {
	defs := []parser.FieldDef{
		{Name: "name", Type: parser.FieldTypeString, Description: "接口名称", Example: "GE0"},
	}
	reg := parser.NewRegistry()
	reg.Register(&stubVendorParser{
		vendor: "testvendor",
		types:  []model.CommandType{model.CmdInterface},
		schema: map[model.CommandType][]parser.FieldDef{
			model.CmdInterface: defs,
		},
	})
	fr := parser.BuildFieldRegistry(reg)

	var buf bytes.Buffer
	cmd := newRuleFieldsCmd(fr, reg)
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"testvendor", "display interface brief"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "name") {
		t.Errorf("expected 'name' field in output, got: %s", out)
	}
}

// stubVendorParser satisfies parser.VendorParser for tests.
type stubVendorParser struct {
	vendor string
	types  []model.CommandType
	schema map[model.CommandType][]parser.FieldDef
}

func (s *stubVendorParser) Vendor() string                    { return s.vendor }
func (s *stubVendorParser) DetectPrompt(string) (string, bool) { return "", false }
func (s *stubVendorParser) ClassifyCommand(cmd string) model.CommandType {
	if strings.Contains(cmd, "interface") {
		return model.CmdInterface
	}
	return model.CmdUnknown
}
func (s *stubVendorParser) ParseOutput(model.CommandType, string) (model.ParseResult, error) {
	return model.ParseResult{}, nil
}
func (s *stubVendorParser) SupportedCmdTypes() []model.CommandType { return s.types }
func (s *stubVendorParser) FieldSchema(ct model.CommandType) []parser.FieldDef {
	if s.schema != nil {
		return s.schema[ct]
	}
	return nil
}
```

- [ ] **Step 2: Run the test — expect compile failure**

```bash
go test ./internal/cli/ -run TestRuleFieldsCmd -v 2>&1 | head -20
```

Expected: `undefined: newRuleFieldsCmd`.

- [ ] **Step 3: Create `internal/cli/fields.go`**

```go
package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/parser"
)

// newRuleFieldsCmd returns the `nethelper rule fields` subcommand.
// It is passed a FieldRegistry and Registry so it can look up fields
// and resolve raw command strings via ClassifyCommand.
func newRuleFieldsCmd(fr *parser.FieldRegistry, reg *parser.Registry) *cobra.Command {
	return &cobra.Command{
		Use:   "fields [vendor] [command]",
		Short: "Browse parser output fields",
		Long: `Browse the field catalog for parsed command outputs.

  nethelper rule fields                        # list all vendors
  nethelper rule fields huawei                 # list all CommandTypes for huawei
  nethelper rule fields huawei "display interface brief"  # list fields for that command`,
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()

			switch len(args) {
			case 0:
				// List all vendors
				vendors := fr.Vendors()
				if len(vendors) == 0 {
					fmt.Fprintln(w, "No vendors registered.")
					return nil
				}
				fmt.Fprintln(w, "Registered vendors:")
				for _, v := range vendors {
					fmt.Fprintf(w, "  %s\n", v)
				}

			case 1:
				// List all CommandTypes for the vendor
				vendor := args[0]
				types := fr.CmdTypes(vendor)
				if types == nil {
					return fmt.Errorf("unknown vendor %q", vendor)
				}
				fmt.Fprintf(w, "Vendor: %s\n", vendor)
				fmt.Fprintf(w, "%-40s  Fields\n", "CommandType")
				fmt.Fprintln(w, strings.Repeat("─", 60))
				for _, ct := range types {
					defs := fr.Fields(vendor, ct)
					fmt.Fprintf(w, "%-40s  %d\n", string(ct), len(defs))
				}

			default:
				// List fields for vendor + command string
				vendor := args[0]
				rawCmd := strings.Join(args[1:], " ")

				// Resolve raw command string → CommandType via the vendor parser
				p, ok := reg.Get(vendor)
				if !ok {
					return fmt.Errorf("unknown vendor %q", vendor)
				}
				cmdType := p.ClassifyCommand(rawCmd)

				defs := fr.Fields(vendor, cmdType)
				if defs == nil {
					return fmt.Errorf("no fields registered for vendor=%q command=%q (CommandType=%q)", vendor, rawCmd, cmdType)
				}

				fmt.Fprintf(w, "Vendor: %s  Command: %s  (CommandType: %s)\n", vendor, rawCmd, cmdType)
				fmt.Fprintf(w, "%-20s  %-8s  %-8s  %-30s  %s\n", "Field", "Type", "Derived", "From", "Description")
				fmt.Fprintln(w, strings.Repeat("─", 80))
				for _, d := range defs {
					derived := "no"
					from := "—"
					if d.Derived {
						derived = "yes"
						from = strings.Join(d.DerivedFrom, ",")
					}
					fmt.Fprintf(w, "%-20s  %-8s  %-8s  %-30s  %s\n",
						d.Name, string(d.Type), derived, from, d.Description)
				}
			}
			return nil
		},
	}
}
```

**Note:** `Registry.Get(vendor string) VendorParser` must exist. Check `internal/parser/types.go` for the existing `Get` method — it should be there from the original `Registry` implementation. If it only exists as `Detect`, look at the actual method and use the correct name.

- [ ] **Step 4: Run the test**

```bash
go test ./internal/cli/ -run TestRuleFieldsCmd -v
```

Expected: all 3 test cases PASS.

- [ ] **Step 5: Wire `newRuleFieldsCmd` into `newRuleCmd()` in `rule.go`**

Open `internal/cli/rule.go`, find `newRuleCmd()` (lines 18–29). Currently it adds 4 subcommands. Add the fields command:

```go
func newRuleCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "rule",
        Short: "Parser Rule Studio — discover, review and promote parser rules",
    }
    cmd.AddCommand(newRuleStudioCmd())
    cmd.AddCommand(newRuleDiscoverCmd())
    cmd.AddCommand(newRuleListCmd())
    cmd.AddCommand(newRuleRegenCmd())
    cmd.AddCommand(newRuleHistoryCmd())
    cmd.AddCommand(newRuleFieldsCmd(fieldRegistry, registry)) // ← add this line
    return cmd
}
```

- [ ] **Step 6: Build and run full tests**

```bash
go build ./... && go test ./internal/cli/...
```

Expected: all pass.

- [ ] **Step 7: Smoke-test the command**

```bash
go build -o /tmp/nethelper-test ./cmd/nethelper && /tmp/nethelper-test rule fields
```

Expected: output lists `huawei`, `cisco`, `h3c`, `juniper`.

- [ ] **Step 8: Commit**

```bash
git add internal/cli/fields.go internal/cli/fields_test.go internal/cli/rule.go
git commit -m "feat(cli): add 'nethelper rule fields' command for field catalog browsing"
```

---

## Task 7: Studio `/api/fields` endpoint + sidebar

**Files:**
- Modify: `internal/studio/server.go`
- Modify: `internal/studio/handlers.go`
- Modify: `internal/cli/rule.go` (update `studio.NewServer()` call to pass `fieldRegistry`)

- [ ] **Step 1: Add `fieldReg` to `Server` struct and `NewServer()` in `server.go`**

Open `internal/studio/server.go`. Current `Server` struct (lines 20–26):

```go
type Server struct {
    mux      *http.ServeMux
    db       *store.DB
    eng      *discovery.Engine
    llmR     *llm.Router
    generate GenerateFn
}
```

Replace with:

```go
type Server struct {
    mux      *http.ServeMux
    db       *store.DB
    eng      *discovery.Engine
    llmR     *llm.Router
    generate GenerateFn
    fieldReg *parser.FieldRegistry
}
```

Add import `"github.com/xavierli/nethelper/internal/parser"` to the import block.

Current `NewServer()` (lines 28–33):

```go
func NewServer(db *store.DB, eng *discovery.Engine, llmR *llm.Router, generate GenerateFn) *Server {
    s := &Server{mux: http.NewServeMux(), db: db, eng: eng, llmR: llmR, generate: generate}
    s.registerRoutes()
    return s
}
```

Replace with:

```go
func NewServer(db *store.DB, eng *discovery.Engine, llmR *llm.Router, generate GenerateFn, fieldReg *parser.FieldRegistry) *Server {
    s := &Server{mux: http.NewServeMux(), db: db, eng: eng, llmR: llmR, generate: generate, fieldReg: fieldReg}
    s.registerRoutes()
    return s
}
```

In `registerRoutes()`, add the new route before the closing brace:

```go
s.mux.HandleFunc("/api/fields", h.apiFields)
```

- [ ] **Step 2: Update `handlers` struct and implement `apiFields` in `handlers.go`**

Open `internal/studio/handlers.go`. Current `handlers` struct (lines 20–24):

```go
type handlers struct {
    db       *store.DB
    eng      *discovery.Engine
    generate GenerateFn
}
```

Replace with:

```go
type handlers struct {
    db       *store.DB
    eng      *discovery.Engine
    generate GenerateFn
    fieldReg *parser.FieldRegistry
}
```

Add the import `"github.com/xavierli/nethelper/internal/parser"` if not already present.

Find where `handlers` is constructed inside `registerRoutes()` in `server.go`:

```go
h := &handlers{db: s.db, eng: s.eng, generate: s.generate}
```

Replace with:

```go
h := &handlers{db: s.db, eng: s.eng, generate: s.generate, fieldReg: s.fieldReg}
```

Now add the `apiFields` handler to `handlers.go` (append to the end of the file before closing):

```go
// apiFields handles GET /api/fields
// Query params:
//   vendor=huawei                            → {"cmdTypes": [...]}
//   vendor=huawei&command=display+interface  → {"cmdType":"interface","fields":[...]}
//   (no params)                              → {"vendors": [...]}
func (h *handlers) apiFields(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    if h.fieldReg == nil {
        http.Error(w, `{"error":"field registry not available"}`, http.StatusServiceUnavailable)
        return
    }

    vendor := r.URL.Query().Get("vendor")
    command := r.URL.Query().Get("command")

    if vendor == "" {
        // Return list of vendors
        vendors := h.fieldReg.Vendors()
        json.NewEncoder(w).Encode(map[string]any{"vendors": vendors})
        return
    }

    if command == "" {
        // Return list of CommandTypes for vendor
        types := h.fieldReg.CmdTypes(vendor)
        if types == nil {
            http.Error(w, `{"error":"unknown vendor"}`, http.StatusNotFound)
            return
        }
        strs := make([]string, len(types))
        for i, t := range types {
            strs[i] = string(t)
        }
        json.NewEncoder(w).Encode(map[string]any{"cmdTypes": strs})
        return
    }

    // Return fields for vendor + command
    // We need the registry to call ClassifyCommand — stored on FieldRegistry is not enough.
    // Use h.fieldReg to check existence, but we need a Registry reference.
    // Solution: FieldRegistry exposes ClassifyCommand indirectly via storing a registry ref.
    // IMPLEMENTATION NOTE: add a Registry *parser.Registry field to FieldRegistry, or pass
    // it separately. See the note below on the simplest approach.
    //
    // Simplest approach: add reg *parser.Registry to FieldRegistry (set in BuildFieldRegistry).
    cmdType := h.fieldReg.ClassifyCommand(vendor, command)
    if cmdType == "" {
        http.Error(w, `{"error":"unknown command"}`, http.StatusNotFound)
        return
    }
    fields := h.fieldReg.Fields(vendor, cmdType)
    json.NewEncoder(w).Encode(map[string]any{
        "cmdType": string(cmdType),
        "fields":  fields,
    })
}
```

**Important implementation note on `ClassifyCommand` in the API handler:** This is already handled — `FieldRegistry` stores a `reg *Registry` (set in `BuildFieldRegistry`) and exposes a `ClassifyCommand(vendor, rawCmd string) model.CommandType` method (defined in Task 2's `field_registry.go`). The handler simply calls `h.fieldReg.ClassifyCommand(vendor, command)`. No changes to `field_registry.go` are needed in this task.

- [ ] **Step 3: Update `NewServer()` call in `rule.go`**

Open `internal/cli/rule.go`, find `newRuleStudioCmd()`. The current call to `studio.NewServer` (around line 42) is:

```go
srv := studio.NewServer(db, eng, llmRouter, generateFn)
```

Replace with:

```go
srv := studio.NewServer(db, eng, llmRouter, generateFn, fieldRegistry)
```

- [ ] **Step 4: Build**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 5: Write a test for the `/api/fields` endpoint**

Open `internal/studio/server_test.go` (already exists). Add:

```go
func TestAPIFields(t *testing.T) {
	db := openTestDB(t)

	// Build a real FieldRegistry with a stub parser via parser.Registry
	reg := parser.NewRegistry()
	reg.Register(&stubFieldParser{})
	fr := parser.BuildFieldRegistry(reg)

	srv := NewServer(db, nil, nil, nil, fr)

	tests := []struct {
		query    string
		wantKey  string
		wantVal  string
	}{
		{"/api/fields", "vendors", "testvendor"},
		{"/api/fields?vendor=testvendor", "cmdTypes", "interface"},
		{"/api/fields?vendor=testvendor&command=display+interface+brief", "cmdType", "interface"},
	}
	for _, tc := range tests {
		req := httptest.NewRequest(http.MethodGet, tc.query, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s: status %d", tc.query, w.Code)
		}
		if !strings.Contains(w.Body.String(), tc.wantVal) {
			t.Errorf("GET %s: expected %q in body, got: %s", tc.query, tc.wantVal, w.Body.String())
		}
	}
}

// stubFieldParser satisfies parser.VendorParser for studio tests.
type stubFieldParser struct{}

func (s *stubFieldParser) Vendor() string                              { return "testvendor" }
func (s *stubFieldParser) DetectPrompt(string) (string, bool)          { return "", false }
func (s *stubFieldParser) ClassifyCommand(cmd string) model.CommandType {
	if strings.Contains(cmd, "interface") { return model.CmdInterface }
	return model.CmdUnknown
}
func (s *stubFieldParser) ParseOutput(model.CommandType, string) (model.ParseResult, error) {
	return model.ParseResult{}, nil
}
func (s *stubFieldParser) SupportedCmdTypes() []model.CommandType {
	return []model.CommandType{model.CmdInterface}
}
func (s *stubFieldParser) FieldSchema(ct model.CommandType) []parser.FieldDef {
	if ct == model.CmdInterface {
		return []parser.FieldDef{{Name: "name", Type: parser.FieldTypeString}}
	}
	return nil
}
```

Add the necessary imports to `server_test.go` (`net/http/httptest`, `strings`, `github.com/xavierli/nethelper/internal/parser`, `github.com/xavierli/nethelper/internal/model`).

- [ ] **Step 7: Run tests**

```bash
go test ./internal/studio/... -v
```

Expected: all tests pass including the new `TestAPIFields`.

- [ ] **Step 8: Commit**

```bash
git add internal/studio/server.go internal/studio/handlers.go internal/studio/server_test.go internal/cli/rule.go internal/parser/field_registry.go
git commit -m "feat(studio): add /api/fields endpoint for field catalog browsing"
```

---

## Task 8: Code generator — `derived` block + Rows return + new sentinel patches

**Files:**
- Modify: `internal/codegen/generator.go`

This task adds support for the `derived` YAML block in `schema_yaml` and patches the two new sentinels (`// GENERATED CMDTYPES`, `// GENERATED FIELD CASES`).

- [ ] **Step 1: Read the existing generator to understand the schema YAML parsing**

The current `generateTableBody()` (lines 107–140 of `generator.go`) unmarshals `schema_yaml` into a struct. Check the exact struct used. It currently has `HeaderPattern`, `SkipLines`, `Columns []ColumnDef`. The `derived` block is new.

- [ ] **Step 2: Add `DerivedDef` and extend `tableSchema` struct**

Find the struct (inside `generator.go`) used to unmarshal `schema_yaml`. Add:

```go
type derivedDef struct {
    Name        string   `yaml:"name"`
    Type        string   `yaml:"type"`
    Description string   `yaml:"description"`
    From        []string `yaml:"from"`
    Example     string   `yaml:"example"`
}

// Extend the existing tableSchemaYAML struct:
type tableSchemaYAML struct {
    HeaderPattern string       `yaml:"header_pattern"`
    SkipLines     int          `yaml:"skip_lines"`
    Columns       []columnYAML `yaml:"columns"`
    Derived       []derivedDef `yaml:"derived"` // ← new
}
```

- [ ] **Step 3: Add schema validation for `derived` fields**

In the function that parses `schema_yaml` (inside `generateTableBody` or its caller), after unmarshaling, add validation:

```go
validFieldTypes := map[string]bool{"string": true, "int": true, "float": true, "bool": true}

// Build set of column names for from-reference validation
colNames := map[string]bool{}
for _, c := range schema.Columns {
    colNames[c.Name] = true
}

for _, d := range schema.Derived {
    if !validFieldTypes[d.Type] {
        return "", fmt.Errorf("derived field %q has invalid type %q (must be string|int|float|bool)", d.Name, d.Type)
    }
    for _, ref := range d.From {
        if !colNames[ref] {
            return "", fmt.Errorf("derived field %q references unknown column %q", d.Name, ref)
        }
    }
}
```

- [ ] **Step 4: Modify `generateTableBody()` to emit derived field skeleton and `Rows` return**

In the generated function body, after the `engine.ParseTable(...)` call and before the `return`, insert the derived-fields loop and change the return to include `Rows`:

The current generated return is (inside the template string in `GenerateParserFile`):

```go
return model.ParseResult{Type: model.CommandType("generated:{{.Vendor}}:{{.Stem}}"), RawText: raw}, nil
```

Change it to:

```go
{{if .DerivedFields}}
    // Derived fields — implement each TODO below
    for i := range tableResult.Rows {
        row := tableResult.Rows[i]
        {{range .DerivedFields}}// derived: {{.Name}} ({{.Type}}) from [{{join .From ", "}}]
        // TODO: row["{{.Name}}"] = ...
        {{end}}_ = row
    }
{{end}}
    return model.ParseResult{
        Type:    model.CommandType("generated:{{.Vendor}}:{{.Stem}}"),
        RawText: raw,
        Rows:    tableResult.Rows,
    }, nil
```

Also add a `FieldSchemaBody` to the template data so the generated `FieldSchema` case can be patched.

- [ ] **Step 5: Update `PatchGeneratedFile()` to patch the two new sentinels**

Current `PatchGeneratedFile` patches two sentinels. Add two more:

```go
// Patch 3: GENERATED CMDTYPES — insert the new CommandType value
cmdTypeLine := fmt.Sprintf("\t\tmodel.CommandType(%q),\n\t\t// GENERATED CMDTYPES", cmdTypeStr)
content = strings.Replace(content, "// GENERATED CMDTYPES — do not edit this comment", cmdTypeLine, 1)

// Patch 4: GENERATED FIELD CASES — insert the FieldSchema case
fieldCase := buildFieldSchemaCase(cmdTypeStr, rule)
fieldCaseLine := fieldCase + "\n\t// GENERATED FIELD CASES — do not edit this comment"
content = strings.Replace(content, "// GENERATED FIELD CASES — do not edit this comment", fieldCaseLine, 1)
```

Implement `buildFieldSchemaCase(cmdTypeStr string, rule store.PendingRule) string` to generate the `case model.CommandType("..."):` block with all columns (and derived fields if present) as `FieldDef` entries.

- [ ] **Step 6: Write a test for the new derived-field generation**

In `internal/codegen/generator_test.go`, add:

```go
func TestGenerateParserFile_WithDerived(t *testing.T) {
    rule := store.PendingRule{
        Vendor:         "huawei",
        CommandPattern: "display traffic-policy statistics interface",
        OutputType:     "table",
        SchemaYAML: `header_pattern: "Interface\\s+InOctets\\s+Bandwidth"
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
`,
    }
    out, err := GenerateParserFile(rule)
    if err != nil {
        t.Fatal(err)
    }
    if !strings.Contains(out, `row["util_pct"]`) {
        t.Errorf("expected derived field TODO in output, got:\n%s", out)
    }
    if !strings.Contains(out, "Rows: tableResult.Rows") {
        t.Errorf("expected Rows in return statement, got:\n%s", out)
    }
}

func TestGenerateParserFile_DerivedValidation(t *testing.T) {
    rule := store.PendingRule{
        Vendor:         "huawei",
        CommandPattern: "display x",
        OutputType:     "table",
        SchemaYAML: `header_pattern: "X"
columns:
  - name: col_a
    index: 0
    type: string
derived:
  - name: bad_derived
    type: float
    from: ["nonexistent_column"]
`,
    }
    _, err := GenerateParserFile(rule)
    if err == nil {
        t.Fatal("expected validation error for unknown column reference")
    }
}
```

- [ ] **Step 7: Run codegen tests**

```bash
go test ./internal/codegen/... -v
```

Expected: existing tests pass + new tests pass.

- [ ] **Step 8: Build everything**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 9: Commit**

```bash
git add internal/codegen/generator.go internal/codegen/generator_test.go
git commit -m "feat(codegen): support derived fields in schema_yaml + patch new generated sentinels"
```

---

## Task 9: Full integration test + final verification

- [ ] **Step 1: Run complete test suite**

```bash
go test ./... 2>&1
```

Expected: all packages pass, zero failures.

- [ ] **Step 2: `go vet`**

```bash
go vet ./...
```

Expected: no issues.

- [ ] **Step 3: End-to-end smoke test of `rule fields`**

```bash
go build -o /tmp/nh ./cmd/nethelper
/tmp/nh rule fields
/tmp/nh rule fields huawei
/tmp/nh rule fields huawei "display interface brief"
```

Expected outputs:
1. Lists `huawei`, `cisco`, `h3c`, `juniper`
2. Lists CommandTypes for huawei (e.g. `interface`, `neighbor`, `rib`, ...)
3. Shows field table for `interface` CommandType with columns: name, phy_status, proto_status, ip_address, mask, bandwidth, description

- [ ] **Step 4: Final commit (if any cleanup needed)**

```bash
git add -p  # review any remaining unstaged changes
go test ./...  # confirm still clean
git commit -m "chore: final cleanup for field introspection feature"
```

---

## Notes for Implementers

- **`Registry.Get(vendor)`**: Check `internal/parser/types.go` — the method may be named differently. Use `grep -n "func.*Registry" internal/parser/types.go` to find it.
- **`strings` import in `*_generated.go`**: The `_ = strings.ToLower` trick keeps the `strings` import live for the `classifyGenerated` switch. If your generated file doesn't need it initially, remove it and the import — the real `strings` usage appears when the first rule is patched.
- **Template functions in `generator.go`**: If `GenerateParserFile` uses `text/template`, you'll need to register a `join` template function: `template.FuncMap{"join": strings.Join}`.
- **`model.CommandType` is a string alias**: `string(result.Type)` works; no `.String()` method required.
- **Sub-package importing parent package**: `internal/parser/huawei` importing `internal/parser` is legal in Go. The reverse (parent importing child) would be a cycle — but we never do that.
