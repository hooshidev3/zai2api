// app.js — KPI updates, tab switching, utilities

function updateKPIs(kpis) {
    if (!kpis) return;
    setText('kpi-requests', formatNumber(kpis.total_requests || 0));
    setText('kpi-latency', (kpis.avg_latency_ms || 0) + 'ms');
    const errRate = kpis.total_requests > 0
        ? ((kpis.total_errors / kpis.total_requests) * 100).toFixed(1)
        : '0.0';
    setText('kpi-errors', errRate + '%');
    setText('kpi-uptime', formatUptime(kpis.uptime_seconds || 0));
}

function updateRecentRequests(reqs) {
    const tbody = document.querySelector('#recent-requests tbody');
    if (!tbody) return;
    if (!reqs || reqs.length === 0) {
        tbody.innerHTML = '<tr><td colspan="6" style="text-align:center;color:var(--text-muted)">No requests yet.</td></tr>';
        return;
    }
    tbody.innerHTML = reqs.map(r => `
        <tr>
            <td>${new Date(r.time).toLocaleTimeString()}</td>
            <td><span class="badge badge-${r.provider || 'glm'}">${r.provider || '-'}</span></td>
            <td>${r.model || '-'}</td>
            <td>${r.account || '-'}</td>
            <td>${r.duration_ms}ms</td>
            <td>${statusBadge(r.status)}</td>
        </tr>
    `).join('');
}

function updateAccountsTable(accounts) {
    // Update from SSE snapshot (but full load is done by accounts.js)
    // This just updates the badge count
    const glmCount = (accounts.glm || []).length;
    const mimoCount = (accounts.mimo || []).length;
    const badge = document.getElementById('accounts-badge');
    if (badge) badge.textContent = glmCount + '+' + mimoCount;
}

function statusBadge(status) {
    if (status >= 200 && status < 300) return `<span class="status-ok">${status} ✓</span>`;
    if (status === 0) return `<span class="status-err">—</span>`;
    return `<span class="status-err">${status} ✗</span>`;
}

function formatNumber(n) {
    return Number(n).toLocaleString();
}

function formatUptime(seconds) {
    const d = Math.floor(seconds / 86400);
    const h = Math.floor((seconds % 86400) / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    if (d > 0) return `${d}d ${h}h`;
    if (h > 0) return `${h}h ${m}m`;
    return `${m}m`;
}

function setText(id, val) {
    const el = document.getElementById(id);
    if (el) el.textContent = val;
}

// Tab switching
document.querySelectorAll('.nav-item').forEach(item => {
    item.addEventListener('click', (e) => {
        e.preventDefault();
        const tab = item.dataset.tab;
        document.querySelectorAll('.nav-item').forEach(n => n.classList.remove('active'));
        document.querySelectorAll('.tab-content').forEach(t => t.classList.remove('active'));
        item.classList.add('active');
        const panel = document.getElementById(tab);
        if (panel) panel.classList.add('active');

        // Lazy-load tab content
        if (tab === 'accounts' && typeof loadAccounts === 'function') loadAccounts();
        if (tab === 'models' && typeof loadModels === 'function') loadModels();
        if (tab === 'providers' && typeof loadProviders === 'function') loadProviders();
        if (tab === 'agents' && typeof loadAgents === 'function') loadAgents();
        if (tab === 'stats' && typeof loadDetailedStats === 'function') loadDetailedStats();
        if (tab === 'settings') {
            if (typeof loadAliases === 'function') loadAliases();
            if (typeof loadRateLimits === 'function') loadRateLimits();
        }
    });
});

// Proxy radio buttons — show/hide proxy fields
document.querySelectorAll('input[name="proxy-type"]').forEach(radio => {
    radio.addEventListener('change', () => {
        const proxyFields = document.getElementById('proxy-fields');
        const val = document.querySelector('input[name="proxy-type"]:checked').value;
        if (val) {
            proxyFields.classList.remove('hidden');
        } else {
            proxyFields.classList.add('hidden');
        }
    });
});
