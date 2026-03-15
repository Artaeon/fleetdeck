package disaster

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/db"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// setupCoverageEnv creates a self-contained test environment with config, DB,
// and backup directories. Caller gets a clean DB with schema tables ready.
func setupCoverageEnv(t *testing.T) (*config.Config, *db.DB, string) {
	t.Helper()

	baseDir := t.TempDir()
	backupDir := filepath.Join(baseDir, "backups")
	os.MkdirAll(backupDir, 0755)

	cfg := config.DefaultConfig()
	cfg.Server.BasePath = baseDir
	cfg.Backup.BasePath = backupDir

	configPath := filepath.Join(baseDir, "config.toml")
	cfg.Save(configPath)

	database, err := db.Open(filepath.Join(baseDir, "fleetdeck.db"))
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	return cfg, database, baseDir
}

func addProject(t *testing.T, database *db.DB, name string) *db.Project {
	t.Helper()
	p := &db.Project{
		Name:        name,
		Domain:      name + ".example.com",
		LinuxUser:   "fleetdeck-" + name,
		ProjectPath: "/opt/fleetdeck/" + name,
		Template:    "node",
		Source:      "created",
	}
	if err := database.CreateProject(p); err != nil {
		t.Fatalf("creating project %s: %v", name, err)
	}
	return p
}

func addBackupWithData(t *testing.T, database *db.DB, cfg *config.Config, project *db.Project, files map[string]string) *db.BackupRecord {
	t.Helper()
	backupDir := filepath.Join(cfg.Backup.BasePath, project.Name, "backup-latest")
	os.MkdirAll(backupDir, 0755)

	for name, content := range files {
		fpath := filepath.Join(backupDir, name)
		os.MkdirAll(filepath.Dir(fpath), 0755)
		os.WriteFile(fpath, []byte(content), 0644)
	}

	record := &db.BackupRecord{
		ProjectID: project.ID,
		Type:      "manual",
		Trigger:   "user",
		Path:      backupDir,
		SizeBytes: 4096,
	}
	if err := database.CreateBackupRecord(record); err != nil {
		t.Fatalf("creating backup record: %v", err)
	}
	return record
}

// readTarGzFile extracts and returns the content of a specific file inside a
// .tar.gz archive.
func readTarGzFile(t *testing.T, archivePath, targetName string) []byte {
	t.Helper()

	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("opening archive: %v", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reading tar: %v", err)
		}
		if hdr.Name == targetName {
			data, err := io.ReadAll(tr)
			if err != nil {
				t.Fatalf("reading tar entry %s: %v", targetName, err)
			}
			return data
		}
	}
	t.Fatalf("entry %q not found in archive", targetName)
	return nil
}

// tarGzEntryNames returns all entry names in the archive.
func tarGzEntryNames(t *testing.T, archivePath string) []string {
	t.Helper()
	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("opening archive: %v", err)
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reading tar: %v", err)
		}
		names = append(names, hdr.Name)
	}
	return names
}

// buildTestTarGz is an alias wrapper around the existing buildTarGz helper
// from disaster_test.go to avoid name collision. We just use buildTarGz
// directly since we are in the same package.

// ---------------------------------------------------------------------------
// Export: verify tarball creation, file inclusion, metadata
// ---------------------------------------------------------------------------

func TestExportStateManifestContent(t *testing.T) {
	cfg, database, baseDir := setupCoverageEnv(t)

	p := addProject(t, database, "svc-a")
	addBackupWithData(t, database, cfg, p, map[string]string{
		"db-dump.sql": "CREATE TABLE t(id int);",
	})

	outputPath := filepath.Join(baseDir, "export-manifest.tar.gz")
	if err := ExportState(cfg, database, outputPath, "4.2.1"); err != nil {
		t.Fatalf("ExportState: %v", err)
	}

	// Read state.json directly from the archive.
	manifestBytes := readTarGzFile(t, outputPath, "state.json")
	var manifest StateManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}

	if manifest.FleetDeckVersion != "4.2.1" {
		t.Errorf("version = %q, want 4.2.1", manifest.FleetDeckVersion)
	}
	if manifest.ProjectCount != 1 {
		t.Errorf("project_count = %d, want 1", manifest.ProjectCount)
	}
	if manifest.BackupCount != 1 {
		t.Errorf("backup_count = %d, want 1", manifest.BackupCount)
	}
	if manifest.ExportTimestamp == "" {
		t.Error("export_timestamp should not be empty")
	}
}

func TestExportStateIncludesBackupFiles(t *testing.T) {
	cfg, database, baseDir := setupCoverageEnv(t)

	p := addProject(t, database, "web")
	addBackupWithData(t, database, cfg, p, map[string]string{
		"data.bin":       "binary-data",
		"sub/nested.txt": "nested file",
	})

	outputPath := filepath.Join(baseDir, "export-backups.tar.gz")
	if err := ExportState(cfg, database, outputPath, "1.0.0"); err != nil {
		t.Fatalf("ExportState: %v", err)
	}

	entries := tarGzEntryNames(t, outputPath)
	entrySet := make(map[string]bool)
	for _, e := range entries {
		entrySet[e] = true
	}

	if !entrySet["backups/web/data.bin"] {
		t.Error("archive missing backups/web/data.bin")
	}
	if !entrySet["backups/web/sub/nested.txt"] {
		t.Error("archive missing backups/web/sub/nested.txt")
	}
}

func TestExportStateMultipleProjectsSomeWithoutBackups(t *testing.T) {
	cfg, database, baseDir := setupCoverageEnv(t)

	p1 := addProject(t, database, "has-backup")
	addBackupWithData(t, database, cfg, p1, map[string]string{
		"dump.sql": "data",
	})
	addProject(t, database, "no-backup") // no backup created

	outputPath := filepath.Join(baseDir, "mixed.tar.gz")
	if err := ExportState(cfg, database, outputPath, "1.0.0"); err != nil {
		t.Fatalf("ExportState: %v", err)
	}

	manifest, err := ReadStateManifest(outputPath)
	if err != nil {
		t.Fatalf("ReadStateManifest: %v", err)
	}
	if manifest.ProjectCount != 2 {
		t.Errorf("project_count = %d, want 2", manifest.ProjectCount)
	}
	if manifest.BackupCount != 1 {
		t.Errorf("backup_count = %d, want 1 (only has-backup)", manifest.BackupCount)
	}
}

func TestExportStateAlwaysIncludesDB(t *testing.T) {
	cfg, database, baseDir := setupCoverageEnv(t)

	outputPath := filepath.Join(baseDir, "db-only.tar.gz")
	if err := ExportState(cfg, database, outputPath, "1.0.0"); err != nil {
		t.Fatalf("ExportState: %v", err)
	}

	entries := tarGzEntryNames(t, outputPath)
	found := false
	for _, e := range entries {
		if e == "fleetdeck.db" {
			found = true
			break
		}
	}
	if !found {
		t.Error("archive must always include fleetdeck.db")
	}
}

func TestExportStateCreatesOutputDirectory(t *testing.T) {
	cfg, database, baseDir := setupCoverageEnv(t)

	// Output to a deeply nested non-existent directory.
	outputPath := filepath.Join(baseDir, "a", "b", "c", "export.tar.gz")
	if err := ExportState(cfg, database, outputPath, "1.0.0"); err != nil {
		t.Fatalf("ExportState should create parent dirs: %v", err)
	}
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("export file should exist")
	}
}

// ---------------------------------------------------------------------------
// Import: verify extraction, validation, error handling
// ---------------------------------------------------------------------------

func TestImportStateValidArchive(t *testing.T) {
	cfg, database, baseDir := setupCoverageEnv(t)

	p := addProject(t, database, "importable")
	addBackupWithData(t, database, cfg, p, map[string]string{
		"snapshot.sql": "SELECT 1;",
	})

	outputPath := filepath.Join(baseDir, "for-import.tar.gz")
	if err := ExportState(cfg, database, outputPath, "2.0.0"); err != nil {
		t.Fatalf("ExportState: %v", err)
	}

	importDir := t.TempDir()
	if err := ImportState(outputPath, importDir); err != nil {
		t.Fatalf("ImportState: %v", err)
	}

	// Database should be valid and contain the project.
	importedDB, err := db.Open(filepath.Join(importDir, "fleetdeck.db"))
	if err != nil {
		t.Fatalf("opening imported DB: %v", err)
	}
	defer importedDB.Close()

	projects, err := importedDB.ListProjects()
	if err != nil {
		t.Fatalf("listing projects: %v", err)
	}
	if len(projects) != 1 || projects[0].Name != "importable" {
		t.Errorf("expected [importable], got %v", projects)
	}

	// Backup files should be present.
	snapshotPath := filepath.Join(importDir, "backups", "importable", "snapshot.sql")
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("reading imported backup: %v", err)
	}
	if string(data) != "SELECT 1;" {
		t.Errorf("backup content = %q, want 'SELECT 1;'", string(data))
	}
}

func TestImportStateCreatesTargetDir(t *testing.T) {
	cfg, database, baseDir := setupCoverageEnv(t)

	outputPath := filepath.Join(baseDir, "create-target.tar.gz")
	if err := ExportState(cfg, database, outputPath, "1.0.0"); err != nil {
		t.Fatalf("ExportState: %v", err)
	}

	importDir := filepath.Join(t.TempDir(), "deep", "nested", "target")
	if err := ImportState(outputPath, importDir); err != nil {
		t.Fatalf("ImportState should create target dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(importDir, "fleetdeck.db")); os.IsNotExist(err) {
		t.Error("DB should exist in newly created target dir")
	}
}

func TestImportStateRejectsCorruptDB(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "corrupt-db.tar.gz")

	manifest := StateManifest{
		ExportTimestamp:  "2025-06-01T00:00:00Z",
		FleetDeckVersion: "1.0.0",
		ProjectCount:    0,
		BackupCount:     0,
	}
	manifestData, _ := json.Marshal(manifest)

	// fleetdeck.db is garbage, not a valid SQLite file.
	buildTarGz(t, archivePath, map[string][]byte{
		"state.json":   manifestData,
		"fleetdeck.db": []byte("this is not a sqlite database!!!"),
	})

	importDir := filepath.Join(tmpDir, "import")
	err := ImportState(archivePath, importDir)
	if err == nil {
		t.Fatal("expected error when importing archive with corrupt database")
	}
	if !strings.Contains(err.Error(), "corrupt") {
		t.Errorf("error should mention corrupt, got: %v", err)
	}

	// Corrupt DB should be cleaned up.
	if _, err := os.Stat(filepath.Join(importDir, "fleetdeck.db")); err == nil {
		t.Error("corrupt database file should be removed after failed validation")
	}
}

func TestImportStateCopiesConfig(t *testing.T) {
	cfg, database, baseDir := setupCoverageEnv(t)

	outputPath := filepath.Join(baseDir, "with-config.tar.gz")
	if err := ExportState(cfg, database, outputPath, "1.0.0"); err != nil {
		t.Fatalf("ExportState: %v", err)
	}

	importDir := t.TempDir()
	if err := ImportState(outputPath, importDir); err != nil {
		t.Fatalf("ImportState: %v", err)
	}

	configPath := filepath.Join(importDir, "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config.toml should be imported")
	}
	data, _ := os.ReadFile(configPath)
	if len(data) == 0 {
		t.Error("config.toml should not be empty")
	}
}

// ---------------------------------------------------------------------------
// Round-trip: export then import, verify data integrity
// ---------------------------------------------------------------------------

func TestRoundTripPreservesBackupContent(t *testing.T) {
	cfg, database, baseDir := setupCoverageEnv(t)

	files := map[string]string{
		"volumes/postgres/data.sql": "INSERT INTO users VALUES (1, 'alice');",
		"volumes/redis/dump.rdb":    "\x00\x01\x02\x03binary-content\xff",
		"env-snapshot.txt":          "APP_NAME=roundtrip\nSECRET=abc123\n",
	}
	p := addProject(t, database, "roundtrip-test")
	addBackupWithData(t, database, cfg, p, files)

	outputPath := filepath.Join(baseDir, "roundtrip.tar.gz")
	if err := ExportState(cfg, database, outputPath, "5.0.0"); err != nil {
		t.Fatalf("ExportState: %v", err)
	}

	importDir := t.TempDir()
	if err := ImportState(outputPath, importDir); err != nil {
		t.Fatalf("ImportState: %v", err)
	}

	// Verify every file matches exactly.
	for name, expectedContent := range files {
		importedPath := filepath.Join(importDir, "backups", "roundtrip-test", name)
		got, err := os.ReadFile(importedPath)
		if err != nil {
			t.Errorf("reading imported %s: %v", name, err)
			continue
		}
		if string(got) != expectedContent {
			t.Errorf("file %s content mismatch:\n  got:  %q\n  want: %q", name, string(got), expectedContent)
		}
	}
}

func TestRoundTripMultipleProjects(t *testing.T) {
	cfg, database, baseDir := setupCoverageEnv(t)

	projectNames := []string{"alpha", "beta", "gamma", "delta"}
	for _, name := range projectNames {
		p := addProject(t, database, name)
		addBackupWithData(t, database, cfg, p, map[string]string{
			"info.txt": "project: " + name,
		})
	}

	outputPath := filepath.Join(baseDir, "multi-roundtrip.tar.gz")
	if err := ExportState(cfg, database, outputPath, "1.0.0"); err != nil {
		t.Fatalf("ExportState: %v", err)
	}

	importDir := t.TempDir()
	if err := ImportState(outputPath, importDir); err != nil {
		t.Fatalf("ImportState: %v", err)
	}

	importedDB, err := db.Open(filepath.Join(importDir, "fleetdeck.db"))
	if err != nil {
		t.Fatalf("opening imported DB: %v", err)
	}
	defer importedDB.Close()

	projects, _ := importedDB.ListProjects()
	if len(projects) != len(projectNames) {
		t.Fatalf("expected %d projects, got %d", len(projectNames), len(projects))
	}

	nameSet := make(map[string]bool)
	for _, p := range projects {
		nameSet[p.Name] = true
	}
	for _, name := range projectNames {
		if !nameSet[name] {
			t.Errorf("project %q missing after round-trip", name)
		}
		// Verify backup content too.
		data, err := os.ReadFile(filepath.Join(importDir, "backups", name, "info.txt"))
		if err != nil {
			t.Errorf("backup for %s not found: %v", name, err)
			continue
		}
		if string(data) != "project: "+name {
			t.Errorf("backup for %s: got %q", name, string(data))
		}
	}
}

func TestRoundTripEmptyProjects(t *testing.T) {
	cfg, database, baseDir := setupCoverageEnv(t)

	// Projects with NO backups.
	addProject(t, database, "empty1")
	addProject(t, database, "empty2")

	outputPath := filepath.Join(baseDir, "empty-projects.tar.gz")
	if err := ExportState(cfg, database, outputPath, "1.0.0"); err != nil {
		t.Fatalf("ExportState: %v", err)
	}

	importDir := t.TempDir()
	if err := ImportState(outputPath, importDir); err != nil {
		t.Fatalf("ImportState: %v", err)
	}

	importedDB, err := db.Open(filepath.Join(importDir, "fleetdeck.db"))
	if err != nil {
		t.Fatalf("opening imported DB: %v", err)
	}
	defer importedDB.Close()

	projects, _ := importedDB.ListProjects()
	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}

	manifest, _ := ReadStateManifest(outputPath)
	if manifest.BackupCount != 0 {
		t.Errorf("backup_count should be 0, got %d", manifest.BackupCount)
	}
}

// ---------------------------------------------------------------------------
// Edge cases: special characters in names, large files
// ---------------------------------------------------------------------------

func TestExportProjectWithHyphenatedName(t *testing.T) {
	cfg, database, baseDir := setupCoverageEnv(t)

	p := addProject(t, database, "my-cool-app")
	addBackupWithData(t, database, cfg, p, map[string]string{
		"data.json": `{"key":"value"}`,
	})

	outputPath := filepath.Join(baseDir, "hyphenated.tar.gz")
	if err := ExportState(cfg, database, outputPath, "1.0.0"); err != nil {
		t.Fatalf("ExportState: %v", err)
	}

	entries := tarGzEntryNames(t, outputPath)
	found := false
	for _, e := range entries {
		if strings.HasPrefix(e, "backups/my-cool-app/") {
			found = true
			break
		}
	}
	if !found {
		t.Error("archive should contain backup entries for hyphenated project name")
	}
}

func TestExportLargeBackupFile(t *testing.T) {
	cfg, database, baseDir := setupCoverageEnv(t)

	// Create a 1 MB backup file.
	largeContent := strings.Repeat("X", 1024*1024)
	p := addProject(t, database, "large")
	addBackupWithData(t, database, cfg, p, map[string]string{
		"big-dump.sql": largeContent,
	})

	outputPath := filepath.Join(baseDir, "large.tar.gz")
	if err := ExportState(cfg, database, outputPath, "1.0.0"); err != nil {
		t.Fatalf("ExportState: %v", err)
	}

	// Verify the large file is in the archive and has correct content.
	data := readTarGzFile(t, outputPath, "backups/large/big-dump.sql")
	if len(data) != 1024*1024 {
		t.Errorf("large file size = %d, want %d", len(data), 1024*1024)
	}
}

func TestRoundTripLargeFile(t *testing.T) {
	cfg, database, baseDir := setupCoverageEnv(t)

	largeContent := strings.Repeat("Y", 512*1024)
	p := addProject(t, database, "big-round")
	addBackupWithData(t, database, cfg, p, map[string]string{
		"payload.bin": largeContent,
	})

	outputPath := filepath.Join(baseDir, "big-roundtrip.tar.gz")
	ExportState(cfg, database, outputPath, "1.0.0")

	importDir := t.TempDir()
	ImportState(outputPath, importDir)

	data, err := os.ReadFile(filepath.Join(importDir, "backups", "big-round", "payload.bin"))
	if err != nil {
		t.Fatalf("reading imported large file: %v", err)
	}
	if string(data) != largeContent {
		t.Errorf("large file content mismatch after round-trip: got %d bytes, want %d", len(data), len(largeContent))
	}
}

// ---------------------------------------------------------------------------
// extractTarGz: additional coverage
// ---------------------------------------------------------------------------

func TestExtractTarGzPreservesFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "perms.tar.gz")

	// Build an archive with a file that has specific permissions.
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("creating archive: %v", err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	content := []byte("executable script")
	tw.WriteHeader(&tar.Header{
		Name: "run.sh",
		Size: int64(len(content)),
		Mode: 0755,
	})
	tw.Write(content)
	tw.Close()
	gw.Close()
	f.Close()

	destDir := filepath.Join(tmpDir, "extracted")
	if err := extractTarGz(archivePath, destDir); err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}

	info, err := os.Stat(filepath.Join(destDir, "run.sh"))
	if err != nil {
		t.Fatalf("stat run.sh: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0755 {
		t.Errorf("run.sh permissions = %o, want 0755", perm)
	}
}

func TestExtractTarGzDeeplyNested(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "deep.tar.gz")

	content := []byte("deeply nested content")
	buildTarGz(t, archivePath, map[string][]byte{
		"a/b/c/d/e/file.txt": content,
	})

	destDir := filepath.Join(tmpDir, "extracted")
	if err := extractTarGz(archivePath, destDir); err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(destDir, "a", "b", "c", "d", "e", "file.txt"))
	if err != nil {
		t.Fatalf("reading deeply nested file: %v", err)
	}
	if string(data) != "deeply nested content" {
		t.Errorf("content = %q, want 'deeply nested content'", string(data))
	}
}

func TestExtractTarGzNonexistentArchive(t *testing.T) {
	err := extractTarGz("/nonexistent/archive.tar.gz", t.TempDir())
	if err == nil {
		t.Fatal("expected error for nonexistent archive")
	}
}

func TestExtractTarGzEmptyArchive(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "empty.tar.gz")
	buildTarGz(t, archivePath, map[string][]byte{})

	destDir := filepath.Join(tmpDir, "extracted")
	if err := extractTarGz(archivePath, destDir); err != nil {
		t.Fatalf("extractTarGz with empty archive should succeed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// copyFile / copyDir: additional coverage
// ---------------------------------------------------------------------------

func TestCopyFileLargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "large.bin")
	dst := filepath.Join(tmpDir, "large_copy.bin")

	// 2 MB file
	data := []byte(strings.Repeat("L", 2*1024*1024))
	os.WriteFile(src, data, 0644)

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile large: %v", err)
	}

	got, _ := os.ReadFile(dst)
	if len(got) != len(data) {
		t.Errorf("copy size = %d, want %d", len(got), len(data))
	}
}

func TestCopyDirWithManyFiles(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "many-src")
	dstDir := filepath.Join(tmpDir, "many-dst")

	os.MkdirAll(srcDir, 0755)
	for i := 0; i < 50; i++ {
		name := filepath.Join(srcDir, strings.Repeat("f", i%10+1)+".txt")
		// Use unique names to avoid overwrite
		name = filepath.Join(srcDir, "file"+strings.Repeat("0", 3)+string(rune('a'+i%26))+".txt")
		os.WriteFile(name, []byte("content"), 0644)
	}

	if err := copyDir(srcDir, dstDir); err != nil {
		t.Fatalf("copyDir: %v", err)
	}

	// Count files in destination.
	var count int
	filepath.Walk(dstDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			count++
		}
		return nil
	})

	// Count files in source.
	var srcCount int
	filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			srcCount++
		}
		return nil
	})

	if count != srcCount {
		t.Errorf("dst has %d files, src has %d", count, srcCount)
	}
}

// ---------------------------------------------------------------------------
// ReadStateManifest: valid archive
// ---------------------------------------------------------------------------

func TestReadStateManifestValidArchive(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "valid.tar.gz")

	manifest := StateManifest{
		ExportTimestamp:  "2025-12-25T12:00:00Z",
		FleetDeckVersion: "3.0.0",
		ProjectCount:    5,
		BackupCount:     3,
	}
	manifestData, _ := json.MarshalIndent(manifest, "", "  ")

	buildTarGz(t, archivePath, map[string][]byte{
		"state.json":   manifestData,
		"fleetdeck.db": []byte("db-content"),
	})

	got, err := ReadStateManifest(archivePath)
	if err != nil {
		t.Fatalf("ReadStateManifest: %v", err)
	}
	if got.FleetDeckVersion != "3.0.0" {
		t.Errorf("version = %q, want 3.0.0", got.FleetDeckVersion)
	}
	if got.ProjectCount != 5 {
		t.Errorf("project_count = %d, want 5", got.ProjectCount)
	}
	if got.BackupCount != 3 {
		t.Errorf("backup_count = %d, want 3", got.BackupCount)
	}
	if got.ExportTimestamp != "2025-12-25T12:00:00Z" {
		t.Errorf("timestamp = %q, want 2025-12-25T12:00:00Z", got.ExportTimestamp)
	}
}

// ---------------------------------------------------------------------------
// addFileToTar / addBytesToTar: indirect coverage via ExportState
// ---------------------------------------------------------------------------

func TestExportStateAlwaysIncludesConfig(t *testing.T) {
	cfg, database, baseDir := setupCoverageEnv(t)

	outputPath := filepath.Join(baseDir, "config-check.tar.gz")
	if err := ExportState(cfg, database, outputPath, "1.0.0"); err != nil {
		t.Fatalf("ExportState: %v", err)
	}

	entries := tarGzEntryNames(t, outputPath)
	found := false
	for _, e := range entries {
		if e == "config.toml" {
			found = true
			break
		}
	}
	if !found {
		t.Error("archive should include config.toml")
	}

	// Verify the config content is non-empty.
	configData := readTarGzFile(t, outputPath, "config.toml")
	if len(configData) == 0 {
		t.Error("config.toml in archive should not be empty")
	}
}
