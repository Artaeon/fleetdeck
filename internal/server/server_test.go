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
	// t.Cleanup runs LIFO, so this close runs LAST — after the drain
	// cleanup below has waited for any webhook-triggered async
	// deployments to finish writing. Without the drain, those writes
	// would race with DB.Close and surface as 'sql: database is
	// closed' in the test log (and, in production, as a broken
	// deployment record).
	t.Cleanup(func() { database.Close() })

	cfg := config.DefaultConfig()
	cfg.Server.BasePath = dir

	srv := New(cfg, database, ":0")
	t.Cleanup(func() {
		srv.metrics.Stop()
		srv.asyncJobs.Wait()
	})
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
	// Drain async jobs before the DB.Close cleanup above runs (LIFO):
	// stop the metrics ticker and wait for any webhook/manual-deploy
	// goroutines to finish writing.
	t.Cleanup(func() {
		srv.metrics.Stop()
		srv.asyncJobs.Wait()
	})
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
	t1, err := GenerateAPIToken()
	if err != nil {
		t.Fatalf("GenerateAPIToken: %v", err)
	}
	t2, err := GenerateAPIToken()
	if err != nil {
		t.Fatalf("GenerateAPIToken: %v", err)
	}

	if len(t1) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("expected 64 char hex token, got %d chars", len(t1))
	}
	if t1 == t2 {
		t.Error("two generated tokens should not be identical")
	}
}

// --- Security Header Tests ---

func TestSecurityHeadersPresent(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Test security headers on multiple endpoint types
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/projects"},
		{"GET", "/api/status"},
		{"GET", "/"},
		{"GET", "/static/style.css"},
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest(ep.method, ep.path, nil)
		w := httptest.NewRecorder()
		srv.server.Handler.ServeHTTP(w, req)

		headers := map[string]string{
			"X-Frame-Options":        "DENY",
			"X-Content-Type-Options":  "nosniff",
			"Referrer-Policy":         "strict-origin-when-cross-origin",
			"Content-Security-Policy": "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'",
		}

		for name, expected := range headers {
			got := w.Header().Get(name)
			if got != expected {
				t.Errorf("%s %s: header %s = %q, want %q", ep.method, ep.path, name, got, expected)
			}
		}
	}
}

func TestSecurityHeadersOnAuthEndpoints(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	// Even auth-rejected requests should have security headers
	req := httptest.NewRequest("GET", "/api/projects", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	if got := w.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options on 401 response = %q, want DENY", got)
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options on 401 response = %q, want nosniff", got)
	}
}

func TestPermissionsPolicyHeader(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/projects", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	pp := w.Header().Get("Permissions-Policy")
	if pp == "" {
		t.Error("expected Permissions-Policy header to be set")
	}
	if !strings.Contains(pp, "geolocation=()") {
		t.Errorf("Permissions-Policy should restrict geolocation, got %q", pp)
	}
}

// --- Cookie Security Tests ---

func TestLoginCookieSecurityFlags(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("POST", "/login", strings.NewReader("token=test-secret-token"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d", w.Code)
	}

	cookies := w.Result().Cookies()
	var session *http.Cookie
	for _, c := range cookies {
		if c.Name == "fleetdeck_session" {
			session = c
			break
		}
	}
	if session == nil {
		t.Fatal("expected fleetdeck_session cookie to be set")
	}

	if !session.HttpOnly {
		t.Error("cookie should have HttpOnly flag set")
	}
	if session.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie SameSite = %v, want SameSiteStrictMode", session.SameSite)
	}
	if !session.Secure {
		t.Error("cookie should have Secure flag set")
	}
	if session.Path != "/" {
		t.Errorf("cookie Path = %q, want /", session.Path)
	}
	if session.MaxAge != 86400*7 {
		t.Errorf("cookie MaxAge = %d, want %d (7 days)", session.MaxAge, 86400*7)
	}
}

// --- Request Size Limit Tests ---

func TestLoginRejectsOversizedBody(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	// Create a body larger than the 64KB limit (1<<16 = 65536)
	bigBody := strings.Repeat("token=x&padding=", 5000) // ~80KB
	req := httptest.NewRequest("POST", "/login", strings.NewReader(bigBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	// MaxBytesReader causes the form parse to fail, so the token won't
	// match and we get 401 (not a 200 redirect). The important thing is
	// the oversized body does NOT succeed in logging in.
	if w.Code == http.StatusFound {
		t.Error("oversized body should not result in successful login redirect")
	}
}

// --- Error Message Sanitization Tests ---

func TestErrorMessageSanitization(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Request a non-existent project — the error message should NOT leak
	// internal details like file paths, SQL queries, or stack traces.
	req := httptest.NewRequest("GET", "/api/projects/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	errMsg := resp["error"]
	if errMsg == "" {
		t.Fatal("expected error message in response")
	}

	// Error should be a clean user-facing message, not a raw internal error
	if strings.Contains(errMsg, "/") && strings.Contains(errMsg, ".go") {
		t.Errorf("error message should not contain file paths: %q", errMsg)
	}
	if strings.Contains(errMsg, "sql") || strings.Contains(errMsg, "SQL") {
		t.Errorf("error message should not contain SQL details: %q", errMsg)
	}
	if strings.Contains(errMsg, "panic") || strings.Contains(errMsg, "goroutine") {
		t.Errorf("error message should not contain stack traces: %q", errMsg)
	}
}

func TestInternalErrorSanitization(t *testing.T) {
	srv, _ := setupTestServer(t)

	// handleGetProject now returns "project not found" rather than raw DB error
	req := httptest.NewRequest("GET", "/api/projects/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)

	errMsg := resp["error"]
	// Should be a generic message, not raw db error like 'project "nonexistent" not found'
	if strings.Contains(errMsg, `"nonexistent"`) {
		t.Errorf("error message should not echo back user input verbatim with quotes: %q", errMsg)
	}
}

// --- Project Name Validation Tests ---

func TestProjectNameValidation(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Names that are safe to put in a URL path directly
	invalidNames := []struct {
		name string
		desc string
	}{
		{"MY-APP", "uppercase letters"},
		{"-leadinghyphen", "leading hyphen"},
		{"trailinghyphen-", "trailing hyphen"},
		{strings.Repeat("a", 200), "excessively long name"},
		{"a", "single character (must start and end with alnum, with middle)"},
		{"my;app", "semicolons"},
	}

	for _, tt := range invalidNames {
		req := httptest.NewRequest("GET", "/api/projects/"+tt.name, nil)
		w := httptest.NewRecorder()
		srv.server.Handler.ServeHTTP(w, req)

		// Invalid names should get 400 (bad request) from validation,
		// or 404/405 from the Go router itself for certain patterns
		if w.Code == http.StatusOK {
			t.Errorf("project name %q (%s) should be rejected, got 200", tt.name, tt.desc)
		}
	}
}

func TestProjectNameValidRegex(t *testing.T) {
	// Test the validProjectName regex directly
	valid := []string{
		"myapp",
		"my-app",
		"app123",
		"a1",
		"test-project-01",
	}
	for _, name := range valid {
		if !validProjectName.MatchString(name) {
			t.Errorf("expected %q to be a valid project name", name)
		}
	}

	invalid := []string{
		"",
		"-start",
		"end-",
		"UPPER",
		"has space",
		"has.dot",
		"has_underscore",
		"a", // too short (regex requires start + middle + end)
	}
	for _, name := range invalid {
		if validProjectName.MatchString(name) {
			t.Errorf("expected %q to be an invalid project name", name)
		}
	}
}

// --- Deployments Endpoint Tests ---

func TestDeploymentsEndpointReturnsJSONArray(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	p := &db.Project{
		Name:        "deploy-test",
		Domain:      "deploy.io",
		LinuxUser:   "fleetdeck-deploy-test",
		ProjectPath: dir,
		Template:    "node",
	}
	database.CreateProject(p)

	req := httptest.NewRequest("GET", "/api/projects/deploy-test/deployments", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}

	// Should decode as a JSON array (empty for a new project)
	var deployments []json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&deployments); err != nil {
		t.Fatalf("response should be a valid JSON array: %v", err)
	}
	if len(deployments) != 0 {
		t.Errorf("expected 0 deployments for new project, got %d", len(deployments))
	}
}

func TestDeploymentsEndpointWithData(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	p := &db.Project{
		Name:        "deploy-data",
		Domain:      "deploy-data.io",
		LinuxUser:   "fleetdeck-deploy-data",
		ProjectPath: dir,
		Template:    "node",
	}
	database.CreateProject(p)

	// Create a deployment record
	dep := &db.Deployment{
		ProjectID: p.ID,
		CommitSHA: "abc123",
		Status:    "success",
	}
	database.CreateDeployment(dep)

	req := httptest.NewRequest("GET", "/api/projects/deploy-data/deployments", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var deployments []json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&deployments); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(deployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(deployments))
	}
}

func TestDeploymentsEndpointProjectNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/projects/nonexistent/deployments", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- Manual Deploy Endpoint Tests ---

func TestManualDeployProjectNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/webhook/deploy/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestManualDeployExistingProject(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "manual-deploy",
		Domain:      "manual.io",
		LinuxUser:   "fleetdeck-manual-deploy",
		ProjectPath: dir,
		Template:    "node",
	})

	req := httptest.NewRequest("POST", "/api/webhook/deploy/manual-deploy", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	// Should accept and start async deployment (returns 200 with deploying status)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "deploying" {
		t.Errorf("expected status=deploying, got %q", resp["status"])
	}
	if resp["project"] != "manual-deploy" {
		t.Errorf("expected project=manual-deploy, got %q", resp["project"])
	}
}

// --- Content-Type Validation Tests ---

func TestAPIResponsesAreJSON(t *testing.T) {
	srv, _ := setupTestServer(t)

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/projects"},
		{"GET", "/api/status"},
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest(ep.method, ep.path, nil)
		w := httptest.NewRecorder()
		srv.server.Handler.ServeHTTP(w, req)

		ct := w.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("%s %s: Content-Type = %q, want application/json", ep.method, ep.path, ct)
		}
	}
}

func TestNotFoundResponseIsJSON(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/projects/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("404 Content-Type = %q, want application/json", ct)
	}

	// Should be valid JSON
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("404 response should be valid JSON: %v", err)
	}
	if resp["error"] == "" {
		t.Error("404 response should contain error field")
	}
}

// --- ValidProjectName Edge Cases ---

func TestValidProjectNameMiddleware(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "valid-app",
		Domain:      "valid.io",
		LinuxUser:   "fleetdeck-valid-app",
		ProjectPath: dir,
		Template:    "node",
	})

	// Valid name should succeed
	req := httptest.NewRequest("GET", "/api/projects/valid-app", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for valid project name, got %d", w.Code)
	}

	// Invalid name with special characters should be rejected
	req = httptest.NewRequest("GET", "/api/projects/invalid%00app", nil)
	w = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Error("null byte in project name should be rejected")
	}
}

// --- Auth on Write Endpoints ---

func TestAuthRequiredForWriteEndpoints(t *testing.T) {
	srv, database := setupAuthTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "authtest",
		Domain:      "auth.io",
		LinuxUser:   "fleetdeck-authtest",
		ProjectPath: dir,
		Template:    "node",
	})

	writeEndpoints := []struct {
		method string
		path   string
	}{
		{"POST", "/api/projects/authtest/start"},
		{"POST", "/api/projects/authtest/stop"},
		{"POST", "/api/projects/authtest/restart"},
		{"GET", "/api/projects/authtest/logs"},
		{"GET", "/api/projects/authtest/backups"},
	}

	for _, ep := range writeEndpoints {
		req := httptest.NewRequest(ep.method, ep.path, nil)
		w := httptest.NewRecorder()
		srv.server.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: expected 401 without auth, got %d", ep.method, ep.path, w.Code)
		}
	}
}
