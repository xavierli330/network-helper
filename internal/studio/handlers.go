package studio

import (
	"context"
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
	"mul":      func(a, b float64) float64 { return a * b },
	"truncate": func(s string, n int) string { if len(s) <= n { return s }; return s[:n-1] + "…" },
	"confClass": func(c float64) string {
		if c >= 0.8 { return "high" }
		if c >= 0.5 { return "mid" }
		return "low"
	},
	"confPct": func(c float64) string { return fmt.Sprintf("%.0f", c*100) },
	"statusClass": func(s string) string {
		switch s {
		case "draft":    return "badge-draft"
		case "testing":  return "badge-testing"
		case "approved": return "badge-approved"
		case "rejected": return "badge-rejected"
		}
		return ""
	},
	"outputTypeClass": func(s string) string {
		switch s {
		case "pipeline":     return "tag-pipeline"
		case "table":        return "tag-table"
		case "raw":          return "tag-raw"
		case "hierarchical": return "tag-hierarchical"
		}
		return ""
	},
}

// ── Helper: common page data ─────────────────────────────────────────────

type pageData struct {
	ActivePage   string
	UnknownCount int
	LLMAvailable bool
	EngAvailable bool
	GitAvailable bool
}

func (h *handlers) newPageData(active string) pageData {
	unknowns, _ := h.db.ListUnknownOutputs("", "new", 1000)
	_, gitErr := exec.LookPath("gh")
	return pageData{
		ActivePage:   active,
		UnknownCount: len(unknowns),
		LLMAvailable: h.eng != nil,
		EngAvailable: h.eng != nil,
		GitAvailable: gitErr == nil,
	}
}

// ── Page: Dashboard ──────────────────────────────────────────────────────

func (h *handlers) dashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	pd := h.newPageData("dashboard")

	// Gather dashboard data
	unknowns, _ := h.db.ListUnknownOutputs("", "new", 1000)
	drafts, _ := h.db.ListPendingRules("draft", 100)
	testing_, _ := h.db.ListPendingRules("testing", 100)
	approved, _ := h.db.ListPendingRules("approved", 100)

	// Compute distinct unknown command count per vendor
	type vendorUnknown struct {
		Vendor string
		Count  int
	}
	vendorMap := map[string]int{}
	for _, u := range unknowns {
		vendorMap[u.Vendor]++
	}
	var vendorUnknowns []vendorUnknown
	for v, c := range vendorMap {
		vendorUnknowns = append(vendorUnknowns, vendorUnknown{v, c})
	}

	// Compute supported command count per vendor from FieldRegistry
	type vendorCoverage struct {
		Vendor         string
		SupportedTypes int
	}
	var vendorCoverages []vendorCoverage
	if h.fieldReg != nil {
		for _, v := range h.fieldReg.Vendors() {
			types := h.fieldReg.CmdTypes(v)
			vendorCoverages = append(vendorCoverages, vendorCoverage{v, len(types)})
		}
	}

	// Build "needs attention" items
	type attentionItem struct {
		Icon    string
		Title   string
		Detail  string
		Link    string
		Urgency string // high, medium, low
	}
	var attention []attentionItem

	if len(unknowns) > 0 {
		attention = append(attention, attentionItem{
			Icon: "⚠️", Title: fmt.Sprintf("%d unknown commands need classification", len(unknowns)),
			Detail: "Ingested commands that no parser matched. Generate rules or ignore.",
			Link: "/unknown", Urgency: "high",
		})
	}
	if len(drafts) > 0 {
		attention = append(attention, attentionItem{
			Icon: "📝", Title: fmt.Sprintf("%d draft rules need review", len(drafts)),
			Detail: "LLM-generated rule drafts awaiting your review and testing.",
			Link: "/rules", Urgency: "high",
		})
	}
	if len(testing_) > 0 {
		attention = append(attention, attentionItem{
			Icon: "🧪", Title: fmt.Sprintf("%d rules in testing", len(testing_)),
			Detail: "Rules being tested. Add test cases and approve when ready.",
			Link: "/rules", Urgency: "medium",
		})
	}
	if len(approved) > 0 {
		attention = append(attention, attentionItem{
			Icon: "✅", Title: fmt.Sprintf("%d rules approved", len(approved)),
			Detail: "Approved rules in history.",
			Link: "/history", Urgency: "low",
		})
	}
	if len(attention) == 0 {
		attention = append(attention, attentionItem{
			Icon: "✨", Title: "All clear — no action needed",
			Detail: "All commands are classified. Import more logs to expand coverage.",
			Link: "", Urgency: "low",
		})
	}

	tmpl := template.Must(template.New("dashboard").Funcs(funcMap).Parse(baseHTML + dashboardHTML))
	tmpl.Execute(w, struct {
		pageData
		UnknownCommands  int
		DraftCount       int
		TestingCount     int
		ApprovedCount    int
		VendorUnknowns   []vendorUnknown
		VendorCoverages  []vendorCoverage
		Attention        []attentionItem
	}{
		pageData:        pd,
		UnknownCommands: len(unknowns),
		DraftCount:      len(drafts),
		TestingCount:    len(testing_),
		ApprovedCount:   len(approved),
		VendorUnknowns:  vendorUnknowns,
		VendorCoverages: vendorCoverages,
		Attention:       attention,
	})
}

func (h *handlers) apiDashboard(w http.ResponseWriter, r *http.Request) {
	unknowns, _ := h.db.ListUnknownOutputs("", "new", 1000)
	drafts, _ := h.db.ListPendingRules("draft", 100)
	testing_, _ := h.db.ListPendingRules("testing", 100)
	approved, _ := h.db.ListPendingRules("approved", 100)

	vendorMap := map[string]int{}
	for _, u := range unknowns {
		vendorMap[u.Vendor]++
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"unknownCount":    len(unknowns),
		"draftCount":      len(drafts),
		"testingCount":    len(testing_),
		"approvedCount":   len(approved),
		"vendorUnknowns":  vendorMap,
		"needsAttention":  len(unknowns) + len(drafts) + len(testing_),
	})
}

// ── Page: Rules List ─────────────────────────────────────────────────────

func (h *handlers) list(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	vendor := r.URL.Query().Get("vendor")
	status := r.URL.Query().Get("status")

	var rules []store.PendingRule
	var err error

	if query != "" || vendor != "" || status != "" {
		// Search mode
		rules, err = h.db.SearchPendingRules(query, vendor, status, 100)
	} else {
		// Default: show all rules (all statuses)
		rules, err = h.db.SearchPendingRules("", "", "", 100)
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// Collect distinct vendors for filter dropdown
	vendorSet := map[string]bool{}
	for _, r := range rules {
		vendorSet[r.Vendor] = true
	}
	var vendors []string
	for v := range vendorSet {
		vendors = append(vendors, v)
	}

	// Counts per status
	drafts, _ := h.db.ListPendingRules("draft", 100)
	testing_, _ := h.db.ListPendingRules("testing", 100)
	approved, _ := h.db.ListPendingRules("approved", 100)
	rejected, _ := h.db.ListPendingRules("rejected", 100)

	pd := h.newPageData("rules")
	tmpl := template.Must(template.New("list").Funcs(funcMap).Parse(baseHTML + listHTML))
	tmpl.Execute(w, struct {
		pageData
		Rules         []store.PendingRule
		Vendors       []string
		Query         string
		FilterVendor  string
		FilterStatus  string
		TotalCount    int
		DraftCount    int
		TestingCount  int
		ApprovedCount int
		RejectedCount int
	}{
		pageData:      pd,
		Rules:         rules,
		Vendors:       vendors,
		Query:         query,
		FilterVendor:  vendor,
		FilterStatus:  status,
		TotalCount:    len(drafts) + len(testing_) + len(approved) + len(rejected),
		DraftCount:    len(drafts),
		TestingCount:  len(testing_),
		ApprovedCount: len(approved),
		RejectedCount: len(rejected),
	})
}

// apiSearchRules returns JSON for AJAX search
func (h *handlers) apiSearchRules(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	vendor := r.URL.Query().Get("vendor")
	status := r.URL.Query().Get("status")
	rules, err := h.db.SearchPendingRules(query, vendor, status, 100)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rules)
}

// ── Page: Rule Editor + Sandbox (merged) ─────────────────────────────────

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
	// POST = save schema/code
	if r.Method == "POST" {
		rule, err := h.db.GetPendingRule(id)
		if err != nil {
			http.Error(w, "rule not found", 404)
			return
		}
		r.ParseForm()
		rule.SchemaYAML = r.FormValue("schema_yaml")
		rule.GoCodeDraft = r.FormValue("go_code_draft")
		rule.Status = "testing"
		h.db.UpdatePendingRule(rule)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}
	h.ruleEditor(w, r, id)
}

func (h *handlers) ruleEditor(w http.ResponseWriter, r *http.Request, id int) {
	rule, err := h.db.GetPendingRule(id)
	if err != nil {
		http.Error(w, "rule not found", 404)
		return
	}
	count, _ := h.db.CountRuleTestCases(id)
	testCases, _ := h.db.ListRuleTestCases(id)
	pd := h.newPageData("rules")
	tmpl := template.Must(template.New("editor").Funcs(funcMap).Parse(baseHTML + editorHTML))
	tmpl.Execute(w, struct {
		pageData
		Rule      store.PendingRule
		TestCount int
		TestCases []store.RuleTestCase
	}{pageData: pd, Rule: rule, TestCount: count, TestCases: testCases})
}

// ── Page: Parser Tester ──────────────────────────────────────────────────

func (h *handlers) tester(w http.ResponseWriter, r *http.Request) {
	var vendors []string
	if h.fieldReg != nil {
		vendors = h.fieldReg.Vendors()
	}
	pd := h.newPageData("tester")
	tmpl := template.Must(template.New("tester").Funcs(funcMap).Parse(baseHTML + testPageHTML))
	tmpl.Execute(w, struct {
		pageData
		Vendors []string
	}{pageData: pd, Vendors: vendors})
}

// ── Page: Field Browser ──────────────────────────────────────────────────

func (h *handlers) fields(w http.ResponseWriter, r *http.Request) {
	pd := h.newPageData("fields")
	tmpl := template.Must(template.New("fields").Funcs(funcMap).Parse(baseHTML + fieldsHTML))
	tmpl.Execute(w, struct{ pageData }{pageData: pd})
}

// ── Page: Unknown Outputs ────────────────────────────────────────────────

func (h *handlers) unknownList(w http.ResponseWriter, r *http.Request) {
	outputs, err := h.db.ListUnknownOutputs("", "new", 500)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	pd := h.newPageData("unknown")
	tmpl := template.Must(template.New("unknown").Funcs(funcMap).Parse(baseHTML + unknownListHTML))
	tmpl.Execute(w, struct {
		pageData
		Outputs []store.UnknownOutput
	}{pageData: pd, Outputs: outputs})
}

// ── Page: History ────────────────────────────────────────────────────────

func (h *handlers) history(w http.ResponseWriter, r *http.Request) {
	approved, _ := h.db.ListPendingRules("approved", 100)
	rejected, _ := h.db.ListPendingRules("rejected", 100)
	rules := append(approved, rejected...)
	pd := h.newPageData("history")
	tmpl := template.Must(template.New("history").Funcs(funcMap).Parse(baseHTML + historyHTML))
	tmpl.Execute(w, struct {
		pageData
		Rules []store.PendingRule
	}{pageData: pd, Rules: rules})
}

// ── Page: Command Patterns ───────────────────────────────────────────────

func (h *handlers) patterns(w http.ResponseWriter, r *http.Request) {
	pd := h.newPageData("patterns")

	type prefixRule struct {
		Prefix  string
		CmdType string
	}
	type vendorPatterns struct {
		Vendor  string
		Rules   []prefixRule
	}

	// Build the classification prefix rules from each vendor's ClassifyCommand
	// We statically define the known prefixes since they're hardcoded in Go
	allPatterns := []vendorPatterns{
		{Vendor: "huawei", Rules: []prefixRule{
			{"display ip routing-table", "rib"}, {"display ip rout", "rib"},
			{"display fib", "fib"},
			{"display mpls lsp", "lfib"}, {"display mpls forwarding", "lfib"},
			{"display interface", "interface"}, {"display int", "interface"},
			{"display ospf peer", "neighbor"}, {"display bgp peer", "neighbor"},
			{"display isis peer", "neighbor"}, {"display mpls ldp session", "neighbor"},
			{"display mpls ldp peer", "neighbor"}, {"display rsvp session", "neighbor"},
			{"display lldp neighbor", "neighbor"},
			{"display mpls te tunnel", "tunnel"},
			{"display segment-routing", "sr_mapping"}, {"display isis segment-routing", "sr_mapping"},
			{"display current-configuration", "config"}, {"display saved-configuration", "config"},
			{"display cur", "config"}, {"display sa", "config"},
		}},
		{Vendor: "cisco", Rules: []prefixRule{
			{"show ip route", "rib"}, {"show route", "rib"},
			{"show ip cef", "fib"},
			{"show mpls forwarding", "lfib"},
			{"show interface", "interface"}, {"show ip interface", "interface"},
			{"show ip ospf neighbor", "neighbor"}, {"show ip bgp summary", "neighbor"},
			{"show bgp summary", "neighbor"}, {"show isis neighbor", "neighbor"},
			{"show mpls ldp neighbor", "neighbor"}, {"show lldp neighbor", "neighbor"},
			{"show mpls traffic-eng tunnel", "tunnel"},
			{"show running-config", "config"}, {"show startup-config", "config"},
		}},
		{Vendor: "h3c", Rules: []prefixRule{
			{"display ip routing-table", "rib"},
			{"display fib", "fib"},
			{"display mpls lsp", "lfib"}, {"display mpls forwarding", "lfib"},
			{"display ip interface", "interface"}, {"display interface", "interface"},
			{"display ospf peer", "neighbor"}, {"display bgp peer", "neighbor"},
			{"display isis peer", "neighbor"}, {"display mpls ldp session", "neighbor"},
			{"display current-configuration", "config"},
		}},
		{Vendor: "juniper", Rules: []prefixRule{
			{"show route", "rib"},
			{"show interface", "interface"},
			{"show ospf neighbor", "neighbor"}, {"show bgp summary", "neighbor"},
			{"show isis adjacency", "neighbor"}, {"show ldp session", "neighbor"},
			{"show rsvp session", "tunnel"},
			{"show route table mpls", "lfib"},
			{"show configuration", "config"},
			{"| display set", "config_set"},
		}},
	}

	// Add unknown command samples from DB
	unknowns, _ := h.db.ListUnknownOutputs("", "new", 100)

	tmpl := template.Must(template.New("patterns").Funcs(funcMap).Parse(baseHTML + patternsHTML))
	tmpl.Execute(w, struct {
		pageData
		Patterns []vendorPatterns
		Unknowns []store.UnknownOutput
	}{pageData: pd, Patterns: allPatterns, Unknowns: unknowns})
}

// ── Page: Cross-Vendor Field Compare ─────────────────────────────────────

func (h *handlers) compare(w http.ResponseWriter, r *http.Request) {
	pd := h.newPageData("compare")

	type cmdTypeFields struct {
		Vendor string
		Fields []string
	}
	type compareRow struct {
		CmdType string
		Vendors []cmdTypeFields
	}

	var rows []compareRow
	if h.fieldReg != nil {
		// Collect all unique CmdTypes across all vendors
		typeSet := map[string]bool{}
		vendors := h.fieldReg.Vendors()
		for _, v := range vendors {
			for _, ct := range h.fieldReg.CmdTypes(v) {
				typeSet[string(ct)] = true
			}
		}
		for ct := range typeSet {
			row := compareRow{CmdType: ct}
			for _, v := range vendors {
				defs := h.fieldReg.Fields(v, model.CommandType(ct))
				var names []string
				for _, d := range defs {
					names = append(names, d.Name)
				}
				row.Vendors = append(row.Vendors, cmdTypeFields{Vendor: v, Fields: names})
			}
			rows = append(rows, row)
		}
	}

	var vendors []string
	if h.fieldReg != nil {
		vendors = h.fieldReg.Vendors()
	}

	tmpl := template.Must(template.New("compare").Funcs(funcMap).Parse(baseHTML + compareHTML))
	tmpl.Execute(w, struct {
		pageData
		Rows    []compareRow
		Vendors []string
	}{pageData: pd, Rows: rows, Vendors: vendors})
}

// ── API: Status ──────────────────────────────────────────────────────────

func (h *handlers) apiStatus(w http.ResponseWriter, r *http.Request) {
	unknowns, _ := h.db.ListUnknownOutputs("", "new", 1000)
	drafts, _ := h.db.ListPendingRules("draft", 100)
	testing_, _ := h.db.ListPendingRules("testing", 100)
	approved, _ := h.db.ListPendingRules("approved", 100)
	_, gitErr := exec.LookPath("gh")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"unknownCount":  len(unknowns),
		"draftCount":    len(drafts),
		"testingCount":  len(testing_),
		"approvedCount": len(approved),
		"llmAvailable":  h.eng != nil,
		"gitAvailable":  gitErr == nil,
	})
}

// ── API: Test / TestCase / Approve / Ignore / SaveLocal ──────────────────

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
	case "delete-testcase":
		h.apiDeleteTestCase(w, r, id, parts)
	case "get-testcase":
		h.apiGetTestCase(w, r, id, parts)
	case "run-all-tests":
		h.apiRunAllTests(w, r, id)
	case "improve":
		h.apiImprove(w, r, id)
	case "approve":
		h.apiApprove(w, r, id)
	case "ignore":
		h.apiIgnore(w, r, id)
	case "save-local":
		h.apiSaveLocal(w, r, id)
	case "delete":
		h.apiDeleteRule(w, r, id)
	case "regenerate":
		h.apiRegenerateRule(w, r, id)
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
	r.ParseForm()
	input := r.FormValue("input")

	switch rule.OutputType {
	case "pipeline":
		// Pipeline DSL: the DSL text is stored in SchemaYAML field
		result, err := engine.ExecPipeline(rule.SchemaYAML, input)
		if err != nil {
			jsonError(w, "pipeline error: "+err.Error(), 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)

	case "table":
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

	default:
		jsonError(w, fmt.Sprintf("live test not supported for %q type rules", rule.OutputType), 400)
	}
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

func (h *handlers) apiDeleteTestCase(w http.ResponseWriter, r *http.Request, ruleID int, parts []string) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}
	tcID, err := strconv.Atoi(parts[2])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.db.DeleteRuleTestCase(tcID); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

func (h *handlers) apiGetTestCase(w http.ResponseWriter, r *http.Request, ruleID int, parts []string) {
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}
	tcID, err := strconv.Atoi(parts[2])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	testCases, err := h.db.ListRuleTestCases(ruleID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	for _, tc := range testCases {
		if tc.ID == tcID {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tc)
			return
		}
	}
	jsonError(w, "test case not found", 404)
}

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
	goFilePath := codegen.TargetFilePath(rule.Vendor, rule.CommandPattern)
	h.db.ApprovePendingRule(id, approvedBy, time.Now())
	h.db.SetPendingRulePR(id, prURL, goFilePath)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "approved", "pr_url": prURL})
}

func (h *handlers) apiIgnore(w http.ResponseWriter, r *http.Request, id int) {
	rule, err := h.db.GetPendingRule(id)
	if err != nil {
		jsonError(w, "rule not found", 404)
		return
	}
	rule.Status = "rejected"
	h.db.UpdatePendingRule(rule)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "rejected"})
}

// ── API: Run All Tests ───────────────────────────────────────────────────

// TestRunResult is a single test case execution result.
type TestRunResult struct {
	TCID     int    `json:"tc_id"`
	Passed   bool   `json:"passed"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Error    string `json:"error,omitempty"`
	Diff     *DiffResult `json:"diff,omitempty"`
}

// DiffResult holds the structured diff between expected and actual.
type DiffResult struct {
	Match         bool        `json:"match"`
	RowCountMatch bool        `json:"row_count_match"`
	ExpectedRows  int         `json:"expected_rows"`
	ActualRows    int         `json:"actual_rows"`
	FieldDiffs    []FieldDiff `json:"field_diffs,omitempty"`
	MissingFields []string    `json:"missing_fields,omitempty"`
	ExtraFields   []string    `json:"extra_fields,omitempty"`
}

// FieldDiff describes a single field mismatch.
type FieldDiff struct {
	Row      int    `json:"row"`
	Field    string `json:"field"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

func (h *handlers) apiRunAllTests(w http.ResponseWriter, r *http.Request, id int) {
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
	if err != nil {
		testCases = nil
	}

	// If no test cases, run against sample inputs as a preview (no pass/fail comparison).
	if len(testCases) == 0 {
		samples := parseSampleInputs(rule.SampleInputs)
		if len(samples) == 0 {
			jsonError(w, "no test cases and no sample inputs — paste a sample and save a test case first", 400)
			return
		}
		// Run DSL against first sample to show what it produces
		var previewResult any
		switch rule.OutputType {
		case "pipeline":
			pResult, execErr := engine.ExecPipeline(rule.SchemaYAML, samples[0])
			if execErr != nil {
				jsonError(w, "DSL execution error on sample: "+execErr.Error(), 400)
				return
			}
			previewResult = pResult
		default:
			jsonError(w, "no test cases found and preview not supported for "+rule.OutputType, 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"all_passed":     false,
			"no_test_cases":  true,
			"sample_preview": previewResult,
			"message":        "No test cases found. Showing current DSL output on sample input — save a test case to enable pass/fail testing.",
			"results":        []TestRunResult{},
			"total":          0,
		})
		return
	}

	results := make([]TestRunResult, len(testCases))
	allPassed := true

	for i, tc := range testCases {
		result := TestRunResult{TCID: tc.ID}

		// Execute parse
		var actualJSON string
		switch rule.OutputType {
		case "pipeline":
			pResult, execErr := engine.ExecPipeline(rule.SchemaYAML, tc.Input)
			if execErr != nil {
				result.Error = execErr.Error()
				result.Passed = false
				allPassed = false
				results[i] = result
				continue
			}
			actualBytes, _ := json.Marshal(pResult)
			actualJSON = string(actualBytes)
		default:
			result.Error = fmt.Sprintf("run-all-tests not yet supported for %q type", rule.OutputType)
			result.Passed = false
			allPassed = false
			results[i] = result
			continue
		}

		// Deep compare expected vs actual
		diff := deepCompare(tc.Expected, actualJSON)
		result.Diff = &diff
		result.Passed = diff.Match
		result.Expected = tc.Expected
		result.Actual = actualJSON
		if !diff.Match {
			allPassed = false
		}
		results[i] = result
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"all_passed": allPassed,
		"results":    results,
		"total":      len(results),
	})
}

// deepCompare performs a structural comparison between expected and actual JSON.
func deepCompare(expectedJSON, actualJSON string) DiffResult {
	result := DiffResult{Match: true}

	// Parse both as generic structures
	var expected, actual map[string]any
	if err := json.Unmarshal([]byte(expectedJSON), &expected); err != nil {
		// Try parsing as array
		var expectedArr, actualArr []map[string]string
		if err2 := json.Unmarshal([]byte(expectedJSON), &expectedArr); err2 != nil {
			result.Match = false
			result.FieldDiffs = append(result.FieldDiffs, FieldDiff{
				Row: 0, Field: "_parse_error", Expected: "valid JSON", Actual: err.Error(),
			})
			return result
		}
		if err2 := json.Unmarshal([]byte(actualJSON), &actualArr); err2 != nil {
			result.Match = false
			return result
		}
		return compareRowArrays(expectedArr, actualArr)
	}

	if err := json.Unmarshal([]byte(actualJSON), &actual); err != nil {
		result.Match = false
		return result
	}

	// Compare rows arrays
	expectedRows := extractRows(expected)
	actualRows := extractRows(actual)

	result.ExpectedRows = len(expectedRows)
	result.ActualRows = len(actualRows)
	result.RowCountMatch = len(expectedRows) == len(actualRows)

	if !result.RowCountMatch {
		result.Match = false
	}

	return compareRowArrays(expectedRows, actualRows)
}

func extractRows(data map[string]any) []map[string]string {
	// Try "rows" key (pipeline result format)
	rawRows, ok := data["rows"]
	if !ok {
		// Try "Rows" key (table result format)
		rawRows, ok = data["Rows"]
		if !ok {
			return nil
		}
	}

	rowsArr, ok := rawRows.([]any)
	if !ok {
		return nil
	}

	var result []map[string]string
	for _, rawRow := range rowsArr {
		rowMap, ok := rawRow.(map[string]any)
		if !ok {
			continue
		}
		strRow := make(map[string]string)
		for k, v := range rowMap {
			switch tv := v.(type) {
			case string:
				strRow[k] = tv
			default:
				strRow[k] = fmt.Sprintf("%v", v)
			}
		}
		result = append(result, strRow)
	}
	return result
}

func compareRowArrays(expected, actual []map[string]string) DiffResult {
	result := DiffResult{
		Match:         true,
		ExpectedRows:  len(expected),
		ActualRows:    len(actual),
		RowCountMatch: len(expected) == len(actual),
	}

	if !result.RowCountMatch {
		result.Match = false
	}

	// Compare up to min(len) rows
	minRows := len(expected)
	if len(actual) < minRows {
		minRows = len(actual)
	}

	for i := 0; i < minRows; i++ {
		expRow := expected[i]
		actRow := actual[i]

		// Find missing/extra fields in first row only
		if i == 0 {
			for k := range expRow {
				if _, ok := actRow[k]; !ok {
					result.MissingFields = append(result.MissingFields, k)
					result.Match = false
				}
			}
			for k := range actRow {
				if _, ok := expRow[k]; !ok {
					result.ExtraFields = append(result.ExtraFields, k)
				}
			}
		}

		// Compare field values
		for k, expVal := range expRow {
			actVal, ok := actRow[k]
			if !ok {
				continue // missing field already recorded
			}
			// Normalize: trim whitespace, case-insensitive for common values
			expNorm := strings.TrimSpace(expVal)
			actNorm := strings.TrimSpace(actVal)
			if expNorm != actNorm {
				result.Match = false
				result.FieldDiffs = append(result.FieldDiffs, FieldDiff{
					Row: i, Field: k, Expected: expVal, Actual: actVal,
				})
			}
		}
	}

	return result
}

// ── API: LLM Improve ─────────────────────────────────────────────────────

func (h *handlers) apiImprove(w http.ResponseWriter, r *http.Request, id int) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	if h.eng == nil {
		jsonError(w, "discovery engine not configured", 503)
		return
	}
	rule, err := h.db.GetPendingRule(id)
	if err != nil {
		jsonError(w, "rule not found", 404)
		return
	}
	testCases, err := h.db.ListRuleTestCases(id)
	if err != nil {
		testCases = nil // treat DB error as empty
	}

	// If no test cases exist, synthesize from SampleInputs.
	// Run the current DSL against sample inputs and treat every result as a
	// "needs improvement" failure, so the LLM can see what the DSL currently
	// produces and improve it.
	if len(testCases) == 0 {
		samples := parseSampleInputs(rule.SampleInputs)
		if len(samples) == 0 {
			jsonError(w, "no test cases and no sample inputs available — paste a sample and save a test case first", 400)
			return
		}
		var failures []discovery.FailedTestCase
		for i, sample := range samples {
			if i >= 3 {
				break // limit to 3 samples to keep LLM context manageable
			}
			var actualJSON string
			switch rule.OutputType {
			case "pipeline":
				pResult, execErr := engine.ExecPipeline(rule.SchemaYAML, sample)
				if execErr != nil {
					failures = append(failures, discovery.FailedTestCase{
						Input: sample, Expected: "(not available)", Actual: "",
						Error: "DSL execution error: " + execErr.Error(),
					})
					continue
				}
				b, _ := json.Marshal(pResult)
				actualJSON = string(b)
			default:
				continue
			}
			// We don't have an expected value, so tell the LLM the current result
			// and ask it to evaluate & improve based on the raw input.
			failures = append(failures, discovery.FailedTestCase{
				Input:    sample,
				Expected: "(no expected value — please infer from the raw input what the correct extraction should be)",
				Actual:   actualJSON,
				Error:    "no test case exists; the current extraction may be incomplete or incorrect",
			})
		}
		if len(failures) == 0 {
			jsonError(w, "could not run DSL against samples", 400)
			return
		}
		improved, improveErr := h.eng.ImproveDSL(r.Context(), rule, failures)
		if improveErr != nil {
			jsonError(w, "LLM improve failed: "+improveErr.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":                      "improved",
			"improved_dsl":                improved.SchemaYAML,
			"output_type":                 improved.OutputType,
			"expected_output_description": improved.ExpectedOutputDescription,
		})
		return
	}

	// Normal path: we have test cases — collect failures.
	var failures []discovery.FailedTestCase
	for _, tc := range testCases {
		var actualJSON string
		var execErr error
		switch rule.OutputType {
		case "pipeline":
			pResult, err := engine.ExecPipeline(rule.SchemaYAML, tc.Input)
			if err != nil {
				execErr = err
			} else {
				b, _ := json.Marshal(pResult)
				actualJSON = string(b)
			}
		default:
			continue
		}

		if execErr != nil {
			failures = append(failures, discovery.FailedTestCase{
				Input:    tc.Input,
				Expected: tc.Expected,
				Actual:   "",
				Error:    execErr.Error(),
			})
			continue
		}

		diff := deepCompare(tc.Expected, actualJSON)
		if !diff.Match {
			failures = append(failures, discovery.FailedTestCase{
				Input:    tc.Input,
				Expected: tc.Expected,
				Actual:   actualJSON,
				Error:    fmt.Sprintf("diff: %d field mismatches, row count %d vs %d", len(diff.FieldDiffs), diff.ExpectedRows, diff.ActualRows),
			})
		}
	}

	if len(failures) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "all_passed"})
		return
	}

	improved, err := h.eng.ImproveDSL(r.Context(), rule, failures)
	if err != nil {
		jsonError(w, "LLM improve failed: "+err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":                      "improved",
		"improved_dsl":                improved.SchemaYAML,
		"output_type":                 improved.OutputType,
		"expected_output_description": improved.ExpectedOutputDescription,
	})
}

// parseSampleInputs extracts sample strings from the JSON array stored in SampleInputs.
func parseSampleInputs(raw string) []string {
	if raw == "" || raw == "[]" {
		return nil
	}
	var samples []string
	if err := json.Unmarshal([]byte(raw), &samples); err != nil {
		return nil
	}
	// Filter out empty samples
	var valid []string
	for _, s := range samples {
		if strings.TrimSpace(s) != "" {
			valid = append(valid, s)
		}
	}
	return valid
}

func (h *handlers) apiDeleteRule(w http.ResponseWriter, r *http.Request, id int) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	if err := h.db.DeletePendingRule(id); err != nil {
		jsonError(w, "delete failed: "+err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

func (h *handlers) apiRegenerateRule(w http.ResponseWriter, r *http.Request, id int) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	if h.eng == nil {
		jsonError(w, "discovery engine not configured", 503)
		return
	}
	rule, err := h.db.GetPendingRule(id)
	if err != nil {
		jsonError(w, "rule not found", 404)
		return
	}
	// Delete old rule
	h.db.DeletePendingRule(id)
	// Reset the associated unknown outputs so they can be re-discovered
	h.db.UpdateUnknownOutputStatus(rule.Vendor, rule.CommandPattern, "new")
	// Find the unknown output for this command and re-generate
	unknowns, _ := h.db.ListUnknownOutputs(rule.Vendor, "new", 100)
	var unknownID int
	for _, u := range unknowns {
		if u.CommandNorm == rule.CommandPattern {
			unknownID = u.ID
			break
		}
	}
	if unknownID == 0 {
		jsonError(w, "no unknown output found for re-generation — the command may have been ingested with different output", 400)
		return
	}
	newRuleID, err := h.eng.GenerateForUnknown(r.Context(), unknownID)
	if err != nil {
		jsonError(w, "regeneration failed: "+err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "regenerated", "rule_id": newRuleID, "redirect": fmt.Sprintf("/rule/%d", newRuleID)})
}

func (h *handlers) apiDeleteDraftRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	n, err := h.db.DeletePendingRulesByStatus("draft")
	if err != nil {
		jsonError(w, "delete failed: "+err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "deleted", "count": n})
}

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
		json.NewEncoder(w).Encode(saveResult{Success: false, Paths: paths, BuildOutput: buildOut, Error: buildErr.Error()})
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
	json.NewEncoder(w).Encode(saveResult{Success: true, Paths: paths, BuildOutput: buildOut, Message: "Parser active. Run `go test ./internal/parser/...` to verify."})
}

func discoverRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ── Parser Tester API ────────────────────────────────────────────────────

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
		"cmdType": string(cmdType), "matched": matched,
		"interfaceCount": len(result.Interfaces), "neighborCount": len(result.Neighbors), "rowCount": len(result.Rows),
	}
	if len(result.Interfaces) > 0 {
		n := 5; if len(result.Interfaces) < n { n = len(result.Interfaces) }
		preview["interfaces"] = result.Interfaces[:n]
	}
	if len(result.Neighbors) > 0 {
		n := 5; if len(result.Neighbors) < n { n = len(result.Neighbors) }
		preview["neighbors"] = result.Neighbors[:n]
	}
	if len(result.Rows) > 0 {
		n := 5; if len(result.Rows) < n { n = len(result.Rows) }
		preview["rows"] = result.Rows[:n]
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(preview)
}

// ── Discovery API ────────────────────────────────────────────────────────

func (h *handlers) apiDiscover(w http.ResponseWriter, r *http.Request) {
	if h.eng == nil {
		jsonError(w, "discovery engine not configured", 503)
		return
	}

	// SSE: Server-Sent Events for real-time progress
	flusher, ok := w.(http.Flusher)
	if !ok {
		jsonError(w, "streaming not supported", 500)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // nginx

	vendor := r.URL.Query().Get("vendor")

	// Use a background context so closing the browser tab doesn't cancel LLM calls.
	// But still respect a reasonable overall timeout (10 min for a full discovery run).
	bgCtx, bgCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer bgCancel()

	// If the client disconnects, cancel the background context.
	clientGone := r.Context().Done()
	go func() {
		select {
		case <-clientGone:
			bgCancel()
		case <-bgCtx.Done():
		}
	}()

	ch := h.eng.RunStream(bgCtx, vendor)
	for ev := range ch {
		data, _ := json.Marshal(ev)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
}

// ── Unknown Outputs API ──────────────────────────────────────────────────

func (h *handlers) unknownDispatch(w http.ResponseWriter, r *http.Request) {
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

	// SSE mode: stream progress events
	flusher, ok := w.(http.Flusher)
	if !ok {
		// Fallback: non-streaming mode
		ruleID, err := h.eng.GenerateForUnknown(r.Context(), id)
		if err != nil {
			jsonError(w, "generate failed: "+err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"status": "ok", "rule_id": ruleID, "redirect": fmt.Sprintf("/rule/%d", ruleID)})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	bgCtx, bgCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer bgCancel()
	clientGone := r.Context().Done()
	go func() {
		select {
		case <-clientGone:
			bgCancel()
		case <-bgCtx.Done():
		}
	}()

	ch := h.eng.GenerateForUnknownStream(bgCtx, id)
	for ev := range ch {
		data, _ := json.Marshal(ev)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
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
	fmt.Fprintf(w, `<tr id="unknown-%d" style="display:none"></tr>`, id)
}

// ── Fields API ───────────────────────────────────────────────────────────

func (h *handlers) apiFields(w http.ResponseWriter, r *http.Request) {
	if h.fieldReg == nil {
		jsonError(w, "field registry not available", http.StatusServiceUnavailable)
		return
	}
	vendor := r.URL.Query().Get("vendor")
	command := r.URL.Query().Get("command")
	if vendor == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"vendors": h.fieldReg.Vendors()})
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
		for i, t := range types { strs[i] = string(t) }
		json.NewEncoder(w).Encode(map[string]any{"cmdTypes": strs})
		return
	}
	cmdType := h.fieldReg.ClassifyCommand(vendor, command)
	if cmdType == model.CmdUnknown {
		jsonError(w, "unknown command", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"cmdType": string(cmdType), "fields": h.fieldReg.Fields(vendor, cmdType)})
}

func (h *handlers) apiFieldsVendorsHTML(w http.ResponseWriter, r *http.Request) {
	if h.fieldReg == nil {
		fmt.Fprint(w, `<p style="color:var(--danger)">Field registry not available</p>`)
		return
	}
	vendors := h.fieldReg.Vendors()
	var b strings.Builder
	b.WriteString(`<div style="display:flex;flex-direction:column;gap:4px">`)
	for _, v := range vendors {
		fmt.Fprintf(&b,
			`<button class="btn btn-ghost btn-sm" style="justify-content:flex-start" `+
				`hx-get="/api/fields/schema-html?vendor=%s" hx-target="#schema-panel" hx-swap="innerHTML" `+
				`onclick="this.parentNode.querySelectorAll('button').forEach(b=>b.classList.remove('btn-primary'));this.classList.add('btn-primary');this.classList.remove('btn-ghost')">`+
				`%s</button>`, v, v)
	}
	b.WriteString(`</div>`)
	fmt.Fprint(w, b.String())
}

func (h *handlers) apiFieldsSchemaHTML(w http.ResponseWriter, r *http.Request) {
	if h.fieldReg == nil {
		fmt.Fprint(w, `<p style="color:var(--danger)">Field registry not available</p>`)
		return
	}
	vendor := r.URL.Query().Get("vendor")
	cmdtype := r.URL.Query().Get("cmdtype")
	types := h.fieldReg.CmdTypes(vendor)
	if types == nil {
		fmt.Fprintf(w, `<p style="color:var(--danger)">Unknown vendor: %s</p>`, vendor)
		return
	}
	var b strings.Builder
	b.WriteString(`<div style="display:grid;grid-template-columns:220px 1fr;gap:16px">`)
	b.WriteString(`<div>`)
	fmt.Fprintf(&b, `<div style="font-weight:600;margin-bottom:8px;color:var(--text-secondary)">%s — Types</div>`, vendor)
	for _, ct := range types {
		defs := h.fieldReg.Fields(vendor, ct)
		cls := "btn btn-ghost btn-sm"
		if string(ct) == cmdtype { cls = "btn btn-primary btn-sm" }
		fmt.Fprintf(&b,
			`<button class="%s" style="width:100%%;justify-content:space-between;margin-bottom:2px" `+
				`hx-get="/api/fields/schema-html?vendor=%s&cmdtype=%s" hx-target="#schema-panel" hx-swap="innerHTML">`+
				`<span>%s</span><span class="tag">%d</span></button>`,
			cls, vendor, string(ct), string(ct), len(defs))
	}
	b.WriteString(`</div><div>`)
	if cmdtype != "" {
		defs := h.fieldReg.Fields(vendor, model.CommandType(cmdtype))
		if len(defs) == 0 {
			fmt.Fprintf(&b, `<p style="color:var(--text-muted)">No fields for %s / %s</p>`, vendor, cmdtype)
		} else {
			fmt.Fprintf(&b, `<div style="font-weight:600;margin-bottom:8px">%s / %s</div>`, vendor, cmdtype)
			b.WriteString(`<table class="data-table"><tr><th>Field</th><th>Type</th><th>Description</th><th>Example</th><th>Derived</th></tr>`)
			for _, d := range defs {
				derived := ""
				if d.Derived {
					derived = fmt.Sprintf(`<span class="tag tag-derived">from: %s</span>`, strings.Join(d.DerivedFrom, ", "))
				}
				fmt.Fprintf(&b, `<tr><td><code>%s</code></td><td><span class="tag">%s</span></td><td>%s</td><td><code>%s</code></td><td>%s</td></tr>`,
					d.Name, string(d.Type), d.Description, d.Example, derived)
			}
			b.WriteString(`</table>`)
		}
	} else {
		b.WriteString(`<p style="color:var(--text-muted)">← Select a command type</p>`)
	}
	b.WriteString(`</div></div>`)
	fmt.Fprint(w, b.String())
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ══════════════════════════════════════════════════════════════════════════
// HTML Templates — Dark theme with sidebar layout
// ══════════════════════════════════════════════════════════════════════════

const baseHTML = `<!DOCTYPE html>
<html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Rule Studio</title>
<link rel="stylesheet" href="/static/style.css">
<script src="/static/htmx.min.js"></script>
<script src="/static/app.js" defer></script>
</head><body>
<div class="app-layout">
  <div class="status-bar">
    <div class="brand">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/></svg>
      Rule Studio
    </div>
    <div class="status-indicators">
      <span><span class="status-dot {{if .LLMAvailable}}ok{{else}}warn{{end}}"></span>LLM</span>
      <span><span class="status-dot {{if .GitAvailable}}ok{{else}}warn{{end}}"></span>Git</span>
      <span><span class="status-dot ok"></span>DB</span>
    </div>
  </div>
  <nav class="sidebar">
    <div class="sidebar-label">Navigation</div>
    <a href="/" {{if eq .ActivePage "dashboard"}}class="active"{{end}}>
      <span class="nav-icon">📊</span>Dashboard
    </a>
    <a href="/rules" {{if eq .ActivePage "rules"}}class="active"{{end}}>
      <span class="nav-icon">📋</span>Rules
    </a>
    <a href="/unknown" {{if eq .ActivePage "unknown"}}class="active"{{end}}>
      <span class="nav-icon">⚠️</span>Unknown{{if gt .UnknownCount 0}}<span class="nav-badge">{{.UnknownCount}}</span>{{end}}
    </a>
    <div class="sidebar-sep"></div>
    <div class="sidebar-label">Inspect</div>
    <a href="/patterns" {{if eq .ActivePage "patterns"}}class="active"{{end}}>
      <span class="nav-icon">🔗</span>Command Patterns
    </a>
    <a href="/fields" {{if eq .ActivePage "fields"}}class="active"{{end}}>
      <span class="nav-icon">🔍</span>Field Browser
    </a>
    <a href="/compare" {{if eq .ActivePage "compare"}}class="active"{{end}}>
      <span class="nav-icon">⚖️</span>Cross-Vendor Compare
    </a>
    <a href="/test" {{if eq .ActivePage "tester"}}class="active"{{end}}>
      <span class="nav-icon">🧪</span>Parser Tester
    </a>
    <div class="sidebar-sep"></div>
    <a href="/history" {{if eq .ActivePage "history"}}class="active"{{end}}>
      <span class="nav-icon">📜</span>History
    </a>
    <div class="sidebar-sep"></div>
    <button class="nav-btn" onclick="runDiscovery()">
      <span class="nav-icon">🔄</span>Run Discovery
    </button>
  </nav>
  <main class="main-content">
`

const pageFooter = `
  </main>
</div>
<div id="toast-container"></div>
</body></html>`

// ── Dashboard ────────────────────────────────────────────────────────────

const dashboardHTML = `
<div class="page-header">
  <h1>📊 Dashboard</h1>
  <div class="actions">
    <button class="btn btn-primary" onclick="runDiscovery()">🔄 Run Discovery</button>
  </div>
</div>

<!-- Needs Attention -->
{{if .Attention}}
<div class="card" style="border-color:var(--warning);border-width:2px">
  <div class="card-header"><h3 style="color:var(--warning)">🔔 Needs Your Attention</h3></div>
  <div class="card-body">
    {{range .Attention}}
    <div class="attention-item {{.Urgency}}">
      <div style="display:flex;align-items:center;gap:10px">
        <span style="font-size:1.2rem">{{.Icon}}</span>
        <div>
          <div style="font-weight:600">{{.Title}}</div>
          <div style="font-size:0.82rem;color:var(--text-secondary)">{{.Detail}}</div>
        </div>
      </div>
      {{if .Link}}<a href="{{.Link}}" class="btn btn-ghost btn-sm">Go →</a>{{end}}
    </div>
    {{end}}
  </div>
</div>
{{end}}

<!-- Stats Grid -->
<div class="info-grid" style="grid-template-columns:repeat(4,1fr)">
  <div class="info-item">
    <div class="label">Unknown Commands</div>
    <div class="value" style="{{if gt .UnknownCommands 0}}color:var(--warning){{end}}">{{.UnknownCommands}}</div>
  </div>
  <div class="info-item">
    <div class="label">Draft Rules</div>
    <div class="value" style="{{if gt .DraftCount 0}}color:var(--accent){{end}}">{{.DraftCount}}</div>
  </div>
  <div class="info-item">
    <div class="label">In Testing</div>
    <div class="value">{{.TestingCount}}</div>
  </div>
  <div class="info-item">
    <div class="label">Approved</div>
    <div class="value" style="color:var(--success)">{{.ApprovedCount}}</div>
  </div>
</div>

<!-- Vendor Coverage -->
{{if .VendorCoverages}}
<div class="card">
  <div class="card-header"><h3>🏭 Vendor Parser Coverage</h3></div>
  <div class="card-body">
    <table class="data-table">
      <tr><th>Vendor</th><th>Supported CommandTypes</th><th>Unknown Commands</th><th>Coverage Status</th></tr>
      {{range .VendorCoverages}}
      <tr>
        <td><span class="badge badge-vendor">{{.Vendor}}</span></td>
        <td><span class="tag">{{.SupportedTypes}} types</span></td>
        <td>{{range $.VendorUnknowns}}{{if eq .Vendor $.VendorCoverages}}{{.Count}}{{end}}{{end}}—</td>
        <td>
          {{if gt .SupportedTypes 5}}<span style="color:var(--success)">● Good</span>
          {{else if gt .SupportedTypes 3}}<span style="color:var(--warning)">● Basic</span>
          {{else}}<span style="color:var(--danger)">● Minimal</span>{{end}}
        </td>
      </tr>
      {{end}}
    </table>
    {{if .VendorUnknowns}}
    <div style="margin-top:12px;font-size:0.82rem;color:var(--text-secondary)">
      Unknown commands by vendor:
      {{range .VendorUnknowns}}
        <span class="badge badge-vendor" style="margin-left:4px">{{.Vendor}}: {{.Count}}</span>
      {{end}}
    </div>
    {{end}}
  </div>
</div>
{{end}}

<!-- Quick Links -->
<div class="card">
  <div class="card-header"><h3>⚡ Quick Actions</h3></div>
  <div class="card-body" style="display:grid;grid-template-columns:repeat(3,1fr);gap:12px">
    <a href="/patterns" class="btn btn-ghost" style="justify-content:center;padding:16px">🔗 View Command Patterns</a>
    <a href="/compare" class="btn btn-ghost" style="justify-content:center;padding:16px">⚖️ Cross-Vendor Compare</a>
    <a href="/test" class="btn btn-ghost" style="justify-content:center;padding:16px">🧪 Test a Parser</a>
  </div>
</div>
` + pageFooter

// ── Rules List ───────────────────────────────────────────────────────────

const listHTML = `
<div class="page-header">
  <h1>📋 Rules</h1>
  <div class="actions">
    {{if gt .DraftCount 0}}<button class="btn btn-danger btn-sm" onclick="confirmAction('Delete All Drafts','This will permanently delete ALL {{.DraftCount}} draft rules and reset their unknown outputs for re-discovery.',function(){fetch('/api/rules/delete-drafts',{method:'POST'}).then(r=>r.json()).then(d=>{if(d.error){Toast.error(d.error)}else{Toast.success(d.count+' draft(s) deleted');setTimeout(()=>location.href='/rules',500)}})})">🗑 Delete All Drafts ({{.DraftCount}})</button>{{end}}
    <button class="btn btn-primary" onclick="runDiscovery()">🔄 Run Discovery</button>
  </div>
</div>

<!-- Search & Filter Bar -->
<div class="card" style="margin-bottom:16px;padding:12px 16px">
  <form method="GET" action="/rules" style="display:flex;gap:12px;align-items:center;flex-wrap:wrap">
    <div style="flex:1;min-width:200px">
      <input type="text" name="q" placeholder="🔍 Search command pattern..." value="{{.Query}}" style="width:100%;margin:0" autofocus>
    </div>
    <select name="vendor" style="width:140px;margin:0">
      <option value="">All Vendors</option>
      {{range .Vendors}}<option value="{{.}}" {{if eq . $.FilterVendor}}selected{{end}}>{{.}}</option>{{end}}
    </select>
    <select name="status" style="width:140px;margin:0">
      <option value="">All Status</option>
      <option value="draft" {{if eq .FilterStatus "draft"}}selected{{end}}>Draft</option>
      <option value="testing" {{if eq .FilterStatus "testing"}}selected{{end}}>Testing</option>
      <option value="approved" {{if eq .FilterStatus "approved"}}selected{{end}}>Approved</option>
      <option value="rejected" {{if eq .FilterStatus "rejected"}}selected{{end}}>Rejected</option>
    </select>
    <button type="submit" class="btn btn-primary btn-sm">Search</button>
    {{if or .Query .FilterVendor .FilterStatus}}<a href="/rules" class="btn btn-ghost btn-sm">✕ Clear</a>{{end}}
  </form>
</div>

<!-- Status Summary Tabs -->
<div style="display:flex;gap:8px;margin-bottom:16px;flex-wrap:wrap">
  <a href="/rules" class="btn btn-ghost btn-sm {{if and (not .FilterStatus) (not .Query)}}btn-primary{{end}}" style="min-width:80px;justify-content:center">
    All <span class="tag" style="margin-left:4px">{{.TotalCount}}</span>
  </a>
  {{if gt .DraftCount 0}}<a href="/rules?status=draft" class="btn btn-ghost btn-sm {{if eq .FilterStatus "draft"}}btn-primary{{end}}" style="min-width:80px;justify-content:center">
    📝 Draft <span class="tag" style="margin-left:4px">{{.DraftCount}}</span>
  </a>{{end}}
  {{if gt .TestingCount 0}}<a href="/rules?status=testing" class="btn btn-ghost btn-sm {{if eq .FilterStatus "testing"}}btn-primary{{end}}" style="min-width:80px;justify-content:center">
    🧪 Testing <span class="tag" style="margin-left:4px">{{.TestingCount}}</span>
  </a>{{end}}
  {{if gt .ApprovedCount 0}}<a href="/rules?status=approved" class="btn btn-ghost btn-sm {{if eq .FilterStatus "approved"}}btn-primary{{end}}" style="min-width:80px;justify-content:center">
    ✅ Approved <span class="tag" style="margin-left:4px">{{.ApprovedCount}}</span>
  </a>{{end}}
  {{if gt .RejectedCount 0}}<a href="/rules?status=rejected" class="btn btn-ghost btn-sm {{if eq .FilterStatus "rejected"}}btn-primary{{end}}" style="min-width:80px;justify-content:center">
    ✕ Rejected <span class="tag" style="margin-left:4px">{{.RejectedCount}}</span>
  </a>{{end}}
</div>

{{if .Rules}}
<div class="card">
  <table class="data-table">
    <tr>
      <th>Vendor</th><th>Command Pattern</th><th>Type</th>
      <th>Confidence</th><th>Status</th><th>Created</th><th>Actions</th>
    </tr>
    {{range .Rules}}
    <tr id="rule-row-{{.ID}}">
      <td><span class="badge badge-vendor">{{.Vendor}}</span></td>
      <td><code>{{truncate .CommandPattern 50}}</code></td>
      <td><span class="tag {{outputTypeClass .OutputType}}">{{.OutputType}}</span></td>
      <td>
        <div class="confidence-bar">
          <div class="confidence-track">
            <div class="confidence-fill {{confClass .Confidence}}" style="width:{{confPct .Confidence}}%"></div>
          </div>
          <span style="font-size:0.8rem;color:var(--text-muted)">{{confPct .Confidence}}%</span>
        </div>
      </td>
      <td><span class="badge {{statusClass .Status}}">{{.Status}}</span></td>
      <td style="font-size:0.8rem;color:var(--text-muted)">{{.CreatedAt.Format "2006-01-02"}}</td>
      <td>
        <div class="btn-group">
          <a href="/rule/{{.ID}}" class="btn btn-ghost btn-sm">Edit & Test</a>
          <button class="btn btn-ghost btn-sm" style="color:var(--danger)" onclick="confirmAction('Delete Rule #{{.ID}}','Permanently delete this rule and its test cases?',function(){fetch('/api/rule/{{.ID}}/delete',{method:'POST'}).then(r=>r.json()).then(d=>{if(d.error){Toast.error(d.error)}else{document.getElementById('rule-row-{{.ID}}').remove();Toast.success('Rule deleted')}})})">✕</button>
        </div>
      </td>
    </tr>
    {{end}}
  </table>
</div>
{{else}}
<div class="empty-state">
  {{if or .Query .FilterVendor .FilterStatus}}
  <h3>No Rules Found</h3>
  <p>No rules match your search criteria. <a href="/rules">Clear filters</a></p>
  {{else}}
  <h3>No Rules</h3>
  <p>Import device logs and run discovery to generate parser rule drafts.</p>
  <div class="steps-flow">
    <div class="step-item done"><span class="step-num">1</span>Import logs</div>
    <div class="step-item active"><span class="step-num">2</span>Unknown commands appear</div>
    <div class="step-item"><span class="step-num">3</span>LLM generates drafts</div>
    <div class="step-item"><span class="step-num">4</span>Review & approve</div>
  </div>
  <div style="margin-top:16px">
    <code style="color:var(--text-muted)">nethelper watch ingest ~/logs/session.log</code>
  </div>
  <button class="btn btn-primary" style="margin-top:16px" onclick="runDiscovery()">🔄 Run Discovery Now</button>
  {{end}}
</div>
{{end}}
` + pageFooter

// ── Rule Editor + Sandbox (merged) ───────────────────────────────────────

const editorHTML = `
<div class="breadcrumb"><a href="/rules">Rules</a> / {{.Rule.CommandPattern}}</div>
<div class="page-header">
  <h1>{{.Rule.CommandPattern}}</h1>
  <div class="actions">
    <span class="badge {{statusClass .Rule.Status}}">{{.Rule.Status}}</span>
    <span class="badge badge-vendor">{{.Rule.Vendor}}</span>
  </div>
</div>

<div class="info-grid">
  <div class="info-item"><div class="label">Output Type</div><div class="value"><span class="tag {{outputTypeClass .Rule.OutputType}}">{{.Rule.OutputType}}</span></div></div>
  <div class="info-item">
    <div class="label">Confidence</div>
    <div class="value">
      <div class="confidence-bar">
        <div class="confidence-track" style="width:80px">
          <div class="confidence-fill {{confClass .Rule.Confidence}}" style="width:{{confPct .Rule.Confidence}}%"></div>
        </div>
        {{confPct .Rule.Confidence}}%
      </div>
    </div>
  </div>
  <div class="info-item"><div class="label">Occurrences</div><div class="value">{{.Rule.OccurrenceCount}}</div></div>
  <div class="info-item"><div class="label">Test Cases</div><div class="value"><span id="tc-count">{{.TestCount}}</span></div></div>
</div>

<!-- ═══ Section 1: Schema / Code Editor ═══ -->
<div class="card">
  <div class="card-header">
    <h3>{{if eq .Rule.OutputType "table"}}📝 Schema YAML{{else if eq .Rule.OutputType "pipeline"}}🔧 Pipeline DSL{{else}}📝 Go Code Draft{{end}}</h3>
    <div class="btn-group">
      {{if .LLMAvailable}}<button class="btn btn-sm btn-ghost" onclick="askLLMImprove({{.Rule.ID}}, '{{.Rule.OutputType}}')">🤖 Ask LLM to Improve</button>{{end}}
      <button class="btn btn-sm btn-primary" onclick="saveSchema({{.Rule.ID}})">💾 Save (⌘S)</button>
    </div>
  </div>
  <div class="card-body">
    {{if eq .Rule.OutputType "pipeline"}}
    <div style="margin-bottom:8px;padding:8px 12px;background:var(--bg-tertiary);border-radius:var(--radius);font-size:0.78rem;color:var(--text-secondary)">
      <strong>DSL Reference:</strong>
      <code>SKIP_UNTIL</code> <code>SKIP_LINES</code> <code>SKIP_BLANK</code> <code>STOP_AT</code> <code>FILTER</code> <code>REJECT</code> — trimming &nbsp;|&nbsp;
      <code>SPLIT $a $b $c</code> — whitespace split (last var gets rest) &nbsp;|&nbsp;
      <code>REGEX (?P&lt;name&gt;...)</code> — named capture &nbsp;|&nbsp;
      <code>REPLACE &lt;pattern&gt; "replacement"</code> &nbsp;|&nbsp;
      <code>SET $x $a "/" $b</code> — concat/ternary &nbsp;|&nbsp;
      <code>SECTION</code> — split into independent sub-pipelines (for multi-table output, joined by row index) &nbsp;|&nbsp;
      Lines starting with <code>#</code> are comments
    </div>
    <textarea name="schema_yaml" class="code-editor" id="pipeline-editor">{{.Rule.SchemaYAML}}</textarea>
    {{else if eq .Rule.OutputType "table"}}
    <textarea name="schema_yaml" class="code-editor" id="schema-editor">{{.Rule.SchemaYAML}}</textarea>
    {{else}}
    <textarea name="go_code_draft" class="code-editor" id="code-editor">{{.Rule.GoCodeDraft}}</textarea>
    {{end}}
  </div>
</div>

<!-- ═══ Section 2: Sample Inputs (from discovery) ═══ -->
<div class="card">
  <div class="card-header">
    <h3>📋 Sample Inputs</h3>
    <span style="font-size:0.78rem;color:var(--text-muted)">Raw device output collected during discovery</span>
  </div>
  <div class="card-body" id="sample-inputs-container">
    <div class="sample-inputs-raw" style="display:none">{{.Rule.SampleInputs}}</div>
  </div>
</div>

<!-- ═══ Section 3: Test & Validate (unified workflow) ═══ -->
<div class="card">
  <div class="card-header">
    <h3>🧪 Test & Validate</h3>
    <span style="font-size:0.78rem;color:var(--text-muted)">Paste output → Parse → Review → Save as test case</span>
  </div>
  <div class="card-body">
    <!-- Step 1: Input -->
    <div class="test-step">
      <div class="test-step-header">
        <span class="step-indicator">1</span>
        <label style="margin:0">Device CLI Output</label>
      </div>
      <textarea id="input-area" placeholder="Paste device CLI output here, or click 'Use Sample' above to load a sample..." style="min-height:160px"></textarea>
    </div>

    <!-- Step 2: Parse -->
    <div class="test-step" style="margin-top:12px">
      <div class="test-step-header">
        <span class="step-indicator">2</span>
        <label style="margin:0">Parse & Review</label>
        <div class="btn-group" style="margin-left:auto">
          <button class="btn btn-primary btn-sm" onclick="runParse({{.Rule.ID}})">▶ Run Parse (⌘↵)</button>
        </div>
      </div>
      <div id="parse-result" class="result-area empty">
        Click "Run Parse" to test the schema against your input
      </div>
    </div>

    <!-- Step 3: Save Test Case -->
    <div class="test-step" style="margin-top:12px">
      <div class="test-step-header">
        <span class="step-indicator">3</span>
        <label style="margin:0">Save as Test Case</label>
      </div>
      <div style="display:grid;grid-template-columns:1fr 2fr;gap:12px;margin-top:8px">
        <div>
          <label style="font-size:0.78rem">Description</label>
          <input type="text" id="tc-desc" placeholder="e.g. NE40E with 3 interfaces up">
        </div>
        <div>
          <label style="font-size:0.78rem">Expected Result <span style="color:var(--text-muted)">(auto-filled from parse result if available)</span></label>
          <textarea id="tc-expected" placeholder='JSON format, e.g.: {"Rows":[{"Port":"XGE2/0/35:1","Status":"S"}]}&#10;&#10;Tip: Click "Run Parse" first, the result will auto-fill here.' style="min-height:200px;max-height:400px"></textarea>
        </div>
      </div>
      <div style="margin-top:8px;display:flex;align-items:center;gap:8px">
        <button class="btn btn-primary btn-sm" onclick="saveTestCase({{.Rule.ID}})">💾 Save Test Case</button>
        <span style="font-size:0.78rem;color:var(--text-muted)">Input from step 1 + expected from here will be saved</span>
      </div>
    </div>
  </div>
</div>

<!-- ═══ Section 4: Test Cases List ═══ -->
<div class="card">
  <div class="card-header">
    <h3>📋 Test Cases (<span id="tc-count-header">{{.TestCount}}</span>)</h3>
    {{if .TestCases}}<button class="btn btn-sm btn-ghost" onclick="runAllTestCases({{.Rule.ID}})">▶ Run All</button>{{end}}
  </div>
  <div class="card-body" id="test-cases-list">
    {{if .TestCases}}
    {{range .TestCases}}
    <div class="test-case-item" id="tc-{{.ID}}">
      <div class="test-case-main" onclick="toggleTestCase({{.ID}})">
        <div style="display:flex;align-items:center;gap:8px">
          <span class="tc-expand-icon" id="tc-icon-{{.ID}}">▸</span>
          <span class="tc-status-dot" id="tc-dot-{{.ID}}"></span>
          <span>{{if .Description}}{{.Description}}{{else}}Test #{{.ID}}{{end}}</span>
        </div>
        <div style="display:flex;align-items:center;gap:8px">
          <span class="test-case-meta">{{.CreatedAt.Format "01-02 15:04"}}</span>
          <button class="btn btn-ghost btn-sm" onclick="event.stopPropagation();loadTestCase({{.ID}}, {{.RuleID}})" title="Load into test area">↑ Load</button>
          <button class="btn btn-ghost btn-sm" onclick="event.stopPropagation();deleteTestCase({{.ID}}, {{.RuleID}})" title="Delete" style="color:var(--danger)">✕</button>
        </div>
      </div>
      <div class="test-case-detail" id="tc-detail-{{.ID}}" style="display:none">
        <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-top:8px">
          <div>
            <label style="font-size:0.75rem;color:var(--text-muted)">Input</label>
            <div class="output-preview" style="max-height:150px;font-size:0.78rem">{{.Input}}</div>
          </div>
          <div>
            <label style="font-size:0.75rem;color:var(--text-muted)">Expected</label>
            <div class="output-preview" style="max-height:150px;font-size:0.78rem">{{.Expected}}</div>
          </div>
        </div>
      </div>
    </div>
    {{end}}
    {{else}}
    <div class="empty-state" style="padding:24px">
      <p style="color:var(--text-muted);font-size:0.85rem">No test cases yet. Use the workflow above to parse output and save test cases.</p>
    </div>
    {{end}}
  </div>
</div>

<!-- ═══ Action Bar ═══ -->
<div class="action-bar">
  <div>
    <a href="/rules" class="btn btn-ghost">← Back to Rules</a>
  </div>
  <div class="btn-group">
    <button class="btn btn-danger btn-sm" onclick="confirmAction('Reject Rule','This will mark the rule as rejected and moved to history.',function(){fetch('/api/rule/{{.Rule.ID}}/ignore',{method:'POST'}).then(()=>{Toast.success('Rule rejected');setTimeout(()=>location.href='/rules',500)})})">
      ✕ Reject
    </button>
    {{if gt .TestCount 0}}
    <button class="btn btn-ghost" onclick="saveLocal({{.Rule.ID}})">💾 Save to Local Files</button>
    <button class="btn btn-success" onclick="approveAndSave({{.Rule.ID}})">
      ✅ Approve & Save
    </button>
    {{else}}
    <span style="color:var(--warning);font-size:0.85rem">⚠ Save at least 1 test case to enable approve</span>
    {{end}}
  </div>
</div>
<div id="save-result" style="margin-top:8px"></div>
` + pageFooter

// ── Parser Tester ────────────────────────────────────────────────────────

const testPageHTML = `
<div class="page-header"><h1>🧪 Parser Tester</h1></div>
<p style="color:var(--text-secondary);margin-bottom:16px">Test existing parsers against CLI output.</p>
<div class="card">
  <div class="card-body">
    <div style="display:grid;grid-template-columns:200px 1fr;gap:12px;margin-bottom:12px">
      <div>
        <label>Vendor</label>
        <select id="vendor">
          {{range .Vendors}}<option value="{{.}}">{{.}}</option>{{end}}
        </select>
      </div>
      <div>
        <label>Command</label>
        <input type="text" id="command" placeholder="e.g. display bgp peer">
      </div>
    </div>
    <label>CLI Output</label>
    <textarea id="output" placeholder="Paste device output here..." style="min-height:200px"></textarea>
    <div style="margin-top:12px">
      <button class="btn btn-primary" hx-post="/api/test" hx-include="#vendor,#command,#output" hx-target="#tester-result" hx-swap="innerHTML">▶ Parse</button>
    </div>
    <div id="tester-result" class="result-area empty" style="margin-top:12px">Paste output and click ▶ Parse</div>
  </div>
</div>
<script>
document.body.addEventListener('htmx:afterSwap', function(evt) {
  if (evt.detail.target.id === 'tester-result') {
    formatParseResult(evt.detail.xhr.responseText, 'tester-result');
  }
});
</script>
` + pageFooter

// ── Field Browser ────────────────────────────────────────────────────────

const fieldsHTML = `
<div class="page-header"><h1>🔍 Field Browser</h1></div>
<p style="color:var(--text-secondary);margin-bottom:16px">Browse parsed field schemas for each vendor.</p>
<div class="card">
  <div class="card-body">
    <div style="display:grid;grid-template-columns:200px 1fr;gap:16px;min-height:300px">
      <div>
        <div style="font-weight:600;margin-bottom:8px;font-size:0.85rem;color:var(--text-secondary)">Vendors</div>
        <div hx-get="/api/fields/vendors-html" hx-trigger="load" hx-swap="outerHTML">
          <span class="spinner"></span> Loading...
        </div>
      </div>
      <div id="schema-panel">
        <p style="color:var(--text-muted)">← Select a vendor</p>
      </div>
    </div>
  </div>
</div>
` + pageFooter

// ── Unknown Outputs ──────────────────────────────────────────────────────

const unknownListHTML = `
<div class="page-header">
  <h1>⚠️ Unknown Outputs</h1>
  <div class="actions">
    <button class="btn btn-primary" onclick="runDiscovery()">🔄 Discover Rules</button>
  </div>
</div>

<!-- Filter info -->
<div class="card" style="margin-bottom:16px;padding:12px 16px;border-color:var(--border)">
  <div style="display:flex;align-items:center;gap:12px;flex-wrap:wrap">
    <span style="font-size:0.82rem;color:var(--text-secondary)">
      <strong>Auto-filtered:</strong> empty outputs • control commands (quit/return/save/system-view/screen-length…) • help queries (?)
    </span>
    <span style="font-size:0.78rem;color:var(--text-muted)">|</span>
    <span style="font-size:0.82rem;color:var(--text-secondary)">
      <strong>Normalised:</strong> abbreviations expanded • trailing arguments (IPs, interface names, IDs) replaced with placeholders
    </span>
  </div>
</div>

<p style="color:var(--text-secondary);margin-bottom:16px">Commands from ingested logs that no parser matched. Only commands with actual output are shown.</p>
{{if .Outputs}}
<div class="card">
  <table class="data-table">
    <tr><th>Vendor</th><th>Command Pattern</th><th>Raw Command</th><th>Count</th><th>First Seen</th><th>Preview</th><th>Actions</th></tr>
    {{range .Outputs}}
    <tr id="unknown-{{.ID}}">
      <td><span class="badge badge-vendor">{{.Vendor}}</span></td>
      <td>
        <code style="font-weight:600">{{.CommandNorm}}</code>
      </td>
      <td style="font-size:0.78rem;color:var(--text-muted)"><code>{{truncate .CommandRaw 40}}</code></td>
      <td>{{.OccurrenceCount}}</td>
      <td style="font-size:0.8rem;color:var(--text-muted)">{{.FirstSeen.Format "2006-01-02"}}</td>
      <td><div class="truncate output-preview" style="max-height:40px;padding:4px 8px;max-width:260px;font-size:0.78rem">{{truncate .RawOutput 120}}</div></td>
      <td>
        <div class="btn-group">
          <button class="btn btn-primary btn-sm" onclick="storeOriginalActions({{.ID}});generateRule({{.ID}}, '{{.CommandNorm}}')">
            🔬 Generate
          </button>
          <button class="btn btn-ghost btn-sm" hx-post="/api/unknown/{{.ID}}/ignore" hx-target="#unknown-{{.ID}}" hx-swap="outerHTML"
                  onclick="Toast.info('Ignored')">✕</button>
        </div>
      </td>
    </tr>
    {{end}}
  </table>
</div>
{{else}}
<div class="empty-state">
  <h3>No Unknown Outputs</h3>
  <p>All ingested commands are matched by existing parsers, or no logs have been imported yet.</p>
  <p style="font-size:0.85rem;color:var(--text-muted);margin-top:8px">Control commands, empty outputs, and help queries are auto-filtered.</p>
</div>
{{end}}
` + pageFooter

// ── History ──────────────────────────────────────────────────────────────

const historyHTML = `
<div class="page-header"><h1>📊 Rule History</h1></div>
<p style="color:var(--text-secondary);margin-bottom:16px">Previously approved and rejected rules.</p>
{{if .Rules}}
<div class="card">
  <table class="data-table">
    <tr><th>ID</th><th>Vendor</th><th>Command</th><th>Status</th><th>Approved By</th><th>PR</th><th>Date</th></tr>
    {{range .Rules}}
    <tr>
      <td style="color:var(--text-muted)">#{{.ID}}</td>
      <td><span class="badge badge-vendor">{{.Vendor}}</span></td>
      <td><code>{{truncate .CommandPattern 45}}</code></td>
      <td><span class="badge {{statusClass .Status}}">{{.Status}}</span></td>
      <td style="color:var(--text-secondary)">{{if .ApprovedBy}}{{.ApprovedBy}}{{else}}—{{end}}</td>
      <td>{{if .PRURL}}<a href="{{.PRURL}}" target="_blank" style="color:var(--accent)">{{truncate .PRURL 30}}</a>{{else}}—{{end}}</td>
      <td style="font-size:0.8rem;color:var(--text-muted)">{{.CreatedAt.Format "2006-01-02"}}</td>
    </tr>
    {{end}}
  </table>
</div>
{{else}}
<div class="empty-state">
  <h3>No History</h3>
  <p>Approved and rejected rules will appear here.</p>
</div>
{{end}}
` + pageFooter

// ── Command Patterns ─────────────────────────────────────────────────────

const patternsHTML = `
<div class="page-header"><h1>🔗 Command Patterns</h1></div>
<p style="color:var(--text-secondary);margin-bottom:16px">Classification prefix rules per vendor. Commands matching these prefixes are parsed; others become "unknown".</p>

{{range .Patterns}}
<div class="card">
  <div class="card-header">
    <h3><span class="badge badge-vendor">{{.Vendor}}</span> — {{len .Rules}} prefix rules</h3>
  </div>
  <div class="card-body">
    <table class="data-table">
      <tr><th style="width:50%">Command Prefix</th><th>Maps to CommandType</th></tr>
      {{range .Rules}}
      <tr>
        <td><code>{{.Prefix}}</code></td>
        <td><span class="tag">{{.CmdType}}</span></td>
      </tr>
      {{end}}
    </table>
  </div>
</div>
{{end}}

{{if .Unknowns}}
<div class="card" style="border-color:var(--warning)">
  <div class="card-header"><h3 style="color:var(--warning)">⚠️ Unmatched Commands ({{len .Unknowns}} unique)</h3></div>
  <div class="card-body">
    <p style="color:var(--text-secondary);margin-bottom:12px;font-size:0.85rem">These commands were seen in ingested logs but matched no prefix rule above.</p>
    {{range .Unknowns}}
    <div class="pattern-unknown-item">
      <div>
        <span class="badge badge-vendor" style="margin-right:8px">{{.Vendor}}</span>
        <code>{{.CommandNorm}}</code>
        <span style="font-size:0.75rem;color:var(--text-muted);margin-left:8px">({{.OccurrenceCount}}x)</span>
      </div>
      <details style="margin-top:4px">
        <summary style="font-size:0.78rem;color:var(--text-muted);cursor:pointer">Show sample output</summary>
        <div class="output-preview" style="max-height:200px;margin-top:4px;font-size:0.75rem">{{truncate .RawOutput 500}}</div>
      </details>
    </div>
    {{end}}
  </div>
</div>
{{end}}
` + pageFooter

// ── Cross-Vendor Compare ─────────────────────────────────────────────────

const compareHTML = `
<div class="page-header"><h1>⚖️ Cross-Vendor Field Compare</h1></div>
<p style="color:var(--text-secondary);margin-bottom:16px">Compare parsed fields for the same CommandType across different vendors.</p>

{{if .Rows}}
{{range .Rows}}
<div class="card">
  <div class="card-header"><h3><span class="tag" style="font-size:0.9rem">{{.CmdType}}</span></h3></div>
  <div class="card-body">
    <div style="display:grid;grid-template-columns:repeat({{len .Vendors}}, 1fr);gap:16px">
      {{range .Vendors}}
      <div>
        <div style="font-weight:600;margin-bottom:8px"><span class="badge badge-vendor">{{.Vendor}}</span></div>
        {{if .Fields}}
        <div style="display:flex;flex-direction:column;gap:2px">
          {{range .Fields}}
          <code style="font-size:0.78rem;color:var(--text-secondary);padding:2px 6px;background:var(--bg-tertiary);border-radius:3px">{{.}}</code>
          {{end}}
        </div>
        {{else}}
        <span style="font-size:0.82rem;color:var(--text-muted);font-style:italic">Not supported</span>
        {{end}}
      </div>
      {{end}}
    </div>
  </div>
</div>
{{end}}
{{else}}
<div class="empty-state">
  <h3>No Field Data</h3>
  <p>Field registry not available. Parser vendors may not be registered.</p>
</div>
{{end}}
` + pageFooter
