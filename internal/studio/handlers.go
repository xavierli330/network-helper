package studio

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

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
	tmpl := template.Must(template.New("list").Funcs(funcMap).Parse(listHTML))
	tmpl.Execute(w, rules)
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
<button hx-post="/api/discover" hx-swap="none">🔄 Run Discovery</button>
<table>
<tr><th>Vendor</th><th>Command Pattern</th><th>Type</th><th>Confidence</th><th>Status</th><th>Created</th><th>Actions</th></tr>
{{range .}}
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

const sandboxHTML = `<!DOCTYPE html>
<html><head><title>Sandbox — {{.Rule.CommandPattern}}</title>
<script src="/static/htmx.min.js"></script>
<style>body{font-family:monospace;max-width:1200px;margin:2rem auto;padding:0 1rem}
textarea{width:100%;height:200px;font-family:monospace}
#result{background:#f0f8ff;padding:1rem;min-height:80px;white-space:pre-wrap}</style>
</head><body>
<h1>🧪 Sandbox — <code>{{.Rule.CommandPattern}}</code></h1>
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
{{else}}<p><em>Save at least one test case to enable approve.</em></p>{{end}}
<br><a href="/">← Back</a>
</body></html>`
