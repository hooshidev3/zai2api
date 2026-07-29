// accounts.js — Account CRUD UI

const API = '/api/v1';
let authToken = '';

// Try to get auth token from cookie (for API calls from dashboard)
function getAuthToken() {
    if (authToken) return authToken;
    // Try cookie
    const match = document.cookie.match(/dashboard_token=([^;]+)/);
    if (match) return match[1];
    // Fallback: prompt user
    return '';
}

function authHeaders() {
    const token = getAuthToken();
    const h = { 'Content-Type': 'application/json' };
    if (token) h['Authorization'] = 'Bearer ' + token;
    return h;
}

async function loadAccounts() {
    try {
        const res = await fetch(`${API}/accounts`, { headers: authHeaders() });
        if (!res.ok) return;
        const data = await res.json();
        const accounts = data.accounts || [];
        renderAccounts('glm', accounts.filter(a => a.provider === 'glm'));
        renderAccounts('mimo', accounts.filter(a => a.provider === 'mimo'));
        // Update badge
        const badge = document.getElementById('accounts-badge');
        if (badge) badge.textContent = accounts.length;
    } catch (err) {
        console.error('loadAccounts error:', err);
    }
}

function renderAccounts(provider, accounts) {
    const tbody = document.querySelector(`#${provider}-accounts tbody`);
    if (!tbody) return;

    if (accounts.length === 0) {
        const colspan = provider === 'glm' ? 9 : 8;
        tbody.innerHTML = `<tr><td colspan="${colspan}" style="text-align:center;color:var(--text-muted)">No accounts. Click "Add ${provider === 'glm' ? 'Z.AI' : 'MiMo'} Account" to create one.</td></tr>`;
        return;
    }

    tbody.innerHTML = accounts.map(a => {
        const status = !a.enabled ? '✕' : (a.err_count > 3 ? '⚠' : '●');
        const statusColor = !a.enabled ? 'var(--text-muted)' : (a.err_count > 3 ? 'var(--accent-yellow)' : 'var(--accent-green)');
        const proxyInfo = a.has_proxy ? `${a.proxy_type}://${a.proxy_host}:${a.proxy_port}` : '-';
        const tokenCol = provider === 'glm' ? (a.zai_token_mask || '-') : (a.service_token ? '•••••' : '-');

        const actions = `
            <button class="btn btn-sm" onclick="testAccount('${a.id}')">Test</button>
            <button class="btn btn-sm" onclick="toggleAccount('${a.id}', ${!a.enabled})">${a.enabled ? 'Disable' : 'Enable'}</button>
            <button class="btn btn-sm btn-danger" onclick="deleteAccount('${a.id}')">Delete</button>
        `;

        if (provider === 'glm') {
            return `
                <tr>
                    <td style="color:${statusColor}">${status}</td>
                    <td>${a.id}</td>
                    <td>${a.display_name || '-'}</td>
                    <td><code>${tokenCol}</code></td>
                    <td>${proxyInfo}</td>
                    <td>${a.req_count}</td>
                    <td>${a.err_count}</td>
                    <td>${a.avg_latency_ms}ms</td>
                    <td>${actions}</td>
                </tr>
            `;
        } else {
            return `
                <tr>
                    <td style="color:${statusColor}">${status}</td>
                    <td>${a.id}</td>
                    <td>${a.display_name || '-'}</td>
                    <td><code>${tokenCol}</code></td>
                    <td>${proxyInfo}</td>
                    <td>${a.req_count}</td>
                    <td>${a.err_count}</td>
                    <td>${actions}</td>
                </tr>
            `;
        }
    }).join('');
}

async function testAccount(id) {
    try {
        const res = await fetch(`${API}/accounts/${id}/test`, { method: 'POST', headers: authHeaders() });
        const result = await res.json();
        const msg = `Proxy: ${result.proxy_status || 'N/A'}${result.proxy_error ? ' (' + result.proxy_error + ')' : ''}\n` +
                    `Provider: ${result.provider_status || 'N/A'}${result.provider_error ? ' (' + result.provider_error + ')' : ''}\n` +
                    `Overall: ${result.overall}`;
        alert(msg);
    } catch (err) {
        alert('Test failed: ' + err.message);
    }
}

async function toggleAccount(id, enabled) {
    try {
        await fetch(`${API}/accounts/${id}/toggle`, {
            method: 'POST',
            headers: authHeaders(),
            body: JSON.stringify({ enabled })
        });
        loadAccounts();
    } catch (err) {
        alert('Toggle failed: ' + err.message);
    }
}

async function deleteAccount(id) {
    if (!confirm(`Delete account "${id}"? This cannot be undone.`)) return;
    try {
        await fetch(`${API}/accounts/${id}`, { method: 'DELETE', headers: authHeaders() });
        loadAccounts();
    } catch (err) {
        alert('Delete failed: ' + err.message);
    }
}

function openAddAccountModal(provider) {
    document.getElementById('account-provider').value = provider;
    document.getElementById('modal-title').textContent = `Add ${provider === 'glm' ? 'Z.AI' : 'MiMo'} Account`;
    document.getElementById('glm-fields').classList.toggle('hidden', provider !== 'glm');
    document.getElementById('mimo-fields').classList.toggle('hidden', provider !== 'mimo');
    // Reset form
    document.getElementById('account-form').reset();
    document.querySelector('input[name="proxy-type"][value=""]').checked = true;
    document.getElementById('proxy-fields').classList.add('hidden');
    document.getElementById('add-account-modal').classList.remove('hidden');
}

function closeAddAccountModal() {
    document.getElementById('add-account-modal').classList.add('hidden');
}

async function saveAccount() {
    const provider = document.getElementById('account-provider').value;
    const body = {
        id: document.getElementById('account-id').value.trim(),
        provider: provider,
        display_name: document.getElementById('account-display-name').value.trim(),
        notes: document.getElementById('account-notes').value.trim(),
    };

    if (!body.id) {
        alert('Account ID is required');
        return;
    }

    if (provider === 'glm') {
        body.zai_token = document.getElementById('account-zai-token').value.trim();
        if (!body.zai_token) {
            alert('Z.AI Token is required for GLM accounts');
            return;
        }
    } else {
        body.service_token = document.getElementById('account-service-token').value.trim();
        body.user_id = document.getElementById('account-user-id').value.trim();
        body.xiaomichatbot_ph = document.getElementById('account-xiaomichatbot-ph').value.trim();
        if (!body.service_token || !body.user_id || !body.xiaomichatbot_ph) {
            alert('Service Token, User ID, and Xiaomi Chatbot PH are required for MiMo accounts');
            return;
        }
    }

    // Proxy (optional)
    const proxyType = document.querySelector('input[name="proxy-type"]:checked')?.value;
    if (proxyType) {
        const host = document.getElementById('proxy-host').value.trim();
        const port = parseInt(document.getElementById('proxy-port').value);
        if (!host || !port) {
            alert('Proxy host and port are required when proxy type is selected');
            return;
        }
        body.proxy = {
            type: proxyType,
            host: host,
            port: port,
            username: document.getElementById('proxy-username').value.trim(),
            password: document.getElementById('proxy-password').value.trim(),
        };
    }

    try {
        const res = await fetch(`${API}/accounts`, {
            method: 'POST',
            headers: authHeaders(),
            body: JSON.stringify(body),
        });

        if (res.ok) {
            closeAddAccountModal();
            loadAccounts();
        } else {
            const err = await res.json().catch(() => ({}));
            alert('Error: ' + (err.error?.message || JSON.stringify(err)));
        }
    } catch (err) {
        alert('Save failed: ' + err.message);
    }
}
