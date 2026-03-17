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
// Placeholder - SPA JS will be added in subsequent commits
document.addEventListener('DOMContentLoaded', function() {
    var body = document.getElementById('content-body');
    if (body) { body.innerHTML = '<div class="loading-spinner"></div>'; }
});
`
