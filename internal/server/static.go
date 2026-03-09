package server

const styleCSS = `
:root {
    --bg: #0f1117;
    --bg-card: #1a1d27;
    --bg-hover: #222633;
    --border: #2a2e3d;
    --text: #e1e4ed;
    --text-dim: #8b8fa3;
    --accent: #6366f1;
    --accent-hover: #818cf8;
    --green: #22c55e;
    --red: #ef4444;
    --yellow: #eab308;
    --blue: #3b82f6;
    --radius: 8px;
}

* { margin: 0; padding: 0; box-sizing: border-box; }

body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
    background: var(--bg);
    color: var(--text);
    line-height: 1.6;
}

a { color: var(--accent); text-decoration: none; }
a:hover { color: var(--accent-hover); }

.navbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 2rem;
    height: 56px;
    background: var(--bg-card);
    border-bottom: 1px solid var(--border);
    position: sticky;
    top: 0;
    z-index: 100;
}

.nav-brand {
    font-size: 1.25rem;
    font-weight: 700;
    color: var(--text);
    display: flex;
    align-items: center;
    gap: 0.5rem;
}

.nav-brand a { color: var(--text); }

.logo { font-size: 1.5rem; }

.nav-links { display: flex; gap: 1.5rem; }
.nav-links a { color: var(--text-dim); font-size: 0.9rem; }
.nav-links a.active, .nav-links a:hover { color: var(--text); }

.container {
    max-width: 1280px;
    margin: 0 auto;
    padding: 2rem;
}

.card {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 1.25rem;
}

.status-cards {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
    gap: 1rem;
    margin-bottom: 2rem;
}

.stat-card {
    text-align: center;
    transition: border-color 0.2s;
}

.stat-card:hover { border-color: var(--accent); }

.stat-label {
    font-size: 0.8rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-dim);
    margin-bottom: 0.25rem;
}

.stat-value { font-size: 1.5rem; font-weight: 700; }
.text-green { color: var(--green); }
.text-red { color: var(--red); }
.text-yellow { color: var(--yellow); }
.text-blue { color: var(--blue); }

.section-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 1rem;
    flex-wrap: wrap;
    gap: 1rem;
}

.section-header h2 { font-size: 1.25rem; }

.filter-bar { display: flex; gap: 0.75rem; align-items: center; }

.input-search, .input-select, .input-text {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    color: var(--text);
    padding: 0.5rem 0.75rem;
    font-size: 0.875rem;
    outline: none;
}

.input-search:focus, .input-select:focus, .input-text:focus { border-color: var(--accent); }
.input-search { width: 220px; }
.input-text { width: 100%; }

.projects-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
    gap: 1rem;
}

.project-card {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 1.25rem;
    cursor: pointer;
    transition: all 0.2s;
}

.project-card:hover {
    border-color: var(--accent);
    transform: translateY(-1px);
}

.project-card-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 0.75rem;
}

.project-name {
    font-weight: 600;
    font-size: 1.05rem;
}

.badge {
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 0.2rem 0.6rem;
    border-radius: 999px;
}

.badge-running { background: rgba(34,197,94,0.15); color: var(--green); }
.badge-stopped { background: rgba(239,68,68,0.15); color: var(--red); }
.badge-created { background: rgba(234,179,8,0.15); color: var(--yellow); }
.badge-error { background: rgba(239,68,68,0.15); color: var(--red); }
.badge-deploying { background: rgba(59,130,246,0.15); color: var(--blue); }
.badge-success { background: rgba(34,197,94,0.15); color: var(--green); }
.badge-failed { background: rgba(239,68,68,0.15); color: var(--red); }

.project-meta {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    font-size: 0.85rem;
    color: var(--text-dim);
}

.project-meta span { display: flex; align-items: center; gap: 0.4rem; }

.project-actions {
    display: flex;
    gap: 0.5rem;
    margin-top: 1rem;
    padding-top: 0.75rem;
    border-top: 1px solid var(--border);
}

.btn {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.4rem 0.9rem;
    border-radius: var(--radius);
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--text);
    font-size: 0.8rem;
    cursor: pointer;
    transition: all 0.15s;
    position: relative;
}

.btn:hover { border-color: var(--accent); color: var(--accent); }
.btn:disabled { opacity: 0.5; cursor: not-allowed; pointer-events: none; }
.btn:disabled:hover { border-color: var(--border); color: var(--text); }

.btn-sm { padding: 0.3rem 0.6rem; font-size: 0.75rem; }

.btn-primary { border-color: var(--accent); color: var(--accent); }
.btn-primary:hover { background: rgba(99,102,241,0.1); }

.btn-green { border-color: var(--green); color: var(--green); }
.btn-green:hover { background: rgba(34,197,94,0.1); }

.btn-red { border-color: var(--red); color: var(--red); }
.btn-red:hover { background: rgba(239,68,68,0.1); }

.btn-yellow { border-color: var(--yellow); color: var(--yellow); }
.btn-yellow:hover { background: rgba(234,179,8,0.1); }

.btn-blue { border-color: var(--blue); color: var(--blue); }
.btn-blue:hover { background: rgba(59,130,246,0.1); }

/* Loading spinner */
@keyframes spin {
    0% { transform: rotate(0deg); }
    100% { transform: rotate(360deg); }
}

.spinner {
    display: inline-block;
    width: 14px;
    height: 14px;
    border: 2px solid var(--border);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.6s linear infinite;
}

.spinner-sm { width: 12px; height: 12px; border-width: 1.5px; }

/* Confirmation modal */
.modal-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    animation: fadeIn 0.15s;
}

.modal {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 1.5rem;
    max-width: 440px;
    width: 90%;
    animation: slideUp 0.2s;
}

.modal h3 { margin-bottom: 0.75rem; font-size: 1.1rem; }
.modal p { color: var(--text-dim); margin-bottom: 1.25rem; font-size: 0.9rem; }

.modal-actions {
    display: flex;
    gap: 0.75rem;
    justify-content: flex-end;
}

@keyframes slideUp {
    from { transform: translateY(10px); opacity: 0; }
    to { transform: none; opacity: 1; }
}

/* Toast notifications */
.toast {
    position: fixed;
    bottom: 2rem;
    right: 2rem;
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 0.75rem 1.25rem;
    font-size: 0.875rem;
    z-index: 1001;
    animation: slideIn 0.3s;
    max-width: 400px;
    word-break: break-word;
}

.toast-success { border-color: var(--green); color: var(--green); }
.toast-error { border-color: var(--red); color: var(--red); }
.toast-info { border-color: var(--blue); color: var(--blue); }

@keyframes slideIn { from { transform: translateY(1rem); opacity: 0; } to { transform: none; opacity: 1; } }

/* Project detail page */
.project-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 1.5rem;
    flex-wrap: wrap;
    gap: 1rem;
}

.project-title {
    font-size: 1.5rem;
    font-weight: 700;
    display: flex;
    align-items: center;
    gap: 0.75rem;
}

/* Tab navigation */
.project-tabs {
    display: flex;
    gap: 0;
    border-bottom: 1px solid var(--border);
    margin-bottom: 1.5rem;
    overflow-x: auto;
}

.tab {
    padding: 0.75rem 1.25rem;
    background: none;
    border: none;
    border-bottom: 2px solid transparent;
    color: var(--text-dim);
    cursor: pointer;
    font-size: 0.9rem;
    transition: all 0.15s;
    white-space: nowrap;
}

.tab:hover { color: var(--text); }
.tab.active { color: var(--accent); border-bottom-color: var(--accent); }

.tab-content { animation: fadeIn 0.2s; }
.hidden { display: none !important; }

@keyframes fadeIn { from { opacity: 0; } to { opacity: 1; } }

.detail-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
    gap: 1rem;
}

.detail-item {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 1rem;
}

.detail-label {
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-dim);
    margin-bottom: 0.25rem;
}

.detail-value { font-size: 1rem; word-break: break-all; }

.log-output {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 1rem;
    font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', monospace;
    font-size: 0.8rem;
    line-height: 1.5;
    overflow-x: auto;
    max-height: 600px;
    overflow-y: auto;
    white-space: pre;
    color: var(--text-dim);
}

.logs-controls {
    display: flex;
    gap: 0.75rem;
    margin-bottom: 1rem;
    align-items: center;
}

/* Backup management */
.backup-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 1rem;
}

.backup-list { display: flex; flex-direction: column; gap: 0.5rem; }

.backup-item {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 1rem;
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 0.75rem;
}

.backup-info { display: flex; flex-direction: column; gap: 0.2rem; }

.backup-type {
    font-weight: 600;
    text-transform: capitalize;
}

.backup-meta {
    font-size: 0.8rem;
    color: var(--text-dim);
}

.backup-actions { display: flex; gap: 0.5rem; }

/* Health status */
.health-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
    gap: 1rem;
}

.health-card {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 1rem;
}

.health-card-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 0.5rem;
}

.health-name { font-weight: 600; }

.health-status {
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    padding: 0.15rem 0.5rem;
    border-radius: 999px;
}

.health-healthy { background: rgba(34,197,94,0.15); color: var(--green); border-color: var(--green); }
.health-unhealthy { background: rgba(239,68,68,0.15); color: var(--red); border-color: var(--red); }
.health-starting { background: rgba(234,179,8,0.15); color: var(--yellow); border-color: var(--yellow); }
.health-none { background: rgba(139,143,163,0.15); color: var(--text-dim); }

.health-detail {
    font-size: 0.8rem;
    color: var(--text-dim);
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
}

/* Deployment history */
.deployment-list { display: flex; flex-direction: column; gap: 0.5rem; }

.deployment-item {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 1rem;
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 0.75rem;
    cursor: pointer;
    transition: border-color 0.15s;
}

.deployment-item:hover { border-color: var(--accent); }

.deployment-info { display: flex; flex-direction: column; gap: 0.2rem; }

.deployment-commit {
    font-weight: 600;
    font-family: 'SF Mono', 'Fira Code', monospace;
    font-size: 0.9rem;
}

.deployment-meta {
    font-size: 0.8rem;
    color: var(--text-dim);
}

.deployment-log {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 1rem;
    font-family: 'SF Mono', 'Fira Code', monospace;
    font-size: 0.75rem;
    line-height: 1.5;
    white-space: pre-wrap;
    color: var(--text-dim);
    margin-top: 0.75rem;
    max-height: 400px;
    overflow-y: auto;
}

/* Create project form */
.form-group {
    margin-bottom: 1rem;
}

.form-group label {
    display: block;
    font-size: 0.8rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-dim);
    margin-bottom: 0.35rem;
}

.form-actions {
    display: flex;
    gap: 0.75rem;
    justify-content: flex-end;
    margin-top: 1.25rem;
}

.loading {
    text-align: center;
    padding: 2rem;
    color: var(--text-dim);
}

.empty-state {
    text-align: center;
    padding: 3rem 1rem;
    color: var(--text-dim);
}

.empty-state p { margin-bottom: 0.5rem; }

/* Audit log section */
.audit-list { display: flex; flex-direction: column; gap: 0.25rem; }

.audit-item {
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 0.6rem 0.75rem;
    font-size: 0.8rem;
    border-bottom: 1px solid var(--border);
}

.audit-time {
    color: var(--text-dim);
    white-space: nowrap;
    font-family: monospace;
}

.audit-action { font-weight: 600; }

@media (max-width: 640px) {
    .container { padding: 1rem; }
    .navbar { padding: 0 1rem; }
    .projects-grid { grid-template-columns: 1fr; }
    .filter-bar { flex-direction: column; }
    .input-search { width: 100%; }
    .health-grid { grid-template-columns: 1fr; }
    .project-tabs { gap: 0; }
    .tab { padding: 0.6rem 0.75rem; font-size: 0.8rem; }
}
`

const appJS = `
// FleetDeck Dashboard JavaScript

const isProjectPage = window.location.pathname.startsWith('/project/');
let modalOpen = false;
let userInteracting = false;

document.addEventListener('DOMContentLoaded', () => {
    if (isProjectPage) {
        initProjectPage();
    } else {
        initDashboard();
    }

    // Track user interaction to pause auto-refresh
    document.addEventListener('mousedown', () => { userInteracting = true; });
    document.addEventListener('mouseup', () => { setTimeout(() => { userInteracting = false; }, 2000); });
    document.addEventListener('keydown', () => { userInteracting = true; });
    document.addEventListener('keyup', () => { setTimeout(() => { userInteracting = false; }, 2000); });
});

// --- Confirmation Modal ---

function confirmDialog(message, confirmText, confirmClass) {
    return new Promise((resolve) => {
        modalOpen = true;
        const overlay = document.createElement('div');
        overlay.className = 'modal-overlay';
        overlay.innerHTML = ` + "`" + `
            <div class="modal">
                <h3>Confirm Action</h3>
                <p>${esc(message)}</p>
                <div class="modal-actions">
                    <button class="btn" id="modal-cancel">Cancel</button>
                    <button class="btn ${confirmClass || 'btn-red'}" id="modal-confirm">${esc(confirmText || 'Confirm')}</button>
                </div>
            </div>
        ` + "`" + `;

        const container = document.getElementById('modal-container') || document.body;
        container.appendChild(overlay);

        const cleanup = (result) => {
            overlay.remove();
            modalOpen = false;
            resolve(result);
        };

        overlay.querySelector('#modal-cancel').addEventListener('click', () => cleanup(false));
        overlay.querySelector('#modal-confirm').addEventListener('click', () => cleanup(true));
        overlay.addEventListener('click', (e) => {
            if (e.target === overlay) cleanup(false);
        });
    });
}

// --- Loading Button State ---

function setButtonLoading(btn, loading) {
    if (!btn) return;
    if (loading) {
        btn.disabled = true;
        btn.dataset.originalText = btn.innerHTML;
        btn.innerHTML = '<span class="spinner spinner-sm"></span> ' + btn.textContent.trim();
    } else {
        btn.disabled = false;
        if (btn.dataset.originalText) {
            btn.innerHTML = btn.dataset.originalText;
            delete btn.dataset.originalText;
        }
    }
}

// --- Dashboard ---

async function initDashboard() {
    loadStatus();
    loadProjects();
    setInterval(() => { if (!modalOpen && !userInteracting) loadStatus(); }, 15000);
    setInterval(() => { if (!modalOpen && !userInteracting) loadProjects(); }, 10000);

    document.getElementById('search').addEventListener('input', filterProjects);
    document.getElementById('filter-status').addEventListener('change', filterProjects);
}

async function loadStatus() {
    try {
        const resp = await fetch('/api/status');
        if (!resp.ok) throw new Error('Failed to load status');
        const s = await resp.json();
        document.getElementById('stat-projects').textContent = s.projects;
        document.getElementById('stat-running').textContent = s.running;
        document.getElementById('stat-containers').textContent = s.containers;
        document.getElementById('stat-cpus').textContent = s.cpus;
        document.getElementById('stat-memory').textContent = s.mem_used ? s.mem_used + ' / ' + s.mem_total : '-';
        document.getElementById('stat-disk').textContent = s.disk_pct || '-';
    } catch (e) {
        console.error('Failed to load status:', e);
    }
}

let allProjects = [];

async function loadProjects() {
    try {
        const resp = await fetch('/api/projects');
        if (!resp.ok) throw new Error('Failed to load projects');
        allProjects = await resp.json();
        renderProjects(allProjects);
    } catch (e) {
        document.getElementById('projects-grid').innerHTML = '<div class="loading">Failed to load projects</div>';
        toast('Failed to load projects: ' + e.message, 'error');
    }
}

function filterProjects() {
    const search = document.getElementById('search').value.toLowerCase();
    const status = document.getElementById('filter-status').value;

    const filtered = allProjects.filter(p => {
        if (search && !p.name.toLowerCase().includes(search) && !(p.domain && p.domain.toLowerCase().includes(search))) return false;
        if (status && p.status !== status) return false;
        return true;
    });

    renderProjects(filtered);
}

function renderProjects(projects) {
    const grid = document.getElementById('projects-grid');

    if (!projects || projects.length === 0) {
        grid.innerHTML = '<div class="empty-state"><p>No projects found</p></div>';
        return;
    }

    grid.innerHTML = projects.map(p => {
        const badgeClass = 'badge-' + (p.status || 'created');
        const created = new Date(p.created_at).toLocaleDateString();
        return ` + "`" + `
        <div class="project-card" onclick="window.location='/project/${encodeURIComponent(p.name)}'">
            <div class="project-card-header">
                <span class="project-name">${esc(p.name)}</span>
                <span class="badge ${badgeClass}">${esc(p.status)}</span>
            </div>
            <div class="project-meta">
                ${p.domain ? '<span>&#127760; ' + esc(p.domain) + '</span>' : ''}
                <span>&#128230; ${p.containers} container${p.containers !== 1 ? 's' : ''}</span>
                <span>&#128196; ${esc(p.template)}</span>
                <span>&#128197; ${created}</span>
            </div>
            <div class="project-actions" onclick="event.stopPropagation()">
                ${p.status === 'running'
                    ? '<button class="btn btn-sm btn-yellow" onclick="projectAction(this, \'' + esc(p.name) + '\', \'restart\')">Restart</button>' +
                      '<button class="btn btn-sm btn-red" onclick="projectAction(this, \'' + esc(p.name) + '\', \'stop\')">Stop</button>'
                    : '<button class="btn btn-sm btn-green" onclick="projectAction(this, \'' + esc(p.name) + '\', \'start\')">Start</button>'
                }
            </div>
        </div>
        ` + "`" + `
    }).join('');
}

async function projectAction(btn, name, action) {
    if (action === 'stop' || action === 'restart') {
        const confirmed = await confirmDialog(
            'Are you sure you want to ' + action + ' project "' + name + '"?',
            action.charAt(0).toUpperCase() + action.slice(1),
            action === 'stop' ? 'btn-red' : 'btn-yellow'
        );
        if (!confirmed) return;
    }

    setButtonLoading(btn, true);
    try {
        const resp = await fetch('/api/projects/' + encodeURIComponent(name) + '/' + action, { method: 'POST' });
        if (resp.ok) {
            toast(name + ' ' + action + 'ed successfully', 'success');
            await loadProjects();
        } else {
            const data = await resp.json().catch(() => ({}));
            toast('Failed to ' + action + ' ' + name + ': ' + (data.error || 'unknown error'), 'error');
        }
    } catch (e) {
        toast('Network error: ' + e.message, 'error');
    } finally {
        setButtonLoading(btn, false);
    }
}

// --- Create Project Modal ---

function showCreateProjectForm() {
    modalOpen = true;
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    overlay.innerHTML = ` + "`" + `
        <div class="modal" style="max-width:500px">
            <h3>Create New Project</h3>
            <form id="create-project-form">
                <div class="form-group">
                    <label>Project Name</label>
                    <input type="text" class="input-text" id="cp-name" placeholder="my-project" required pattern="[a-z0-9][a-z0-9-]*[a-z0-9]">
                </div>
                <div class="form-group">
                    <label>Domain</label>
                    <input type="text" class="input-text" id="cp-domain" placeholder="example.com">
                </div>
                <div class="form-group">
                    <label>Template</label>
                    <select class="input-select" id="cp-template" style="width:100%">
                        <option value="static">Static</option>
                        <option value="node">Node.js</option>
                        <option value="python">Python</option>
                        <option value="go">Go</option>
                        <option value="rails">Rails</option>
                        <option value="php">PHP</option>
                    </select>
                </div>
                <div class="form-actions">
                    <button type="button" class="btn" id="cp-cancel">Cancel</button>
                    <button type="submit" class="btn btn-primary" id="cp-submit">Create Project</button>
                </div>
            </form>
        </div>
    ` + "`" + `;

    const container = document.getElementById('modal-container') || document.body;
    container.appendChild(overlay);

    const cleanup = () => {
        overlay.remove();
        modalOpen = false;
    };

    overlay.querySelector('#cp-cancel').addEventListener('click', cleanup);
    overlay.addEventListener('click', (e) => { if (e.target === overlay) cleanup(); });

    overlay.querySelector('#create-project-form').addEventListener('submit', async (e) => {
        e.preventDefault();
        const name = document.getElementById('cp-name').value.trim();
        const domain = document.getElementById('cp-domain').value.trim();
        const tmpl = document.getElementById('cp-template').value;
        const submitBtn = document.getElementById('cp-submit');

        if (!name) { toast('Project name is required', 'error'); return; }

        setButtonLoading(submitBtn, true);
        try {
            const resp = await fetch('/api/webhook/deploy/' + encodeURIComponent(name), {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ domain: domain, template: tmpl })
            });
            if (resp.ok) {
                toast('Deployment triggered for ' + name, 'success');
                cleanup();
                await loadProjects();
            } else {
                const data = await resp.json().catch(() => ({}));
                toast('Failed: ' + (data.error || 'unknown error'), 'error');
            }
        } catch (err) {
            toast('Network error: ' + err.message, 'error');
        } finally {
            setButtonLoading(submitBtn, false);
        }
    });
}

// Make available globally
window.showCreateProjectForm = showCreateProjectForm;

// --- Project Detail Page ---

let currentProject = null;

async function initProjectPage() {
    const name = decodeURIComponent(window.location.pathname.split('/project/')[1]);
    if (!name) return;

    // Tabs
    document.querySelectorAll('.tab').forEach(tab => {
        tab.addEventListener('click', () => {
            document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
            document.querySelectorAll('.tab-content').forEach(c => c.classList.add('hidden'));
            tab.classList.add('active');
            document.getElementById('tab-' + tab.dataset.tab).classList.remove('hidden');

            if (tab.dataset.tab === 'logs') loadLogs();
            if (tab.dataset.tab === 'backups') loadBackups();
            if (tab.dataset.tab === 'health') loadHealth();
            if (tab.dataset.tab === 'deployments') loadDeployments();
        });
    });

    await loadProject(name);
    loadLogs();
    setInterval(() => { if (!modalOpen && !userInteracting) loadProject(name); }, 10000);
}

async function loadProject(name) {
    try {
        const resp = await fetch('/api/projects/' + encodeURIComponent(name));
        if (!resp.ok) {
            const data = await resp.json().catch(() => ({}));
            throw new Error(data.error || 'Failed to load project');
        }
        currentProject = await resp.json();
        renderProjectHeader();
        renderProjectDetails();
    } catch (e) {
        document.getElementById('project-header').innerHTML = '<div class="loading">Failed to load project: ' + esc(e.message) + '</div>';
    }
}

function renderProjectHeader() {
    const p = currentProject;
    const badgeClass = 'badge-' + (p.status || 'created');
    document.getElementById('project-header').innerHTML = ` + "`" + `
        <div class="project-title">
            ${esc(p.name)}
            <span class="badge ${badgeClass}">${esc(p.status)}</span>
        </div>
        <div style="display:flex;gap:0.5rem">
            ${p.status === 'running'
                ? '<button class="btn btn-yellow" onclick="doAction(this, \'restart\')">Restart</button>' +
                  '<button class="btn btn-red" onclick="doAction(this, \'stop\')">Stop</button>'
                : '<button class="btn btn-green" onclick="doAction(this, \'start\')">Start</button>'
            }
            <button class="btn btn-blue" onclick="doManualDeploy(this)">Deploy</button>
            <button class="btn btn-red" onclick="doDeleteProject(this)">Delete</button>
        </div>
    ` + "`" + `;
}

function renderProjectDetails() {
    const p = currentProject;
    const items = [
        { label: 'Domain', value: p.domain || '-' },
        { label: 'Template', value: p.template },
        { label: 'Status', value: p.status },
        { label: 'Source', value: p.source },
        { label: 'Path', value: p.project_path },
        { label: 'Linux User', value: p.linux_user },
        { label: 'GitHub', value: p.github_repo || '-' },
        { label: 'Containers', value: p.containers },
        { label: 'Created', value: new Date(p.created_at).toLocaleString() },
    ];

    document.getElementById('project-details').innerHTML = items.map(i => ` + "`" + `
        <div class="detail-item">
            <div class="detail-label">${i.label}</div>
            <div class="detail-value">${esc(String(i.value))}</div>
        </div>
    ` + "`" + `).join('');
}

async function doAction(btn, action) {
    if (!currentProject) return;

    if (action === 'stop' || action === 'restart') {
        const confirmed = await confirmDialog(
            'Are you sure you want to ' + action + ' project "' + currentProject.name + '"?',
            action.charAt(0).toUpperCase() + action.slice(1),
            action === 'stop' ? 'btn-red' : 'btn-yellow'
        );
        if (!confirmed) return;
    }

    setButtonLoading(btn, true);
    try {
        const resp = await fetch('/api/projects/' + encodeURIComponent(currentProject.name) + '/' + action, { method: 'POST' });
        if (resp.ok) {
            toast(currentProject.name + ' ' + action + 'ed successfully', 'success');
            await loadProject(currentProject.name);
        } else {
            const data = await resp.json().catch(() => ({}));
            toast('Failed to ' + action + ': ' + (data.error || 'unknown error'), 'error');
        }
    } catch (e) {
        toast('Network error: ' + e.message, 'error');
    } finally {
        setButtonLoading(btn, false);
    }
}

async function doManualDeploy(btn) {
    if (!currentProject) return;
    const confirmed = await confirmDialog(
        'Trigger a manual deployment for "' + currentProject.name + '"? This will pull the latest code, rebuild, and restart.',
        'Deploy',
        'btn-blue'
    );
    if (!confirmed) return;

    setButtonLoading(btn, true);
    try {
        const resp = await fetch('/api/webhook/deploy/' + encodeURIComponent(currentProject.name), { method: 'POST' });
        if (resp.ok) {
            toast('Deployment triggered for ' + currentProject.name, 'success');
            await loadProject(currentProject.name);
        } else {
            const data = await resp.json().catch(() => ({}));
            toast('Deploy failed: ' + (data.error || 'unknown error'), 'error');
        }
    } catch (e) {
        toast('Network error: ' + e.message, 'error');
    } finally {
        setButtonLoading(btn, false);
    }
}

async function doDeleteProject(btn) {
    if (!currentProject) return;
    const confirmed = await confirmDialog(
        'Are you sure you want to delete project "' + currentProject.name + '"? This action cannot be undone and will remove all associated data.',
        'Delete Project',
        'btn-red'
    );
    if (!confirmed) return;

    setButtonLoading(btn, true);
    try {
        const resp = await fetch('/api/projects/' + encodeURIComponent(currentProject.name), { method: 'DELETE' });
        if (resp.ok) {
            toast(currentProject.name + ' deleted', 'success');
            window.location = '/';
        } else {
            const data = await resp.json().catch(() => ({}));
            toast('Delete failed: ' + (data.error || 'This operation may not be supported via the API'), 'error');
        }
    } catch (e) {
        toast('Network error: ' + e.message, 'error');
    } finally {
        setButtonLoading(btn, false);
    }
}

// --- Logs Tab ---

async function loadLogs() {
    if (!currentProject) return;
    const lines = document.getElementById('log-lines')?.value || '100';
    try {
        const resp = await fetch('/api/projects/' + encodeURIComponent(currentProject.name) + '/logs?lines=' + lines);
        if (!resp.ok) throw new Error('Failed to load logs');
        const data = await resp.json();
        const el = document.getElementById('log-output');
        if (el) el.textContent = data.logs || 'No logs available';
    } catch (e) {
        const el = document.getElementById('log-output');
        if (el) el.textContent = 'Failed to load logs: ' + e.message;
    }
}

window.loadLogs = loadLogs;

// --- Health Tab ---

async function loadHealth() {
    if (!currentProject) return;
    const el = document.getElementById('health-grid');
    if (!el) return;

    el.innerHTML = '<div class="loading"><span class="spinner"></span> Checking health...</div>';

    try {
        const resp = await fetch('/api/projects/' + encodeURIComponent(currentProject.name) + '/health');
        if (!resp.ok) {
            // Health endpoint may not exist; fall back to showing container status from project data
            el.innerHTML = '<div class="empty-state"><p>Health endpoint not available.</p><p>Container count: ' + esc(String(currentProject.containers)) + '</p></div>';
            return;
        }
        const health = await resp.json();

        if (!health || !health.containers || health.containers.length === 0) {
            el.innerHTML = '<div class="empty-state"><p>No container health data available</p></div>';
            return;
        }

        el.innerHTML = health.containers.map(c => {
            const state = (c.state || 'unknown').toLowerCase();
            const hlth = (c.health || 'none').toLowerCase();
            let statusClass = 'health-none';
            let statusText = state;
            if (hlth === 'healthy') { statusClass = 'health-healthy'; statusText = 'healthy'; }
            else if (hlth === 'unhealthy') { statusClass = 'health-unhealthy'; statusText = 'unhealthy'; }
            else if (hlth === 'starting') { statusClass = 'health-starting'; statusText = 'starting'; }
            else if (state === 'running') { statusClass = 'health-healthy'; }
            else if (state === 'exited' || state === 'dead') { statusClass = 'health-unhealthy'; }

            return ` + "`" + `
                <div class="health-card">
                    <div class="health-card-header">
                        <span class="health-name">${esc(c.name || c.Name || 'unknown')}</span>
                        <span class="health-status ${statusClass}">${esc(statusText)}</span>
                    </div>
                    <div class="health-detail">
                        <span>Image: ${esc(c.image || c.Image || '-')}</span>
                        <span>State: ${esc(state)}</span>
                        ${c.ports ? '<span>Ports: ' + esc(c.ports) + '</span>' : ''}
                    </div>
                </div>
            ` + "`" + `;
        }).join('');
    } catch (e) {
        el.innerHTML = '<div class="empty-state"><p>Could not load health data. The health endpoint may not be available.</p><p>Container count: ' + esc(String(currentProject.containers)) + '</p></div>';
    }
}

// --- Backups Tab ---

async function loadBackups() {
    if (!currentProject) return;
    const el = document.getElementById('backup-list');
    if (!el) return;

    el.innerHTML = '<div class="loading"><span class="spinner"></span> Loading backups...</div>';

    try {
        const resp = await fetch('/api/projects/' + encodeURIComponent(currentProject.name) + '/backups');
        if (!resp.ok) throw new Error('Failed to load backups');
        const backups = await resp.json();

        if (!backups || backups.length === 0) {
            el.innerHTML = '<div class="empty-state"><p>No backups found</p></div>';
            return;
        }

        el.innerHTML = backups.map(b => ` + "`" + `
            <div class="backup-item">
                <div class="backup-info">
                    <span class="backup-type">${esc(b.type)} backup</span>
                    <span class="backup-meta">
                        ${esc(b.trigger)} &middot; ${esc(b.size_human)} &middot; ${new Date(b.created_at).toLocaleString()}
                    </span>
                </div>
                <div class="backup-actions">
                    <span class="badge badge-created">${esc(b.id.substring(0, 8))}</span>
                    <button class="btn btn-sm btn-green" onclick="restoreBackup(this, '${esc(b.id)}')">Restore</button>
                    <button class="btn btn-sm btn-red" onclick="deleteBackup(this, '${esc(b.id)}')">Delete</button>
                </div>
            </div>
        ` + "`" + `).join('');
    } catch (e) {
        el.innerHTML = '<div class="loading">Failed to load backups: ' + esc(e.message) + '</div>';
    }
}

async function createBackup(btn) {
    if (!currentProject) return;
    setButtonLoading(btn, true);
    try {
        const resp = await fetch('/api/projects/' + encodeURIComponent(currentProject.name) + '/backups', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ type: 'snapshot', trigger: 'dashboard' })
        });
        if (resp.ok) {
            toast('Backup created successfully', 'success');
            await loadBackups();
        } else {
            const data = await resp.json().catch(() => ({}));
            toast('Backup creation failed: ' + (data.error || 'This operation may not be supported via the API'), 'error');
        }
    } catch (e) {
        toast('Network error: ' + e.message, 'error');
    } finally {
        setButtonLoading(btn, false);
    }
}

async function restoreBackup(btn, backupId) {
    if (!currentProject) return;
    const confirmed = await confirmDialog(
        'Are you sure you want to restore this backup? This will overwrite the current project files.',
        'Restore',
        'btn-yellow'
    );
    if (!confirmed) return;

    setButtonLoading(btn, true);
    try {
        const resp = await fetch('/api/projects/' + encodeURIComponent(currentProject.name) + '/backups/' + encodeURIComponent(backupId) + '/restore', { method: 'POST' });
        if (resp.ok) {
            toast('Backup restored successfully', 'success');
            await loadProject(currentProject.name);
        } else {
            const data = await resp.json().catch(() => ({}));
            toast('Restore failed: ' + (data.error || 'This operation may not be supported via the API'), 'error');
        }
    } catch (e) {
        toast('Network error: ' + e.message, 'error');
    } finally {
        setButtonLoading(btn, false);
    }
}

async function deleteBackup(btn, backupId) {
    if (!currentProject) return;
    const confirmed = await confirmDialog(
        'Are you sure you want to delete this backup? This cannot be undone.',
        'Delete Backup',
        'btn-red'
    );
    if (!confirmed) return;

    setButtonLoading(btn, true);
    try {
        const resp = await fetch('/api/projects/' + encodeURIComponent(currentProject.name) + '/backups/' + encodeURIComponent(backupId), { method: 'DELETE' });
        if (resp.ok) {
            toast('Backup deleted', 'success');
            await loadBackups();
        } else {
            const data = await resp.json().catch(() => ({}));
            toast('Delete failed: ' + (data.error || 'This operation may not be supported via the API'), 'error');
        }
    } catch (e) {
        toast('Network error: ' + e.message, 'error');
    } finally {
        setButtonLoading(btn, false);
    }
}

window.createBackup = createBackup;
window.restoreBackup = restoreBackup;
window.deleteBackup = deleteBackup;

// --- Deployments Tab ---

async function loadDeployments() {
    if (!currentProject) return;
    const el = document.getElementById('deployment-list');
    if (!el) return;

    el.innerHTML = '<div class="loading"><span class="spinner"></span> Loading deployments...</div>';

    try {
        const resp = await fetch('/api/projects/' + encodeURIComponent(currentProject.name) + '/deployments');
        if (!resp.ok) throw new Error('Failed to load deployments');
        const deployments = await resp.json();

        if (!deployments || deployments.length === 0) {
            el.innerHTML = '<div class="empty-state"><p>No deployments yet</p></div>';
            return;
        }

        el.innerHTML = deployments.map(d => {
            const statusBadge = 'badge-' + (d.status || 'created');
            const started = new Date(d.started_at).toLocaleString();
            const finished = d.finished_at ? new Date(d.finished_at).toLocaleString() : 'In progress';
            const duration = d.finished_at
                ? formatDuration(new Date(d.finished_at) - new Date(d.started_at))
                : 'Running...';

            return ` + "`" + `
                <div class="deployment-item" onclick="toggleDeploymentLog(this)">
                    <div class="deployment-info">
                        <span class="deployment-commit">${esc(d.commit_sha || 'manual')}</span>
                        <span class="deployment-meta">${started} &middot; ${duration}</span>
                    </div>
                    <div>
                        <span class="badge ${statusBadge}">${esc(d.status)}</span>
                    </div>
                    ${d.log ? '<div class="deployment-log hidden">' + esc(d.log) + '</div>' : ''}
                </div>
            ` + "`" + `;
        }).join('');
    } catch (e) {
        el.innerHTML = '<div class="loading">Failed to load deployments: ' + esc(e.message) + '</div>';
    }
}

function toggleDeploymentLog(item) {
    const log = item.querySelector('.deployment-log');
    if (log) log.classList.toggle('hidden');
}

function formatDuration(ms) {
    if (ms < 1000) return '<1s';
    const seconds = Math.floor(ms / 1000);
    if (seconds < 60) return seconds + 's';
    const minutes = Math.floor(seconds / 60);
    const remaining = seconds % 60;
    return minutes + 'm ' + remaining + 's';
}

window.toggleDeploymentLog = toggleDeploymentLog;

// --- Utilities ---

function esc(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

function toast(msg, type) {
    const el = document.createElement('div');
    el.className = 'toast toast-' + (type || 'info');
    el.textContent = msg;
    document.body.appendChild(el);
    setTimeout(() => {
        el.style.opacity = '0';
        el.style.transition = 'opacity 0.3s';
        setTimeout(() => el.remove(), 300);
    }, 4000);
}
`
