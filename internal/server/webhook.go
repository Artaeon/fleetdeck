package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/compose"
	"github.com/fleetdeck/fleetdeck/internal/db"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// WebhookConfig holds webhook authentication settings.
type WebhookConfig struct {
	Secret string // HMAC secret for verifying GitHub webhooks
}

// AddWebhookRoutes registers webhook endpoints on the given mux.
func (s *Server) AddWebhookRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/webhook/github", s.handleGitHubWebhook)
	mux.HandleFunc("POST /api/webhook/deploy/{name}", s.requireAuth(s.handleManualDeploy))
}

type githubPushPayload struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

func (s *Server) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	// Verify HMAC signature (required — reject if no secret configured)
	if s.webhookSecret == "" {
		writeError(w, http.StatusForbidden, "webhook secret not configured")
		return
	}
	sig := r.Header.Get("X-Hub-Signature-256")
	if !verifyHMAC(body, sig, s.webhookSecret) {
		writeError(w, http.StatusUnauthorized, "invalid signature")
		return
	}

	event := r.Header.Get("X-GitHub-Event")
	if event == "ping" {
		writeJSON(w, map[string]string{"status": "pong"})
		return
	}
	if event != "push" {
		writeJSON(w, map[string]string{"status": "ignored", "event": event})
		return
	}

	var payload githubPushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	// Only deploy on main/master branch pushes
	ref := payload.Ref
	if ref != "refs/heads/main" && ref != "refs/heads/master" {
		writeJSON(w, map[string]string{"status": "ignored", "reason": "not main branch"})
		return
	}

	// Find matching project by GitHub repo
	repoName := payload.Repository.FullName
	project := s.findProjectByRepo(repoName)
	if project == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("no project for repo %s", repoName))
		return
	}

	commitSHA := payload.After
	if len(commitSHA) > 12 {
		commitSHA = commitSHA[:12]
	}

	// Start async deployment
	go s.runDeployment(project, payload.After, commitSHA)

	writeJSON(w, map[string]string{
		"status":  "deploying",
		"project": project.Name,
		"commit":  commitSHA,
	})
}

func (s *Server) handleManualDeploy(w http.ResponseWriter, r *http.Request) {
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

	go s.runDeployment(p, "", "manual")

	writeJSON(w, map[string]string{"status": "deploying", "project": p.Name})
}



func (s *Server) runDeployment(p *db.Project, fullSHA, shortSHA string) {
	mu := s.projectMutex(p.Name)
	mu.Lock()
	defer mu.Unlock()

	dep := &db.Deployment{
		ID:        uuid.New().String(),
		ProjectID: p.ID,
		CommitSHA: shortSHA,
		Status:    "deploying",
		StartedAt: time.Now(),
	}
	s.db.CreateDeployment(dep)
	s.db.UpdateProjectStatus(p.Name, "deploying")

	var logBuf strings.Builder

	// Record current container image IDs for rollback
	logBuf.WriteString("=== Pre-deployment State ===\n")
	previousImages := captureContainerImages(p.ProjectPath)
	if len(previousImages) > 0 {
		for svc, img := range previousImages {
			logBuf.WriteString(fmt.Sprintf("  %s: %s\n", svc, img))
		}
	} else {
		logBuf.WriteString("  (no running containers)\n")
	}

	// Step 1: git pull
	logBuf.WriteString("\n=== Git Pull ===\n")
	cmd := exec.Command("git", "pull", "--ff-only")
	cmd.Dir = p.ProjectPath
	if out, err := cmd.CombinedOutput(); err != nil {
		logBuf.WriteString(string(out))
		logBuf.WriteString(fmt.Sprintf("\ngit pull failed: %v\n", err))
		s.finishDeployment(dep, "failed", logBuf.String(), p.Name)
		return
	} else {
		logBuf.WriteString(string(out))
	}

	// Step 2: validate compose configuration
	logBuf.WriteString("\n=== Compose Config Validation ===\n")
	if err := compose.Validate(p.ProjectPath); err != nil {
		logBuf.WriteString(fmt.Sprintf("validation failed: %v\n", err))
		s.finishDeployment(dep, "failed", logBuf.String(), p.Name)
		return
	}
	logBuf.WriteString("Compose configuration is valid.\n")

	// Step 3: docker compose build
	logBuf.WriteString("\n=== Docker Compose Build ===\n")
	cmd = exec.Command("docker", "compose", "build")
	cmd.Dir = p.ProjectPath
	if out, err := cmd.CombinedOutput(); err != nil {
		logBuf.WriteString(string(out))
		logBuf.WriteString(fmt.Sprintf("\nbuild failed: %v\n", err))
		s.finishDeployment(dep, "failed", logBuf.String(), p.Name)
		return
	} else {
		logBuf.WriteString(string(out))
	}

	// Load .fleetdeck.yml hooks
	fdCfg := loadFleetdeckConfig(p.ProjectPath)

	// Step 4a: Run pre-deploy hook from .fleetdeck.yml
	if fdCfg != nil && fdCfg.Hooks.PreDeploy != "" {
		logBuf.WriteString("\n=== Pre-Deploy Hook ===\n")
		logBuf.WriteString(fmt.Sprintf("Running: %s\n", fdCfg.Hooks.PreDeploy))
		hookCmd := exec.Command("docker", "compose", "exec", "-T", "app", "sh", "-c", fdCfg.Hooks.PreDeploy)
		hookCmd.Dir = p.ProjectPath
		if out, err := hookCmd.CombinedOutput(); err != nil {
			logBuf.WriteString(string(out))
			logBuf.WriteString(fmt.Sprintf("\npre-deploy hook failed: %v\n", err))
			s.finishDeployment(dep, "failed", logBuf.String(), p.Name)
			return
		} else {
			logBuf.WriteString(string(out))
		}
		logBuf.WriteString("Pre-deploy hook completed.\n")
	}

	// Step 4b: docker compose up -d
	logBuf.WriteString("\n=== Docker Compose Up ===\n")
	cmd = exec.Command("docker", "compose", "up", "-d")
	cmd.Dir = p.ProjectPath
	if out, err := cmd.CombinedOutput(); err != nil {
		logBuf.WriteString(string(out))
		logBuf.WriteString(fmt.Sprintf("\ncompose up failed: %v\n", err))
		s.finishDeployment(dep, "failed", logBuf.String(), p.Name)
		return
	} else {
		logBuf.WriteString(string(out))
	}

	// Step 5: Health check with rollback
	logBuf.WriteString("\n=== Health Check ===\n")
	report := waitForHealthy(p.ProjectPath, 30*time.Second)
	if report != nil && report.Healthy {
		logBuf.WriteString("All services healthy.\n")

		// Step 6: Run post-deploy hook from .fleetdeck.yml
		if fdCfg != nil && fdCfg.Hooks.PostDeploy != "" {
			logBuf.WriteString("\n=== Post-Deploy Hook ===\n")
			logBuf.WriteString(fmt.Sprintf("Running: %s\n", fdCfg.Hooks.PostDeploy))
			hookCmd := exec.Command("docker", "compose", "exec", "-T", "app", "sh", "-c", fdCfg.Hooks.PostDeploy)
			hookCmd.Dir = p.ProjectPath
			if out, err := hookCmd.CombinedOutput(); err != nil {
				logBuf.WriteString(string(out))
				logBuf.WriteString(fmt.Sprintf("\npost-deploy hook failed: %v\n", err))
				s.finishDeployment(dep, "failed", logBuf.String(), p.Name)
				return
			} else {
				logBuf.WriteString(string(out))
			}
			logBuf.WriteString("Post-deploy hook completed.\n")
		}

		logBuf.WriteString("\nDeployment successful!\n")
		s.finishDeployment(dep, "success", logBuf.String(), p.Name)
		return
	}

	// Unhealthy — attempt rollback if we have previous state
	if report != nil {
		for _, e := range report.Errors {
			logBuf.WriteString(fmt.Sprintf("  UNHEALTHY: %s\n", e))
		}
	} else {
		logBuf.WriteString("  Could not determine health status.\n")
	}

	if len(previousImages) == 0 {
		logBuf.WriteString("\nNo previous images to rollback to — skipping rollback.\n")
		logBuf.WriteString("\nDeployment completed with health warnings.\n")
		s.finishDeployment(dep, "success", logBuf.String(), p.Name)
		return
	}

	logBuf.WriteString("\n=== Automatic Rollback ===\n")
	logBuf.WriteString("Services unhealthy after 30s — rolling back to previous images.\n")

	// Rollback: docker compose up -d to restart with previous state.
	// We do git checkout to restore the previous compose file state.
	rollbackCmd := exec.Command("git", "checkout", "HEAD~1", "--", "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml")
	rollbackCmd.Dir = p.ProjectPath
	if out, err := rollbackCmd.CombinedOutput(); err != nil {
		// Git checkout may fail if some files don't exist — that's OK, try compose up anyway.
		logBuf.WriteString(fmt.Sprintf("  git checkout previous compose files: %s (continuing)\n", strings.TrimSpace(string(out))))
	}

	upCmd := exec.Command("docker", "compose", "up", "-d")
	upCmd.Dir = p.ProjectPath
	if out, err := upCmd.CombinedOutput(); err != nil {
		logBuf.WriteString(fmt.Sprintf("  rollback compose up failed: %s: %v\n", strings.TrimSpace(string(out)), err))
		logBuf.WriteString("\nDeployment failed and rollback failed.\n")
		s.finishDeployment(dep, "failed", logBuf.String(), p.Name)
		return
	} else {
		logBuf.WriteString(string(out))
	}

	// Restore working tree to current HEAD after rollback
	restoreCmd := exec.Command("git", "checkout", "HEAD", "--", ".")
	restoreCmd.Dir = p.ProjectPath
	restoreCmd.CombinedOutput() // best-effort

	logBuf.WriteString("\nRolled back to previous state. Deployment marked as failed.\n")
	s.finishDeployment(dep, "failed", logBuf.String(), p.Name)
}

// captureContainerImages records the current image ID for each running
// container in the compose project, used for rollback decisions.
func captureContainerImages(projectPath string) map[string]string {
	cmd := exec.Command("docker", "compose", "ps", "--format", "json")
	cmd.Dir = projectPath
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	images := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		var entry struct {
			Name  string `json:"Name"`
			Image string `json:"Image"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Name != "" && entry.Image != "" {
			images[entry.Name] = entry.Image
		}
	}
	return images
}

// waitForHealthy polls the project's container health for up to the given
// duration, returning the final health report.
func waitForHealthy(projectPath string, timeout time.Duration) *healthReport {
	deadline := time.Now().Add(timeout)
	var last *healthReport

	for time.Now().Before(deadline) {
		r := checkProjectHealth(projectPath)
		last = r
		if r != nil && r.Healthy {
			return r
		}
		time.Sleep(2 * time.Second)
	}

	// One final check.
	if r := checkProjectHealth(projectPath); r != nil {
		return r
	}
	return last
}

// healthReport is a lightweight health report used internally by the webhook
// deployment pipeline (mirrors health.HealthReport but avoids the import to
// keep the server package self-contained for testing).
type healthReport struct {
	Healthy bool
	Errors  []string
}

func checkProjectHealth(projectPath string) *healthReport {
	cmd := exec.Command("docker", "compose", "ps", "--format", "json")
	cmd.Dir = projectPath
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	report := &healthReport{Healthy: true}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		report.Healthy = false
		report.Errors = append(report.Errors, "no containers found")
		return report
	}

	for _, line := range lines {
		if line == "" {
			continue
		}
		var entry struct {
			Name   string `json:"Name"`
			State  string `json:"State"`
			Health string `json:"Health"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		state := strings.ToLower(entry.State)
		hlth := strings.ToLower(entry.Health)

		if hlth == "unhealthy" || state == "restarting" {
			report.Healthy = false
			report.Errors = append(report.Errors, fmt.Sprintf("%s is %s", entry.Name, state))
		} else if state != "running" {
			report.Healthy = false
			report.Errors = append(report.Errors, fmt.Sprintf("%s has state %q", entry.Name, state))
		}
	}
	return report
}

func (s *Server) finishDeployment(dep *db.Deployment, status, logOutput, projectName string) {
	if err := s.db.UpdateDeployment(dep.ID, status, logOutput); err != nil {
		log.Printf("failed to update deployment %s: %v", dep.ID, err)
	}

	finalStatus := "running"
	if status == "failed" {
		finalStatus = "error"
	}
	s.db.UpdateProjectStatus(projectName, finalStatus)
}

func (s *Server) findProjectByRepo(repoName string) *db.Project {
	projects, err := s.db.ListProjects()
	if err != nil {
		return nil
	}
	for _, p := range projects {
		if strings.EqualFold(p.GitHubRepo, repoName) {
			return p
		}
	}
	return nil
}

func verifyHMAC(body []byte, signature, secret string) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	sig, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)
	return hmac.Equal(sig, expected)
}

// fleetdeckConfig represents the project-level .fleetdeck.yml configuration.
type fleetdeckConfig struct {
	Hooks struct {
		PreDeploy  string `yaml:"pre_deploy"`
		PostDeploy string `yaml:"post_deploy"`
	} `yaml:"hooks"`
}

// loadFleetdeckConfig reads a .fleetdeck.yml file from the project directory
// and returns the parsed configuration.
func loadFleetdeckConfig(projectPath string) *fleetdeckConfig {
	for _, name := range []string{".fleetdeck.yml", "fleetdeck.yml"} {
		data, err := os.ReadFile(filepath.Join(projectPath, name))
		if err != nil {
			continue
		}
		var cfg fleetdeckConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			log.Printf("warning: failed to parse %s: %v", name, err)
			return nil
		}
		return &cfg
	}
	return nil
}
