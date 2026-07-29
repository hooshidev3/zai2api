// sse.js — SSE client for live stats (no location.reload on disconnect)
// EventSource auto-reconnects; we just update the UI status indicator.

const evt = new EventSource('/api/v1/stats/stream');

evt.addEventListener('open', () => setConnectionStatus('connected'));

evt.addEventListener('error', () => {
    setConnectionStatus('reconnecting');
    // EventSource will auto-reconnect — no action needed
});

evt.addEventListener('stats', (e) => {
    setConnectionStatus('connected');
    try {
        const data = JSON.parse(e.data);
        if (window.updateKPIs) updateKPIs(data.kpis);
        if (window.updateAccountsTable) updateAccountsTable(data.accounts);
        if (window.updateRecentRequests) updateRecentRequests(data.recent_requests);
        if (window.updateCharts) updateCharts(data.kpis);
    } catch (err) {
        console.error('SSE parse error:', err);
    }
});

function setConnectionStatus(status) {
    const el = document.getElementById('connection-status');
    if (!el) return;
    if (status === 'connected') {
        el.className = 'status-connected';
        el.textContent = '● Live';
    } else {
        el.className = 'status-reconnecting';
        el.textContent = '○ Reconnecting...';
    }
}
