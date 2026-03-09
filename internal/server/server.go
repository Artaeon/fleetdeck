package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/db"
)

type Server struct {
	cfg           *config.Config
	db            *db.DB
	server        *http.Server
	webhookSecret string
	apiToken      string
	deploymentMu  sync.Map // maps project name -> *sync.Mutex
}

// GenerateAPIToken creates a random 32-byte hex token for dashboard auth.
func GenerateAPIToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// projectMutex returns a per-project mutex, creating one lazily if needed.
// This prevents concurrent deployments or compose operations on the same project.
func (s *Server) projectMutex(name string) *sync.Mutex {
	v, _ := s.deploymentMu.LoadOrStore(name, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// validProjectName matches valid project name path parameters.
var validProjectName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)

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

func New(cfg *config.Config, database *db.DB, addr string) *Server {
	s := &Server{
		cfg:           cfg,
		db:            database,
		webhookSecret: cfg.Server.WebhookSecret,
		apiToken:      cfg.Server.APIToken,
	}

	mux := http.NewServeMux()

	// API routes (require auth)
	mux.HandleFunc("GET /api/projects", s.requireAuth(s.handleListProjects))
	mux.HandleFunc("GET /api/projects/{name}", s.requireAuth(s.handleGetProject))
	mux.HandleFunc("POST /api/projects/{name}/start", s.requireAuth(s.handleStartProject))
	mux.HandleFunc("POST /api/projects/{name}/stop", s.requireAuth(s.handleStopProject))
	mux.HandleFunc("POST /api/projects/{name}/restart", s.requireAuth(s.handleRestartProject))
	mux.HandleFunc("GET /api/projects/{name}/logs", s.requireAuth(s.handleProjectLogs))
	mux.HandleFunc("GET /api/projects/{name}/backups", s.requireAuth(s.handleListBackups))
	mux.HandleFunc("GET /api/status", s.requireAuth(s.handleServerStatus))

	// Webhook routes (use HMAC auth, not bearer token)
	s.AddWebhookRoutes(mux)

	// Dashboard UI (require auth via cookie or query param)
	mux.HandleFunc("GET /login", s.handleLogin)
	mux.HandleFunc("POST /login", s.handleLoginSubmit)
	mux.HandleFunc("GET /", s.requirePageAuth(s.handleDashboard))
	mux.HandleFunc("GET /project/{name}", s.requirePageAuth(s.handleProjectPage))
	mux.HandleFunc("GET /static/app.js", s.handleJS)
	mux.HandleFunc("GET /static/style.css", s.handleCSS)

	s.server = &http.Server{
		Addr:           addr,
		Handler:        securityHeaders(mux),
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
	r.ParseForm()
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
	return s.server.Shutdown(ctx)
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
		log.Printf("handleListProjects: %v", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
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
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("compose up failed: %s\n%s", err, out))
		return
	}

	s.db.UpdateProjectStatus(name, "running")
	writeJSON(w, map[string]string{"status": "started"})
}

func (s *Server) handleStopProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
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
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("compose down failed: %s\n%s", err, out))
		return
	}

	s.db.UpdateProjectStatus(name, "stopped")
	writeJSON(w, map[string]string{"status": "stopped"})
}

func (s *Server) handleRestartProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
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
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("compose restart failed: %s\n%s", err, out))
		return
	}

	writeJSON(w, map[string]string{"status": "restarted"})
}

func (s *Server) handleProjectLogs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
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

func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p, err := s.db.GetProject(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	records, err := s.db.ListBackupRecords(p.ID, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
