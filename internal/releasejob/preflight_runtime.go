package releasejob

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const defaultReleasePreflightRuntimeDirectory = "/run/fleetdeck/release-preflight"

type releasePreflightRuntime struct {
	directory string
	ownerUID  uint32
}

func defaultReleasePreflightRuntime() releasePreflightRuntime {
	return releasePreflightRuntime{
		directory: defaultReleasePreflightRuntimeDirectory,
		ownerUID:  0,
	}
}

func (runtime releasePreflightRuntime) prepare(projectPath string) (string, error) {
	if runtime.directory == "" || !filepath.IsAbs(runtime.directory) {
		return "", errors.New("release preflight runtime directory must be an absolute path")
	}
	directory := filepath.Clean(runtime.directory)
	if pathInside(projectPath, directory) || pathInside(directory, projectPath) {
		return "", errors.New("release preflight runtime directory must be isolated from the project path")
	}
	if err := validateRuntimePath(directory, runtime.ownerUID, false); err != nil {
		return "", err
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", errors.New("create release preflight runtime directory")
	}
	if err := validateRuntimePath(directory, runtime.ownerUID, true); err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return "", errors.New("resolve release preflight runtime directory")
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", errors.New("resolve release preflight runtime directory")
	}
	if resolved != directory {
		return "", errors.New("release preflight runtime directory must not contain symlinks")
	}
	if pathInside(projectPath, resolved) || pathInside(resolved, projectPath) {
		return "", errors.New("release preflight runtime directory must be isolated from the project path")
	}
	return resolved, nil
}

func validateRuntimePath(path string, expectedOwner uint32, requireLeaf bool) error {
	current := path
	isLeaf := true
	for {
		info, err := os.Lstat(current)
		switch {
		case err == nil:
			if info.Mode()&os.ModeSymlink != 0 {
				return errors.New("release preflight runtime directory must not contain symlinks")
			}
			if !info.IsDir() {
				return errors.New("release preflight runtime path contains a non-directory component")
			}
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok || (stat.Uid != 0 && stat.Uid != expectedOwner) {
				return errors.New("release preflight runtime path has unsafe ownership")
			}
			if isLeaf {
				if stat.Uid != expectedOwner || info.Mode().Perm() != 0700 {
					return errors.New("release preflight runtime directory must have the configured owner and mode 0700")
				}
			} else if info.Mode().Perm()&0022 != 0 && info.Mode()&os.ModeSticky == 0 {
				return errors.New("release preflight runtime path has a writable non-sticky ancestor")
			}
		case errors.Is(err, os.ErrNotExist):
			if isLeaf && requireLeaf {
				return errors.New("release preflight runtime directory is unavailable")
			}
		case err != nil:
			return fmt.Errorf("inspect release preflight runtime path: %w", err)
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
		isLeaf = false
	}
	return nil
}
