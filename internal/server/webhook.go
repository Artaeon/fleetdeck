package server

import (
	"context"
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

	// Dedupe redeliveries by X-GitHub-Delivery. GitHub retries webhooks
	// when our response times out, which is routine for multi-minute
	// deploys — without this check we'd start a second parallel deploy
	// of the same commit every 10 seconds until the first one finished.
	if deliveryID := r.Header.Get("X-GitHub-Delivery"); s.webhookDedup != nil && s.webhookDedup.seenRecently(deliveryID) {
		writeJSON(w, map[string]string{
			"status":   "ignored",
			"reason":   "duplicate delivery",
			"delivery": deliveryID,
		})
		return
	}

	var payload githubPushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	// Find matching project by GitHub repo
	repoName := payload.Repository.FullName
	project := s.findProjectByRepo(repoName)
	if project == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("no project for repo %s", repoName))
		return
	}

	// Extract branch name and resolve target environment
	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")
	targetEnv := resolveBranchEnvironment(project, branch)
	if targetEnv == "" {
		writeJSON(w, map[string]string{"status": "ignored", "reason": "branch not mapped"})
		return
	}

	commitSHA := payload.After
	if len(commitSHA) > 12 {
		commitSHA = commitSHA[:12]
	}

	// Start async deployment, tracked via asyncJobs so Shutdown waits
	// for it before closing the DB handle. The context is the server's
	// shutdownCtx, which runDeployment threads through to every
	// exec.CommandContext so a SIGTERM mid-deploy cancels git / docker
	// subprocesses instead of letting them run to completion against
	// a DB that is about to close.
	p := project
	sha := payload.After
	s.goAsyncJob(func(ctx context.Context) { s.runDeployment(ctx, p, sha, commitSHA) })

	writeJSON(w, map[string]string{
		"status":      "deploying",
		"project":     project.Name,
		"commit":      commitSHA,
		"environment": targetEnv,
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

	proj := p
	s.goAsyncJob(func(ctx context.Context) { s.runDeployment(ctx, proj, "", "manual") })

	writeJSON(w, map[string]string{"status": "deploying", "project": p.Name})
}



// runStep wraps exec.CommandContext with a per-step timeout and
// appends its output to logBuf regardless of outcome. Returns the
// error from the command (including context.DeadlineExceeded on
// timeout) so the caller can decide whether to abort or continue.
//
// The timeout replaces the previous "wait forever" behavior: a hung
// docker daemon or a DNS stall during git pull now produces a clean
// "step X timed out after N" line in the deployment log rather than
// pinning a goroutine until the process exits.
func runStep(ctx context.Context, dir string, timeout time.Duration, logBuf *strings.Builder, name string, args ...string) error {
	stepCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(stepCtx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		logBuf.Write(out)
		if !strings.HasSuffix(string(out), "\n") {
			logBuf.WriteString("\n")
		}
	}
	if stepCtx.Err() == context.DeadlineExceeded {
		logBuf.WriteString(fmt.Sprintf("(step timed out after %s)\n", timeout.Round(time.Second)))
		return fmt.Errorf("%s timed out after %s", name, timeout.Round(time.Second))
	}
	if ctx.Err() == context.Canceled {
		logBuf.WriteString("(deploy cancelled)\n")
		return fmt.Errorf("deploy cancelled: %w", ctx.Err())
	}
	return err
}

// Per-step timeouts. Generous enough for real workloads (a 1 GB
// docker-image build on a modest VPS takes 5-10 min), tight enough
// that a genuinely hung step doesn't burn a webhook worker slot
// for the life of the process.
const (
	stepTimeoutGitPull     = 2 * time.Minute
	stepTimeoutComposeBuild = 15 * time.Minute
	stepTimeoutComposeUp    = 5 * time.Minute
	stepTimeoutComposeExec  = 10 * time.Minute
	stepTimeoutGitCheckout  = 30 * time.Second
)

func (s *Server) runDeployment(ctx context.Context, p *db.Project, fullSHA, shortSHA string) {
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

	// Report deployment status to GitHub
	reportStatus := s.reportDeploymentStatus(p.GitHubRepo, fullSHA, "production")
	defer func() {
		reportStatus(dep.Status == "success")
	}()

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
	if err := runStep(ctx, p.ProjectPath, stepTimeoutGitPull, &logBuf, "git", "pull", "--ff-only"); err != nil {
		logBuf.WriteString(fmt.Sprintf("git pull failed: %v\n", err))
		s.finishDeployment(dep, "failed", logBuf.String(), p.Name)
		return
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
	if err := runStep(ctx, p.ProjectPath, stepTimeoutComposeBuild, &logBuf, "docker", "compose", "build"); err != nil {
		logBuf.WriteString(fmt.Sprintf("build failed: %v\n", err))
		s.finishDeployment(dep, "failed", logBuf.String(), p.Name)
		return
	}

	// Load .fleetdeck.yml hooks
	fdCfg := loadFleetdeckConfig(p.ProjectPath)

	// Step 4a: Run pre-deploy hook from .fleetdeck.yml
	if fdCfg != nil && fdCfg.Hooks.PreDeploy != "" {
		logBuf.WriteString("\n=== Pre-Deploy Hook ===\n")
		logBuf.WriteString(fmt.Sprintf("Running: %s\n", fdCfg.Hooks.PreDeploy))
		if err := runStep(ctx, p.ProjectPath, stepTimeoutComposeExec, &logBuf,
			"docker", "compose", "exec", "-T", "app", "sh", "-c", fdCfg.Hooks.PreDeploy); err != nil {
			logBuf.WriteString(fmt.Sprintf("pre-deploy hook failed: %v\n", err))
			s.finishDeployment(dep, "failed", logBuf.String(), p.Name)
			return
		}
		logBuf.WriteString("Pre-deploy hook completed.\n")
	}

	// Step 4b: docker compose up -d
	logBuf.WriteString("\n=== Docker Compose Up ===\n")
	if err := runStep(ctx, p.ProjectPath, stepTimeoutComposeUp, &logBuf, "docker", "compose", "up", "-d"); err != nil {
		logBuf.WriteString(fmt.Sprintf("compose up failed: %v\n", err))
		s.finishDeployment(dep, "failed", logBuf.String(), p.Name)
		return
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
			if err := runStep(ctx, p.ProjectPath, stepTimeoutComposeExec, &logBuf,
				"docker", "compose", "exec", "-T", "app", "sh", "-c", fdCfg.Hooks.PostDeploy); err != nil {
				logBuf.WriteString(fmt.Sprintf("post-deploy hook failed: %v\n", err))
				s.finishDeployment(dep, "failed", logBuf.String(), p.Name)
				return
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
	// Rollback uses the parent context (not ctx) deliberately: when
	// the deploy was cancelled via SIGTERM, ctx is already Done and
	// the rollback would be a no-op. Use a fresh Background context
	// with a short timeout so rollback still gets a chance to run.
	rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer rollbackCancel()

	if err := runStep(rollbackCtx, p.ProjectPath, stepTimeoutGitCheckout, &logBuf,
		"git", "checkout", "HEAD~1", "--", "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"); err != nil {
		// Git checkout may fail if some files don't exist — that's OK, try compose up anyway.
		logBuf.WriteString(fmt.Sprintf("  git checkout previous compose files returned: %v (continuing)\n", err))
	}

	if err := runStep(rollbackCtx, p.ProjectPath, stepTimeoutComposeUp, &logBuf, "docker", "compose", "up", "-d"); err != nil {
		logBuf.WriteString(fmt.Sprintf("  rollback compose up failed: %v\n", err))
		logBuf.WriteString("\nDeployment failed and rollback failed.\n")
		s.finishDeployment(dep, "failed", logBuf.String(), p.Name)
		return
	}

	// Restore working tree to current HEAD after rollback (best-effort,
	// uses the rollback context so it also bounded by the 2-min budget).
	restoreCmd := exec.CommandContext(rollbackCtx, "git", "checkout", "HEAD", "--", ".")
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
	dep.Status = status
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

// resolveBranchEnvironment determines which environment a branch should deploy to.
// Returns "" if the branch is not mapped to any environment.
func resolveBranchEnvironment(p *db.Project, branch string) string {
	// Check explicit branch mappings first
	if p.BranchMappings != "" {
		var mappings map[string]string
		if err := json.Unmarshal([]byte(p.BranchMappings), &mappings); err == nil {
			if env, ok := mappings[branch]; ok {
				return env
			}
		}
	}

	// Default: main/master -> production
	if branch == "main" || branch == "master" {
		return "production"
	}

	return ""
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
