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

.section-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 1rem;
    flex-wrap: wrap;
    gap: 1rem;
}

.section-header h2 { font-size: 1.25rem; }

.filter-bar { display: flex; gap: 0.75rem; }

.input-search, .input-select {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    color: var(--text);
    padding: 0.5rem 0.75rem;
    font-size: 0.875rem;
    outline: none;
}

.input-search:focus, .input-select:focus { border-color: var(--accent); }
.input-search { width: 220px; }

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
}

.btn:hover { border-color: var(--accent); color: var(--accent); }
.btn:disabled { opacity: 0.4; cursor: not-allowed; }

.btn-sm { padding: 0.3rem 0.6rem; font-size: 0.75rem; }

.btn-green { border-color: var(--green); color: var(--green); }
.btn-green:hover { background: rgba(34,197,94,0.1); }

.btn-red { border-color: var(--red); color: var(--red); }
.btn-red:hover { background: rgba(239,68,68,0.1); }

.btn-yellow { border-color: var(--yellow); color: var(--yellow); }
.btn-yellow:hover { background: rgba(234,179,8,0.1); }

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

.project-tabs {
    display: flex;
    gap: 0;
    border-bottom: 1px solid var(--border);
    margin-bottom: 1.5rem;
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

.loading {
    text-align: center;
    padding: 2rem;
    color: var(--text-dim);
}

.toast {
    position: fixed;
    bottom: 2rem;
    right: 2rem;
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 0.75rem 1.25rem;
    font-size: 0.875rem;
    z-index: 1000;
    animation: slideIn 0.3s;
}

.toast-success { border-color: var(--green); }
.toast-error { border-color: var(--red); }

@keyframes slideIn { from { transform: translateY(1rem); opacity: 0; } to { transform: none; opacity: 1; } }

@media (max-width: 640px) {
    .container { padding: 1rem; }
    .navbar { padding: 0 1rem; }
    .projects-grid { grid-template-columns: 1fr; }
    .filter-bar { flex-direction: column; }
    .input-search { width: 100%; }
}
`

const appJS = `
// FleetDeck Dashboard JavaScript

const isProjectPage = window.location.pathname.startsWith('/project/');

document.addEventListener('DOMContentLoaded', () => {
    if (isProjectPage) {
        initProjectPage();
    } else {
        initDashboard();
    }
});

// --- Dashboard ---

async function initDashboard() {
    loadStatus();
    loadProjects();
    setInterval(loadStatus, 15000);
    setInterval(loadProjects, 10000);

    document.getElementById('search').addEventListener('input', filterProjects);
    document.getElementById('filter-status').addEventListener('change', filterProjects);
}

async function loadStatus() {
    try {
        const resp = await fetch('/api/status');
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
        allProjects = await resp.json();
        renderProjects(allProjects);
    } catch (e) {
        document.getElementById('projects-grid').innerHTML = '<div class="loading">Failed to load projects</div>';
    }
}

function filterProjects() {
    const search = document.getElementById('search').value.toLowerCase();
    const status = document.getElementById('filter-status').value;

    const filtered = allProjects.filter(p => {
        if (search && !p.name.toLowerCase().includes(search) && !p.domain.toLowerCase().includes(search)) return false;
        if (status && p.status !== status) return false;
        return true;
    });

    renderProjects(filtered);
}

function renderProjects(projects) {
    const grid = document.getElementById('projects-grid');

    if (!projects || projects.length === 0) {
        grid.innerHTML = '<div class="loading">No projects found</div>';
        return;
    }

    grid.innerHTML = projects.map(p => {
        const badgeClass = 'badge-' + (p.status || 'created');
        const created = new Date(p.created_at).toLocaleDateString();
        return ` + "`" + `
        <div class="project-card" onclick="window.location='/project/${p.name}'">
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
                    ? '<button class="btn btn-sm btn-yellow" onclick="projectAction(\'' + p.name + '\', \'restart\')">Restart</button>' +
                      '<button class="btn btn-sm btn-red" onclick="projectAction(\'' + p.name + '\', \'stop\')">Stop</button>'
                    : '<button class="btn btn-sm btn-green" onclick="projectAction(\'' + p.name + '\', \'start\')">Start</button>'
                }
            </div>
        </div>
        ` + "`" + `
    }).join('');
}

async function projectAction(name, action) {
    try {
        const resp = await fetch('/api/projects/' + encodeURIComponent(name) + '/' + action, { method: 'POST' });
        if (resp.ok) {
            toast(name + ' ' + action + 'ed', 'success');
            loadProjects();
        } else {
            const data = await resp.json();
            toast('Error: ' + (data.error || 'unknown'), 'error');
        }
    } catch (e) {
        toast('Network error', 'error');
    }
}

// --- Project Page ---

let currentProject = null;

async function initProjectPage() {
    const name = window.location.pathname.split('/project/')[1];
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
        });
    });

    await loadProject(name);
    loadLogs();
    setInterval(() => loadProject(name), 10000);
}

async function loadProject(name) {
    try {
        const resp = await fetch('/api/projects/' + encodeURIComponent(name));
        currentProject = await resp.json();
        renderProjectHeader();
        renderProjectDetails();
    } catch (e) {
        document.getElementById('project-header').innerHTML = '<div class="loading">Failed to load project</div>';
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
                ? '<button class="btn btn-yellow" onclick="doAction(\'restart\')">Restart</button>' +
                  '<button class="btn btn-red" onclick="doAction(\'stop\')">Stop</button>'
                : '<button class="btn btn-green" onclick="doAction(\'start\')">Start</button>'
            }
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

async function doAction(action) {
    if (!currentProject) return;
    try {
        const resp = await fetch('/api/projects/' + encodeURIComponent(currentProject.name) + '/' + action, { method: 'POST' });
        if (resp.ok) {
            toast(currentProject.name + ' ' + action + 'ed', 'success');
            await loadProject(currentProject.name);
        } else {
            const data = await resp.json();
            toast('Error: ' + (data.error || 'unknown'), 'error');
        }
    } catch (e) {
        toast('Network error', 'error');
    }
}

async function loadLogs() {
    if (!currentProject) return;
    const lines = document.getElementById('log-lines')?.value || '100';
    try {
        const resp = await fetch('/api/projects/' + encodeURIComponent(currentProject.name) + '/logs?lines=' + lines);
        const data = await resp.json();
        const el = document.getElementById('log-output');
        if (el) el.textContent = data.logs || 'No logs available';
    } catch (e) {
        const el = document.getElementById('log-output');
        if (el) el.textContent = 'Failed to load logs';
    }
}

// Make loadLogs available globally for the onclick handler
window.loadLogs = loadLogs;

async function loadBackups() {
    if (!currentProject) return;
    try {
        const resp = await fetch('/api/projects/' + encodeURIComponent(currentProject.name) + '/backups');
        const backups = await resp.json();
        const el = document.getElementById('backup-list');

        if (!backups || backups.length === 0) {
            el.innerHTML = '<div class="loading">No backups found</div>';
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
                <div>
                    <span class="badge badge-created">${esc(b.id.substring(0, 8))}</span>
                </div>
            </div>
        ` + "`" + `).join('');
    } catch (e) {
        document.getElementById('backup-list').innerHTML = '<div class="loading">Failed to load backups</div>';
    }
}

// --- Utilities ---

function esc(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

function toast(msg, type) {
    const el = document.createElement('div');
    el.className = 'toast toast-' + type;
    el.textContent = msg;
    document.body.appendChild(el);
    setTimeout(() => el.remove(), 3000);
}
`
