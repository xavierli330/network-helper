package studio

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/xavierli/nethelper/internal/parser"
	"github.com/xavierli/nethelper/internal/store"
)

// ── Page: Batch Import ───────────────────────────────────────────────────

func (h *handlers) batchImportPage(w http.ResponseWriter, r *http.Request) {
	pd := h.newPageData("import")
	history, _ := h.db.ListImportHistory(20)
	tmpl := template.Must(template.New("import").Funcs(funcMap).Parse(baseHTML + batchImportHTML))
	tmpl.Execute(w, struct {
		pageData
		History []store.ImportHistory
	}{pageData: pd, History: history})
}

// ── API: Analyze Log ─────────────────────────────────────────────────────

// apiAnalyzeLogRequest is the JSON request body for manual text submission.
type apiAnalyzeLogRequest struct {
	Content string `json:"content"`
	Vendor  string `json:"vendor"`
}

func (h *handlers) apiAnalyzeLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}

	var raw []byte
	var vendor string

	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		// File upload mode
		r.ParseMultipartForm(50 << 20) // 50MB limit
		file, _, err := r.FormFile("file")
		if err != nil {
			jsonError(w, "file upload failed: "+err.Error(), 400)
			return
		}
		defer file.Close()
		raw, err = io.ReadAll(file)
		if err != nil {
			jsonError(w, "read file failed: "+err.Error(), 500)
			return
		}
		vendor = r.FormValue("vendor")
	} else {
		// JSON body mode (for pasted text)
		var req apiAnalyzeLogRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		raw = []byte(req.Content)
		vendor = req.Vendor
	}

	if len(raw) == 0 {
		jsonError(w, "empty content", 400)
		return
	}

	// Use the field registry's underlying parser registry for analysis
	var reg *parser.Registry
	if h.fieldReg != nil {
		reg = h.fieldReg.Reg()
	}
	if reg == nil {
		jsonError(w, "parser registry not available", 503)
		return
	}

	analysis, err := parser.AnalyzeSessionLog(reg, string(raw))
	if err != nil {
		jsonError(w, "analysis failed: "+err.Error(), 500)
		return
	}

	// Phase 3A: Apply vendor hostname hints to override detected vendors.
	// This is critical for devices like H3C that share prompt format with Huawei.
	if h.hintCache != nil {
		for i, cmd := range analysis.Commands {
			if hintVendor, ok := h.hintCache.Lookup(cmd.Hostname); ok && hintVendor != cmd.Vendor {
				analysis.Commands[i].Vendor = hintVendor
			}
		}
	}

	// If vendor was specified, filter to only that vendor's commands
	if vendor != "" && vendor != "auto" {
		var filtered []parser.AnalyzedCommand
		for _, cmd := range analysis.Commands {
			if cmd.Vendor == vendor {
				filtered = append(filtered, cmd)
			}
		}
		analysis.Commands = filtered
		analysis.Total = len(filtered)
		analysis.Vendor = vendor
	}

	// Check existing rules in DB to update HasExistingRule
	for i, cmd := range analysis.Commands {
		_, err := h.db.GetPendingRuleByCommandNorm(cmd.Vendor, cmd.Pattern)
		if err == nil {
			analysis.Commands[i].HasExistingRule = true
			analysis.Commands[i].Selected = false
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(analysis)
}

// ── API: Batch Generate Rules ────────────────────────────────────────────

type batchGenerateRequest struct {
	Vendor   string              `json:"vendor"`
	Commands []batchCommandInput `json:"commands"`
}

type batchCommandInput struct {
	Pattern      string `json:"pattern"`
	RawCommand   string `json:"raw_command"`
	SampleOutput string `json:"sample_output"`
}

func (h *handlers) apiGenerateBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	if h.eng == nil {
		jsonError(w, "discovery engine not configured", 503)
		return
	}

	var req batchGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON: "+err.Error(), 400)
		return
	}

	if len(req.Commands) == 0 {
		jsonError(w, "no commands provided", 400)
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
	w.Header().Set("X-Accel-Buffering", "no")

	bgCtx, bgCancel := context.WithTimeout(context.Background(), 20*time.Minute)
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

	total := len(req.Commands)
	created := 0
	failed := 0

	// Emit start event
	sendSSE(w, flusher, map[string]any{
		"type": "start", "total": total,
	})

	for i, cmdInput := range req.Commands {
		if bgCtx.Err() != nil {
			break
		}

		idx := i + 1
		vendor := req.Vendor
		pattern := cmdInput.Pattern
		sampleOutput := cmdInput.SampleOutput

		// Emit pending event
		sendSSE(w, flusher, map[string]any{
			"type": "pending", "index": idx, "total": total,
			"command": pattern, "vendor": vendor,
		})

		// Check if a rule already exists for this pattern
		existing, err := h.db.GetPendingRuleByCommandNorm(vendor, pattern)
		if err == nil {
			sendSSE(w, flusher, map[string]any{
				"type": "skipped", "index": idx, "total": total,
				"command": pattern, "rule_id": existing.ID,
				"message": "rule already exists",
			})
			continue
		}

		// First, upsert into unknown_outputs so we have a record
		hash := hashForBatch(sampleOutput)
		_ = h.db.UpsertUnknownOutput(store.UnknownOutput{
			DeviceID:    "", // no device ID for manual import
			Vendor:      vendor,
			CommandRaw:  cmdInput.RawCommand,
			CommandNorm: pattern,
			RawOutput:   sampleOutput,
			ContentHash: hash,
		})

		// Find the unknown output we just created/updated
		unknowns, _ := h.db.ListUnknownOutputs(vendor, "new", 1000)
		var unknownID int
		for _, u := range unknowns {
			if u.CommandNorm == pattern {
				unknownID = u.ID
				break
			}
		}

		if unknownID == 0 {
			// Couldn't find it, try with other statuses
			all, _ := h.db.ListUnknownOutputs(vendor, "", 1000)
			for _, u := range all {
				if u.CommandNorm == pattern {
					unknownID = u.ID
					// Reset to new so GenerateForUnknown can pick it up
					h.db.UpdateUnknownOutputStatus(vendor, pattern, "new")
					break
				}
			}
		}

		if unknownID == 0 {
			failed++
			sendSSE(w, flusher, map[string]any{
				"type": "failed", "index": idx, "total": total,
				"command": pattern,
				"error":   "could not create unknown output record",
			})
			continue
		}

		// Generate rule using discovery engine (synchronous per-command)
		ruleID, genErr := h.eng.GenerateForUnknown(bgCtx, unknownID)
		if genErr != nil {
			failed++
			sendSSE(w, flusher, map[string]any{
				"type": "failed", "index": idx, "total": total,
				"command": pattern,
				"error":   genErr.Error(),
			})
			continue
		}

		created++

		// Get rule details for confidence info
		rule, _ := h.db.GetPendingRule(ruleID)
		testCount, _ := h.db.CountRuleTestCases(ruleID)

		sendSSE(w, flusher, map[string]any{
			"type": "success", "index": idx, "total": total,
			"command":     pattern,
			"rule_id":     ruleID,
			"confidence":  rule.Confidence,
			"status":      rule.Status,
			"test_cases":  testCount,
		})
	}

	// Emit done event
	sendSSE(w, flusher, map[string]any{
		"type": "done", "total": total, "created": created, "failed": failed,
		"message": fmt.Sprintf("Batch complete: %d created, %d failed, %d total", created, failed, total),
	})

	// Record import history
	h.db.CreateImportHistory(store.ImportHistory{
		Filename:         "batch_import",
		Vendor:           req.Vendor,
		SourceType:       "file",
		TotalCommands:    total,
		SelectedCommands: total,
		CreatedCount:     created,
		FailedCount:      failed,
		SkippedCount:     total - created - failed,
	})
}

func sendSSE(w http.ResponseWriter, flusher http.Flusher, data map[string]any) {
	jsonData, _ := json.Marshal(data)
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
	flusher.Flush()
}

// hashForBatch returns the first 16 hex chars of SHA-256 for dedup,
// same format as collector.go's hashContent.
func hashForBatch(s string) string {
	sum := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", sum[:8])
}

// ── API: Manual Add Command ───────────────────────────────────────────────

type apiManualAddRequest struct {
	Vendor  string `json:"vendor"`
	Command string `json:"command"`
	Output  string `json:"output"`
}

func (h *handlers) apiManualAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}

	var req apiManualAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON: "+err.Error(), 400)
		return
	}

	if req.Vendor == "" {
		jsonError(w, "vendor is required", 400)
		return
	}
	if req.Command == "" {
		jsonError(w, "command is required", 400)
		return
	}
	if strings.TrimSpace(req.Output) == "" {
		jsonError(w, "output is required (need sample output for rule generation)", 400)
		return
	}

	// Normalise command and strip args to get pattern
	norm := parser.NormaliseCommand(req.Vendor, req.Command)
	pattern := parser.StripArgs(norm)

	// Check if a rule already exists for this pattern
	existing, err := h.db.GetPendingRuleByCommandNorm(req.Vendor, pattern)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":  "exists",
			"rule_id": existing.ID,
			"pattern": pattern,
			"message": fmt.Sprintf("Rule already exists for %s (rule #%d)", pattern, existing.ID),
		})
		return
	}

	// Upsert into unknown_outputs
	hash := hashForBatch(req.Output)
	upsertErr := h.db.UpsertUnknownOutput(store.UnknownOutput{
		DeviceID:    "",
		Vendor:      req.Vendor,
		CommandRaw:  req.Command,
		CommandNorm: pattern,
		RawOutput:   req.Output,
		ContentHash: hash,
	})
	if upsertErr != nil {
		jsonError(w, "failed to save: "+upsertErr.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":     "added",
		"vendor":     req.Vendor,
		"command":    req.Command,
		"pattern":    pattern,
		"normalized": norm,
		"message":    fmt.Sprintf("Added %s → %s to unknown outputs", req.Command, pattern),
	})
}

// ── Batch Import HTML Template ───────────────────────────────────────────

const batchImportHTML = `
<div class="page-header">
  <h1>📥 Batch Import</h1>
  <div class="actions">
    <span style="font-size:0.82rem;color:var(--text-secondary)">Upload a session log → AI identifies commands → You review → Batch generate rules</span>
  </div>
</div>

<!-- Step 1: Upload -->
<div class="card" id="step-upload">
  <div class="card-header">
    <h3><span class="step-indicator" style="margin-right:8px">1</span>Upload Session Log</h3>
  </div>
  <div class="card-body">
    <div style="display:grid;grid-template-columns:1fr 200px;gap:16px;align-items:start">
      <div>
        <div id="drop-zone" class="drop-zone" ondrop="handleDrop(event)" ondragover="event.preventDefault();this.classList.add('drag-over')" ondragleave="this.classList.remove('drag-over')">
          <div style="text-align:center">
            <div style="font-size:2rem;margin-bottom:8px">📂</div>
            <div style="font-weight:600;margin-bottom:4px">Drop session log file here</div>
            <div style="font-size:0.82rem;color:var(--text-muted)">or click to browse · .log, .txt supported · Max 50MB</div>
            <input type="file" id="file-input" accept=".log,.txt,.text" style="display:none" onchange="handleFileSelect(this)">
          </div>
        </div>
        <div style="text-align:center;margin:12px 0;font-size:0.82rem;color:var(--text-muted)">— or paste text directly —</div>
        <textarea id="paste-area" placeholder="Paste session log content here..." style="min-height:120px;font-family:var(--font-mono);font-size:0.82rem"></textarea>
      </div>
      <div>
        <label style="font-size:0.82rem;color:var(--text-secondary)">Vendor</label>
        <select id="import-vendor" style="width:100%">
          <option value="auto">Auto-detect</option>
          <option value="huawei">Huawei</option>
          <option value="h3c">H3C</option>
          <option value="cisco">Cisco</option>
          <option value="juniper">Juniper</option>
        </select>
        <div style="margin-top:16px">
          <button class="btn btn-primary" style="width:100%" onclick="analyzeLog()" id="btn-analyze">
            🔍 Analyze
          </button>
        </div>
        <div id="file-info" style="margin-top:12px;font-size:0.78rem;color:var(--text-muted)"></div>
      </div>
    </div>
  </div>
</div>

<!-- Manual Add: Single command + output entry -->
<div class="card" id="step-manual-add">
  <div class="card-header">
    <h3>➕ Manual Add</h3>
    <span style="font-size:0.82rem;color:var(--text-muted)">Add a single command and its output directly</span>
  </div>
  <div class="card-body" id="manual-add-body" style="display:none">
    <div style="display:grid;grid-template-columns:160px 1fr;gap:12px;margin-bottom:12px">
      <div>
        <label style="font-size:0.82rem">Vendor</label>
        <select id="manual-vendor" style="width:100%">
          <option value="huawei">Huawei</option>
          <option value="h3c">H3C</option>
          <option value="cisco">Cisco</option>
          <option value="juniper">Juniper</option>
        </select>
      </div>
      <div>
        <label style="font-size:0.82rem">Command</label>
        <input type="text" id="manual-command" placeholder="e.g. display bgp peer verbose" style="font-family:var(--font-mono);font-size:0.85rem">
      </div>
    </div>
    <div style="margin-bottom:12px">
      <label style="font-size:0.82rem">Output <span style="color:var(--text-muted)">(paste the full CLI output for this command)</span></label>
      <textarea id="manual-output" placeholder="Paste the device CLI output here..." style="min-height:180px;font-family:var(--font-mono);font-size:0.82rem"></textarea>
    </div>
    <div style="display:flex;align-items:center;gap:12px">
      <button class="btn btn-primary" onclick="manualAddCommand()" id="btn-manual-add">➕ Add to Unknown Outputs</button>
      <span id="manual-add-result" style="font-size:0.82rem"></span>
    </div>
  </div>
  <div class="card-body" id="manual-add-toggle" style="cursor:pointer;padding:12px" onclick="toggleManualAdd()">
    <span style="font-size:0.85rem;color:var(--text-secondary)">▸ Click to expand — add a single command + output without uploading a log file</span>
  </div>
</div>

<!-- Step 2: Command Review (hidden until analyze) -->
<div class="card" id="step-review" style="display:none">
  <div class="card-header">
    <h3><span class="step-indicator" style="margin-right:8px">2</span>Review Commands</h3>
    <div class="btn-group">
      <button class="btn btn-ghost btn-sm" onclick="selectAll()">Select All</button>
      <button class="btn btn-ghost btn-sm" onclick="selectNone()">Select None</button>
      <button class="btn btn-ghost btn-sm" onclick="selectRecommended()">Select Recommended</button>
    </div>
  </div>
  <div class="card-body">
    <div id="analysis-summary" style="margin-bottom:12px;padding:10px 14px;background:var(--bg-tertiary);border-radius:var(--radius);font-size:0.82rem"></div>
    <div id="command-list"></div>
  </div>
</div>

<!-- Step 3: Generate (hidden until review) -->
<div class="card" id="step-generate" style="display:none">
  <div class="card-header">
    <h3><span class="step-indicator" style="margin-right:8px">3</span>Batch Generate Rules</h3>
    <div class="btn-group">
      <span id="selected-count" style="font-size:0.82rem;color:var(--text-secondary)">0 commands selected</span>
      <button class="btn btn-primary" id="btn-generate" onclick="batchGenerate()">
        ⚡ Generate Rules
      </button>
    </div>
  </div>
  <div class="card-body">
    <div id="generate-progress" style="display:none">
      <div id="gen-status" style="margin-bottom:12px;padding:10px 14px;background:var(--bg-tertiary);border-radius:var(--radius);font-size:0.82rem"></div>
      <div id="gen-log" style="max-height:400px;overflow-y:auto;font-family:var(--font-mono);font-size:0.78rem"></div>
    </div>
    <div id="generate-results" style="display:none"></div>
  </div>
</div>

<!-- Import History -->
{{if .History}}
<div class="card">
  <div class="card-header">
    <h3>📜 Import History</h3>
    <span style="font-size:0.82rem;color:var(--text-muted)">Recent batch imports</span>
  </div>
  <div class="card-body">
    <table class="data-table">
      <tr><th>Time</th><th>Source</th><th>Vendor</th><th>Total</th><th>Selected</th><th>Created</th><th>Failed</th><th>Skipped</th></tr>
      {{range .History}}
      <tr>
        <td style="font-size:0.8rem;color:var(--text-muted)">{{.ImportedAt.Format "01-02 15:04"}}</td>
        <td><span class="tag">{{.SourceType}}</span></td>
        <td><span class="badge badge-vendor">{{.Vendor}}</span></td>
        <td>{{.TotalCommands}}</td>
        <td>{{.SelectedCommands}}</td>
        <td style="color:var(--success)">{{.CreatedCount}}</td>
        <td style="color:var(--danger)">{{.FailedCount}}</td>
        <td style="color:var(--text-muted)">{{.SkippedCount}}</td>
      </tr>
      {{end}}
    </table>
  </div>
</div>
{{end}}
` + pageFooter
