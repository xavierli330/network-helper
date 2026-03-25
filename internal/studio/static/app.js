/* ═══════════════════════════════════════════════════════════════════════════
   Rule Studio — App JavaScript
   Toast system, interaction helpers, result formatting, WebSocket
   ═══════════════════════════════════════════════════════════════════════════ */

// ── Toast System ──────────────────────────────────────────────────────────
const Toast = {
  show(message, type = 'info', duration = 4000) {
    let container = document.getElementById('toast-container');
    if (!container) {
      container = document.createElement('div');
      container.id = 'toast-container';
      document.body.appendChild(container);
    }
    const toast = document.createElement('div');
    toast.className = `toast toast-${type}`;
    const icons = { success: '✓', error: '✕', info: 'ℹ', warning: '⚠' };
    toast.innerHTML = `<span>${icons[type] || 'ℹ'}</span><span>${message}</span>`;
    container.appendChild(toast);
    setTimeout(() => {
      toast.classList.add('fade-out');
      setTimeout(() => toast.remove(), 300);
    }, duration);
  },
  success(msg) { this.show(msg, 'success'); },
  error(msg) { this.show(msg, 'error', 6000); },
  info(msg) { this.show(msg, 'info'); },
  warning(msg) { this.show(msg, 'warning', 5000); },
};

// ── HTMX Event Hooks ─────────────────────────────────────────────────────
document.addEventListener('htmx:responseError', function(evt) {
  let msg = 'Request failed';
  try {
    const data = JSON.parse(evt.detail.xhr.responseText);
    if (data.error) msg = data.error;
  } catch(e) { msg = evt.detail.xhr.statusText || msg; }
  Toast.error(msg);
});

document.addEventListener('htmx:sendError', function() {
  Toast.error('Network error — server unreachable');
});

// ── Result Formatter ─────────────────────────────────────────────────────
function formatParseResult(jsonStr, targetId) {
  const target = document.getElementById(targetId);
  if (!target) return;
  try {
    const data = JSON.parse(jsonStr);
    if (data.error) {
      target.innerHTML = `<div class="toast-error" style="padding:8px;border-radius:4px">${data.error}</div>`;
      return;
    }
    // Table result (from schema engine)
    if (data.Rows && data.Rows.length > 0) {
      const keys = Object.keys(data.Rows[0]);
      let html = '<table class="result-table"><tr>';
      keys.forEach(k => html += `<th>${k}</th>`);
      html += '</tr>';
      data.Rows.forEach(row => {
        html += '<tr>';
        keys.forEach(k => {
          let val = row[k] || '';
          let cls = '';
          if (val.toLowerCase() === 'up') cls = ' class="val-up"';
          else if (val.toLowerCase() === 'down') cls = ' class="val-down"';
          html += `<td${cls}>${escHtml(val)}</td>`;
        });
        html += '</tr>';
      });
      html += '</table>';
      html += `<div style="margin-top:8px;font-size:0.78rem;color:var(--text-muted)">${data.Rows.length} row(s) parsed</div>`;
      target.innerHTML = html;
      Toast.success(`Parsed ${data.Rows.length} rows`);
      return;
    }
    // Parser tester result
    if (data.cmdType !== undefined) {
      let html = '<div class="info-grid">';
      html += `<div class="info-item"><div class="label">Command Type</div><div class="value">${data.matched ? '✓' : '✕'} ${data.cmdType}</div></div>`;
      if (data.interfaceCount) html += `<div class="info-item"><div class="label">Interfaces</div><div class="value">${data.interfaceCount}</div></div>`;
      if (data.neighborCount) html += `<div class="info-item"><div class="label">Neighbors</div><div class="value">${data.neighborCount}</div></div>`;
      if (data.rowCount) html += `<div class="info-item"><div class="label">Rows</div><div class="value">${data.rowCount}</div></div>`;
      html += '</div>';
      // Show sample data
      if (data.interfaces) html += buildResultTable(data.interfaces);
      if (data.neighbors) html += buildResultTable(data.neighbors);
      if (data.rows) html += buildResultTable(data.rows);
      target.innerHTML = html;
      return;
    }
    // Fallback: pretty JSON
    target.innerHTML = `<pre style="margin:0">${escHtml(JSON.stringify(data, null, 2))}</pre>`;
  } catch(e) {
    target.innerHTML = `<pre style="margin:0">${escHtml(jsonStr)}</pre>`;
  }
}

function buildResultTable(items) {
  if (!items || items.length === 0) return '';
  const isObject = typeof items[0] === 'object' && !Array.isArray(items[0]);
  if (!isObject) return `<pre>${escHtml(JSON.stringify(items, null, 2))}</pre>`;
  const keys = Object.keys(items[0]);
  let html = '<table class="result-table"><tr>';
  keys.forEach(k => html += `<th>${k}</th>`);
  html += '</tr>';
  items.forEach(row => {
    html += '<tr>';
    keys.forEach(k => {
      let v = row[k];
      if (v === null || v === undefined) v = '';
      else if (typeof v === 'object') v = JSON.stringify(v);
      html += `<td>${escHtml(String(v))}</td>`;
    });
    html += '</tr>';
  });
  html += '</table>';
  return html;
}

function escHtml(str) {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

// ── API Helpers ───────────────────────────────────────────────────────────
async function apiPost(url, data) {
  const formData = new URLSearchParams(data);
  const resp = await fetch(url, { method: 'POST', body: formData, headers: {'Content-Type': 'application/x-www-form-urlencoded'} });
  return resp.json();
}

// ── Sample Inputs Renderer ───────────────────────────────────────────────
// SampleInputs is stored as JSON (e.g. ["raw output text", "another sample"])
// We need to parse it and render as readable plain text blocks.
function renderSampleInputs() {
  const container = document.getElementById('sample-inputs-container');
  if (!container) return;
  const rawEl = container.querySelector('.sample-inputs-raw');
  if (!rawEl) return;
  const rawText = rawEl.textContent.trim();
  if (!rawText) {
    container.innerHTML = '<p style="color:var(--text-muted);font-size:0.85rem">No sample inputs collected during discovery.</p>';
    return;
  }
  let samples = [];
  try {
    samples = JSON.parse(rawText);
    if (!Array.isArray(samples)) samples = [String(samples)];
  } catch(e) {
    // Not JSON — treat as plain text
    samples = [rawText];
  }
  let html = '';
  samples.forEach((sample, idx) => {
    const sampleText = typeof sample === 'string' ? sample : JSON.stringify(sample, null, 2);
    const collapsed = sampleText.split('\n').length > 8;
    html += `<div class="sample-item" style="margin-bottom:8px">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:4px">
        <span style="font-size:0.78rem;color:var(--text-muted)">Sample #${idx + 1} (${sampleText.split('\\n').length} lines)</span>
        <div class="btn-group">
          <button class="btn btn-ghost btn-sm" onclick="useSampleInput(${idx})" title="Copy to test input area">↓ Use as Test Input</button>
          ${collapsed ? `<button class="btn btn-ghost btn-sm" onclick="toggleSample(this, ${idx})" title="Expand/collapse">▸ Expand</button>` : ''}
        </div>
      </div>
      <div class="output-preview sample-text" id="sample-${idx}" style="${collapsed ? 'max-height:160px;overflow:hidden' : ''}">${escHtml(sampleText)}</div>
    </div>`;
  });
  container.innerHTML = html;
  // Store samples for later use
  window._sampleInputs = samples;
}

function useSampleInput(idx) {
  const samples = window._sampleInputs || [];
  if (idx >= samples.length) return;
  const textarea = document.getElementById('input-area');
  if (textarea) {
    textarea.value = samples[idx];
    textarea.scrollIntoView({ behavior: 'smooth', block: 'center' });
    textarea.focus();
    Toast.info('Sample loaded into test input');
  }
}

function toggleSample(btn, idx) {
  const el = document.getElementById('sample-' + idx);
  if (!el) return;
  if (el.style.maxHeight) {
    el.style.maxHeight = '';
    el.style.overflow = '';
    btn.textContent = '▾ Collapse';
  } else {
    el.style.maxHeight = '160px';
    el.style.overflow = 'hidden';
    btn.textContent = '▸ Expand';
  }
}

// ── Test Case Save Handler ───────────────────────────────────────────────
// Stores last parse result for auto-filling expected
let lastParseResult = null;

async function saveTestCase(ruleId) {
  const input = document.getElementById('input-area')?.value || '';
  let expected = document.getElementById('tc-expected')?.value || '';
  const desc = document.getElementById('tc-desc')?.value || '';

  if (!input) {
    Toast.warning('Paste device output in Step 1 first');
    return;
  }
  // Auto-fill expected from last parse result if empty
  if (!expected && lastParseResult) {
    expected = JSON.stringify(lastParseResult, null, 2);
    document.getElementById('tc-expected').value = expected;
  }
  if (!expected) {
    Toast.warning('Run Parse first, or enter expected result manually');
    return;
  }
  try {
    const result = await apiPost(`/api/rule/${ruleId}/testcase`, { input, expected, description: desc });
    if (result.error) { Toast.error(result.error); return; }
    Toast.success(`Test case #${result.id} saved`);
    // Update counts
    ['tc-count', 'tc-count-header'].forEach(id => {
      const el = document.getElementById(id);
      if (el) el.textContent = parseInt(el.textContent || '0') + 1;
    });
    // Clear description
    document.getElementById('tc-desc').value = '';
    // Reload page to show new test case
    setTimeout(() => location.reload(), 500);
  } catch(e) { Toast.error('Save failed: ' + e.message); }
}

// ── Run Parse Handler ────────────────────────────────────────────────────
async function runParse(ruleId) {
  const input = document.getElementById('input-area')?.value || '';
  if (!input.trim()) { Toast.warning('Paste device output in Step 1 first'); return; }
  const btn = event?.target;
  if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spinner"></span> Parsing...'; }
  try {
    const resp = await fetch(`/api/rule/${ruleId}/test`, {
      method: 'POST', body: new URLSearchParams({ input }),
      headers: {'Content-Type': 'application/x-www-form-urlencoded'}
    });
    const text = await resp.text();
    try {
      lastParseResult = JSON.parse(text);
      // Auto-fill expected field
      const expectedEl = document.getElementById('tc-expected');
      if (expectedEl && !expectedEl.value) {
        expectedEl.value = JSON.stringify(lastParseResult, null, 2);
      }
    } catch(e) { lastParseResult = null; }
    formatParseResult(text, 'parse-result');
  } catch(e) { Toast.error('Parse failed: ' + e.message); }
  if (btn) { btn.disabled = false; btn.innerHTML = '▶ Run Parse (⌘↵)'; }
}

// ── Test Case Management ─────────────────────────────────────────────────
function toggleTestCase(tcId) {
  const detail = document.getElementById('tc-detail-' + tcId);
  const icon = document.getElementById('tc-icon-' + tcId);
  if (!detail) return;
  if (detail.style.display === 'none') {
    detail.style.display = 'block';
    if (icon) icon.textContent = '▾';
  } else {
    detail.style.display = 'none';
    if (icon) icon.textContent = '▸';
  }
}

async function loadTestCase(tcId, ruleId) {
  try {
    const resp = await fetch(`/api/rule/${ruleId}/get-testcase/${tcId}`);
    const tc = await resp.json();
    if (tc.error) { Toast.error(tc.error); return; }
    const inputArea = document.getElementById('input-area');
    const expectedArea = document.getElementById('tc-expected');
    const descArea = document.getElementById('tc-desc');
    if (inputArea) inputArea.value = tc.Input || '';
    if (expectedArea) expectedArea.value = tc.Expected || '';
    if (descArea) descArea.value = tc.Description ? (tc.Description + ' (copy)') : '';
    inputArea?.scrollIntoView({ behavior: 'smooth', block: 'center' });
    Toast.info('Test case loaded into input area');
  } catch(e) { Toast.error('Load failed: ' + e.message); }
}

async function deleteTestCase(tcId, ruleId) {
  if (!confirm('Delete this test case?')) return;
  try {
    const result = await apiPost(`/api/rule/${ruleId}/delete-testcase/${tcId}`, {});
    if (result.error) { Toast.error(result.error); return; }
    const el = document.getElementById('tc-' + tcId);
    if (el) el.remove();
    // Update counts
    ['tc-count', 'tc-count-header'].forEach(id => {
      const countEl = document.getElementById(id);
      if (countEl) countEl.textContent = Math.max(0, parseInt(countEl.textContent || '1') - 1);
    });
    Toast.success('Test case deleted');
  } catch(e) { Toast.error('Delete failed: ' + e.message); }
}

async function runAllTestCases(ruleId) {
  const items = document.querySelectorAll('[id^="tc-dot-"]');
  Toast.info(`Running ${items.length} test case(s)...`);
  // For now, just show visual feedback — full run requires backend test execution
  items.forEach(dot => {
    dot.style.background = 'var(--warning)';
    dot.style.boxShadow = '0 0 4px var(--warning)';
  });
  Toast.info('Batch test run: server-side execution coming soon. Use individual Load + Run Parse for now.');
}

// ── LLM Improve ──────────────────────────────────────────────────────────
async function askLLMImprove(ruleId, outputType) {
  Toast.info('Asking LLM to review and improve...');
  // For now, show a helpful message about the LLM integration
  Toast.info('LLM improvement: use "Run Discovery" to regenerate, or manually edit the schema above. Full LLM-assisted editing coming soon.');
}

// ── Run Discovery Handler ────────────────────────────────────────────────
async function runDiscovery() {
  Toast.info('Running discovery...');
  try {
    const resp = await fetch('/api/discover', { method: 'POST' });
    const data = await resp.json();
    if (data.error) { Toast.error(data.error); return; }
    Toast.success(`Discovery complete: ${data.created} new draft(s)`);
    if (data.created > 0) setTimeout(() => location.reload(), 1000);
  } catch(e) { Toast.error('Discovery failed: ' + e.message); }
}

// ── Confirm Dialog ───────────────────────────────────────────────────────
function confirmAction(title, message, onConfirm) {
  const overlay = document.createElement('div');
  overlay.className = 'modal-overlay';
  overlay.innerHTML = `
    <div class="modal">
      <h3>${title}</h3>
      <p>${message}</p>
      <div class="modal-actions">
        <button class="btn btn-ghost" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
        <button class="btn btn-primary" id="modal-confirm">Confirm</button>
      </div>
    </div>`;
  document.body.appendChild(overlay);
  overlay.querySelector('#modal-confirm').onclick = () => { overlay.remove(); onConfirm(); };
  overlay.onclick = (e) => { if (e.target === overlay) overlay.remove(); };
}

// ── Save Local Handler ───────────────────────────────────────────────────
async function saveLocal(ruleId) {
  const btn = event?.target;
  if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spinner"></span> Building...'; }
  try {
    const resp = await fetch(`/api/rule/${ruleId}/save-local`, { method: 'POST' });
    const data = await resp.json();
    const resultEl = document.getElementById('save-result');
    if (data.success) {
      Toast.success('Saved to local files. Build passed!');
      if (resultEl) {
        resultEl.innerHTML = `<div style="color:var(--success)">✓ ${data.message}</div>
          <div style="margin-top:8px;font-size:0.82rem;color:var(--text-muted)">Files: ${(data.paths||[]).join(', ')}</div>`;
      }
    } else {
      Toast.error('Build failed');
      if (resultEl) {
        resultEl.innerHTML = `<div style="color:var(--danger)">✕ ${data.error}</div>
          <pre class="output-preview" style="margin-top:8px">${escHtml(data.build_output)}</pre>`;
      }
    }
  } catch(e) { Toast.error('Save failed: ' + e.message); }
  if (btn) { btn.disabled = false; btn.innerHTML = '💾 Save to Local Files'; }
}

// ── Schema Editor Save Handler ───────────────────────────────────────────
async function saveSchema(ruleId) {
  const schemaYaml = document.querySelector('[name="schema_yaml"]')?.value;
  const goCode = document.querySelector('[name="go_code_draft"]')?.value;
  const form = new URLSearchParams();
  if (schemaYaml !== undefined) form.set('schema_yaml', schemaYaml);
  if (goCode !== undefined) form.set('go_code_draft', goCode);
  try {
    const resp = await fetch(`/rule/${ruleId}`, { method: 'POST', body: form, headers: {'Content-Type': 'application/x-www-form-urlencoded'}, redirect: 'manual' });
    Toast.success('Schema saved');
  } catch(e) { Toast.error('Save failed'); }
}

// ── WebSocket for live updates (Phase 3) ─────────────────────────────────
let ws = null;
function connectWS() {
  if (!window.WebSocket) return;
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  try {
    ws = new WebSocket(`${proto}//${location.host}/ws`);
    ws.onmessage = function(evt) {
      try {
        const msg = JSON.parse(evt.data);
        if (msg.type === 'discovery_progress') {
          Toast.info(`Discovery: ${msg.message}`);
        } else if (msg.type === 'build_log') {
          const el = document.getElementById('build-log');
          if (el) el.textContent += msg.line + '\n';
        } else if (msg.type === 'refresh') {
          location.reload();
        }
      } catch(e) {}
    };
    ws.onclose = function() { setTimeout(connectWS, 5000); };
    ws.onerror = function() { ws.close(); };
  } catch(e) {}
}

// ── Monaco Editor (Phase 3) ──────────────────────────────────────────────
// Load Monaco from CDN only when needed (YAML/Go editing)
let monacoLoaded = false;
let monacoLoadPromise = null;

function loadMonaco() {
  if (monacoLoaded) return Promise.resolve();
  if (monacoLoadPromise) return monacoLoadPromise;
  monacoLoadPromise = new Promise((resolve, reject) => {
    const loaderScript = document.createElement('script');
    loaderScript.src = 'https://cdn.jsdelivr.net/npm/monaco-editor@0.45.0/min/vs/loader.min.js';
    loaderScript.onload = function() {
      require.config({ paths: { vs: 'https://cdn.jsdelivr.net/npm/monaco-editor@0.45.0/min/vs' }});
      require(['vs/editor/editor.main'], function() {
        monacoLoaded = true;
        // Define dark theme matching our design system
        monaco.editor.defineTheme('studio-dark', {
          base: 'vs-dark',
          inherit: true,
          rules: [],
          colors: {
            'editor.background': '#0f1117',
            'editor.foreground': '#e4e6ed',
            'editorCursor.foreground': '#2563eb',
            'editor.lineHighlightBackground': '#1e2130',
            'editor.selectionBackground': '#2a3048',
            'editorLineNumber.foreground': '#6b7394',
          }
        });
        resolve();
      });
    };
    loaderScript.onerror = function() {
      // Monaco CDN not available (air-gapped environment) — degrade gracefully
      monacoLoadPromise = null;
      resolve();
    };
    document.head.appendChild(loaderScript);
  });
  return monacoLoadPromise;
}

// Upgrade a textarea to Monaco editor
async function upgradeToMonaco(textareaId, language) {
  const textarea = document.getElementById(textareaId);
  if (!textarea) return;

  await loadMonaco();
  if (!monacoLoaded) return; // graceful fallback to textarea

  // Create a container
  const container = document.createElement('div');
  container.style.width = '100%';
  container.style.height = textarea.style.minHeight || '300px';
  container.style.minHeight = '300px';
  container.style.border = '1px solid var(--border)';
  container.style.borderRadius = '0 0 var(--radius) var(--radius)';
  textarea.parentNode.replaceChild(container, textarea);

  const editor = monaco.editor.create(container, {
    value: textarea.value,
    language: language || 'yaml',
    theme: 'studio-dark',
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
    fontSize: 13,
    fontFamily: "'JetBrains Mono', 'Fira Code', 'SF Mono', Menlo, monospace",
    lineNumbers: 'on',
    renderLineHighlight: 'line',
    automaticLayout: true,
    tabSize: 2,
    wordWrap: 'on',
    padding: { top: 8, bottom: 8 },
  });

  // Sync back to a hidden input for form submission
  const hidden = document.createElement('input');
  hidden.type = 'hidden';
  hidden.name = textarea.name;
  hidden.value = textarea.value;
  container.parentNode.appendChild(hidden);

  editor.onDidChangeModelContent(() => {
    hidden.value = editor.getValue();
  });

  // Store reference
  container.dataset.editorId = textareaId;
  window['monacoEditor_' + textareaId] = editor;
  return editor;
}

// ── Keyboard Shortcuts ───────────────────────────────────────────────────
document.addEventListener('keydown', function(e) {
  // Ctrl+S / Cmd+S = Save schema
  if ((e.ctrlKey || e.metaKey) && e.key === 's') {
    e.preventDefault();
    const saveBtn = document.querySelector('[onclick*="saveSchema"]');
    if (saveBtn) saveBtn.click();
  }
  // Ctrl+Enter = Run parse
  if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
    e.preventDefault();
    const parseBtn = document.querySelector('[onclick*="runParse"]');
    if (parseBtn) parseBtn.click();
  }
});

// Init
document.addEventListener('DOMContentLoaded', function() {
  connectWS();

  // Render sample inputs from JSON to readable text
  renderSampleInputs();

  // Auto-upgrade textareas to Monaco if available
  const schemaTextarea = document.querySelector('[name="schema_yaml"]');
  if (schemaTextarea && schemaTextarea.tagName === 'TEXTAREA') {
    schemaTextarea.id = schemaTextarea.id || 'schema-editor';
    upgradeToMonaco(schemaTextarea.id, 'yaml');
  }
  const codeTextarea = document.querySelector('[name="go_code_draft"]');
  if (codeTextarea && codeTextarea.tagName === 'TEXTAREA') {
    codeTextarea.id = codeTextarea.id || 'code-editor';
    upgradeToMonaco(codeTextarea.id, 'go');
  }
});
