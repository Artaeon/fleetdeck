package deploy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireAndReleaseLock(t *testing.T) {
	dir := t.TempDir()

	lock, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}

	// Lock file should exist
	lockPath := filepath.Join(dir, ".fleetdeck.lock")
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file should exist: %v", err)
	}

	if err := lock.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}

	// Lock file should be removed
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("lock file should be removed after release")
	}
}

func TestAcquireLockConcurrent(t *testing.T) {
	dir := t.TempDir()

	lock1, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("first AcquireLock: %v", err)
	}
	defer lock1.Release()

	// Second lock should fail
	_, err = AcquireLock(dir)
	if err == nil {
		t.Fatal("expected error for concurrent lock")
	}
	if err.Error() != "project is already being deployed" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAcquireLockAfterRelease(t *testing.T) {
	dir := t.TempDir()

	lock1, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("first AcquireLock: %v", err)
	}
	lock1.Release()

	// Should be able to lock again
	lock2, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("second AcquireLock after release: %v", err)
	}
	lock2.Release()
}

func TestAcquireLockInvalidPath(t *testing.T) {
	_, err := AcquireLock("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestReleaseNilLockFile(t *testing.T) {
	lock := &ProjectLock{}
	if err := lock.Release(); err != nil {
		t.Fatalf("Release with nil lockFile: %v", err)
	}
}
