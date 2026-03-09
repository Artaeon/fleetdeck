package backup

import (
	"os"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/db"
	"github.com/fleetdeck/fleetdeck/internal/ui"
)

func EnforceRetention(cfg *config.Config, database *db.DB, projectID string) error {
	// Enforce max manual backups
	if err := enforceMaxCount(database, projectID, "manual", cfg.Backup.MaxManualBackups); err != nil {
		return err
	}

	// Enforce max snapshots
	if err := enforceMaxCount(database, projectID, "snapshot", cfg.Backup.MaxSnapshots); err != nil {
		return err
	}

	// Enforce max age
	if cfg.Backup.MaxAgeDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -cfg.Backup.MaxAgeDays)
		expired, err := database.GetExpiredBackups(projectID, cutoff)
		if err != nil {
			return err
		}

		// Always keep at least the most recent backup of each type
		for _, b := range expired {
			count, _ := database.CountBackupsByType(projectID, b.Type)
			if count <= 1 {
				continue // never delete the last backup
			}
			deleteBackup(database, b)
		}
	}

	return nil
}

func enforceMaxCount(database *db.DB, projectID, backupType string, maxCount int) error {
	if maxCount <= 0 {
		return nil
	}

	count, err := database.CountBackupsByType(projectID, backupType)
	if err != nil {
		return err
	}

	if count <= maxCount {
		return nil
	}

	excess := count - maxCount
	oldest, err := database.GetOldestBackups(projectID, backupType, excess)
	if err != nil {
		return err
	}

	for _, b := range oldest {
		deleteBackup(database, b)
	}

	return nil
}

func deleteBackup(database *db.DB, b *db.BackupRecord) {
	if err := os.RemoveAll(b.Path); err != nil {
		ui.Warn("Could not remove backup at %s: %v", b.Path, err)
	}
	if err := database.DeleteBackupRecord(b.ID); err != nil {
		ui.Warn("Could not remove backup record %s: %v", b.ID, err)
	}
}
