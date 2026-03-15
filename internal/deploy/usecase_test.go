package deploy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestUseCaseConcurrentDeployBlocked simulates two developers trying to deploy
// the same project at the same time. The first deploy acquires the lock, and
// the second should get an "already being deployed" error. After the first
// releases, the second should be able to proceed.
func TestUseCaseConcurrentDeployBlocked(t *testing.T) {
	dir := t.TempDir()

	// First deploy acquires the lock.
	lock1, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("first AcquireLock: %v", err)
	}

	// Second deploy tries in a goroutine -- should be blocked.
	var wg sync.WaitGroup
	var lock2Err error

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, lock2Err = AcquireLock(dir)
	}()
	wg.Wait()

	// The second lock attempt should have failed immediately (LOCK_NB).
	if lock2Err == nil {
		t.Fatal("second AcquireLock should fail while first is held")
	}
	if !strings.Contains(lock2Err.Error(), "already being deployed") {
		t.Errorf("error should say 'already being deployed', got: %v", lock2Err)
	}

	// Release the first lock.
	if err := lock1.Release(); err != nil {
		t.Fatalf("Release first lock: %v", err)
	}

	// Now the second deploy should be able to acquire the lock.
	lock2, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("second AcquireLock after release should succeed: %v", err)
	}
	lock2.Release()
}

// TestUseCaseLockSurvivesProcessCrash simulates a crash where the lock is
// acquired but never released. The lock file remains on disk, but the OS
// releases the flock when the file handle is closed (process death). We
// simulate this by closing the file handle directly.
func TestUseCaseLockSurvivesProcessCrash(t *testing.T) {
	dir := t.TempDir()

	// Acquire lock (simulating a deploy that will "crash").
	lock, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}

	// Verify the lock file exists.
	lockPath := filepath.Join(dir, ".fleetdeck.lock")
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Fatal("lock file should exist while lock is held")
	}

	// Simulate a crash: close the file handle without calling Release.
	// This releases the flock at the OS level but leaves the file on disk.
	lock.lockFile.Close()

	// The lock file should still exist on disk (crash left it behind).
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Fatal("lock file should still exist after crash (file handle closed)")
	}

	// But a new process should be able to re-acquire it because the OS
	// released the flock when the file handle was closed.
	lock2, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("re-acquiring lock after crash should succeed: %v", err)
	}
	lock2.Release()
}

// TestUseCaseSelectDeployStrategy verifies that for each known strategy name,
// GetStrategy returns the correct type and the returned strategy has a Deploy
// method (implements the Strategy interface).
func TestUseCaseSelectDeployStrategy(t *testing.T) {
	strategies := []struct {
		name     string
		wantType string
	}{
		{"basic", "*deploy.BasicStrategy"},
		{"bluegreen", "*deploy.BlueGreenStrategy"},
		{"rolling", "*deploy.RollingStrategy"},
		{"", "*deploy.BasicStrategy"}, // empty defaults to basic
	}

	for _, tt := range strategies {
		t.Run("strategy_"+tt.name, func(t *testing.T) {
			strategy, err := GetStrategy(tt.name)
			if err != nil {
				t.Fatalf("GetStrategy(%q) error: %v", tt.name, err)
			}
			if strategy == nil {
				t.Fatalf("GetStrategy(%q) returned nil", tt.name)
			}

			// Verify the concrete type.
			typeName := fmt.Sprintf("%T", strategy)
			if typeName != tt.wantType {
				t.Errorf("GetStrategy(%q) type = %s, want %s", tt.name, typeName, tt.wantType)
			}

			// Verify it has a Deploy method (satisfies the Strategy interface).
			var _ Strategy = strategy
		})
	}
}

// TestUseCaseBlueGreenHealthCheckFlow simulates a real blue-green deployment
// where the new containers start returning 500 (still booting) and then
// switch to 200 (ready). waitHealthy should eventually succeed.
func TestUseCaseBlueGreenHealthCheckFlow(t *testing.T) {
	var mu sync.Mutex
	callCount := 0
	switchAfter := 3 // Return 200 after 3 failed attempts.

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		current := callCount
		mu.Unlock()

		if current <= switchAfter {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	s := &BlueGreenStrategy{}
	ctx := context.Background()
	opts := DeployOptions{
		HealthCheckURL: server.URL + "/health",
	}

	err := s.waitHealthy(ctx, opts, 30*time.Second)
	if err != nil {
		t.Fatalf("waitHealthy should succeed after retries, got: %v", err)
	}

	mu.Lock()
	finalCount := callCount
	mu.Unlock()

	// Should have made at least switchAfter+1 calls (failures + success).
	if finalCount < switchAfter+1 {
		t.Errorf("expected at least %d health check calls, got %d", switchAfter+1, finalCount)
	}
}

// TestUseCaseDeployLockAndStrategy exercises the full flow that deployLocal
// performs: acquire lock, get strategy, verify lock is held during strategy
// selection, then release. This ensures the lock and strategy systems work
// together correctly.
func TestUseCaseDeployLockAndStrategy(t *testing.T) {
	dir := t.TempDir()

	// Step 1: Acquire lock (like deployLocal does).
	lock, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}

	// Step 2: While lock is held, select a strategy.
	strategy, err := GetStrategy("bluegreen")
	if err != nil {
		lock.Release()
		t.Fatalf("GetStrategy: %v", err)
	}
	if strategy == nil {
		lock.Release()
		t.Fatal("strategy should not be nil")
	}

	// Step 3: Verify the lock is still held (another process cannot acquire).
	_, err = AcquireLock(dir)
	if err == nil {
		lock.Release()
		t.Fatal("lock should still be held during strategy selection")
	}

	// Step 4: Release the lock (like defer lock.Release() in deployLocal).
	if err := lock.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}

	// Step 5: Verify the lock is released.
	lockPath := filepath.Join(dir, ".fleetdeck.lock")
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("lock file should be removed after release")
	}
}

// TestUseCaseInvalidStrategyName verifies that common user typos and alternate
// spellings of strategy names produce a clear, user-friendly error.
func TestUseCaseInvalidStrategyName(t *testing.T) {
	invalidNames := []struct {
		name   string
		reason string
	}{
		{"blue-green", "hyphenated variant"},
		{"Blue/Green", "slash-separated with capitals"},
		{"zero-downtime", "alternate naming"},
		{"canary", "unsupported strategy"},
		{"BLUEGREEN", "uppercase"},
		{"rolling-update", "verbose variant"},
	}

	for _, tt := range invalidNames {
		t.Run(tt.reason, func(t *testing.T) {
			strategy, err := GetStrategy(tt.name)
			if err == nil {
				t.Errorf("GetStrategy(%q) should fail (%s), got strategy %T", tt.name, tt.reason, strategy)
			}
			if strategy != nil {
				t.Errorf("GetStrategy(%q) should return nil on error", tt.name)
			}

			// The error message should contain the invalid name so the user
			// can see what they typed wrong.
			if !strings.Contains(err.Error(), tt.name) {
				t.Errorf("error should contain %q, got: %v", tt.name, err)
			}
			// The error should mention "unknown" to be clear about the problem.
			if !strings.Contains(err.Error(), "unknown") {
				t.Errorf("error should mention 'unknown', got: %v", err)
			}
		})
	}
}
