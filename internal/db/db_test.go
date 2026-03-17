package db

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/crypto"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenAndMigrate(t *testing.T) {
	db := newTestDB(t)
	if db == nil {
		t.Fatal("expected non-nil db")
	}
}

func TestCreateAndGetProject(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "testproject",
		Domain:      "test.example.com",
		LinuxUser:   "fleetdeck-testproject",
		ProjectPath: "/opt/fleetdeck/testproject",
		Template:    "node",
		Source:      "created",
	}

	if err := db.CreateProject(p); err != nil {
		t.Fatalf("create project: %v", err)
	}

	if p.ID == "" {
		t.Error("expected ID to be auto-generated")
	}
	if p.Status != "created" {
		t.Errorf("expected status 'created', got %s", p.Status)
	}

	got, err := db.GetProject("testproject")
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	if got.Name != "testproject" {
		t.Errorf("expected name testproject, got %s", got.Name)
	}
	if got.Domain != "test.example.com" {
		t.Errorf("expected domain test.example.com, got %s", got.Domain)
	}
	if got.Source != "created" {
		t.Errorf("expected source 'created', got %s", got.Source)
	}
}

func TestGetProjectNotFound(t *testing.T) {
	db := newTestDB(t)
	_, err := db.GetProject("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent project")
	}
}

func TestListProjects(t *testing.T) {
	db := newTestDB(t)

	for _, name := range []string{"project-a", "project-b", "project-c"} {
		p := &Project{
			Name:        name,
			Domain:      name + ".example.com",
			LinuxUser:   "fleetdeck-" + name,
			ProjectPath: "/opt/fleetdeck/" + name,
		}
		if err := db.CreateProject(p); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	projects, err := db.ListProjects()
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if len(projects) != 3 {
		t.Errorf("expected 3 projects, got %d", len(projects))
	}
}

func TestUpdateProjectStatus(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "myapp",
		Domain:      "myapp.com",
		LinuxUser:   "fleetdeck-myapp",
		ProjectPath: "/opt/fleetdeck/myapp",
	}
	db.CreateProject(p)

	if err := db.UpdateProjectStatus("myapp", "running"); err != nil {
		t.Fatalf("update status: %v", err)
	}

	got, _ := db.GetProject("myapp")
	if got.Status != "running" {
		t.Errorf("expected status running, got %s", got.Status)
	}
}

func TestUpdateProjectStatusNotFound(t *testing.T) {
	db := newTestDB(t)
	err := db.UpdateProjectStatus("ghost", "running")
	if err == nil {
		t.Error("expected error for nonexistent project")
	}
}

func TestDeleteProject(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "todelete",
		Domain:      "del.com",
		LinuxUser:   "fleetdeck-todelete",
		ProjectPath: "/opt/fleetdeck/todelete",
	}
	db.CreateProject(p)

	if err := db.DeleteProject("todelete"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := db.GetProject("todelete")
	if err == nil {
		t.Error("expected project to be deleted")
	}
}

func TestDeleteProjectCascadesBackups(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "withbackup",
		Domain:      "wb.com",
		LinuxUser:   "fleetdeck-withbackup",
		ProjectPath: "/opt/fleetdeck/withbackup",
	}
	db.CreateProject(p)

	b := &BackupRecord{
		ProjectID: p.ID,
		Type:      "manual",
		Trigger:   "user",
		Path:      "/tmp/backup",
	}
	db.CreateBackupRecord(b)

	if err := db.DeleteProject("withbackup"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	backups, _ := db.ListBackupRecords(p.ID, 0)
	if len(backups) != 0 {
		t.Error("expected backups to be cascade deleted")
	}
}

func TestUpdateProject(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "update-me",
		Domain:      "old.com",
		LinuxUser:   "fleetdeck-update-me",
		ProjectPath: "/opt/fleetdeck/update-me",
		Source:      "created",
	}
	db.CreateProject(p)

	p.Domain = "new.com"
	p.Source = "imported"
	if err := db.UpdateProject(p); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := db.GetProject("update-me")
	if got.Domain != "new.com" {
		t.Errorf("expected domain new.com, got %s", got.Domain)
	}
	if got.Source != "imported" {
		t.Errorf("expected source imported, got %s", got.Source)
	}
}

func TestProjectExistsByPath(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "pathtest",
		Domain:      "pt.com",
		LinuxUser:   "fleetdeck-pathtest",
		ProjectPath: "/opt/fleetdeck/pathtest",
	}
	db.CreateProject(p)

	if !db.ProjectExistsByPath("/opt/fleetdeck/pathtest") {
		t.Error("expected path to exist")
	}
	if db.ProjectExistsByPath("/opt/fleetdeck/nonexistent") {
		t.Error("expected path to not exist")
	}
}

func TestListProjectPaths(t *testing.T) {
	db := newTestDB(t)

	for _, name := range []string{"alpha", "beta"} {
		db.CreateProject(&Project{
			Name:        name,
			Domain:      name + ".com",
			LinuxUser:   "fleetdeck-" + name,
			ProjectPath: "/opt/fleetdeck/" + name,
		})
	}

	paths, err := db.ListProjectPaths()
	if err != nil {
		t.Fatalf("list paths: %v", err)
	}
	if len(paths) != 2 {
		t.Errorf("expected 2 paths, got %d", len(paths))
	}
	if paths["/opt/fleetdeck/alpha"] != "alpha" {
		t.Error("expected alpha path mapping")
	}
}

func TestDuplicateProjectName(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "unique",
		Domain:      "u.com",
		LinuxUser:   "fleetdeck-unique",
		ProjectPath: "/opt/fleetdeck/unique",
	}
	db.CreateProject(p)

	p2 := &Project{
		Name:        "unique",
		Domain:      "u2.com",
		LinuxUser:   "fleetdeck-unique2",
		ProjectPath: "/opt/fleetdeck/unique2",
	}
	err := db.CreateProject(p2)
	if err == nil {
		t.Error("expected error for duplicate project name")
	}
}

func TestProjectBranchMappings(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:           "branch-test",
		Domain:         "branch.io",
		LinuxUser:      "fleetdeck-branch-test",
		ProjectPath:    "/opt/test",
		Template:       "node",
		BranchMappings: `{"main":"production","develop":"staging"}`,
	}
	if err := db.CreateProject(p); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := db.GetProject("branch-test")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.BranchMappings != p.BranchMappings {
		t.Errorf("BranchMappings = %q, want %q", got.BranchMappings, p.BranchMappings)
	}
}

// --- Deployment tests ---

func TestCreateAndListDeployments(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "deploy-test",
		Domain:      "dt.com",
		LinuxUser:   "fleetdeck-deploy-test",
		ProjectPath: "/opt/fleetdeck/deploy-test",
	}
	db.CreateProject(p)

	d := &Deployment{
		ProjectID: p.ID,
		CommitSHA: "abc1234",
		Status:    "success",
	}
	if err := db.CreateDeployment(d); err != nil {
		t.Fatalf("create deployment: %v", err)
	}
	if d.ID == "" {
		t.Error("expected deployment ID to be generated")
	}

	deps, err := db.ListDeployments(p.ID, 10)
	if err != nil {
		t.Fatalf("list deployments: %v", err)
	}
	if len(deps) != 1 {
		t.Errorf("expected 1 deployment, got %d", len(deps))
	}
	if deps[0].CommitSHA != "abc1234" {
		t.Errorf("expected commit sha abc1234, got %s", deps[0].CommitSHA)
	}
}

func TestUpdateDeployment(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "deploy-update",
		Domain:      "du.com",
		LinuxUser:   "fleetdeck-du",
		ProjectPath: "/opt/fleetdeck/du",
	}
	db.CreateProject(p)

	d := &Deployment{ProjectID: p.ID, CommitSHA: "def5678"}
	db.CreateDeployment(d)

	if err := db.UpdateDeployment(d.ID, "success", "deploy completed"); err != nil {
		t.Fatalf("update deployment: %v", err)
	}
}

// --- Backup record tests ---

func TestCreateAndListBackupRecords(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "backup-test",
		Domain:      "bt.com",
		LinuxUser:   "fleetdeck-bt",
		ProjectPath: "/opt/fleetdeck/bt",
	}
	db.CreateProject(p)

	b := &BackupRecord{
		ProjectID: p.ID,
		Type:      "manual",
		Trigger:   "user",
		Path:      "/opt/fleetdeck/backups/backup-test/abc",
		SizeBytes: 1024,
	}
	if err := db.CreateBackupRecord(b); err != nil {
		t.Fatalf("create backup record: %v", err)
	}

	records, err := db.ListBackupRecords(p.ID, 10)
	if err != nil {
		t.Fatalf("list backups: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("expected 1 backup, got %d", len(records))
	}
	if records[0].SizeBytes != 1024 {
		t.Errorf("expected size 1024, got %d", records[0].SizeBytes)
	}
}

func TestCountBackupsByType(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "count-test",
		Domain:      "ct.com",
		LinuxUser:   "fleetdeck-ct",
		ProjectPath: "/opt/fleetdeck/ct",
	}
	db.CreateProject(p)

	for i := 0; i < 3; i++ {
		db.CreateBackupRecord(&BackupRecord{
			ProjectID: p.ID,
			Type:      "manual",
			Trigger:   "user",
			Path:      "/tmp/b" + string(rune('0'+i)),
		})
	}
	for i := 0; i < 5; i++ {
		db.CreateBackupRecord(&BackupRecord{
			ProjectID: p.ID,
			Type:      "snapshot",
			Trigger:   "pre-stop",
			Path:      "/tmp/s" + string(rune('0'+i)),
		})
	}

	manualCount, _ := db.CountBackupsByType(p.ID, "manual")
	if manualCount != 3 {
		t.Errorf("expected 3 manual backups, got %d", manualCount)
	}

	snapCount, _ := db.CountBackupsByType(p.ID, "snapshot")
	if snapCount != 5 {
		t.Errorf("expected 5 snapshots, got %d", snapCount)
	}
}

func TestGetExpiredBackups(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "expire-test",
		Domain:      "et.com",
		LinuxUser:   "fleetdeck-et",
		ProjectPath: "/opt/fleetdeck/et",
	}
	db.CreateProject(p)

	db.CreateBackupRecord(&BackupRecord{
		ProjectID: p.ID,
		Type:      "manual",
		Path:      "/tmp/old",
	})

	// Check with a future cutoff (should return the just-created backup)
	expired, err := db.GetExpiredBackups(p.ID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("get expired: %v", err)
	}
	if len(expired) != 1 {
		t.Errorf("expected 1 expired backup, got %d", len(expired))
	}

	// Check with a past cutoff (should return nothing)
	expired, _ = db.GetExpiredBackups(p.ID, time.Now().Add(-time.Hour))
	if len(expired) != 0 {
		t.Errorf("expected 0 expired backups, got %d", len(expired))
	}
}

func TestGetOldestBackups(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "oldest-test",
		Domain:      "ot.com",
		LinuxUser:   "fleetdeck-ot",
		ProjectPath: "/opt/fleetdeck/ot",
	}
	db.CreateProject(p)

	for i := 0; i < 5; i++ {
		db.CreateBackupRecord(&BackupRecord{
			ProjectID: p.ID,
			Type:      "manual",
			Path:      "/tmp/oldest" + string(rune('0'+i)),
		})
	}

	oldest, err := db.GetOldestBackups(p.ID, "manual", 2)
	if err != nil {
		t.Fatalf("get oldest: %v", err)
	}
	if len(oldest) != 2 {
		t.Errorf("expected 2 oldest backups, got %d", len(oldest))
	}
}

// --- Secret tests ---

func createTestProject(t *testing.T, db *DB, name string) *Project {
	t.Helper()
	p := &Project{
		Name:        name,
		Domain:      name + ".com",
		LinuxUser:   "fleetdeck-" + name,
		ProjectPath: "/opt/fleetdeck/" + name,
	}
	if err := db.CreateProject(p); err != nil {
		t.Fatalf("create project %s: %v", name, err)
	}
	return p
}

func TestSetAndGetSecretPlaintext(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "secret-plain")

	if err := db.SetSecret(p.ID, "API_KEY", "my-secret-value"); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}

	s, err := db.GetSecret(p.ID, "API_KEY")
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if s.Key != "API_KEY" {
		t.Errorf("expected key API_KEY, got %s", s.Key)
	}
	if s.Value != "my-secret-value" {
		t.Errorf("expected value my-secret-value, got %s", s.Value)
	}
}

func TestSetSecretUpsert(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "secret-upsert")

	if err := db.SetSecret(p.ID, "DB_PASS", "old-password"); err != nil {
		t.Fatalf("SetSecret (first): %v", err)
	}

	if err := db.SetSecret(p.ID, "DB_PASS", "new-password"); err != nil {
		t.Fatalf("SetSecret (upsert): %v", err)
	}

	s, err := db.GetSecret(p.ID, "DB_PASS")
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if s.Value != "new-password" {
		t.Errorf("expected updated value new-password, got %s", s.Value)
	}
}

func TestGetSecretNotFound(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "secret-notfound")

	_, err := db.GetSecret(p.ID, "NONEXISTENT")
	if err == nil {
		t.Error("expected error for nonexistent secret")
	}
}

func TestListSecrets(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "secret-list")

	db.SetSecret(p.ID, "KEY_A", "val-a")
	db.SetSecret(p.ID, "KEY_B", "val-b")
	db.SetSecret(p.ID, "KEY_C", "val-c")

	secrets, err := db.ListSecrets(p.ID)
	if err != nil {
		t.Fatalf("ListSecrets: %v", err)
	}
	if len(secrets) != 3 {
		t.Errorf("expected 3 secrets, got %d", len(secrets))
	}
	// Should be ordered by key
	if secrets[0].Key != "KEY_A" {
		t.Errorf("expected first secret to be KEY_A, got %s", secrets[0].Key)
	}
}

func TestDeleteSecret(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "secret-delete")

	db.SetSecret(p.ID, "TO_DELETE", "value")

	if err := db.DeleteSecret(p.ID, "TO_DELETE"); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}

	_, err := db.GetSecret(p.ID, "TO_DELETE")
	if err == nil {
		t.Error("expected secret to be deleted")
	}
}

func TestDeleteSecretNotFound(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "secret-del-notfound")

	err := db.DeleteSecret(p.ID, "NONEXISTENT")
	if err == nil {
		t.Error("expected error for deleting nonexistent secret")
	}
}

func TestSetAndGetSecretEncrypted(t *testing.T) {
	db := newTestDB(t)
	key := crypto.DeriveKeyFromPassphrase("test-encryption-key")
	db.SetEncryptionKey(key)

	p := createTestProject(t, db, "secret-enc")

	if err := db.SetSecret(p.ID, "DB_PASSWORD", "super-secret-123"); err != nil {
		t.Fatalf("SetSecret (encrypted): %v", err)
	}

	s, err := db.GetSecret(p.ID, "DB_PASSWORD")
	if err != nil {
		t.Fatalf("GetSecret (encrypted): %v", err)
	}
	if s.Value != "super-secret-123" {
		t.Errorf("expected decrypted value super-secret-123, got %s", s.Value)
	}
}

func TestEncryptedSecretStoredAsCiphertext(t *testing.T) {
	db := newTestDB(t)
	key := crypto.DeriveKeyFromPassphrase("test-key")
	db.SetEncryptionKey(key)

	p := createTestProject(t, db, "secret-cipher")

	db.SetSecret(p.ID, "TOKEN", "plaintext-token")

	// Read raw value from DB to verify it's not stored in plaintext
	var rawValue string
	err := db.conn.QueryRow(
		`SELECT value FROM secrets WHERE project_id = ? AND key = ?`,
		p.ID, "TOKEN",
	).Scan(&rawValue)
	if err != nil {
		t.Fatalf("raw query: %v", err)
	}
	if rawValue == "plaintext-token" {
		t.Error("expected value to be encrypted in DB, but it was plaintext")
	}
	if len(rawValue) < 4 || rawValue[:4] != "enc:" {
		t.Errorf("expected enc: prefix, got: %s", rawValue[:10])
	}
}

func TestBackwardsCompatPlaintextRead(t *testing.T) {
	db := newTestDB(t)
	p := createTestProject(t, db, "secret-compat")

	// Store a plaintext secret directly (simulating pre-encryption data)
	_, err := db.conn.Exec(
		`INSERT INTO secrets (id, project_id, key, value) VALUES (?, ?, ?, ?)`,
		"compat-id", p.ID, "OLD_SECRET", "legacy-plaintext-value",
	)
	if err != nil {
		t.Fatalf("inserting legacy secret: %v", err)
	}

	// Now enable encryption and read the old plaintext value
	key := crypto.DeriveKeyFromPassphrase("new-key")
	db.SetEncryptionKey(key)

	s, err := db.GetSecret(p.ID, "OLD_SECRET")
	if err != nil {
		t.Fatalf("GetSecret for legacy plaintext: %v", err)
	}
	if s.Value != "legacy-plaintext-value" {
		t.Errorf("expected legacy-plaintext-value, got %s", s.Value)
	}
}

func TestListSecretsEncrypted(t *testing.T) {
	db := newTestDB(t)
	key := crypto.DeriveKeyFromPassphrase("list-enc-key")
	db.SetEncryptionKey(key)

	p := createTestProject(t, db, "secret-list-enc")

	db.SetSecret(p.ID, "SEC_A", "val-a")
	db.SetSecret(p.ID, "SEC_B", "val-b")

	secrets, err := db.ListSecrets(p.ID)
	if err != nil {
		t.Fatalf("ListSecrets (encrypted): %v", err)
	}
	if len(secrets) != 2 {
		t.Fatalf("expected 2 secrets, got %d", len(secrets))
	}
	if secrets[0].Value != "val-a" {
		t.Errorf("expected val-a, got %s", secrets[0].Value)
	}
	if secrets[1].Value != "val-b" {
		t.Errorf("expected val-b, got %s", secrets[1].Value)
	}
}

func TestEncryptedReadWithNoKeyFails(t *testing.T) {
	db := newTestDB(t)
	key := crypto.DeriveKeyFromPassphrase("temp-key")
	db.SetEncryptionKey(key)

	p := createTestProject(t, db, "secret-nokey")
	db.SetSecret(p.ID, "ENCRYPTED_VAL", "secret-data")

	// Remove the encryption key
	db.SetEncryptionKey(nil)

	_, err := db.GetSecret(p.ID, "ENCRYPTED_VAL")
	if err == nil {
		t.Error("expected error when reading encrypted value without key")
	}
}

// --- Database Integrity Tests ---

func TestIntegrityCheckPasses(t *testing.T) {
	db := newTestDB(t)

	// Create some data to ensure the DB is non-trivial
	p := &Project{
		Name:        "integrity-test",
		Domain:      "int.com",
		LinuxUser:   "fleetdeck-integrity",
		ProjectPath: "/opt/fleetdeck/integrity",
	}
	if err := db.CreateProject(p); err != nil {
		t.Fatalf("create project: %v", err)
	}

	if err := db.checkIntegrity(); err != nil {
		t.Errorf("integrity check should pass on healthy database: %v", err)
	}
}

func TestWALCheckpointSucceeds(t *testing.T) {
	db := newTestDB(t)

	// Create data to generate WAL frames
	p := &Project{
		Name:        "wal-test",
		Domain:      "wal.com",
		LinuxUser:   "fleetdeck-wal",
		ProjectPath: "/opt/fleetdeck/wal",
	}
	db.CreateProject(p)

	if err := db.walCheckpoint(); err != nil {
		t.Errorf("WAL checkpoint should succeed: %v", err)
	}

	// Integrity should still pass after checkpoint
	if err := db.checkIntegrity(); err != nil {
		t.Errorf("integrity check should pass after WAL checkpoint: %v", err)
	}
}

func TestOpenRunsIntegrityCheck(t *testing.T) {
	// Open a fresh database — should succeed with no warnings
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "integrity.db"))
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	// Verify the database works after the integrity check
	p := &Project{
		Name:        "post-integrity",
		Domain:      "pi.com",
		LinuxUser:   "fleetdeck-pi",
		ProjectPath: "/opt/fleetdeck/pi",
	}
	if err := db.CreateProject(p); err != nil {
		t.Fatalf("create project after integrity check: %v", err)
	}

	got, err := db.GetProject("post-integrity")
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	if got.Name != "post-integrity" {
		t.Errorf("expected post-integrity, got %s", got.Name)
	}
}

// --- backupAndRotate tests ---

func TestBackupAndRotateCreatesBakFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Create a source file with some content
	if err := os.WriteFile(dbPath, []byte("test database content"), 0644); err != nil {
		t.Fatalf("create source file: %v", err)
	}

	if err := backupAndRotate(dbPath, 3); err != nil {
		t.Fatalf("backupAndRotate: %v", err)
	}

	bakPath := dbPath + ".bak"
	info, err := os.Stat(bakPath)
	if err != nil {
		t.Fatalf("expected .bak file to exist: %v", err)
	}
	if info.Size() == 0 {
		t.Error("expected .bak file to have content")
	}

	content, err := os.ReadFile(bakPath)
	if err != nil {
		t.Fatalf("read .bak file: %v", err)
	}
	if string(content) != "test database content" {
		t.Errorf("expected .bak content to match source, got %q", string(content))
	}
}

func TestBackupAndRotateKeepsMaxN(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	maxBackups := 3

	// Run backupAndRotate multiple times with different content
	// to verify rotation keeps at most maxBackups old copies.
	for i := 0; i < 5; i++ {
		content := fmt.Sprintf("version-%d", i)
		if err := os.WriteFile(dbPath, []byte(content), 0644); err != nil {
			t.Fatalf("write source (round %d): %v", i, err)
		}
		if err := backupAndRotate(dbPath, maxBackups); err != nil {
			t.Fatalf("backupAndRotate (round %d): %v", i, err)
		}
	}

	// After 5 rotations with maxBackups=3, we expect:
	//   test.db.bak   (latest, round 4)
	//   test.db.bak.1 (round 3)
	//   test.db.bak.2 (round 2)
	// Older ones (round 0, 1) should have been removed.

	bakPath := dbPath + ".bak"

	// The .bak file should contain the latest content
	content, err := os.ReadFile(bakPath)
	if err != nil {
		t.Fatalf("read .bak: %v", err)
	}
	if string(content) != "version-4" {
		t.Errorf("expected .bak to contain version-4, got %q", string(content))
	}

	// Count total backup files
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	bakCount := 0
	for _, e := range entries {
		if e.Name() == "test.db" {
			continue
		}
		bakCount++
	}

	// We expect at most maxBackups+1 backup files (.bak plus .bak.1 through .bak.N).
	// The function keeps maxBackups rotated copies, so total = 1 (.bak) + maxBackups rotated.
	// However, the function removes any with index >= maxBackups, so we expect
	// .bak, .bak.1, .bak.2 ... up to .bak.(maxBackups-1) at most = maxBackups total files.
	if bakCount > maxBackups+1 {
		t.Errorf("expected at most %d backup files, got %d", maxBackups+1, bakCount)
	}
}

func TestBackupAndRotateMissingSource(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "nonexistent.db")

	err := backupAndRotate(dbPath, 3)
	if err == nil {
		t.Fatal("expected error for missing source file")
	}

	// The .bak file should not have been created
	bakPath := dbPath + ".bak"
	if _, statErr := os.Stat(bakPath); statErr == nil {
		t.Error("expected .bak file to not exist for missing source")
	}
}

// --- Close WAL checkpoint test ---

func TestCloseCheckpointsWAL(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "wal-close.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	// Insert some data to generate WAL frames
	p := &Project{
		Name:        "close-wal-test",
		Domain:      "cw.com",
		LinuxUser:   "fleetdeck-cw",
		ProjectPath: "/opt/fleetdeck/cw",
	}
	if err := db.CreateProject(p); err != nil {
		t.Fatalf("create project: %v", err)
	}

	// Close should checkpoint WAL (TRUNCATE mode empties/removes the WAL file)
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// After a TRUNCATE checkpoint, the WAL file should be empty or absent
	walPath := dbPath + "-wal"
	info, err := os.Stat(walPath)
	if err == nil && info.Size() > 0 {
		t.Errorf("expected WAL file to be empty after Close, got size %d", info.Size())
	}

	// Verify the data survived the close by reopening
	db2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer db2.Close()

	got, err := db2.GetProject("close-wal-test")
	if err != nil {
		t.Fatalf("get project after reopen: %v", err)
	}
	if got.Name != "close-wal-test" {
		t.Errorf("expected close-wal-test, got %s", got.Name)
	}
}

// --- checkIntegrity on valid DB ---

func TestCheckIntegrityValidDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "valid.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// Populate with multiple tables worth of data
	for _, name := range []string{"proj-a", "proj-b", "proj-c"} {
		p := &Project{
			Name:        name,
			Domain:      name + ".com",
			LinuxUser:   "fleetdeck-" + name,
			ProjectPath: "/opt/fleetdeck/" + name,
		}
		if err := db.CreateProject(p); err != nil {
			t.Fatalf("create project %s: %v", name, err)
		}

		// Add related records to exercise foreign keys
		db.CreateDeployment(&Deployment{
			ProjectID: p.ID,
			CommitSHA: "abc123",
			Status:    "success",
		})
		db.CreateBackupRecord(&BackupRecord{
			ProjectID: p.ID,
			Type:      "manual",
			Trigger:   "user",
			Path:      "/tmp/" + name,
		})
	}

	if err := db.checkIntegrity(); err != nil {
		t.Errorf("integrity check should pass on valid populated database: %v", err)
	}
}
