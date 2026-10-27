package deploy

import (
	"os"
	"path/filepath"
	"syscall"
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

	// The inode must remain stable across acquisitions
	if _, err := os.Stat(lockPath); err != nil {
		t.Error("lock file must remain after release")
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

func TestLockKeepsExistingDescriptorSerialized(t *testing.T) {
	dir := t.TempDir()
	first, err := AcquireLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	waiter, err := os.OpenFile(filepath.Join(dir, ".fleetdeck.lock"), os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer waiter.Close()
	if err = first.Release(); err != nil {
		t.Fatal(err)
	}
	if err = syscall.Flock(int(waiter.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	defer syscall.Flock(int(waiter.Fd()), syscall.LOCK_UN)
	if third, err := AcquireLock(dir); err == nil {
		third.Release()
		t.Fatal("different inode allowed concurrent deploy")
	}
}
func TestLockRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "private")
	os.WriteFile(outside, []byte("keep"), 0600)
	if err := os.Symlink(outside, filepath.Join(dir, ".fleetdeck.lock")); err != nil {
		t.Fatal(err)
	}
	if lock, err := AcquireLock(dir); err == nil {
		lock.Release()
		t.Fatal("accepted symlink lock")
	}
}
