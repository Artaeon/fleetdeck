//go:build integration

package deploy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Deploy with a real compose file (docker available on this system)
// ---------------------------------------------------------------------------

const minimalComposeYML = `services:
  web:
    image: alpine:latest
    command: ["sleep", "3600"]
`

func writeComposeFile(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(content), 0644); err != nil {
		t.Fatalf("writing docker-compose.yml: %v", err)
	}
}

// TestBasicDeployWithRealCompose exercises the full BasicStrategy.Deploy flow
// using a real (minimal) docker compose file.
func TestIntegrationBasicDeployWithRealCompose(t *testing.T) {
	skipIfNoIntegration(t)
	dir := t.TempDir()
	writeComposeFile(t, dir, minimalComposeYML)
	// Ensure cleanup via docker compose down.
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		bg := &BlueGreenStrategy{}
		bg.stopOld(ctx, DeployOptions{ProjectPath: dir})
	}()

	strategy := &BasicStrategy{}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := strategy.Deploy(ctx, DeployOptions{
		ProjectPath: dir,
	})
	if err != nil {
		t.Fatalf("BasicStrategy.Deploy failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Error("expected result.Success = true")
	}
	if result.Duration == 0 {
		t.Error("expected non-zero duration")
	}
	if len(result.NewContainers) == 0 {
		t.Error("expected at least one new container")
	}
	if len(result.Logs) == 0 {
		t.Error("expected at least one log entry")
	}
}

// TestRollingDeployWithRealCompose exercises the RollingStrategy.Deploy flow.
func TestIntegrationRollingDeployWithRealCompose(t *testing.T) {
	skipIfNoIntegration(t)
	dir := t.TempDir()
	writeComposeFile(t, dir, minimalComposeYML)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		bg := &BlueGreenStrategy{}
		bg.stopOld(ctx, DeployOptions{ProjectPath: dir})
	}()

	strategy := &RollingStrategy{}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := strategy.Deploy(ctx, DeployOptions{
		ProjectPath: dir,
	})
	if err != nil {
		t.Fatalf("RollingStrategy.Deploy failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Error("expected result.Success = true")
	}
	if result.Duration == 0 {
		t.Error("expected non-zero duration")
	}
	if len(result.NewContainers) == 0 {
		t.Error("expected at least one new container")
	}
	// Rolling strategy should have per-service log entries.
	if len(result.Logs) == 0 {
		t.Error("expected at least one log entry")
	}
}

// TestBlueGreenDeployWithRealCompose exercises the BlueGreenStrategy.Deploy
// flow with a real compose file (no health check URL, so it uses the brief
// pause path).
func TestIntegrationBlueGreenDeployWithRealCompose(t *testing.T) {
	skipIfNoIntegration(t)
	dir := t.TempDir()
	writeComposeFile(t, dir, minimalComposeYML)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		bg := &BlueGreenStrategy{}
		bg.stopOld(ctx, DeployOptions{ProjectPath: dir})
		// Also clean up the -new project.
		bg.removeProject(dir, "testbg-new")
	}()

	strategy := &BlueGreenStrategy{}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	result, err := strategy.Deploy(ctx, DeployOptions{
		ProjectPath: dir,
		ProjectName: "testbg",
	})
	if err != nil {
		t.Fatalf("BlueGreenStrategy.Deploy failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Errorf("expected result.Success = true, logs: %v", result.Logs)
	}
	if result.Duration == 0 {
		t.Error("expected non-zero duration")
	}
	// Should have multiple log entries (starting, health check, stopping, promoting, complete).
	if len(result.Logs) < 4 {
		t.Errorf("expected at least 4 log entries for blue-green flow, got %d: %v", len(result.Logs), result.Logs)
	}
	// Check specific log steps.
	logJoined := strings.Join(result.Logs, " | ")
	for _, want := range []string{"starting new containers", "health checks", "deployment complete"} {
		if !strings.Contains(logJoined, want) {
			t.Errorf("logs should contain %q, got: %s", want, logJoined)
		}
	}
}

// TestBlueGreenDeployWithHealthCheck exercises the blue-green flow with a
// health check URL backed by a test server.
func TestIntegrationBlueGreenDeployWithHealthCheck(t *testing.T) {
	skipIfNoIntegration(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dir := t.TempDir()
	writeComposeFile(t, dir, minimalComposeYML)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		bg := &BlueGreenStrategy{}
		bg.stopOld(ctx, DeployOptions{ProjectPath: dir})
		bg.removeProject(dir, "testbghc-new")
	}()

	strategy := &BlueGreenStrategy{}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	result, err := strategy.Deploy(ctx, DeployOptions{
		ProjectPath:    dir,
		ProjectName:    "testbghc",
		HealthCheckURL: server.URL + "/health",
		Timeout:        15 * time.Second,
	})
	if err != nil {
		t.Fatalf("BlueGreenStrategy.Deploy with health check failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Errorf("expected result.Success = true, logs: %v", result.Logs)
	}
	logJoined := strings.Join(result.Logs, " | ")
	if !strings.Contains(logJoined, "health checks passed") {
		t.Errorf("logs should contain 'health checks passed', got: %s", logJoined)
	}
}

// TestBasicDeployPostHookFailure verifies that when the deploy succeeds but
// the post-deploy hook fails, the result reflects the failure.
func TestIntegrationBasicDeployPostHookFailure(t *testing.T) {
	skipIfNoIntegration(t)
	dir := t.TempDir()
	writeComposeFile(t, dir, minimalComposeYML)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		bg := &BlueGreenStrategy{}
		bg.stopOld(ctx, DeployOptions{ProjectPath: dir})
	}()

	strategy := &BasicStrategy{}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := strategy.Deploy(ctx, DeployOptions{
		ProjectPath:    dir,
		PostDeployHook: "exit 1", // will fail
	})
	if err == nil {
		t.Fatal("expected error from post-deploy hook failure")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !strings.Contains(err.Error(), "post-deploy") {
		t.Errorf("error should mention 'post-deploy', got: %v", err)
	}
	if result.Success {
		t.Error("result.Success should be false when post-deploy hook fails")
	}
}

// TestRollingDeployWithComposeFileReal verifies rolling strategy with a
// custom compose file.
func TestIntegrationRollingDeployWithComposeFileReal(t *testing.T) {
	skipIfNoIntegration(t)
	dir := t.TempDir()
	// Write to a custom filename.
	if err := os.WriteFile(filepath.Join(dir, "custom.yml"), []byte(minimalComposeYML), 0644); err != nil {
		t.Fatalf("writing custom.yml: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		bg := &BlueGreenStrategy{}
		bg.stopOld(ctx, DeployOptions{ProjectPath: dir, ComposeFile: "custom.yml"})
	}()

	strategy := &RollingStrategy{}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := strategy.Deploy(ctx, DeployOptions{
		ProjectPath: dir,
		ComposeFile: "custom.yml",
	})
	if err != nil {
		t.Fatalf("RollingStrategy.Deploy with custom compose file failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Error("expected result.Success = true")
	}
}

// TestListServicesWithRealCompose verifies listServices returns services
// from a real compose file.
func TestIntegrationListServicesWithRealCompose(t *testing.T) {
	skipIfNoIntegration(t)
	dir := t.TempDir()
	writeComposeFile(t, dir, minimalComposeYML)

	services, err := listServices(context.Background(), DeployOptions{
		ProjectPath: dir,
	})
	if err != nil {
		t.Fatalf("listServices failed: %v", err)
	}
	if len(services) != 1 {
		t.Fatalf("expected 1 service, got %d: %v", len(services), services)
	}
	if services[0] != "web" {
		t.Errorf("expected service 'web', got %q", services[0])
	}
}

// TestListServicesMultiService verifies listServices with multiple services.
func TestIntegrationListServicesMultiService(t *testing.T) {
	skipIfNoIntegration(t)
	multiServiceCompose := `services:
  web:
    image: alpine:latest
    command: ["sleep", "3600"]
  db:
    image: alpine:latest
    command: ["sleep", "3600"]
  cache:
    image: alpine:latest
    command: ["sleep", "3600"]
`
	dir := t.TempDir()
	writeComposeFile(t, dir, multiServiceCompose)

	services, err := listServices(context.Background(), DeployOptions{
		ProjectPath: dir,
	})
	if err != nil {
		t.Fatalf("listServices failed: %v", err)
	}
	if len(services) != 3 {
		t.Fatalf("expected 3 services, got %d: %v", len(services), services)
	}
}
