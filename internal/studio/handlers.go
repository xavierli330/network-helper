package studio

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/xavierli/nethelper/internal/codegen"
	"github.com/xavierli/nethelper/internal/discovery"
	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/parser"
	"github.com/xavierli/nethelper/internal/parser/engine"
	"github.com/xavierli/nethelper/internal/store"
	"gopkg.in/yaml.v3"
)

type handlers struct {
	db       *store.DB
	eng      *discovery.Engine
	generate GenerateFn // may be nil
	fieldReg *parser.FieldRegistry
	repoRoot string
}

var funcMap = template.FuncMap{
	"mul": func(a, b float64) float64 { return a * b },
}

func (h *handlers) list(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	draft, err := h.db.ListPendingRules("draft", 50)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	testing_, err := h.db.ListPendingRules("testing", 50)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	rules := append(draft, testing_...)

	unknowns, _ := h.db.ListUnknownOutputs("", "new", 1000)
	unknownCount := len(unknowns)

	tmpl := template.Must(template.New("list").Funcs(funcMap).Parse(listHTML))
	tmpl.Execute(w, struct {
		Rules        []store.PendingRule
		UnknownCount int
	}{Rules: rules, UnknownCount: unknownCount})
}

func (h *handlers) ruleDispatch(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/rule/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 2 && parts[1] == "sandbox" {
		h.sandbox(w, r, id)
		return
	}
	h.editor(w, r, id)
}

func (h *handlers) editor(w http.ResponseWriter, r *http.Request, id int) {
	rule, err := h.db.GetPendingRule(id)
	if err != nil {
		http.Error(w, "rule not found", 404)
		return
	}
	if r.Method == "POST" {
		r.ParseForm()
		rule.SchemaYAML = r.FormValue("schema_yaml")
		rule.GoCodeDraft = r.FormValue("go_code_draft")
		rule.Status = "testing"
		h.db.UpdatePendingRule(rule)
		http.Redirect(w, r, fmt.Sprintf("/rule/%d/sandbox", id), http.StatusFound)
		return
	}
	tmpl := template.Must(template.New("editor").Funcs(funcMap).Parse(editorHTML))
	tmpl.Execute(w, rule)
}

func (h *handlers) sandbox(w http.ResponseWriter, r *http.Request, id int) {
	rule, err := h.db.GetPendingRule(id)
	if err != nil {
		http.Error(w, "rule not found", 404)
		return
	}
	count, _ := h.db.CountRuleTestCases(id)
	data := struct {
		Rule      store.PendingRule
		TestCount int
	}{Rule: rule, TestCount: count}
	tmpl := template.Must(template.New("sandbox").Funcs(funcMap).Parse(sandboxHTML))
	tmpl.Execute(w, data)
}

func (h *handlers) apiDispatch(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/rule/"), "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch parts[1] {
	case "test":
		h.apiTest(w, r, id)
	case "testcase":
		h.apiSaveTestCase(w, r, id)
	case "approve":
		h.apiApprove(w, r, id)
	case "ignore":
		h.apiIgnore(w, r, id)
	case "save-local":
		h.apiSaveLocal(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (h *handlers) apiTest(w http.ResponseWriter, r *http.Request, id int) {
	rule, err := h.db.GetPendingRule(id)
	if err != nil {
		jsonError(w, "rule not found", 404)
		return
	}
	if rule.OutputType != "table" {
		jsonError(w, "live test only supported for table-type rules", 400)
		return
	}
	r.ParseForm()
	input := r.FormValue("input")

	var schemaDef struct {
		HeaderPattern string `yaml:"header_pattern"`
		SkipLines     int    `yaml:"skip_lines"`
		Columns       []struct {
			Name     string `yaml:"name"`
			Index    int    `yaml:"index"`
			Type     string `yaml:"type"`
			Optional bool   `yaml:"optional"`
		} `yaml:"columns"`
	}
	if err := yaml.Unmarshal([]byte(rule.SchemaYAML), &schemaDef); err != nil {
		jsonError(w, "invalid schema YAML: "+err.Error(), 400)
		return
	}
	cols := make([]engine.ColumnDef, len(schemaDef.Columns))
	for i, c := range schemaDef.Columns {
		cols[i] = engine.ColumnDef{Name: c.Name, Index: c.Index, Type: c.Type, Optional: c.Optional}
	}
	schema := engine.TableSchema{
		HeaderPattern: schemaDef.HeaderPattern,
		SkipLines:     schemaDef.SkipLines,
		Columns:       cols,
	}
	result, err := engine.ParseTable(schema, input)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *handlers) apiSaveTestCase(w http.ResponseWriter, r *http.Request, id int) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	r.ParseForm()
	input := r.FormValue("input")
	expected := r.FormValue("expected")
	if input == "" || expected == "" {
		jsonError(w, "input and expected are required", 400)
		return
	}
	tcID, err := h.db.CreateRuleTestCase(store.RuleTestCase{
		RuleID:      id,
		Description: r.FormValue("description"),
		Input:       input,
		Expected:    expected,
	})
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"id": tcID})
}

// apiApprove triggers the code generator and records approval.
func (h *handlers) apiApprove(w http.ResponseWriter, r *http.Request, id int) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	if h.generate == nil {
		jsonError(w, "code generator not available (codegen not wired)", 503)
		return
	}
	rule, err := h.db.GetPendingRule(id)
	if err != nil {
		jsonError(w, "rule not found", 404)
		return
	}
	testCases, err := h.db.ListRuleTestCases(id)
	if err != nil || len(testCases) == 0 {
		jsonError(w, "at least one test case is required before approving", 400)
		return
	}

	approvedBy := ""
	if u, err := user.Current(); err == nil {
		approvedBy = u.Username
	}

	repoRoot, _ := os.Getwd()
	prURL, err := h.generate(rule, testCases, repoRoot, approvedBy)
	if err != nil {
		jsonError(w, "code generation failed: "+err.Error(), 500)
		return
	}

	vendor := strings.ToLower(rule.Vendor)
	lower := strings.ToLower(strings.TrimSpace(rule.CommandPattern))
	for _, p := range []string{"display ", "show ", "dis ", "sh "} {
		lower = strings.TrimPrefix(lower, p)
	}
	importParts := strings.FieldsFunc(lower, func(r rune) bool { return r == ' ' || r == '-' })
	goFilePath := fmt.Sprintf("internal/parser/%s/%s.go", vendor, strings.Join(importParts, "_"))

	h.db.ApprovePendingRule(id, approvedBy, time.Now())
	h.db.SetPendingRulePR(id, prURL, goFilePath)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *handlers) apiIgnore(w http.ResponseWriter, r *http.Request, id int) {
	rule, err := h.db.GetPendingRule(id)
	if err != nil {
		jsonError(w, "rule not found", 404)
		return
	}
	rule.Status = "rejected"
	h.db.UpdatePendingRule(rule)
	http.Redirect(w, r, "/", http.StatusFound)
}

// ── Parser Tester ────────────────────────────────────────────────────────────

func (h *handlers) tester(w http.ResponseWriter, r *http.Request) {
	var vendors []string
	if h.fieldReg != nil {
		vendors = h.fieldReg.Vendors()
	}
	tmpl := template.Must(template.New("tester").Funcs(funcMap).Parse(testPageHTML))
	tmpl.Execute(w, vendors)
}

func (h *handlers) apiParserTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	if h.fieldReg == nil {
		jsonError(w, "field registry not available", 503)
		return
	}
	r.ParseForm()
	vendor := r.FormValue("vendor")
	command := r.FormValue("command")
	output := r.FormValue("output")

	reg := h.fieldReg.Reg()
	p, ok := reg.Get(vendor)
	if !ok {
		jsonError(w, fmt.Sprintf("unknown vendor: %s", vendor), 400)
		return
	}
	cmdType := p.ClassifyCommand(command)
	matched := cmdType != model.CmdUnknown

	result, _ := p.ParseOutput(cmdType, output)

	preview := map[string]any{
		"cmdType":        string(cmdType),
		"matched":        matched,
		"interfaceCount": len(result.Interfaces),
		"neighborCount":  len(result.Neighbors),
		"rowCount":       len(result.Rows),
	}
	if len(result.Interfaces) > 0 {
		n := 5
		if len(result.Interfaces) < n {
			n = len(result.Interfaces)
		}
		preview["interfaces"] = result.Interfaces[:n]
	}
	if len(result.Neighbors) > 0 {
		n := 5
		if len(result.Neighbors) < n {
			n = len(result.Neighbors)
		}
		preview["neighbors"] = result.Neighbors[:n]
	}
	if len(result.Rows) > 0 {
		n := 5
		if len(result.Rows) < n {
			n = len(result.Rows)
		}
		preview["rows"] = result.Rows[:n]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(preview)
}

// ── Unknown Outputs Browser ───────────────────────────────────────────────────

func (h *handlers) unknownList(w http.ResponseWriter, r *http.Request) {
	outputs, err := h.db.ListUnknownOutputs("", "new", 500)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	tmpl := template.Must(template.New("unknown").Funcs(funcMap).Parse(unknownListHTML))
	tmpl.Execute(w, outputs)
}

func (h *handlers) unknownDispatch(w http.ResponseWriter, r *http.Request) {
	// Path: /api/unknown/:id/generate or /api/unknown/:id/ignore
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/unknown/"), "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch parts[1] {
	case "generate":
		h.apiUnknownGenerate(w, r, id)
	case "ignore":
		h.apiUnknownIgnore(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (h *handlers) apiUnknownGenerate(w http.ResponseWriter, r *http.Request, id int) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	if h.eng == nil {
		jsonError(w, "discovery engine not configured", 503)
		return
	}
	ruleID, err := h.eng.GenerateForUnknown(r.Context(), id)
	if err != nil {
		jsonError(w, "generate failed: "+err.Error(), 500)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/rule/%d/sandbox", ruleID), http.StatusFound)
}

func (h *handlers) apiUnknownIgnore(w http.ResponseWriter, r *http.Request, id int) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	u, err := h.db.GetUnknownOutputByID(id)
	if err != nil {
		jsonError(w, "unknown output not found", 404)
		return
	}
	h.db.UpdateUnknownOutputStatus(u.Vendor, u.CommandNorm, "ignored")
	// Return HTMX fragment to remove the row
	fmt.Fprintf(w, `<tr id="unknown-%d" style="display:none"></tr>`, id)
}

// ── Local Save ────────────────────────────────────────────────────────────────

func (h *handlers) apiSaveLocal(w http.ResponseWriter, r *http.Request, id int) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	rule, err := h.db.GetPendingRule(id)
	if err != nil {
		jsonError(w, "rule not found", 404)
		return
	}
	testCases, err := h.db.ListRuleTestCases(id)
	if err != nil || len(testCases) == 0 {
		jsonError(w, "at least one test case is required before saving locally", 400)
		return
	}

	root := h.repoRoot
	if root == "" {
		root, err = discoverRepoRoot()
		if err != nil {
			jsonError(w, "cannot detect repo root: "+err.Error(), 500)
			return
		}
	}

	paths, buildOut, buildErr := codegen.GenerateLocal(rule, testCases, root)

	type saveResult struct {
		Success     bool     `json:"success"`
		Paths       []string `json:"paths,omitempty"`
		BuildOutput string   `json:"build_output"`
		Error       string   `json:"error,omitempty"`
		Message     string   `json:"message,omitempty"`
	}

	w.Header().Set("Content-Type", "application/json")
	if buildErr != nil {
		json.NewEncoder(w).Encode(saveResult{
			Success:     false,
			Paths:       paths,
			BuildOutput: buildOut,
			Error:       buildErr.Error(),
		})
		return
	}

	approvedBy := ""
	if u, err2 := user.Current(); err2 == nil {
		approvedBy = u.Username
	}
	h.db.ApprovePendingRule(id, approvedBy, time.Now())
	if len(paths) > 0 {
		h.db.SetPendingRulePR(id, "local", paths[0])
	}

	json.NewEncoder(w).Encode(saveResult{
		Success:     true,
		Paths:       paths,
		BuildOutput: buildOut,
		Message:     "Parser active. Run `go test ./internal/parser/...` to verify.",
	})
}

// discoverRepoRoot runs git rev-parse --show-toplevel to find the repo root.
func discoverRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (h *handlers) apiDiscover(w http.ResponseWriter, r *http.Request) {
	if h.eng == nil {
		jsonError(w, "discovery engine not configured", 503)
		return
	}
	vendor := r.URL.Query().Get("vendor")
	n, err := h.eng.RunOnce(r.Context(), vendor)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"created": n})
}

func (h *handlers) fields(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, fieldsHTML)
}

func (h *handlers) apiFieldsVendorsHTML(w http.ResponseWriter, r *http.Request) {
	if h.fieldReg == nil {
		fmt.Fprint(w, `<p style="color:red">Field registry not available</p>`)
		return
	}
	vendors := h.fieldReg.Vendors()
	var b strings.Builder
	b.WriteString(`<div>`)
	for _, v := range vendors {
		fmt.Fprintf(&b,
			`<button class="vendor-btn" `+
				`hx-get="/api/fields/schema-html?vendor=%s" `+
				`hx-target="#schema-panel" `+
				`hx-swap="innerHTML" `+
				`onclick="document.querySelectorAll('.vendor-btn').forEach(b=>b.classList.remove('active'));this.classList.add('active')">`+
				`%s</button>`, v, v)
	}
	b.WriteString(`</div>`)
	fmt.Fprint(w, b.String())
}

func (h *handlers) apiFieldsSchemaHTML(w http.ResponseWriter, r *http.Request) {
	if h.fieldReg == nil {
		fmt.Fprint(w, `<p style="color:red">Field registry not available</p>`)
		return
	}
	vendor := r.URL.Query().Get("vendor")
	cmdtype := r.URL.Query().Get("cmdtype")

	types := h.fieldReg.CmdTypes(vendor)
	if types == nil {
		fmt.Fprintf(w, `<p style="color:red">Unknown vendor: %s</p>`, vendor)
		return
	}

	var b strings.Builder

	b.WriteString(`<div style="display:grid;grid-template-columns:220px 1fr;gap:1rem">`)

	// CmdType list
	b.WriteString(`<div>`)
	fmt.Fprintf(&b, `<h4>%s — CommandTypes</h4>`, vendor)
	for _, ct := range types {
		defs := h.fieldReg.Fields(vendor, ct)
		activeClass := ""
		if string(ct) == cmdtype {
			activeClass = " active"
		}
		fmt.Fprintf(&b,
			`<div class="cmdtype-row%s" `+
				`hx-get="/api/fields/schema-html?vendor=%s&cmdtype=%s" `+
				`hx-target="closest .layout > #schema-panel" `+
				`hx-swap="innerHTML">`+
				`<span>%s</span><span class="field-count">%d fields</span></div>`,
			activeClass, vendor, string(ct), string(ct), len(defs))
	}
	b.WriteString(`</div>`)

	// Field table for selected cmdtype (if any)
	b.WriteString(`<div>`)
	if cmdtype != "" {
		defs := h.fieldReg.Fields(vendor, model.CommandType(cmdtype))
		if len(defs) == 0 {
			fmt.Fprintf(&b, `<p style="color:#888">No fields defined for <code>%s / %s</code> yet.</p>`, vendor, cmdtype)
		} else {
			fmt.Fprintf(&b, `<h4>%s / %s</h4>`, vendor, cmdtype)
			b.WriteString(`<table><tr><th>Field</th><th>Type</th><th>Description</th><th>Example</th><th>Derived</th></tr>`)
			for _, d := range defs {
				derived := ""
				if d.Derived {
					from := strings.Join(d.DerivedFrom, ", ")
					derived = fmt.Sprintf(`<span class="tag tag-derived">from: %s</span>`, from)
				}
				fmt.Fprintf(&b, `<tr><td><code>%s</code></td><td><span class="tag">%s</span></td><td>%s</td><td><code>%s</code></td><td>%s</td></tr>`,
					d.Name, string(d.Type), d.Description, d.Example, derived)
			}
			b.WriteString(`</table>`)
		}
	} else {
		b.WriteString(`<p style="color:#888">← Select a command type</p>`)
	}
	b.WriteString(`</div>`)

	b.WriteString(`</div>`) // close grid

	fmt.Fprint(w, b.String())
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// apiFields handles GET /api/fields
// Query params:
//
//	(no params)                              → {"vendors": [...]}
//	vendor=huawei                            → {"cmdTypes": [...]}
//	vendor=huawei&command=display+interface  → {"cmdType":"interface","fields":[...]}
func (h *handlers) apiFields(w http.ResponseWriter, r *http.Request) {
	if h.fieldReg == nil {
		jsonError(w, "field registry not available", http.StatusServiceUnavailable)
		return
	}

	vendor := r.URL.Query().Get("vendor")
	command := r.URL.Query().Get("command")

	if vendor == "" {
		w.Header().Set("Content-Type", "application/json")
		vendors := h.fieldReg.Vendors()
		json.NewEncoder(w).Encode(map[string]any{"vendors": vendors})
		return
	}

	if command == "" {
		types := h.fieldReg.CmdTypes(vendor)
		if types == nil {
			jsonError(w, "unknown vendor", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		strs := make([]string, len(types))
		for i, t := range types {
			strs[i] = string(t)
		}
		json.NewEncoder(w).Encode(map[string]any{"cmdTypes": strs})
		return
	}

	cmdType := h.fieldReg.ClassifyCommand(vendor, command)
	if cmdType == model.CmdUnknown {
		jsonError(w, "unknown command", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fields := h.fieldReg.Fields(vendor, cmdType)
	json.NewEncoder(w).Encode(map[string]any{
		"cmdType": string(cmdType),
		"fields":  fields,
	})
}

// ── Inline HTML templates ────────────────────────────────────────────────────

const listHTML = `<!DOCTYPE html>
<html><head><title>Rule Studio</title>
<script src="/static/htmx.min.js"></script>
<style>body{font-family:monospace;max-width:1200px;margin:2rem auto;padding:0 1rem}
table{width:100%;border-collapse:collapse}th,td{text-align:left;padding:0.4rem 0.8rem;border-bottom:1px solid #ddd}
th{background:#f5f5f5}.badge{padding:2px 8px;border-radius:4px;font-size:0.85em}
.draft{background:#fff3cd}.testing{background:#cce5ff}.approved{background:#d4edda}
a{color:#0066cc}</style></head>
<body>
<h1>🔬 Rule Studio</h1>
<div style="display:flex;gap:1rem;align-items:center;margin-bottom:1rem">
  <button hx-post="/api/discover" hx-swap="none">🔄 Run Discovery</button>
  <a href="/fields">🔍 Browse Fields</a>
  <a href="/unknown">⚠️ Unknown ({{.UnknownCount}})</a>
  <a href="/test">🧪 Parser Tester</a>
</div>
<table>
<tr><th>Vendor</th><th>Command Pattern</th><th>Type</th><th>Confidence</th><th>Status</th><th>Created</th><th>Actions</th></tr>
{{range .Rules}}
<tr>
  <td>{{.Vendor}}</td><td><code>{{.CommandPattern}}</code></td><td>{{.OutputType}}</td>
  <td>{{printf "%.0f%%" (mul .Confidence 100.0)}}</td>
  <td><span class="badge {{.Status}}">{{.Status}}</span></td>
  <td>{{.CreatedAt.Format "2006-01-02"}}</td>
  <td><a href="/rule/{{.ID}}">Edit</a> · <a href="/rule/{{.ID}}/sandbox">Sandbox</a></td>
</tr>
{{else}}<tr><td colspan="7">No pending rules. Run discovery to populate.</td></tr>
{{end}}</table></body></html>`

const editorHTML = `<!DOCTYPE html>
<html><head><title>Rule Editor</title>
<script src="/static/htmx.min.js"></script>
<style>body{font-family:monospace;max-width:1200px;margin:2rem auto;padding:0 1rem}
.grid{display:grid;grid-template-columns:1fr 1fr;gap:1rem}textarea{width:100%;height:400px;font-family:monospace}</style>
</head><body>
<h1>✏️ Rule Editor — <code>{{.CommandPattern}}</code></h1>
<details style="margin-bottom:1rem;border:1px solid #ddd;padding:0.5rem;border-radius:4px">
<summary>ℹ️ Schema Guide</summary>
<div style="margin-top:0.5rem;font-size:0.9em">
<ul>
<li>Generated types auto-register as <code>generated:{vendor}:{cmd_stem}</code></li>
<li>Results stored in <code>ParseResult.Rows[]</code> → queryable via <code>nethelper show scratch</code></li>
<li>Table YAML: <code>columns</code> indexed by position; <code>derived</code> for computed fields</li>
<li>Go code: must return <code>model.ParseResult{Type: cmdType, Rows: [...]}</code></li>
<li><a href="/fields">🔍 Browse existing field schemas</a></li>
</ul>
</div>
</details>
<p>Vendor: {{.Vendor}} · Type: {{.OutputType}} · Confidence: {{printf "%.0f%%" (mul .Confidence 100.0)}}</p>
<form method="POST"><div class="grid">
  <div><h3>{{if eq .OutputType "table"}}Schema YAML{{else}}Go Code Draft{{end}}</h3>
    {{if eq .OutputType "table"}}
    <textarea name="schema_yaml">{{.SchemaYAML}}</textarea>
    {{else}}
    <textarea name="go_code_draft">{{.GoCodeDraft}}</textarea>
    <p><em>⚠️ Go code rules: live test unavailable. Validation after PR merge.</em></p>
    {{end}}
  </div>
  <div><h3>Sample Inputs</h3>
    <pre style="overflow:auto;max-height:400px;background:#f8f8f8;padding:1rem">{{.SampleInputs}}</pre>
  </div>
</div>
<button type="submit">💾 Save → Sandbox</button> <a href="/">← Back</a>
</form></body></html>`

const fieldsHTML = `<!DOCTYPE html>
<html>
<head>
<title>Field Browser — Rule Studio</title>
<script src="/static/htmx.min.js"></script>
<style>
  body{font-family:monospace;max-width:1200px;margin:2rem auto;padding:0 1rem}
  .layout{display:grid;grid-template-columns:200px 1fr;gap:1.5rem;margin-top:1rem}
  .vendor-list{border-right:1px solid #ddd;padding-right:1rem}
  .vendor-btn{display:block;width:100%;text-align:left;padding:0.4rem 0.6rem;
    margin:2px 0;border:1px solid #ddd;background:#f9f9f9;cursor:pointer;
    font-family:monospace;font-size:1em;border-radius:3px}
  .vendor-btn:hover,.vendor-btn.active{background:#0066cc;color:white;border-color:#0066cc}
  .cmdtype-row{display:flex;justify-content:space-between;align-items:center;
    padding:0.35rem 0.6rem;border-bottom:1px solid #eee;cursor:pointer}
  .cmdtype-row:hover{background:#f0f8ff}
  .cmdtype-row.active{background:#e8f4fd}
  .field-count{color:#888;font-size:0.9em}
  table{width:100%;border-collapse:collapse;margin-top:0.5rem}
  th,td{text-align:left;padding:0.4rem 0.8rem;border-bottom:1px solid #ddd}
  th{background:#f5f5f5}
  .tag{padding:1px 6px;border-radius:3px;font-size:0.85em;background:#e9ecef}
  .tag-derived{background:#fff3cd}
  #schema-panel{padding-left:1rem}
  h4{margin:0.5rem 0}
</style>
</head>
<body>
<h1>🔍 Field Browser</h1>
<p>Browse the field schema for each vendor's parsed commands. <a href="/">← Rule Studio</a></p>
<div class="layout">
  <div class="vendor-list">
    <h4>Vendors</h4>
    <div hx-get="/api/fields/vendors-html" hx-trigger="load" hx-swap="outerHTML">
      Loading...
    </div>
  </div>
  <div id="schema-panel">
    <p style="color:#888">← Select a vendor</p>
  </div>
</div>
</body>
</html>`

const sandboxHTML = `<!DOCTYPE html>
<html><head><title>Sandbox — {{.Rule.CommandPattern}}</title>
<script src="/static/htmx.min.js"></script>
<style>body{font-family:monospace;max-width:1200px;margin:2rem auto;padding:0 1rem}
textarea{width:100%;height:200px;font-family:monospace}
#result{background:#f0f8ff;padding:1rem;min-height:80px;white-space:pre-wrap}</style>
</head><body>
<h1>🧪 Sandbox — <code>{{.Rule.CommandPattern}}</code></h1>
<details style="margin-bottom:1rem;border:1px solid #ddd;padding:0.5rem;border-radius:4px">
<summary>ℹ️ Schema Guide</summary>
<div style="margin-top:0.5rem;font-size:0.9em">
<ul>
<li>Generated types auto-register as <code>generated:{vendor}:{cmd_stem}</code></li>
<li>Results stored in <code>ParseResult.Rows[]</code> → queryable via <code>nethelper show scratch</code></li>
<li>Table YAML: <code>columns</code> indexed by position; <code>derived</code> for computed fields</li>
<li>Go code: must return <code>model.ParseResult{Type: cmdType, Rows: [...]}</code></li>
<li><a href="/fields">🔍 Browse existing field schemas</a></li>
</ul>
</div>
</details>
<p>Vendor: {{.Rule.Vendor}} · Type: {{.Rule.OutputType}} · Test cases: {{.TestCount}}</p>

{{if eq .Rule.OutputType "table"}}
<h3>Paste device output:</h3>
<textarea id="input-area" name="input"></textarea><br>
<button hx-post="/api/rule/{{.Rule.ID}}/test"
        hx-include="#input-area" hx-target="#result">▶ Run Parse</button>
<div id="result"></div>
{{else}}
<p><em>⚠️ Go code rule — live execution unavailable. Review draft below, save test cases manually.</em></p>
<pre style="background:#f8f8f8;padding:1rem;overflow:auto">{{.Rule.GoCodeDraft}}</pre>
<textarea id="input-area" name="input" placeholder="Paste CLI output..."></textarea>
{{end}}

<h3>Save test case:</h3>
<input id="tc-desc" name="description" type="text" placeholder="Description (optional)" style="width:300px"><br>
<textarea id="tc-expected" name="expected" placeholder='Expected JSON result, e.g. {"rows":[...]}'></textarea>
<button hx-post="/api/rule/{{.Rule.ID}}/testcase"
        hx-include="#input-area,#tc-desc,#tc-expected"
        hx-target="#tc-status">💾 Save Test Case</button>
<span id="tc-status"></span>

{{if gt .TestCount 0}}
<br><br>
<form method="POST" action="/api/rule/{{.Rule.ID}}/approve">
  <button style="background:#28a745;color:white;padding:0.5rem 1rem;border:none;cursor:pointer">
    ✅ Approve &amp; Generate PR ({{.TestCount}} test case{{if gt .TestCount 1}}s{{end}})
  </button>
</form>
<br>
<button hx-post="/api/rule/{{.Rule.ID}}/save-local"
        hx-target="#save-result"
        hx-swap="innerHTML"
        style="background:#0066cc;color:white;padding:0.5rem 1rem;border:none;cursor:pointer">
  💾 Save to Local Files
</button>
<div id="save-result" style="margin-top:0.5rem;font-family:monospace;white-space:pre-wrap"></div>
{{else}}<p><em>Save at least one test case to enable approve.</em></p>{{end}}
<br><a href="/">← Back</a>
</body></html>`

const testPageHTML = `<!DOCTYPE html>
<html><head><title>Parser Tester — Rule Studio</title>
<script src="/static/htmx.min.js"></script>
<style>body{font-family:monospace;max-width:1100px;margin:2rem auto;padding:0 1rem}
textarea{width:100%;height:250px;font-family:monospace}
select,input{font-family:monospace;padding:0.3rem}
#result{background:#f0f8ff;padding:1rem;min-height:80px;white-space:pre-wrap;margin-top:1rem}
.matched{color:#28a745}.unmatched{color:#dc3545}</style></head>
<body>
<h1>🧪 Parser Tester</h1>
<p>Paste CLI output, pick vendor + command to see what the existing parsers extract. <a href="/">← Rule Studio</a></p>
<label>Vendor:
  <select name="vendor" id="vendor">
    {{range .}}<option value="{{.}}">{{.}}</option>{{end}}
  </select>
</label>
<br><br>
<label>Command: <input type="text" name="command" id="command" size="50" placeholder="e.g. display bgp peer"></label>
<br><br>
<label>CLI Output:</label>
<textarea name="output" id="output" placeholder="Paste device output here..."></textarea>
<br>
<button hx-post="/api/test"
        hx-include="#vendor,#command,#output"
        hx-target="#result"
        hx-swap="innerHTML">▶ Parse</button>
<div id="result"><em>Paste output and click ▶ Parse</em></div>
</body></html>`

const unknownListHTML = `<!DOCTYPE html>
<html><head><title>Unknown Outputs — Rule Studio</title>
<script src="/static/htmx.min.js"></script>
<style>body{font-family:monospace;max-width:1200px;margin:2rem auto;padding:0 1rem}
table{width:100%;border-collapse:collapse}th,td{text-align:left;padding:0.4rem 0.8rem;border-bottom:1px solid #ddd}
th{background:#f5f5f5}.badge{padding:2px 8px;border-radius:4px;font-size:0.85em;background:#fff3cd}
pre{margin:0;font-size:0.8em;max-height:60px;overflow:hidden}
button{cursor:pointer;padding:2px 8px;margin-right:4px}</style></head>
<body>
<h1>⚠️ Unknown Outputs</h1>
<p>Commands seen in ingested logs that no parser matched. <a href="/">← Rule Studio</a></p>
<table>
<tr><th>Vendor</th><th>Command</th><th>Count</th><th>First Seen</th><th>Preview</th><th>Actions</th></tr>
{{range .}}
<tr id="unknown-{{.ID}}">
  <td><span class="badge">{{.Vendor}}</span></td>
  <td><code>{{.CommandNorm}}</code></td>
  <td>{{.OccurrenceCount}}</td>
  <td>{{.FirstSeen.Format "2006-01-02"}}</td>
  <td><pre>{{.RawOutput}}</pre></td>
  <td>
    <button hx-post="/api/unknown/{{.ID}}/generate"
            hx-swap="none">🔬 Generate Rule</button>
    <button hx-post="/api/unknown/{{.ID}}/ignore"
            hx-target="#unknown-{{.ID}}"
            hx-swap="outerHTML">✕ Ignore</button>
  </td>
</tr>
{{else}}<tr><td colspan="6">No new unknown outputs.</td></tr>
{{end}}</table></body></html>`
