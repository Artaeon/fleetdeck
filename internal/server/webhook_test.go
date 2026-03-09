package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/db"
)

func TestHandleGitHubWebhookPing(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/webhook/github", strings.NewReader(`{}`))
	req.Header.Set("X-GitHub-Event", "ping")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "pong" {
		t.Errorf("expected pong, got %s", resp["status"])
	}
}

func TestHandleGitHubWebhookIgnoresNonPush(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/webhook/github", strings.NewReader(`{}`))
	req.Header.Set("X-GitHub-Event", "issues")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ignored" {
		t.Errorf("expected ignored, got %s", resp["status"])
	}
}

func TestHandleGitHubWebhookIgnoresNonMainBranch(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"ref":"refs/heads/feature-branch","after":"abc123","repository":{"full_name":"org/repo"}}`
	req := httptest.NewRequest("POST", "/api/webhook/github", strings.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ignored" {
		t.Errorf("expected ignored, got %s", resp["status"])
	}
}

func TestHandleGitHubWebhookNoMatchingProject(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"ref":"refs/heads/main","after":"abc123def456","repository":{"full_name":"org/nonexistent"}}`
	req := httptest.NewRequest("POST", "/api/webhook/github", strings.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestVerifyHMAC(t *testing.T) {
	secret := "test-secret"
	body := []byte("hello world")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !verifyHMAC(body, sig, secret) {
		t.Error("valid HMAC should verify")
	}

	if verifyHMAC(body, "sha256=invalid", secret) {
		t.Error("invalid HMAC should not verify")
	}

	if verifyHMAC(body, "invalid-format", secret) {
		t.Error("missing sha256= prefix should not verify")
	}
}

func TestHandleListDeployments(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	p := &db.Project{
		Name:        "deployapp",
		Domain:      "deploy.io",
		LinuxUser:   "fleetdeck-deployapp",
		ProjectPath: dir,
		Template:    "node",
	}
	database.CreateProject(p)

	req := httptest.NewRequest("GET", "/api/projects/deployapp/deployments", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleGitHubWebhookWithHMACValidation(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.webhookSecret = "mysecret"

	body := `{"ref":"refs/heads/main","after":"abc123","repository":{"full_name":"org/repo"}}`

	// Without signature — should fail
	req := httptest.NewRequest("POST", "/api/webhook/github", strings.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without signature, got %d", w.Code)
	}

	// With valid signature
	mac := hmac.New(sha256.New, []byte("mysecret"))
	mac.Write([]byte(body))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req = httptest.NewRequest("POST", "/api/webhook/github", strings.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", sig)
	w = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	// Will be 404 because no project matches, but NOT 401
	if w.Code == http.StatusUnauthorized {
		t.Error("valid signature should not return 401")
	}
}
