package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/db"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/google/uuid"
)

type Manifest struct {
	Version     string          `json:"version"`
	ProjectName string          `json:"project_name"`
	ProjectPath string          `json:"project_path"`
	Domain      string          `json:"domain"`
	CreatedAt   string          `json:"created_at"`
	Type        string          `json:"type"`
	Trigger     string          `json:"trigger"`
	Components  []ComponentInfo `json:"components"`
}

type ComponentInfo struct {
	Type      string `json:"type"` // "config", "volume", "database"
	Name      string `json:"name"`
	Path      string `json:"path"` // relative path within backup
	SizeBytes int64  `json:"size_bytes"`
	Checksum  string `json:"checksum"` // SHA256
}

type Options struct {
	SkipDB      bool
	SkipVolumes bool
}

func CreateBackup(cfg *config.Config, database *db.DB, project *db.Project, backupType, trigger string, opts Options) (*db.BackupRecord, error) {
	backupID := uuid.New().String()
	backupDir := filepath.Join(cfg.Backup.BasePath, project.Name, backupID)

	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return nil, fmt.Errorf("creating backup directory: %w", err)
	}

	manifest := Manifest{
		Version:     "1",
		ProjectName: project.Name,
		ProjectPath: project.ProjectPath,
		Domain:      project.Domain,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		Type:        backupType,
		Trigger:     trigger,
	}

	totalSteps := 3
	if opts.SkipDB {
		totalSteps--
	}
	if opts.SkipVolumes {
		totalSteps--
	}
	step := 0

	// Step: Backup config files
	step++
	ui.Step(step, totalSteps, "Backing up configuration files...")
	configComponents, err := BackupConfigFiles(project.ProjectPath, backupDir)
	if err != nil {
		return nil, fmt.Errorf("backing up config files: %w", err)
	}
	manifest.Components = append(manifest.Components, configComponents...)
	ui.Success("Configuration files backed up (%d files)", len(configComponents))

	// Step: Dump databases
	if !opts.SkipDB {
		step++
		ui.Step(step, totalSteps, "Dumping databases...")
		dbComponents, err := BackupDatabases(project.ProjectPath, backupDir)
		if err != nil {
			os.RemoveAll(backupDir)
			return nil, fmt.Errorf("database dump failed: %w", err)
		} else if len(dbComponents) > 0 {
			manifest.Components = append(manifest.Components, dbComponents...)
			ui.Success("Database dumps created (%d databases)", len(dbComponents))
		} else {
			ui.Info("No databases detected to dump")
		}
	}

	// Step: Backup volumes
	if !opts.SkipVolumes {
		step++
		ui.Step(step, totalSteps, "Backing up volumes...")
		volComponents, err := BackupVolumes(project.ProjectPath, backupDir)
		if err != nil {
			os.RemoveAll(backupDir)
			return nil, fmt.Errorf("volume backup failed: %w", err)
		} else if len(volComponents) > 0 {
			manifest.Components = append(manifest.Components, volComponents...)
			ui.Success("Volumes backed up (%d volumes)", len(volComponents))
		} else {
			ui.Info("No volumes detected to backup")
		}
	}

	// Write manifest
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, "manifest.json"), manifestData, 0644); err != nil {
		return nil, fmt.Errorf("writing manifest: %w", err)
	}

	// Calculate total size
	totalSize := dirSize(backupDir)

	// Record in database
	record := &db.BackupRecord{
		ID:        backupID,
		ProjectID: project.ID,
		Type:      backupType,
		Trigger:   trigger,
		Path:      backupDir,
		SizeBytes: totalSize,
	}
	if err := database.CreateBackupRecord(record); err != nil {
		// Database record failed — clean up orphaned backup directory
		ui.Warn("Could not save backup record to database: %v", err)
		ui.Warn("Cleaning up orphaned backup directory...")
		os.RemoveAll(backupDir)
		return nil, fmt.Errorf("saving backup record: %w", err)
	}

	return record, nil
}

func dirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		size += info.Size()
		return nil
	})
	return size
}

func copyFileWithChecksum(src, dst string) (int64, string, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, "", err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return 0, "", err
	}

	out, err := os.Create(dst)
	if err != nil {
		return 0, "", err
	}
	defer out.Close()

	hasher := sha256.New()
	writer := io.MultiWriter(out, hasher)
	size, err := io.Copy(writer, in)
	if err != nil {
		return 0, "", err
	}

	return size, hex.EncodeToString(hasher.Sum(nil)), nil
}

func FormatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
