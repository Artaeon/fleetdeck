package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// New snapshots identify their exact project-relative destination. Extract and
// validate before replacing it, so stale SQLite WAL files and deleted uploads
// cannot survive a restore. Old snapshots retain their legacy layout.
func restoreProjectBindMount(archive, projectPath, relative string) error {
	if relative == "" {
		return restoreBindMount(archive, projectPath)
	}
	clean := filepath.Clean(relative)
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("invalid bind restore destination")
	}
	root, err := filepath.EvalSymlinks(projectPath)
	if err != nil {
		return err
	}
	target := filepath.Join(root, clean)
	// Refuse links in destination parents; an operator can move external data
	// into a real project directory before restoring it.
	for path := target; path != root; path = filepath.Dir(path) {
		info, err := os.Lstat(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
			return fmt.Errorf("bind restore destination is not a real directory")
		}
	}
	if err := validateBindArchive(archive, filepath.Base(clean)); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(target), ".fleetdeck-restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := restoreBindMount(archive, stage); err != nil {
		return err
	}
	prepared := filepath.Join(stage, filepath.Base(clean))
	if info, err := os.Stat(prepared); err != nil || !info.IsDir() {
		return fmt.Errorf("archive has no destination directory")
	}
	previous, err := os.MkdirTemp(stage, ".previous-")
	if err != nil {
		return err
	}
	if err := os.Remove(previous); err != nil {
		return err
	}
	hadPrevious := false
	if _, err := os.Lstat(target); err == nil {
		if err := os.Rename(target, previous); err != nil {
			return err
		}
		hadPrevious = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(prepared, target); err != nil {
		if hadPrevious {
			if rollbackErr := os.Rename(previous, target); rollbackErr != nil {
				// Keep the recovery directory if both renames failed.
				preserved := stage + "-recovery"
				_ = os.Rename(stage, preserved)
				return fmt.Errorf("activating restored bind: %v; restoring original: %v; recovery at %s", err, rollbackErr, preserved)
			}
		}
		return err
	}
	return nil
}

func validateBindArchive(path, root string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	count := 0
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(header.Name)
		if filepath.IsAbs(name) || (name != root && !strings.HasPrefix(name, root+"/")) || strings.Contains(header.Name, "\\") {
			return fmt.Errorf("archive entry escapes bind directory")
		}
		if header.Typeflag != tar.TypeDir && header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return fmt.Errorf("bind archive contains unsupported link or special file")
		}
		count++
	}
	if count == 0 {
		return fmt.Errorf("empty bind archive")
	}
	return nil
}
