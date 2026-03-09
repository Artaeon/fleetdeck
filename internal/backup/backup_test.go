package backup

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFileWithChecksum(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "source.txt")
	content := "hello fleetdeck"
	os.WriteFile(src, []byte(content), 0644)

	dst := filepath.Join(dir, "dest.txt")
	size, checksum, err := copyFileWithChecksum(src, dst)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}

	if size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), size)
	}
	if checksum == "" {
		t.Error("expected non-empty checksum")
	}

	// Verify dest content
	data, _ := os.ReadFile(dst)
	if string(data) != content {
		t.Errorf("content mismatch: got %q", string(data))
	}

	// Copy same file again, checksum should be identical
	_, checksum2, _ := copyFileWithChecksum(src, filepath.Join(dir, "dest2.txt"))
	if checksum != checksum2 {
		t.Error("checksums should be identical for same content")
	}
}

func TestCopyFileWithChecksumMissingSrc(t *testing.T) {
	dir := t.TempDir()
	_, _, err := copyFileWithChecksum("/nonexistent", filepath.Join(dir, "dst"))
	if err == nil {
		t.Error("expected error for missing source")
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{5368709120, "5.0 GB"},
	}

	for _, tt := range tests {
		got := FormatSize(tt.bytes)
		if got != tt.expected {
			t.Errorf("FormatSize(%d) = %q, want %q", tt.bytes, got, tt.expected)
		}
	}
}

func TestDirSize(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("world!"), 0644)

	size := dirSize(dir)
	if size != 11 {
		t.Errorf("expected size 11, got %d", size)
	}
}

func TestDirSizeEmpty(t *testing.T) {
	dir := t.TempDir()
	size := dirSize(dir)
	if size != 0 {
		t.Errorf("expected size 0, got %d", size)
	}
}

func TestBackupConfigFiles(t *testing.T) {
	projectDir := t.TempDir()
	backupDir := t.TempDir()

	// Create some project files
	os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte("services: {}"), 0644)
	os.WriteFile(filepath.Join(projectDir, ".env"), []byte("SECRET=abc"), 0600)
	os.WriteFile(filepath.Join(projectDir, "Dockerfile"), []byte("FROM alpine"), 0644)

	components, err := BackupConfigFiles(projectDir, backupDir)
	if err != nil {
		t.Fatalf("backup config files: %v", err)
	}

	if len(components) < 3 {
		t.Errorf("expected at least 3 components, got %d", len(components))
	}

	// Verify files were copied
	for _, c := range components {
		fullPath := filepath.Join(backupDir, c.Path)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			t.Errorf("backed up file %s does not exist", fullPath)
		}
		if c.Checksum == "" {
			t.Errorf("missing checksum for %s", c.Name)
		}
		if c.Type != "config" {
			t.Errorf("expected type 'config', got %s", c.Type)
		}
	}
}

func TestBackupConfigFilesWithWorkflows(t *testing.T) {
	projectDir := t.TempDir()
	backupDir := t.TempDir()

	os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte("services: {}"), 0644)
	workflowDir := filepath.Join(projectDir, ".github", "workflows")
	os.MkdirAll(workflowDir, 0755)
	os.WriteFile(filepath.Join(workflowDir, "deploy.yml"), []byte("name: Deploy"), 0644)

	components, err := BackupConfigFiles(projectDir, backupDir)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	hasWorkflow := false
	for _, c := range components {
		if c.Name == ".github/workflows/deploy.yml" {
			hasWorkflow = true
			break
		}
	}
	if !hasWorkflow {
		t.Error("expected workflow file to be backed up")
	}
}

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".env"), []byte(`
# Database config
POSTGRES_USER=myapp
POSTGRES_PASSWORD=secret123
POSTGRES_DB=myapp_db

# Empty line above
PORT=3000
`), 0644)

	env := loadEnvFile(dir)

	if env["POSTGRES_USER"] != "myapp" {
		t.Errorf("expected POSTGRES_USER=myapp, got %s", env["POSTGRES_USER"])
	}
	if env["POSTGRES_PASSWORD"] != "secret123" {
		t.Errorf("expected password secret123, got %s", env["POSTGRES_PASSWORD"])
	}
	if env["PORT"] != "3000" {
		t.Errorf("expected PORT=3000, got %s", env["PORT"])
	}
	if _, exists := env["#"]; exists {
		t.Error("comments should not be parsed as env vars")
	}
}

func TestLoadEnvFileMissing(t *testing.T) {
	env := loadEnvFile("/nonexistent")
	if len(env) != 0 {
		t.Error("expected empty env for missing file")
	}
}

func TestParseComposeFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(`
services:
  app:
    image: myapp:latest
    container_name: myapp-web
  postgres:
    image: postgres:15-alpine
    container_name: myapp-postgres
`), 0644)

	cf, err := parseComposeFile(dir)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(cf.Services) != 2 {
		t.Errorf("expected 2 services, got %d", len(cf.Services))
	}
	if cf.Services["app"].Image != "myapp:latest" {
		t.Errorf("expected image myapp:latest, got %s", cf.Services["app"].Image)
	}
	if cf.Services["postgres"].Container != "myapp-postgres" {
		t.Errorf("expected container name myapp-postgres, got %s", cf.Services["postgres"].Container)
	}
}

func TestParseComposeFileMissing(t *testing.T) {
	_, err := parseComposeFile("/nonexistent")
	if err == nil {
		t.Error("expected error for missing compose file")
	}
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with/slash", "with_slash"},
		{"with.dot", "with_dot"},
		{"with space", "with_space"},
		{"complex/path.name here", "complex_path_name_here"},
	}

	for _, tt := range tests {
		got := sanitizeName(tt.input)
		if got != tt.expected {
			t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		args     []string
		expected string
	}{
		{[]string{"echo", "hello"}, "'echo' 'hello'"},
		{[]string{"cmd", "it's a test"}, "'cmd' 'it'\"'\"'s a test'"},
		{[]string{"docker", "exec", "my-container"}, "'docker' 'exec' 'my-container'"},
	}

	for _, tt := range tests {
		got := shellQuote(tt.args...)
		if got != tt.expected {
			t.Errorf("shellQuote(%v) = %q, want %q", tt.args, got, tt.expected)
		}
	}
}

func TestReadManifest(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{
		"version": "1",
		"project_name": "testapp",
		"project_path": "/opt/fleetdeck/testapp",
		"domain": "testapp.com",
		"created_at": "2025-03-09T10:00:00Z",
		"type": "manual",
		"trigger": "user",
		"components": [
			{"type": "config", "name": "docker-compose.yml", "path": "config/docker-compose.yml", "size_bytes": 100}
		]
	}`), 0644)

	m, err := ReadManifest(dir)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if m.ProjectName != "testapp" {
		t.Errorf("expected project name testapp, got %s", m.ProjectName)
	}
	if m.Domain != "testapp.com" {
		t.Errorf("expected domain testapp.com, got %s", m.Domain)
	}
	if len(m.Components) != 1 {
		t.Errorf("expected 1 component, got %d", len(m.Components))
	}
}

func TestReadManifestMissing(t *testing.T) {
	_, err := ReadManifest("/nonexistent")
	if err == nil {
		t.Error("expected error for missing manifest")
	}
}

// --- Additional BackupConfigFiles tests ---

func TestBackupConfigFilesNginxAndCaddyfile(t *testing.T) {
	projectDir := t.TempDir()
	backupDir := t.TempDir()

	os.WriteFile(filepath.Join(projectDir, "nginx.conf"), []byte("server {}"), 0644)
	os.WriteFile(filepath.Join(projectDir, "Caddyfile"), []byte(":80\nrespond OK"), 0644)
	os.WriteFile(filepath.Join(projectDir, "Makefile"), []byte("build:\n\tgo build"), 0644)

	components, err := BackupConfigFiles(projectDir, backupDir)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	names := make(map[string]bool)
	for _, c := range components {
		names[c.Name] = true
	}

	for _, expected := range []string{"nginx.conf", "Caddyfile", "Makefile"} {
		if !names[expected] {
			t.Errorf("expected %s to be backed up", expected)
		}
	}
}

func TestBackupConfigFilesDockerfileVariants(t *testing.T) {
	projectDir := t.TempDir()
	backupDir := t.TempDir()

	os.WriteFile(filepath.Join(projectDir, "Dockerfile"), []byte("FROM alpine"), 0644)
	os.WriteFile(filepath.Join(projectDir, "Dockerfile.prod"), []byte("FROM ubuntu"), 0644)
	os.WriteFile(filepath.Join(projectDir, "app.dockerfile"), []byte("FROM node"), 0644)

	components, err := BackupConfigFiles(projectDir, backupDir)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	names := make(map[string]bool)
	for _, c := range components {
		names[c.Name] = true
	}

	if !names["Dockerfile"] {
		t.Error("expected Dockerfile to be backed up")
	}
	if !names["Dockerfile.prod"] {
		t.Error("expected Dockerfile.prod to be backed up")
	}
	if !names["app.dockerfile"] {
		t.Error("expected app.dockerfile to be backed up")
	}
}

func TestBackupConfigFilesEmptyProject(t *testing.T) {
	projectDir := t.TempDir()
	backupDir := t.TempDir()

	components, err := BackupConfigFiles(projectDir, backupDir)
	if err != nil {
		t.Fatalf("backup empty: %v", err)
	}
	if len(components) != 0 {
		t.Errorf("expected 0 components for empty project, got %d", len(components))
	}
}

func TestBackupConfigFilesSkipsDirectories(t *testing.T) {
	projectDir := t.TempDir()
	backupDir := t.TempDir()

	// Create a directory with a matching name — it should be skipped
	os.MkdirAll(filepath.Join(projectDir, "Dockerfile"), 0755)
	os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte("services: {}"), 0644)

	components, err := BackupConfigFiles(projectDir, backupDir)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	for _, c := range components {
		if c.Name == "Dockerfile" {
			t.Error("directory named Dockerfile should not be backed up")
		}
	}
}

// --- Manifest creation and reading roundtrip ---

func TestManifestRoundtrip(t *testing.T) {
	dir := t.TempDir()

	original := Manifest{
		Version:     "1",
		ProjectName: "roundtrip-test",
		ProjectPath: "/opt/fleetdeck/roundtrip-test",
		Domain:      "roundtrip.example.com",
		CreatedAt:   "2025-06-15T12:00:00Z",
		Type:        "manual",
		Trigger:     "user",
		Components: []ComponentInfo{
			{Type: "config", Name: "docker-compose.yml", Path: "config/docker-compose.yml", SizeBytes: 256, Checksum: "abc123"},
			{Type: "database", Name: "postgres (PostgreSQL)", Path: "databases/postgres.sql.gz", SizeBytes: 4096},
			{Type: "volume", Name: "app/data", Path: "volumes/app_data.tar.gz", SizeBytes: 8192},
		},
	}

	data, err := json.MarshalIndent(original, "", "  ")
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

	if read.ProjectName != original.ProjectName {
		t.Errorf("project name: got %q, want %q", read.ProjectName, original.ProjectName)
	}
	if read.Domain != original.Domain {
		t.Errorf("domain: got %q, want %q", read.Domain, original.Domain)
	}
	if len(read.Components) != len(original.Components) {
		t.Fatalf("components count: got %d, want %d", len(read.Components), len(original.Components))
	}
	for i, comp := range read.Components {
		orig := original.Components[i]
		if comp.Type != orig.Type || comp.Name != orig.Name || comp.Path != orig.Path {
			t.Errorf("component %d mismatch: got %+v, want %+v", i, comp, orig)
		}
	}
}

func TestReadManifestInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("not json at all"), 0644)

	_, err := ReadManifest(dir)
	if err == nil {
		t.Error("expected error for invalid JSON manifest")
	}
}

// --- Backup directory structure ---

func TestBackupDirectoryStructure(t *testing.T) {
	projectDir := t.TempDir()
	backupDir := t.TempDir()

	os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte("services: {}"), 0644)
	os.WriteFile(filepath.Join(projectDir, ".env"), []byte("KEY=val"), 0644)

	_, err := BackupConfigFiles(projectDir, backupDir)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	// config subdirectory must exist
	configDir := filepath.Join(backupDir, "config")
	info, err := os.Stat(configDir)
	if err != nil {
		t.Fatalf("config directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("config should be a directory")
	}

	// Verify permission is 0700
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("expected config dir perm 0700, got %o", perm)
	}
}

// --- FormatSize edge cases ---

func TestFormatSizeEdgeCases(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{1, "1 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1025, "1.0 KB"},
		{1048575, "1024.0 KB"},
		{1048576, "1.0 MB"},
		{1073741823, "1024.0 MB"},
		{1073741824, "1.0 GB"},
		{10737418240, "10.0 GB"},
	}

	for _, tt := range tests {
		got := FormatSize(tt.bytes)
		if got != tt.expected {
			t.Errorf("FormatSize(%d) = %q, want %q", tt.bytes, got, tt.expected)
		}
	}
}

// --- VerifyBackup tests ---

// createTestBackup builds a realistic backup directory with manifest and files.
func createTestBackup(t *testing.T) string {
	t.Helper()
	backupDir := t.TempDir()

	// Create config file
	configDir := filepath.Join(backupDir, "config")
	os.MkdirAll(configDir, 0700)
	configContent := []byte("services:\n  app:\n    image: myapp:latest\n")
	os.WriteFile(filepath.Join(configDir, "docker-compose.yml"), configContent, 0644)

	h := sha256.Sum256(configContent)
	configChecksum := hex.EncodeToString(h[:])

	// Create a valid gzip database dump
	dbDir := filepath.Join(backupDir, "databases")
	os.MkdirAll(dbDir, 0700)
	dbFile, _ := os.Create(filepath.Join(dbDir, "postgres.sql.gz"))
	gw := gzip.NewWriter(dbFile)
	gw.Write([]byte("CREATE TABLE test (id int);"))
	gw.Close()
	dbFile.Close()
	dbInfo, _ := os.Stat(filepath.Join(dbDir, "postgres.sql.gz"))

	// Create a valid gzip volume archive
	volDir := filepath.Join(backupDir, "volumes")
	os.MkdirAll(volDir, 0700)
	volFile, _ := os.Create(filepath.Join(volDir, "app_data.tar.gz"))
	gw2 := gzip.NewWriter(volFile)
	gw2.Write([]byte("fake tar content"))
	gw2.Close()
	volFile.Close()
	volInfo, _ := os.Stat(filepath.Join(volDir, "app_data.tar.gz"))

	manifest := Manifest{
		Version:     "1",
		ProjectName: "testapp",
		ProjectPath: "/opt/fleetdeck/testapp",
		Domain:      "testapp.com",
		CreatedAt:   "2025-06-15T12:00:00Z",
		Type:        "manual",
		Trigger:     "user",
		Components: []ComponentInfo{
			{Type: "config", Name: "docker-compose.yml", Path: "config/docker-compose.yml", SizeBytes: int64(len(configContent)), Checksum: configChecksum},
			{Type: "database", Name: "postgres (PostgreSQL)", Path: "databases/postgres.sql.gz", SizeBytes: dbInfo.Size()},
			{Type: "volume", Name: "app/data", Path: "volumes/app_data.tar.gz", SizeBytes: volInfo.Size()},
		},
	}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	os.WriteFile(filepath.Join(backupDir, "manifest.json"), data, 0644)

	return backupDir
}

func TestVerifyBackupAllOK(t *testing.T) {
	backupDir := createTestBackup(t)

	results, err := VerifyBackup(backupDir)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	total, ok, failed, missing := CountResults(results)
	if total != 3 {
		t.Errorf("expected 3 total, got %d", total)
	}
	if ok != 3 {
		t.Errorf("expected 3 OK, got %d", ok)
	}
	if failed != 0 || missing != 0 {
		t.Errorf("expected 0 failed/missing, got %d/%d", failed, missing)
	}
	if HasFailures(results) {
		t.Error("expected no failures")
	}
}

func TestVerifyBackupMissingFile(t *testing.T) {
	backupDir := createTestBackup(t)

	// Remove one component file
	os.Remove(filepath.Join(backupDir, "config", "docker-compose.yml"))

	results, err := VerifyBackup(backupDir)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	if !HasFailures(results) {
		t.Error("expected failures for missing file")
	}

	_, _, _, missing := CountResults(results)
	if missing != 1 {
		t.Errorf("expected 1 missing, got %d", missing)
	}
}

func TestVerifyBackupChecksumMismatch(t *testing.T) {
	backupDir := createTestBackup(t)

	// Corrupt the config file content
	os.WriteFile(filepath.Join(backupDir, "config", "docker-compose.yml"), []byte("corrupted content"), 0644)

	results, err := VerifyBackup(backupDir)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	if !HasFailures(results) {
		t.Error("expected failures for checksum mismatch")
	}

	_, _, failed, _ := CountResults(results)
	if failed != 1 {
		t.Errorf("expected 1 failed, got %d", failed)
	}

	// Check that the error message mentions checksum
	for _, r := range results {
		if r.Status == VerifyFailed {
			if r.Error == nil {
				t.Error("expected error message for failed result")
			}
		}
	}
}

func TestVerifyBackupCorruptGzip(t *testing.T) {
	backupDir := createTestBackup(t)

	// Corrupt the database dump (write non-gzip data)
	os.WriteFile(filepath.Join(backupDir, "databases", "postgres.sql.gz"), []byte("not a gzip file"), 0644)

	results, err := VerifyBackup(backupDir)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	if !HasFailures(results) {
		t.Error("expected failures for corrupt gzip")
	}

	_, _, failed, _ := CountResults(results)
	if failed != 1 {
		t.Errorf("expected 1 failed, got %d", failed)
	}
}

func TestVerifyBackupNoManifest(t *testing.T) {
	dir := t.TempDir()

	_, err := VerifyBackup(dir)
	if err == nil {
		t.Error("expected error for missing manifest")
	}
}

func TestVerifyBackupPartialBackupDetected(t *testing.T) {
	backupDir := createTestBackup(t)

	// Remove the database dump AND volume archive to simulate a partial backup
	os.Remove(filepath.Join(backupDir, "databases", "postgres.sql.gz"))
	os.Remove(filepath.Join(backupDir, "volumes", "app_data.tar.gz"))

	results, err := VerifyBackup(backupDir)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	if !HasFailures(results) {
		t.Error("expected failures for partial backup")
	}

	total, ok, _, missing := CountResults(results)
	if total != 3 {
		t.Errorf("expected 3 total, got %d", total)
	}
	if ok != 1 {
		t.Errorf("expected 1 OK (config file only), got %d", ok)
	}
	if missing != 2 {
		t.Errorf("expected 2 missing, got %d", missing)
	}
}

func TestVerifyBackupConfigWithNoChecksum(t *testing.T) {
	backupDir := t.TempDir()

	configDir := filepath.Join(backupDir, "config")
	os.MkdirAll(configDir, 0700)
	os.WriteFile(filepath.Join(configDir, ".env"), []byte("KEY=val"), 0644)

	manifest := Manifest{
		Version:     "1",
		ProjectName: "test",
		Components: []ComponentInfo{
			{Type: "config", Name: ".env", Path: "config/.env", SizeBytes: 7, Checksum: ""},
		},
	}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	os.WriteFile(filepath.Join(backupDir, "manifest.json"), data, 0644)

	results, err := VerifyBackup(backupDir)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	// A config with no checksum should still pass (existence check only)
	if HasFailures(results) {
		t.Error("expected config with empty checksum to pass verification")
	}
}

func TestCountResultsEmpty(t *testing.T) {
	total, ok, failed, missing := CountResults(nil)
	if total != 0 || ok != 0 || failed != 0 || missing != 0 {
		t.Errorf("expected all zeros for nil results, got %d/%d/%d/%d", total, ok, failed, missing)
	}
}

func TestHasFailuresEmpty(t *testing.T) {
	if HasFailures(nil) {
		t.Error("expected no failures for nil results")
	}
	if HasFailures([]VerifyResult{}) {
		t.Error("expected no failures for empty results")
	}
}
