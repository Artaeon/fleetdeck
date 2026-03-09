package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/db"
)

func setupTestServer(t *testing.T) (*Server, *db.DB) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := config.DefaultConfig()
	cfg.Server.BasePath = dir

	srv := New(cfg, database, ":0")
	return srv, database
}

func setupAuthTestServer(t *testing.T) (*Server, *db.DB) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := config.DefaultConfig()
	cfg.Server.BasePath = dir
	cfg.Server.APIToken = "test-secret-token"

	srv := New(cfg, database, ":0")
	return srv, database
}

func TestHandleListProjectsEmpty(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/projects", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var projects []apiProject
	if err := json.NewDecoder(w.Body).Decode(&projects); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(projects))
	}
}

func TestHandleListProjectsWithData(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	p := &db.Project{
		Name:        "testapp",
		Domain:      "test.com",
		LinuxUser:   "fleetdeck-testapp",
		ProjectPath: dir,
		Template:    "node",
		Status:      "running",
		Source:      "created",
	}
	if err := database.CreateProject(p); err != nil {
		t.Fatalf("create project: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/projects", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var projects []apiProject
	if err := json.NewDecoder(w.Body).Decode(&projects); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].Name != "testapp" {
		t.Errorf("expected testapp, got %s", projects[0].Name)
	}
	if projects[0].Domain != "test.com" {
		t.Errorf("expected test.com, got %s", projects[0].Domain)
	}
}

func TestHandleGetProject(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	p := &db.Project{
		Name:        "myapp",
		Domain:      "myapp.io",
		LinuxUser:   "fleetdeck-myapp",
		ProjectPath: dir,
		Template:    "go",
		Status:      "running",
	}
	database.CreateProject(p)

	req := httptest.NewRequest("GET", "/api/projects/myapp", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var project apiProject
	json.NewDecoder(w.Body).Decode(&project)
	if project.Name != "myapp" {
		t.Errorf("expected myapp, got %s", project.Name)
	}
}

func TestHandleGetProjectNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/projects/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleServerStatus(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var status apiStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if status.CPUs <= 0 {
		t.Error("expected CPUs > 0")
	}
}

func TestHandleDashboard(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("expected text/html, got %s", ct)
	}
	body := w.Body.String()
	if len(body) < 100 {
		t.Error("expected HTML content")
	}
}

func TestHandleCSS(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/static/style.css", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/css" {
		t.Errorf("expected text/css, got %s", ct)
	}
}

func TestHandleJS(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/static/app.js", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/javascript" {
		t.Errorf("expected application/javascript, got %s", ct)
	}
}

func TestHandleListBackups(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	p := &db.Project{
		Name:        "backupapp",
		Domain:      "backup.io",
		LinuxUser:   "fleetdeck-backupapp",
		ProjectPath: dir,
		Template:    "node",
	}
	database.CreateProject(p)

	// Create a backup record
	backupDir := filepath.Join(dir, "backups")
	os.MkdirAll(backupDir, 0755)
	b := &db.BackupRecord{
		ProjectID: p.ID,
		Type:      "manual",
		Trigger:   "user",
		Path:      backupDir,
		SizeBytes: 1024,
	}
	database.CreateBackupRecord(b)

	req := httptest.NewRequest("GET", "/api/projects/backupapp/backups", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var backups []apiBackup
	json.NewDecoder(w.Body).Decode(&backups)
	if len(backups) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(backups))
	}
	if backups[0].Size != "1.0 KB" {
		t.Errorf("expected 1.0 KB, got %s", backups[0].Size)
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		got := formatSize(tt.bytes)
		if got != tt.expected {
			t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, got, tt.expected)
		}
	}
}

// --- Authentication Tests ---

func TestAPIRequiresAuthWhenConfigured(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	// No auth header — should get 401
	req := httptest.NewRequest("GET", "/api/projects", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", w.Code)
	}
}

func TestAPIAllowsValidBearerToken(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("GET", "/api/projects", nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with valid token, got %d", w.Code)
	}
}

func TestAPIRejectsInvalidBearerToken(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("GET", "/api/projects", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong token, got %d", w.Code)
	}
}

func TestAPIAllowsValidCookie(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("GET", "/api/projects", nil)
	req.AddCookie(&http.Cookie{Name: "fleetdeck_session", Value: "test-secret-token"})
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with valid cookie, got %d", w.Code)
	}
}

func TestDashboardRedirectsToLoginWhenAuthConfigured(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %s", loc)
	}
}

func TestLoginPageAccessible(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestLoginWithValidToken(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("POST", "/login", strings.NewReader("token=test-secret-token"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect after login, got %d", w.Code)
	}

	// Should set cookie
	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "fleetdeck_session" && c.Value == "test-secret-token" {
			found = true
			if !c.HttpOnly {
				t.Error("session cookie should be HttpOnly")
			}
		}
	}
	if !found {
		t.Error("expected session cookie to be set")
	}
}

func TestLoginWithInvalidToken(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("POST", "/login", strings.NewReader("token=wrong"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong token, got %d", w.Code)
	}
}

func TestNoAuthAllowsAccessByDefault(t *testing.T) {
	// Default test server has no API token set
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/projects", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 without auth when no token configured, got %d", w.Code)
	}
}

// --- Input Validation Tests ---

func TestLogsEndpointRejectsInvalidLines(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "testlogs",
		Domain:      "test.io",
		LinuxUser:   "fleetdeck-testlogs",
		ProjectPath: dir,
		Template:    "node",
	})

	tests := []struct {
		lines    string
		wantCode int
	}{
		{"abc", http.StatusBadRequest},
		{"-1", http.StatusBadRequest},
		{"0", http.StatusBadRequest},
		{"99999", http.StatusBadRequest},
	}

	for _, tt := range tests {
		req := httptest.NewRequest("GET", "/api/projects/testlogs/logs?lines="+tt.lines, nil)
		w := httptest.NewRecorder()
		srv.server.Handler.ServeHTTP(w, req)

		if w.Code != tt.wantCode {
			t.Errorf("lines=%q: expected %d, got %d", tt.lines, tt.wantCode, w.Code)
		}
	}
}

func TestGenerateAPIToken(t *testing.T) {
	t1 := GenerateAPIToken()
	t2 := GenerateAPIToken()

	if len(t1) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("expected 64 char hex token, got %d chars", len(t1))
	}
	if t1 == t2 {
		t.Error("two generated tokens should not be identical")
	}
}
