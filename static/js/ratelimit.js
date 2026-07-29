// ratelimit.js — Rate Limit CRUD UI

async function loadRateLimits() {
    try {
        const res = await fetch(`${API}/models/rate-limits`, { headers: authHeaders() });
        if (!res.ok) return;
        const data = await res.json();
        renderRateLimits(data.rate_limits || []);
    } catch (err) {
        console.error('loadRateLimits error:', err);
    }
}

function renderRateLimits(limits) {
    const tbody = document.querySelector('#ratelimits-table tbody');
    if (!tbody) return;

    if (limits.length === 0) {
        tbody.innerHTML = '<tr><td colspan="5" style="text-align:center;color:var(--text-muted)">No rate limits configured. Click "Add Rate Limit" to create one.</td></tr>';
        return;
    }

    tbody.innerHTML = limits.map(l => `
        <tr>
            <td><code>${l.model}</code></td>
            <td>${l.max_rpm || '∞'}</td>
            <td>${l.max_tpm || '∞'}</td>
            <td>${l.max_context || '∞'}</td>
            <td>
                <button class="btn btn-sm" onclick="editRateLimit('${l.model}', ${l.max_rpm}, ${l.max_tpm}, ${l.max_context})">Edit</button>
                <button class="btn btn-danger btn-sm" onclick="deleteRateLimit('${l.model}')">Delete</button>
            </td>
        </tr>
    `).join('');
}

function openAddRateLimitModal() {
    const model = prompt('Model name (e.g., "glm-5.1"):');
    if (!model) return;
    const rpm = parseInt(prompt('Max RPM (0 = unlimited):', '60')) || 0;
    const tpm = parseInt(prompt('Max TPM (0 = unlimited):', '100000')) || 0;
    const ctx = parseInt(prompt('Max context length (0 = unlimited):', '0')) || 0;
    setRateLimit(model, rpm, tpm, ctx);
}

function editRateLimit(model, rpm, tpm, ctx) {
    const newRpm = parseInt(prompt('Max RPM:', String(rpm))) || 0;
    const newTpm = parseInt(prompt('Max TPM:', String(tpm))) || 0;
    const newCtx = parseInt(prompt('Max context:', String(ctx))) || 0;
    setRateLimit(model, newRpm, newTpm, newCtx);
}

async function setRateLimit(model, maxRpm, maxTpm, maxContext) {
    try {
        const res = await fetch(`${API}/models/${model}/rate-limit`, {
            method: 'PUT',
            headers: { ...authHeaders(), 'Content-Type': 'application/json' },
            body: JSON.stringify({ max_rpm: maxRpm, max_tpm: maxTpm, max_context: maxContext }),
        });
        if (res.ok) {
            loadRateLimits();
        } else {
            const err = await res.json().catch(() => ({}));
            alert('Error: ' + (err.error || JSON.stringify(err)));
        }
    } catch (err) {
        alert('Failed: ' + err.message);
    }
}

async function deleteRateLimit(model) {
    if (!confirm(`Delete rate limit for "${model}"?`)) return;
    try {
        await fetch(`${API}/models/${model}/rate-limit`, {
            method: 'DELETE',
            headers: authHeaders(),
        });
        loadRateLimits();
    } catch (err) {
        alert('Delete failed: ' + err.message);
    }
}
