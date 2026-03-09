package disaster

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/db"
)

// ImportState extracts a state export archive to the target base path,
// restoring the database and project backups.
func ImportState(archivePath string, targetBasePath string) error {
	// Extract to a temporary directory first for validation
	tmpDir, err := os.MkdirTemp("", "fleetdeck-import-*")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := extractTarGz(archivePath, tmpDir); err != nil {
		return fmt.Errorf("extracting archive: %w", err)
	}

	// Read and validate state.json
	manifestPath := filepath.Join(tmpDir, "state.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("reading state manifest: %w (archive may be invalid)", err)
	}

	var manifest StateManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("parsing state manifest: %w", err)
	}

	if manifest.ExportTimestamp == "" {
		return fmt.Errorf("invalid state manifest: missing export_timestamp")
	}

	// Copy database to target base path
	srcDB := filepath.Join(tmpDir, "fleetdeck.db")
	if _, err := os.Stat(srcDB); os.IsNotExist(err) {
		return fmt.Errorf("archive does not contain fleetdeck.db")
	}

	if err := os.MkdirAll(targetBasePath, 0755); err != nil {
		return fmt.Errorf("creating target base path: %w", err)
	}

	dstDB := filepath.Join(targetBasePath, "fleetdeck.db")
	if err := copyFile(srcDB, dstDB); err != nil {
		return fmt.Errorf("copying database: %w", err)
	}

	// Validate the imported database opens correctly
	testDB, err := db.Open(dstDB)
	if err != nil {
		// Clean up the broken database
		os.Remove(dstDB)
		return fmt.Errorf("imported database is corrupt: %w", err)
	}
	testDB.Close()

	// Copy backups to target backup path (same structure as in archive)
	srcBackups := filepath.Join(tmpDir, "backups")
	if info, err := os.Stat(srcBackups); err == nil && info.IsDir() {
		dstBackups := filepath.Join(targetBasePath, "backups")
		if err := copyDir(srcBackups, dstBackups); err != nil {
			return fmt.Errorf("copying backups: %w", err)
		}
	}

	// Copy config if present
	srcConfig := filepath.Join(tmpDir, "config.toml")
	if _, err := os.Stat(srcConfig); err == nil {
		dstConfig := filepath.Join(targetBasePath, "config.toml")
		if err := copyFile(srcConfig, dstConfig); err != nil {
			return fmt.Errorf("copying config: %w", err)
		}
	}

	return nil
}

// ReadStateManifest reads the state.json from an export archive without
// fully importing it.
func ReadStateManifest(archivePath string) (*StateManifest, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("not a valid gzip file: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading archive: %w", err)
		}

		if header.Name == "state.json" {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			var manifest StateManifest
			if err := json.Unmarshal(data, &manifest); err != nil {
				return nil, err
			}
			return &manifest, nil
		}
	}

	return nil, fmt.Errorf("state.json not found in archive")
}

// extractTarGz extracts a .tar.gz archive into the specified directory.
func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("not a valid gzip file: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Prevent path traversal
		cleanName := filepath.Clean(header.Name)
		if strings.Contains(cleanName, "..") {
			continue
		}

		target := filepath.Join(destDir, cleanName)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}

	return nil
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// copyDir recursively copies a directory tree from src to dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		return copyFile(path, targetPath)
	})
}
