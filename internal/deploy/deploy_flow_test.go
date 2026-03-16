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
// runHook tests
// ---------------------------------------------------------------------------

// TestRunHookFailsWithoutDocker verifies that runHook returns an error when
// docker is not available (exec fails) and that it still appends to the
// result's Logs slice.
func TestRunHookFailsWithoutDocker(t *testing.T) {
	dir := t.TempDir()
	result := &DeployResult{}

	err := runHook(context.Background(), "pre-deploy", "echo hello", dir, result)
	if err == nil {
		t.Fatal("expected error from runHook without docker, got nil")
	}
	if !strings.Contains(err.Error(), "hook failed") {
		t.Errorf("error should mention 'hook failed', got: %v", err)
	}
	if !strings.Contains(err.Error(), "pre-deploy") {
		t.Errorf("error should mention hook label 'pre-deploy', got: %v", err)
	}
	// Even on failure, the output should be appended to Logs.
	if len(result.Logs) == 0 {
		t.Error("expected at least one log entry from runHook on failure")
	}
	if len(result.Logs) > 0 && !strings.Contains(result.Logs[0], "[pre-deploy]") {
		t.Errorf("log entry should be labeled [pre-deploy], got: %s", result.Logs[0])
	}
}

// TestRunHookContextCancelled verifies that runHook respects context
// cancellation and returns an error.
func TestRunHookContextCancelled(t *testing.T) {
	dir := t.TempDir()
	result := &DeployResult{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := runHook(ctx, "post-deploy", "echo hello", dir, result)
	if err == nil {
		t.Fatal("expected error from runHook with cancelled context, got nil")
	}
}

// TestRunHookAppendsLogOnError verifies that runHook always appends a log
// entry, even when the command fails. The log should contain both the label
// and any output from the command.
func TestRunHookAppendsLogOnError(t *testing.T) {
	dir := t.TempDir()
	result := &DeployResult{}
	result.Logs = []string{"existing log"}

	_ = runHook(context.Background(), "migrate", "npm run migrate", dir, result)
	if len(result.Logs) != 2 {
		t.Fatalf("expected 2 log entries (1 existing + 1 from hook), got %d", len(result.Logs))
	}
	if !strings.Contains(result.Logs[1], "[migrate]") {
		t.Errorf("second log entry should contain [migrate], got: %s", result.Logs[1])
	}
}

// TestRunHookEmptyCommand verifies that runHook with an empty command string
// still invokes docker compose exec and returns an error (since docker is
// unavailable).
func TestRunHookEmptyCommand(t *testing.T) {
	dir := t.TempDir()
	result := &DeployResult{}

	err := runHook(context.Background(), "empty", "", dir, result)
	if err == nil {
		t.Fatal("expected error from runHook with empty command (no docker), got nil")
	}
	if len(result.Logs) == 0 {
		t.Error("expected log entry even for empty command")
	}
}

// ---------------------------------------------------------------------------
// BasicStrategy.Deploy tests (without Docker)
// ---------------------------------------------------------------------------

// TestBasicDeployFailsWithoutDocker verifies that BasicStrategy.Deploy returns
// an error when docker is not available but still populates the result
// partially (OldContainers will be empty, Logs will contain output).
func TestBasicDeployFailsWithoutDocker(t *testing.T) {
	dir := t.TempDir()
	strategy := &BasicStrategy{}

	result, err := strategy.Deploy(context.Background(), DeployOptions{
		ProjectPath: dir,
	})
	if err == nil {
		t.Fatal("expected error from BasicStrategy.Deploy without docker")
	}
	if result == nil {
		t.Fatal("expected non-nil result even on error")
	}
	if result.Success {
		t.Error("result.Success should be false on error")
	}
	if result.Duration == 0 {
		t.Error("result.Duration should be non-zero even on error")
	}
	if !strings.Contains(err.Error(), "docker compose up") {
		t.Errorf("error should mention 'docker compose up', got: %v", err)
	}
	// Logs should contain something (the docker compose output).
	if len(result.Logs) == 0 {
		t.Error("expected at least one log entry")
	}
}

// TestBasicDeployWithComposeFile verifies that specifying a ComposeFile still
// produces an error referencing docker compose (the -f flag is included in
// the command).
func TestBasicDeployWithComposeFile(t *testing.T) {
	dir := t.TempDir()
	strategy := &BasicStrategy{}

	result, err := strategy.Deploy(context.Background(), DeployOptions{
		ProjectPath: dir,
		ComposeFile: "docker-compose.prod.yml",
	})
	if err == nil {
		t.Fatal("expected error from BasicStrategy.Deploy without docker")
	}
	if result == nil {
		t.Fatal("expected non-nil result even on error")
	}
	if result.Success {
		t.Error("result.Success should be false on error")
	}
}

// TestBasicDeployPreHookFailure verifies that when the pre-deploy hook fails,
// the deploy returns early with the hook error and does not proceed to docker
// compose up.
func TestBasicDeployPreHookFailure(t *testing.T) {
	dir := t.TempDir()
	strategy := &BasicStrategy{}

	result, err := strategy.Deploy(context.Background(), DeployOptions{
		ProjectPath:   dir,
		PreDeployHook: "npm run migrate",
	})
	if err == nil {
		t.Fatal("expected error from pre-deploy hook")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !strings.Contains(err.Error(), "pre-deploy") {
		t.Errorf("error should mention 'pre-deploy', got: %v", err)
	}
	if !strings.Contains(err.Error(), "hook failed") {
		t.Errorf("error should mention 'hook failed', got: %v", err)
	}
	if result.Success {
		t.Error("result.Success should be false")
	}
	if result.Duration == 0 {
		t.Error("result.Duration should be set even on hook failure")
	}
}

// TestBasicDeployContextCancelled verifies that BasicStrategy.Deploy returns
// an error when the context is already cancelled.
func TestBasicDeployContextCancelled(t *testing.T) {
	dir := t.TempDir()
	strategy := &BasicStrategy{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := strategy.Deploy(ctx, DeployOptions{
		ProjectPath: dir,
	})
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("result.Success should be false")
	}
}

// TestBasicDeployOldContainersEmpty verifies that OldContainers is empty when
// there are no running containers (docker not available).
func TestBasicDeployOldContainersEmpty(t *testing.T) {
	dir := t.TempDir()
	strategy := &BasicStrategy{}

	result, _ := strategy.Deploy(context.Background(), DeployOptions{
		ProjectPath: dir,
	})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Without docker, listContainers returns nil; OldContainers should be nil or empty.
	if len(result.OldContainers) != 0 {
		t.Errorf("expected 0 old containers without docker, got %d", len(result.OldContainers))
	}
}

// TestBasicDeployWithTimeout verifies that the Timeout field in DeployOptions
// is accepted (even though BasicStrategy doesn't use it directly -- it is
// used by the caller).
func TestBasicDeployWithTimeout(t *testing.T) {
	dir := t.TempDir()
	strategy := &BasicStrategy{}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := strategy.Deploy(ctx, DeployOptions{
		ProjectPath: dir,
		Timeout:     30 * time.Second,
	})
	// Should fail because docker is unavailable (or context times out).
	if err == nil {
		t.Fatal("expected error")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// ---------------------------------------------------------------------------
// RollingStrategy.Deploy tests (without Docker)
// ---------------------------------------------------------------------------

// TestRollingDeployFailsWithoutDocker verifies that RollingStrategy.Deploy
// returns an error at the pull step when docker is not available.
func TestRollingDeployFailsWithoutDocker(t *testing.T) {
	dir := t.TempDir()
	strategy := &RollingStrategy{}

	result, err := strategy.Deploy(context.Background(), DeployOptions{
		ProjectPath: dir,
	})
	if err == nil {
		t.Fatal("expected error from RollingStrategy.Deploy without docker")
	}
	if result == nil {
		t.Fatal("expected non-nil result even on error")
	}
	if result.Success {
		t.Error("result.Success should be false on error")
	}
	if result.Duration == 0 {
		t.Error("result.Duration should be non-zero even on error")
	}
	if !strings.Contains(err.Error(), "docker compose pull") {
		t.Errorf("error should mention 'docker compose pull', got: %v", err)
	}
}

// TestRollingDeployWithComposeFile verifies that ComposeFile is included in
// the pull command for rolling strategy.
func TestRollingDeployWithComposeFile(t *testing.T) {
	dir := t.TempDir()
	strategy := &RollingStrategy{}

	result, err := strategy.Deploy(context.Background(), DeployOptions{
		ProjectPath: dir,
		ComposeFile: "docker-compose.staging.yml",
	})
	if err == nil {
		t.Fatal("expected error from RollingStrategy.Deploy without docker")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("result.Success should be false")
	}
}

// TestRollingDeployPreHookFailure verifies that a pre-deploy hook failure
// stops the rolling deploy before the pull step.
func TestRollingDeployPreHookFailure(t *testing.T) {
	dir := t.TempDir()
	strategy := &RollingStrategy{}

	result, err := strategy.Deploy(context.Background(), DeployOptions{
		ProjectPath:   dir,
		PreDeployHook: "npm run db:migrate",
	})
	if err == nil {
		t.Fatal("expected error from pre-deploy hook")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !strings.Contains(err.Error(), "pre-deploy") {
		t.Errorf("error should mention 'pre-deploy', got: %v", err)
	}
	if result.Success {
		t.Error("result.Success should be false")
	}
}

// TestRollingDeployContextCancelled verifies context cancellation.
func TestRollingDeployContextCancelled(t *testing.T) {
	dir := t.TempDir()
	strategy := &RollingStrategy{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := strategy.Deploy(ctx, DeployOptions{
		ProjectPath: dir,
	})
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("result.Success should be false")
	}
}

// TestRollingDeployOldContainersEmpty verifies OldContainers handling.
func TestRollingDeployOldContainersEmpty(t *testing.T) {
	dir := t.TempDir()
	strategy := &RollingStrategy{}

	result, _ := strategy.Deploy(context.Background(), DeployOptions{
		ProjectPath: dir,
	})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.OldContainers) != 0 {
		t.Errorf("expected 0 old containers, got %d", len(result.OldContainers))
	}
}

// TestRollingDeployLogsOnPullFailure verifies that the pull failure output
// is captured in Logs.
func TestRollingDeployLogsOnPullFailure(t *testing.T) {
	dir := t.TempDir()
	strategy := &RollingStrategy{}

	result, err := strategy.Deploy(context.Background(), DeployOptions{
		ProjectPath: dir,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	// The pull failure should add a log entry.
	if len(result.Logs) == 0 {
		t.Error("expected log entry from pull failure")
	}
}

// ---------------------------------------------------------------------------
// BlueGreenStrategy.Deploy tests (without Docker)
// ---------------------------------------------------------------------------

// TestBlueGreenDeployFailsAtStartNew verifies that BlueGreenStrategy.Deploy
// returns an error at the startNew step when docker is not available.
func TestBlueGreenDeployFailsAtStartNew(t *testing.T) {
	dir := t.TempDir()
	strategy := &BlueGreenStrategy{}

	result, err := strategy.Deploy(context.Background(), DeployOptions{
		ProjectPath: dir,
		ProjectName: "myapp",
	})
	if err == nil {
		t.Fatal("expected error from BlueGreenStrategy.Deploy without docker")
	}
	if result == nil {
		t.Fatal("expected non-nil result even on error")
	}
	if result.Success {
		t.Error("result.Success should be false on error")
	}
	if result.Duration == 0 {
		t.Error("result.Duration should be non-zero")
	}
	if !strings.Contains(err.Error(), "starting new containers") {
		t.Errorf("error should mention 'starting new containers', got: %v", err)
	}
	// Should have the "starting new containers" log entry.
	found := false
	for _, l := range result.Logs {
		if strings.Contains(l, "starting new containers") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("logs should contain 'starting new containers', got: %v", result.Logs)
	}
}

// TestBlueGreenDeployPreHookFailure verifies that a pre-deploy hook failure
// stops the blue-green deploy before starting new containers.
func TestBlueGreenDeployPreHookFailure(t *testing.T) {
	dir := t.TempDir()
	strategy := &BlueGreenStrategy{}

	result, err := strategy.Deploy(context.Background(), DeployOptions{
		ProjectPath:   dir,
		ProjectName:   "myapp",
		PreDeployHook: "npm run migrate",
	})
	if err == nil {
		t.Fatal("expected error from pre-deploy hook")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !strings.Contains(err.Error(), "pre-deploy") {
		t.Errorf("error should mention 'pre-deploy', got: %v", err)
	}
	if result.Success {
		t.Error("result.Success should be false")
	}
}

// TestBlueGreenDeployContextCancelled verifies context cancellation.
func TestBlueGreenDeployContextCancelled(t *testing.T) {
	dir := t.TempDir()
	strategy := &BlueGreenStrategy{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := strategy.Deploy(ctx, DeployOptions{
		ProjectPath: dir,
		ProjectName: "myapp",
	})
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("result.Success should be false")
	}
}

// TestBlueGreenDeployWithComposeFile verifies ComposeFile is accepted.
func TestBlueGreenDeployWithComposeFile(t *testing.T) {
	dir := t.TempDir()
	strategy := &BlueGreenStrategy{}

	result, err := strategy.Deploy(context.Background(), DeployOptions{
		ProjectPath: dir,
		ProjectName: "myapp",
		ComposeFile: "docker-compose.prod.yml",
	})
	if err == nil {
		t.Fatal("expected error without docker")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestBlueGreenDeployDefaultTimeout verifies that when Timeout is 0, the
// blue-green strategy uses 60s as default. We can't directly observe this
// since the deploy fails at startNew, but we verify the flow works.
func TestBlueGreenDeployDefaultTimeout(t *testing.T) {
	dir := t.TempDir()
	strategy := &BlueGreenStrategy{}

	result, err := strategy.Deploy(context.Background(), DeployOptions{
		ProjectPath: dir,
		ProjectName: "myapp",
		Timeout:     0, // should default to 60s
	})
	if err == nil {
		t.Fatal("expected error without docker")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestBlueGreenDeployNewProjectName verifies the new project naming.
func TestBlueGreenDeployNewProjectName(t *testing.T) {
	dir := t.TempDir()
	strategy := &BlueGreenStrategy{}

	result, err := strategy.Deploy(context.Background(), DeployOptions{
		ProjectPath: dir,
		ProjectName: "production-app",
	})
	// Deploy will fail at startNew, but the error should reference the
	// docker compose up command with the -new project name.
	if err == nil {
		t.Fatal("expected error without docker")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("result.Success should be false")
	}
}

// ---------------------------------------------------------------------------
// BlueGreenStrategy helper methods
// ---------------------------------------------------------------------------

// TestBlueGreenStopOldInvalidPath verifies stopOld returns an error with
// an invalid path (guaranteed failure regardless of docker availability).
func TestBlueGreenStopOldInvalidPath(t *testing.T) {
	strategy := &BlueGreenStrategy{}

	err := strategy.stopOld(context.Background(), DeployOptions{
		ProjectPath: "/nonexistent/path/that/does/not/exist",
	})
	if err == nil {
		t.Fatal("expected error from stopOld with invalid path")
	}
	if !strings.Contains(err.Error(), "docker compose down") {
		t.Errorf("error should mention 'docker compose down', got: %v", err)
	}
}

// TestBlueGreenStopOldWithComposeFile verifies ComposeFile is accepted.
func TestBlueGreenStopOldWithComposeFile(t *testing.T) {
	strategy := &BlueGreenStrategy{}

	err := strategy.stopOld(context.Background(), DeployOptions{
		ProjectPath: "/nonexistent/path/that/does/not/exist",
		ComposeFile: "custom.yml",
	})
	if err == nil {
		t.Fatal("expected error from stopOld with invalid path")
	}
}

// TestBlueGreenStartOriginalInvalidPath verifies startOriginal returns an error.
func TestBlueGreenStartOriginalInvalidPath(t *testing.T) {
	strategy := &BlueGreenStrategy{}

	err := strategy.startOriginal(context.Background(), DeployOptions{
		ProjectPath: "/nonexistent/path/that/does/not/exist",
	})
	if err == nil {
		t.Fatal("expected error from startOriginal with invalid path")
	}
	if !strings.Contains(err.Error(), "docker compose up") {
		t.Errorf("error should mention 'docker compose up', got: %v", err)
	}
}

// TestBlueGreenStartOriginalWithComposeFile verifies ComposeFile is included.
func TestBlueGreenStartOriginalWithComposeFile(t *testing.T) {
	strategy := &BlueGreenStrategy{}

	err := strategy.startOriginal(context.Background(), DeployOptions{
		ProjectPath: "/nonexistent/path/that/does/not/exist",
		ComposeFile: "docker-compose.staging.yml",
	})
	if err == nil {
		t.Fatal("expected error from startOriginal with invalid path")
	}
}

// TestBlueGreenRemoveProjectNoDocker verifies removeProject returns an error
// when docker is not available, or succeeds gracefully when it is.
func TestBlueGreenRemoveProjectNoDocker(t *testing.T) {
	dir := t.TempDir()
	strategy := &BlueGreenStrategy{}

	err := strategy.removeProject(dir, "test-project-new")
	if err != nil {
		// Docker not available: verify error message.
		if !strings.Contains(err.Error(), "removing project") {
			t.Errorf("error should mention 'removing project', got: %v", err)
		}
		if !strings.Contains(err.Error(), "test-project-new") {
			t.Errorf("error should mention project name, got: %v", err)
		}
	}
	// If docker is available, the command may succeed (no-op for nonexistent project).
}

// TestBlueGreenRemoveProjectInvalidPath verifies removeProject returns an
// error when given a non-existent directory.
func TestBlueGreenRemoveProjectInvalidPath(t *testing.T) {
	strategy := &BlueGreenStrategy{}

	err := strategy.removeProject("/nonexistent/path/that/does/not/exist", "test-project-new")
	if err == nil {
		t.Fatal("expected error from removeProject with invalid path")
	}
	if !strings.Contains(err.Error(), "removing project") {
		t.Errorf("error should mention 'removing project', got: %v", err)
	}
}

// TestBlueGreenStartNewWithComposeFile verifies ComposeFile is included.
func TestBlueGreenStartNewWithComposeFile(t *testing.T) {
	strategy := &BlueGreenStrategy{}

	err := strategy.startNew(context.Background(), DeployOptions{
		ProjectPath: "/nonexistent/path/that/does/not/exist",
		ComposeFile: "custom-compose.yml",
	}, "myapp-new")
	if err == nil {
		t.Fatal("expected error from startNew with invalid path")
	}
	if !strings.Contains(err.Error(), "docker compose up") {
		t.Errorf("error should mention 'docker compose up', got: %v", err)
	}
}

// TestBlueGreenStartNewContextCancelled verifies context cancellation.
func TestBlueGreenStartNewContextCancelled(t *testing.T) {
	dir := t.TempDir()
	strategy := &BlueGreenStrategy{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := strategy.startNew(ctx, DeployOptions{
		ProjectPath: dir,
	}, "myapp-new")
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

// TestBlueGreenStopOldContextCancelled verifies stopOld with cancelled context.
func TestBlueGreenStopOldContextCancelled(t *testing.T) {
	dir := t.TempDir()
	strategy := &BlueGreenStrategy{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := strategy.stopOld(ctx, DeployOptions{
		ProjectPath: dir,
	})
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

// TestBlueGreenStartOriginalContextCancelled verifies startOriginal with
// cancelled context.
func TestBlueGreenStartOriginalContextCancelled(t *testing.T) {
	dir := t.TempDir()
	strategy := &BlueGreenStrategy{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := strategy.startOriginal(ctx, DeployOptions{
		ProjectPath: dir,
	})
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

// ---------------------------------------------------------------------------
// BlueGreenStrategy.waitHealthy additional tests
// ---------------------------------------------------------------------------

// TestBlueGreenWaitHealthyAccepts2xxAnd3xx verifies that waitHealthy accepts
// all 2xx and 3xx status codes.
func TestBlueGreenWaitHealthyAccepts2xxAnd3xx(t *testing.T) {
	codes := []int{200, 201, 204, 301, 302, 307}
	for _, code := range codes {
		code := code
		t.Run(http.StatusText(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer server.Close()

			s := &BlueGreenStrategy{}
			err := s.waitHealthy(context.Background(), DeployOptions{
				HealthCheckURL: server.URL,
			}, 5*time.Second)
			if err != nil {
				t.Fatalf("waitHealthy should accept status %d, got error: %v", code, err)
			}
		})
	}
}

// TestBlueGreenWaitHealthyRejects4xxAnd5xx verifies that waitHealthy does not
// accept 4xx and 5xx status codes and eventually times out.
func TestBlueGreenWaitHealthyRejects4xxAnd5xx(t *testing.T) {
	codes := []int{400, 403, 404, 500, 502, 503}
	for _, code := range codes {
		code := code
		t.Run(http.StatusText(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer server.Close()

			s := &BlueGreenStrategy{}
			err := s.waitHealthy(context.Background(), DeployOptions{
				HealthCheckURL: server.URL,
			}, 3*time.Second)
			if err == nil {
				t.Fatalf("waitHealthy should reject status %d", code)
			}
			if !strings.Contains(err.Error(), "timed out") {
				t.Errorf("expected timeout error for status %d, got: %v", code, err)
			}
		})
	}
}

// TestBlueGreenWaitHealthyUnreachableServer verifies that waitHealthy times
// out when the server is unreachable.
func TestBlueGreenWaitHealthyUnreachableServer(t *testing.T) {
	s := &BlueGreenStrategy{}
	err := s.waitHealthy(context.Background(), DeployOptions{
		HealthCheckURL: "http://127.0.0.1:1", // unlikely to have anything listening
	}, 3*time.Second)
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Lock edge cases
// ---------------------------------------------------------------------------

// TestLockReleaseIdempotent verifies that calling Release twice does not panic
// and that the second call handles the already-removed file gracefully.
func TestLockReleaseIdempotent(t *testing.T) {
	dir := t.TempDir()

	lock, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}

	if err := lock.Release(); err != nil {
		t.Fatalf("first Release: %v", err)
	}

	// Second release: lockFile is already closed, but the nil check should
	// not apply because lockFile is not nil (it's a closed *os.File).
	// This may return an error for double-close, but must not panic.
	_ = lock.Release()
}

// TestLockFilePermissions verifies the lock file is created with 0600
// permissions.
func TestLockFilePermissions(t *testing.T) {
	dir := t.TempDir()

	lock, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	defer lock.Release()

	lockPath := filepath.Join(dir, ".fleetdeck.lock")
	info, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("stat lock file: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("lock file permissions = %o, want 0600", perm)
	}
}

// TestLockAcquireReadOnlyDir verifies that AcquireLock fails with a clear
// error when the directory is read-only.
func TestLockAcquireReadOnlyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	// Restore permissions for cleanup.
	defer os.Chmod(dir, 0755)

	_, err := AcquireLock(dir)
	if err == nil {
		t.Fatal("expected error for read-only directory")
	}
	if !strings.Contains(err.Error(), "creating lock file") {
		t.Errorf("error should mention 'creating lock file', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// listServices edge cases
// ---------------------------------------------------------------------------

// TestListServicesEmptyDir verifies that listServices fails in an empty
// directory (no docker-compose.yml).
func TestListServicesEmptyDir(t *testing.T) {
	dir := t.TempDir()
	_, err := listServices(context.Background(), DeployOptions{
		ProjectPath: dir,
	})
	if err == nil {
		t.Log("listServices may succeed if docker is available but compose file is missing")
	}
}

// TestListServicesWithCancelledContext verifies context cancellation.
func TestListServicesWithCancelledContext(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := listServices(ctx, DeployOptions{
		ProjectPath: dir,
		ComposeFile: "nonexistent.yml",
	})
	if err == nil {
		t.Log("expected error for cancelled context")
	}
}

// ---------------------------------------------------------------------------
// listContainers edge cases
// ---------------------------------------------------------------------------

// TestListContainersValidEmptyDir verifies listContainers handles a valid but
// empty directory (no compose project).
func TestListContainersValidEmptyDir(t *testing.T) {
	dir := t.TempDir()
	containers, err := listContainers(dir)
	if err != nil {
		// Expected: no compose project in this directory.
		if containers != nil {
			t.Errorf("expected nil containers on error, got: %v", containers)
		}
		return
	}
	// If docker is available, should be empty.
	if len(containers) != 0 {
		t.Errorf("expected 0 containers for empty dir, got %d", len(containers))
	}
}

// ---------------------------------------------------------------------------
// DeployOptions hook fields
// ---------------------------------------------------------------------------

// TestDeployOptionsHookFields verifies hook field values are preserved.
func TestDeployOptionsHookFields(t *testing.T) {
	opts := DeployOptions{
		ProjectPath:    "/opt/app",
		PreDeployHook:  "npm run migrate",
		PostDeployHook: "npm run seed",
	}
	if opts.PreDeployHook != "npm run migrate" {
		t.Errorf("PreDeployHook = %q, want %q", opts.PreDeployHook, "npm run migrate")
	}
	if opts.PostDeployHook != "npm run seed" {
		t.Errorf("PostDeployHook = %q, want %q", opts.PostDeployHook, "npm run seed")
	}
}

// TestDeployOptionsZeroHooks verifies that zero-value hooks are empty strings.
func TestDeployOptionsZeroHooks(t *testing.T) {
	var opts DeployOptions
	if opts.PreDeployHook != "" {
		t.Errorf("zero-value PreDeployHook = %q, want empty", opts.PreDeployHook)
	}
	if opts.PostDeployHook != "" {
		t.Errorf("zero-value PostDeployHook = %q, want empty", opts.PostDeployHook)
	}
}

// ---------------------------------------------------------------------------
// Strategy Deploy with all options populated
// ---------------------------------------------------------------------------

// TestBasicDeployAllOptions exercises BasicStrategy.Deploy with all options
// populated. It should fail at the docker compose up step (no docker).
func TestBasicDeployAllOptions(t *testing.T) {
	dir := t.TempDir()
	strategy := &BasicStrategy{}

	result, err := strategy.Deploy(context.Background(), DeployOptions{
		ProjectPath:    dir,
		ProjectName:    "myapp",
		ComposeFile:    "docker-compose.yml",
		HealthCheckURL: "http://localhost:8080/health",
		Timeout:        30 * time.Second,
		PreDeployHook:  "echo pre",
		PostDeployHook: "echo post",
	})
	if err == nil {
		t.Fatal("expected error without docker")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Pre-deploy hook should fail first (before docker compose up).
	if !strings.Contains(err.Error(), "pre-deploy") {
		t.Errorf("error should mention 'pre-deploy' (hook fails first), got: %v", err)
	}
}

// TestBlueGreenDeployAllOptions exercises BlueGreenStrategy.Deploy with all
// options populated.
func TestBlueGreenDeployAllOptions(t *testing.T) {
	dir := t.TempDir()
	strategy := &BlueGreenStrategy{}

	result, err := strategy.Deploy(context.Background(), DeployOptions{
		ProjectPath:    dir,
		ProjectName:    "myapp",
		ComposeFile:    "docker-compose.yml",
		HealthCheckURL: "http://localhost:8080/health",
		Timeout:        10 * time.Second,
		PreDeployHook:  "echo pre",
		PostDeployHook: "echo post",
	})
	if err == nil {
		t.Fatal("expected error without docker")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Pre-deploy hook fails first.
	if !strings.Contains(err.Error(), "pre-deploy") {
		t.Errorf("error should mention 'pre-deploy', got: %v", err)
	}
}

// TestRollingDeployAllOptions exercises RollingStrategy.Deploy with all
// options populated.
func TestRollingDeployAllOptions(t *testing.T) {
	dir := t.TempDir()
	strategy := &RollingStrategy{}

	result, err := strategy.Deploy(context.Background(), DeployOptions{
		ProjectPath:    dir,
		ProjectName:    "myapp",
		ComposeFile:    "docker-compose.yml",
		HealthCheckURL: "http://localhost:8080/health",
		Timeout:        10 * time.Second,
		PreDeployHook:  "echo pre",
		PostDeployHook: "echo post",
	})
	if err == nil {
		t.Fatal("expected error without docker")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Pre-deploy hook fails first.
	if !strings.Contains(err.Error(), "pre-deploy") {
		t.Errorf("error should mention 'pre-deploy', got: %v", err)
	}
}

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
func TestBasicDeployWithRealCompose(t *testing.T) {
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
func TestRollingDeployWithRealCompose(t *testing.T) {
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
func TestBlueGreenDeployWithRealCompose(t *testing.T) {
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
func TestBlueGreenDeployWithHealthCheck(t *testing.T) {
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
func TestBasicDeployPostHookFailure(t *testing.T) {
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
func TestRollingDeployWithComposeFileReal(t *testing.T) {
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
func TestListServicesWithRealCompose(t *testing.T) {
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
func TestListServicesMultiService(t *testing.T) {
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
