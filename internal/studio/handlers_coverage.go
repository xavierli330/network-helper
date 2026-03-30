package studio

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/xavierli/nethelper/internal/parser"
	"github.com/xavierli/nethelper/internal/store"
)

// ── Coverage Check Handlers ─────────────────────────────────────────────────

// coveragePage renders the /coverage HTML page.
func (h *handlers) coveragePage(w http.ResponseWriter, r *http.Request) {
	checks, _ := h.db.ListCoverageChecks()

	// Aggregate stats
	var totalDevices, fullyCovered, partialCovered, uncovered int
	var avgPct float64
	totalDevices = len(checks)
	for _, c := range checks {
		avgPct += c.CoveragePct
		if c.CoveragePct >= 100 {
			fullyCovered++
		} else if c.CoveragePct > 0 {
			partialCovered++
		} else {
			uncovered++
		}
	}
	if totalDevices > 0 {
		avgPct /= float64(totalDevices)
	}

	data := struct {
		pageData
		Checks         []store.CoverageCheck
		TotalDevices   int
		FullyCovered   int
		PartialCovered int
		Uncovered      int
		AvgPct         float64
	}{
		pageData:       h.newPageData("coverage"),
		Checks:         checks,
		TotalDevices:   totalDevices,
		FullyCovered:   fullyCovered,
		PartialCovered: partialCovered,
		Uncovered:      uncovered,
		AvgPct:         avgPct,
	}

	tmpl := template.Must(template.New("coverage").Funcs(funcMap).Parse(baseHTML + coverageHTML + pageFooter))
	tmpl.Execute(w, data)
}

// coverageDetail renders the detail page for a single device's coverage.
func (h *handlers) coverageDetail(w http.ResponseWriter, r *http.Request) {
	deviceID := strings.TrimPrefix(r.URL.Path, "/coverage/")
	if deviceID == "" {
		http.Error(w, "missing device_id", http.StatusBadRequest)
		return
	}

	cc, err := h.db.GetCoverageCheck(deviceID)
	if err != nil || cc == nil {
		http.Error(w, "no coverage data for device", http.StatusNotFound)
		return
	}

	items, _ := store.ParseCoverageItems(cc.ItemsJSON)

	// Group by category
	type categoryGroup struct {
		Name    string
		Items   []store.CoverageItemRow
		Covered int
		Total   int
	}
	catMap := make(map[string]*categoryGroup)
	var catOrder []string
	for _, item := range items {
		if _, ok := catMap[item.Category]; !ok {
			catMap[item.Category] = &categoryGroup{Name: item.Category}
			catOrder = append(catOrder, item.Category)
		}
		g := catMap[item.Category]
		g.Items = append(g.Items, item)
		g.Total++
		if item.Status == "covered" {
			g.Covered++
		}
	}
	var groups []categoryGroup
	for _, cat := range catOrder {
		groups = append(groups, *catMap[cat])
	}

	data := struct {
		pageData
		Check  *store.CoverageCheck
		Items  []store.CoverageItemRow
		Groups []categoryGroup
	}{
		pageData: h.newPageData("coverage"),
		Check:    cc,
		Items:    items,
		Groups:   groups,
	}

	tmpl := template.Must(template.New("coverage-detail").Funcs(funcMap).Parse(baseHTML + coverageDetailHTML + pageFooter))
	tmpl.Execute(w, data)
}

// apiCoverageRecheck triggers a re-check for a device.
func (h *handlers) apiCoverageRecheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	deviceID := r.FormValue("device_id")
	if deviceID == "" {
		http.Error(w, "missing device_id", http.StatusBadRequest)
		return
	}

	// Get the device info
	dev, err := h.db.GetDevice(deviceID)
	if err != nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}

	// Get the latest config
	configs, err := h.db.GetConfigSnapshots(deviceID)
	if err != nil || len(configs) == 0 {
		http.Error(w, "no config found for device", http.StatusNotFound)
		return
	}

	// Run coverage check
	report := parser.CheckCoverage(dev.Vendor, configs[0].ConfigText, h.fieldReg.Reg())
	if report == nil {
		http.Error(w, "coverage check failed", http.StatusInternalServerError)
		return
	}
	report.DeviceID = deviceID

	itemsJSON, _ := json.Marshal(report.Items)
	h.db.InsertCoverageCheck(store.CoverageCheck{
		DeviceID:     deviceID,
		Vendor:       dev.Vendor,
		TotalCount:   report.TotalCount,
		CoveredCount: report.CoveredCount,
		CoveragePct:  report.CoveragePct,
		ItemsJSON:    string(itemsJSON),
		CheckedAt:    report.CheckedAt,
	})

	// Redirect back to detail
	http.Redirect(w, r, "/coverage/"+deviceID, http.StatusSeeOther)
}

// apiCoverageSSH returns the SSH script for uncovered commands.
func (h *handlers) apiCoverageSSH(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("device_id")
	if deviceID == "" {
		http.Error(w, "missing device_id", http.StatusBadRequest)
		return
	}

	dev, err := h.db.GetDevice(deviceID)
	if err != nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}

	configs, err := h.db.GetConfigSnapshots(deviceID)
	if err != nil || len(configs) == 0 {
		http.Error(w, "no config found", http.StatusNotFound)
		return
	}

	report := parser.CheckCoverage(dev.Vendor, configs[0].ConfigText, h.fieldReg.Reg())
	if report == nil {
		http.Error(w, "coverage check failed", http.StatusInternalServerError)
		return
	}
	report.DeviceID = deviceID

	script := parser.GenerateSSHScript(report)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=%s_uncovered_commands.txt", deviceID))
	w.Write([]byte(script))
}

// apiCoverageSummary returns JSON summary for all devices.
func (h *handlers) apiCoverageSummary(w http.ResponseWriter, r *http.Request) {
	checks, err := h.db.ListCoverageChecks()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(checks)
}

// apiCoverageBoost attempts to generate rules for all uncovered commands of a device.
// It finds matching unknown_outputs and triggers generation via the discovery engine.
// POST /api/coverage/boost with form: device_id=xxx
func (h *handlers) apiCoverageBoost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.eng == nil {
		jsonError(w, "LLM discovery engine not configured — cannot generate rules", 503)
		return
	}

	deviceID := r.FormValue("device_id")
	if deviceID == "" {
		jsonError(w, "missing device_id", 400)
		return
	}

	cc, err := h.db.GetCoverageCheck(deviceID)
	if err != nil || cc == nil {
		jsonError(w, "no coverage data for device", 404)
		return
	}

	items, _ := store.ParseCoverageItems(cc.ItemsJSON)
	var uncovered []store.CoverageItemRow
	for _, item := range items {
		if item.Status == "uncovered" {
			uncovered = append(uncovered, item)
		}
	}
	if len(uncovered) == 0 {
		jsonError(w, "all commands are already covered", 400)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		jsonError(w, "streaming not supported", 500)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	total := len(uncovered)
	created := 0
	failed := 0
	skipped := 0

	sendSSE(w, flusher, map[string]any{"type": "start", "total": total, "device_id": deviceID})

	// Gather all unknown outputs for this vendor
	allUnknowns, _ := h.db.ListUnknownOutputs(cc.Vendor, "", 5000)
	unknownMap := make(map[string]int) // commandNorm -> unknown ID
	for _, u := range allUnknowns {
		unknownMap[u.CommandNorm] = u.ID
	}

	for i, item := range uncovered {
		idx := i + 1

		// Try to find a matching unknown output by command name
		unknownID, found := unknownMap[item.Command]
		if !found {
			skipped++
			sendSSE(w, flusher, map[string]any{
				"type": "skipped", "index": idx, "total": total,
				"command": item.Command,
				"message": "no matching unknown output — run the command on the device and re-ingest the log first",
			})
			continue
		}

		// Check if a rule already exists
		existing, err := h.db.GetPendingRuleByCommandNorm(cc.Vendor, item.Command)
		if err == nil {
			skipped++
			sendSSE(w, flusher, map[string]any{
				"type": "skipped", "index": idx, "total": total,
				"command": item.Command, "rule_id": existing.ID,
				"message": "rule already exists",
			})
			continue
		}

		sendSSE(w, flusher, map[string]any{
			"type": "pending", "index": idx, "total": total,
			"command": item.Command,
		})

		// Reset status to "new" so engine can pick it up
		h.db.UpdateUnknownOutputStatus(cc.Vendor, item.Command, "new")

		ruleID, genErr := h.eng.GenerateForUnknown(r.Context(), unknownID)
		if genErr != nil {
			failed++
			sendSSE(w, flusher, map[string]any{
				"type": "failed", "index": idx, "total": total,
				"command": item.Command, "error": genErr.Error(),
			})
			continue
		}

		created++
		rule, ruleErr := h.db.GetPendingRule(ruleID)
		conf := 0.0
		if ruleErr == nil {
			conf = rule.Confidence
		}
		sendSSE(w, flusher, map[string]any{
			"type": "success", "index": idx, "total": total,
			"command": item.Command, "rule_id": ruleID,
			"confidence": conf,
		})
	}

	sendSSE(w, flusher, map[string]any{
		"type":    "done",
		"total":   total,
		"created": created,
		"failed":  failed,
		"skipped": skipped,
		"message": fmt.Sprintf("Boost complete: %d generated, %d skipped, %d failed out of %d", created, skipped, failed, total),
	})
}

// ── HTML Templates (shared layout via baseHTML + pageFooter) ─────────────

const coverageHTML = `
<div class="page-header">
  <h1>📈 Parser Coverage Self-Check</h1>
  <p style="color:var(--text-secondary);margin:0">基于设备配置自动推导应支持的命令列表，检查解析器覆盖情况</p>
</div>

{{if eq .TotalDevices 0}}
<div class="empty-state">
  <h3>暂无覆盖检查数据</h3>
  <p>使用 <code>nethelper watch</code> 导入包含 <code>display current-configuration</code> 的日志后，系统会自动进行覆盖检查。</p>
</div>
{{else}}
<div class="cov-summary-cards">
  <div class="card cov-summary-card">
    <div class="cov-num cov-num-accent">{{.TotalDevices}}</div>
    <div class="cov-label">已检查设备</div>
  </div>
  <div class="card cov-summary-card">
    <div class="cov-num cov-num-success">{{.FullyCovered}}</div>
    <div class="cov-label">完全覆盖</div>
  </div>
  <div class="card cov-summary-card">
    <div class="cov-num cov-num-warning">{{.PartialCovered}}</div>
    <div class="cov-label">部分覆盖</div>
  </div>
  <div class="card cov-summary-card">
    <div class="cov-num" style="color:var(--text-primary)">{{printf "%.1f" .AvgPct}}%</div>
    <div class="cov-label">平均覆盖率</div>
  </div>
</div>

<div class="cov-grid">
{{range .Checks}}
  <a href="/coverage/{{.DeviceID}}" class="card cov-card">
    <h3 class="cov-card-title">{{.DeviceID}}</h3>
    <span class="badge badge-vendor">{{.Vendor}}</span>
    <div class="cov-bar">
      <div class="cov-bar-fill {{if ge .CoveragePct 80.0}}cov-fill-high{{else if ge .CoveragePct 50.0}}cov-fill-mid{{else}}cov-fill-low{{end}}"
           style="width: {{printf "%.1f" .CoveragePct}}%"></div>
    </div>
    <div class="cov-stats">
      <span>{{.CoveredCount}} / {{.TotalCount}} 命令已覆盖</span>
      <span><strong>{{printf "%.1f" .CoveragePct}}%</strong></span>
    </div>
  </a>
{{end}}
</div>
{{end}}
`

const coverageDetailHTML = `
<div class="page-header" style="flex-direction:column;align-items:stretch;">
  <div style="display:flex;justify-content:space-between;align-items:center;">
    <div>
      <h1 style="display:inline;margin-right:12px;">{{.Check.DeviceID}}</h1>
      <span class="badge badge-vendor">{{.Check.Vendor}}</span>
      <span style="color:var(--text-muted);margin-left:8px;font-size:0.85rem;">检查时间: {{.Check.CheckedAt.Format "2006-01-02 15:04:05"}}</span>
    </div>
    <div style="text-align:right;">
      <div class="cov-pct {{if ge .Check.CoveragePct 80.0}}cov-pct-high{{else if ge .Check.CoveragePct 50.0}}cov-pct-mid{{else}}cov-pct-low{{end}}">
        {{printf "%.1f" .Check.CoveragePct}}%
      </div>
      <div style="color:var(--text-secondary);font-size:0.85rem;">{{.Check.CoveredCount}} / {{.Check.TotalCount}} 命令已覆盖</div>
    </div>
  </div>
</div>

<div class="actions" style="margin-bottom:16px;display:flex;gap:8px;flex-wrap:wrap;">
  <form method="POST" action="/api/coverage/recheck" style="display:inline;">
    <input type="hidden" name="device_id" value="{{.Check.DeviceID}}">
    <button type="submit" class="btn btn-primary btn-sm">🔄 重新检查</button>
  </form>
  <a href="/api/coverage/ssh?device_id={{.Check.DeviceID}}" class="btn btn-sm" download>📋 导出未覆盖命令 (SSH)</a>
  <button class="btn btn-primary btn-sm" id="btn-cov-boost" onclick="coverageBoostAll('{{.Check.DeviceID}}', '{{.Check.Vendor}}')">
    ⚡ 一键提高覆盖率
  </button>
  <a href="/coverage" class="btn btn-sm">← 返回列表</a>
</div>

<div id="cov-boost-progress" style="display:none;margin-bottom:16px;" class="card" style="padding:12px 16px;">
  <div style="display:flex;align-items:center;gap:8px;">
    <span class="spinner"></span>
    <span id="cov-boost-status" style="font-size:0.85rem;color:var(--accent)">正在生成规则...</span>
  </div>
  <div id="cov-boost-log" style="margin-top:8px;max-height:200px;overflow-y:auto;font-size:0.82rem;"></div>
</div>

{{range .Groups}}
<div class="card" style="margin-bottom:16px;">
  <div style="display:flex;justify-content:space-between;align-items:center;padding:12px 16px;border-bottom:1px solid var(--border);">
    <h3 style="margin:0;text-transform:capitalize;font-size:0.95rem;">{{.Name}}</h3>
    <span style="font-size:0.85rem;color:var(--text-secondary);">{{.Covered}}/{{.Total}} 覆盖</span>
  </div>
  <table class="data-table">
    <thead>
      <tr>
        <th style="width:35%">命令</th>
        <th style="width:8%">优先级</th>
        <th style="width:10%">状态</th>
        <th style="width:12%">解析类型</th>
        <th>推导原因</th>
        <th style="width:10%">操作</th>
      </tr>
    </thead>
    <tbody>
    {{range .Items}}
      <tr id="cov-row-{{.Command}}" class="{{if eq .Status "uncovered"}}cov-row-uncovered{{end}}">
        <td><code style="font-weight:600;">{{.Command}}</code></td>
        <td><span class="cov-priority-{{.Priority}}">{{.Priority}}</span></td>
        <td>
          {{if eq .Status "covered"}}
            <span style="color:var(--success);font-weight:600;">✅ 已覆盖</span>
          {{else}}
            <span style="color:var(--danger);font-weight:600;">❌ 未覆盖</span>
          {{end}}
        </td>
        <td>
          {{if ne .CmdType "unknown"}}
            <span class="badge" style="background:var(--accent-bg);color:var(--accent);">{{.CmdType}}</span>
          {{else}}
            <span style="color:var(--text-muted)">—</span>
          {{end}}
        </td>
        <td style="color:var(--text-secondary);font-size:0.85rem;">{{.Reason}}</td>
        <td>
          {{if eq .Status "uncovered"}}
            <button class="btn btn-primary btn-sm cov-gen-btn" onclick="coverageGenOne(this, '{{.Command}}')" title="跳转到 Unknown 页面生成规则">
              🔬 生成
            </button>
          {{end}}
        </td>
      </tr>
    {{end}}
    </tbody>
  </table>
</div>
{{end}}
`
