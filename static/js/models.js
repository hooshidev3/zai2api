// models.js — Models listing, feature configuration, refresh, auto-refresh

// State for auto-refresh and last-updated display
let modelsAutoRefreshTimer = null;
let modelsLastUpdated = null;

async function loadModels() {
    try {
        const res = await fetch(`${API}/models`, { headers: authHeaders() });
        if (!res.ok) return;
        const data = await res.json();
        renderModels(data.data || []);
        modelsLastUpdated = data.generated_at || new Date().toISOString();
        updateModelsLastUpdated();
    } catch (err) {
        console.error('loadModels error:', err);
    }
}

function renderModels(models) {
    const tbody = document.querySelector('#models-table tbody');
    if (!tbody) return;

    if (!models || models.length === 0) {
        tbody.innerHTML = '<tr><td colspan="9" style="text-align:center;color:var(--text-muted)">No models available.</td></tr>';
        return;
    }

    tbody.innerHTML = models.map(m => {
        const provider = m._provider || (m.owned_by === 'zai' ? 'glm' : 'mimo');
        const isGLM = provider === 'glm';

        // Capabilities badges (from fetchModelsWithDetails)
        const caps = m.capabilities || {};
        const capBadges = [];
        if (caps.vision) capBadges.push('<span class="badge badge-glm" title="Vision input">👁 vision</span>');
        if (caps.web_search || caps.webSearch) capBadges.push('<span class="badge badge-glm" title="Web search">🔍 web</span>');
        if (caps.think || caps.thinking || caps.enable_thinking) capBadges.push('<span class="badge badge-glm" title="Reasoning / thinking">🧠 think</span>');
        if (caps.agent_mode || caps.mcp) capBadges.push('<span class="badge badge-glm" title="Agent / MCP">⚡ agent</span>');
        if (caps.file_qa) capBadges.push('<span class="badge badge-glm" title="File QA">📄 file</span>');
        const capsHtml = capBadges.length > 0
            ? `<div style="display:flex;flex-wrap:wrap;gap:4px;margin-top:4px">${capBadges.join('')}</div>`
            : '';

        // Name column: show display name + capabilities + description
        const nameDisplay = m.name && m.name !== m.id
            ? `<code>${m.id}</code><div style="font-size:11px;color:var(--text-muted);margin-top:2px">${escapeHtml(m.name)}</div>`
            : `<code>${m.id}</code>`;
        const descHtml = m.description
            ? `<div style="font-size:11px;color:var(--text-muted);margin-top:2px">${escapeHtml(m.description)}</div>`
            : '';

        if (isGLM) {
            return `
                <tr>
                    <td>${nameDisplay}${capsHtml}${descHtml}</td>
                    <td><span class="badge badge-glm">${provider}</span></td>
                    <td>${m.owned_by || '-'}</td>
                    <td><input type="checkbox" data-model="${m.id}" data-feature="enable_thinking" ${caps.think || caps.thinking || caps.enable_thinking ? 'checked' : ''}></td>
                    <td><input type="checkbox" data-model="${m.id}" data-feature="web_search" ${caps.web_search || caps.webSearch ? 'checked' : ''}></td>
                    <td><input type="checkbox" data-model="${m.id}" data-feature="auto_web_search"></td>
                    <td><input type="checkbox" data-model="${m.id}" data-feature="preview_mode"></td>
                    <td><button class="btn btn-sm" onclick="saveFeatures('${m.id}')">Save</button></td>
                </tr>
            `;
        } else {
            return `
                <tr>
                    <td>${nameDisplay}${capsHtml}${descHtml}</td>
                    <td><span class="badge badge-mimo">${provider}</span></td>
                    <td>${m.owned_by || '-'}</td>
                    <td colspan="4" style="text-align:center;color:var(--text-muted)">Feature config not available for MiMo</td>
                    <td>-</td>
                </tr>
            `;
        }
    }).join('');
}

// refreshModels — calls POST /api/v1/models/refresh and reloads the list
async function refreshModels() {
    const btn = document.getElementById('btn-refresh-models');
    if (btn) {
        btn.disabled = true;
        btn.textContent = '⏳ Refreshing...';
    }

    try {
        // Try the refresh endpoint (invalidates GLM cache). If GLM is not
        // initialized, it returns 503 — fall back to a normal GET.
        const refreshRes = await fetch(`${API}/models/refresh`, {
            method: 'POST',
            headers: authHeaders(),
        });
        if (refreshRes.ok) {
            const data = await refreshRes.json();
            // Refresh returns just IDs; do a full GET to get details.
            await loadModels();
            showModelsToast(`✓ Refreshed ${data.model_count || 0} models`, 'ok');
        } else {
            // Fallback: just reload
            await loadModels();
            showModelsToast('✓ Reloaded models', 'ok');
        }
    } catch (err) {
        showModelsToast('✗ Refresh failed: ' + err.message, 'err');
    } finally {
        if (btn) {
            btn.disabled = false;
            btn.textContent = '🔄 Refresh';
        }
    }
}

// toggleModelsAutoRefresh — starts/stops a 60s auto-refresh timer
function toggleModelsAutoRefresh() {
    const btn = document.getElementById('btn-auto-refresh-models');
    if (modelsAutoRefreshTimer) {
        clearInterval(modelsAutoRefreshTimer);
        modelsAutoRefreshTimer = null;
        if (btn) {
            btn.textContent = '⏸ Auto-Refresh: OFF';
            btn.classList.remove('btn-active');
        }
        showModelsToast('Auto-refresh stopped', 'ok');
    } else {
        modelsAutoRefreshTimer = setInterval(() => {
            // Silent refresh — no toast, no button state change
            loadModels();
        }, 60000);
        if (btn) {
            btn.textContent = '▶ Auto-Refresh: ON';
            btn.classList.add('btn-active');
        }
        showModelsToast('Auto-refresh started (every 60s)', 'ok');
    }
}

function updateModelsLastUpdated() {
    const el = document.getElementById('models-last-updated');
    if (el && modelsLastUpdated) {
        const d = new Date(modelsLastUpdated);
        el.textContent = 'Last updated: ' + d.toLocaleTimeString();
    }
}

function showModelsToast(msg, kind) {
    const el = document.getElementById('models-toast');
    if (!el) return;
    el.textContent = msg;
    el.style.color = kind === 'err' ? 'var(--accent-red)' : 'var(--accent-green)';
    el.style.display = 'block';
    setTimeout(() => { el.style.display = 'none'; }, 2500);
}

// escapeHtml prevents XSS when rendering model names/descriptions from the API
function escapeHtml(s) {
    if (!s) return '';
    return String(s)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#039;');
}

async function saveFeatures(modelId) {
    const overrides = {};
    document.querySelectorAll(`input[data-model="${modelId}"]`).forEach(cb => {
        overrides[cb.dataset.feature] = cb.checked;
    });

    try {
        const res = await fetch(`${API}/models/${modelId}/features`, {
            method: 'PUT',
            headers: authHeaders(),
            body: JSON.stringify({ include_all: false, overrides }),
        });

        if (res.ok) {
            // Brief visual feedback
            const btn = event.target;
            const origText = btn.textContent;
            btn.textContent = '✓ Saved';
            btn.style.color = 'var(--accent-green)';
            setTimeout(() => {
                btn.textContent = origText;
                btn.style.color = '';
            }, 1500);
        } else {
            alert('Failed to save features');
        }
    } catch (err) {
        alert('Save failed: ' + err.message);
    }
}
