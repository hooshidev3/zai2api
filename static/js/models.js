// models.js — Models listing and feature configuration

async function loadModels() {
    try {
        const res = await fetch(`${API}/models`, { headers: authHeaders() });
        if (!res.ok) return;
        const data = await res.json();
        renderModels(data.data || []);
    } catch (err) {
        console.error('loadModels error:', err);
    }
}

function renderModels(models) {
    const tbody = document.querySelector('#models-table tbody');
    if (!tbody) return;

    if (!models || models.length === 0) {
        tbody.innerHTML = '<tr><td colspan="8" style="text-align:center;color:var(--text-muted)">No models available.</td></tr>';
        return;
    }

    tbody.innerHTML = models.map(m => {
        const provider = m._provider || (m.owned_by === 'zai' ? 'glm' : 'mimo');
        const isGLM = provider === 'glm';

        if (isGLM) {
            return `
                <tr>
                    <td><code>${m.id}</code></td>
                    <td><span class="badge badge-glm">${provider}</span></td>
                    <td>${m.owned_by || '-'}</td>
                    <td><input type="checkbox" data-model="${m.id}" data-feature="enable_thinking" checked></td>
                    <td><input type="checkbox" data-model="${m.id}" data-feature="web_search"></td>
                    <td><input type="checkbox" data-model="${m.id}" data-feature="auto_web_search"></td>
                    <td><input type="checkbox" data-model="${m.id}" data-feature="preview_mode"></td>
                    <td><button class="btn btn-sm" onclick="saveFeatures('${m.id}')">Save</button></td>
                </tr>
            `;
        } else {
            return `
                <tr>
                    <td><code>${m.id}</code></td>
                    <td><span class="badge badge-mimo">${provider}</span></td>
                    <td>${m.owned_by || '-'}</td>
                    <td colspan="4" style="text-align:center;color:var(--text-muted)">Feature config not available for MiMo</td>
                    <td>-</td>
                </tr>
            `;
        }
    }).join('');
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
