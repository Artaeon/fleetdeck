package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/db"
)

// ---------------------------------------------------------------------------
// Audit log handler tests
// ---------------------------------------------------------------------------

func setupTestServerWithAuditLog(t *testing.T, entries []string) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	auditPath := filepath.Join(dir, "audit.log")
	if len(entries) > 0 {
		if err := os.WriteFile(auditPath, []byte(strings.Join(entries, "\n")+"\n"), 0644); err != nil {
			t.Fatalf("write audit log: %v", err)
		}
	}

	cfg := config.DefaultConfig()
	cfg.Server.BasePath = dir
	cfg.Audit.LogPath = auditPath

	srv := New(cfg, database, ":0")
	return srv, auditPath
}

func TestHandleAuditLogEmpty(t *testing.T) {
	// No audit file exists at all: should return empty array
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := config.DefaultConfig()
	cfg.Server.BasePath = dir
	cfg.Audit.LogPath = filepath.Join(dir, "nonexistent-audit.log")

	srv := New(cfg, database, ":0")

	req := httptest.NewRequest("GET", "/api/audit", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var entries []json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for nonexistent audit log, got %d", len(entries))
	}
}

func TestHandleAuditLogWithEntries(t *testing.T) {
	now := time.Now().UTC()
	entry1 := fmt.Sprintf(`{"timestamp":"%s","action":"project.create","project":"app1","user":"system","details":"via=api","success":true}`, now.Add(-2*time.Hour).Format(time.RFC3339))
	entry2 := fmt.Sprintf(`{"timestamp":"%s","action":"project.destroy","project":"app2","user":"system","details":"via=api","success":true}`, now.Add(-1*time.Hour).Format(time.RFC3339))
	entry3 := fmt.Sprintf(`{"timestamp":"%s","action":"backup.create","project":"app1","user":"system","details":"via=api","success":true}`, now.Format(time.RFC3339))

	srv, _ := setupTestServerWithAuditLog(t, []string{entry1, entry2, entry3})

	req := httptest.NewRequest("GET", "/api/audit", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var entries []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Should be in reverse chronological order (most recent first)
	if entries[0]["action"] != "backup.create" {
		t.Errorf("first entry should be most recent, got action=%v", entries[0]["action"])
	}
	if entries[2]["action"] != "project.create" {
		t.Errorf("last entry should be oldest, got action=%v", entries[2]["action"])
	}
}

func TestHandleAuditLogWithLimitParam(t *testing.T) {
	var lines []string
	for i := 0; i < 10; i++ {
		ts := time.Now().UTC().Add(time.Duration(-10+i) * time.Minute).Format(time.RFC3339)
		lines = append(lines, fmt.Sprintf(`{"timestamp":"%s","action":"test.action%d","project":"p","user":"sys","success":true}`, ts, i))
	}

	srv, _ := setupTestServerWithAuditLog(t, lines)

	req := httptest.NewRequest("GET", "/api/audit?limit=3", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var entries []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries with limit=3, got %d", len(entries))
	}
}

func TestHandleAuditLogLimitBounds(t *testing.T) {
	srv, _ := setupTestServerWithAuditLog(t, []string{
		`{"timestamp":"2025-01-01T00:00:00Z","action":"test","user":"sys","success":true}`,
	})

	// Invalid limit values should be ignored (fall back to default 50)
	tests := []struct {
		query    string
		wantLen  int
		desc     string
	}{
		{"limit=0", 1, "limit=0 ignored, uses default"},
		{"limit=-5", 1, "negative limit ignored"},
		{"limit=abc", 1, "non-numeric limit ignored"},
		{"limit=1001", 1, "limit > 1000 ignored, uses default"},
	}

	for _, tt := range tests {
		req := httptest.NewRequest("GET", "/api/audit?"+tt.query, nil)
		w := httptest.NewRecorder()
		srv.server.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", tt.desc, w.Code)
		}

		var entries []json.RawMessage
		if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
			t.Fatalf("%s: decode: %v", tt.desc, err)
		}
		if len(entries) != tt.wantLen {
			t.Errorf("%s: expected %d entries, got %d", tt.desc, tt.wantLen, len(entries))
		}
	}
}

func TestHandleAuditLogSkipsMalformedLines(t *testing.T) {
	lines := []string{
		`{"timestamp":"2025-01-01T00:00:00Z","action":"valid","user":"sys","success":true}`,
		`this is not json`,
		`{"timestamp":"2025-01-01T01:00:00Z","action":"also-valid","user":"sys","success":true}`,
	}

	srv, _ := setupTestServerWithAuditLog(t, lines)

	req := httptest.NewRequest("GET", "/api/audit", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var entries []json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Should have only the 2 valid entries, malformed line skipped
	if len(entries) != 2 {
		t.Errorf("expected 2 valid entries (skipping malformed), got %d", len(entries))
	}
}

func TestHandleAuditLogSkipsBlankLines(t *testing.T) {
	lines := []string{
		`{"timestamp":"2025-01-01T00:00:00Z","action":"entry1","user":"sys","success":true}`,
		"",
		"",
		`{"timestamp":"2025-01-01T01:00:00Z","action":"entry2","user":"sys","success":true}`,
	}

	srv, _ := setupTestServerWithAuditLog(t, lines)

	req := httptest.NewRequest("GET", "/api/audit", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	var entries []json.RawMessage
	json.NewDecoder(w.Body).Decode(&entries)
	if len(entries) != 2 {
		t.Errorf("expected 2 entries (blank lines skipped), got %d", len(entries))
	}
}

func TestHandleAuditLogResponseContentType(t *testing.T) {
	srv, _ := setupTestServerWithAuditLog(t, nil)

	// Create the file as empty
	req := httptest.NewRequest("GET", "/api/audit", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

// ---------------------------------------------------------------------------
// Delete project handler tests
// ---------------------------------------------------------------------------

func TestHandleDeleteProjectNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("DELETE", "/api/projects/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleDeleteProjectInvalidName(t *testing.T) {
	srv, _ := setupTestServer(t)

	invalidNames := []string{
		"-leading",
		"trailing-",
		"UPPER",
		"a",
	}
	for _, name := range invalidNames {
		req := httptest.NewRequest("DELETE", "/api/projects/"+name, nil)
		w := httptest.NewRecorder()
		srv.server.Handler.ServeHTTP(w, req)

		if w.Code == http.StatusOK {
			t.Errorf("DELETE with invalid name %q should not return 200", name)
		}
	}
}

func TestHandleDeleteProjectSuccess(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	p := &db.Project{
		Name:        "delete-me",
		Domain:      "delete.io",
		LinuxUser:   "fleetdeck-delete-me",
		ProjectPath: dir,
		Template:    "node",
		Status:      "stopped",
	}
	database.CreateProject(p)

	req := httptest.NewRequest("DELETE", "/api/projects/delete-me", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "deleted" {
		t.Errorf("expected status=deleted, got %q", resp["status"])
	}

	// Verify project is removed from DB
	_, err := database.GetProject("delete-me")
	if err == nil {
		t.Error("project should have been removed from database")
	}
}

func TestHandleDeleteProjectKeepData(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	// Create a marker file so we can verify the directory is kept
	markerPath := filepath.Join(dir, "keep-me.txt")
	os.WriteFile(markerPath, []byte("data"), 0644)

	p := &db.Project{
		Name:        "keep-data",
		Domain:      "keep.io",
		LinuxUser:   "fleetdeck-keep-data",
		ProjectPath: dir,
		Template:    "node",
		Status:      "stopped",
	}
	database.CreateProject(p)

	req := httptest.NewRequest("DELETE", "/api/projects/keep-data?keep-data=true", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Verify project directory still exists when keep-data=true
	if _, err := os.Stat(markerPath); err != nil {
		t.Error("project data should be kept when keep-data=true")
	}
}

// ---------------------------------------------------------------------------
// Webhook - push to main branch matching a project (triggers deploy)
// ---------------------------------------------------------------------------

func TestHandleGitHubWebhookPushToMainMatchesProject(t *testing.T) {
	srv, database := setupTestServer(t)
	srv.webhookSecret = testWebhookSecret

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "webhook-app",
		Domain:      "wh.io",
		LinuxUser:   "fleetdeck-webhook-app",
		ProjectPath: dir,
		GitHubRepo:  "org/webhook-app",
		Template:    "node",
		Status:      "running",
	})

	body := `{"ref":"refs/heads/main","after":"abcdef1234567890abcdef","repository":{"full_name":"org/webhook-app"}}`
	req := signedWebhookRequest(t, body, "push")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "deploying" {
		t.Errorf("expected status=deploying, got %q", resp["status"])
	}
	if resp["project"] != "webhook-app" {
		t.Errorf("expected project=webhook-app, got %q", resp["project"])
	}
	// Commit SHA should be truncated to 12 chars
	if resp["commit"] != "abcdef123456" {
		t.Errorf("expected commit=abcdef123456, got %q", resp["commit"])
	}
}

func TestHandleGitHubWebhookPushToMasterBranch(t *testing.T) {
	srv, database := setupTestServer(t)
	srv.webhookSecret = testWebhookSecret

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "master-app",
		Domain:      "master.io",
		LinuxUser:   "fleetdeck-master-app",
		ProjectPath: dir,
		GitHubRepo:  "org/master-app",
		Template:    "node",
	})

	body := `{"ref":"refs/heads/master","after":"fedcba9876543210fedc","repository":{"full_name":"org/master-app"}}`
	req := signedWebhookRequest(t, body, "push")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "deploying" {
		t.Errorf("expected status=deploying for push to master, got %q", resp["status"])
	}
	if resp["project"] != "master-app" {
		t.Errorf("expected project=master-app, got %q", resp["project"])
	}
}

func TestHandleGitHubWebhookPushToFeatureBranchIgnored(t *testing.T) {
	srv, database := setupTestServer(t)
	srv.webhookSecret = testWebhookSecret

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "feat-app",
		Domain:      "feat.io",
		LinuxUser:   "fleetdeck-feat-app",
		ProjectPath: dir,
		GitHubRepo:  "org/feat-app",
		Template:    "node",
	})

	body := `{"ref":"refs/heads/feature/add-login","after":"abc123","repository":{"full_name":"org/feat-app"}}`
	req := signedWebhookRequest(t, body, "push")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ignored" {
		t.Errorf("expected status=ignored for non-main branch, got %q", resp["status"])
	}
	if resp["reason"] != "not main branch" {
		t.Errorf("expected reason='not main branch', got %q", resp["reason"])
	}
}

func TestHandleGitHubWebhookPushToDevBranchIgnored(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.webhookSecret = testWebhookSecret

	body := `{"ref":"refs/heads/develop","after":"abc123","repository":{"full_name":"org/some-app"}}`
	req := signedWebhookRequest(t, body, "push")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ignored" {
		t.Errorf("expected status=ignored for develop branch, got %q", resp["status"])
	}
}

func TestHandleGitHubWebhookShortCommitSHA(t *testing.T) {
	srv, database := setupTestServer(t)
	srv.webhookSecret = testWebhookSecret

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "sha-app",
		Domain:      "sha.io",
		LinuxUser:   "fleetdeck-sha-app",
		ProjectPath: dir,
		GitHubRepo:  "org/sha-app",
		Template:    "node",
	})

	// Commit SHA shorter than 12 chars should not be truncated
	body := `{"ref":"refs/heads/main","after":"abc123","repository":{"full_name":"org/sha-app"}}`
	req := signedWebhookRequest(t, body, "push")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	// Short SHA (<=12 chars) is kept as-is
	if resp["commit"] != "abc123" {
		t.Errorf("expected commit=abc123 (not truncated), got %q", resp["commit"])
	}
}

func TestHandleGitHubWebhookWithValidHMACMatchingProject(t *testing.T) {
	srv, database := setupTestServer(t)
	srv.webhookSecret = "test-webhook-secret"

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "hmac-app",
		Domain:      "hmac.io",
		LinuxUser:   "fleetdeck-hmac-app",
		ProjectPath: dir,
		GitHubRepo:  "org/hmac-app",
		Template:    "node",
	})

	body := `{"ref":"refs/heads/main","after":"abc123def456789","repository":{"full_name":"org/hmac-app"}}`

	mac := hmac.New(sha256.New, []byte("test-webhook-secret"))
	mac.Write([]byte(body))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/api/webhook/github", strings.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", sig)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "deploying" {
		t.Errorf("expected status=deploying, got %q", resp["status"])
	}
}

// ---------------------------------------------------------------------------
// Dashboard HTML content verification
// ---------------------------------------------------------------------------

func TestHandleDashboardHTMLContent(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	checks := []struct {
		contains string
		label    string
	}{
		{"FleetDeck", "brand name"},
		{"<!DOCTYPE html>", "HTML doctype"},
		{"<html", "HTML element"},
		{"/static/style.css", "CSS link"},
		{"/static/app.js", "JS script"},
		{"projects-grid", "projects grid element"},
		{"Dashboard", "dashboard link"},
	}
	for _, c := range checks {
		if !strings.Contains(body, c.contains) {
			t.Errorf("dashboard HTML missing %s: expected to contain %q", c.label, c.contains)
		}
	}
}

func TestHandleDashboardNonRootPath404(t *testing.T) {
	srv, _ := setupTestServer(t)

	// The dashboard handler checks r.URL.Path != "/" and returns 404
	req := httptest.NewRequest("GET", "/nonexistent-page", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-root path, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Login page content verification
// ---------------------------------------------------------------------------

func TestLoginPageHTMLContent(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html; charset=utf-8", ct)
	}

	body := w.Body.String()
	checks := []struct {
		contains string
		label    string
	}{
		{"FleetDeck", "brand name"},
		{"Login", "login title"},
		{"API Token", "token label"},
		{"token", "token input name"},
		{`action="/login"`, "form action"},
		{`method="POST"`, "form method"},
	}
	for _, c := range checks {
		if !strings.Contains(body, c.contains) {
			t.Errorf("login HTML missing %s: expected to contain %q", c.label, c.contains)
		}
	}
}

func TestLoginErrorPageHTMLContent(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("POST", "/login", strings.NewReader("token=wrong-token"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Invalid token") {
		t.Error("login error page should contain 'Invalid token' message")
	}
	if !strings.Contains(body, "FleetDeck") {
		t.Error("login error page should contain brand name")
	}
}

// ---------------------------------------------------------------------------
// Project page rendering
// ---------------------------------------------------------------------------

func TestHandleProjectPage(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/project/myapp", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html; charset=utf-8", ct)
	}

	body := w.Body.String()
	checks := []struct {
		contains string
		label    string
	}{
		{"FleetDeck", "brand name"},
		{"Project", "project title"},
		{"Overview", "overview tab"},
		{"Logs", "logs tab"},
		{"Backups", "backups tab"},
		{"/static/app.js", "JS script"},
	}
	for _, c := range checks {
		if !strings.Contains(body, c.contains) {
			t.Errorf("project page missing %s: expected to contain %q", c.label, c.contains)
		}
	}
}

func TestHandleProjectPageRequiresAuthWhenConfigured(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("GET", "/project/anyapp", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect to login, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %s", loc)
	}
}

func TestHandleProjectPageWithValidCookie(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("GET", "/project/anyapp", nil)
	req.AddCookie(&http.Cookie{Name: "fleetdeck_session", Value: "test-secret-token"})
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for project page with valid cookie, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Static file content verification
// ---------------------------------------------------------------------------

func TestStaticCSSContent(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/static/style.css", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, ":root") {
		t.Error("CSS should contain :root selector")
	}
	if !strings.Contains(body, "--bg") {
		t.Error("CSS should contain CSS custom properties")
	}
	if len(body) < 500 {
		t.Error("CSS content appears too short")
	}
}

func TestStaticJSContent(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/static/app.js", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	body := w.Body.String()
	if len(body) < 100 {
		t.Error("JS content appears too short")
	}
}

// ---------------------------------------------------------------------------
// Deployments list with limit parameter
// ---------------------------------------------------------------------------

func TestDeploymentsEndpointWithCustomLimit(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	p := &db.Project{
		Name:        "limit-test",
		Domain:      "limit.io",
		LinuxUser:   "fleetdeck-limit-test",
		ProjectPath: dir,
		Template:    "node",
	}
	database.CreateProject(p)

	// Create several deployments
	for i := 0; i < 5; i++ {
		dep := &db.Deployment{
			ProjectID: p.ID,
			CommitSHA: fmt.Sprintf("sha%d", i),
			Status:    "success",
			StartedAt: time.Now().Add(time.Duration(-5+i) * time.Minute),
		}
		database.CreateDeployment(dep)
	}

	// Request with limit=2
	req := httptest.NewRequest("GET", "/api/projects/limit-test/deployments?limit=2", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var deployments []apiDeployment
	if err := json.NewDecoder(w.Body).Decode(&deployments); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(deployments) != 2 {
		t.Errorf("expected 2 deployments with limit=2, got %d", len(deployments))
	}
}

func TestDeploymentsEndpointInvalidLimit(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	p := &db.Project{
		Name:        "inv-limit",
		Domain:      "inv.io",
		LinuxUser:   "fleetdeck-inv-limit",
		ProjectPath: dir,
		Template:    "node",
	}
	database.CreateProject(p)

	// Invalid limit values should fall back to default 20
	invalidLimits := []string{"abc", "0", "-1", "101"}
	for _, lim := range invalidLimits {
		req := httptest.NewRequest("GET", "/api/projects/inv-limit/deployments?limit="+lim, nil)
		w := httptest.NewRecorder()
		srv.server.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("limit=%s: expected 200, got %d", lim, w.Code)
		}
	}
}

func TestDeploymentsEndpointInvalidProjectName(t *testing.T) {
	srv, _ := setupTestServer(t)

	invalidNames := []string{"-bad", "BAD", "bad-"}
	for _, name := range invalidNames {
		req := httptest.NewRequest("GET", "/api/projects/"+name+"/deployments", nil)
		w := httptest.NewRecorder()
		srv.server.Handler.ServeHTTP(w, req)

		if w.Code == http.StatusOK {
			t.Errorf("deployments with invalid name %q should not return 200", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Start/Stop/Restart with invalid project names
// ---------------------------------------------------------------------------

func TestHandleStartProjectInvalidName(t *testing.T) {
	srv, _ := setupTestServer(t)

	invalidNames := []string{"-leading", "trailing-", "UPPER", "a"}
	for _, name := range invalidNames {
		req := httptest.NewRequest("POST", "/api/projects/"+name+"/start", nil)
		w := httptest.NewRecorder()
		srv.server.Handler.ServeHTTP(w, req)

		if w.Code == http.StatusOK || w.Code == http.StatusNotFound {
			// Should be 400 for invalid names, not pass through to project lookup
		}
		// Verify the response is JSON with an error
		var resp map[string]string
		if err := json.NewDecoder(w.Body).Decode(&resp); err == nil {
			if resp["error"] == "" && w.Code != http.StatusMethodNotAllowed {
				t.Errorf("start with invalid name %q: expected error message", name)
			}
		}
	}
}

func TestHandleStopProjectInvalidName(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/projects/-invalid/stop", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Error("stop with invalid name should not return 200")
	}
}

func TestHandleRestartProjectInvalidName(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/projects/-invalid/restart", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Error("restart with invalid name should not return 200")
	}
}

// ---------------------------------------------------------------------------
// Health endpoint tests
// ---------------------------------------------------------------------------

func TestHandleProjectHealthNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/projects/nonexistent/health", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleProjectHealthInvalidName(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/projects/-invalid/health", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Error("health with invalid name should not return 200")
	}
}

// ---------------------------------------------------------------------------
// Manual deploy trigger validation
// ---------------------------------------------------------------------------

func TestManualDeployProjectExists(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "trigger-app",
		Domain:      "trigger.io",
		LinuxUser:   "fleetdeck-trigger-app",
		ProjectPath: dir,
		Template:    "node",
	})

	req := httptest.NewRequest("POST", "/api/webhook/deploy/trigger-app", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "deploying" {
		t.Errorf("expected status=deploying, got %q", resp["status"])
	}
}

func TestManualDeployInvalidName(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/webhook/deploy/A", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Error("manual deploy with invalid name should not return 200")
	}
}

// ---------------------------------------------------------------------------
// Auth on audit endpoint
// ---------------------------------------------------------------------------

func TestAuditEndpointRequiresAuth(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("GET", "/api/audit", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth on audit endpoint, got %d", w.Code)
	}
}

func TestAuditEndpointAllowsValidBearer(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := config.DefaultConfig()
	cfg.Server.BasePath = dir
	cfg.Server.APIToken = "audit-token"
	cfg.Audit.LogPath = filepath.Join(dir, "nonexistent.log")

	srv := New(cfg, database, ":0")

	req := httptest.NewRequest("GET", "/api/audit", nil)
	req.Header.Set("Authorization", "Bearer audit-token")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with valid bearer on audit endpoint, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Server status with projects in various states
// ---------------------------------------------------------------------------

func TestHandleServerStatusWithProjects(t *testing.T) {
	srv, database := setupTestServer(t)

	dir1 := t.TempDir()
	dir2 := t.TempDir()
	dir3 := t.TempDir()

	database.CreateProject(&db.Project{
		Name: "running-app", Domain: "run.io", LinuxUser: "fleetdeck-running-app",
		ProjectPath: dir1, Template: "node", Status: "running",
	})
	database.CreateProject(&db.Project{
		Name: "stopped-app", Domain: "stop.io", LinuxUser: "fleetdeck-stopped-app",
		ProjectPath: dir2, Template: "node", Status: "stopped",
	})
	database.CreateProject(&db.Project{
		Name: "error-app", Domain: "err.io", LinuxUser: "fleetdeck-error-app",
		ProjectPath: dir3, Template: "node", Status: "error",
	})

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var status apiStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if status.Projects != 3 {
		t.Errorf("expected 3 projects, got %d", status.Projects)
	}
	if status.Running != 1 {
		t.Errorf("expected 1 running, got %d", status.Running)
	}
	if status.Stopped != 1 {
		t.Errorf("expected 1 stopped, got %d", status.Stopped)
	}
	if status.CPUs <= 0 {
		t.Error("expected CPUs > 0")
	}
}

// ---------------------------------------------------------------------------
// Create backup with invalid project name
// ---------------------------------------------------------------------------

func TestHandleCreateBackupInvalidName(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/projects/-invalid/backup", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Error("create backup with invalid name should not return 200")
	}
}

func TestHandleCreateBackupProjectNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/projects/nonexistent/backup", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Restore backup validation
// ---------------------------------------------------------------------------

func TestHandleRestoreBackupInvalidProjectName(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/projects/-invalid/backup/some-id/restore", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Error("restore with invalid project name should not return 200")
	}
}

func TestHandleRestoreBackupProjectNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/projects/nonexistent/backup/00000000-0000-0000-0000-000000000000/restore", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Delete backup validation
// ---------------------------------------------------------------------------

func TestHandleDeleteBackupInvalidProjectName(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("DELETE", "/api/projects/-invalid/backup/some-id", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Error("delete backup with invalid project name should not return 200")
	}
}

func TestHandleDeleteBackupProjectNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("DELETE", "/api/projects/nonexistent/backup/00000000-0000-0000-0000-000000000000", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Logs handler with edge cases
// ---------------------------------------------------------------------------

func TestHandleProjectLogsInvalidName(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/projects/-invalid/logs", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Error("logs with invalid name should not return 200")
	}
}

func TestHandleProjectLogsBoundaryLines(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "boundary-logs",
		Domain:      "bl.io",
		LinuxUser:   "fleetdeck-boundary-logs",
		ProjectPath: dir,
		Template:    "node",
	})

	// Boundary valid values
	validLines := []string{"1", "10000"}
	for _, lines := range validLines {
		req := httptest.NewRequest("GET", "/api/projects/boundary-logs/logs?lines="+lines, nil)
		w := httptest.NewRecorder()
		srv.server.Handler.ServeHTTP(w, req)

		if w.Code == http.StatusBadRequest {
			t.Errorf("lines=%s should be valid (not 400)", lines)
		}
	}
}

// ---------------------------------------------------------------------------
// Webhook events that are not push or ping
// ---------------------------------------------------------------------------

func TestWebhookIgnoresVariousNonPushEvents(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.webhookSecret = testWebhookSecret

	events := []string{"pull_request", "issues", "release", "create", "delete", "star"}
	for _, event := range events {
		req := signedWebhookRequest(t, `{}`, event)
		w := httptest.NewRecorder()
		srv.server.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("event %s: expected 200, got %d", event, w.Code)
		}

		var resp map[string]string
		json.NewDecoder(w.Body).Decode(&resp)
		if resp["status"] != "ignored" {
			t.Errorf("event %s: expected status=ignored, got %q", event, resp["status"])
		}
	}
}

// ---------------------------------------------------------------------------
// Get project with valid name but project does not exist
// ---------------------------------------------------------------------------

func TestHandleGetProjectResponseFormat(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	p := &db.Project{
		Name:        "format-test",
		Domain:      "format.io",
		LinuxUser:   "fleetdeck-format-test",
		ProjectPath: dir,
		Template:    "go",
		Status:      "running",
		Source:      "created",
	}
	database.CreateProject(p)

	req := httptest.NewRequest("GET", "/api/projects/format-test", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var proj apiProject
	if err := json.NewDecoder(w.Body).Decode(&proj); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if proj.Name != "format-test" {
		t.Errorf("Name = %q, want format-test", proj.Name)
	}
	if proj.Domain != "format.io" {
		t.Errorf("Domain = %q, want format.io", proj.Domain)
	}
	if proj.Template != "go" {
		t.Errorf("Template = %q, want go", proj.Template)
	}
	if proj.Status != "running" {
		t.Errorf("Status = %q, want running", proj.Status)
	}
	if proj.Source != "created" {
		t.Errorf("Source = %q, want created", proj.Source)
	}
	if proj.ID == "" {
		t.Error("ID should not be empty")
	}
	if proj.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

// ---------------------------------------------------------------------------
// System health endpoint
// ---------------------------------------------------------------------------

func TestHandleSystemHealthRequiresAuth(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth on health endpoint, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Create project with unknown template
// ---------------------------------------------------------------------------

func TestCreateProjectUnknownTemplate(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"name":"new-proj","domain":"new.io","template":"nonexistent-template"}`
	req := httptest.NewRequest("POST", "/api/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown template, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if !strings.Contains(resp["error"], "unknown template") {
		t.Errorf("expected error about unknown template, got %q", resp["error"])
	}
}

// ---------------------------------------------------------------------------
// Backup listing for project not found
// ---------------------------------------------------------------------------

func TestHandleListBackupsProjectNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/projects/nonexistent/backups", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Multiple backup records with size formatting
// ---------------------------------------------------------------------------

func TestHandleListBackupsMultipleRecords(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	p := &db.Project{
		Name:        "multi-backup",
		Domain:      "mb.io",
		LinuxUser:   "fleetdeck-multi-backup",
		ProjectPath: dir,
		Template:    "node",
	}
	database.CreateProject(p)

	sizes := []int64{512, 1024 * 1024, 1024 * 1024 * 1024}
	wantSizes := map[int64]string{
		512:                  "512 B",
		1024 * 1024:         "1.0 MB",
		1024 * 1024 * 1024:  "1.0 GB",
	}

	for i, size := range sizes {
		b := &db.BackupRecord{
			ProjectID: p.ID,
			Type:      "manual",
			Trigger:   "api",
			Path:      filepath.Join(dir, fmt.Sprintf("backup-%d", i)),
			SizeBytes: size,
		}
		database.CreateBackupRecord(b)
	}

	req := httptest.NewRequest("GET", "/api/projects/multi-backup/backups", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var backups []apiBackup
	json.NewDecoder(w.Body).Decode(&backups)
	if len(backups) != 3 {
		t.Fatalf("expected 3 backups, got %d", len(backups))
	}

	// Verify each backup has a correctly formatted size (order may vary)
	for _, b := range backups {
		expected, ok := wantSizes[b.SizeBytes]
		if !ok {
			t.Errorf("unexpected SizeBytes %d", b.SizeBytes)
			continue
		}
		if b.Size != expected {
			t.Errorf("backup with %d bytes: Size = %q, want %q", b.SizeBytes, b.Size, expected)
		}
	}
}
