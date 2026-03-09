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
	"os/exec"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/db"
	"github.com/google/uuid"
)

// WebhookConfig holds webhook authentication settings.
type WebhookConfig struct {
	Secret string // HMAC secret for verifying GitHub webhooks
}

// AddWebhookRoutes registers webhook endpoints on the given mux.
func (s *Server) AddWebhookRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/webhook/github", s.handleGitHubWebhook)
	mux.HandleFunc("POST /api/webhook/deploy/{name}", s.handleManualDeploy)
	mux.HandleFunc("GET /api/projects/{name}/deployments", s.handleListDeployments)
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

	// Verify HMAC signature if webhook secret is configured
	if s.webhookSecret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if !verifyHMAC(body, sig, s.webhookSecret) {
			writeError(w, http.StatusUnauthorized, "invalid signature")
			return
		}
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
	p, err := s.db.GetProject(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	go s.runDeployment(p, "", "manual")

	writeJSON(w, map[string]string{"status": "deploying", "project": p.Name})
}

func (s *Server) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p, err := s.db.GetProject(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	deployments, err := s.db.ListDeployments(p.ID, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type apiDeployment struct {
		ID         string     `json:"id"`
		CommitSHA  string     `json:"commit_sha"`
		Status     string     `json:"status"`
		StartedAt  time.Time  `json:"started_at"`
		FinishedAt *time.Time `json:"finished_at,omitempty"`
		Log        string     `json:"log,omitempty"`
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

func (s *Server) runDeployment(p *db.Project, fullSHA, shortSHA string) {
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

	// Step 1: git pull
	logBuf.WriteString("=== Git Pull ===\n")
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

	// Step 2: docker compose build
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

	// Step 3: docker compose up -d
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

	logBuf.WriteString("\nDeployment successful!\n")
	s.finishDeployment(dep, "success", logBuf.String(), p.Name)
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
