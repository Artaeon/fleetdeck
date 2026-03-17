package server

const styleCSS = `
:root {
    --bg: #f5f5f7;
    --bg-card: #ffffff;
    --bg-sidebar: rgba(255,255,255,0.72);
    --bg-hover: rgba(0,0,0,0.04);
    --border: #d2d2d7;
    --border-light: #e8e8ed;
    --text: #1d1d1f;
    --text-secondary: #86868b;
    --text-tertiary: #aeaeb2;
    --accent: #007aff;
    --accent-hover: #0056b3;
    --green: #34c759;
    --red: #ff3b30;
    --yellow: #ff9500;
    --orange: #ff9500;
    --purple: #af52de;
    --radius: 16px;
    --radius-sm: 10px;
    --radius-xs: 8px;
    --shadow: 0 2px 8px rgba(0,0,0,0.04), 0 1px 2px rgba(0,0,0,0.06);
    --shadow-lg: 0 8px 30px rgba(0,0,0,0.08), 0 2px 8px rgba(0,0,0,0.04);
    --font: -apple-system, BlinkMacSystemFont, 'SF Pro Display', 'Inter', system-ui, sans-serif;
    --sidebar-width: 260px;
    --content-padding: 32px;
    --transition: 0.2s ease;
}

/* Reset */
*, *::before, *::after { margin: 0; padding: 0; box-sizing: border-box; }

body {
    font-family: var(--font);
    background: var(--bg);
    color: var(--text);
    line-height: 1.5;
    -webkit-font-smoothing: antialiased;
    -moz-osx-font-smoothing: grayscale;
}

a { color: var(--accent); text-decoration: none; }
a:hover { color: var(--accent-hover); }

/* App Layout */
.app {
    display: flex;
    min-height: 100vh;
}

/* Sidebar */
.sidebar {
    position: fixed;
    left: 0;
    top: 0;
    bottom: 0;
    width: var(--sidebar-width);
    background: var(--bg-sidebar);
    backdrop-filter: blur(20px) saturate(180%);
    -webkit-backdrop-filter: blur(20px) saturate(180%);
    border-right: 1px solid var(--border-light);
    z-index: 50;
    display: flex;
    flex-direction: column;
    overflow-y: auto;
}

.sidebar-header {
    padding: 20px 20px 16px;
    border-bottom: 1px solid var(--border-light);
}

.sidebar-logo {
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 1.2rem;
    font-weight: 700;
    color: var(--text);
}

.sidebar-logo svg { color: var(--accent); }

.sidebar-nav {
    flex: 1;
    padding: 12px;
    display: flex;
    flex-direction: column;
    gap: 2px;
}

.nav-item {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 9px 14px;
    border-radius: var(--radius-xs);
    color: var(--text-secondary);
    font-size: 0.9rem;
    font-weight: 500;
    transition: all var(--transition);
    text-decoration: none;
}

.nav-item:hover {
    background: var(--bg-hover);
    color: var(--text);
}

.nav-item.active {
    background: var(--accent);
    color: #fff;
}

.nav-item.active svg { stroke: #fff; }

/* Content Area */
.content {
    flex: 1;
    margin-left: var(--sidebar-width);
    min-height: 100vh;
    display: flex;
    flex-direction: column;
}

.content-header {
    padding: 24px var(--content-padding) 0;
}

.content-header h1 {
    font-size: 1.75rem;
    font-weight: 700;
    letter-spacing: -0.02em;
}

.content-header .breadcrumbs {
    font-size: 0.85rem;
    color: var(--text-secondary);
    margin-bottom: 4px;
}

.content-header .breadcrumbs a { color: var(--text-secondary); }
.content-header .breadcrumbs a:hover { color: var(--accent); }

.header-actions {
    display: flex;
    gap: 10px;
    align-items: center;
    margin-top: 16px;
}

.content-body {
    flex: 1;
    padding: var(--content-padding);
}

/* Cards */
.card {
    background: var(--bg-card);
    border-radius: var(--radius);
    box-shadow: var(--shadow);
    padding: 24px;
}

.card-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 16px;
}

.card-title {
    font-size: 1rem;
    font-weight: 600;
}

/* Stat Cards */
.stats-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: 16px;
    margin-bottom: 24px;
}

.stat-card {
    background: var(--bg-card);
    border-radius: var(--radius);
    box-shadow: var(--shadow);
    padding: 20px;
}

.stat-card .stat-label {
    font-size: 0.8rem;
    font-weight: 500;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    margin-bottom: 6px;
}

.stat-card .stat-value {
    font-size: 1.75rem;
    font-weight: 700;
    letter-spacing: -0.02em;
}

.stat-card .stat-trend {
    font-size: 0.8rem;
    margin-top: 4px;
}

.stat-card .stat-trend.up { color: var(--green); }
.stat-card .stat-trend.down { color: var(--red); }

/* Data Tables */
.data-table {
    width: 100%;
    border-collapse: collapse;
}

.data-table th {
    text-align: left;
    padding: 10px 16px;
    font-size: 0.78rem;
    font-weight: 600;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    border-bottom: 1px solid var(--border-light);
}

.data-table td {
    padding: 12px 16px;
    font-size: 0.9rem;
    border-bottom: 1px solid var(--border-light);
    vertical-align: middle;
}

.data-table tr:last-child td { border-bottom: none; }

.data-table tr:hover td {
    background: var(--bg-hover);
}

.table-link {
    color: var(--text);
    font-weight: 500;
}
.table-link:hover { color: var(--accent); }

/* Status Indicators */
.status-dot {
    display: inline-block;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    margin-right: 6px;
    vertical-align: middle;
}

.status-text {
    display: inline-flex;
    align-items: center;
    font-size: 0.85rem;
    font-weight: 500;
}

/* Buttons */
.btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    padding: 8px 16px;
    border-radius: var(--radius-xs);
    font-size: 0.875rem;
    font-weight: 500;
    font-family: var(--font);
    cursor: pointer;
    transition: all var(--transition);
    border: none;
    text-decoration: none;
}

.btn-primary {
    background: var(--accent);
    color: #fff;
}
.btn-primary:hover { background: var(--accent-hover); color: #fff; }

.btn-secondary {
    background: transparent;
    color: var(--accent);
    border: 1px solid var(--border);
}
.btn-secondary:hover { background: var(--bg-hover); }

.btn-danger {
    background: var(--red);
    color: #fff;
}
.btn-danger:hover { opacity: 0.9; }

.btn-sm {
    padding: 5px 12px;
    font-size: 0.8rem;
}

.btn-full { width: 100%; }

.btn-ghost {
    background: transparent;
    color: var(--text-secondary);
    padding: 6px 10px;
}
.btn-ghost:hover { color: var(--text); background: var(--bg-hover); }

.btn-group {
    display: flex;
    gap: 8px;
}

/* Forms */
.form-group {
    margin-bottom: 16px;
}

.form-label {
    display: block;
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--text-secondary);
    margin-bottom: 6px;
}

.form-input, .form-select, .form-textarea {
    width: 100%;
    padding: 10px 14px;
    border: 1px solid var(--border);
    border-radius: var(--radius-xs);
    font-size: 0.9rem;
    font-family: var(--font);
    color: var(--text);
    background: var(--bg-card);
    transition: border-color var(--transition);
}

.form-input:focus, .form-select:focus, .form-textarea:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px rgba(0,122,255,0.12);
}

.form-input::placeholder { color: var(--text-tertiary); }

.form-textarea {
    min-height: 100px;
    resize: vertical;
}

.form-hint {
    font-size: 0.78rem;
    color: var(--text-tertiary);
    margin-top: 4px;
}

/* Search Bar */
.search-bar {
    position: relative;
    max-width: 360px;
}

.search-bar svg {
    position: absolute;
    left: 12px;
    top: 50%;
    transform: translateY(-50%);
    color: var(--text-tertiary);
}

.search-bar input {
    width: 100%;
    padding: 9px 14px 9px 38px;
    border: 1px solid var(--border);
    border-radius: var(--radius-xs);
    font-size: 0.875rem;
    font-family: var(--font);
    color: var(--text);
    background: var(--bg-card);
}

.search-bar input:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px rgba(0,122,255,0.12);
}

/* Tabs */
.tabs {
    display: flex;
    gap: 0;
    border-bottom: 1px solid var(--border-light);
    margin-bottom: 24px;
}

.tab-btn {
    padding: 10px 20px;
    font-size: 0.875rem;
    font-weight: 500;
    color: var(--text-secondary);
    background: none;
    border: none;
    border-bottom: 2px solid transparent;
    cursor: pointer;
    font-family: var(--font);
    transition: all var(--transition);
}

.tab-btn:hover { color: var(--text); }
.tab-btn.active {
    color: var(--accent);
    border-bottom-color: var(--accent);
}

.tab-content { display: none; }
.tab-content.active { display: block; }

/* Badges / Tags */
.badge {
    display: inline-flex;
    align-items: center;
    padding: 2px 10px;
    border-radius: 100px;
    font-size: 0.75rem;
    font-weight: 600;
    letter-spacing: 0.02em;
}

.badge-green { background: rgba(52,199,89,0.12); color: var(--green); }
.badge-red { background: rgba(255,59,48,0.12); color: var(--red); }
.badge-yellow { background: rgba(255,149,0,0.12); color: var(--yellow); }
.badge-blue { background: rgba(0,122,255,0.12); color: var(--accent); }
.badge-gray { background: rgba(0,0,0,0.06); color: var(--text-secondary); }
.badge-purple { background: rgba(175,82,222,0.12); color: var(--purple); }

/* Alerts */
.alert {
    padding: 12px 16px;
    border-radius: var(--radius-xs);
    font-size: 0.875rem;
    margin-bottom: 16px;
}

.alert-error {
    background: rgba(255,59,48,0.1);
    color: var(--red);
    border: 1px solid rgba(255,59,48,0.2);
}

.alert-success {
    background: rgba(52,199,89,0.1);
    color: var(--green);
    border: 1px solid rgba(52,199,89,0.2);
}

.alert-info {
    background: rgba(0,122,255,0.08);
    color: var(--accent);
    border: 1px solid rgba(0,122,255,0.15);
}

/* Slide Panel */
.slide-panel {
    position: fixed;
    right: 0;
    top: 0;
    bottom: 0;
    width: 480px;
    max-width: 100vw;
    background: var(--bg-card);
    box-shadow: -4px 0 30px rgba(0,0,0,0.1);
    z-index: 200;
    transform: translateX(100%);
    transition: transform 0.3s ease;
    display: flex;
    flex-direction: column;
    overflow-y: auto;
}

.slide-panel.open {
    transform: translateX(0);
}

.slide-panel.hidden { display: none; }

.panel-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 20px 24px;
    border-bottom: 1px solid var(--border-light);
}

.panel-header h2 {
    font-size: 1.15rem;
    font-weight: 600;
}

.panel-close {
    background: none;
    border: none;
    font-size: 1.4rem;
    cursor: pointer;
    color: var(--text-secondary);
    padding: 4px;
    line-height: 1;
}
.panel-close:hover { color: var(--text); }

.panel-body {
    padding: 24px;
    flex: 1;
}

.panel-footer {
    padding: 16px 24px;
    border-top: 1px solid var(--border-light);
    display: flex;
    gap: 10px;
    justify-content: flex-end;
}

/* Modal */
.modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,0.4);
    backdrop-filter: blur(4px);
    -webkit-backdrop-filter: blur(4px);
    z-index: 300;
    display: flex;
    align-items: center;
    justify-content: center;
}

.modal-backdrop.hidden { display: none; }

.modal {
    background: var(--bg-card);
    border-radius: var(--radius);
    box-shadow: var(--shadow-lg);
    max-width: 480px;
    width: calc(100% - 32px);
    padding: 24px;
}

.modal h3 {
    font-size: 1.1rem;
    font-weight: 600;
    margin-bottom: 12px;
}

.modal p {
    color: var(--text-secondary);
    font-size: 0.9rem;
    margin-bottom: 20px;
}

.modal-actions {
    display: flex;
    gap: 10px;
    justify-content: flex-end;
}

/* Toast Notifications */
.toast-container {
    position: fixed;
    bottom: 24px;
    right: 24px;
    z-index: 400;
    display: flex;
    flex-direction: column;
    gap: 8px;
}

.toast {
    padding: 12px 20px;
    border-radius: var(--radius-sm);
    font-size: 0.875rem;
    font-weight: 500;
    box-shadow: var(--shadow-lg);
    animation: toastIn 0.3s ease;
    max-width: 400px;
}

.toast-success { background: var(--green); color: #fff; }
.toast-error { background: var(--red); color: #fff; }
.toast-info { background: var(--accent); color: #fff; }

.toast-fade-out {
    animation: toastOut 0.3s ease forwards;
}

@keyframes toastIn {
    from { opacity: 0; transform: translateY(16px); }
    to { opacity: 1; transform: translateY(0); }
}

@keyframes toastOut {
    from { opacity: 1; transform: translateY(0); }
    to { opacity: 0; transform: translateY(16px); }
}

/* Loading Spinner */
.loading-spinner {
    display: flex;
    justify-content: center;
    align-items: center;
    padding: 60px 0;
}

.loading-spinner::after {
    content: '';
    width: 32px;
    height: 32px;
    border: 3px solid var(--border-light);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.7s linear infinite;
}

@keyframes spin {
    to { transform: rotate(360deg); }
}

/* Log Output */
.log-output {
    background: #1d1d1f;
    color: #e0e0e0;
    padding: 16px;
    border-radius: var(--radius-xs);
    font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', monospace;
    font-size: 0.8rem;
    line-height: 1.6;
    overflow-x: auto;
    max-height: 600px;
    overflow-y: auto;
    white-space: pre;
    tab-size: 4;
}

/* Empty State */
.empty-state {
    text-align: center;
    padding: 60px 20px;
    color: var(--text-secondary);
}

.empty-state svg {
    margin-bottom: 16px;
    color: var(--text-tertiary);
}

.empty-state h3 {
    font-size: 1.1rem;
    font-weight: 600;
    color: var(--text);
    margin-bottom: 6px;
}

.empty-state p {
    font-size: 0.9rem;
    max-width: 360px;
    margin: 0 auto;
}

/* Project Cards Grid */
.projects-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
    gap: 16px;
}

.project-card {
    background: var(--bg-card);
    border-radius: var(--radius);
    box-shadow: var(--shadow);
    padding: 20px;
    cursor: pointer;
    transition: all var(--transition);
    border: 1px solid transparent;
}

.project-card:hover {
    box-shadow: var(--shadow-lg);
    border-color: var(--border-light);
}

.project-card-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    margin-bottom: 12px;
}

.project-card-name {
    font-weight: 600;
    font-size: 1rem;
    color: var(--text);
}

.project-card-meta {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 0.82rem;
    color: var(--text-secondary);
}

/* Detail Grid for project detail */
.detail-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
    gap: 16px;
}

.detail-item {
    display: flex;
    flex-direction: column;
    gap: 2px;
}

.detail-label {
    font-size: 0.78rem;
    font-weight: 500;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.04em;
}

.detail-value {
    font-size: 0.95rem;
    font-weight: 500;
}

/* Section Headers */
.section-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 16px;
}

.section-title {
    font-size: 1.1rem;
    font-weight: 600;
}

/* Key-Value Row */
.kv-row {
    display: flex;
    justify-content: space-between;
    padding: 10px 0;
    border-bottom: 1px solid var(--border-light);
    font-size: 0.9rem;
}
.kv-row:last-child { border-bottom: none; }
.kv-key { color: var(--text-secondary); font-weight: 500; }
.kv-value { font-weight: 500; }

/* Filter Bar */
.filter-bar {
    display: flex;
    gap: 10px;
    align-items: center;
    flex-wrap: wrap;
}

/* Login Page */
.login-page {
    background: var(--bg);
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
}

.login-container {
    width: 100%;
    max-width: 400px;
    padding: 20px;
}

.login-card {
    background: var(--bg-card);
    border-radius: var(--radius);
    box-shadow: var(--shadow-lg);
    padding: 40px 32px;
    text-align: center;
}

.login-logo {
    margin-bottom: 16px;
}

.login-title {
    font-size: 1.5rem;
    font-weight: 700;
    margin-bottom: 6px;
}

.login-subtitle {
    font-size: 0.9rem;
    color: var(--text-secondary);
    margin-bottom: 24px;
}

.login-card .form-group {
    text-align: left;
}

.login-card .btn {
    margin-top: 8px;
}

.login-hint {
    font-size: 0.78rem;
    color: var(--text-tertiary);
    margin-top: 20px;
}

/* Inline Code */
code {
    background: rgba(0,0,0,0.06);
    padding: 2px 6px;
    border-radius: 4px;
    font-size: 0.85em;
    font-family: 'SF Mono', 'Fira Code', monospace;
}

/* Utility Classes */
.hidden { display: none !important; }
.text-green { color: var(--green); }
.text-red { color: var(--red); }
.text-yellow { color: var(--yellow); }
.text-secondary { color: var(--text-secondary); }
.text-center { text-align: center; }
.mt-16 { margin-top: 16px; }
.mt-24 { margin-top: 24px; }
.mb-16 { margin-bottom: 16px; }
.mb-24 { margin-bottom: 24px; }
.gap-8 { gap: 8px; }
.flex { display: flex; }
.flex-col { flex-direction: column; }
.items-center { align-items: center; }
.justify-between { justify-content: space-between; }

/* Responsive */
@media (max-width: 1024px) {
    .sidebar {
        transform: translateX(-100%);
        transition: transform 0.3s ease;
    }
    .sidebar.open { transform: translateX(0); }
    .content { margin-left: 0; }
    .content-body { padding: 20px; }
    .content-header { padding: 20px 20px 0; }
    .slide-panel { width: 100vw; }
}

@media (max-width: 640px) {
    .stats-grid { grid-template-columns: repeat(2, 1fr); }
    .projects-grid { grid-template-columns: 1fr; }
    .header-actions { flex-wrap: wrap; }
}
`

const appJS = `
(function() {
    'use strict';

    // -----------------------------------------------------------------------
    // State
    // -----------------------------------------------------------------------
    var state = {
        projects: [],
        status: null,
        refreshTimer: null
    };

    // -----------------------------------------------------------------------
    // API helper
    // -----------------------------------------------------------------------
    function api(method, path, body) {
        var opts = { method: method, credentials: 'same-origin', headers: {} };
        if (body) {
            opts.headers['Content-Type'] = 'application/json';
            opts.body = JSON.stringify(body);
        }
        return fetch(path, opts).then(function(res) {
            if (res.status === 204) return null;
            if (!res.ok) {
                return res.json().catch(function() {
                    return { error: res.statusText };
                }).then(function(err) {
                    throw new Error(err.error || res.statusText);
                });
            }
            return res.json();
        });
    }

    // -----------------------------------------------------------------------
    // Toast notifications
    // -----------------------------------------------------------------------
    function toast(message, type) {
        var container = document.getElementById('toast-container');
        var el = document.createElement('div');
        el.className = 'toast toast-' + (type || 'info');
        el.textContent = message;
        container.appendChild(el);
        setTimeout(function() {
            el.classList.add('toast-fade-out');
            setTimeout(function() { el.remove(); }, 300);
        }, 3000);
    }

    // -----------------------------------------------------------------------
    // Slide Panel
    // -----------------------------------------------------------------------
    function openPanel(title, contentHTML) {
        var panel = document.getElementById('slide-panel');
        panel.classList.remove('hidden');
        panel.innerHTML =
            '<div class="panel-header">' +
                '<h2>' + escapeHTML(title) + '</h2>' +
                '<button class="panel-close" onclick="window.__closePanel()">&times;</button>' +
            '</div>' +
            '<div class="panel-body">' + contentHTML + '</div>';
        requestAnimationFrame(function() {
            panel.classList.add('open');
        });
    }

    function closePanel() {
        var panel = document.getElementById('slide-panel');
        panel.classList.remove('open');
        setTimeout(function() { panel.classList.add('hidden'); }, 300);
    }
    window.__closePanel = closePanel;

    // -----------------------------------------------------------------------
    // Modal
    // -----------------------------------------------------------------------
    function openModal(contentHTML) {
        var backdrop = document.getElementById('modal-backdrop');
        backdrop.classList.remove('hidden');
        backdrop.innerHTML = '<div class="modal">' + contentHTML + '</div>';
        backdrop.onclick = function(e) {
            if (e.target === backdrop) closeModal();
        };
    }

    function closeModal() {
        document.getElementById('modal-backdrop').classList.add('hidden');
    }
    window.__closeModal = closeModal;

    // -----------------------------------------------------------------------
    // Utilities
    // -----------------------------------------------------------------------
    function escapeHTML(str) {
        if (!str) return '';
        return String(str).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
    }

    function timeAgo(date) {
        if (!date) return 'never';
        var seconds = Math.floor((new Date() - new Date(date)) / 1000);
        if (seconds < 0) seconds = 0;
        if (seconds < 60) return 'just now';
        if (seconds < 3600) return Math.floor(seconds / 60) + 'm ago';
        if (seconds < 86400) return Math.floor(seconds / 3600) + 'h ago';
        return Math.floor(seconds / 86400) + 'd ago';
    }

    function statusDot(status) {
        var colors = {
            running: 'var(--green)',
            stopped: 'var(--red)',
            error: 'var(--red)',
            deploying: 'var(--yellow)',
            created: 'var(--text-tertiary)',
            success: 'var(--green)',
            failed: 'var(--red)',
            pending: 'var(--yellow)'
        };
        var c = colors[status] || 'var(--text-tertiary)';
        return '<span class="status-dot" style="background:' + c + '"></span>';
    }

    function statusBadge(status) {
        var map = {
            running: 'badge-green',
            stopped: 'badge-red',
            error: 'badge-red',
            deploying: 'badge-yellow',
            created: 'badge-gray',
            success: 'badge-green',
            failed: 'badge-red',
            pending: 'badge-yellow',
            healthy: 'badge-green',
            unhealthy: 'badge-red'
        };
        var cls = map[status] || 'badge-gray';
        return '<span class="badge ' + cls + '">' + escapeHTML(status || 'unknown') + '</span>';
    }

    function formatBytes(bytes) {
        if (!bytes || bytes === 0) return '0 B';
        var units = ['B', 'KB', 'MB', 'GB', 'TB'];
        var i = Math.floor(Math.log(bytes) / Math.log(1024));
        if (i >= units.length) i = units.length - 1;
        return (bytes / Math.pow(1024, i)).toFixed(1) + ' ' + units[i];
    }

    function setHeader(title, breadcrumbs) {
        var el = document.getElementById('content-header');
        var html = '';
        if (breadcrumbs) {
            html += '<div class="breadcrumbs">' + breadcrumbs + '</div>';
        }
        html += '<h1>' + escapeHTML(title) + '</h1>';
        el.innerHTML = html;
    }

    function setBody(html) {
        document.getElementById('content-body').innerHTML = html;
    }

    function showLoading() {
        setBody('<div class="loading-spinner"></div>');
    }

    function showError(msg) {
        setBody(
            '<div class="empty-state">' +
                '<h3>Error</h3>' +
                '<p>' + escapeHTML(msg) + '</p>' +
            '</div>'
        );
    }

    // -----------------------------------------------------------------------
    // Router
    // -----------------------------------------------------------------------
    var routes = [
        { pattern: /^\/$/, fn: renderDashboard },
        { pattern: /^\/projects$/, fn: renderProjects },
        { pattern: /^\/project\/([a-z0-9][a-z0-9-]*[a-z0-9])$/, fn: renderProjectDetail },
        { pattern: /^\/servers$/, fn: renderServers },
        { pattern: /^\/deploy$/, fn: renderDeploy },
        { pattern: /^\/dns$/, fn: renderDNS },
        { pattern: /^\/volumes$/, fn: renderVolumes },
        { pattern: /^\/scheduling$/, fn: renderScheduling },
        { pattern: /^\/discovery$/, fn: renderDiscovery },
        { pattern: /^\/templates$/, fn: renderTemplates },
        { pattern: /^\/audit$/, fn: renderAudit },
        { pattern: /^\/settings$/, fn: renderSettings }
    ];

    function navigate() {
        if (state.refreshTimer) {
            clearInterval(state.refreshTimer);
            state.refreshTimer = null;
        }

        var hash = location.hash.slice(1) || '/';
        // Update active nav
        var navItems = document.querySelectorAll('.nav-item');
        for (var i = 0; i < navItems.length; i++) {
            var route = navItems[i].getAttribute('data-route');
            var isActive = false;
            if (route === '/') {
                isActive = (hash === '/');
            } else if (route && hash.indexOf(route) === 0) {
                isActive = true;
            }
            if (isActive) {
                navItems[i].classList.add('active');
            } else {
                navItems[i].classList.remove('active');
            }
        }

        for (var j = 0; j < routes.length; j++) {
            var match = hash.match(routes[j].pattern);
            if (match) {
                routes[j].fn.apply(null, match.slice(1));
                return;
            }
        }

        // 404
        setHeader('Not Found');
        setBody(
            '<div class="empty-state">' +
                '<h3>Page not found</h3>' +
                '<p>The page you are looking for does not exist.</p>' +
            '</div>'
        );
    }

    // -----------------------------------------------------------------------
    // Dashboard Page
    // -----------------------------------------------------------------------
    function renderDashboard() {
        setHeader('Dashboard');
        showLoading();

        Promise.all([
            api('GET', '/api/status'),
            api('GET', '/api/projects')
        ]).then(function(results) {
            state.status = results[0];
            state.projects = results[1] || [];
            renderDashboardContent();
        }).catch(function(err) {
            showError(err.message);
        });

        state.refreshTimer = setInterval(function() {
            Promise.all([
                api('GET', '/api/status'),
                api('GET', '/api/projects')
            ]).then(function(results) {
                state.status = results[0];
                state.projects = results[1] || [];
                renderDashboardContent();
            }).catch(function() {});
        }, 15000);
    }

    function renderDashboardContent() {
        var s = state.status || {};
        var projects = state.projects || [];

        var running = 0;
        var stopped = 0;
        var errors = 0;
        for (var i = 0; i < projects.length; i++) {
            if (projects[i].status === 'running') running++;
            else if (projects[i].status === 'stopped') stopped++;
            else if (projects[i].status === 'error') errors++;
        }

        var html = '';

        // Stats
        html += '<div class="stats-grid">';
        html += statCard('Projects', projects.length, '');
        html += statCard('Running', running, 'text-green');
        html += statCard('Stopped', stopped, '');
        html += statCard('Errors', errors, errors > 0 ? 'text-red' : '');
        html += statCard('CPU Cores', s.cpus || '-', '');
        html += statCard('Memory', s.memory_total ? formatBytes(s.memory_total) : '-', '');
        html += '</div>';

        // Recent projects
        html += '<div class="card">';
        html += '<div class="card-header"><span class="card-title">Projects</span>';
        html += '<a href="#/projects" class="btn btn-secondary btn-sm">View All</a></div>';
        if (projects.length === 0) {
            html += '<div class="empty-state"><h3>No projects yet</h3><p>Create your first project to get started.</p></div>';
        } else {
            html += '<table class="data-table"><thead><tr>';
            html += '<th>Name</th><th>Status</th><th>Domain</th><th>Template</th><th>Created</th>';
            html += '</tr></thead><tbody>';
            var shown = projects.slice(0, 10);
            for (var j = 0; j < shown.length; j++) {
                var p = shown[j];
                html += '<tr onclick="location.hash=\'#/project/' + escapeHTML(p.name) + '\'" style="cursor:pointer">';
                html += '<td><span class="table-link">' + escapeHTML(p.name) + '</span></td>';
                html += '<td>' + statusDot(p.status) + escapeHTML(p.status) + '</td>';
                html += '<td class="text-secondary">' + escapeHTML(p.domain) + '</td>';
                html += '<td>' + statusBadge(p.template) + '</td>';
                html += '<td class="text-secondary">' + timeAgo(p.created_at) + '</td>';
                html += '</tr>';
            }
            html += '</tbody></table>';
        }
        html += '</div>';

        setBody(html);
    }

    function statCard(label, value, cls) {
        return '<div class="stat-card">' +
            '<div class="stat-label">' + escapeHTML(label) + '</div>' +
            '<div class="stat-value ' + (cls || '') + '">' + escapeHTML(String(value)) + '</div>' +
            '</div>';
    }

    // -----------------------------------------------------------------------
    // Projects Page
    // -----------------------------------------------------------------------
    function renderProjects() {
        setHeader('Projects');
        showLoading();

        api('GET', '/api/projects').then(function(projects) {
            state.projects = projects || [];
            renderProjectsList(projects || []);
        }).catch(function(err) {
            showError(err.message);
        });
    }

    function renderProjectsList(projects) {
        var html = '';

        // Actions bar
        html += '<div class="section-header">';
        html += '<div class="filter-bar">';
        html += '<div class="search-bar">';
        html += '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>';
        html += '<input type="text" id="project-search" placeholder="Search projects..." oninput="window.__filterProjects()">';
        html += '</div>';
        html += '<select id="project-filter-status" class="form-select" style="width:auto" onchange="window.__filterProjects()">';
        html += '<option value="">All Status</option><option value="running">Running</option>';
        html += '<option value="stopped">Stopped</option><option value="error">Error</option>';
        html += '</select>';
        html += '</div>';
        html += '<button class="btn btn-primary" onclick="window.__openCreateProject()">New Project</button>';
        html += '</div>';

        html += '<div id="projects-list">';
        html += renderProjectsTable(projects);
        html += '</div>';

        setBody(html);
    }

    function renderProjectsTable(projects) {
        if (projects.length === 0) {
            return '<div class="empty-state"><h3>No projects found</h3><p>No projects match your filter criteria.</p></div>';
        }
        var html = '<div class="projects-grid">';
        for (var i = 0; i < projects.length; i++) {
            var p = projects[i];
            html += '<div class="project-card" onclick="location.hash=\'#/project/' + escapeHTML(p.name) + '\'">';
            html += '<div class="project-card-header">';
            html += '<span class="project-card-name">' + escapeHTML(p.name) + '</span>';
            html += statusBadge(p.status);
            html += '</div>';
            html += '<div class="project-card-meta">';
            html += '<span>' + escapeHTML(p.domain || 'No domain') + '</span>';
            html += '<span>' + escapeHTML(p.template) + ' &middot; ' + timeAgo(p.created_at) + '</span>';
            html += '</div>';
            html += '</div>';
        }
        html += '</div>';
        return html;
    }

    window.__filterProjects = function() {
        var search = (document.getElementById('project-search').value || '').toLowerCase();
        var status = document.getElementById('project-filter-status').value;
        var filtered = (state.projects || []).filter(function(p) {
            if (search && p.name.toLowerCase().indexOf(search) === -1 && (p.domain || '').toLowerCase().indexOf(search) === -1) return false;
            if (status && p.status !== status) return false;
            return true;
        });
        document.getElementById('projects-list').innerHTML = renderProjectsTable(filtered);
    };

    window.__openCreateProject = function() {
        var html = '<form id="create-project-form">';
        html += '<div class="form-group"><label class="form-label">Name</label>';
        html += '<input class="form-input" name="name" placeholder="my-app" required></div>';
        html += '<div class="form-group"><label class="form-label">Domain</label>';
        html += '<input class="form-input" name="domain" placeholder="myapp.example.com"></div>';
        html += '<div class="form-group"><label class="form-label">Template</label>';
        html += '<select class="form-select" name="template" id="create-template-select">';
        html += '<option value="">Loading...</option></select></div>';
        html += '<div class="form-group"><label class="form-label">GitHub Repo (optional)</label>';
        html += '<input class="form-input" name="github_repo" placeholder="user/repo"></div>';
        html += '<div class="panel-footer" style="padding:0;border:none;margin-top:16px">';
        html += '<button type="button" class="btn btn-secondary" onclick="window.__closePanel()">Cancel</button>';
        html += '<button type="submit" class="btn btn-primary">Create Project</button>';
        html += '</div></form>';

        openPanel('New Project', html);

        // Load templates
        api('GET', '/api/templates').then(function(templates) {
            var sel = document.getElementById('create-template-select');
            if (!sel) return;
            sel.innerHTML = '';
            if (templates && templates.length) {
                for (var i = 0; i < templates.length; i++) {
                    var opt = document.createElement('option');
                    opt.value = templates[i].name || templates[i];
                    opt.textContent = templates[i].name || templates[i];
                    sel.appendChild(opt);
                }
            }
        }).catch(function() {});

        setTimeout(function() {
            var form = document.getElementById('create-project-form');
            if (form) {
                form.onsubmit = function(e) {
                    e.preventDefault();
                    var fd = new FormData(form);
                    var data = {
                        name: fd.get('name'),
                        domain: fd.get('domain'),
                        template: fd.get('template'),
                        github_repo: fd.get('github_repo')
                    };
                    api('POST', '/api/projects', data).then(function() {
                        closePanel();
                        toast('Project created successfully', 'success');
                        navigate();
                    }).catch(function(err) {
                        toast(err.message, 'error');
                    });
                };
            }
        }, 100);
    };

    // -----------------------------------------------------------------------
    // Project Detail Page
    // -----------------------------------------------------------------------
    function renderProjectDetail(name) {
        setHeader(name, '<a href="#/projects">Projects</a> / ' + escapeHTML(name));
        showLoading();

        api('GET', '/api/projects/' + name).then(function(project) {
            renderProjectDetailContent(project);
        }).catch(function(err) {
            showError(err.message);
        });

        state.refreshTimer = setInterval(function() {
            api('GET', '/api/projects/' + name).then(function(project) {
                // Only update if still on same page
                if (location.hash === '#/project/' + name) {
                    renderProjectDetailContent(project);
                }
            }).catch(function() {});
        }, 10000);
    }

    function renderProjectDetailContent(project) {
        var name = project.name;
        var html = '';

        // Action buttons
        html += '<div class="header-actions mb-24">';
        html += statusBadge(project.status);
        html += '<div class="btn-group" style="margin-left:auto">';
        if (project.status === 'running') {
            html += '<button class="btn btn-secondary btn-sm" onclick="window.__projectAction(\'' + escapeHTML(name) + '\',\'restart\')">Restart</button>';
            html += '<button class="btn btn-secondary btn-sm" onclick="window.__projectAction(\'' + escapeHTML(name) + '\',\'stop\')">Stop</button>';
        } else {
            html += '<button class="btn btn-primary btn-sm" onclick="window.__projectAction(\'' + escapeHTML(name) + '\',\'start\')">Start</button>';
        }
        html += '<button class="btn btn-secondary btn-sm" onclick="window.__deployProject(\'' + escapeHTML(name) + '\')">Deploy</button>';
        html += '<button class="btn btn-danger btn-sm" onclick="window.__deleteProject(\'' + escapeHTML(name) + '\')">Delete</button>';
        html += '</div></div>';

        // Tabs
        html += '<div class="tabs">';
        html += '<button class="tab-btn active" onclick="window.__switchTab(this,\'overview\')">Overview</button>';
        html += '<button class="tab-btn" onclick="window.__switchTab(this,\'logs\')">Logs</button>';
        html += '<button class="tab-btn" onclick="window.__switchTab(this,\'backups\')">Backups</button>';
        html += '<button class="tab-btn" onclick="window.__switchTab(this,\'deployments\')">Deployments</button>';
        html += '<button class="tab-btn" onclick="window.__switchTab(this,\'envs\')">Environments</button>';
        html += '<button class="tab-btn" onclick="window.__switchTab(this,\'health\')">Health</button>';
        html += '</div>';

        // Tab: Overview
        html += '<div class="tab-content active" id="tab-overview">';
        html += '<div class="card"><div class="detail-grid">';
        html += detailItem('Name', project.name);
        html += detailItem('Domain', project.domain || '-');
        html += detailItem('Template', project.template);
        html += detailItem('Status', project.status);
        html += detailItem('Source', project.source || '-');
        html += detailItem('GitHub', project.github_repo || '-');
        html += detailItem('Created', project.created_at ? new Date(project.created_at).toLocaleString() : '-');
        html += detailItem('Updated', project.updated_at ? new Date(project.updated_at).toLocaleString() : '-');
        html += '</div></div>';
        html += '</div>';

        // Tab: Logs
        html += '<div class="tab-content" id="tab-logs">';
        html += '<div class="section-header mb-16">';
        html += '<select id="log-lines" class="form-select" style="width:auto">';
        html += '<option value="50">50 lines</option><option value="100" selected>100 lines</option>';
        html += '<option value="500">500 lines</option></select>';
        html += '<button class="btn btn-secondary btn-sm" onclick="window.__loadLogs(\'' + escapeHTML(name) + '\')">Refresh</button>';
        html += '</div>';
        html += '<pre class="log-output" id="log-output">Click refresh to load logs...</pre>';
        html += '</div>';

        // Tab: Backups
        html += '<div class="tab-content" id="tab-backups">';
        html += '<div class="section-header mb-16"><span class="section-title">Backups</span>';
        html += '<button class="btn btn-primary btn-sm" onclick="window.__createBackup(\'' + escapeHTML(name) + '\')">Create Backup</button></div>';
        html += '<div id="backup-list"><div class="loading-spinner"></div></div>';
        html += '</div>';

        // Tab: Deployments
        html += '<div class="tab-content" id="tab-deployments">';
        html += '<div id="deployments-list"><div class="loading-spinner"></div></div>';
        html += '</div>';

        // Tab: Environments
        html += '<div class="tab-content" id="tab-envs">';
        html += '<div id="envs-content"><div class="loading-spinner"></div></div>';
        html += '</div>';

        // Tab: Health
        html += '<div class="tab-content" id="tab-health">';
        html += '<div id="health-content"><div class="loading-spinner"></div></div>';
        html += '</div>';

        setBody(html);
    }

    function detailItem(label, value) {
        return '<div class="detail-item"><span class="detail-label">' + escapeHTML(label) + '</span>' +
            '<span class="detail-value">' + escapeHTML(String(value)) + '</span></div>';
    }

    window.__switchTab = function(btn, tabId) {
        var tabs = btn.parentElement.querySelectorAll('.tab-btn');
        for (var i = 0; i < tabs.length; i++) tabs[i].classList.remove('active');
        btn.classList.add('active');

        var contents = document.querySelectorAll('.tab-content');
        for (var j = 0; j < contents.length; j++) contents[j].classList.remove('active');
        var target = document.getElementById('tab-' + tabId);
        if (target) target.classList.add('active');

        // Lazy load tab content
        var name = location.hash.replace('#/project/', '');
        if (tabId === 'logs') window.__loadLogs(name);
        if (tabId === 'backups') window.__loadBackups(name);
        if (tabId === 'deployments') window.__loadDeployments(name);
        if (tabId === 'envs') window.__loadEnvs(name);
        if (tabId === 'health') window.__loadHealth(name);
    };

    window.__projectAction = function(name, action) {
        api('POST', '/api/projects/' + name + '/' + action).then(function() {
            toast('Project ' + action + ' initiated', 'success');
            setTimeout(navigate, 1000);
        }).catch(function(err) {
            toast(err.message, 'error');
        });
    };

    window.__deployProject = function(name) {
        api('POST', '/api/webhook/deploy/' + name).then(function() {
            toast('Deployment started', 'success');
        }).catch(function(err) {
            toast(err.message, 'error');
        });
    };

    window.__deleteProject = function(name) {
        openModal(
            '<h3>Delete Project</h3>' +
            '<p>Are you sure you want to delete <strong>' + escapeHTML(name) + '</strong>? This action cannot be undone.</p>' +
            '<div class="modal-actions">' +
                '<button class="btn btn-secondary" onclick="window.__closeModal()">Cancel</button>' +
                '<button class="btn btn-danger" onclick="window.__confirmDelete(\'' + escapeHTML(name) + '\')">Delete</button>' +
            '</div>'
        );
    };

    window.__confirmDelete = function(name) {
        api('DELETE', '/api/projects/' + name).then(function() {
            closeModal();
            toast('Project deleted', 'success');
            location.hash = '#/projects';
        }).catch(function(err) {
            closeModal();
            toast(err.message, 'error');
        });
    };

    window.__loadLogs = function(name) {
        var lines = document.getElementById('log-lines');
        var n = lines ? lines.value : '100';
        var el = document.getElementById('log-output');
        if (el) el.textContent = 'Loading...';
        api('GET', '/api/projects/' + name + '/logs?lines=' + n).then(function(data) {
            if (el) el.textContent = data.logs || 'No logs available.';
        }).catch(function(err) {
            if (el) el.textContent = 'Error: ' + err.message;
        });
    };

    window.__loadBackups = function(name) {
        var el = document.getElementById('backup-list');
        api('GET', '/api/projects/' + name + '/backups').then(function(backups) {
            if (!backups || backups.length === 0) {
                el.innerHTML = '<div class="empty-state"><p>No backups yet.</p></div>';
                return;
            }
            var html = '<table class="data-table"><thead><tr><th>ID</th><th>Type</th><th>Size</th><th>Created</th><th></th></tr></thead><tbody>';
            for (var i = 0; i < backups.length; i++) {
                var b = backups[i];
                html += '<tr><td class="text-secondary">' + escapeHTML(b.id || '').substring(0, 8) + '</td>';
                html += '<td>' + escapeHTML(b.type) + '</td>';
                html += '<td>' + escapeHTML(b.size) + '</td>';
                html += '<td class="text-secondary">' + timeAgo(b.created_at) + '</td>';
                html += '<td><button class="btn btn-ghost btn-sm" onclick="window.__restoreBackup(\'' + escapeHTML(name) + '\',\'' + escapeHTML(b.id) + '\')">Restore</button></td>';
                html += '</tr>';
            }
            html += '</tbody></table>';
            el.innerHTML = html;
        }).catch(function(err) {
            el.innerHTML = '<div class="alert alert-error">' + escapeHTML(err.message) + '</div>';
        });
    };

    window.__createBackup = function(name) {
        api('POST', '/api/projects/' + name + '/backup').then(function() {
            toast('Backup created', 'success');
            window.__loadBackups(name);
        }).catch(function(err) {
            toast(err.message, 'error');
        });
    };

    window.__restoreBackup = function(name, backupId) {
        openModal(
            '<h3>Restore Backup</h3>' +
            '<p>Are you sure you want to restore this backup? Current data will be overwritten.</p>' +
            '<div class="modal-actions">' +
                '<button class="btn btn-secondary" onclick="window.__closeModal()">Cancel</button>' +
                '<button class="btn btn-primary" onclick="window.__confirmRestore(\'' + escapeHTML(name) + '\',\'' + escapeHTML(backupId) + '\')">Restore</button>' +
            '</div>'
        );
    };

    window.__confirmRestore = function(name, backupId) {
        api('POST', '/api/projects/' + name + '/backup/' + backupId + '/restore').then(function() {
            closeModal();
            toast('Backup restored', 'success');
        }).catch(function(err) {
            closeModal();
            toast(err.message, 'error');
        });
    };

    window.__loadDeployments = function(name) {
        var el = document.getElementById('deployments-list');
        api('GET', '/api/projects/' + name + '/deployments').then(function(deps) {
            if (!deps || deps.length === 0) {
                el.innerHTML = '<div class="empty-state"><p>No deployments yet.</p></div>';
                return;
            }
            var html = '<table class="data-table"><thead><tr><th>Commit</th><th>Status</th><th>Started</th><th>Duration</th></tr></thead><tbody>';
            for (var i = 0; i < deps.length; i++) {
                var d = deps[i];
                html += '<tr>';
                html += '<td><code>' + escapeHTML(d.commit_sha || '-') + '</code></td>';
                html += '<td>' + statusBadge(d.status) + '</td>';
                html += '<td class="text-secondary">' + timeAgo(d.started_at) + '</td>';
                html += '<td class="text-secondary">' + escapeHTML(d.duration || '-') + '</td>';
                html += '</tr>';
            }
            html += '</tbody></table>';
            el.innerHTML = html;
        }).catch(function(err) {
            el.innerHTML = '<div class="alert alert-error">' + escapeHTML(err.message) + '</div>';
        });
    };

    window.__loadEnvs = function(name) {
        var el = document.getElementById('envs-content');
        api('GET', '/api/projects/' + name + '/env').then(function(data) {
            var env = data || {};
            var keys = Object.keys(env);
            if (keys.length === 0) {
                el.innerHTML = '<div class="empty-state"><p>No environment variables set.</p></div>';
                return;
            }
            var html = '<div class="card">';
            for (var i = 0; i < keys.length; i++) {
                html += '<div class="kv-row"><span class="kv-key">' + escapeHTML(keys[i]) + '</span>';
                html += '<span class="kv-value"><code>' + escapeHTML(String(env[keys[i]])) + '</code></span></div>';
            }
            html += '</div>';
            el.innerHTML = html;
        }).catch(function(err) {
            el.innerHTML = '<div class="empty-state"><p>Environment variables not available.</p></div>';
        });
    };

    window.__loadHealth = function(name) {
        var el = document.getElementById('health-content');
        api('GET', '/api/projects/' + name + '/health').then(function(data) {
            var html = '<div class="card"><div class="detail-grid">';
            html += detailItem('Status', data.status || 'unknown');
            html += detailItem('Uptime', data.uptime || '-');
            html += detailItem('Last Check', data.last_check ? timeAgo(data.last_check) : '-');
            html += '</div></div>';
            el.innerHTML = html;
        }).catch(function(err) {
            el.innerHTML = '<div class="empty-state"><p>Health data not available.</p></div>';
        });
    };

    // -----------------------------------------------------------------------
    // Initialize
    // -----------------------------------------------------------------------
    window.addEventListener('hashchange', navigate);
    document.addEventListener('DOMContentLoaded', function() {
        navigate();
    });
})();
`
