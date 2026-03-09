package disaster

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/db"
)

// StateManifest describes the contents of a full state export archive.
type StateManifest struct {
	ExportTimestamp string `json:"export_timestamp"`
	FleetDeckVersion string `json:"fleetdeck_version"`
	ProjectCount    int    `json:"project_count"`
	BackupCount     int    `json:"backup_count"`
}

// ExportState creates a tar.gz archive containing the full FleetDeck state:
// the SQLite database, the current config file, the latest backup for each
// project, and a state.json manifest.
func ExportState(cfg *config.Config, database *db.DB, outputPath string, version string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("creating archive file: %w", err)
	}
	defer outFile.Close()

	gw := gzip.NewWriter(outFile)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	// 1. Create a consistent snapshot of the SQLite database (handles WAL mode)
	tmpDir, err := os.MkdirTemp("", "fleetdeck-export-*")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	snapshotPath := filepath.Join(tmpDir, "fleetdeck.db")
	if err := database.Snapshot(snapshotPath); err != nil {
		return fmt.Errorf("creating database snapshot: %w", err)
	}
	if err := addFileToTar(tw, snapshotPath, "fleetdeck.db"); err != nil {
		return fmt.Errorf("adding database to archive: %w", err)
	}

	// 2. Copy config.toml
	configPath := config.DefaultConfigPath
	if _, err := os.Stat(configPath); err == nil {
		if err := addFileToTar(tw, configPath, "config.toml"); err != nil {
			return fmt.Errorf("adding config to archive: %w", err)
		}
	} else {
		// Generate current config to include in the archive
		configData, err := marshalConfig(cfg)
		if err != nil {
			return fmt.Errorf("marshaling config: %w", err)
		}
		if err := addBytesToTar(tw, configData, "config.toml"); err != nil {
			return fmt.Errorf("adding config to archive: %w", err)
		}
	}

	// 3. Include the latest backup for each project
	projects, err := database.ListProjects()
	if err != nil {
		return fmt.Errorf("listing projects: %w", err)
	}

	backupCount := 0
	for _, p := range projects {
		backups, err := database.ListBackupRecords(p.ID, 1)
		if err != nil {
			continue
		}
		if len(backups) == 0 {
			continue
		}

		latest := backups[0]
		backupDir := latest.Path
		if _, err := os.Stat(backupDir); os.IsNotExist(err) {
			continue
		}

		// Walk the backup directory and add all files
		err = filepath.Walk(backupDir, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return nil // skip errors
			}
			if info.IsDir() {
				return nil
			}

			relPath, err := filepath.Rel(backupDir, path)
			if err != nil {
				return nil
			}

			archiveName := filepath.Join("backups", p.Name, relPath)
			return addFileToTar(tw, path, archiveName)
		})
		if err != nil {
			return fmt.Errorf("adding backup for project %s: %w", p.Name, err)
		}
		backupCount++
	}

	// 4. Write state.json manifest
	manifest := StateManifest{
		ExportTimestamp:  time.Now().UTC().Format(time.RFC3339),
		FleetDeckVersion: version,
		ProjectCount:    len(projects),
		BackupCount:     backupCount,
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state manifest: %w", err)
	}
	if err := addBytesToTar(tw, manifestData, "state.json"); err != nil {
		return fmt.Errorf("adding state manifest to archive: %w", err)
	}

	return nil
}

// addFileToTar reads a file from disk and writes it into the tar archive
// under the given archive name.
func addFileToTar(tw *tar.Writer, srcPath, archiveName string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	header := &tar.Header{
		Name:    archiveName,
		Size:    info.Size(),
		Mode:    int64(info.Mode()),
		ModTime: info.ModTime(),
	}

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	_, err = io.Copy(tw, f)
	return err
}

// addBytesToTar writes raw bytes into the tar archive under the given name.
func addBytesToTar(tw *tar.Writer, data []byte, archiveName string) error {
	header := &tar.Header{
		Name:    archiveName,
		Size:    int64(len(data)),
		Mode:    0644,
		ModTime: time.Now(),
	}

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	_, err := tw.Write(data)
	return err
}

// marshalConfig serializes the config to TOML bytes.
func marshalConfig(cfg *config.Config) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "fleetdeck-export-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	tmpPath := filepath.Join(tmpDir, "config.toml")
	if err := cfg.Save(tmpPath); err != nil {
		return nil, err
	}

	return os.ReadFile(tmpPath)
}
