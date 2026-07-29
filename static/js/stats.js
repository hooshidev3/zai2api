// stats.js — Detailed statistics with filtering and CSV export

async function loadDetailedStats() {
    const provider = document.getElementById('stats-provider-filter')?.value || '';
    const range = document.getElementById('stats-time-range')?.value || '24h';

    const params = new URLSearchParams({ range });
    if (provider) params.set('provider', provider);

    try {
        const res = await fetch(`${API}/stats/detailed?${params}`, { headers: authHeaders() });
        if (!res.ok) return;
        const data = await res.json();
        renderModelStats(data.models || []);
    } catch (err) {
        console.error('loadDetailedStats error:', err);
    }
}

function renderModelStats(models) {
    const tbody = document.querySelector('#stats-model-table tbody');
    if (!tbody) return;

    if (!models || models.length === 0) {
        tbody.innerHTML = '<tr><td colspan="6" style="text-align:center;color:var(--text-muted)">No requests in this time range.</td></tr>';
        return;
    }

    tbody.innerHTML = models.map(m => `
        <tr>
            <td><code>${m.model}</code></td>
            <td><span class="badge badge-${m.provider}">${m.provider}</span></td>
            <td>${(m.requests || 0).toLocaleString()}</td>
            <td>${(m.tokens || 0).toLocaleString()}</td>
            <td>${m.avg_latency_ms || 0}ms</td>
            <td>${(m.error_rate || 0).toFixed(1)}%</td>
        </tr>
    `).join('');
}

function exportStatsCSV() {
    const provider = document.getElementById('stats-provider-filter')?.value || '';
    const range = document.getElementById('stats-time-range')?.value || '24h';
    const params = new URLSearchParams({ range });
    if (provider) params.set('provider', provider);

    // Use fetch + blob for proper auth header support
    fetch(`${API}/stats/export?${params}`, { headers: authHeaders() })
        .then(res => res.blob())
        .then(blob => {
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = `stats_${range}.csv`;
            a.click();
            URL.revokeObjectURL(url);
        })
        .catch(err => alert('Export failed: ' + err.message));
}

// Filter change handlers
document.addEventListener('DOMContentLoaded', () => {
    const provFilter = document.getElementById('stats-provider-filter');
    const rangeFilter = document.getElementById('stats-time-range');
    if (provFilter) provFilter.addEventListener('change', loadDetailedStats);
    if (rangeFilter) rangeFilter.addEventListener('change', loadDetailedStats);
});
