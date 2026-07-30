// accounts.js — Account CRUD UI with mandatory pre-create connection test

const API = '/api/v1';
let authToken = '';

// connectionTestPassed tracks whether the current form values have passed
// a connection test. Any field change invalidates it, forcing a re-test
// before Save is allowed.
let connectionTestPassed = false;

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

    // Reset connection-test state — Save is disabled until a test passes
    connectionTestPassed = false;
    updateSaveButtonState();
    setTestConnectionStatus('Not tested yet', 'muted');
}

function closeAddAccountModal() {
    document.getElementById('add-account-modal').classList.add('hidden');
}

// collectFormValues builds a CreateAccountRequest from the form fields.
// Used by both testConnectionBeforeCreate and saveAccount so they stay
// in sync.
function collectFormValues() {
    const provider = document.getElementById('account-provider').value;
    const body = {
        id: document.getElementById('account-id').value.trim(),
        provider: provider,
        display_name: document.getElementById('account-display-name').value.trim(),
        notes: document.getElementById('account-notes').value.trim(),
    };

    if (provider === 'glm') {
        body.zai_token = document.getElementById('account-zai-token').value.trim();
    } else {
        body.service_token = document.getElementById('account-service-token').value.trim();
        body.user_id = document.getElementById('account-user-id').value.trim();
        body.xiaomichatbot_ph = document.getElementById('account-xiaomichatbot-ph').value.trim();
    }

    const proxyType = document.querySelector('input[name="proxy-type"]:checked')?.value;
    if (proxyType) {
        body.proxy = {
            type: proxyType,
            host: document.getElementById('proxy-host').value.trim(),
            port: parseInt(document.getElementById('proxy-port').value) || 0,
            username: document.getElementById('proxy-username').value.trim(),
            password: document.getElementById('proxy-password').value.trim(),
        };
    }
    return body;
}

// testConnectionBeforeCreate — calls POST /api/v1/accounts/test-connection
// Validates credentials WITHOUT creating the account.
async function testConnectionBeforeCreate() {
    const body = collectFormValues();

    // Basic client-side validation before hitting the API
    if (!body.id) {
        setTestConnectionStatus('Account ID is required', 'err');
        return;
    }
    if (body.provider === 'glm' && !body.zai_token) {
        setTestConnectionStatus('Z.AI Token is required', 'err');
        return;
    }
    if (body.provider === 'mimo' && (!body.service_token || !body.user_id || !body.xiaomichatbot_ph)) {
        setTestConnectionStatus('Service Token, User ID, and Xiaomi Chatbot PH are required', 'err');
        return;
    }
    if (body.proxy && (!body.proxy.host || !body.proxy.port)) {
        setTestConnectionStatus('Proxy host and port are required', 'err');
        return;
    }

    const btn = document.getElementById('btn-test-connection');
    if (btn) {
        btn.disabled = true;
        btn.textContent = '⏳ Testing...';
    }
    setTestConnectionStatus('Testing connection (up to 20s)...', 'muted');

    try {
        const res = await fetch(`${API}/accounts/test-connection`, {
            method: 'POST',
            headers: authHeaders(),
            body: JSON.stringify(body),
        });
        const result = await res.json();

        if (res.ok && result.overall === 'ok') {
            connectionTestPassed = true;
            const latency = result.provider_latency_ms || 0;
            setTestConnectionStatus(`✓ Connection OK (provider: ${latency}ms)`, 'ok');
        } else {
            connectionTestPassed = false;
            const details = result.error?.details || result;
            const parts = [];
            if (details.proxy_status && details.proxy_status !== 'ok') {
                parts.push(`Proxy: ${details.proxy_error || details.proxy_status}`);
            }
            if (details.provider_status && details.provider_status !== 'ok') {
                parts.push(`Provider: ${details.provider_error || details.provider_status}`);
            }
            if (parts.length === 0) {
                parts.push(result.error?.message || 'Connection test failed');
            }
            setTestConnectionStatus('✗ ' + parts.join(' | '), 'err');
        }
    } catch (err) {
        connectionTestPassed = false;
        setTestConnectionStatus('✗ Test failed: ' + err.message, 'err');
    } finally {
        if (btn) {
            btn.disabled = false;
            btn.textContent = '🔌 Test Connection';
        }
        updateSaveButtonState();
    }
}

// invalidateConnectionTest — called on any field change.
// Resets the test state so the user must re-test before saving.
function invalidateConnectionTest() {
    if (connectionTestPassed) {
        connectionTestPassed = false;
        setTestConnectionStatus('Fields changed — re-test required', 'muted');
        updateSaveButtonState();
    }
}

function updateSaveButtonState() {
    const btn = document.getElementById('btn-save-account');
    if (!btn) return;
    if (connectionTestPassed) {
        btn.disabled = false;
        btn.title = '';
    } else {
        btn.disabled = true;
        btn.title = 'Test the connection first (click "Test Connection")';
    }
}

function setTestConnectionStatus(msg, kind) {
    const el = document.getElementById('test-connection-status');
    if (!el) return;
    el.textContent = msg;
    el.style.color = kind === 'ok' ? 'var(--accent-green)' :
                     kind === 'err' ? 'var(--accent-red)' :
                     'var(--text-muted)';
}

async function saveAccount() {
    // Guard: connection test must have passed
    if (!connectionTestPassed) {
        setTestConnectionStatus('Please test the connection first', 'err');
        return;
    }

    const body = collectFormValues();

    try {
        // skip_test=true because we already tested via test-connection endpoint
        const res = await fetch(`${API}/accounts?skip_test=true`, {
            method: 'POST',
            headers: authHeaders(),
            body: JSON.stringify(body),
        });

        if (res.ok) {
            closeAddAccountModal();
            loadAccounts();
            // After adding a GLM account, the model list may change (lazy init).
            // Refresh models in the background so the Models tab is up to date.
            if (body.provider === 'glm' && typeof loadModels === 'function') {
                setTimeout(() => loadModels(), 2000);
            }
        } else {
            const err = await res.json().catch(() => ({}));
            alert('Error: ' + (err.error?.message || JSON.stringify(err)));
        }
    } catch (err) {
        alert('Save failed: ' + err.message);
    }
}

// ── Form field change listeners ─────────────────────────────────────
// Any change to a form field invalidates the connection test result,
// forcing the user to re-test before Save is enabled.
// This listener is attached on DOMContentLoaded so all form fields exist.
document.addEventListener('DOMContentLoaded', () => {
    const fieldIds = [
        'account-id', 'account-display-name', 'account-zai-token',
        'account-service-token', 'account-user-id', 'account-xiaomichatbot-ph',
        'proxy-host', 'proxy-port', 'proxy-username', 'proxy-password',
        'account-notes',
    ];
    fieldIds.forEach(id => {
        const el = document.getElementById(id);
        if (el) {
            el.addEventListener('input', invalidateConnectionTest);
            el.addEventListener('change', invalidateConnectionTest);
        }
    });
    // Proxy type radios
    document.querySelectorAll('input[name="proxy-type"]').forEach(radio => {
        radio.addEventListener('change', invalidateConnectionTest);
    });
});
