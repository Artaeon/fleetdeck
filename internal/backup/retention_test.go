package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/db"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func createProject(t *testing.T, database *db.DB, name string) *db.Project {
	t.Helper()
	p := &db.Project{
		Name:        name,
		Domain:      name + ".example.com",
		LinuxUser:   "fleetdeck-" + name,
		ProjectPath: "/opt/fleetdeck/" + name,
	}
	if err := database.CreateProject(p); err != nil {
		t.Fatalf("create project %s: %v", name, err)
	}
	return p
}

func createBackupRecord(t *testing.T, database *db.DB, projectID, backupType, trigger string) *db.BackupRecord {
	t.Helper()
	dir := t.TempDir()
	b := &db.BackupRecord{
		ProjectID: projectID,
		Type:      backupType,
		Trigger:   trigger,
		Path:      dir,
		SizeBytes: 1024,
	}
	if err := database.CreateBackupRecord(b); err != nil {
		t.Fatalf("create backup record: %v", err)
	}
	return b
}

// --- EnforceRetention tests ---

func TestEnforceRetentionMaxManualBackups(t *testing.T) {
	database := newTestDB(t)
	p := createProject(t, database, "retention-manual")

	cfg := config.DefaultConfig()
	cfg.Backup.MaxManualBackups = 2
	cfg.Backup.MaxSnapshots = 100
	cfg.Backup.MaxAgeDays = 0 // disable age enforcement

	// Create 4 manual backups
	for i := 0; i < 4; i++ {
		createBackupRecord(t, database, p.ID, "manual", "user")
		// Small delay so created_at differs
		time.Sleep(10 * time.Millisecond)
	}

	count, _ := database.CountBackupsByType(p.ID, "manual")
	if count != 4 {
		t.Fatalf("expected 4 manual backups before retention, got %d", count)
	}

	if err := EnforceRetention(cfg, database, p.ID); err != nil {
		t.Fatalf("enforce retention: %v", err)
	}

	count, _ = database.CountBackupsByType(p.ID, "manual")
	if count != 2 {
		t.Errorf("expected 2 manual backups after retention, got %d", count)
	}
}

func TestEnforceRetentionMaxSnapshots(t *testing.T) {
	database := newTestDB(t)
	p := createProject(t, database, "retention-snap")

	cfg := config.DefaultConfig()
	cfg.Backup.MaxManualBackups = 100
	cfg.Backup.MaxSnapshots = 3
	cfg.Backup.MaxAgeDays = 0

	// Create 5 snapshots
	for i := 0; i < 5; i++ {
		createBackupRecord(t, database, p.ID, "snapshot", "pre-stop")
		time.Sleep(10 * time.Millisecond)
	}

	if err := EnforceRetention(cfg, database, p.ID); err != nil {
		t.Fatalf("enforce retention: %v", err)
	}

	count, _ := database.CountBackupsByType(p.ID, "snapshot")
	if count != 3 {
		t.Errorf("expected 3 snapshots after retention, got %d", count)
	}
}

func TestEnforceRetentionNewestBackupNeverDeleted(t *testing.T) {
	database := newTestDB(t)
	p := createProject(t, database, "retention-newest")

	cfg := config.DefaultConfig()
	cfg.Backup.MaxManualBackups = 2
	cfg.Backup.MaxSnapshots = 100
	cfg.Backup.MaxAgeDays = 0

	// Create 5 manual backups — track the last two IDs
	var ids []string
	for i := 0; i < 5; i++ {
		b := createBackupRecord(t, database, p.ID, "manual", "user")
		ids = append(ids, b.ID)
		time.Sleep(10 * time.Millisecond)
	}

	if err := EnforceRetention(cfg, database, p.ID); err != nil {
		t.Fatalf("enforce retention: %v", err)
	}

	// The two newest (last created) should still exist
	remaining, _ := database.ListBackupRecords(p.ID, 0)
	remainingIDs := make(map[string]bool)
	for _, r := range remaining {
		remainingIDs[r.ID] = true
	}

	// ids[3] and ids[4] are the two newest
	if !remainingIDs[ids[3]] {
		t.Error("expected 4th backup (second newest) to survive retention")
	}
	if !remainingIDs[ids[4]] {
		t.Error("expected 5th backup (newest) to survive retention")
	}
}

func TestEnforceRetentionMaxAge(t *testing.T) {
	database := newTestDB(t)
	p := createProject(t, database, "retention-age")

	cfg := config.DefaultConfig()
	cfg.Backup.MaxManualBackups = 100 // disable count enforcement
	cfg.Backup.MaxSnapshots = 100
	cfg.Backup.MaxAgeDays = 7

	// Create 3 manual backups (they'll all be "just created" so not expired)
	for i := 0; i < 3; i++ {
		createBackupRecord(t, database, p.ID, "manual", "user")
	}

	if err := EnforceRetention(cfg, database, p.ID); err != nil {
		t.Fatalf("enforce retention: %v", err)
	}

	// None should be deleted since they're all fresh
	count, _ := database.CountBackupsByType(p.ID, "manual")
	if count != 3 {
		t.Errorf("expected 3 manual backups (none expired), got %d", count)
	}
}

func TestEnforceRetentionNoDeleteWhenUnderLimit(t *testing.T) {
	database := newTestDB(t)
	p := createProject(t, database, "retention-under")

	cfg := config.DefaultConfig()
	cfg.Backup.MaxManualBackups = 10
	cfg.Backup.MaxSnapshots = 20
	cfg.Backup.MaxAgeDays = 0

	createBackupRecord(t, database, p.ID, "manual", "user")
	createBackupRecord(t, database, p.ID, "snapshot", "pre-stop")

	if err := EnforceRetention(cfg, database, p.ID); err != nil {
		t.Fatalf("enforce retention: %v", err)
	}

	manualCount, _ := database.CountBackupsByType(p.ID, "manual")
	snapCount, _ := database.CountBackupsByType(p.ID, "snapshot")
	if manualCount != 1 {
		t.Errorf("expected 1 manual backup, got %d", manualCount)
	}
	if snapCount != 1 {
		t.Errorf("expected 1 snapshot, got %d", snapCount)
	}
}

func TestEnforceRetentionZeroMaxCountDisables(t *testing.T) {
	database := newTestDB(t)
	p := createProject(t, database, "retention-zero")

	cfg := config.DefaultConfig()
	cfg.Backup.MaxManualBackups = 0 // should disable max count
	cfg.Backup.MaxSnapshots = 0
	cfg.Backup.MaxAgeDays = 0

	for i := 0; i < 10; i++ {
		createBackupRecord(t, database, p.ID, "manual", "user")
	}

	if err := EnforceRetention(cfg, database, p.ID); err != nil {
		t.Fatalf("enforce retention: %v", err)
	}

	count, _ := database.CountBackupsByType(p.ID, "manual")
	if count != 10 {
		t.Errorf("expected all 10 backups retained when max=0 (disabled), got %d", count)
	}
}

// --- enforceMaxCount tests ---

func TestEnforceMaxCountExactlyAtLimit(t *testing.T) {
	database := newTestDB(t)
	p := createProject(t, database, "maxcount-exact")

	for i := 0; i < 5; i++ {
		createBackupRecord(t, database, p.ID, "manual", "user")
	}

	if err := enforceMaxCount(database, p.ID, "manual", 5); err != nil {
		t.Fatalf("enforce max count: %v", err)
	}

	count, _ := database.CountBackupsByType(p.ID, "manual")
	if count != 5 {
		t.Errorf("expected 5 backups at limit, got %d", count)
	}
}

// --- deleteBackup tests ---

func TestDeleteBackupRemovesFilesAndRecord(t *testing.T) {
	database := newTestDB(t)
	p := createProject(t, database, "delete-test")

	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backup-to-delete")
	os.MkdirAll(backupDir, 0700)
	os.WriteFile(filepath.Join(backupDir, "manifest.json"), []byte("{}"), 0644)

	b := &db.BackupRecord{
		ProjectID: p.ID,
		Type:      "manual",
		Trigger:   "user",
		Path:      backupDir,
		SizeBytes: 100,
	}
	if err := database.CreateBackupRecord(b); err != nil {
		t.Fatalf("create backup record: %v", err)
	}

	deleteBackup(database, b)

	// Files should be gone
	if _, err := os.Stat(backupDir); !os.IsNotExist(err) {
		t.Error("expected backup directory to be deleted")
	}

	// Record should be gone
	_, err := database.GetBackupRecord(b.ID)
	if err == nil {
		t.Error("expected backup record to be deleted from database")
	}
}

func TestDeleteBackupNonexistentPath(t *testing.T) {
	database := newTestDB(t)
	p := createProject(t, database, "delete-nopath")

	b := &db.BackupRecord{
		ProjectID: p.ID,
		Type:      "manual",
		Trigger:   "user",
		Path:      "/tmp/nonexistent-fleetdeck-backup-path",
		SizeBytes: 0,
	}
	if err := database.CreateBackupRecord(b); err != nil {
		t.Fatalf("create backup record: %v", err)
	}

	// Should not panic even if path doesn't exist
	deleteBackup(database, b)

	// Record should still be deleted
	_, err := database.GetBackupRecord(b.ID)
	if err == nil {
		t.Error("expected backup record to be deleted even when path doesn't exist")
	}
}

func TestDeleteBackupOnlyAffectsTargetRecord(t *testing.T) {
	database := newTestDB(t)
	p := createProject(t, database, "delete-isolation")

	b1 := createBackupRecord(t, database, p.ID, "manual", "user")
	b2 := createBackupRecord(t, database, p.ID, "manual", "user")

	deleteBackup(database, b1)

	// b2 should still exist
	remaining, err := database.GetBackupRecord(b2.ID)
	if err != nil {
		t.Fatalf("expected b2 to still exist: %v", err)
	}
	if remaining.ID != b2.ID {
		t.Error("b2 should not be affected by deleting b1")
	}
}
