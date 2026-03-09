package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/db"
)

type Server struct {
	cfg    *config.Config
	db     *db.DB
	server *http.Server
}

func New(cfg *config.Config, database *db.DB, addr string) *Server {
	s := &Server{
		cfg: cfg,
		db:  database,
	}

	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("GET /api/projects", s.handleListProjects)
	mux.HandleFunc("GET /api/projects/{name}", s.handleGetProject)
	mux.HandleFunc("POST /api/projects/{name}/start", s.handleStartProject)
	mux.HandleFunc("POST /api/projects/{name}/stop", s.handleStopProject)
	mux.HandleFunc("POST /api/projects/{name}/restart", s.handleRestartProject)
	mux.HandleFunc("GET /api/projects/{name}/logs", s.handleProjectLogs)
	mux.HandleFunc("GET /api/projects/{name}/backups", s.handleListBackups)
	mux.HandleFunc("GET /api/status", s.handleServerStatus)

	// Dashboard UI
	mux.HandleFunc("GET /", s.handleDashboard)
	mux.HandleFunc("GET /project/{name}", s.handleProjectPage)
	mux.HandleFunc("GET /static/app.js", s.handleJS)
	mux.HandleFunc("GET /static/style.css", s.handleCSS)

	s.server = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
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
		writeError(w, http.StatusInternalServerError, err.Error())
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
	p, err := s.db.GetProject(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
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
