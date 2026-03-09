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

// setupTestEnv creates a temporary environment with a config, database, and
// optional project backup directory. It returns the config, opened DB, and a
// cleanup function.
func setupTestEnv(t *testing.T) (*config.Config, *db.DB, string) {
	t.Helper()

	baseDir := t.TempDir()
	backupDir := filepath.Join(baseDir, "backups")
	os.MkdirAll(backupDir, 0755)

	cfg := config.DefaultConfig()
	cfg.Server.BasePath = baseDir
	cfg.Backup.BasePath = backupDir

	// Save a config.toml so export can find it
	configPath := filepath.Join(baseDir, "config.toml")
	cfg.Save(configPath)

	database, err := db.Open(filepath.Join(baseDir, "fleetdeck.db"))
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	return cfg, database, baseDir
}

func createTestProject(t *testing.T, database *db.DB, name string) *db.Project {
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
		t.Fatalf("creating test project %s: %v", name, err)
	}
	return p
}

func createTestBackup(t *testing.T, database *db.DB, cfg *config.Config, project *db.Project) *db.BackupRecord {
	t.Helper()

	backupDir := filepath.Join(cfg.Backup.BasePath, project.Name, "test-backup")
	os.MkdirAll(backupDir, 0755)

	// Create a fake manifest in the backup dir
	manifest := map[string]interface{}{
		"version":      "1",
		"project_name": project.Name,
		"created_at":   "2025-01-01T00:00:00Z",
	}
	data, _ := json.Marshal(manifest)
	os.WriteFile(filepath.Join(backupDir, "manifest.json"), data, 0644)
	os.WriteFile(filepath.Join(backupDir, "test-data.txt"), []byte("backup data for "+project.Name), 0644)

	record := &db.BackupRecord{
		ProjectID: project.ID,
		Type:      "manual",
		Trigger:   "user",
		Path:      backupDir,
		SizeBytes: 1024,
	}
	if err := database.CreateBackupRecord(record); err != nil {
		t.Fatalf("creating backup record: %v", err)
	}

	return record
}

// listTarGzEntries returns the file names inside a .tar.gz archive.
func listTarGzEntries(t *testing.T, archivePath string) []string {
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
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reading tar entry: %v", err)
		}
		names = append(names, header.Name)
	}
	return names
}

func TestExportState(t *testing.T) {
	cfg, database, baseDir := setupTestEnv(t)

	p1 := createTestProject(t, database, "webapp")
	p2 := createTestProject(t, database, "api")
	createTestBackup(t, database, cfg, p1)
	createTestBackup(t, database, cfg, p2)

	outputPath := filepath.Join(baseDir, "exports", "test-export.tar.gz")
	if err := ExportState(cfg, database, outputPath, "1.0.0-test"); err != nil {
		t.Fatalf("ExportState: %v", err)
	}

	// Verify archive exists
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("export file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("export file is empty")
	}

	// Verify archive contents
	entries := listTarGzEntries(t, outputPath)
	entrySet := make(map[string]bool)
	for _, e := range entries {
		entrySet[e] = true
	}

	if !entrySet["fleetdeck.db"] {
		t.Error("archive missing fleetdeck.db")
	}
	if !entrySet["config.toml"] {
		t.Error("archive missing config.toml")
	}
	if !entrySet["state.json"] {
		t.Error("archive missing state.json")
	}

	// Verify backup entries exist for both projects
	hasWebappBackup := false
	hasAPIBackup := false
	for _, e := range entries {
		if len(e) > len("backups/webapp/") && e[:len("backups/webapp/")] == "backups/webapp/" {
			hasWebappBackup = true
		}
		if len(e) > len("backups/api/") && e[:len("backups/api/")] == "backups/api/" {
			hasAPIBackup = true
		}
	}
	if !hasWebappBackup {
		t.Error("archive missing backup entries for webapp")
	}
	if !hasAPIBackup {
		t.Error("archive missing backup entries for api")
	}

	// Verify state.json content
	manifest, err := ReadStateManifest(outputPath)
	if err != nil {
		t.Fatalf("ReadStateManifest: %v", err)
	}
	if manifest.ProjectCount != 2 {
		t.Errorf("expected project count 2, got %d", manifest.ProjectCount)
	}
	if manifest.BackupCount != 2 {
		t.Errorf("expected backup count 2, got %d", manifest.BackupCount)
	}
	if manifest.FleetDeckVersion != "1.0.0-test" {
		t.Errorf("expected version 1.0.0-test, got %s", manifest.FleetDeckVersion)
	}
	if manifest.ExportTimestamp == "" {
		t.Error("expected non-empty export timestamp")
	}
}

func TestImportState(t *testing.T) {
	cfg, database, baseDir := setupTestEnv(t)

	p1 := createTestProject(t, database, "myapp")
	createTestBackup(t, database, cfg, p1)

	// Export
	outputPath := filepath.Join(baseDir, "export.tar.gz")
	if err := ExportState(cfg, database, outputPath, "1.0.0"); err != nil {
		t.Fatalf("ExportState: %v", err)
	}

	// Import to a new location
	importDir := t.TempDir()
	if err := ImportState(outputPath, importDir); err != nil {
		t.Fatalf("ImportState: %v", err)
	}

	// Verify database was copied and is valid
	importedDB, err := db.Open(filepath.Join(importDir, "fleetdeck.db"))
	if err != nil {
		t.Fatalf("opening imported database: %v", err)
	}
	defer importedDB.Close()

	projects, err := importedDB.ListProjects()
	if err != nil {
		t.Fatalf("listing imported projects: %v", err)
	}
	if len(projects) != 1 {
		t.Errorf("expected 1 project, got %d", len(projects))
	}
	if len(projects) > 0 && projects[0].Name != "myapp" {
		t.Errorf("expected project name 'myapp', got %s", projects[0].Name)
	}

	// Verify backups were copied
	backupDir := filepath.Join(importDir, "backups", "myapp")
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		t.Error("backup directory was not imported")
	}

	// Verify config was copied
	configPath := filepath.Join(importDir, "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not imported")
	}
}

func TestExportImportRoundtrip(t *testing.T) {
	cfg, database, baseDir := setupTestEnv(t)

	// Create multiple projects with backups
	for _, name := range []string{"frontend", "backend", "worker"} {
		p := createTestProject(t, database, name)
		createTestBackup(t, database, cfg, p)
	}

	// Export
	outputPath := filepath.Join(baseDir, "roundtrip.tar.gz")
	if err := ExportState(cfg, database, outputPath, "2.0.0"); err != nil {
		t.Fatalf("ExportState: %v", err)
	}

	// Import to fresh location
	importDir := t.TempDir()
	if err := ImportState(outputPath, importDir); err != nil {
		t.Fatalf("ImportState: %v", err)
	}

	// Verify all projects survived the roundtrip
	importedDB, err := db.Open(filepath.Join(importDir, "fleetdeck.db"))
	if err != nil {
		t.Fatalf("opening imported DB: %v", err)
	}
	defer importedDB.Close()

	projects, err := importedDB.ListProjects()
	if err != nil {
		t.Fatalf("listing projects: %v", err)
	}
	if len(projects) != 3 {
		t.Fatalf("expected 3 projects after roundtrip, got %d", len(projects))
	}

	nameSet := make(map[string]bool)
	for _, p := range projects {
		nameSet[p.Name] = true
	}
	for _, expected := range []string{"frontend", "backend", "worker"} {
		if !nameSet[expected] {
			t.Errorf("project %s missing after roundtrip", expected)
		}
	}

	// Verify backups for each project
	for _, name := range []string{"frontend", "backend", "worker"} {
		backupPath := filepath.Join(importDir, "backups", name)
		if _, err := os.Stat(backupPath); os.IsNotExist(err) {
			t.Errorf("backup for %s missing after roundtrip", name)
		}
	}

	// Verify state manifest can be read
	manifest, err := ReadStateManifest(outputPath)
	if err != nil {
		t.Fatalf("ReadStateManifest: %v", err)
	}
	if manifest.FleetDeckVersion != "2.0.0" {
		t.Errorf("expected version 2.0.0, got %s", manifest.FleetDeckVersion)
	}
}

func TestExportEmptyDB(t *testing.T) {
	cfg, _, baseDir := setupTestEnv(t)

	// Database is empty — no projects
	database, err := db.Open(filepath.Join(baseDir, "fleetdeck.db"))
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	defer database.Close()

	outputPath := filepath.Join(baseDir, "empty-export.tar.gz")
	if err := ExportState(cfg, database, outputPath, "1.0.0"); err != nil {
		t.Fatalf("ExportState with empty DB: %v", err)
	}

	// Verify archive exists and has required entries
	entries := listTarGzEntries(t, outputPath)
	entrySet := make(map[string]bool)
	for _, e := range entries {
		entrySet[e] = true
	}

	if !entrySet["fleetdeck.db"] {
		t.Error("archive missing fleetdeck.db")
	}
	if !entrySet["state.json"] {
		t.Error("archive missing state.json")
	}

	// Verify manifest shows zero projects and backups
	manifest, err := ReadStateManifest(outputPath)
	if err != nil {
		t.Fatalf("ReadStateManifest: %v", err)
	}
	if manifest.ProjectCount != 0 {
		t.Errorf("expected 0 projects, got %d", manifest.ProjectCount)
	}
	if manifest.BackupCount != 0 {
		t.Errorf("expected 0 backups, got %d", manifest.BackupCount)
	}
}

func TestImportInvalidArchive(t *testing.T) {
	// Create a non-archive file
	tmpDir := t.TempDir()
	fakePath := filepath.Join(tmpDir, "notarchive.tar.gz")
	os.WriteFile(fakePath, []byte("this is not a tar.gz file"), 0644)

	importDir := filepath.Join(tmpDir, "import")
	err := ImportState(fakePath, importDir)
	if err == nil {
		t.Fatal("expected error when importing invalid archive")
	}
}

// buildTarGz creates a .tar.gz archive at archivePath from the supplied
// entries. Each entry is a name/content pair written as a regular file.
func buildTarGz(t *testing.T, archivePath string, entries map[string][]byte) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(archivePath), 0755); err != nil {
		t.Fatalf("creating parent dir for archive: %v", err)
	}

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("creating archive file: %v", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	for name, data := range entries {
		hdr := &tar.Header{
			Name: name,
			Size: int64(len(data)),
			Mode: 0644,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("writing tar header for %s: %v", name, err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatalf("writing tar data for %s: %v", name, err)
		}
	}
}

// --- ImportState with corrupt archives ---

func TestImportState_CorruptGzipData(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "corrupt.tar.gz")

	// Write bytes that start with a valid gzip header but contain garbage
	// after. The gzip magic number is 0x1f 0x8b.
	os.WriteFile(archivePath, []byte{0x1f, 0x8b, 0x08, 0x00, 0xff, 0xff, 0xff}, 0644)

	importDir := filepath.Join(tmpDir, "import")
	err := ImportState(archivePath, importDir)
	if err == nil {
		t.Fatal("expected error when importing archive with corrupt gzip data")
	}
}

func TestImportState_TruncatedArchive(t *testing.T) {
	tmpDir := t.TempDir()

	// Build a valid archive, then truncate it
	fullPath := filepath.Join(tmpDir, "full.tar.gz")
	manifest := StateManifest{
		ExportTimestamp:  "2025-01-01T00:00:00Z",
		FleetDeckVersion: "1.0.0",
		ProjectCount:    0,
		BackupCount:     0,
	}
	manifestData, _ := json.Marshal(manifest)
	buildTarGz(t, fullPath, map[string][]byte{
		"state.json":   manifestData,
		"fleetdeck.db": []byte("not-a-real-db"),
	})

	// Read full archive and truncate to half the bytes
	full, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("reading full archive: %v", err)
	}
	truncPath := filepath.Join(tmpDir, "trunc.tar.gz")
	os.WriteFile(truncPath, full[:len(full)/2], 0644)

	importDir := filepath.Join(tmpDir, "import")
	err = ImportState(truncPath, importDir)
	if err == nil {
		t.Fatal("expected error when importing truncated archive")
	}
}

func TestImportState_EmptyTarGz(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "empty.tar.gz")

	// Create a valid gzip wrapping an empty tar (no entries)
	buildTarGz(t, archivePath, map[string][]byte{})

	importDir := filepath.Join(tmpDir, "import")
	err := ImportState(archivePath, importDir)
	if err == nil {
		t.Fatal("expected error when importing archive without state.json")
	}
}

// --- ImportState with missing state.json ---

func TestImportState_MissingStateJSON(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "no-manifest.tar.gz")

	// Archive that has a DB but no state.json
	buildTarGz(t, archivePath, map[string][]byte{
		"fleetdeck.db": []byte("not-a-real-db"),
		"config.toml":  []byte("[server]\nport = 8080\n"),
	})

	importDir := filepath.Join(tmpDir, "import")
	err := ImportState(archivePath, importDir)
	if err == nil {
		t.Fatal("expected error when importing archive without state.json")
	}
	if !strings.Contains(err.Error(), "state manifest") {
		t.Errorf("error should mention state manifest, got: %v", err)
	}
}

func TestImportState_InvalidStateJSON(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "bad-manifest.tar.gz")

	// Archive with state.json that is not valid JSON
	buildTarGz(t, archivePath, map[string][]byte{
		"state.json":   []byte("{this is not json!!!"),
		"fleetdeck.db": []byte("data"),
	})

	importDir := filepath.Join(tmpDir, "import")
	err := ImportState(archivePath, importDir)
	if err == nil {
		t.Fatal("expected error when state.json contains invalid JSON")
	}
}

func TestImportState_EmptyTimestampInManifest(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "empty-ts.tar.gz")

	// state.json with empty export_timestamp should be rejected
	manifest := StateManifest{
		ExportTimestamp:  "",
		FleetDeckVersion: "1.0.0",
		ProjectCount:    0,
		BackupCount:     0,
	}
	manifestData, _ := json.Marshal(manifest)
	buildTarGz(t, archivePath, map[string][]byte{
		"state.json":   manifestData,
		"fleetdeck.db": []byte("data"),
	})

	importDir := filepath.Join(tmpDir, "import")
	err := ImportState(archivePath, importDir)
	if err == nil {
		t.Fatal("expected error when export_timestamp is empty")
	}
	if !strings.Contains(err.Error(), "missing export_timestamp") {
		t.Errorf("error should mention missing export_timestamp, got: %v", err)
	}
}

func TestImportState_MissingDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "no-db.tar.gz")

	manifest := StateManifest{
		ExportTimestamp:  "2025-01-01T00:00:00Z",
		FleetDeckVersion: "1.0.0",
		ProjectCount:    0,
		BackupCount:     0,
	}
	manifestData, _ := json.Marshal(manifest)
	buildTarGz(t, archivePath, map[string][]byte{
		"state.json": manifestData,
	})

	importDir := filepath.Join(tmpDir, "import")
	err := ImportState(archivePath, importDir)
	if err == nil {
		t.Fatal("expected error when archive is missing fleetdeck.db")
	}
	if !strings.Contains(err.Error(), "fleetdeck.db") {
		t.Errorf("error should mention fleetdeck.db, got: %v", err)
	}
}

func TestImportState_NonexistentArchive(t *testing.T) {
	importDir := t.TempDir()
	err := ImportState("/nonexistent/path/archive.tar.gz", importDir)
	if err == nil {
		t.Fatal("expected error when archive file does not exist")
	}
}

// --- ReadStateManifest edge cases ---

func TestReadStateManifest_NonexistentFile(t *testing.T) {
	_, err := ReadStateManifest("/nonexistent/archive.tar.gz")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestReadStateManifest_NotGzip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "plain.tar.gz")
	os.WriteFile(path, []byte("plain text, not gzip"), 0644)

	_, err := ReadStateManifest(path)
	if err == nil {
		t.Fatal("expected error for non-gzip file")
	}
}

func TestReadStateManifest_GzipButNoStateJSON(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "no-state.tar.gz")

	buildTarGz(t, archivePath, map[string][]byte{
		"other.txt": []byte("hello"),
	})

	_, err := ReadStateManifest(archivePath)
	if err == nil {
		t.Fatal("expected error when state.json is not in archive")
	}
	if !strings.Contains(err.Error(), "state.json not found") {
		t.Errorf("error should mention state.json not found, got: %v", err)
	}
}

func TestReadStateManifest_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "bad-json.tar.gz")

	buildTarGz(t, archivePath, map[string][]byte{
		"state.json": []byte("NOT JSON AT ALL"),
	})

	_, err := ReadStateManifest(archivePath)
	if err == nil {
		t.Fatal("expected error when state.json is not valid JSON")
	}
}

func TestReadStateManifest_EmptyArchive(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "empty.tar.gz")

	buildTarGz(t, archivePath, map[string][]byte{})

	_, err := ReadStateManifest(archivePath)
	if err == nil {
		t.Fatal("expected error when archive has no entries")
	}
}

// --- extractTarGz path traversal protection ---

func TestExtractTarGz_PathTraversalSkipped(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "traversal.tar.gz")

	// Manually craft an archive with a path-traversal entry
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("creating archive: %v", err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	// Legitimate entry
	safeContent := []byte("safe file content")
	tw.WriteHeader(&tar.Header{Name: "safe.txt", Size: int64(len(safeContent)), Mode: 0644})
	tw.Write(safeContent)

	// Path traversal entry with ../
	evilContent := []byte("evil payload")
	tw.WriteHeader(&tar.Header{Name: "../../../etc/evil.txt", Size: int64(len(evilContent)), Mode: 0644})
	tw.Write(evilContent)

	// Another traversal variant: nested with ..
	evil2 := []byte("evil2")
	tw.WriteHeader(&tar.Header{Name: "subdir/../../outside.txt", Size: int64(len(evil2)), Mode: 0644})
	tw.Write(evil2)

	tw.Close()
	gw.Close()
	f.Close()

	destDir := filepath.Join(tmpDir, "extracted")
	if err := extractTarGz(archivePath, destDir); err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}

	// safe.txt should exist
	if _, err := os.Stat(filepath.Join(destDir, "safe.txt")); os.IsNotExist(err) {
		t.Error("safe.txt should have been extracted")
	}

	// Traversal targets should NOT exist anywhere outside destDir
	if _, err := os.Stat(filepath.Join(tmpDir, "etc", "evil.txt")); err == nil {
		t.Error("path traversal entry ../../../etc/evil.txt should have been skipped")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "outside.txt")); err == nil {
		t.Error("path traversal entry subdir/../../outside.txt should have been skipped")
	}

	// Also verify they don't exist inside destDir under mangled names
	if _, err := os.Stat(filepath.Join(destDir, "etc", "evil.txt")); err == nil {
		t.Error("traversal entry should not be written inside destDir either")
	}
}

func TestExtractTarGz_DirectoriesCreated(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "dirs.tar.gz")

	// Build archive with explicit directory entry and nested file
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("creating archive: %v", err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	// Directory entry
	tw.WriteHeader(&tar.Header{Name: "mydir/", Typeflag: tar.TypeDir, Mode: 0755})

	// File inside that directory
	content := []byte("nested content")
	tw.WriteHeader(&tar.Header{Name: "mydir/file.txt", Size: int64(len(content)), Mode: 0644})
	tw.Write(content)

	tw.Close()
	gw.Close()
	f.Close()

	destDir := filepath.Join(tmpDir, "extracted")
	if err := extractTarGz(archivePath, destDir); err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}

	info, err := os.Stat(filepath.Join(destDir, "mydir"))
	if err != nil {
		t.Fatalf("mydir should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("mydir should be a directory")
	}

	data, err := os.ReadFile(filepath.Join(destDir, "mydir", "file.txt"))
	if err != nil {
		t.Fatalf("reading nested file: %v", err)
	}
	if string(data) != "nested content" {
		t.Errorf("expected 'nested content', got %q", string(data))
	}
}

// --- copyFile and copyDir edge cases ---

func TestCopyFile_Basic(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	dstPath := filepath.Join(tmpDir, "dest.txt")

	content := []byte("hello, copy test")
	os.WriteFile(srcPath, content, 0644)

	if err := copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("reading dest: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("expected %q, got %q", content, got)
	}
}

func TestCopyFile_CreatesParentDirs(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	dstPath := filepath.Join(tmpDir, "a", "b", "c", "dest.txt")

	os.WriteFile(srcPath, []byte("deep copy"), 0644)

	if err := copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("copyFile with nested dest: %v", err)
	}

	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("reading deeply nested dest: %v", err)
	}
	if string(got) != "deep copy" {
		t.Errorf("expected 'deep copy', got %q", string(got))
	}
}

func TestCopyFile_NonexistentSource(t *testing.T) {
	tmpDir := t.TempDir()
	err := copyFile(filepath.Join(tmpDir, "nope.txt"), filepath.Join(tmpDir, "dst.txt"))
	if err == nil {
		t.Fatal("expected error when source does not exist")
	}
}

func TestCopyFile_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "empty.txt")
	dstPath := filepath.Join(tmpDir, "empty_copy.txt")

	os.WriteFile(srcPath, []byte{}, 0644)

	if err := copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("copyFile empty file: %v", err)
	}

	info, err := os.Stat(dstPath)
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("expected empty file, got size %d", info.Size())
	}
}

func TestCopyDir_Basic(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	// Build a small directory tree
	os.MkdirAll(filepath.Join(srcDir, "sub1"), 0755)
	os.MkdirAll(filepath.Join(srcDir, "sub2", "nested"), 0755)
	os.WriteFile(filepath.Join(srcDir, "root.txt"), []byte("root"), 0644)
	os.WriteFile(filepath.Join(srcDir, "sub1", "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(srcDir, "sub2", "nested", "b.txt"), []byte("b"), 0644)

	if err := copyDir(srcDir, dstDir); err != nil {
		t.Fatalf("copyDir: %v", err)
	}

	// Verify all files were copied with correct contents
	cases := []struct {
		rel     string
		content string
	}{
		{"root.txt", "root"},
		{filepath.Join("sub1", "a.txt"), "a"},
		{filepath.Join("sub2", "nested", "b.txt"), "b"},
	}
	for _, tc := range cases {
		got, err := os.ReadFile(filepath.Join(dstDir, tc.rel))
		if err != nil {
			t.Errorf("reading %s: %v", tc.rel, err)
			continue
		}
		if string(got) != tc.content {
			t.Errorf("%s: expected %q, got %q", tc.rel, tc.content, string(got))
		}
	}
}

func TestCopyDir_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "empty-src")
	dstDir := filepath.Join(tmpDir, "empty-dst")
	os.MkdirAll(srcDir, 0755)

	if err := copyDir(srcDir, dstDir); err != nil {
		t.Fatalf("copyDir on empty dir: %v", err)
	}

	info, err := os.Stat(dstDir)
	if err != nil {
		t.Fatalf("dstDir should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("dstDir should be a directory")
	}
}

func TestCopyDir_NonexistentSource(t *testing.T) {
	tmpDir := t.TempDir()
	err := copyDir(filepath.Join(tmpDir, "nope"), filepath.Join(tmpDir, "dst"))
	if err == nil {
		t.Fatal("expected error when source dir does not exist")
	}
}

// --- Full round-trip: export then import and verify all data matches ---

func TestRoundTrip_DataIntegrity(t *testing.T) {
	cfg, database, baseDir := setupTestEnv(t)

	// Create projects with meaningful backup data
	type projectDef struct {
		name       string
		backupData string
	}
	defs := []projectDef{
		{"webui", "webui backup payload 12345"},
		{"auth-svc", "auth service backup with special chars: <>&"},
		{"db-migrator", "binary-ish: \x00\x01\x02\x03"},
	}

	for _, d := range defs {
		p := createTestProject(t, database, d.name)

		// Write custom backup data so we can verify contents after round-trip
		backupDir := filepath.Join(cfg.Backup.BasePath, d.name, "latest")
		os.MkdirAll(backupDir, 0755)
		os.WriteFile(filepath.Join(backupDir, "data.bin"), []byte(d.backupData), 0644)

		record := &db.BackupRecord{
			ProjectID: p.ID,
			Type:      "manual",
			Trigger:   "user",
			Path:      backupDir,
			SizeBytes: int64(len(d.backupData)),
		}
		if err := database.CreateBackupRecord(record); err != nil {
			t.Fatalf("creating backup record for %s: %v", d.name, err)
		}
	}

	// Export
	outputPath := filepath.Join(baseDir, "integrity.tar.gz")
	if err := ExportState(cfg, database, outputPath, "3.5.0"); err != nil {
		t.Fatalf("ExportState: %v", err)
	}

	// Import to a completely new directory
	importDir := t.TempDir()
	if err := ImportState(outputPath, importDir); err != nil {
		t.Fatalf("ImportState: %v", err)
	}

	// 1. Verify manifest
	manifest, err := ReadStateManifest(outputPath)
	if err != nil {
		t.Fatalf("ReadStateManifest: %v", err)
	}
	if manifest.ProjectCount != 3 {
		t.Errorf("manifest project count: expected 3, got %d", manifest.ProjectCount)
	}
	if manifest.BackupCount != 3 {
		t.Errorf("manifest backup count: expected 3, got %d", manifest.BackupCount)
	}
	if manifest.FleetDeckVersion != "3.5.0" {
		t.Errorf("manifest version: expected 3.5.0, got %s", manifest.FleetDeckVersion)
	}

	// 2. Verify database contents match
	importedDB, err := db.Open(filepath.Join(importDir, "fleetdeck.db"))
	if err != nil {
		t.Fatalf("opening imported DB: %v", err)
	}
	defer importedDB.Close()

	projects, err := importedDB.ListProjects()
	if err != nil {
		t.Fatalf("listing imported projects: %v", err)
	}
	if len(projects) != 3 {
		t.Fatalf("expected 3 projects, got %d", len(projects))
	}

	projectsByName := make(map[string]*db.Project)
	for _, p := range projects {
		projectsByName[p.Name] = p
	}

	for _, d := range defs {
		p, ok := projectsByName[d.name]
		if !ok {
			t.Errorf("project %s missing after round-trip", d.name)
			continue
		}
		expectedDomain := d.name + ".example.com"
		if p.Domain != expectedDomain {
			t.Errorf("project %s: domain expected %q, got %q", d.name, expectedDomain, p.Domain)
		}
	}

	// 3. Verify backup file contents match exactly
	for _, d := range defs {
		backupFile := filepath.Join(importDir, "backups", d.name, "data.bin")
		got, err := os.ReadFile(backupFile)
		if err != nil {
			t.Errorf("reading backup for %s: %v", d.name, err)
			continue
		}
		if string(got) != d.backupData {
			t.Errorf("backup data for %s: expected %q, got %q", d.name, d.backupData, string(got))
		}
	}

	// 4. Verify config was preserved
	if _, err := os.Stat(filepath.Join(importDir, "config.toml")); os.IsNotExist(err) {
		t.Error("config.toml missing after round-trip")
	}
}

func TestRoundTrip_NoBackups(t *testing.T) {
	cfg, database, baseDir := setupTestEnv(t)

	// Projects with no backups
	createTestProject(t, database, "lonely-app")

	outputPath := filepath.Join(baseDir, "no-backups.tar.gz")
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

	projects, err := importedDB.ListProjects()
	if err != nil {
		t.Fatalf("listing projects: %v", err)
	}
	if len(projects) != 1 || projects[0].Name != "lonely-app" {
		t.Errorf("expected [lonely-app], got %v", projects)
	}

	// backups directory should not exist (no backups to import)
	manifest, err := ReadStateManifest(outputPath)
	if err != nil {
		t.Fatalf("ReadStateManifest: %v", err)
	}
	if manifest.BackupCount != 0 {
		t.Errorf("expected 0 backups, got %d", manifest.BackupCount)
	}
}
