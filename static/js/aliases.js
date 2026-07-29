// aliases.js — Model Aliases CRUD

async function loadAliases() {
    try {
        const res = await fetch(`${API}/models/aliases`, { headers: authHeaders() });
        if (!res.ok) return;
        const data = await res.json();
        renderAliases(data.aliases || []);
    } catch (err) {
        console.error('loadAliases error:', err);
    }
}

function renderAliases(aliases) {
    const tbody = document.querySelector('#aliases-table tbody');
    if (!tbody) return;

    if (aliases.length === 0) {
        tbody.innerHTML = '<tr><td colspan="4" style="text-align:center;color:var(--text-muted)">No aliases. Click "Add Alias" to create one.</td></tr>';
        return;
    }

    tbody.innerHTML = aliases.map(a => `
        <tr>
            <td><code>${a.alias}</code></td>
            <td><code>${a.target_model}</code></td>
            <td>${a.created_at ? new Date(a.created_at).toLocaleDateString() : '-'}</td>
            <td><button class="btn btn-danger btn-sm" onclick="deleteAlias('${a.alias}')">Delete</button></td>
        </tr>
    `).join('');
}

function openAddAliasModal() {
    const alias = prompt('Alias name (e.g., "fast"):');
    if (!alias) return;
    const target = prompt('Target model (e.g., "glm-4.5-air"):');
    if (!target) return;
    addAlias(alias, target);
}

async function addAlias(alias, targetModel) {
    try {
        const res = await fetch(`${API}/models/aliases`, {
            method: 'POST',
            headers: { ...authHeaders(), 'Content-Type': 'application/json' },
            body: JSON.stringify({ alias, target_model: targetModel }),
        });
        if (res.ok) {
            loadAliases();
        } else {
            const err = await res.json().catch(() => ({}));
            alert('Error: ' + (err.error || JSON.stringify(err)));
        }
    } catch (err) {
        alert('Failed: ' + err.message);
    }
}

async function deleteAlias(alias) {
    if (!confirm(`Delete alias "${alias}"?`)) return;
    try {
        await fetch(`${API}/models/aliases/${alias}`, {
            method: 'DELETE',
            headers: authHeaders(),
        });
        loadAliases();
    } catch (err) {
        alert('Delete failed: ' + err.message);
    }
}
