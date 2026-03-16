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

const testWebhookSecret = "test-webhook-secret"

// signedWebhookRequest creates a POST request to the webhook endpoint with a
// valid HMAC signature. The test server's webhookSecret must be set to
// testWebhookSecret.
func signedWebhookRequest(t *testing.T, body, event string) *http.Request {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(testWebhookSecret))
	mac.Write([]byte(body))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/api/webhook/github", strings.NewReader(body))
	req.Header.Set("X-GitHub-Event", event)
	req.Header.Set("X-Hub-Signature-256", sig)
	return req
}

func TestHandleGitHubWebhookPing(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.webhookSecret = testWebhookSecret

	req := signedWebhookRequest(t, `{}`, "ping")
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
	srv.webhookSecret = testWebhookSecret

	req := signedWebhookRequest(t, `{}`, "issues")
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
	srv.webhookSecret = testWebhookSecret

	body := `{"ref":"refs/heads/feature-branch","after":"abc123","repository":{"full_name":"org/repo"}}`
	req := signedWebhookRequest(t, body, "push")
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
	srv.webhookSecret = testWebhookSecret

	body := `{"ref":"refs/heads/main","after":"abc123def456","repository":{"full_name":"org/nonexistent"}}`
	req := signedWebhookRequest(t, body, "push")
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

// --- findProjectByRepo tests ---

func TestFindProjectByRepoMatch(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "myapp",
		Domain:      "myapp.com",
		LinuxUser:   "fleetdeck-myapp",
		ProjectPath: dir,
		GitHubRepo:  "myorg/myapp",
	})

	p := srv.findProjectByRepo("myorg/myapp")
	if p == nil {
		t.Fatal("expected to find project by repo")
	}
	if p.Name != "myapp" {
		t.Errorf("expected name myapp, got %s", p.Name)
	}
}

func TestFindProjectByRepoCaseInsensitive(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "caseapp",
		Domain:      "case.com",
		LinuxUser:   "fleetdeck-caseapp",
		ProjectPath: dir,
		GitHubRepo:  "MyOrg/CaseApp",
	})

	p := srv.findProjectByRepo("myorg/caseapp")
	if p == nil {
		t.Fatal("expected case-insensitive match")
	}
	if p.Name != "caseapp" {
		t.Errorf("expected name caseapp, got %s", p.Name)
	}
}

func TestFindProjectByRepoNoMatch(t *testing.T) {
	srv, _ := setupTestServer(t)

	p := srv.findProjectByRepo("org/nonexistent")
	if p != nil {
		t.Errorf("expected nil for non-matching repo, got %v", p.Name)
	}
}

func TestFindProjectByRepoEmptyDB(t *testing.T) {
	srv, _ := setupTestServer(t)

	p := srv.findProjectByRepo("any/repo")
	if p != nil {
		t.Error("expected nil for empty database")
	}
}

func TestFindProjectByRepoMultipleProjects(t *testing.T) {
	srv, database := setupTestServer(t)

	dir1 := t.TempDir()
	dir2 := t.TempDir()
	dir3 := t.TempDir()

	database.CreateProject(&db.Project{
		Name: "alpha", Domain: "alpha.com", LinuxUser: "fleetdeck-alpha",
		ProjectPath: dir1, GitHubRepo: "org/alpha",
	})
	database.CreateProject(&db.Project{
		Name: "beta", Domain: "beta.com", LinuxUser: "fleetdeck-beta",
		ProjectPath: dir2, GitHubRepo: "org/beta",
	})
	database.CreateProject(&db.Project{
		Name: "gamma", Domain: "gamma.com", LinuxUser: "fleetdeck-gamma",
		ProjectPath: dir3, GitHubRepo: "org/gamma",
	})

	p := srv.findProjectByRepo("org/beta")
	if p == nil {
		t.Fatal("expected to find beta")
	}
	if p.Name != "beta" {
		t.Errorf("expected name beta, got %s", p.Name)
	}
}

// --- Start/Stop/Restart handler tests ---

func TestHandleStartProjectNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/projects/nonexistent/start", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] == "" {
		t.Error("expected error message in response")
	}
}

func TestHandleStopProjectNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/projects/nonexistent/stop", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] == "" {
		t.Error("expected error message in response")
	}
}

func TestHandleRestartProjectNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/projects/nonexistent/restart", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] == "" {
		t.Error("expected error message in response")
	}
}

func TestHandleStartProjectLookupSucceeds(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "startme",
		Domain:      "start.com",
		LinuxUser:   "fleetdeck-startme",
		ProjectPath: dir,
		Template:    "node",
	})

	req := httptest.NewRequest("POST", "/api/projects/startme/start", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	// Project lookup succeeds, but docker compose will fail (no docker).
	// We expect 500 (compose up failed), NOT 404.
	if w.Code == http.StatusNotFound {
		t.Error("expected project lookup to succeed (not 404)")
	}
}

func TestHandleStopProjectLookupSucceeds(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "stopme",
		Domain:      "stop.com",
		LinuxUser:   "fleetdeck-stopme",
		ProjectPath: dir,
		Template:    "node",
	})

	req := httptest.NewRequest("POST", "/api/projects/stopme/stop", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	// Project lookup succeeds, but docker compose will fail.
	// We expect 500, NOT 404.
	if w.Code == http.StatusNotFound {
		t.Error("expected project lookup to succeed (not 404)")
	}
}

func TestHandleRestartProjectLookupSucceeds(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "restartme",
		Domain:      "restart.com",
		LinuxUser:   "fleetdeck-restartme",
		ProjectPath: dir,
		Template:    "node",
	})

	req := httptest.NewRequest("POST", "/api/projects/restartme/restart", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	// Project lookup succeeds, docker compose fails.
	if w.Code == http.StatusNotFound {
		t.Error("expected project lookup to succeed (not 404)")
	}
}

// --- Logs handler tests ---

func TestHandleProjectLogsNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/projects/nonexistent/logs", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleProjectLogsValidLines(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "logapp",
		Domain:      "log.com",
		LinuxUser:   "fleetdeck-logapp",
		ProjectPath: dir,
		Template:    "node",
	})

	req := httptest.NewRequest("GET", "/api/projects/logapp/logs?lines=50", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	// Project found, lines param valid. Docker compose will fail but
	// the handler still returns 200 with whatever output (possibly empty).
	if w.Code == http.StatusNotFound {
		t.Error("expected project lookup to succeed (not 404)")
	}
	if w.Code == http.StatusBadRequest {
		t.Error("lines=50 should be valid (not 400)")
	}
}

func TestHandleProjectLogsDefaultLines(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "logdefault",
		Domain:      "logdef.com",
		LinuxUser:   "fleetdeck-logdefault",
		ProjectPath: dir,
		Template:    "node",
	})

	// No lines param — defaults to 100
	req := httptest.NewRequest("GET", "/api/projects/logdefault/logs", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code == http.StatusBadRequest {
		t.Error("default lines param should be valid (not 400)")
	}
	if w.Code == http.StatusNotFound {
		t.Error("expected project lookup to succeed (not 404)")
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
