package server

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/backup"
	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/db"
	"github.com/fleetdeck/fleetdeck/internal/health"
	"github.com/fleetdeck/fleetdeck/internal/project"
	"github.com/fleetdeck/fleetdeck/internal/templates"
	"golang.org/x/time/rate"
)

// validProjectName matches valid project name path parameters.
var validProjectName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)

// validUUID matches UUID v4 format strings.
var validUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Server struct {
	cfg           *config.Config
	db            *db.DB
	server        *http.Server
	webhookSecret string
	apiToken      string
	deploymentMu  sync.Map // maps project name -> *sync.Mutex
	rateLimiter   *ipLimiter
	metrics       *Metrics
}

// GenerateAPIToken creates a random 32-byte hex token for dashboard auth.
func GenerateAPIToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating API token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// projectMutex returns a per-project mutex, creating one lazily if needed.
// This prevents concurrent deployments or compose operations on the same project.
func (s *Server) projectMutex(name string) *sync.Mutex {
	v, _ := s.deploymentMu.LoadOrStore(name, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// securityHeaders wraps a handler to set standard security response headers.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")
		next.ServeHTTP(w, r)
	})
}

// statusResponseWriter wraps http.ResponseWriter to capture the status code.
type statusResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// requestLogger logs method, path, status, duration, and client IP for every request.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(sw, r)
		duration := time.Since(start)

		ip := r.RemoteAddr
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			ip = strings.TrimSpace(strings.Split(fwd, ",")[0])
		}

		log.Printf("HTTP %s %s %d %s %s", r.Method, r.URL.Path, sw.statusCode, duration, ip)
	})
}

func New(cfg *config.Config, database *db.DB, addr string) *Server {
	s := &Server{
		cfg:           cfg,
		db:            database,
		webhookSecret: cfg.Server.WebhookSecret,
		apiToken:      cfg.Server.APIToken,
		rateLimiter:   newIPLimiter(rate.Limit(10), 20),
		metrics:       newMetrics(),
	}

	s.metrics.startCacheRefresh(s)

	mux := http.NewServeMux()

	// API routes (require auth)
	mux.HandleFunc("GET /api/projects", s.requireAuth(s.handleListProjects))
	mux.HandleFunc("POST /api/projects", s.requireAuth(s.handleCreateProject))
	mux.HandleFunc("GET /api/projects/{name}", s.requireAuth(s.handleGetProject))
	mux.HandleFunc("DELETE /api/projects/{name}", s.requireAuth(s.handleDeleteProject))
	mux.HandleFunc("POST /api/projects/{name}/start", s.requireAuth(s.handleStartProject))
	mux.HandleFunc("POST /api/projects/{name}/stop", s.requireAuth(s.handleStopProject))
	mux.HandleFunc("POST /api/projects/{name}/restart", s.requireAuth(s.handleRestartProject))
	mux.HandleFunc("GET /api/projects/{name}/logs", s.requireAuth(s.handleProjectLogs))
	mux.HandleFunc("GET /api/projects/{name}/health", s.requireAuth(s.handleProjectHealth))
	mux.HandleFunc("GET /api/projects/{name}/backups", s.requireAuth(s.handleListBackups))
	mux.HandleFunc("POST /api/projects/{name}/backup", s.requireAuth(s.handleCreateBackup))
	mux.HandleFunc("POST /api/projects/{name}/backup/{id}/restore", s.requireAuth(s.handleRestoreBackup))
	mux.HandleFunc("DELETE /api/projects/{name}/backup/{id}", s.requireAuth(s.handleDeleteBackup))
	mux.HandleFunc("GET /api/projects/{name}/deployments", s.requireAuth(s.handleListDeployments))
	mux.HandleFunc("GET /api/servers", s.requireAuth(s.handleListServers))
	mux.HandleFunc("POST /api/servers", s.requireAuth(s.handleCreateServer))
	mux.HandleFunc("DELETE /api/servers/{name}", s.requireAuth(s.handleDeleteServer))
	mux.HandleFunc("POST /api/servers/{name}/check", s.requireAuth(s.handleCheckServer))
	mux.HandleFunc("GET /api/status", s.requireAuth(s.handleServerStatus))
	mux.HandleFunc("GET /api/health", s.requireAuth(s.handleSystemHealth))
	mux.HandleFunc("GET /api/audit", s.requireAuth(s.handleAuditLog))

	// Environment management
	mux.HandleFunc("GET /api/projects/{name}/environments", s.requireAuth(s.handleListEnvironments))
	mux.HandleFunc("POST /api/projects/{name}/environments", s.requireAuth(s.handleCreateEnvironment))
	mux.HandleFunc("DELETE /api/projects/{name}/environments/{env}", s.requireAuth(s.handleDeleteEnvironment))
	mux.HandleFunc("POST /api/projects/{name}/environments/promote", s.requireAuth(s.handlePromoteEnvironment))

	// DNS management
	mux.HandleFunc("GET /api/dns/{domain}", s.requireAuth(s.handleListDNSRecords))
	mux.HandleFunc("POST /api/dns/{domain}/setup", s.requireAuth(s.handleSetupDNS))
	mux.HandleFunc("DELETE /api/dns/{domain}/{type}/{record}", s.requireAuth(s.handleDeleteDNSRecord))

	// Scheduling
	mux.HandleFunc("GET /api/schedule", s.requireAuth(s.handleListSchedules))
	mux.HandleFunc("POST /api/schedule/{project}/enable", s.requireAuth(s.handleEnableSchedule))
	mux.HandleFunc("POST /api/schedule/{project}/disable", s.requireAuth(s.handleDisableSchedule))

	// Volumes
	mux.HandleFunc("GET /api/volumes", s.requireAuth(s.handleListVolumes))
	mux.HandleFunc("DELETE /api/volumes/{name}", s.requireAuth(s.handleDeleteVolume))

	// Prometheus metrics endpoint (auth required)
	mux.HandleFunc("GET /metrics", s.requireAuth(s.handleMetrics))

	// Unauthenticated health check for load balancers
	mux.HandleFunc("GET /healthz", s.handleHealthz)

	// Webhook routes (GitHub uses HMAC auth; manual deploy uses bearer token)
	s.AddWebhookRoutes(mux)

	// Dashboard UI (require auth via cookie or query param)
	mux.HandleFunc("GET /login", s.handleLogin)
	mux.HandleFunc("POST /login", s.handleLoginSubmit)
	mux.HandleFunc("GET /", s.requirePageAuth(s.handleDashboard))
	mux.HandleFunc("GET /project/{name}", s.requirePageAuth(s.handleProjectPage))
	mux.HandleFunc("GET /static/app.js", s.handleJS)
	mux.HandleFunc("GET /static/style.css", s.handleCSS)

	// Wrap the mux with metrics counting and rate limiting.
	metricsMiddleware := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.metrics.incRequests()
		sw := &statusResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		mux.ServeHTTP(sw, r)
		if sw.statusCode >= 400 {
			s.metrics.incErrors()
		}
	})
	handler := rateLimitMiddleware(s.rateLimiter, metricsMiddleware)

	s.server = &http.Server{
		Addr:           addr,
		Handler:        requestLogger(securityHeaders(handler)),
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	return s
}

// requireAuth validates API requests via Bearer token or cookie.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.apiToken == "" {
			// No token configured — allow (development mode)
			next(w, r)
			return
		}

		// Check Authorization header
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimPrefix(auth, "Bearer ")
			if subtle.ConstantTimeCompare([]byte(token), []byte(s.apiToken)) == 1 {
				next(w, r)
				return
			}
		}

		// Check session cookie
		if cookie, err := r.Cookie("fleetdeck_session"); err == nil {
			if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(s.apiToken)) == 1 {
				next(w, r)
				return
			}
		}

		writeError(w, http.StatusUnauthorized, "unauthorized: provide Bearer token or login via dashboard")
	}
}

// requirePageAuth redirects to login for unauthenticated page requests.
func (s *Server) requirePageAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.apiToken == "" {
			next(w, r)
			return
		}

		if cookie, err := r.Cookie("fleetdeck_session"); err == nil {
			if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(s.apiToken)) == 1 {
				next(w, r)
				return
			}
		}

		http.Redirect(w, r, "/login", http.StatusFound)
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(loginHTML))
}

func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16) // 64KB limit
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid form data")
		return
	}
	token := r.FormValue("token")

	if s.apiToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.apiToken)) == 1 {
		http.SetCookie(w, &http.Cookie{
			Name:     "fleetdeck_session",
			Value:    s.apiToken,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   86400 * 7, // 7 days
		})
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(loginErrorHTML))
}

func (s *Server) Start() error {
	log.Printf("FleetDeck dashboard starting on %s", s.server.Addr)
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	close(s.metrics.stopCh)
	return s.server.Shutdown(ctx)
}

// handleHealthz is an unauthenticated health check for load balancers and
// uptime monitors. Returns 200 if the server is running and the database is
// accessible.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if err := s.db.Ping(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		writeJSON(w, map[string]string{"status": "unhealthy"})
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// --- API Handlers ---

type apiProject struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Domain      string    `json:"domain"`
	GitHubRepo  string    `json:"github_repo,omitempty"`
	LinuxUser   string    `json:"linux_user"`
	ProjectPath string    `json:"project_path"`
	Template    string    `json:"template"`
	Status      string    `json:"status"`
	Source      string    `json:"source"`
	Containers  int       `json:"containers"`
	CreatedAt   time.Time `json:"created_at"`
}

type apiBackup struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Trigger   string    `json:"trigger"`
	Path      string    `json:"path"`
	SizeBytes int64     `json:"size_bytes"`
	Size      string    `json:"size_human"`
	CreatedAt time.Time `json:"created_at"`
}

type apiDeployment struct {
	ID         string     `json:"id"`
	CommitSHA  string     `json:"commit_sha"`
	Status     string     `json:"status"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Log        string     `json:"log,omitempty"`
}

type apiStatus struct {
	CPUs       int    `json:"cpus"`
	MemUsed    string `json:"mem_used"`
	MemTotal   string `json:"mem_total"`
	DiskUsed   string `json:"disk_used"`
	DiskTotal  string `json:"disk_total"`
	DiskPct    string `json:"disk_pct"`
	Projects   int    `json:"projects"`
	Running    int    `json:"running"`
	Stopped    int    `json:"stopped"`
	Containers int    `json:"containers"`
	Traefik    string `json:"traefik"`
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.db.ListProjects()
	if err != nil {
		log.Printf("failed to list projects: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}

	result := make([]apiProject, 0, len(projects))
	for _, p := range projects {
		_, total := countContainers(p.ProjectPath)
		result = append(result, apiProject{
			ID:          p.ID,
			Name:        p.Name,
			Domain:      p.Domain,
			GitHubRepo:  p.GitHubRepo,
			LinuxUser:   p.LinuxUser,
			ProjectPath: p.ProjectPath,
			Template:    p.Template,
			Status:      p.Status,
			Source:      p.Source,
			Containers:  total,
			CreatedAt:   p.CreatedAt,
		})
	}

	writeJSON(w, result)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Domain   string `json:"domain"`
		Template string `json:"template"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Name == "" || req.Domain == "" {
		writeError(w, http.StatusBadRequest, "name and domain are required")
		return
	}

	if err := project.ValidateName(req.Name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Template == "" {
		req.Template = "custom"
	}

	tmpl, err := templates.Get(req.Template)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown template: %s", req.Template))
		return
	}

	// Check for duplicate
	if _, err := s.db.GetProject(req.Name); err == nil {
		writeError(w, http.StatusConflict, fmt.Sprintf("project %q already exists", req.Name))
		return
	}

	projectPath := s.cfg.ProjectPath(req.Name)
	linuxUser := project.LinuxUserName(req.Name)

	data := templates.TemplateData{
		Name:            req.Name,
		Domain:          req.Domain,
		PostgresVersion: s.cfg.Defaults.PostgresVersion,
	}

	// Scaffold the project directory
	if err := project.ScaffoldProject(projectPath, tmpl, data); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("scaffolding project: %v", err))
		return
	}

	// Store in database
	p := &db.Project{
		Name:        req.Name,
		Domain:      req.Domain,
		LinuxUser:   linuxUser,
		ProjectPath: projectPath,
		Template:    req.Template,
		Status:      "created",
		Source:      "created",
	}
	if err := s.db.CreateProject(p); err != nil {
		// Clean up scaffolded directory on DB failure
		os.RemoveAll(projectPath)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("saving to database: %v", err))
		return
	}

	audit.Log("project.create", req.Name, fmt.Sprintf("template=%s domain=%s via=api", req.Template, req.Domain), true)

	_, total := countContainers(projectPath)
	writeJSON(w, apiProject{
		ID:          p.ID,
		Name:        p.Name,
		Domain:      p.Domain,
		LinuxUser:   p.LinuxUser,
		ProjectPath: p.ProjectPath,
		Template:    p.Template,
		Status:      p.Status,
		Source:      p.Source,
		Containers:  total,
		CreatedAt:   p.CreatedAt,
	})
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validProjectName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid project name")
		return
	}

	p, err := s.db.GetProject(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	keepData := r.URL.Query().Get("keep-data") == "true"

	mu := s.projectMutex(name)
	mu.Lock()
	defer mu.Unlock()

	// Stop containers
	_ = project.ComposeDown(p.ProjectPath)

	// Remove data unless asked to keep it
	if !keepData {
		os.RemoveAll(p.ProjectPath)
	}

	// Remove from database
	if err := s.db.DeleteProject(name); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("deleting from database: %v", err))
		return
	}

	audit.Log("project.destroy", name, fmt.Sprintf("keep_data=%v via=api", keepData), true)
	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validProjectName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid project name")
		return
	}
	p, err := s.db.GetProject(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	_, total := countContainers(p.ProjectPath)
	writeJSON(w, apiProject{
		ID:          p.ID,
		Name:        p.Name,
		Domain:      p.Domain,
		GitHubRepo:  p.GitHubRepo,
		LinuxUser:   p.LinuxUser,
		ProjectPath: p.ProjectPath,
		Template:    p.Template,
		Status:      p.Status,
		Source:      p.Source,
		Containers:  total,
		CreatedAt:   p.CreatedAt,
	})
}

func (s *Server) handleStartProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validProjectName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid project name")
		return
	}
	p, err := s.db.GetProject(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	mu := s.projectMutex(name)
	mu.Lock()
	defer mu.Unlock()

	cmd := exec.Command("docker", "compose", "up", "-d")
	cmd.Dir = p.ProjectPath
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("compose up failed for project %s: %v\n%s", name, err, out)
		writeError(w, http.StatusInternalServerError, "failed to start project")
		return
	}

	s.db.UpdateProjectStatus(name, "running")
	writeJSON(w, map[string]string{"status": "started"})
}

func (s *Server) handleStopProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validProjectName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid project name")
		return
	}
	p, err := s.db.GetProject(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	mu := s.projectMutex(name)
	mu.Lock()
	defer mu.Unlock()

	cmd := exec.Command("docker", "compose", "down")
	cmd.Dir = p.ProjectPath
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("compose down failed for project %s: %v\n%s", name, err, out)
		writeError(w, http.StatusInternalServerError, "failed to stop project")
		return
	}

	s.db.UpdateProjectStatus(name, "stopped")
	writeJSON(w, map[string]string{"status": "stopped"})
}

func (s *Server) handleRestartProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validProjectName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid project name")
		return
	}
	p, err := s.db.GetProject(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	mu := s.projectMutex(name)
	mu.Lock()
	defer mu.Unlock()

	cmd := exec.Command("docker", "compose", "restart")
	cmd.Dir = p.ProjectPath
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("compose restart failed for project %s: %v\n%s", name, err, out)
		writeError(w, http.StatusInternalServerError, "failed to restart project")
		return
	}

	writeJSON(w, map[string]string{"status": "restarted"})
}

func (s *Server) handleProjectLogs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validProjectName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid project name")
		return
	}
	p, err := s.db.GetProject(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	lines := r.URL.Query().Get("lines")
	if lines == "" {
		lines = "100"
	}
	// Validate lines is a positive integer to prevent injection
	if n, err := strconv.Atoi(lines); err != nil || n < 1 || n > 10000 {
		writeError(w, http.StatusBadRequest, "lines must be a number between 1 and 10000")
		return
	}

	cmd := exec.Command("docker", "compose", "logs", "--tail", lines, "--no-color")
	cmd.Dir = p.ProjectPath
	out, _ := cmd.CombinedOutput()

	writeJSON(w, map[string]string{"logs": string(out)})
}

func (s *Server) handleProjectHealth(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validProjectName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid project name")
		return
	}

	p, err := s.db.GetProject(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	report, err := health.CheckProject(p.ProjectPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("health check failed: %v", err))
		return
	}

	writeJSON(w, report)
}

func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validProjectName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid project name")
		return
	}
	p, err := s.db.GetProject(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	records, err := s.db.ListBackupRecords(p.ID, 20)
	if err != nil {
		log.Printf("failed to list backups for project %s: %v", name, err)
		writeError(w, http.StatusInternalServerError, "failed to list backups")
		return
	}

	result := make([]apiBackup, 0, len(records))
	for _, b := range records {
		result = append(result, apiBackup{
			ID:        b.ID,
			Type:      b.Type,
			Trigger:   b.Trigger,
			Path:      b.Path,
			SizeBytes: b.SizeBytes,
			Size:      formatSize(b.SizeBytes),
			CreatedAt: b.CreatedAt,
		})
	}

	writeJSON(w, result)
}

func (s *Server) handleCreateBackup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validProjectName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid project name")
		return
	}

	p, err := s.db.GetProject(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	record, err := backup.CreateBackup(s.cfg, s.db, p, "manual", "api", backup.Options{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("backup failed: %v", err))
		return
	}

	audit.Log("backup.create", name, fmt.Sprintf("backup_id=%s via=api", record.ID), true)

	writeJSON(w, apiBackup{
		ID:        record.ID,
		Type:      record.Type,
		Trigger:   record.Trigger,
		Path:      record.Path,
		SizeBytes: record.SizeBytes,
		Size:      formatSize(record.SizeBytes),
		CreatedAt: record.CreatedAt,
	})
}

func (s *Server) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	backupID := r.PathValue("id")

	if !validProjectName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid project name")
		return
	}
	if !validUUID.MatchString(backupID) {
		writeError(w, http.StatusBadRequest, "invalid backup ID")
		return
	}

	p, err := s.db.GetProject(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	record, err := s.db.GetBackupRecord(backupID)
	if err != nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}

	// Verify the backup belongs to this project
	if record.ProjectID != p.ID {
		writeError(w, http.StatusNotFound, "backup not found for this project")
		return
	}

	mu := s.projectMutex(name)
	mu.Lock()
	defer mu.Unlock()

	if err := backup.RestoreBackup(record.Path, p.ProjectPath, backup.RestoreOptions{}); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("restore failed: %v", err))
		return
	}

	s.db.UpdateProjectStatus(name, "running")
	audit.Log("backup.restore", name, fmt.Sprintf("backup_id=%s via=api", backupID), true)

	writeJSON(w, map[string]string{"status": "restored"})
}

func (s *Server) handleDeleteBackup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	backupID := r.PathValue("id")

	if !validProjectName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid project name")
		return
	}
	if !validUUID.MatchString(backupID) {
		writeError(w, http.StatusBadRequest, "invalid backup ID")
		return
	}

	p, err := s.db.GetProject(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	record, err := s.db.GetBackupRecord(backupID)
	if err != nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}

	// Verify the backup belongs to this project
	if record.ProjectID != p.ID {
		writeError(w, http.StatusNotFound, "backup not found for this project")
		return
	}

	// Remove backup files from disk
	if record.Path != "" {
		os.RemoveAll(record.Path)
	}

	// Remove from database
	if err := s.db.DeleteBackupRecord(backupID); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("deleting backup record: %v", err))
		return
	}

	audit.Log("backup.delete", name, fmt.Sprintf("backup_id=%s via=api", backupID), true)
	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validProjectName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid project name")
		return
	}

	p, err := s.db.GetProject(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	deployments, err := s.db.ListDeployments(p.ID, limit)
	if err != nil {
		log.Printf("failed to list deployments for project %s: %v", name, err)
		writeError(w, http.StatusInternalServerError, "failed to list deployments")
		return
	}

	result := make([]apiDeployment, 0, len(deployments))
	for _, d := range deployments {
		result = append(result, apiDeployment{
			ID:         d.ID,
			CommitSHA:  d.CommitSHA,
			Status:     d.Status,
			StartedAt:  d.StartedAt,
			FinishedAt: d.FinishedAt,
			Log:        d.Log,
		})
	}

	writeJSON(w, result)
}

func (s *Server) handleServerStatus(w http.ResponseWriter, r *http.Request) {
	status := apiStatus{
		CPUs: runtime.NumCPU(),
	}

	// Memory
	if out, err := exec.Command("free", "-h", "--si").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "Mem:") {
				fields := strings.Fields(line)
				if len(fields) >= 3 {
					status.MemTotal = fields[1]
					status.MemUsed = fields[2]
				}
			}
		}
	}

	// Disk
	if out, err := exec.Command("df", "-h", s.cfg.Server.BasePath).Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 5 {
				status.DiskTotal = fields[1]
				status.DiskUsed = fields[2]
				status.DiskPct = fields[4]
			}
		}
	}

	// Projects
	projects, _ := s.db.ListProjects()
	status.Projects = len(projects)
	for _, p := range projects {
		switch p.Status {
		case "running":
			status.Running++
		case "stopped":
			status.Stopped++
		}
		_, total := countContainers(p.ProjectPath)
		status.Containers += total
	}

	// Traefik
	if out, err := exec.Command("docker", "ps", "--filter", "name=traefik", "--format", "{{.Status}}").Output(); err == nil && len(strings.TrimSpace(string(out))) > 0 {
		status.Traefik = "running"
	} else {
		status.Traefik = "stopped"
	}

	writeJSON(w, status)
}

func (s *Server) handleSystemHealth(w http.ResponseWriter, r *http.Request) {
	projects, err := s.db.ListProjects()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}

	type projectHealthSummary struct {
		Name    string   `json:"name"`
		Healthy bool     `json:"healthy"`
		Errors  []string `json:"errors,omitempty"`
	}

	allHealthy := true
	results := make([]projectHealthSummary, 0, len(projects))
	for _, p := range projects {
		summary := projectHealthSummary{Name: p.Name}
		report, err := health.CheckProject(p.ProjectPath)
		if err != nil {
			summary.Healthy = false
			summary.Errors = []string{err.Error()}
			allHealthy = false
		} else {
			summary.Healthy = report.Healthy
			summary.Errors = report.Errors
			if !report.Healthy {
				allHealthy = false
			}
		}
		results = append(results, summary)
	}

	writeJSON(w, map[string]interface{}{
		"healthy":  allHealthy,
		"projects": results,
	})
}

func (s *Server) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}

	logPath := s.cfg.Audit.LogPath
	if logPath == "" {
		logPath = audit.DefaultLogPath
	}

	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, []audit.AuditEntry{})
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("opening audit log: %v", err))
		return
	}
	defer f.Close()

	// Read all lines, then return the last N entries
	var allEntries []audit.AuditEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var entry audit.AuditEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		allEntries = append(allEntries, entry)
	}

	// Return the last `limit` entries in reverse chronological order
	start := 0
	if len(allEntries) > limit {
		start = len(allEntries) - limit
	}
	recent := allEntries[start:]

	// Reverse so most recent is first
	for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
		recent[i], recent[j] = recent[j], recent[i]
	}

	writeJSON(w, recent)
}

func (s *Server) handleListServers(w http.ResponseWriter, r *http.Request) {
	servers, err := s.db.ListServers()
	if err != nil {
		log.Printf("failed to list servers: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list servers")
		return
	}

	type apiServer struct {
		ID        string    `json:"id"`
		Name      string    `json:"name"`
		Host      string    `json:"host"`
		Port      string    `json:"port"`
		User      string    `json:"user"`
		Status    string    `json:"status"`
		CreatedAt time.Time `json:"created_at"`
	}

	result := make([]apiServer, 0, len(servers))
	for _, sv := range servers {
		result = append(result, apiServer{
			ID:        sv.ID,
			Name:      sv.Name,
			Host:      sv.Host,
			Port:      sv.Port,
			User:      sv.User,
			Status:    sv.Status,
			CreatedAt: sv.CreatedAt,
		})
	}

	writeJSON(w, result)
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func countContainers(path string) (int, int) {
	cmd := exec.Command("docker", "compose", "ps", "--format", "json")
	cmd.Dir = path
	out, err := cmd.Output()
	if err != nil {
		return 0, 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	total := 0
	running := 0
	for _, line := range lines {
		if line == "" {
			continue
		}
		total++
		if strings.Contains(line, `"running"`) {
			running++
		}
	}
	return running, total
}

func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
