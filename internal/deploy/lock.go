package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// ProjectLock holds an exclusive file lock for a project directory to prevent
// concurrent deployments.
type ProjectLock struct {
	lockFile *os.File
	path     string
}

// AcquireLock creates a .fleetdeck.lock file in the project directory and
// acquires an exclusive lock on it. Returns an error if the lock is already
// held by another process.
func AcquireLock(projectPath string) (*ProjectLock, error) {
	lockPath := filepath.Join(projectPath, ".fleetdeck.lock")

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("creating lock file: %w", err)
	}

	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("project is already being deployed")
	}

	return &ProjectLock{lockFile: f, path: lockPath}, nil
}

// Release releases the file lock and removes the lock file.
func (l *ProjectLock) Release() error {
	if l.lockFile == nil {
		return nil
	}
	syscall.Flock(int(l.lockFile.Fd()), syscall.LOCK_UN)
	l.lockFile.Close()
	os.Remove(l.path)
	return nil
}
