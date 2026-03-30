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
    // Pipeline result (has columns array and mode field)
    if (data.columns && data.rows && data.rows.length > 0) {
      const cols = data.columns;
      let html = '';
      html += `<div style="margin-bottom:8px;padding:6px 10px;background:var(--bg-tertiary);border-radius:var(--radius);font-size:0.78rem;color:var(--text-secondary)">
        <strong>🔧 Pipeline (${data.mode || 'table'} mode)</strong> — ${cols.length} column(s): ${cols.map(c => `<code style="background:var(--bg-secondary);padding:1px 5px;border-radius:3px">${c}</code>`).join(' ')}
      </div>`;
      html += '<table class="result-table"><tr>';
      cols.forEach(c => html += `<th>${c}</th>`);
      html += '</tr>';
      data.rows.forEach(row => {
        html += '<tr>';
        cols.forEach(c => {
          let val = row[c] || '';
          let cls = '';
          if (val.toLowerCase() === 'up') cls = ' class="val-up"';
          else if (val.toLowerCase() === 'down') cls = ' class="val-down"';
          html += `<td${cls}>${escHtml(val)}</td>`;
        });
        html += '</tr>';
      });
      html += '</table>';
      html += `<div style="margin-top:8px;font-size:0.78rem;color:var(--text-muted)">${data.rows.length} row(s) parsed</div>`;
      target.innerHTML = html;
      Toast.success(`Parsed ${data.rows.length} rows`);
      return;
    }
    // Pipeline result with 0 rows
    if (data.columns && data.rows !== undefined && (!data.rows || data.rows.length === 0)) {
      target.innerHTML = `<div style="padding:12px;color:var(--text-muted);font-style:italic">No data extracted. Check your DSL pattern matches the input text.</div>`;
      Toast.warning('No rows extracted');
      return;
    }
    // Table result (from schema engine — uses Rows with capital R)
    if (data.Rows && data.Rows.length > 0) {
      const keys = Object.keys(data.Rows[0]);
      let html = '';
      // Show auto-columns hint if columns were auto-detected from header
      if (data.auto_columns && data.auto_columns.length > 0) {
        html += `<div style="margin-bottom:10px;padding:8px 12px;background:var(--accent-bg);border:1px solid var(--accent);border-radius:var(--radius);font-size:0.82rem">
          <strong>⚡ Auto-detected columns from header:</strong>
          <span style="margin-left:8px;color:var(--text-secondary)">${data.auto_columns.map((c,i) => `<code style="background:var(--bg-tertiary);padding:1px 5px;border-radius:3px;margin:0 2px">[${i}] ${c.Name}</code>`).join(' ')}</span>
          <div style="margin-top:4px;color:var(--text-muted);font-size:0.75rem">💡 Tip: Schema has no <code>columns</code> defined — columns were inferred by splitting the header line on whitespace. Add explicit <code>columns</code> in the YAML to customize.</div>
        </div>`;
      }
      html += '<table class="result-table"><tr>';
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
    const lineCount = sampleText.split('\n').length;
    const canCollapse = lineCount > 8;
    const startCollapsed = canCollapse; // default collapsed for long samples
    html += `<div class="sample-item" style="margin-bottom:8px">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:4px">
        <span style="font-size:0.78rem;color:var(--text-muted)">Sample #${idx + 1} (${lineCount} lines)</span>
        <div class="btn-group">
          <button class="btn btn-ghost btn-sm" onclick="useSampleInput(${idx})" title="Copy to test input area">↓ Use as Test Input</button>
          ${canCollapse ? `<button class="btn btn-ghost btn-sm" onclick="toggleSample(this, ${idx})" title="Expand/collapse">${startCollapsed ? '▸ Expand' : '▾ Collapse'}</button>` : ''}
        </div>
      </div>
      <div class="output-preview sample-text" id="sample-${idx}" tabindex="0" ${startCollapsed ? 'style="max-height:160px;overflow:hidden"' : ''}>${escHtml(sampleText)}</div>
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
    // Currently collapsed → expand
    el.style.maxHeight = '';
    el.style.overflow = '';
    btn.textContent = '▾ Collapse';
  } else {
    // Currently expanded → collapse
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
  const runBtn = event?.target;
  if (runBtn) { runBtn.disabled = true; runBtn.innerHTML = '<span class="spinner"></span> Running...'; }

  // Set all dots to pending (yellow)
  items.forEach(dot => {
    dot.style.background = 'var(--warning)';
    dot.style.boxShadow = '0 0 4px var(--warning)';
  });

  Toast.info(`Running ${items.length} test case(s)...`);

  try {
    const resp = await fetch(`/api/rule/${ruleId}/run-all-tests`, { method: 'POST' });
    const data = await resp.json();

    if (data.error) {
      Toast.error(data.error);
      items.forEach(dot => { dot.style.background = 'var(--text-muted)'; dot.style.boxShadow = 'none'; });
      return;
    }

    // Handle "no test cases" preview mode
    if (data.no_test_cases && data.sample_preview) {
      Toast.info(data.message || 'No test cases — showing sample preview');
      const resultEl = document.getElementById('parse-result');
      if (resultEl) {
        formatParseResult(JSON.stringify(data.sample_preview), 'parse-result');
      }
      return;
    }

    // Update status dots per test case
    let passCount = 0;
    (data.results || []).forEach(r => {
      const dot = document.getElementById('tc-dot-' + r.tc_id);
      if (dot) {
        dot.style.background = r.passed ? 'var(--success)' : 'var(--danger)';
        dot.style.boxShadow = r.passed ? '0 0 4px var(--success)' : '0 0 4px var(--danger)';
        dot.classList.add(r.passed ? 'pass' : 'fail');
        dot.classList.remove(r.passed ? 'fail' : 'pass');
      }
      if (r.passed) passCount++;

      // Show diff detail if test failed
      if (!r.passed) {
        const detailEl = document.getElementById('tc-detail-' + r.tc_id);
        if (detailEl) {
          detailEl.style.display = 'block';
          const iconEl = document.getElementById('tc-icon-' + r.tc_id);
          if (iconEl) iconEl.textContent = '▾';
          // Append diff view
          let diffHtml = buildDiffHtml(r);
          let diffContainer = detailEl.querySelector('.tc-diff-result');
          if (!diffContainer) {
            diffContainer = document.createElement('div');
            diffContainer.className = 'tc-diff-result';
            diffContainer.style.marginTop = '8px';
            detailEl.appendChild(diffContainer);
          }
          diffContainer.innerHTML = diffHtml;
        }
      }
    });

    // Store results for approve gate check
    window._lastTestResults = data;

    if (data.all_passed) {
      Toast.success(`All ${data.total} test(s) passed ✓`);
    } else {
      Toast.warning(`${passCount}/${data.total} tests passed — ${data.total - passCount} failed`);
    }
  } catch (e) {
    Toast.error('Run all tests failed: ' + e.message);
    items.forEach(dot => { dot.style.background = 'var(--text-muted)'; dot.style.boxShadow = 'none'; });
  } finally {
    if (runBtn) { runBtn.disabled = false; runBtn.innerHTML = '▶ Run All'; }
  }
}

// Build HTML for a diff result of a single failed test case
function buildDiffHtml(testResult) {
  if (testResult.error) {
    return `<div class="diff-error"><span style="color:var(--danger)">✕ Error:</span> ${escHtml(testResult.error)}</div>`;
  }
  const diff = testResult.diff;
  if (!diff) return '<div style="color:var(--text-muted)">No diff data</div>';

  let html = '<div class="diff-view">';
  html += `<div class="diff-header">`;
  html += `<span class="diff-stat ${diff.row_count_match ? 'diff-ok' : 'diff-fail'}">Rows: ${diff.expected_rows} expected → ${diff.actual_rows} actual</span>`;
  html += `</div>`;

  if (diff.missing_fields && diff.missing_fields.length > 0) {
    html += `<div class="diff-missing">Missing fields: ${diff.missing_fields.map(f => `<code>${escHtml(f)}</code>`).join(', ')}</div>`;
  }
  if (diff.extra_fields && diff.extra_fields.length > 0) {
    html += `<div class="diff-extra">Extra fields: ${diff.extra_fields.map(f => `<code>${escHtml(f)}</code>`).join(', ')}</div>`;
  }
  if (diff.field_diffs && diff.field_diffs.length > 0) {
    html += '<table class="diff-table"><tr><th>Row</th><th>Field</th><th>Expected</th><th>Actual</th></tr>';
    diff.field_diffs.slice(0, 20).forEach(fd => {
      html += `<tr>
        <td>${fd.row}</td>
        <td><code>${escHtml(fd.field)}</code></td>
        <td class="diff-expected">${escHtml(fd.expected)}</td>
        <td class="diff-actual">${escHtml(fd.actual)}</td>
      </tr>`;
    });
    if (diff.field_diffs.length > 20) {
      html += `<tr><td colspan="4" style="color:var(--text-muted);text-align:center">… and ${diff.field_diffs.length - 20} more</td></tr>`;
    }
    html += '</table>';
  }
  html += '</div>';
  return html;
}

// ── Unknown Badge Update ─────────────────────────────────────────────────
// Dynamically update the sidebar Unknown count badge without a full page reload.
function updateUnknownBadge(delta) {
  const badge = document.querySelector('.sidebar a[href="/unknown"] .nav-badge');
  if (badge) {
    const cur = parseInt(badge.textContent || '0', 10);
    const next = Math.max(0, cur + delta);
    if (next === 0) {
      badge.remove();
    } else {
      badge.textContent = next;
    }
  } else if (delta > 0) {
    // Create badge if it doesn't exist yet
    const link = document.querySelector('.sidebar a[href="/unknown"]');
    if (link) {
      const span = document.createElement('span');
      span.className = 'nav-badge';
      span.textContent = delta;
      link.appendChild(span);
    }
  }
}

// ── Generate Task Manager ────────────────────────────────────────────────
// Supports parallel generation of multiple unknown outputs with per-row progress.
const generateTasks = new Map(); // id -> AbortController

async function generateRule(unknownId, commandNorm) {
  if (generateTasks.has(unknownId)) {
    Toast.warning('Generation already in progress for this item');
    return;
  }

  const row = document.getElementById('unknown-' + unknownId);
  if (!row) return;

  const actionsCell = row.querySelector('td:last-child');
  if (!actionsCell) return;

  const originalContent = actionsCell.innerHTML;

  // Show SSE progress UI with step indicators
  actionsCell.innerHTML = `
    <div class="generate-progress" id="gen-progress-${unknownId}">
      <div style="display:flex;align-items:center;gap:8px">
        <span class="spinner"></span>
        <span class="gen-step-text" style="font-size:0.82rem;color:var(--accent)">Connecting…</span>
      </div>
      <div class="gen-steps" style="display:flex;gap:4px;margin-top:4px">
        <span class="gen-step-dot" data-step="llm" title="LLM">●</span>
        <span class="gen-step-dot" data-step="validate" title="Validate">●</span>
        <span class="gen-step-dot" data-step="selftest" title="Self-test">●</span>
      </div>
    </div>`;

  row.style.background = 'var(--accent-bg)';
  generateTasks.set(unknownId, true);

  const stepText = actionsCell.querySelector('.gen-step-text');
  const stepDots = actionsCell.querySelectorAll('.gen-step-dot');

  function setStepActive(stepName) {
    stepDots.forEach(dot => {
      if (dot.dataset.step === stepName) {
        dot.style.color = 'var(--accent)';
        dot.classList.add('gen-step-active');
      }
    });
  }
  function setStepDone(stepName, ok) {
    stepDots.forEach(dot => {
      if (dot.dataset.step === stepName) {
        dot.style.color = ok ? 'var(--success)' : 'var(--danger)';
        dot.textContent = ok ? '✓' : '✕';
        dot.classList.remove('gen-step-active');
      }
    });
  }

  // SSE via POST workaround: use fetch with streaming reader
  // Note: EventSource only supports GET. Since our endpoint is POST, we use fetch + ReadableStream.
  try {
    const resp = await fetch(`/api/unknown/${unknownId}/generate`, { method: 'POST' });

    if (!resp.ok) {
      const errData = await resp.json().catch(() => ({ error: resp.statusText }));
      throw new Error(errData.error || resp.statusText);
    }

    const contentType = resp.headers.get('content-type') || '';
    if (contentType.includes('text/event-stream')) {
      // SSE streaming mode — parse SSE from ReadableStream
      const reader = resp.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      let lastEvent = null;

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });

        // Parse SSE lines
        const lines = buffer.split('\n');
        buffer = lines.pop(); // keep incomplete line

        for (const line of lines) {
          if (!line.startsWith('data: ')) continue;
          try {
            const ev = JSON.parse(line.slice(6));
            lastEvent = ev;

            switch (ev.type) {
              case 'generate_llm':
                stepText.textContent = '🤖 ' + (ev.message || 'Calling LLM…');
                setStepActive('llm');
                break;
              case 'generate_validate':
                setStepDone('llm', true);
                stepText.textContent = '🔍 ' + (ev.message || 'Validating DSL…');
                setStepActive('validate');
                break;
              case 'generate_selftest':
                setStepDone('validate', true);
                stepText.textContent = '🧪 ' + (ev.message || 'Self-testing…');
                setStepActive('selftest');
                break;
              case 'generate_fix':
                stepText.textContent = `🔧 Fix ${ev.attempt || ''}/${ev.max_attempts || 2}: ${ev.message || 'Fixing…'}`;
                setStepActive('selftest');
                break;
              case 'generate_done':
                if (ev.error) {
                  setStepDone('selftest', false);
                  throw new Error(ev.error);
                }
                setStepDone('selftest', ev.self_test_passed);
                // Final: show result
                const badge = ev.self_test_passed
                  ? `<span style="color:var(--success)">✓</span>`
                  : `<span class="badge badge-draft" style="font-size:0.7rem">⚠ draft</span>`;
                actionsCell.innerHTML = `
                  <div style="display:flex;align-items:center;gap:8px">
                    ${badge}
                    <span style="font-size:0.82rem">Rule #${ev.rule_id}</span>
                    <a href="/rule/${ev.rule_id}" class="btn btn-primary btn-sm">View →</a>
                  </div>`;
                row.style.background = ev.self_test_passed ? 'var(--success-bg)' : 'var(--warning-bg)';
                Toast.success(`Rule created for ${commandNorm || 'unknown command'}${ev.self_test_passed ? '' : ' (self-test warnings)'}`);
                updateUnknownBadge(-1);
                break;
            }
          } catch (parseErr) {
            if (parseErr.message && !parseErr.message.startsWith('Unexpected')) throw parseErr;
          }
        }
      }

      // If we finished reading without a done event, handle it
      if (!lastEvent || lastEvent.type !== 'generate_done') {
        throw new Error('Stream ended unexpectedly');
      }
    } else {
      // JSON fallback (non-SSE server response)
      const data = await resp.json();
      if (data.error) throw new Error(data.error);
      actionsCell.innerHTML = `
        <div style="display:flex;align-items:center;gap:8px">
          <span style="color:var(--success);font-size:0.85rem">✓ Rule #${data.rule_id}</span>
          <a href="${data.redirect || '/rule/' + data.rule_id}" class="btn btn-primary btn-sm">View Rule →</a>
        </div>`;
      row.style.background = 'var(--success-bg)';
      Toast.success(`Rule created for ${commandNorm || 'unknown command'}`);
      updateUnknownBadge(-1);
    }
  } catch (err) {
    actionsCell.innerHTML = `
      <div style="display:flex;flex-direction:column;gap:4px">
        <span style="font-size:0.78rem;color:var(--danger)">✕ ${escHtml(err.message || 'Network error')}</span>
        <div class="btn-group">
          <button class="btn btn-primary btn-sm" onclick="generateRule(${unknownId}, '${escHtml(commandNorm)}')">↻ Retry</button>
          <button class="btn btn-ghost btn-sm" onclick="restoreGenerateRow(${unknownId})">Cancel</button>
        </div>
      </div>`;
    row.style.background = 'var(--danger-bg)';
    Toast.error('Generate failed: ' + (err.message || 'network error'));
  } finally {
    generateTasks.delete(unknownId);
  }
}

// Store original action buttons HTML per row for restoration
const originalRowActions = new Map();

function storeOriginalActions(unknownId) {
  const row = document.getElementById('unknown-' + unknownId);
  if (row && !originalRowActions.has(unknownId)) {
    const actionsCell = row.querySelector('td:last-child');
    if (actionsCell) originalRowActions.set(unknownId, actionsCell.innerHTML);
  }
}

function restoreGenerateRow(unknownId) {
  const row = document.getElementById('unknown-' + unknownId);
  if (!row) return;
  row.style.background = '';
  const actionsCell = row.querySelector('td:last-child');
  if (actionsCell && originalRowActions.has(unknownId)) {
    actionsCell.innerHTML = originalRowActions.get(unknownId);
  }
}

// ── Ignore Unknown with Badge Update ─────────────────────────────────────
// HTMX alone doesn't update the badge. We add an event listener for HTMX afterSwap.
document.addEventListener('htmx:afterSwap', function(evt) {
  // Check if this was an unknown ignore operation (target id starts with "unknown-")
  const target = evt.detail.target;
  if (target && target.id && target.id.startsWith('unknown-') && target.style.display === 'none') {
    updateUnknownBadge(-1);
  }
});

// ── LLM Expected Output Display ──────────────────────────────────────────
// Shows the LLM's understanding of what the correct extraction should look like.
function showLLMExpectedOutput(description) {
  let panel = document.getElementById('llm-expected-panel');
  if (!panel) {
    // Create the panel after the editor card
    const editorCard = document.querySelector('.card-header .btn-group')?.closest('.card');
    if (!editorCard) return;
    panel = document.createElement('div');
    panel.id = 'llm-expected-panel';
    panel.className = 'card';
    panel.style.borderColor = 'var(--accent)';
    panel.style.borderWidth = '2px';
    editorCard.after(panel);
  }
  panel.innerHTML = `
    <div class="card-header" style="background:var(--accent-bg)">
      <h3 style="color:var(--accent)">🧠 LLM's Understanding of Expected Output</h3>
      <button class="btn btn-ghost btn-sm" onclick="this.closest('.card').remove()" title="Dismiss">✕</button>
    </div>
    <div class="card-body">
      <div style="padding:12px;background:var(--bg-tertiary);border-radius:var(--radius);font-size:0.88rem;line-height:1.6;white-space:pre-wrap">${escHtml(description)}</div>
      <div style="margin-top:8px;font-size:0.78rem;color:var(--text-muted)">
        💡 This is what the LLM thinks the correct extraction should produce. If this doesn't match your expectations, the LLM may be misunderstanding the sample input — try adding a test case with the correct expected output.
      </div>
    </div>`;
  panel.scrollIntoView({ behavior: 'smooth', block: 'center' });
}

// ── LLM Improve ──────────────────────────────────────────────────────────
async function askLLMImprove(ruleId, outputType) {
  const btn = event?.target;
  if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spinner"></span> Asking LLM…'; }

  Toast.info('Running failed tests and asking LLM to improve DSL...');

  try {
    const resp = await fetch(`/api/rule/${ruleId}/improve`, { method: 'POST' });
    const data = await resp.json();

    if (data.error) {
      Toast.error(data.error);
      return;
    }

    if (data.status === 'all_passed') {
      Toast.success('All test cases already pass — no improvement needed! ✓');
      return;
    }

    if (data.status === 'improved' && data.improved_dsl) {
      // Update the editor content
      const monacoEditor = window['monacoEditor_pipeline-editor'] || window['monacoEditor_schema-editor'];
      if (monacoEditor) {
        monacoEditor.setValue(data.improved_dsl);
        Toast.success('LLM improved the DSL — review changes in the editor');
      } else {
        // Fallback: update textarea directly
        const textarea = document.querySelector('[name="schema_yaml"]');
        if (textarea) {
          textarea.value = data.improved_dsl;
          Toast.success('LLM improved the DSL — review changes in the editor');
        }
        // Also update hidden input if exists
        const hidden = document.querySelector('input[name="schema_yaml"][type="hidden"]');
        if (hidden) hidden.value = data.improved_dsl;
      }

      // Show LLM's expected output description if available
      if (data.expected_output_description) {
        showLLMExpectedOutput(data.expected_output_description);
      }

      // Hint to save
      Toast.info('Remember to Save (⌘S) and Run All Tests to verify the improvement');
    }
  } catch (e) {
    Toast.error('LLM improve failed: ' + e.message);
  } finally {
    if (btn) { btn.disabled = false; btn.innerHTML = '🤖 Ask LLM to Improve'; }
  }
}

// ── Run Discovery Handler (SSE real-time progress) ───────────────────────
let discoveryRunning = false;

function runDiscovery() {
  if (discoveryRunning) {
    Toast.warning('Discovery already running');
    return;
  }
  discoveryRunning = true;

  // Create / show progress panel
  let panel = document.getElementById('discovery-panel');
  if (!panel) {
    panel = document.createElement('div');
    panel.id = 'discovery-panel';
    panel.className = 'discovery-panel';
    document.body.appendChild(panel);
  }
  panel.style.display = 'block';
  panel.innerHTML = `
    <div class="discovery-header">
      <span>🔄 Discovery Running…</span>
      <button class="btn btn-ghost btn-sm" onclick="closeDiscoveryPanel()" title="Minimize">—</button>
    </div>
    <div class="discovery-progress" id="disc-progress">
      <div class="disc-status">Connecting…</div>
    </div>
    <div class="discovery-log" id="disc-log"></div>
  `;

  const logEl = document.getElementById('disc-log');
  const statusEl = panel.querySelector('.disc-status');

  const evtSource = new EventSource('/api/discover');

  evtSource.onmessage = function(e) {
    try {
      const ev = JSON.parse(e.data);
      const line = document.createElement('div');
      line.className = 'disc-line disc-' + ev.type;

      switch (ev.type) {
        case 'start':
          statusEl.innerHTML = `📊 ${ev.total} command group(s) to process`;
          line.textContent = `▸ ${ev.message}`;
          break;
        case 'pending':
          statusEl.innerHTML = `⏳ [${ev.index}/${ev.total}] Calling LLM: <code>${escHtml(ev.command_norm)}</code>`;
          line.innerHTML = `<span style="color:var(--accent)">⏳ [${ev.index}/${ev.total}]</span> ${escHtml(ev.vendor)}/${escHtml(ev.command_norm)}`;
          break;
        case 'success':
          line.innerHTML = `<span style="color:var(--success)">✓ [${ev.index}/${ev.total}]</span> ${escHtml(ev.command_norm)} — rule created`;
          break;
        case 'skipped':
          line.innerHTML = `<span style="color:var(--text-muted)">— [${ev.index}/${ev.total}]</span> ${escHtml(ev.command_norm)} — ${escHtml(ev.message)}`;
          break;
        case 'failed':
          line.innerHTML = `<span style="color:var(--danger)">✕ [${ev.index}/${ev.total}]</span> ${escHtml(ev.command_norm)} — ${escHtml(ev.error)}`;
          break;
        case 'done':
          statusEl.innerHTML = `✅ ${ev.message}`;
          panel.querySelector('.discovery-header span').textContent = '✅ Discovery Complete';
          evtSource.close();
          discoveryRunning = false;
          if (ev.created > 0) {
            Toast.success(`Discovery: ${ev.created} new draft(s)`);
            // Only auto-reload if no Generate tasks are in progress
            if (generateTasks.size === 0) {
              setTimeout(() => location.reload(), 1500);
            } else {
              Toast.info('Page not reloaded — Generate tasks still in progress. Reload manually when ready.');
            }
          } else {
            Toast.info('Discovery complete — no new rules created');
          }
          return;
      }
      logEl.appendChild(line);
      logEl.scrollTop = logEl.scrollHeight;
    } catch(err) {
      console.warn('SSE parse error', err);
    }
  };

  evtSource.onerror = function() {
    evtSource.close();
    discoveryRunning = false;
    statusEl.innerHTML = '<span style="color:var(--danger)">Connection lost</span>';
    panel.querySelector('.discovery-header span').textContent = '⚠ Discovery Disconnected';
    Toast.error('Discovery: connection lost');
  };
}

function closeDiscoveryPanel() {
  const panel = document.getElementById('discovery-panel');
  if (panel) panel.style.display = 'none';
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

// ── Save Local Handler (with approve gate) ───────────────────────────────
async function saveLocal(ruleId) {
  const btn = event?.target;

  // Approve gate: run all tests first
  const tcDots = document.querySelectorAll('[id^="tc-dot-"]');
  if (tcDots.length > 0) {
    if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spinner"></span> Running tests...'; }
    try {
      const testResp = await fetch(`/api/rule/${ruleId}/run-all-tests`, { method: 'POST' });
      const testData = await testResp.json();
      if (testData.error) { Toast.error(testData.error); return; }
      if (!testData.all_passed) {
        Toast.warning(`Cannot save: ${testData.total - (testData.results||[]).filter(r=>r.passed).length} test(s) failed. Fix the DSL or edit test cases first.`);
        // Update dots visually
        (testData.results || []).forEach(r => {
          const dot = document.getElementById('tc-dot-' + r.tc_id);
          if (dot) {
            dot.style.background = r.passed ? 'var(--success)' : 'var(--danger)';
            dot.style.boxShadow = r.passed ? '0 0 4px var(--success)' : '0 0 4px var(--danger)';
          }
        });
        if (btn) { btn.disabled = false; btn.innerHTML = '💾 Save to Local Files'; }
        return;
      }
      Toast.success('All tests passed — proceeding to save...');
    } catch (e) {
      Toast.error('Test check failed: ' + e.message);
      if (btn) { btn.disabled = false; btn.innerHTML = '💾 Save to Local Files'; }
      return;
    }
  }

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

// ── Approve & Save (with test gate) ──────────────────────────────────────
async function approveAndSave(ruleId) {
  // Run all tests first
  const tcDots = document.querySelectorAll('[id^="tc-dot-"]');
  if (tcDots.length > 0) {
    Toast.info('Running all tests before approve...');
    try {
      const testResp = await fetch(`/api/rule/${ruleId}/run-all-tests`, { method: 'POST' });
      const testData = await testResp.json();
      if (testData.error) { Toast.error(testData.error); return; }
      if (!testData.all_passed) {
        const failCount = (testData.results||[]).filter(r=>!r.passed).length;
        Toast.warning(`Cannot approve: ${failCount} test(s) failed. Fix DSL or edit test cases first.`);
        // Update dots
        (testData.results || []).forEach(r => {
          const dot = document.getElementById('tc-dot-' + r.tc_id);
          if (dot) {
            dot.style.background = r.passed ? 'var(--success)' : 'var(--danger)';
            dot.style.boxShadow = r.passed ? '0 0 4px var(--success)' : '0 0 4px var(--danger)';
          }
        });
        return;
      }
    } catch (e) {
      Toast.error('Test check failed: ' + e.message);
      return;
    }
  }

  // All tests passed, proceed with save
  confirmAction('Approve & Save',
    'All tests passed ✓ — This will approve the rule and save it to local config.',
    function() {
      fetch(`/api/rule/${ruleId}/save-local`, { method: 'POST' })
        .then(r => r.json())
        .then(d => {
          if (d.error) { Toast.error(d.error); }
          else { Toast.success('Rule approved and saved locally'); setTimeout(() => location.href = '/rules', 1000); }
        });
    }
  );
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
          // Don't force reload if Generate tasks are in progress
          if (generateTasks.size === 0) {
            location.reload();
          }
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
  // Ctrl+A / Cmd+A inside .output-preview: select only the box content, not the whole page
  if ((e.ctrlKey || e.metaKey) && e.key === 'a') {
    const active = document.activeElement;
    if (active && active.classList.contains('output-preview')) {
      e.preventDefault();
      const range = document.createRange();
      range.selectNodeContents(active);
      const sel = window.getSelection();
      sel.removeAllRanges();
      sel.addRange(range);
    }
  }
});

// ══════════════════════════════════════════════════════════════════════════
// Batch Import — File upload, analyze, review, batch generate
// ══════════════════════════════════════════════════════════════════════════

// Global state for batch import
let batchAnalysis = null; // last analysis result

// ── File Upload (drag & drop + click) ────────────────────────────────────
function handleDrop(e) {
  e.preventDefault();
  const zone = document.getElementById('drop-zone');
  if (zone) zone.classList.remove('drag-over');
  const files = e.dataTransfer?.files;
  if (files && files.length > 0) {
    loadFile(files[0]);
  }
}

function handleFileSelect(input) {
  if (input.files && input.files.length > 0) {
    loadFile(input.files[0]);
  }
}

function loadFile(file) {
  const info = document.getElementById('file-info');
  if (file.size > 50 * 1024 * 1024) {
    Toast.error('File too large (max 50MB)');
    return;
  }
  const reader = new FileReader();
  reader.onload = function(e) {
    const textarea = document.getElementById('paste-area');
    if (textarea) textarea.value = e.target.result;
    if (info) info.innerHTML = `📄 ${escHtml(file.name)} (${(file.size / 1024).toFixed(1)} KB, ${e.target.result.split('\n').length} lines)`;
    Toast.info(`Loaded ${file.name}`);
  };
  reader.readAsText(file);
}

// Make drop zone clickable
document.addEventListener('click', function(e) {
  if (e.target.closest('#drop-zone')) {
    const fileInput = document.getElementById('file-input');
    if (fileInput && e.target.tagName !== 'INPUT') fileInput.click();
  }
});

// ── Analyze Log ──────────────────────────────────────────────────────────
async function analyzeLog() {
  const content = document.getElementById('paste-area')?.value || '';
  const vendor = document.getElementById('import-vendor')?.value || 'auto';
  const btn = document.getElementById('btn-analyze');

  if (!content.trim()) {
    Toast.warning('Upload a file or paste session log content first');
    return;
  }

  if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spinner"></span> Analyzing...'; }

  try {
    const resp = await fetch('/api/import/analyze', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ content, vendor }),
    });
    const data = await resp.json();
    if (data.error) { Toast.error(data.error); return; }

    batchAnalysis = data;
    renderCommandList(data);

    // Show step 2 and step 3
    document.getElementById('step-review').style.display = '';
    document.getElementById('step-generate').style.display = '';
    document.getElementById('step-review').scrollIntoView({ behavior: 'smooth', block: 'start' });

    Toast.success(`Analyzed: ${data.total_blocks} unique commands found`);
  } catch (e) {
    Toast.error('Analysis failed: ' + e.message);
  } finally {
    if (btn) { btn.disabled = false; btn.innerHTML = '🔍 Analyze'; }
  }
}

// ── Render Command List ──────────────────────────────────────────────────
function renderCommandList(analysis) {
  const container = document.getElementById('command-list');
  const summary = document.getElementById('analysis-summary');
  if (!container) return;

  // Summary
  const cmds = analysis.commands || [];
  const selectedCount = cmds.filter(c => c.selected).length;
  const controlCount = cmds.filter(c => c.is_control).length;
  const helpCount = cmds.filter(c => c.is_help).length;
  const errorCount = cmds.filter(c => c.is_error).length;
  const existingCount = cmds.filter(c => c.has_existing_rule).length;

  if (summary) {
    summary.innerHTML = `
      <strong>Vendor:</strong> ${escHtml(analysis.vendor || 'auto')} &nbsp;|&nbsp;
      <strong>Total:</strong> ${cmds.length} unique commands &nbsp;|&nbsp;
      <span style="color:var(--success)">✓ ${selectedCount} recommended</span> &nbsp;|&nbsp;
      <span style="color:var(--text-muted)">${controlCount} control, ${helpCount} help, ${errorCount} error, ${existingCount} existing rule</span>
    `;
  }

  // Column header
  let html = `<div class="cmd-row-header">
    <div>☑</div><div>Command Pattern</div><div>Category</div><div>Output</div><div>Status</div>
  </div>`;

  // Rows
  cmds.forEach((cmd, i) => {
    const classes = ['cmd-row'];
    if (cmd.selected) classes.push('selected');
    if (cmd.is_control) classes.push('is-control');
    if (cmd.is_help) classes.push('is-help');
    if (cmd.is_error) classes.push('is-error');
    if (cmd.has_existing_rule) classes.push('has-rule');

    let statusBadge = '';
    if (cmd.is_control) statusBadge = '<span class="tag" style="color:var(--text-muted)">control</span>';
    else if (cmd.is_help) statusBadge = '<span class="tag" style="color:var(--text-muted)">help</span>';
    else if (cmd.is_error) statusBadge = '<span class="tag" style="color:var(--danger)">error</span>';
    else if (cmd.has_existing_rule) statusBadge = `<span class="tag" style="color:var(--success)">✓ ${escHtml(cmd.existing_cmd_type)}</span>`;
    else statusBadge = '<span class="tag" style="color:var(--accent)">new</span>';

    html += `<div class="${classes.join(' ')}" data-index="${i}" onclick="toggleCommand(${i})">
      <div><input type="checkbox" ${cmd.selected ? 'checked' : ''} onclick="event.stopPropagation();toggleCommand(${i})"></div>
      <div>
        <code style="font-weight:600;cursor:pointer" onclick="event.stopPropagation();editPattern(${i})" title="Click to edit pattern">${escHtml(cmd.pattern)}</code>
        <div style="font-size:0.72rem;color:var(--text-muted);margin-top:2px">${escHtml(cmd.raw)}</div>
      </div>
      <div><span class="category-tag">${escHtml(cmd.category)}</span></div>
      <div style="font-size:0.78rem;color:var(--text-muted)">${cmd.has_output ? cmd.output_lines + ' lines' : 'none'}</div>
      <div>${statusBadge}</div>
    </div>`;
  });

  container.innerHTML = html;
  updateSelectedCount();
}

// ── Toggle Command Selection ─────────────────────────────────────────────
function toggleCommand(index) {
  if (!batchAnalysis || !batchAnalysis.commands) return;
  batchAnalysis.commands[index].selected = !batchAnalysis.commands[index].selected;

  const rows = document.querySelectorAll('.cmd-row');
  const row = rows[index];
  if (row) {
    const cb = row.querySelector('input[type="checkbox"]');
    if (cb) cb.checked = batchAnalysis.commands[index].selected;
    row.classList.toggle('selected', batchAnalysis.commands[index].selected);
  }
  updateSelectedCount();
}

function selectAll() {
  if (!batchAnalysis) return;
  batchAnalysis.commands.forEach(c => c.selected = true);
  renderCommandList(batchAnalysis);
}

function selectNone() {
  if (!batchAnalysis) return;
  batchAnalysis.commands.forEach(c => c.selected = false);
  renderCommandList(batchAnalysis);
}

function selectRecommended() {
  if (!batchAnalysis) return;
  batchAnalysis.commands.forEach(c => {
    c.selected = c.has_output && !c.is_control && !c.is_help && !c.is_error && !c.has_existing_rule;
  });
  renderCommandList(batchAnalysis);
}

function updateSelectedCount() {
  const el = document.getElementById('selected-count');
  if (!el || !batchAnalysis) return;
  const count = (batchAnalysis.commands || []).filter(c => c.selected).length;
  el.textContent = `${count} command${count !== 1 ? 's' : ''} selected`;
}

// ── Batch Generate ───────────────────────────────────────────────────────
async function batchGenerate() {
  if (!batchAnalysis || !batchAnalysis.commands) {
    Toast.warning('Analyze a log file first');
    return;
  }

  const selected = batchAnalysis.commands.filter(c => c.selected);
  if (selected.length === 0) {
    Toast.warning('Select at least one command to generate rules for');
    return;
  }

  const btn = document.getElementById('btn-generate');
  if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spinner"></span> Generating...'; }

  const progressEl = document.getElementById('generate-progress');
  const statusEl = document.getElementById('gen-status');
  const logEl = document.getElementById('gen-log');
  const resultsEl = document.getElementById('generate-results');

  if (progressEl) progressEl.style.display = '';
  if (logEl) logEl.innerHTML = '';
  if (resultsEl) { resultsEl.style.display = 'none'; resultsEl.innerHTML = ''; }

  const payload = {
    vendor: batchAnalysis.vendor || '',
    commands: selected.map(c => ({
      pattern: c.pattern,
      raw_command: c.raw,
      sample_output: c.full_output,
    })),
  };

  try {
    const resp = await fetch('/api/import/generate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });

    if (!resp.ok) {
      const errData = await resp.json().catch(() => ({ error: resp.statusText }));
      throw new Error(errData.error || resp.statusText);
    }

    // SSE stream via ReadableStream (same technique as generateRule)
    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    let created = 0, failed = 0, skipped = 0;

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });

      const lines = buffer.split('\n');
      buffer = lines.pop();

      for (const line of lines) {
        if (!line.startsWith('data: ')) continue;
        try {
          const ev = JSON.parse(line.slice(6));

          switch (ev.type) {
            case 'start':
              if (statusEl) statusEl.innerHTML = `⏳ Processing ${ev.total} command(s)...`;
              break;

            case 'pending':
              if (statusEl) statusEl.innerHTML = `⏳ [${ev.index}/${ev.total}] Generating: <code>${escHtml(ev.command)}</code>`;
              appendGenLog(logEl, 'pending', `⏳ [${ev.index}/${ev.total}] ${ev.command} — calling LLM...`);
              break;

            case 'success':
              created++;
              appendGenLog(logEl, 'success', `✓ [${ev.index}/${ev.total}] ${ev.command} — rule #${ev.rule_id} (${(ev.confidence * 100).toFixed(0)}% confidence)`);
              break;

            case 'skipped':
              skipped++;
              appendGenLog(logEl, 'skipped', `— [${ev.index}/${ev.total}] ${ev.command} — ${ev.message}`);
              break;

            case 'failed':
              failed++;
              appendGenLog(logEl, 'failed', `✕ [${ev.index}/${ev.total}] ${ev.command} — ${ev.error}`);
              break;

            case 'done':
              if (statusEl) statusEl.innerHTML = `✅ ${ev.message}`;
              Toast.success(ev.message);
              showGenerateResults(created, failed, skipped, ev.total);
              break;
          }
        } catch (parseErr) {
          // ignore parse errors from incomplete SSE lines
        }
      }
    }
  } catch (e) {
    Toast.error('Batch generate failed: ' + e.message);
    if (statusEl) statusEl.innerHTML = `<span style="color:var(--danger)">✕ ${escHtml(e.message)}</span>`;
  } finally {
    if (btn) { btn.disabled = false; btn.innerHTML = '⚡ Generate Rules'; }
  }
}

function appendGenLog(logEl, type, text) {
  if (!logEl) return;
  const line = document.createElement('div');
  line.className = 'gen-log-line ' + type;
  line.textContent = text;
  logEl.appendChild(line);
  logEl.scrollTop = logEl.scrollHeight;
}

function showGenerateResults(created, failed, skipped, total) {
  const el = document.getElementById('generate-results');
  if (!el) return;
  el.style.display = '';
  el.innerHTML = `
    <div style="display:grid;grid-template-columns:repeat(4,1fr);gap:12px;margin-top:16px">
      <div class="info-item"><div class="label">Total</div><div class="value">${total}</div></div>
      <div class="info-item"><div class="label">Created</div><div class="value" style="color:var(--success)">${created}</div></div>
      <div class="info-item"><div class="label">Failed</div><div class="value" style="color:var(--danger)">${failed}</div></div>
      <div class="info-item"><div class="label">Skipped</div><div class="value" style="color:var(--text-muted)">${skipped}</div></div>
    </div>
    ${created > 0 ? '<div style="margin-top:16px;text-align:center"><a href="/rules" class="btn btn-primary">📋 View Created Rules →</a></div>' : ''}
  `;
}

// ══════════════════════════════════════════════════════════════════════════
// Phase 2 — Manual Add Command
// ══════════════════════════════════════════════════════════════════════════

function toggleManualAdd() {
  const body = document.getElementById('manual-add-body');
  const toggle = document.getElementById('manual-add-toggle');
  if (!body || !toggle) return;
  if (body.style.display === 'none') {
    body.style.display = '';
    toggle.style.display = 'none';
  } else {
    body.style.display = 'none';
    toggle.style.display = '';
  }
}

async function manualAddCommand() {
  const vendor = document.getElementById('manual-vendor')?.value || '';
  const command = document.getElementById('manual-command')?.value || '';
  const output = document.getElementById('manual-output')?.value || '';
  const resultEl = document.getElementById('manual-add-result');
  const btn = document.getElementById('btn-manual-add');

  if (!vendor) { Toast.warning('Select a vendor'); return; }
  if (!command.trim()) { Toast.warning('Enter a command'); return; }
  if (!output.trim()) { Toast.warning('Paste command output (required for rule generation)'); return; }

  if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spinner"></span> Adding...'; }

  try {
    const resp = await fetch('/api/import/manual', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ vendor, command: command.trim(), output }),
    });
    const data = await resp.json();
    if (data.error) { Toast.error(data.error); if (resultEl) resultEl.innerHTML = `<span style="color:var(--danger)">✕ ${escHtml(data.error)}</span>`; return; }

    if (data.status === 'exists') {
      Toast.warning(data.message);
      if (resultEl) resultEl.innerHTML = `<span style="color:var(--warning)">⚠ ${escHtml(data.message)} — <a href="/rule/${data.rule_id}">View Rule →</a></span>`;
    } else {
      Toast.success(data.message);
      if (resultEl) resultEl.innerHTML = `<span style="color:var(--success)">✓ Pattern: <code>${escHtml(data.pattern)}</code> — <a href="/unknown">View in Unknown Outputs →</a></span>`;
      // Clear inputs
      document.getElementById('manual-command').value = '';
      document.getElementById('manual-output').value = '';
      updateUnknownBadge(1);
    }
  } catch (e) {
    Toast.error('Manual add failed: ' + e.message);
    if (resultEl) resultEl.innerHTML = `<span style="color:var(--danger)">✕ ${escHtml(e.message)}</span>`;
  } finally {
    if (btn) { btn.disabled = false; btn.innerHTML = '➕ Add to Unknown Outputs'; }
  }
}

// ══════════════════════════════════════════════════════════════════════════
// Phase 2 — Unknown Page Batch Select + Generate
// ══════════════════════════════════════════════════════════════════════════

function toggleUnknownSelect(id) {
  const cb = document.getElementById('unknown-cb-' + id);
  if (cb) cb.checked = !cb.checked;
  updateUnknownBatchCount();
}

function selectAllUnknown(masterCb) {
  const checked = masterCb.checked;
  document.querySelectorAll('.unknown-row-cb').forEach(cb => cb.checked = checked);
  updateUnknownBatchCount();
}

function updateUnknownBatchCount() {
  const count = document.querySelectorAll('.unknown-row-cb:checked').length;
  const el = document.getElementById('unknown-batch-count');
  if (el) el.textContent = `${count} selected`;
  const btn = document.getElementById('btn-unknown-batch');
  if (btn) btn.disabled = count === 0;
}

async function batchGenerateUnknown() {
  const checked = document.querySelectorAll('.unknown-row-cb:checked');
  if (checked.length === 0) { Toast.warning('Select at least one command'); return; }

  const ids = [];
  checked.forEach(cb => ids.push(parseInt(cb.dataset.id)));

  const btn = document.getElementById('btn-unknown-batch');
  if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spinner"></span> Generating...'; }

  try {
    const resp = await fetch('/api/unknown/batch-generate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ids }),
    });

    if (!resp.ok) {
      const errData = await resp.json().catch(() => ({ error: resp.statusText }));
      throw new Error(errData.error || resp.statusText);
    }

    // SSE stream
    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    let created = 0, failed = 0;

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop();

      for (const line of lines) {
        if (!line.startsWith('data: ')) continue;
        try {
          const ev = JSON.parse(line.slice(6));
          const row = document.getElementById('unknown-' + ev.unknown_id);

          switch (ev.type) {
            case 'start':
              Toast.info(`Batch generating ${ev.total} rules...`);
              break;
            case 'pending':
              if (row) row.style.background = 'var(--accent-bg)';
              break;
            case 'success':
              created++;
              if (row) {
                row.style.background = 'var(--success-bg)';
                const actions = row.querySelector('td:last-child');
                if (actions) actions.innerHTML = `<span style="color:var(--success)">✓ Rule #${ev.rule_id}</span> <a href="/rule/${ev.rule_id}" class="btn btn-primary btn-sm">View →</a>`;
              }
              updateUnknownBadge(-1);
              break;
            case 'skipped':
              if (row) {
                row.style.background = 'var(--warning-bg)';
                const actions = row.querySelector('td:last-child');
                if (actions) actions.innerHTML = `<span style="color:var(--warning)">— ${escHtml(ev.message)}</span>`;
              }
              break;
            case 'failed':
              failed++;
              if (row) {
                row.style.background = 'var(--danger-bg)';
                const actions = row.querySelector('td:last-child');
                if (actions) actions.innerHTML = `<span style="color:var(--danger)">✕ ${escHtml(ev.error)}</span>`;
              }
              break;
            case 'done':
              Toast.success(ev.message);
              break;
          }
        } catch (parseErr) { /* ignore incomplete SSE lines */ }
      }
    }
  } catch (e) {
    Toast.error('Batch generate failed: ' + e.message);
  } finally {
    if (btn) { btn.disabled = false; btn.innerHTML = '⚡ Batch Generate Selected'; }
    updateUnknownBatchCount();
  }
}

// ══════════════════════════════════════════════════════════════════════════
// Phase 2 — Pattern Editor (inline editing in command review list)
// ══════════════════════════════════════════════════════════════════════════

function editPattern(index) {
  if (!batchAnalysis || !batchAnalysis.commands) return;
  const cmd = batchAnalysis.commands[index];
  const rows = document.querySelectorAll('.cmd-row');
  const row = rows[index];
  if (!row) return;

  // Find the pattern cell (second column content)
  const patternCell = row.children[1];
  if (!patternCell) return;

  const currentPattern = cmd.pattern;
  patternCell.innerHTML = `
    <div style="display:flex;align-items:center;gap:4px">
      <input type="text" class="pattern-edit-input" value="${escHtml(currentPattern)}"
        style="flex:1;font-family:var(--font-mono);font-size:0.82rem;padding:4px 8px;margin:0"
        onkeydown="if(event.key==='Enter'){savePattern(${index},this.value);event.preventDefault()}"
        onblur="savePattern(${index},this.value)"
        placeholder="e.g. display bgp peer {ip}">
      <span style="font-size:0.7rem;color:var(--text-muted)" title="Placeholders: {name} {ip} {id} {interface}">?</span>
    </div>
    <div style="font-size:0.68rem;color:var(--text-muted);margin-top:2px">Placeholders: {name} {ip} {id} {interface} · Enter to save</div>
  `;
  patternCell.querySelector('input').focus();
  patternCell.querySelector('input').select();
}

function savePattern(index, newPattern) {
  if (!batchAnalysis || !batchAnalysis.commands) return;
  const trimmed = newPattern.trim();
  if (!trimmed) return; // don't save empty

  const cmd = batchAnalysis.commands[index];
  const oldPattern = cmd.pattern;
  cmd.pattern = trimmed;

  // Re-render the row (not the whole list to avoid losing state)
  const rows = document.querySelectorAll('.cmd-row');
  const row = rows[index];
  if (!row) return;

  const patternCell = row.children[1];
  if (!patternCell) return;

  const edited = trimmed !== oldPattern;
  patternCell.innerHTML = `
    <code style="font-weight:600${edited ? ';color:var(--warning)' : ''}">${escHtml(trimmed)}</code>
    ${edited ? '<span class="tag" style="font-size:0.65rem;color:var(--warning);margin-left:4px">edited</span>' : ''}
    <div style="font-size:0.72rem;color:var(--text-muted);margin-top:2px">${escHtml(cmd.raw)}</div>
  `;

  if (edited) Toast.info(`Pattern updated: ${trimmed}`);
}

// Init
document.addEventListener('DOMContentLoaded', function() {
  connectWS();

  // Render sample inputs from JSON to readable text
  renderSampleInputs();

  // Make all .output-preview elements focusable so Ctrl+A selects only their content
  document.querySelectorAll('.output-preview').forEach(el => {
    if (!el.hasAttribute('tabindex')) el.setAttribute('tabindex', '0');
  });
  // Also observe for dynamically added output-preview elements
  const observer = new MutationObserver(mutations => {
    mutations.forEach(m => {
      m.addedNodes.forEach(node => {
        if (node.nodeType === 1) {
          if (node.classList && node.classList.contains('output-preview') && !node.hasAttribute('tabindex')) {
            node.setAttribute('tabindex', '0');
          }
          node.querySelectorAll && node.querySelectorAll('.output-preview').forEach(el => {
            if (!el.hasAttribute('tabindex')) el.setAttribute('tabindex', '0');
          });
        }
      });
    });
  });
  observer.observe(document.body, { childList: true, subtree: true });

  // Auto-upgrade textareas to Monaco if available
  const schemaTextarea = document.querySelector('[name="schema_yaml"]');
  if (schemaTextarea && schemaTextarea.tagName === 'TEXTAREA') {
    schemaTextarea.id = schemaTextarea.id || 'schema-editor';
    // Use plaintext for pipeline DSL, yaml for table schema
    const lang = schemaTextarea.id === 'pipeline-editor' ? 'plaintext' : 'yaml';
    upgradeToMonaco(schemaTextarea.id, lang);
  }
  const codeTextarea = document.querySelector('[name="go_code_draft"]');
  if (codeTextarea && codeTextarea.tagName === 'TEXTAREA') {
    codeTextarea.id = codeTextarea.id || 'code-editor';
    upgradeToMonaco(codeTextarea.id, 'go');
  }
});

// ══════════════════════════════════════════════════════════════════════════
// Coverage Boost — one-click rule generation for uncovered commands
// ══════════════════════════════════════════════════════════════════════════

async function coverageBoostAll(deviceID, vendor) {
  const btn = document.getElementById('btn-cov-boost');
  if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spinner"></span> 生成中...'; }

  const progressEl = document.getElementById('cov-boost-progress');
  const statusEl = document.getElementById('cov-boost-status');
  const logEl = document.getElementById('cov-boost-log');
  if (progressEl) progressEl.style.display = '';
  if (logEl) logEl.innerHTML = '';

  try {
    const formData = new URLSearchParams();
    formData.append('device_id', deviceID);

    const resp = await fetch('/api/coverage/boost', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: formData.toString(),
    });

    if (!resp.ok) {
      const errData = await resp.json().catch(() => ({ error: resp.statusText }));
      throw new Error(errData.error || resp.statusText);
    }

    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop();

      for (const line of lines) {
        if (!line.startsWith('data: ')) continue;
        try {
          const ev = JSON.parse(line.slice(6));
          switch (ev.type) {
            case 'start':
              if (statusEl) statusEl.innerHTML = `⏳ 正在处理 ${ev.total} 条未覆盖命令...`;
              break;
            case 'pending':
              if (statusEl) statusEl.innerHTML = `⏳ [${ev.index}/${ev.total}] 生成中: <code>${escHtml(ev.command)}</code>`;
              appendCovBoostLog(logEl, 'pending', `⏳ [${ev.index}/${ev.total}] ${ev.command} — 调用 LLM...`);
              break;
            case 'success':
              appendCovBoostLog(logEl, 'success', `✓ [${ev.index}/${ev.total}] ${ev.command} — 规则 #${ev.rule_id} (${(ev.confidence * 100).toFixed(0)}%)`);
              break;
            case 'skipped':
              appendCovBoostLog(logEl, 'skipped', `— [${ev.index}/${ev.total}] ${ev.command} — ${ev.message}`);
              break;
            case 'failed':
              appendCovBoostLog(logEl, 'failed', `✕ [${ev.index}/${ev.total}] ${ev.command} — ${ev.error}`);
              break;
            case 'done':
              if (statusEl) statusEl.innerHTML = `✅ ${ev.message}`;
              Toast.success(ev.message);
              break;
          }
        } catch (parseErr) { /* ignore incomplete SSE lines */ }
      }
    }
  } catch (e) {
    Toast.error('Coverage boost failed: ' + e.message);
    if (statusEl) statusEl.innerHTML = `<span style="color:var(--danger)">✕ ${escHtml(e.message)}</span>`;
  } finally {
    if (btn) { btn.disabled = false; btn.innerHTML = '⚡ 一键提高覆盖率'; }
  }
}

function appendCovBoostLog(logEl, cls, html) {
  if (!logEl) return;
  const div = document.createElement('div');
  div.className = 'cov-boost-log-entry ' + cls;
  div.innerHTML = html;
  logEl.appendChild(div);
  logEl.scrollTop = logEl.scrollHeight;
}

// Single command: navigate to Unknown page filtered by this command
function coverageGenOne(btn, command) {
  // Navigate to Unknown outputs page — the user can generate from there
  window.location.href = '/unknown?q=' + encodeURIComponent(command);
}

// ── Pattern Matching (4D) ────────────────────────────────────────────────

// Save model_pattern and os_pattern for a pending rule.
async function savePatterns(ruleId) {
  const modelPattern = document.getElementById('model-pattern')?.value || '.*';
  const osPattern = document.getElementById('os-pattern')?.value || '.*';
  const modelHint = document.getElementById('model-pattern-hint');
  const osHint = document.getElementById('os-pattern-hint');

  // Client-side regex validation
  try { new RegExp(modelPattern); if (modelHint) modelHint.textContent = ''; }
  catch(e) { if (modelHint) { modelHint.textContent = '⚠ Invalid regex: ' + e.message; modelHint.style.color = 'var(--danger)'; } return; }
  try { new RegExp(osPattern); if (osHint) osHint.textContent = ''; }
  catch(e) { if (osHint) { osHint.textContent = '⚠ Invalid regex: ' + e.message; osHint.style.color = 'var(--danger)'; } return; }

  try {
    const result = await apiPost(`/api/rule/${ruleId}/save-patterns`, {
      model_pattern: modelPattern,
      os_pattern: osPattern,
    });
    if (result.error) { Toast.error(result.error); return; }
    Toast.success('Patterns saved');
  } catch(e) { Toast.error('Save failed: ' + e.message); }
}

// Live regex validation for pattern inputs
document.addEventListener('DOMContentLoaded', function() {
  ['model-pattern', 'os-pattern'].forEach(function(id) {
    const input = document.getElementById(id);
    const hint = document.getElementById(id + '-hint');
    if (!input || !hint) return;
    input.addEventListener('input', function() {
      try {
        new RegExp(input.value || '.*');
        hint.textContent = '';
      } catch(e) {
        hint.textContent = '⚠ ' + e.message;
        hint.style.color = 'var(--danger)';
      }
    });
  });
});
