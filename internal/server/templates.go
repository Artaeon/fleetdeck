package server

import "net/http"

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(dashboardHTML))
}

func (s *Server) handleProjectPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(projectHTML))
}

func (s *Server) handleJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Write([]byte(appJS))
}

func (s *Server) handleCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css")
	w.Write([]byte(styleCSS))
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>FleetDeck</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <nav class="navbar">
        <div class="nav-brand">
            <span class="logo">&#9096;</span> FleetDeck
        </div>
        <div class="nav-links">
            <a href="/" class="active">Dashboard</a>
        </div>
    </nav>

    <main class="container">
        <section class="status-cards" id="status-cards">
            <div class="card stat-card">
                <div class="stat-label">Projects</div>
                <div class="stat-value" id="stat-projects">-</div>
            </div>
            <div class="card stat-card">
                <div class="stat-label">Running</div>
                <div class="stat-value text-green" id="stat-running">-</div>
            </div>
            <div class="card stat-card">
                <div class="stat-label">Containers</div>
                <div class="stat-value" id="stat-containers">-</div>
            </div>
            <div class="card stat-card">
                <div class="stat-label">CPU Cores</div>
                <div class="stat-value" id="stat-cpus">-</div>
            </div>
            <div class="card stat-card">
                <div class="stat-label">Memory</div>
                <div class="stat-value" id="stat-memory">-</div>
            </div>
            <div class="card stat-card">
                <div class="stat-label">Disk</div>
                <div class="stat-value" id="stat-disk">-</div>
            </div>
        </section>

        <section class="projects-section">
            <div class="section-header">
                <h2>Projects</h2>
                <div class="filter-bar">
                    <input type="text" id="search" placeholder="Search projects..." class="input-search">
                    <select id="filter-status" class="input-select">
                        <option value="">All Status</option>
                        <option value="running">Running</option>
                        <option value="stopped">Stopped</option>
                        <option value="created">Created</option>
                        <option value="error">Error</option>
                    </select>
                </div>
            </div>
            <div class="projects-grid" id="projects-grid">
                <div class="loading">Loading projects...</div>
            </div>
        </section>
    </main>

    <script src="/static/app.js"></script>
</body>
</html>`

const loginHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>FleetDeck - Login</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div style="display:flex;align-items:center;justify-content:center;min-height:100vh">
        <div class="card" style="width:100%;max-width:400px">
            <h2 style="margin-bottom:1rem;text-align:center">FleetDeck</h2>
            <form method="POST" action="/login">
                <label class="stat-label" style="display:block;margin-bottom:0.5rem">API Token</label>
                <input type="password" name="token" class="input-search" style="width:100%;margin-bottom:1rem" placeholder="Enter your API token" autofocus>
                <button type="submit" class="btn" style="width:100%;justify-content:center;padding:0.6rem">Login</button>
            </form>
            <p style="color:var(--text-dim);font-size:0.8rem;margin-top:1rem;text-align:center">
                Token is configured in /etc/fleetdeck/config.toml under [server] api_token
            </p>
        </div>
    </div>
</body>
</html>`

const loginErrorHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>FleetDeck - Login</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div style="display:flex;align-items:center;justify-content:center;min-height:100vh">
        <div class="card" style="width:100%;max-width:400px">
            <h2 style="margin-bottom:1rem;text-align:center">FleetDeck</h2>
            <div style="background:rgba(239,68,68,0.15);border:1px solid var(--red);border-radius:var(--radius);padding:0.75rem;margin-bottom:1rem;color:var(--red);font-size:0.875rem;text-align:center">
                Invalid token. Please try again.
            </div>
            <form method="POST" action="/login">
                <label class="stat-label" style="display:block;margin-bottom:0.5rem">API Token</label>
                <input type="password" name="token" class="input-search" style="width:100%;margin-bottom:1rem" placeholder="Enter your API token" autofocus>
                <button type="submit" class="btn" style="width:100%;justify-content:center;padding:0.6rem">Login</button>
            </form>
        </div>
    </div>
</body>
</html>`

const projectHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>FleetDeck - Project</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <nav class="navbar">
        <div class="nav-brand">
            <span class="logo">&#9096;</span> <a href="/">FleetDeck</a>
        </div>
        <div class="nav-links">
            <a href="/">Dashboard</a>
        </div>
    </nav>

    <main class="container">
        <div class="project-header" id="project-header">
            <div class="loading">Loading...</div>
        </div>

        <div class="project-tabs">
            <button class="tab active" data-tab="overview">Overview</button>
            <button class="tab" data-tab="logs">Logs</button>
            <button class="tab" data-tab="backups">Backups</button>
        </div>

        <div class="tab-content" id="tab-overview">
            <div class="detail-grid" id="project-details">
                <div class="loading">Loading...</div>
            </div>
        </div>

        <div class="tab-content hidden" id="tab-logs">
            <div class="logs-controls">
                <select id="log-lines" class="input-select">
                    <option value="50">50 lines</option>
                    <option value="100" selected>100 lines</option>
                    <option value="500">500 lines</option>
                </select>
                <button class="btn btn-sm" onclick="loadLogs()">Refresh</button>
            </div>
            <pre class="log-output" id="log-output">Loading logs...</pre>
        </div>

        <div class="tab-content hidden" id="tab-backups">
            <div class="backup-list" id="backup-list">
                <div class="loading">Loading backups...</div>
            </div>
        </div>
    </main>

    <script src="/static/app.js"></script>
</body>
</html>`
