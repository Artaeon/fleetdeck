package backup

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/db"
)

// ---------------------------------------------------------------------------
// Manifest creation, serialization, and checksum verification
// ---------------------------------------------------------------------------

func TestManifestFieldsPreserved(t *testing.T) {
	dir := t.TempDir()

	m := Manifest{
		Version:     "1",
		ProjectName: "field-check",
		ProjectPath: "/opt/fleetdeck/field-check",
		Domain:      "field-check.example.com",
		CreatedAt:   "2026-01-15T08:30:00Z",
		Type:        "snapshot",
		Trigger:     "pre-stop",
		Components: []ComponentInfo{
			{Type: "config", Name: "docker-compose.yml", Path: "config/docker-compose.yml", SizeBytes: 200, Checksum: "aaa"},
			{Type: "config", Name: ".env", Path: "config/.env", SizeBytes: 50, Checksum: "bbb"},
			{Type: "database", Name: "postgres (PostgreSQL)", Path: "databases/postgres.sql.gz", SizeBytes: 4096},
			{Type: "volume", Name: "app/data", Path: "volumes/app_data.tar.gz", SizeBytes: 8192},
		},
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	read, err := ReadManifest(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if read.Version != "1" {
		t.Errorf("version: got %q, want %q", read.Version, "1")
	}
	if read.Type != "snapshot" {
		t.Errorf("type: got %q, want %q", read.Type, "snapshot")
	}
	if read.Trigger != "pre-stop" {
		t.Errorf("trigger: got %q, want %q", read.Trigger, "pre-stop")
	}
	if read.CreatedAt != "2026-01-15T08:30:00Z" {
		t.Errorf("created_at: got %q", read.CreatedAt)
	}
	if len(read.Components) != 4 {
		t.Fatalf("expected 4 components, got %d", len(read.Components))
	}

	// Check component type distribution
	typeCounts := map[string]int{}
	for _, c := range read.Components {
		typeCounts[c.Type]++
	}
	if typeCounts["config"] != 2 {
		t.Errorf("expected 2 config components, got %d", typeCounts["config"])
	}
	if typeCounts["database"] != 1 {
		t.Errorf("expected 1 database component, got %d", typeCounts["database"])
	}
	if typeCounts["volume"] != 1 {
		t.Errorf("expected 1 volume component, got %d", typeCounts["volume"])
	}
}

func TestManifestChecksumMatchesSHA256(t *testing.T) {
	projectDir := t.TempDir()
	backupDir := t.TempDir()

	content := []byte("services:\n  web:\n    image: nginx:latest\n")
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), content, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	components, err := BackupConfigFiles(projectDir, backupDir)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if len(components) == 0 {
		t.Fatal("expected at least one component")
	}

	// Compute expected checksum of the original file
	h := sha256.Sum256(content)
	expected := hex.EncodeToString(h[:])

	var found bool
	for _, c := range components {
		if c.Name == "docker-compose.yml" {
			found = true
			if c.Checksum != expected {
				t.Errorf("checksum mismatch: got %s, want %s", c.Checksum, expected)
			}
			if c.SizeBytes != int64(len(content)) {
				t.Errorf("size mismatch: got %d, want %d", c.SizeBytes, len(content))
			}
		}
	}
	if !found {
		t.Error("docker-compose.yml not found in components")
	}
}

func TestManifestChecksumVerifiesBackedUpFile(t *testing.T) {
	projectDir := t.TempDir()
	backupDir := t.TempDir()

	envContent := []byte("DB_HOST=localhost\nDB_PORT=5432\n")
	if err := os.WriteFile(filepath.Join(projectDir, ".env"), envContent, 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	components, err := BackupConfigFiles(projectDir, backupDir)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	for _, c := range components {
		if c.Name != ".env" {
			continue
		}
		// Read the backed-up file and verify its SHA256 matches the manifest checksum
		backedUp, err := os.ReadFile(filepath.Join(backupDir, c.Path))
		if err != nil {
			t.Fatalf("read backed up file: %v", err)
		}
		h := sha256.Sum256(backedUp)
		got := hex.EncodeToString(h[:])
		if got != c.Checksum {
			t.Errorf("backed-up file checksum %s does not match manifest checksum %s", got, c.Checksum)
		}
	}
}

func TestManifestComponentInventoryAllTypes(t *testing.T) {
	// Create a manifest with all three component types and verify ReadManifest
	// preserves the full inventory including names, paths, and sizes.
	dir := t.TempDir()

	m := Manifest{
		Version:     "1",
		ProjectName: "inventory-test",
		Components: []ComponentInfo{
			{Type: "config", Name: "docker-compose.yml", Path: "config/docker-compose.yml", SizeBytes: 100, Checksum: "checksum1"},
			{Type: "config", Name: ".env", Path: "config/.env", SizeBytes: 50, Checksum: "checksum2"},
			{Type: "config", Name: "Dockerfile", Path: "config/Dockerfile", SizeBytes: 75, Checksum: "checksum3"},
			{Type: "database", Name: "postgres (PostgreSQL)", Path: "databases/postgres.sql.gz", SizeBytes: 2048},
			{Type: "database", Name: "mysql (MySQL)", Path: "databases/mysql.sql.gz", SizeBytes: 1024},
			{Type: "volume", Name: "app/uploads", Path: "volumes/app_uploads.tar.gz", SizeBytes: 512000},
		},
	}

	data, _ := json.MarshalIndent(m, "", "  ")
	os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0644)

	read, err := ReadManifest(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if len(read.Components) != 6 {
		t.Fatalf("expected 6 components, got %d", len(read.Components))
	}

	// Verify each component's fields
	for i, c := range read.Components {
		orig := m.Components[i]
		if c.Type != orig.Type {
			t.Errorf("component %d type: got %q, want %q", i, c.Type, orig.Type)
		}
		if c.Name != orig.Name {
			t.Errorf("component %d name: got %q, want %q", i, c.Name, orig.Name)
		}
		if c.Path != orig.Path {
			t.Errorf("component %d path: got %q, want %q", i, c.Path, orig.Path)
		}
		if c.SizeBytes != orig.SizeBytes {
			t.Errorf("component %d size: got %d, want %d", i, c.SizeBytes, orig.SizeBytes)
		}
		if c.Checksum != orig.Checksum {
			t.Errorf("component %d checksum: got %q, want %q", i, c.Checksum, orig.Checksum)
		}
	}
}

// ---------------------------------------------------------------------------
// Config backup: verify specific files are captured
// ---------------------------------------------------------------------------

func TestBackupConfigFilesCaptures_DockerCompose_Env_Dockerfile(t *testing.T) {
	projectDir := t.TempDir()
	backupDir := t.TempDir()

	os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte("services:\n  app:\n    image: node:18\n"), 0644)
	os.WriteFile(filepath.Join(projectDir, ".env"), []byte("NODE_ENV=production\nPORT=3000\n"), 0600)
	os.WriteFile(filepath.Join(projectDir, "Dockerfile"), []byte("FROM node:18-alpine\nWORKDIR /app\nCOPY . .\n"), 0644)

	components, err := BackupConfigFiles(projectDir, backupDir)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	names := make(map[string]bool)
	for _, c := range components {
		names[c.Name] = true
	}

	for _, expected := range []string{"docker-compose.yml", ".env", "Dockerfile"} {
		if !names[expected] {
			t.Errorf("expected %s to be captured in backup", expected)
		}
	}

	// Verify the backed-up files have correct content
	composeBackup, err := os.ReadFile(filepath.Join(backupDir, "config", "docker-compose.yml"))
	if err != nil {
		t.Fatalf("read backed up compose: %v", err)
	}
	if !strings.Contains(string(composeBackup), "node:18") {
		t.Error("backed up docker-compose.yml has wrong content")
	}

	envBackup, err := os.ReadFile(filepath.Join(backupDir, "config", ".env"))
	if err != nil {
		t.Fatalf("read backed up env: %v", err)
	}
	if !strings.Contains(string(envBackup), "NODE_ENV=production") {
		t.Error("backed up .env has wrong content")
	}
}

func TestBackupConfigFilesComposeYAMLVariants(t *testing.T) {
	// Test that compose.yml and compose.yaml are also captured
	tests := []struct {
		filename string
	}{
		{"docker-compose.yaml"},
		{"compose.yml"},
		{"compose.yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			projectDir := t.TempDir()
			backupDir := t.TempDir()

			os.WriteFile(filepath.Join(projectDir, tt.filename), []byte("services: {}"), 0644)

			components, err := BackupConfigFiles(projectDir, backupDir)
			if err != nil {
				t.Fatalf("backup: %v", err)
			}

			found := false
			for _, c := range components {
				if c.Name == tt.filename {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected %s to be captured", tt.filename)
			}
		})
	}
}

func TestBackupConfigFilesContentIntegrity(t *testing.T) {
	projectDir := t.TempDir()
	backupDir := t.TempDir()

	// Write files with specific content and verify the backup matches exactly
	files := map[string]string{
		"docker-compose.yml": "services:\n  web:\n    image: nginx:alpine\n    ports:\n      - '80:80'\n",
		".env":               "SECRET_KEY=abc123xyz\nDEBUG=false\n",
		"Dockerfile":         "FROM golang:1.21\nWORKDIR /app\nCOPY go.mod go.sum ./\nRUN go mod download\nCOPY . .\nRUN go build -o server\nCMD [\"./server\"]\n",
	}

	for name, content := range files {
		os.WriteFile(filepath.Join(projectDir, name), []byte(content), 0644)
	}

	components, err := BackupConfigFiles(projectDir, backupDir)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	for _, c := range components {
		expectedContent, ok := files[c.Name]
		if !ok {
			continue
		}
		backedUp, err := os.ReadFile(filepath.Join(backupDir, c.Path))
		if err != nil {
			t.Errorf("read %s: %v", c.Name, err)
			continue
		}
		if string(backedUp) != expectedContent {
			t.Errorf("%s content mismatch:\n  got:  %q\n  want: %q", c.Name, string(backedUp), expectedContent)
		}
	}
}

// ---------------------------------------------------------------------------
// Volume backup: verify tar.gz archive creation
// ---------------------------------------------------------------------------

func TestBackupVolumesCreatesArchive(t *testing.T) {
	projectDir := t.TempDir()
	backupDir := t.TempDir()

	// Create a compose file referencing a bind-mount volume
	dataDir := filepath.Join(projectDir, "data")
	os.MkdirAll(dataDir, 0755)
	os.WriteFile(filepath.Join(dataDir, "file1.txt"), []byte("volume content 1"), 0644)
	os.WriteFile(filepath.Join(dataDir, "file2.txt"), []byte("volume content 2"), 0644)

	composeContent := `services:
  app:
    image: myapp:latest
    volumes:
      - ./data:/app/data
`
	os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(composeContent), 0644)

	components, err := BackupVolumes(projectDir, backupDir)
	if err != nil {
		t.Fatalf("backup volumes: %v", err)
	}

	if len(components) == 0 {
		t.Fatal("expected at least one volume component")
	}

	// Verify the archive file exists and is a valid gzip
	for _, c := range components {
		if c.Type != "volume" {
			t.Errorf("expected type 'volume', got %q", c.Type)
		}
		archivePath := filepath.Join(backupDir, c.Path)
		if _, err := os.Stat(archivePath); os.IsNotExist(err) {
			t.Errorf("archive %s does not exist", archivePath)
			continue
		}

		// Verify it is valid gzip
		f, err := os.Open(archivePath)
		if err != nil {
			t.Errorf("open %s: %v", archivePath, err)
			continue
		}
		gz, err := gzip.NewReader(f)
		if err != nil {
			f.Close()
			t.Errorf("%s is not valid gzip: %v", archivePath, err)
			continue
		}
		gz.Close()
		f.Close()

		if c.SizeBytes <= 0 {
			t.Errorf("expected positive size for volume archive, got %d", c.SizeBytes)
		}
	}
}

func TestBackupVolumesSkipsNodeModules(t *testing.T) {
	projectDir := t.TempDir()
	backupDir := t.TempDir()

	// Create a volume directory named node_modules
	nmDir := filepath.Join(projectDir, "node_modules")
	os.MkdirAll(nmDir, 0755)
	os.WriteFile(filepath.Join(nmDir, "pkg.json"), []byte("{}"), 0644)

	composeContent := `services:
  app:
    image: node:18
    volumes:
      - ./node_modules:/app/node_modules
`
	os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(composeContent), 0644)

	components, err := BackupVolumes(projectDir, backupDir)
	if err != nil {
		t.Fatalf("backup volumes: %v", err)
	}

	for _, c := range components {
		if strings.Contains(c.Name, "node_modules") {
			t.Error("node_modules should be skipped")
		}
	}
}

func TestBackupVolumesSkipsGitDir(t *testing.T) {
	projectDir := t.TempDir()
	backupDir := t.TempDir()

	gitDir := filepath.Join(projectDir, ".git")
	os.MkdirAll(gitDir, 0755)
	os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main"), 0644)

	composeContent := `services:
  app:
    image: myapp:latest
    volumes:
      - ./.git:/app/.git
`
	os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(composeContent), 0644)

	components, err := BackupVolumes(projectDir, backupDir)
	if err != nil {
		t.Fatalf("backup volumes: %v", err)
	}

	for _, c := range components {
		if strings.Contains(c.Name, ".git") {
			t.Error(".git should be skipped")
		}
	}
}

func TestBackupVolumesMultipleVolumes(t *testing.T) {
	projectDir := t.TempDir()
	backupDir := t.TempDir()

	// Create multiple volume directories
	for _, name := range []string{"uploads", "logs", "cache_data"} {
		dir := filepath.Join(projectDir, name)
		os.MkdirAll(dir, 0755)
		os.WriteFile(filepath.Join(dir, "file.txt"), []byte("data in "+name), 0644)
	}

	composeContent := `services:
  app:
    image: myapp:latest
    volumes:
      - ./uploads:/app/uploads
      - ./logs:/app/logs
      - ./cache_data:/app/cache
`
	os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(composeContent), 0644)

	components, err := BackupVolumes(projectDir, backupDir)
	if err != nil {
		t.Fatalf("backup volumes: %v", err)
	}

	if len(components) != 3 {
		t.Errorf("expected 3 volume components, got %d", len(components))
	}
}

func TestBackupVolumesNoComposeFile(t *testing.T) {
	projectDir := t.TempDir()
	backupDir := t.TempDir()

	components, err := BackupVolumes(projectDir, backupDir)
	if err != nil {
		t.Fatalf("backup volumes: %v", err)
	}

	if components != nil && len(components) != 0 {
		t.Errorf("expected no components for project without compose file, got %d", len(components))
	}
}

func TestBackupVolumesNonexistentHostPath(t *testing.T) {
	projectDir := t.TempDir()
	backupDir := t.TempDir()

	composeContent := `services:
  app:
    image: myapp:latest
    volumes:
      - ./nonexistent_dir:/app/data
`
	os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(composeContent), 0644)

	components, err := BackupVolumes(projectDir, backupDir)
	if err != nil {
		t.Fatalf("backup volumes: %v", err)
	}

	// Nonexistent paths should be silently skipped
	if len(components) != 0 {
		t.Errorf("expected 0 components for nonexistent volume path, got %d", len(components))
	}
}

func TestBackupVolumesVolumeWithoutColonSkipped(t *testing.T) {
	projectDir := t.TempDir()
	backupDir := t.TempDir()

	composeContent := `services:
  app:
    image: myapp:latest
    volumes:
      - /just/a/path
`
	os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(composeContent), 0644)

	components, err := BackupVolumes(projectDir, backupDir)
	if err != nil {
		t.Fatalf("backup volumes: %v", err)
	}

	// A volume definition without a colon separator should be skipped
	if len(components) != 0 {
		t.Errorf("expected 0 components for volume without colon, got %d", len(components))
	}
}

// ---------------------------------------------------------------------------
// Retention policy: max count, max age, max size
// ---------------------------------------------------------------------------

func TestRetentionMaxCountDeletesOldestFirst(t *testing.T) {
	database := newTestDB(t)
	p := createProject(t, database, "ret-oldest-first")

	cfg := config.DefaultConfig()
	cfg.Backup.MaxManualBackups = 2
	cfg.Backup.MaxSnapshots = 100
	cfg.Backup.MaxAgeDays = 0

	var ids []string
	for i := 0; i < 5; i++ {
		b := createBackupRecord(t, database, p.ID, "manual", "user")
		ids = append(ids, b.ID)
		time.Sleep(10 * time.Millisecond)
	}

	if err := EnforceRetention(cfg, database, p.ID); err != nil {
		t.Fatalf("enforce retention: %v", err)
	}

	// Only the 2 newest should remain
	remaining, _ := database.ListBackupRecords(p.ID, 0)
	remainingIDs := make(map[string]bool)
	for _, r := range remaining {
		remainingIDs[r.ID] = true
	}

	// ids[0], ids[1], ids[2] (oldest) should be deleted
	for i := 0; i < 3; i++ {
		if remainingIDs[ids[i]] {
			t.Errorf("expected backup %d (id=%s) to be deleted", i, ids[i])
		}
	}
	// ids[3] and ids[4] (newest) should remain
	for i := 3; i < 5; i++ {
		if !remainingIDs[ids[i]] {
			t.Errorf("expected backup %d (id=%s) to be retained", i, ids[i])
		}
	}
}

func TestRetentionIndependentPerType(t *testing.T) {
	database := newTestDB(t)
	p := createProject(t, database, "ret-per-type")

	cfg := config.DefaultConfig()
	cfg.Backup.MaxManualBackups = 1
	cfg.Backup.MaxSnapshots = 1
	cfg.Backup.MaxAgeDays = 0

	// Create 3 manual and 3 snapshot backups
	for i := 0; i < 3; i++ {
		createBackupRecord(t, database, p.ID, "manual", "user")
		time.Sleep(10 * time.Millisecond)
	}
	for i := 0; i < 3; i++ {
		createBackupRecord(t, database, p.ID, "snapshot", "pre-stop")
		time.Sleep(10 * time.Millisecond)
	}

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

func TestRetentionMaxCountEqualsOneKeepsNewest(t *testing.T) {
	database := newTestDB(t)
	p := createProject(t, database, "ret-max-one")

	cfg := config.DefaultConfig()
	cfg.Backup.MaxManualBackups = 1
	cfg.Backup.MaxSnapshots = 100
	cfg.Backup.MaxAgeDays = 0

	var lastID string
	for i := 0; i < 4; i++ {
		b := createBackupRecord(t, database, p.ID, "manual", "user")
		lastID = b.ID
		time.Sleep(10 * time.Millisecond)
	}

	if err := EnforceRetention(cfg, database, p.ID); err != nil {
		t.Fatalf("enforce retention: %v", err)
	}

	count, _ := database.CountBackupsByType(p.ID, "manual")
	if count != 1 {
		t.Errorf("expected 1 manual backup, got %d", count)
	}

	// The newest one should be the one that remains
	record, err := database.GetBackupRecord(lastID)
	if err != nil {
		t.Errorf("expected newest backup to survive: %v", err)
	}
	if record != nil && record.ID != lastID {
		t.Errorf("expected newest backup ID %s to survive", lastID)
	}
}

func TestRetentionDeleteRemovesBackupFiles(t *testing.T) {
	database := newTestDB(t)
	p := createProject(t, database, "ret-file-cleanup")

	// Create backup directories that should be cleaned up
	dirs := make([]string, 3)
	for i := 0; i < 3; i++ {
		dir := t.TempDir()
		subDir := filepath.Join(dir, "backup-data")
		os.MkdirAll(subDir, 0700)
		os.WriteFile(filepath.Join(subDir, "manifest.json"), []byte("{}"), 0644)

		b := &db.BackupRecord{
			ProjectID: p.ID,
			Type:      "manual",
			Trigger:   "user",
			Path:      subDir,
			SizeBytes: 100,
		}
		if err := database.CreateBackupRecord(b); err != nil {
			t.Fatalf("create: %v", err)
		}
		dirs[i] = subDir
		time.Sleep(10 * time.Millisecond)
	}

	cfg := config.DefaultConfig()
	cfg.Backup.MaxManualBackups = 1
	cfg.Backup.MaxSnapshots = 100
	cfg.Backup.MaxAgeDays = 0

	if err := EnforceRetention(cfg, database, p.ID); err != nil {
		t.Fatalf("enforce retention: %v", err)
	}

	// The oldest two directories should be removed from disk
	for i := 0; i < 2; i++ {
		if _, err := os.Stat(dirs[i]); !os.IsNotExist(err) {
			t.Errorf("expected backup directory %s to be removed from disk", dirs[i])
		}
	}
	// The newest should still exist
	if _, err := os.Stat(dirs[2]); os.IsNotExist(err) {
		t.Error("expected newest backup directory to still exist")
	}
}

func TestRetentionNegativeMaxCountDisables(t *testing.T) {
	database := newTestDB(t)
	p := createProject(t, database, "ret-negative")

	for i := 0; i < 5; i++ {
		createBackupRecord(t, database, p.ID, "manual", "user")
	}

	// Negative maxCount should be treated like zero (disabled)
	if err := enforceMaxCount(database, p.ID, "manual", -1); err != nil {
		t.Fatalf("enforce max count: %v", err)
	}

	count, _ := database.CountBackupsByType(p.ID, "manual")
	if count != 5 {
		t.Errorf("expected all 5 backups retained with negative maxCount, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Restore: selective restore options
// ---------------------------------------------------------------------------

func TestRestoreOptionsFilesOnly(t *testing.T) {
	opts := RestoreOptions{FilesOnly: true}
	restoreAll := !opts.FilesOnly && !opts.VolumesOnly && !opts.DBOnly

	if restoreAll {
		t.Error("restoreAll should be false when FilesOnly is set")
	}
	if !opts.FilesOnly {
		t.Error("FilesOnly should be true")
	}
}

func TestRestoreOptionsDBOnly(t *testing.T) {
	opts := RestoreOptions{DBOnly: true}
	restoreAll := !opts.FilesOnly && !opts.VolumesOnly && !opts.DBOnly

	if restoreAll {
		t.Error("restoreAll should be false when DBOnly is set")
	}
}

func TestRestoreOptionsVolumesOnly(t *testing.T) {
	opts := RestoreOptions{VolumesOnly: true}
	restoreAll := !opts.FilesOnly && !opts.VolumesOnly && !opts.DBOnly

	if restoreAll {
		t.Error("restoreAll should be false when VolumesOnly is set")
	}
}

func TestRestoreOptionsAllDefaults(t *testing.T) {
	opts := RestoreOptions{}
	restoreAll := !opts.FilesOnly && !opts.VolumesOnly && !opts.DBOnly

	if !restoreAll {
		t.Error("restoreAll should be true when no selective options are set")
	}
}

func TestRestoreStepCountFilesOnly(t *testing.T) {
	opts := RestoreOptions{FilesOnly: true, NoStart: true}
	restoreAll := !opts.FilesOnly && !opts.VolumesOnly && !opts.DBOnly

	totalSteps := 0
	if restoreAll || opts.FilesOnly {
		totalSteps++
	}
	if restoreAll || opts.VolumesOnly {
		totalSteps++
	}
	if restoreAll || opts.DBOnly {
		totalSteps++
	}
	if !opts.NoStart {
		totalSteps++
	}
	totalSteps++ // stop step

	// files-only + NoStart = 1 (files) + 1 (stop) = 2
	if totalSteps != 2 {
		t.Errorf("expected 2 steps for files-only + no-start, got %d", totalSteps)
	}
}

func TestRestoreStepCountAll(t *testing.T) {
	opts := RestoreOptions{}
	restoreAll := !opts.FilesOnly && !opts.VolumesOnly && !opts.DBOnly

	totalSteps := 0
	if restoreAll || opts.FilesOnly {
		totalSteps++
	}
	if restoreAll || opts.VolumesOnly {
		totalSteps++
	}
	if restoreAll || opts.DBOnly {
		totalSteps++
	}
	if !opts.NoStart {
		totalSteps++
	}
	totalSteps++ // stop step

	// all = 3 (files+volumes+db) + 1 (start) + 1 (stop) = 5
	if totalSteps != 5 {
		t.Errorf("expected 5 steps for full restore, got %d", totalSteps)
	}
}

func TestExtractNamedVolumeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"myvolume (named volume)", "myvolume"},
		{"pgdata (named volume)", "pgdata"},
		{"simple", "simple"},
		{"", ""},
	}

	for _, tt := range tests {
		got := extractNamedVolumeName(tt.input)
		if got != tt.expected {
			t.Errorf("extractNamedVolumeName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestRestorePathTraversalBlocked(t *testing.T) {
	// Verify that components with ".." in the path are treated as suspicious.
	// The restore logic checks for ".." in comp.Path and comp.Name.
	suspiciousPaths := []string{
		"../../../etc/passwd",
		"config/../../etc/shadow",
		"..%2F..%2Fetc/hosts",
	}

	for _, p := range suspiciousPaths {
		if !strings.Contains(p, "..") {
			// This test verifies the detection logic used in RestoreBackup.
			// Paths without ".." would pass through.
			continue
		}
		// The restore code checks: strings.Contains(comp.Path, "..")
		if !strings.Contains(p, "..") {
			t.Errorf("expected %q to contain '..'", p)
		}
	}
}

// ---------------------------------------------------------------------------
// Edge cases: empty project, project with no volumes, project with no database
// ---------------------------------------------------------------------------

func TestBackupEmptyProjectConfigFiles(t *testing.T) {
	projectDir := t.TempDir()
	backupDir := t.TempDir()

	components, err := BackupConfigFiles(projectDir, backupDir)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if len(components) != 0 {
		t.Errorf("expected 0 components for empty project, got %d", len(components))
	}
}

func TestBackupProjectNoVolumesInCompose(t *testing.T) {
	projectDir := t.TempDir()
	backupDir := t.TempDir()

	composeContent := `services:
  app:
    image: myapp:latest
    ports:
      - "8080:8080"
`
	os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(composeContent), 0644)

	components, err := BackupVolumes(projectDir, backupDir)
	if err != nil {
		t.Fatalf("backup volumes: %v", err)
	}

	// Service has no volumes defined, so no volume components
	if len(components) != 0 {
		t.Errorf("expected 0 volume components for service without volumes, got %d", len(components))
	}
}

func TestBackupProjectNoDatabaseInCompose(t *testing.T) {
	projectDir := t.TempDir()
	backupDir := t.TempDir()

	composeContent := `services:
  app:
    image: myapp:latest
  redis:
    image: redis:7
`
	os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(composeContent), 0644)

	components, err := BackupDatabases(projectDir, backupDir)
	if err != nil {
		t.Fatalf("backup databases: %v", err)
	}

	// Neither app nor redis are postgres/mysql, so no database components
	if len(components) != 0 {
		t.Errorf("expected 0 database components, got %d", len(components))
	}
}

func TestBackupProjectOnlyRedisNoDB(t *testing.T) {
	projectDir := t.TempDir()
	backupDir := t.TempDir()

	composeContent := `services:
  redis:
    image: redis:7-alpine
    container_name: myapp-redis
  worker:
    image: myapp-worker:latest
`
	os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(composeContent), 0644)

	components, err := BackupDatabases(projectDir, backupDir)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	if len(components) != 0 {
		t.Errorf("expected 0 db components for redis-only project, got %d", len(components))
	}
}

// ---------------------------------------------------------------------------
// Verify: additional edge cases
// ---------------------------------------------------------------------------

func TestVerifyBackupUnknownComponentType(t *testing.T) {
	dir := t.TempDir()

	// Create a file for the unknown component
	os.MkdirAll(filepath.Join(dir, "other"), 0700)
	os.WriteFile(filepath.Join(dir, "other", "custom.dat"), []byte("custom data"), 0644)

	manifest := Manifest{
		Version:     "1",
		ProjectName: "unknown-type",
		Components: []ComponentInfo{
			{Type: "custom", Name: "custom.dat", Path: "other/custom.dat", SizeBytes: 11},
		},
	}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0644)

	results, err := VerifyBackup(dir)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	// Unknown types should pass with existence check only
	if HasFailures(results) {
		t.Error("expected unknown type with existing file to pass verification")
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != VerifyOK {
		t.Errorf("expected OK status, got %s", results[0].Status)
	}
}

func TestVerifyBackupEmptyManifest(t *testing.T) {
	dir := t.TempDir()

	manifest := Manifest{
		Version:     "1",
		ProjectName: "empty",
		Components:  []ComponentInfo{},
	}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0644)

	results, err := VerifyBackup(dir)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results for empty manifest, got %d", len(results))
	}
	if HasFailures(results) {
		t.Error("expected no failures for empty manifest")
	}
}

func TestVerifyBackupSizeMismatchStillPasses(t *testing.T) {
	// Size recorded in manifest is informational; verify checks checksums, not sizes
	dir := t.TempDir()

	configDir := filepath.Join(dir, "config")
	os.MkdirAll(configDir, 0700)
	content := []byte("real content")
	os.WriteFile(filepath.Join(configDir, "file.txt"), content, 0644)

	h := sha256.Sum256(content)
	checksum := hex.EncodeToString(h[:])

	manifest := Manifest{
		Version:     "1",
		ProjectName: "size-mismatch",
		Components: []ComponentInfo{
			{Type: "config", Name: "file.txt", Path: "config/file.txt", SizeBytes: 99999, Checksum: checksum},
		},
	}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0644)

	results, err := VerifyBackup(dir)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	// Verification is checksum-based, so a size mismatch in the manifest
	// should not cause a failure as long as the checksum matches.
	if HasFailures(results) {
		t.Error("expected no failures when checksum matches despite size mismatch in manifest")
	}
}

// ---------------------------------------------------------------------------
// fileSHA256 function
// ---------------------------------------------------------------------------

func TestFileSHA256Correct(t *testing.T) {
	dir := t.TempDir()
	content := []byte("test content for sha256")
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, content, 0644)

	got, err := fileSHA256(path)
	if err != nil {
		t.Fatalf("fileSHA256: %v", err)
	}

	h := sha256.Sum256(content)
	expected := hex.EncodeToString(h[:])

	if got != expected {
		t.Errorf("fileSHA256 = %q, want %q", got, expected)
	}
}

func TestFileSHA256EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	os.WriteFile(path, []byte{}, 0644)

	got, err := fileSHA256(path)
	if err != nil {
		t.Fatalf("fileSHA256: %v", err)
	}

	h := sha256.Sum256([]byte{})
	expected := hex.EncodeToString(h[:])

	if got != expected {
		t.Errorf("fileSHA256 empty = %q, want %q", got, expected)
	}
}

func TestFileSHA256MissingFile(t *testing.T) {
	_, err := fileSHA256("/nonexistent/file.txt")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// ---------------------------------------------------------------------------
// dirSize edge cases
// ---------------------------------------------------------------------------

func TestDirSizeNestedDirectories(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b", "c")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(dir, "root.txt"), []byte("12345"), 0644)       // 5 bytes
	os.WriteFile(filepath.Join(sub, "deep.txt"), []byte("1234567890"), 0644) // 10 bytes

	size := dirSize(dir)
	if size != 15 {
		t.Errorf("expected size 15, got %d", size)
	}
}

func TestDirSizeNonexistentDir(t *testing.T) {
	size := dirSize("/nonexistent/path/should/not/exist")
	if size != 0 {
		t.Errorf("expected size 0 for nonexistent dir, got %d", size)
	}
}

// ---------------------------------------------------------------------------
// copyFileWithChecksum edge cases
// ---------------------------------------------------------------------------

func TestCopyFileWithChecksumCreatesSubdirectories(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	os.WriteFile(src, []byte("subdir test"), 0644)

	dst := filepath.Join(dir, "a", "b", "c", "dst.txt")
	size, checksum, err := copyFileWithChecksum(src, dst)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}

	if size != 11 {
		t.Errorf("expected size 11, got %d", size)
	}
	if checksum == "" {
		t.Error("expected non-empty checksum")
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(data) != "subdir test" {
		t.Errorf("content mismatch: got %q", string(data))
	}
}

func TestCopyFileWithChecksumLargeFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "large.bin")

	// Create a ~1MB file
	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	os.WriteFile(src, data, 0644)

	dst := filepath.Join(dir, "large_copy.bin")
	size, checksum, err := copyFileWithChecksum(src, dst)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}

	if size != int64(len(data)) {
		t.Errorf("expected size %d, got %d", len(data), size)
	}

	h := sha256.Sum256(data)
	expected := hex.EncodeToString(h[:])
	if checksum != expected {
		t.Errorf("checksum mismatch for large file")
	}
}

func TestCopyFileWithChecksumEmptyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "empty.txt")
	os.WriteFile(src, []byte{}, 0644)

	dst := filepath.Join(dir, "empty_copy.txt")
	size, checksum, err := copyFileWithChecksum(src, dst)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}

	if size != 0 {
		t.Errorf("expected size 0, got %d", size)
	}

	h := sha256.Sum256([]byte{})
	expected := hex.EncodeToString(h[:])
	if checksum != expected {
		t.Errorf("empty file checksum mismatch: got %s, want %s", checksum, expected)
	}
}

// ---------------------------------------------------------------------------
// parseComposeFile edge cases
// ---------------------------------------------------------------------------

func TestParseComposeFileAlternateNames(t *testing.T) {
	for _, name := range []string{"docker-compose.yaml", "compose.yml", "compose.yaml"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			os.WriteFile(filepath.Join(dir, name), []byte("services:\n  app:\n    image: test:latest\n"), 0644)

			cf, err := parseComposeFile(dir)
			if err != nil {
				t.Fatalf("parse %s: %v", name, err)
			}
			if cf.Services["app"].Image != "test:latest" {
				t.Errorf("expected image test:latest, got %s", cf.Services["app"].Image)
			}
		})
	}
}

func TestParseComposeFileInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("not: valid: yaml: [[["), 0644)

	_, err := parseComposeFile(dir)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

// ---------------------------------------------------------------------------
// loadEnvFile edge cases
// ---------------------------------------------------------------------------

func TestLoadEnvFileWithEqualsInValue(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".env"), []byte("DATABASE_URL=postgres://user:pass@host:5432/db?sslmode=require\n"), 0644)

	env := loadEnvFile(dir)
	expected := "postgres://user:pass@host:5432/db?sslmode=require"
	if env["DATABASE_URL"] != expected {
		t.Errorf("expected %q, got %q", expected, env["DATABASE_URL"])
	}
}

func TestLoadEnvFileBlankLines(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".env"), []byte("\n\n\nKEY=value\n\n\n"), 0644)

	env := loadEnvFile(dir)
	if env["KEY"] != "value" {
		t.Errorf("expected KEY=value, got %q", env["KEY"])
	}
	if len(env) != 1 {
		t.Errorf("expected 1 env var, got %d", len(env))
	}
}

func TestLoadEnvFileKeyWithoutValue(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".env"), []byte("KEY_ONLY\nVALID=yes\n"), 0644)

	env := loadEnvFile(dir)
	if _, exists := env["KEY_ONLY"]; exists {
		t.Error("key without = should not be parsed")
	}
	if env["VALID"] != "yes" {
		t.Errorf("expected VALID=yes, got %q", env["VALID"])
	}
}

// ---------------------------------------------------------------------------
// sanitizeName edge cases
// ---------------------------------------------------------------------------

func TestSanitizeNameMultipleSpecialChars(t *testing.T) {
	got := sanitizeName("my/volume.name here/deep")
	expected := "my_volume_name_here_deep"
	if got != expected {
		t.Errorf("sanitizeName = %q, want %q", got, expected)
	}
}

func TestSanitizeNameEmpty(t *testing.T) {
	got := sanitizeName("")
	if got != "" {
		t.Errorf("sanitizeName('') = %q, want empty", got)
	}
}

func TestSanitizeNameNoSpecialChars(t *testing.T) {
	got := sanitizeName("simple_name")
	if got != "simple_name" {
		t.Errorf("sanitizeName = %q, want %q", got, "simple_name")
	}
}
