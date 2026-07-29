// agents.js — MiMo Agents monitoring with GLM limitation note

async function loadAgents() {
    try {
        const res = await fetch(`${API}/agents`, { headers: authHeaders() });
        if (!res.ok) {
            renderAgents([]);
            return;
        }
        const data = await res.json();
        renderAgents(data.agents || []);
    } catch (err) {
        console.error('loadAgents error:', err);
        renderAgents([]);
    }
}

function renderAgents(agents) {
    const tbody = document.querySelector('#agents-table tbody');
    if (!tbody) return;

    // GLM agent note (shown above the table)
    const noteEl = document.getElementById('glm-agent-note');
    if (noteEl && !noteEl.dataset.rendered) {
        noteEl.innerHTML = `
            <div style="background:rgba(88,166,255,0.1);border:1px solid rgba(88,166,255,0.3);border-radius:6px;padding:12px 16px;margin-bottom:16px;font-size:13px">
                <strong>ℹ️ GLM Agent Mode:</strong>
                GLM از agent loop واقعی پشتیبانی نمی‌کند.
                «Agent mode» در GLM فقط ترجمه tool-calling است
                (OpenAI tools ↔ Z.AI text contract).
                برای agent loop واقعی (planner/critic/executor)،
                از مدل‌های MiMo استفاده کنید.
            </div>
        `;
        noteEl.dataset.rendered = 'true';
    }

    if (!agents || agents.length === 0) {
        tbody.innerHTML = '<tr><td colspan="4" style="text-align:center;color:var(--text-muted)">No active MiMo agents.</td></tr>';
        return;
    }

    tbody.innerHTML = agents.map(a => {
        const id = a.goal_id || a.id || '-';
        const status = a.status || 'unknown';
        const statusColor = status === 'running' ? 'var(--accent-green)' :
                           status === 'done' || status === 'completed' ? 'var(--accent-blue)' :
                           status === 'failed' ? 'var(--accent-red)' : 'var(--text-muted)';
        const started = a.started_at || a.created_at || '';
        const startedStr = started ? new Date(started).toLocaleString() : '-';

        return `
            <tr>
                <td><code>${id}</code></td>
                <td style="color:${statusColor}">${status}</td>
                <td>${startedStr}</td>
                <td>
                    ${status === 'running' ? `<button class="btn btn-sm" onclick="viewAgentStream('${id}')">Stream</button>` : ''}
                </td>
            </tr>
        `;
    }).join('');
}

function viewAgentStream(id) {
    const win = window.open('', '_blank', 'width=800,height=600');
    if (!win) {
        alert('Popup blocked. Please allow popups for this site.');
        return;
    }
    win.document.write(`
        <html><head><title>Agent Stream: ${id}</title>
        <style>body{font-family:monospace;background:#0d1117;color:#c9d1d9;padding:20px}
        pre{white-space:pre-wrap;word-wrap:break-word}</style>
        </head><body>
        <h3>Agent Stream: ${id}</h3>
        <pre id="output">Connecting...</pre>
        <script>
        const evt = new EventSource('${API}/agents/${id}/stream');
        const output = document.getElementById('output');
        output.textContent = '';
        evt.onmessage = (e) => {
            output.textContent += e.data + '\\n';
            output.scrollTop = output.scrollHeight;
        };
        evt.onerror = () => {
            output.textContent += '\\n[Connection closed]\\n';
            evt.close();
        };
        <\/script>
        </body></html>
    `);
    win.document.close();
}
