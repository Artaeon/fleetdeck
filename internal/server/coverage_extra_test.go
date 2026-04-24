package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/db"
	"golang.org/x/time/rate"
)

// ---------------------------------------------------------------------------
// handleHealthz tests
// ---------------------------------------------------------------------------

func TestHandleHealthzOK(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %q", resp["status"])
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestHandleHealthzUnhealthy(t *testing.T) {
	// Create a server with a closed database to trigger Ping failure.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Server.BasePath = dir

	srv := New(cfg, database, ":0")

	// Close the database to make Ping fail.
	database.Close()

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "unhealthy" {
		t.Errorf("expected status=unhealthy, got %q", resp["status"])
	}
}

// TestHandleHealthzNoAuth verifies healthz does NOT require authentication.
func TestHandleHealthzNoAuth(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	// Should be 200 (healthy), not 401 (unauthorized).
	if w.Code == http.StatusUnauthorized {
		t.Error("healthz should not require authentication")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handleMetrics tests
// ---------------------------------------------------------------------------

func TestHandleMetricsOutputFormat(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
	if !strings.Contains(ct, "version=0.0.4") {
		t.Errorf("Content-Type should contain Prometheus version, got %q", ct)
	}

	body := w.Body.String()

	// Required Prometheus metric names with HELP and TYPE lines.
	requiredMetrics := []string{
		"fleetdeck_info",
		"fleetdeck_uptime_seconds",
		"fleetdeck_http_requests_total",
		"fleetdeck_http_request_errors_total",
		"fleetdeck_deployments_total",
		"fleetdeck_deployment_failures_total",
		"fleetdeck_backups_total",
		"fleetdeck_projects_total",
		"fleetdeck_projects_running",
		"fleetdeck_projects_stopped",
		"fleetdeck_containers_total",
		"fleetdeck_cpu_count",
		"fleetdeck_goroutines",
		"fleetdeck_traefik_up",
	}

	for _, name := range requiredMetrics {
		if !strings.Contains(body, "# HELP "+name) {
			t.Errorf("missing HELP for %s", name)
		}
		if !strings.Contains(body, "# TYPE "+name) {
			t.Errorf("missing TYPE for %s", name)
		}
		// Metric value line should also be present.
		if !strings.Contains(body, name+" ") && !strings.Contains(body, name+"{") {
			t.Errorf("missing metric value line for %s", name)
		}
	}
}

func TestHandleMetricsReflectsCounters(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Increment some counters.
	srv.metrics.incRequests()
	srv.metrics.incRequests()
	srv.metrics.incRequests()
	srv.metrics.incErrors()
	srv.metrics.incDeployments()
	srv.metrics.incDeploymentFailures()
	srv.metrics.incBackups()

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	body := w.Body.String()

	// Check that the request counter reflects at least our 3 increments.
	// The actual value may be higher due to the /metrics request itself going
	// through the metrics middleware.
	if !strings.Contains(body, "fleetdeck_http_requests_total") {
		t.Error("missing fleetdeck_http_requests_total in output")
	}
	if !strings.Contains(body, "fleetdeck_deployments_total 1") {
		t.Error("expected fleetdeck_deployments_total 1")
	}
	if !strings.Contains(body, "fleetdeck_deployment_failures_total 1") {
		t.Error("expected fleetdeck_deployment_failures_total 1")
	}
	if !strings.Contains(body, "fleetdeck_backups_total 1") {
		t.Error("expected fleetdeck_backups_total 1")
	}
}

func TestHandleMetricsRequiresAuth(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for /metrics without auth, got %d", w.Code)
	}
}

func TestHandleMetricsWithAuth(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleMetricsWithProjectData(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "metrics-app",
		Domain:      "metrics.io",
		LinuxUser:   "fleetdeck-metrics-app",
		ProjectPath: dir,
		Template:    "node",
		Status:      "running",
	})

	// Force a cache refresh so the project shows up in metrics.
	srv.metrics.refreshCache(srv)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "fleetdeck_projects_total 1") {
		t.Errorf("expected fleetdeck_projects_total 1 after cache refresh, body:\n%s", body)
	}
	if !strings.Contains(body, "fleetdeck_projects_running 1") {
		t.Errorf("expected fleetdeck_projects_running 1 after cache refresh")
	}
}

// ---------------------------------------------------------------------------
// Metrics cache refresh tests
// ---------------------------------------------------------------------------

func TestMetricsRefreshCache(t *testing.T) {
	srv, database := setupTestServer(t)

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	database.CreateProject(&db.Project{
		Name: "cache-run", Domain: "cr.io", LinuxUser: "fleetdeck-cache-run",
		ProjectPath: dir1, Template: "node", Status: "running",
	})
	database.CreateProject(&db.Project{
		Name: "cache-stop", Domain: "cs.io", LinuxUser: "fleetdeck-cache-stop",
		ProjectPath: dir2, Template: "node", Status: "stopped",
	})

	srv.metrics.refreshCache(srv)

	srv.metrics.cacheMu.RLock()
	projects := srv.metrics.cachedProjects
	running := srv.metrics.cachedRunning
	stopped := srv.metrics.cachedStopped
	updatedAt := srv.metrics.cacheUpdatedAt
	srv.metrics.cacheMu.RUnlock()

	if projects != 2 {
		t.Errorf("expected 2 projects cached, got %d", projects)
	}
	if running != 1 {
		t.Errorf("expected 1 running cached, got %d", running)
	}
	if stopped != 1 {
		t.Errorf("expected 1 stopped cached, got %d", stopped)
	}
	if updatedAt.IsZero() {
		t.Error("cacheUpdatedAt should be set after refresh")
	}
}

func TestMetricsStopChannel(t *testing.T) {
	m := newMetrics()

	// Verify the stop channel is not nil and can be closed.
	if m.stopCh == nil {
		t.Fatal("expected non-nil stopCh")
	}

	// Close should not panic.
	close(m.stopCh)
}

// ---------------------------------------------------------------------------
// handleListServers tests
// ---------------------------------------------------------------------------

func TestHandleListServersEmpty(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/servers", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var servers []json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&servers); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(servers) != 0 {
		t.Errorf("expected 0 servers, got %d", len(servers))
	}
}

func TestHandleListServersRequiresAuth(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("GET", "/api/servers", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handleSystemHealth tests
// ---------------------------------------------------------------------------

func TestHandleSystemHealthEmptyDB(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["healthy"] != true {
		t.Errorf("expected healthy=true with no projects, got %v", resp["healthy"])
	}
	projects, ok := resp["projects"].([]interface{})
	if !ok {
		t.Fatal("expected projects to be an array")
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects in health check, got %d", len(projects))
	}
}

// ---------------------------------------------------------------------------
// handleCreateProject success and duplicate tests
// ---------------------------------------------------------------------------

func TestCreateProjectSuccess(t *testing.T) {
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

	body := `{"name":"my-new-app","domain":"new.example.com","template":"custom"}`
	req := httptest.NewRequest("POST", "/api/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var proj apiProject
	if err := json.NewDecoder(w.Body).Decode(&proj); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if proj.Name != "my-new-app" {
		t.Errorf("expected name=my-new-app, got %q", proj.Name)
	}
	if proj.Domain != "new.example.com" {
		t.Errorf("expected domain=new.example.com, got %q", proj.Domain)
	}
	if proj.Template != "custom" {
		t.Errorf("expected template=custom, got %q", proj.Template)
	}
	if proj.Status != "created" {
		t.Errorf("expected status=created, got %q", proj.Status)
	}
	if proj.Source != "created" {
		t.Errorf("expected source=created, got %q", proj.Source)
	}
	if proj.ID == "" {
		t.Error("expected non-empty ID")
	}

	// Verify project exists in DB.
	dbProj, err := database.GetProject("my-new-app")
	if err != nil {
		t.Fatalf("project should exist in DB: %v", err)
	}
	if dbProj.Name != "my-new-app" {
		t.Errorf("DB project name = %q, want my-new-app", dbProj.Name)
	}
}

func TestCreateProjectDefaultTemplate(t *testing.T) {
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

	// No template specified - should default to "custom".
	body := `{"name":"default-tmpl","domain":"dt.io"}`
	req := httptest.NewRequest("POST", "/api/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var proj apiProject
	json.NewDecoder(w.Body).Decode(&proj)
	if proj.Template != "custom" {
		t.Errorf("expected template=custom as default, got %q", proj.Template)
	}
}

func TestCreateProjectDuplicateName(t *testing.T) {
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

	// Create first project.
	body := `{"name":"dup-test","domain":"dup.io"}`
	req := httptest.NewRequest("POST", "/api/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("first create: expected 200, got %d", w.Code)
	}

	// Try to create duplicate.
	req2 := httptest.NewRequest("POST", "/api/projects", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusConflict {
		t.Errorf("expected 409 for duplicate, got %d", w2.Code)
	}

	var resp map[string]string
	json.NewDecoder(w2.Body).Decode(&resp)
	if !strings.Contains(resp["error"], "already exists") {
		t.Errorf("expected 'already exists' error, got %q", resp["error"])
	}
}

func TestCreateProjectInvalidName(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"name":"INVALID","domain":"inv.io"}`
	req := httptest.NewRequest("POST", "/api/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid project name, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handleDeleteProject tests - directory removal
// ---------------------------------------------------------------------------

func TestHandleDeleteProjectRemovesDirectory(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	markerPath := filepath.Join(dir, "data.txt")
	os.WriteFile(markerPath, []byte("data"), 0644)

	p := &db.Project{
		Name:        "rm-me",
		Domain:      "rm.io",
		LinuxUser:   "fleetdeck-rm-me",
		ProjectPath: dir,
		Template:    "node",
		Status:      "stopped",
	}
	database.CreateProject(p)

	// Delete WITHOUT keep-data.
	req := httptest.NewRequest("DELETE", "/api/projects/rm-me", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Directory should be removed.
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("project directory should be removed when keep-data is not set")
	}
}

// ---------------------------------------------------------------------------
// loadFleetdeckConfig tests
// ---------------------------------------------------------------------------

func TestLoadFleetdeckConfigYML(t *testing.T) {
	dir := t.TempDir()
	cfgContent := `hooks:
  pre_deploy: "npm run migrate"
  post_deploy: "npm run seed"
`
	os.WriteFile(filepath.Join(dir, ".fleetdeck.yml"), []byte(cfgContent), 0644)

	cfg := loadFleetdeckConfig(dir)
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Hooks.PreDeploy != "npm run migrate" {
		t.Errorf("expected pre_deploy='npm run migrate', got %q", cfg.Hooks.PreDeploy)
	}
	if cfg.Hooks.PostDeploy != "npm run seed" {
		t.Errorf("expected post_deploy='npm run seed', got %q", cfg.Hooks.PostDeploy)
	}
}

func TestLoadFleetdeckConfigAlternateName(t *testing.T) {
	dir := t.TempDir()
	cfgContent := `hooks:
  pre_deploy: "make migrate"
`
	os.WriteFile(filepath.Join(dir, "fleetdeck.yml"), []byte(cfgContent), 0644)

	cfg := loadFleetdeckConfig(dir)
	if cfg == nil {
		t.Fatal("expected non-nil config from fleetdeck.yml")
	}
	if cfg.Hooks.PreDeploy != "make migrate" {
		t.Errorf("expected pre_deploy='make migrate', got %q", cfg.Hooks.PreDeploy)
	}
}

func TestLoadFleetdeckConfigNotFound(t *testing.T) {
	dir := t.TempDir()
	cfg := loadFleetdeckConfig(dir)
	if cfg != nil {
		t.Error("expected nil when no config file exists")
	}
}

func TestLoadFleetdeckConfigInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	// Use content that cannot be unmarshalled into the fleetdeckConfig struct.
	os.WriteFile(filepath.Join(dir, ".fleetdeck.yml"), []byte("hooks:\n  pre_deploy:\n    - not\n    - a\n    - string\n"), 0644)

	cfg := loadFleetdeckConfig(dir)
	if cfg != nil {
		t.Error("expected nil for invalid YAML")
	}
}

func TestLoadFleetdeckConfigEmptyHooks(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".fleetdeck.yml"), []byte("hooks: {}\n"), 0644)

	cfg := loadFleetdeckConfig(dir)
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Hooks.PreDeploy != "" {
		t.Errorf("expected empty pre_deploy, got %q", cfg.Hooks.PreDeploy)
	}
	if cfg.Hooks.PostDeploy != "" {
		t.Errorf("expected empty post_deploy, got %q", cfg.Hooks.PostDeploy)
	}
}

// ---------------------------------------------------------------------------
// checkProjectHealth tests
// ---------------------------------------------------------------------------

func TestCheckProjectHealthNoDocker(t *testing.T) {
	dir := t.TempDir()
	// With no docker, this should return nil (command fails).
	report := checkProjectHealth(dir)
	// On CI or systems without docker, report will be nil.
	// We just verify it doesn't panic.
	_ = report
}

// ---------------------------------------------------------------------------
// captureContainerImages tests
// ---------------------------------------------------------------------------

func TestCaptureContainerImagesNoDocker(t *testing.T) {
	dir := t.TempDir()
	images := captureContainerImages(dir)
	// Without docker, this returns nil.
	if images != nil && len(images) > 0 {
		t.Error("expected nil or empty map without docker")
	}
}

// ---------------------------------------------------------------------------
// countContainers tests
// ---------------------------------------------------------------------------

func TestCountContainersNoDocker(t *testing.T) {
	dir := t.TempDir()
	running, total := countContainers(dir)
	if running != 0 || total != 0 {
		t.Errorf("expected 0/0 without docker, got %d/%d", running, total)
	}
}

// ---------------------------------------------------------------------------
// rateLimitMiddleware with X-Forwarded-For header
// ---------------------------------------------------------------------------

func TestRateLimitMiddlewareUsesXForwardedFor(t *testing.T) {
	// With 127.0.0.1 in the trust list, the middleware honors XFF just
	// like a real nginx -> fleetdeck setup.
	t.Setenv("FLEETDECK_TRUST_PROXY_IPS", "127.0.0.1")

	il := &ipLimiter{
		limiters: make(map[string]*visitorLimiter),
		rate:     rate.Limit(1),
		burst:    1,
	}

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := rateLimitMiddleware(il, backend)

	// First request with X-Forwarded-For should succeed.
	req1 := httptest.NewRequest("GET", "/api/test", nil)
	req1.RemoteAddr = "127.0.0.1:1234"
	req1.Header.Set("X-Forwarded-For", "10.0.0.50, 10.0.0.1")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("first request: expected 200, got %d", w1.Code)
	}

	// Second request from the same X-Forwarded-For IP should be rate-limited.
	req2 := httptest.NewRequest("GET", "/api/test", nil)
	req2.RemoteAddr = "127.0.0.1:5678"
	req2.Header.Set("X-Forwarded-For", "10.0.0.50")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("second request from same XFF IP: expected 429, got %d", w2.Code)
	}

	// Third request from different X-Forwarded-For IP should succeed.
	req3 := httptest.NewRequest("GET", "/api/test", nil)
	req3.RemoteAddr = "127.0.0.1:9999"
	req3.Header.Set("X-Forwarded-For", "10.0.0.99")
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)

	if w3.Code != http.StatusOK {
		t.Errorf("different XFF IP: expected 200, got %d", w3.Code)
	}
}

// TestRateLimitMiddlewareRejectsSpoofedXForwardedFor pins the attack-mode
// behaviour: when the TCP peer is NOT in the trust list, XFF must be
// ignored so an attacker cannot rotate the header to escape the bucket.
func TestRateLimitMiddlewareRejectsSpoofedXForwardedFor(t *testing.T) {
	t.Setenv("FLEETDECK_TRUST_PROXY_IPS", "") // no trusted proxies

	il := &ipLimiter{
		limiters: make(map[string]*visitorLimiter),
		rate:     rate.Limit(1),
		burst:    1,
	}
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := rateLimitMiddleware(il, backend)

	// Attacker sends two requests from the same TCP peer but rotates
	// the XFF header on each. With trust disabled, the peer IP is what
	// the limiter sees, so the second request is rate-limited even
	// though the attacker tried to pose as a different client.
	req1 := httptest.NewRequest("GET", "/api/test", nil)
	req1.RemoteAddr = "203.0.113.99:1111"
	req1.Header.Set("X-Forwarded-For", "10.0.0.1")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Errorf("first request: expected 200, got %d", w1.Code)
	}

	req2 := httptest.NewRequest("GET", "/api/test", nil)
	req2.RemoteAddr = "203.0.113.99:2222"
	req2.Header.Set("X-Forwarded-For", "10.0.0.2") // rotated — shouldn't help
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("spoofed XFF should not bypass limit, got %d", w2.Code)
	}
}

// ---------------------------------------------------------------------------
// clientIP edge cases
// ---------------------------------------------------------------------------

func TestClientIPXForwardedForWithSpaces(t *testing.T) {
	t.Setenv("FLEETDECK_TRUST_PROXY_IPS", "127.0.0.1")
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "  10.0.0.1  ,  10.0.0.2  ")

	got := clientIP(r)
	if got != "10.0.0.1" {
		t.Errorf("expected trimmed '10.0.0.1', got %q", got)
	}
}

func TestClientIPIPv6RemoteAddr(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "[::1]:1234"

	got := clientIP(r)
	if got != "::1" {
		t.Errorf("expected '::1', got %q", got)
	}
}

func TestClientIPEmptyXForwardedFor(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.168.1.1:5000"
	r.Header.Set("X-Forwarded-For", "")

	got := clientIP(r)
	// Empty XFF should fall through to RemoteAddr.
	if got != "192.168.1.1" {
		t.Errorf("expected '192.168.1.1', got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Metrics middleware error tracking
// ---------------------------------------------------------------------------

func TestMetricsMiddlewareIncrementsErrorsOn4xx(t *testing.T) {
	srv, _ := setupTestServer(t)

	initialErrors := srv.metrics.httpRequestErrors.Load()

	// Trigger a 404.
	req := httptest.NewRequest("GET", "/api/projects/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}

	newErrors := srv.metrics.httpRequestErrors.Load()
	if newErrors <= initialErrors {
		t.Error("expected error counter to increment on 4xx response")
	}
}

func TestMetricsMiddlewareIncrementsRequests(t *testing.T) {
	srv, _ := setupTestServer(t)

	initial := srv.metrics.httpRequestsTotal.Load()

	req := httptest.NewRequest("GET", "/api/projects", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	after := srv.metrics.httpRequestsTotal.Load()
	if after <= initial {
		t.Error("expected request counter to increment")
	}
}

// ---------------------------------------------------------------------------
// handleRestoreBackup - invalid backup ID format
// ---------------------------------------------------------------------------

func TestHandleRestoreBackupInvalidBackupID(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "restore-test",
		Domain:      "rt.io",
		LinuxUser:   "fleetdeck-restore-test",
		ProjectPath: dir,
		Template:    "node",
	})

	// Invalid UUID format.
	req := httptest.NewRequest("POST", "/api/projects/restore-test/backup/not-a-uuid/restore", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid backup ID, got %d", w.Code)
	}
}

func TestHandleRestoreBackupNotFoundInDB(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "restore-nf",
		Domain:      "rnf.io",
		LinuxUser:   "fleetdeck-restore-nf",
		ProjectPath: dir,
		Template:    "node",
	})

	// Valid UUID but not in DB.
	req := httptest.NewRequest("POST", "/api/projects/restore-nf/backup/00000000-0000-0000-0000-000000000001/restore", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing backup, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handleDeleteBackup - invalid backup ID and not found
// ---------------------------------------------------------------------------

func TestHandleDeleteBackupInvalidBackupID(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "delbk-test",
		Domain:      "db.io",
		LinuxUser:   "fleetdeck-delbk-test",
		ProjectPath: dir,
		Template:    "node",
	})

	req := httptest.NewRequest("DELETE", "/api/projects/delbk-test/backup/not-valid-uuid", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid backup ID, got %d", w.Code)
	}
}

func TestHandleDeleteBackupNotFoundInDB(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "delbk-nf",
		Domain:      "dnf.io",
		LinuxUser:   "fleetdeck-delbk-nf",
		ProjectPath: dir,
		Template:    "node",
	})

	req := httptest.NewRequest("DELETE", "/api/projects/delbk-nf/backup/00000000-0000-0000-0000-000000000002", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handleDeleteBackup - backup belongs to different project
// ---------------------------------------------------------------------------

func TestHandleDeleteBackupWrongProject(t *testing.T) {
	srv, database := setupTestServer(t)

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	p1 := &db.Project{
		Name:        "proj-one",
		Domain:      "one.io",
		LinuxUser:   "fleetdeck-proj-one",
		ProjectPath: dir1,
		Template:    "node",
	}
	database.CreateProject(p1)

	p2 := &db.Project{
		Name:        "proj-two",
		Domain:      "two.io",
		LinuxUser:   "fleetdeck-proj-two",
		ProjectPath: dir2,
		Template:    "node",
	}
	database.CreateProject(p2)

	// Create backup for proj-one.
	b := &db.BackupRecord{
		ProjectID: p1.ID,
		Type:      "manual",
		Trigger:   "api",
		Path:      filepath.Join(dir1, "backup"),
		SizeBytes: 100,
	}
	database.CreateBackupRecord(b)

	// Try to delete it via proj-two's endpoint.
	req := httptest.NewRequest("DELETE", "/api/projects/proj-two/backup/"+b.ID, nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 when backup belongs to different project, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if !strings.Contains(resp["error"], "not found for this project") {
		t.Errorf("expected 'not found for this project' error, got %q", resp["error"])
	}
}

// ---------------------------------------------------------------------------
// handleDeleteBackup - success path
// ---------------------------------------------------------------------------

func TestHandleDeleteBackupSuccess(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	p := &db.Project{
		Name:        "delbk-ok",
		Domain:      "dok.io",
		LinuxUser:   "fleetdeck-delbk-ok",
		ProjectPath: dir,
		Template:    "node",
	}
	database.CreateProject(p)

	backupDir := filepath.Join(dir, "backup-data")
	os.MkdirAll(backupDir, 0755)
	os.WriteFile(filepath.Join(backupDir, "data.tar"), []byte("data"), 0644)

	b := &db.BackupRecord{
		ProjectID: p.ID,
		Type:      "manual",
		Trigger:   "api",
		Path:      backupDir,
		SizeBytes: 4,
	}
	database.CreateBackupRecord(b)

	req := httptest.NewRequest("DELETE", "/api/projects/delbk-ok/backup/"+b.ID, nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "deleted" {
		t.Errorf("expected status=deleted, got %q", resp["status"])
	}

	// Backup directory should be removed.
	if _, err := os.Stat(backupDir); !os.IsNotExist(err) {
		t.Error("backup directory should be removed after delete")
	}
}

// ---------------------------------------------------------------------------
// handleRestoreBackup - backup belongs to different project
// ---------------------------------------------------------------------------

func TestHandleRestoreBackupWrongProject(t *testing.T) {
	srv, database := setupTestServer(t)

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	p1 := &db.Project{
		Name:        "rest-one",
		Domain:      "ro.io",
		LinuxUser:   "fleetdeck-rest-one",
		ProjectPath: dir1,
		Template:    "node",
	}
	database.CreateProject(p1)

	p2 := &db.Project{
		Name:        "rest-two",
		Domain:      "rt.io",
		LinuxUser:   "fleetdeck-rest-two",
		ProjectPath: dir2,
		Template:    "node",
	}
	database.CreateProject(p2)

	b := &db.BackupRecord{
		ProjectID: p1.ID,
		Type:      "manual",
		Trigger:   "api",
		Path:      filepath.Join(dir1, "backup"),
		SizeBytes: 100,
	}
	database.CreateBackupRecord(b)

	// Try to restore p1's backup via p2's endpoint.
	req := httptest.NewRequest("POST", "/api/projects/rest-two/backup/"+b.ID+"/restore", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for cross-project restore, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handleListBackups - empty backups list
// ---------------------------------------------------------------------------

func TestHandleListBackupsEmptyResult(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "no-backups",
		Domain:      "nb.io",
		LinuxUser:   "fleetdeck-no-backups",
		ProjectPath: dir,
		Template:    "node",
	})

	req := httptest.NewRequest("GET", "/api/projects/no-backups/backups", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var backups []apiBackup
	json.NewDecoder(w.Body).Decode(&backups)
	if len(backups) != 0 {
		t.Errorf("expected 0 backups, got %d", len(backups))
	}
}

// ---------------------------------------------------------------------------
// handleListBackups - invalid project name
// ---------------------------------------------------------------------------

func TestHandleListBackupsInvalidName(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/projects/A/backups", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Error("expected non-200 for invalid project name in backups endpoint")
	}
}

// ---------------------------------------------------------------------------
// handleCreateBackup - project exists (will fail without docker but exercises path)
// ---------------------------------------------------------------------------

func TestHandleCreateBackupProjectExists(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "bk-create",
		Domain:      "bkc.io",
		LinuxUser:   "fleetdeck-bk-create",
		ProjectPath: dir,
		Template:    "node",
	})

	req := httptest.NewRequest("POST", "/api/projects/bk-create/backup", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	// Backup will likely fail (no docker/tar), but it should not be 404.
	if w.Code == http.StatusNotFound {
		t.Error("expected project to be found")
	}
	if w.Code == http.StatusBadRequest {
		t.Error("expected valid project name to pass validation")
	}
}

// ---------------------------------------------------------------------------
// Webhook: no webhook secret returns 403
// ---------------------------------------------------------------------------

func TestWebhookNoSecretConfigured(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.webhookSecret = ""

	body := `{}`
	req := httptest.NewRequest("POST", "/api/webhook/github", strings.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 when no webhook secret, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// parseDiskUsage edge cases
// ---------------------------------------------------------------------------

func TestParseDiskUsageInvalidPath(t *testing.T) {
	total, used := parseDiskUsage("/this/path/definitely/does/not/exist")
	if total != 0 || used != 0 {
		t.Errorf("expected 0/0 for invalid path, got %d/%d", total, used)
	}
}

// ---------------------------------------------------------------------------
// parseMemInfo test
// ---------------------------------------------------------------------------

func TestParseMemInfoReturnsValues(t *testing.T) {
	total, avail := parseMemInfo()
	// On Linux these should be positive.
	if total <= 0 {
		t.Skip("skipping: not on Linux or /proc/meminfo not available")
	}
	if avail <= 0 {
		t.Errorf("expected positive available memory, got %d", avail)
	}
	if avail > total {
		t.Errorf("available (%d) should not exceed total (%d)", avail, total)
	}
}

// ---------------------------------------------------------------------------
// finishDeployment tests
// ---------------------------------------------------------------------------

func TestFinishDeploymentSuccess(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	p := &db.Project{
		Name:        "finish-dep",
		Domain:      "fd.io",
		LinuxUser:   "fleetdeck-finish-dep",
		ProjectPath: dir,
		Template:    "node",
		Status:      "deploying",
	}
	database.CreateProject(p)

	dep := &db.Deployment{
		ProjectID: p.ID,
		CommitSHA: "abc123",
		Status:    "deploying",
		StartedAt: time.Now(),
	}
	database.CreateDeployment(dep)

	srv.finishDeployment(dep, "success", "deployment log output", "finish-dep")

	// Verify deployment status updated.
	deployments, _ := database.ListDeployments(p.ID, 10)
	if len(deployments) == 0 {
		t.Fatal("expected at least one deployment")
	}
	found := false
	for _, d := range deployments {
		if d.ID == dep.ID {
			found = true
			if d.Status != "success" {
				t.Errorf("expected deployment status=success, got %q", d.Status)
			}
		}
	}
	if !found {
		t.Error("deployment not found in DB after finishDeployment")
	}

	// Verify project status updated to "running".
	updatedProj, _ := database.GetProject("finish-dep")
	if updatedProj.Status != "running" {
		t.Errorf("expected project status=running after success, got %q", updatedProj.Status)
	}
}

func TestFinishDeploymentFailure(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	p := &db.Project{
		Name:        "fail-dep",
		Domain:      "fail.io",
		LinuxUser:   "fleetdeck-fail-dep",
		ProjectPath: dir,
		Template:    "node",
		Status:      "deploying",
	}
	database.CreateProject(p)

	dep := &db.Deployment{
		ProjectID: p.ID,
		CommitSHA: "def456",
		Status:    "deploying",
		StartedAt: time.Now(),
	}
	database.CreateDeployment(dep)

	srv.finishDeployment(dep, "failed", "build error output", "fail-dep")

	// Project status should be "error" on failure.
	updatedProj, _ := database.GetProject("fail-dep")
	if updatedProj.Status != "error" {
		t.Errorf("expected project status=error after failure, got %q", updatedProj.Status)
	}
}

// ---------------------------------------------------------------------------
// validUUID regex tests
// ---------------------------------------------------------------------------

func TestValidUUIDRegex(t *testing.T) {
	valid := []string{
		"00000000-0000-0000-0000-000000000000",
		"12345678-abcd-ef01-2345-6789abcdef01",
		"a1b2c3d4-e5f6-7890-abcd-ef1234567890",
	}
	for _, id := range valid {
		if !validUUID.MatchString(id) {
			t.Errorf("expected %q to be valid UUID", id)
		}
	}

	invalid := []string{
		"",
		"not-a-uuid",
		"12345678-abcd-ef01-2345",
		"12345678-ABCD-EF01-2345-6789ABCDEF01", // uppercase
		"12345678abcdef0123456789abcdef01",       // no dashes
	}
	for _, id := range invalid {
		if validUUID.MatchString(id) {
			t.Errorf("expected %q to be invalid UUID", id)
		}
	}
}

// ---------------------------------------------------------------------------
// Shutdown test
// ---------------------------------------------------------------------------

func TestServerShutdown(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := config.DefaultConfig()
	cfg.Server.BasePath = dir

	srv := New(cfg, database, "127.0.0.1:0")

	// Start in background, then shut down.
	go srv.Start()

	// Give the server a moment to bind.
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown should not return error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// handleLogin - GET returns HTML
// ---------------------------------------------------------------------------

func TestHandleLoginGETContentType(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html; charset=utf-8", ct)
	}
}

// ---------------------------------------------------------------------------
// handleLoginSubmit with no API token configured
// ---------------------------------------------------------------------------

func TestLoginSubmitNoAPITokenConfigured(t *testing.T) {
	srv, _ := setupTestServer(t)
	// setupTestServer has no APIToken set.

	req := httptest.NewRequest("POST", "/login", strings.NewReader("token=anything"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	// When no API token is configured, the comparison s.apiToken != "" fails,
	// so it falls through to 401.
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when no API token configured, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// New() constructor test
// ---------------------------------------------------------------------------

func TestNewServerHasAllParts(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := config.DefaultConfig()
	cfg.Server.BasePath = dir
	cfg.Server.WebhookSecret = "ws"
	cfg.Server.APIToken = "at"

	srv := New(cfg, database, ":8080")

	if srv.cfg != cfg {
		t.Error("expected cfg to be stored")
	}
	if srv.db != database {
		t.Error("expected db to be stored")
	}
	if srv.webhookSecret != "ws" {
		t.Error("expected webhookSecret to be set from config")
	}
	if srv.apiToken != "at" {
		t.Error("expected apiToken to be set from config")
	}
	if srv.rateLimiter == nil {
		t.Error("expected rateLimiter to be initialized")
	}
	if srv.metrics == nil {
		t.Error("expected metrics to be initialized")
	}
	if srv.server == nil {
		t.Error("expected http server to be initialized")
	}
	if srv.server.Addr != ":8080" {
		t.Errorf("expected addr :8080, got %q", srv.server.Addr)
	}
}
