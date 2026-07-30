// providers.js — Provider status cards with Note field

async function loadProviders() {
    try {
        const res = await fetch(`${API}/providers/status`, { headers: authHeaders() });
        if (!res.ok) return;
        const data = await res.json();
        renderProviders(data.providers || []);
    } catch (err) {
        console.error('loadProviders error:', err);
    }
}

function renderProviders(providers) {
    const container = document.getElementById('providers-container');
    if (!container) return;

    if (providers.length === 0) {
        container.innerHTML = '<p style="color:var(--text-muted)">No providers available.</p>';
        return;
    }

    container.innerHTML = providers.map(p => {
        const statusColor = p.status === 'ready' ? 'var(--accent-green)' :
                           p.status === 'degraded' ? 'var(--accent-yellow)' :
                           'var(--accent-red)';
        const statusIcon = p.status === 'ready' ? '●' :
                          p.status === 'degraded' ? '⚠' : '✕';

        const models = p.details?.models || [];
        const modelsHtml = models.length > 0
            ? `<div style="margin-top:12px;font-size:12px;color:var(--text-muted)">Models: ${models.map(m => `<code>${m}</code>`).join(' ')}</div>`
            : '';

        // Note field — shown as a highlighted callout below the status line.
        // Only render if non-empty (backend omits it when empty via omitempty).
        const noteHtml = p.note
            ? `<div style="margin-top:8px;padding:8px 12px;background:var(--bg-secondary);border-left:3px solid ${statusColor};border-radius:4px;font-size:12px;color:var(--text-muted)">
                   <strong>ℹ Note:</strong> ${escapeHtmlProvider(p.note)}
               </div>`
            : '';

        return `
            <div style="background:var(--bg-tertiary);border:1px solid var(--border);border-radius:6px;padding:20px">
                <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:16px">
                    <h4 style="margin:0;font-size:16px">${p.name}</h4>
                    <span style="color:${statusColor};font-size:14px;font-weight:500">
                        ${statusIcon} ${p.status}
                    </span>
                </div>
                <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px;font-size:13px">
                    <div>
                        <div style="color:var(--text-muted);font-size:11px;text-transform:uppercase">Uptime</div>
                        <div>${p.uptime}</div>
                    </div>
                    <div>
                        <div style="color:var(--text-muted);font-size:11px;text-transform:uppercase">Accounts</div>
                        <div>${p.active_count}/${p.account_count} active</div>
                    </div>
                    <div>
                        <div style="color:var(--text-muted);font-size:11px;text-transform:uppercase">Requests</div>
                        <div>${(p.total_requests || 0).toLocaleString()}</div>
                    </div>
                    <div>
                        <div style="color:var(--text-muted);font-size:11px;text-transform:uppercase">Errors</div>
                        <div>${p.total_errors || 0}</div>
                    </div>
                    <div>
                        <div style="color:var(--text-muted);font-size:11px;text-transform:uppercase">Avg Latency</div>
                        <div>${p.avg_latency_ms || 0}ms</div>
                    </div>
                </div>
                ${noteHtml}
                ${modelsHtml}
            </div>
        `;
    }).join('');
}

// escapeHtmlProvider prevents XSS when rendering the provider note
function escapeHtmlProvider(s) {
    if (!s) return '';
    return String(s)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#039;');
}
