package disaster

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
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
