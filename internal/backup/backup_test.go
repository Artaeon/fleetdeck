package backup

import (
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
