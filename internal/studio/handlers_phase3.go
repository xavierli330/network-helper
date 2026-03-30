package studio

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/xavierli/nethelper/internal/store"
)

// ══════════════════════════════════════════════════════════════════════════
// Phase 3A: Vendor Hostname Hints — Page + API Handlers
// ══════════════════════════════════════════════════════════════════════════

func (h *handlers) vendorHintsPage(w http.ResponseWriter, r *http.Request) {
	pd := h.newPageData("vendor-hints")
	hints, _ := h.db.ListVendorHints()
	tmpl := template.Must(template.New("vendor-hints").Funcs(funcMap).Parse(baseHTML + vendorHintsHTML))
	tmpl.Execute(w, struct {
		pageData
		Hints []store.VendorHint
	}{pageData: pd, Hints: hints})
}

// apiVendorHints handles GET (list) and POST (create) on /api/vendor-hints.
func (h *handlers) apiVendorHints(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		hints, err := h.db.ListVendorHints()
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(hints)

	case "POST":
		var hint store.VendorHint
		if err := json.NewDecoder(r.Body).Decode(&hint); err != nil {
			jsonError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		hint.Keyword = strings.TrimSpace(hint.Keyword)
		hint.Vendor = strings.TrimSpace(strings.ToLower(hint.Vendor))
		if hint.Keyword == "" || hint.Vendor == "" {
			jsonError(w, "keyword and vendor are required", 400)
			return
		}
		id, err := h.db.CreateVendorHint(hint)
		if err != nil {
			jsonError(w, "create failed: "+err.Error(), 500)
			return
		}
		hint.ID = id
		// Hot-reload cache
		h.hintCache.Reload(h.db)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(hint)

	default:
		http.Error(w, "method not allowed", 405)
	}
}

// apiVendorHintDispatch handles PUT and DELETE on /api/vendor-hints/{id}.
func (h *handlers) apiVendorHintDispatch(w http.ResponseWriter, r *http.Request) {
	// Extract ID from URL path: /api/vendor-hints/123
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/vendor-hints/"), "/")
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}

	switch r.Method {
	case "PUT":
		var hint store.VendorHint
		if err := json.NewDecoder(r.Body).Decode(&hint); err != nil {
			jsonError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		hint.ID = id
		hint.Keyword = strings.TrimSpace(hint.Keyword)
		hint.Vendor = strings.TrimSpace(strings.ToLower(hint.Vendor))
		if hint.Keyword == "" || hint.Vendor == "" {
			jsonError(w, "keyword and vendor are required", 400)
			return
		}
		if err := h.db.UpdateVendorHint(hint); err != nil {
			jsonError(w, "update failed: "+err.Error(), 500)
			return
		}
		// Hot-reload cache
		h.hintCache.Reload(h.db)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(hint)

	case "DELETE":
		if err := h.db.DeleteVendorHint(id); err != nil {
			jsonError(w, "delete failed: "+err.Error(), 500)
			return
		}
		// Hot-reload cache
		h.hintCache.Reload(h.db)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	default:
		http.Error(w, "method not allowed", 405)
	}
}

// ══════════════════════════════════════════════════════════════════════════
// Phase 3C: Classification Patterns — API Handlers
// ══════════════════════════════════════════════════════════════════════════

// apiPatterns handles GET (list) and POST (create) on /api/patterns.
func (h *handlers) apiPatterns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		vendor := r.URL.Query().Get("vendor")
		patterns, err := h.db.ListClassificationPatterns(vendor)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(patterns)

	case "POST":
		var p store.ClassificationPattern
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			jsonError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		p.Vendor = strings.TrimSpace(strings.ToLower(p.Vendor))
		p.Prefix = strings.TrimSpace(p.Prefix)
		p.CmdType = strings.TrimSpace(strings.ToLower(p.CmdType))
		if p.Vendor == "" || p.Prefix == "" || p.CmdType == "" {
			jsonError(w, "vendor, prefix, and cmd_type are required", 400)
			return
		}
		id, err := h.db.CreateClassificationPattern(p)
		if err != nil {
			jsonError(w, "create failed: "+err.Error(), 500)
			return
		}
		p.ID = id
		// Hot-reload cache
		h.patternCache.Reload(h.db)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)

	default:
		http.Error(w, "method not allowed", 405)
	}
}

// apiPatternDispatch handles PUT and DELETE on /api/patterns/{id}.
func (h *handlers) apiPatternDispatch(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/patterns/"), "/")
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}

	switch r.Method {
	case "PUT":
		var p store.ClassificationPattern
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			jsonError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		p.ID = id
		p.Vendor = strings.TrimSpace(strings.ToLower(p.Vendor))
		p.Prefix = strings.TrimSpace(p.Prefix)
		p.CmdType = strings.TrimSpace(strings.ToLower(p.CmdType))
		if p.Vendor == "" || p.Prefix == "" || p.CmdType == "" {
			jsonError(w, "vendor, prefix, and cmd_type are required", 400)
			return
		}
		if err := h.db.UpdateClassificationPattern(p); err != nil {
			jsonError(w, "update failed: "+err.Error(), 500)
			return
		}
		// Hot-reload cache
		h.patternCache.Reload(h.db)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)

	case "DELETE":
		if err := h.db.DeleteClassificationPattern(id); err != nil {
			jsonError(w, "delete failed: "+err.Error(), 500)
			return
		}
		// Hot-reload cache
		h.patternCache.Reload(h.db)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	default:
		http.Error(w, "method not allowed", 405)
	}
}

// ══════════════════════════════════════════════════════════════════════════
// Vendor Hints Page HTML Template
// ══════════════════════════════════════════════════════════════════════════

const vendorHintsHTML = `
{{define "title"}}Vendor Hostname Hints{{end}}
{{define "content"}}
<div class="page-header">
  <h2>🏷️ Vendor Hostname Hints</h2>
  <p class="subtitle">Map hostname keywords to vendors for automatic device detection.<br>
    When a hostname contains a keyword (case-insensitive), the device is assigned to the corresponding vendor.<br>
    <em>Example:</em> keyword <code>H9850</code> → vendor <code>h3c</code> means hostname <code>&lt;BJ-Core-H9850-1&gt;</code> is detected as H3C.</p>
</div>

<div class="card" style="margin-bottom:1.5rem;">
  <h3 style="margin-top:0;">Add New Hint</h3>
  <form id="hint-form" style="display:flex;gap:0.75rem;align-items:flex-end;flex-wrap:wrap;">
    <div>
      <label style="display:block;font-size:0.8rem;margin-bottom:0.25rem;">Hostname Keyword</label>
      <input type="text" id="hint-keyword" placeholder="e.g. H9850, NE40, S12700" style="padding:0.5rem 0.75rem;border:1px solid var(--border);border-radius:6px;background:var(--bg-input);color:var(--text-primary);font-size:0.9rem;" required>
    </div>
    <div>
      <label style="display:block;font-size:0.8rem;margin-bottom:0.25rem;">Vendor</label>
      <select id="hint-vendor" style="padding:0.5rem 0.75rem;border:1px solid var(--border);border-radius:6px;background:var(--bg-input);color:var(--text-primary);font-size:0.9rem;">
        <option value="huawei">huawei</option>
        <option value="h3c">h3c</option>
        <option value="cisco">cisco</option>
        <option value="juniper">juniper</option>
      </select>
    </div>
    <div>
      <label style="display:block;font-size:0.8rem;margin-bottom:0.25rem;">Note (optional)</label>
      <input type="text" id="hint-note" placeholder="e.g. H3C S9850 series switch" style="padding:0.5rem 0.75rem;border:1px solid var(--border);border-radius:6px;background:var(--bg-input);color:var(--text-primary);font-size:0.9rem;">
    </div>
    <button type="submit" class="btn btn-primary" style="height:fit-content;">Add Hint</button>
  </form>
</div>

<div class="card">
  <table class="data-table" style="width:100%;">
    <thead>
      <tr>
        <th>Keyword</th>
        <th>Vendor</th>
        <th>Note</th>
        <th style="width:120px;">Actions</th>
      </tr>
    </thead>
    <tbody id="hints-tbody">
      {{range .Hints}}
      <tr data-id="{{.ID}}">
        <td><code>{{.Keyword}}</code></td>
        <td><span class="badge badge-vendor">{{.Vendor}}</span></td>
        <td>{{.Note}}</td>
        <td>
          <button class="btn btn-sm btn-danger hint-delete" data-id="{{.ID}}">Delete</button>
        </td>
      </tr>
      {{else}}
      <tr><td colspan="4" style="text-align:center;color:var(--text-muted);padding:2rem;">No hints configured yet. Add one above.</td></tr>
      {{end}}
    </tbody>
  </table>
</div>

<script>
document.getElementById('hint-form').addEventListener('submit', async (e) => {
  e.preventDefault();
  const keyword = document.getElementById('hint-keyword').value.trim();
  const vendor = document.getElementById('hint-vendor').value;
  const note = document.getElementById('hint-note').value.trim();
  if (!keyword) return;

  try {
    const resp = await fetch('/api/vendor-hints', {
      method: 'POST',
      headers: {'Content-Type':'application/json'},
      body: JSON.stringify({keyword, vendor, note})
    });
    if (!resp.ok) {
      const err = await resp.json();
      showToast(err.error || 'Failed to create hint', 'error');
      return;
    }
    showToast('Hint added successfully', 'success');
    setTimeout(() => location.reload(), 500);
  } catch(e) {
    showToast('Network error: ' + e.message, 'error');
  }
});

document.querySelectorAll('.hint-delete').forEach(btn => {
  btn.addEventListener('click', async () => {
    const id = btn.dataset.id;
    if (!confirm('Delete this hint?')) return;
    try {
      const resp = await fetch('/api/vendor-hints/' + id, {method:'DELETE'});
      if (!resp.ok) {
        showToast('Delete failed', 'error');
        return;
      }
      showToast('Hint deleted', 'success');
      btn.closest('tr').remove();
    } catch(e) {
      showToast('Network error: ' + e.message, 'error');
    }
  });
});
</script>
{{end}}
`

// ══════════════════════════════════════════════════════════════════════════
// Phase 3D: Field Schemas — API Handlers
// ══════════════════════════════════════════════════════════════════════════

// apiFieldSchemas handles GET (list) and POST (upsert) on /api/field-schemas.
func (h *handlers) apiFieldSchemas(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		vendor := r.URL.Query().Get("vendor")
		cmdType := r.URL.Query().Get("cmd_type")
		schemas, err := h.db.ListFieldSchemas(vendor, cmdType)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(schemas)

	case "POST":
		var f store.FieldSchema
		if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
			jsonError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		f.Vendor = strings.TrimSpace(strings.ToLower(f.Vendor))
		f.CmdType = strings.TrimSpace(strings.ToLower(f.CmdType))
		f.FieldName = strings.TrimSpace(f.FieldName)
		if f.Vendor == "" || f.CmdType == "" || f.FieldName == "" {
			jsonError(w, "vendor, cmd_type, and field_name are required", 400)
			return
		}
		id, err := h.db.UpsertFieldSchema(f)
		if err != nil {
			jsonError(w, "upsert failed: "+err.Error(), 500)
			return
		}
		f.ID = id
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(f)

	default:
		http.Error(w, "method not allowed", 405)
	}
}

// apiFieldSchemaDispatch handles PUT and DELETE on /api/field-schemas/{id}.
func (h *handlers) apiFieldSchemaDispatch(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/field-schemas/"), "/")
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}

	switch r.Method {
	case "PUT":
		var f store.FieldSchema
		if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
			jsonError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		f.ID = id
		if err := h.db.UpdateFieldSchema(f); err != nil {
			jsonError(w, "update failed: "+err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(f)

	case "DELETE":
		if err := h.db.DeleteFieldSchema(id); err != nil {
			jsonError(w, "delete failed: "+err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	default:
		http.Error(w, "method not allowed", 405)
	}
}

// apiFieldSchemasSync extracts fields from DSL and syncs to field_schemas.
// POST /api/field-schemas/sync with {vendor, cmd_type, dsl_text}
func (h *handlers) apiFieldSchemasSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	var req struct {
		Vendor  string `json:"vendor"`
		CmdType string `json:"cmd_type"`
		DSLText string `json:"dsl_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON: "+err.Error(), 400)
		return
	}
	req.Vendor = strings.TrimSpace(strings.ToLower(req.Vendor))
	req.CmdType = strings.TrimSpace(strings.ToLower(req.CmdType))
	if req.Vendor == "" || req.CmdType == "" || req.DSLText == "" {
		jsonError(w, "vendor, cmd_type, and dsl_text are required", 400)
		return
	}
	count, err := h.db.SyncFieldSchemasFromDSL(req.Vendor, req.CmdType, req.DSLText)
	if err != nil {
		jsonError(w, "sync failed: "+err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"new_fields": count,
		"fields":     store.ExtractNamedGroups(req.DSLText),
	})
}

// ══════════════════════════════════════════════════════════════════════════
// Phase 3B: Runtime Rules — API Handlers
// ══════════════════════════════════════════════════════════════════════════

// apiRuntimeRules handles GET (list) and POST (create/upsert) on /api/runtime-rules.
func (h *handlers) apiRuntimeRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		vendor := r.URL.Query().Get("vendor")
		rules, err := h.db.ListRuntimeRules(vendor, false)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rules)

	case "POST":
		var rr store.RuntimeRule
		if err := json.NewDecoder(r.Body).Decode(&rr); err != nil {
			jsonError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		rr.Vendor = strings.TrimSpace(strings.ToLower(rr.Vendor))
		rr.CommandPattern = strings.TrimSpace(rr.CommandPattern)
		if rr.Vendor == "" || rr.CommandPattern == "" || rr.DSLText == "" {
			jsonError(w, "vendor, command_pattern, and dsl_text are required", 400)
			return
		}
		if rr.OutputType == "" {
			rr.OutputType = "pipeline"
		}
		if rr.CmdType == "" {
			rr.CmdType = "unknown"
		}
		id, err := h.db.UpsertRuntimeRule(rr)
		if err != nil {
			jsonError(w, "upsert failed: "+err.Error(), 500)
			return
		}
		rr.ID = id
		// Sync field schemas
		h.db.SyncFieldSchemasFromDSL(rr.Vendor, rr.CmdType, rr.DSLText)
		// Hot-reload
		if h.runtimeReg != nil {
			h.runtimeReg.Reload(h.db)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rr)

	default:
		http.Error(w, "method not allowed", 405)
	}
}

// apiRuntimeRuleDispatch handles GET/PUT/DELETE on /api/runtime-rules/{id}.
func (h *handlers) apiRuntimeRuleDispatch(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/runtime-rules/"), "/")
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}

	switch r.Method {
	case "GET":
		rr, err := h.db.GetRuntimeRule(id)
		if err != nil {
			jsonError(w, "not found", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rr)

	case "PUT":
		var rr store.RuntimeRule
		if err := json.NewDecoder(r.Body).Decode(&rr); err != nil {
			jsonError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		rr.ID = id
		if err := h.db.UpdateRuntimeRule(rr); err != nil {
			jsonError(w, "update failed: "+err.Error(), 500)
			return
		}
		// Hot-reload
		if h.runtimeReg != nil {
			h.runtimeReg.Reload(h.db)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rr)

	case "DELETE":
		if err := h.db.DeleteRuntimeRule(id); err != nil {
			jsonError(w, "delete failed: "+err.Error(), 500)
			return
		}
		// Hot-reload
		if h.runtimeReg != nil {
			h.runtimeReg.Reload(h.db)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	default:
		http.Error(w, "method not allowed", 405)
	}
}

// ══════════════════════════════════════════════════════════════════════════
// Phase 3E: Vendor Reassignment — API Handlers
// ══════════════════════════════════════════════════════════════════════════

// apiVendorReassign changes the vendor on unknown_outputs, pending_rules,
// or runtime_rules. POST /api/vendor-reassign with JSON body.
func (h *handlers) apiVendorReassign(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}

	var req struct {
		Table     string `json:"table"`      // "unknown_outputs", "pending_rules", "runtime_rules"
		ID        int    `json:"id"`
		NewVendor string `json:"new_vendor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON: "+err.Error(), 400)
		return
	}
	req.NewVendor = strings.TrimSpace(strings.ToLower(req.NewVendor))
	if req.NewVendor == "" || req.ID == 0 {
		jsonError(w, "id and new_vendor are required", 400)
		return
	}

	var err error
	switch req.Table {
	case "unknown_outputs":
		_, err = h.db.Exec(`UPDATE unknown_outputs SET vendor=? WHERE id=?`, req.NewVendor, req.ID)
	case "pending_rules":
		_, err = h.db.Exec(`UPDATE pending_rules SET vendor=? WHERE id=?`, req.NewVendor, req.ID)
	case "runtime_rules":
		err = h.db.UpdateRuntimeRuleVendor(req.ID, req.NewVendor)
		if err == nil && h.runtimeReg != nil {
			h.runtimeReg.Reload(h.db)
		}
	default:
		jsonError(w, "invalid table: must be unknown_outputs, pending_rules, or runtime_rules", 400)
		return
	}

	if err != nil {
		jsonError(w, "update failed: "+err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "vendor": req.NewVendor})
}
