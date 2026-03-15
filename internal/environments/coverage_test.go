package environments

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// saveMetadata / loadMetadata round-trip
// ---------------------------------------------------------------------------

func TestSaveAndLoadMetadata(t *testing.T) {
	base := t.TempDir()
	m := NewManager(base)

	envPath := filepath.Join(base, "project1", "environments", "staging")
	if err := os.MkdirAll(envPath, 0755); err != nil {
		t.Fatal(err)
	}

	original := &Environment{
		Name:        "staging",
		Domain:      "staging.example.com",
		Branch:      "develop",
		ProjectName: "project1",
		Status:      "running",
		CreatedAt:   time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC),
	}

	if err := m.saveMetadata(envPath, original); err != nil {
		t.Fatalf("saveMetadata() error: %v", err)
	}

	loaded, err := m.loadMetadata(envPath)
	if err != nil {
		t.Fatalf("loadMetadata() error: %v", err)
	}

	if loaded.Name != original.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, original.Name)
	}
	if loaded.Domain != original.Domain {
		t.Errorf("Domain = %q, want %q", loaded.Domain, original.Domain)
	}
	if loaded.Branch != original.Branch {
		t.Errorf("Branch = %q, want %q", loaded.Branch, original.Branch)
	}
	if loaded.ProjectName != original.ProjectName {
		t.Errorf("ProjectName = %q, want %q", loaded.ProjectName, original.ProjectName)
	}
	if loaded.Status != original.Status {
		t.Errorf("Status = %q, want %q", loaded.Status, original.Status)
	}
	if !loaded.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", loaded.CreatedAt, original.CreatedAt)
	}
}

func TestLoadMetadataFileNotFound(t *testing.T) {
	base := t.TempDir()
	m := NewManager(base)

	_, err := m.loadMetadata(filepath.Join(base, "nonexistent"))
	if err == nil {
		t.Error("loadMetadata() should return error for nonexistent path")
	}
}

func TestLoadMetadataMalformedJSON(t *testing.T) {
	base := t.TempDir()
	m := NewManager(base)

	envPath := filepath.Join(base, "corrupt")
	if err := os.MkdirAll(envPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envPath, "environment.json"), []byte("not valid json{{{"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := m.loadMetadata(envPath)
	if err == nil {
		t.Error("loadMetadata() should return error for malformed JSON")
	}
}

// ---------------------------------------------------------------------------
// adjustComposeForEnv
// ---------------------------------------------------------------------------

func TestAdjustComposeForEnvProjectName(t *testing.T) {
	compose := `services:
  app:
    image: myapp:latest
`
	result := adjustComposeForEnv(compose, "myapp", "staging", "staging.example.com")

	expectedPrefix := "name: staging-myapp\n"
	if !strings.HasPrefix(result, expectedPrefix) {
		t.Errorf("expected compose to start with %q, got:\n%s", expectedPrefix, result)
	}
}

func TestAdjustComposeForEnvExistingName(t *testing.T) {
	compose := `name: already-set
services:
  app:
    image: myapp:latest
`
	result := adjustComposeForEnv(compose, "myapp", "staging", "staging.example.com")

	if strings.Count(result, "name:") != 1 {
		t.Errorf("expected exactly one name: field, got %d", strings.Count(result, "name:"))
	}
	// The existing name should be preserved.
	if !strings.Contains(result, "name: already-set") {
		t.Error("existing name: field should be preserved")
	}
}

func TestAdjustComposeForEnvHostRules(t *testing.T) {
	compose := "services:\n  app:\n    labels:\n      - \"traefik.http.routers.app.rule=Host(`prod.example.com`)\"\n"
	result := adjustComposeForEnv(compose, "myapp", "staging", "staging.example.com")

	if !strings.Contains(result, "Host(`staging.example.com`)") {
		t.Errorf("expected Host rule to contain staging domain, got:\n%s", result)
	}
	if strings.Contains(result, "Host(`prod.example.com`)") {
		t.Error("original production domain should be replaced")
	}
}

// ---------------------------------------------------------------------------
// adjustEnvFile
// ---------------------------------------------------------------------------

func TestAdjustEnvFileNoDomain(t *testing.T) {
	envContent := "PORT=3000\nDB_HOST=localhost\nSECRET=abc\n"
	result := adjustEnvFile(envContent, "new.example.com")

	if result != envContent {
		t.Errorf("expected unchanged env file when no DOMAIN keys, got:\n%s", result)
	}
}

func TestAdjustEnvFileMixedCase(t *testing.T) {
	envContent := "APP_DOMAIN=old.example.com\nDOMAIN=old.example.com\nSECRET=abc\n"
	result := adjustEnvFile(envContent, "new.example.com")

	if !strings.Contains(result, "APP_DOMAIN=new.example.com") {
		t.Errorf("APP_DOMAIN should be replaced, got:\n%s", result)
	}
	if !strings.Contains(result, "DOMAIN=new.example.com") {
		t.Errorf("DOMAIN should be replaced, got:\n%s", result)
	}
	if !strings.Contains(result, "SECRET=abc") {
		t.Error("SECRET should remain unchanged")
	}
}

// ---------------------------------------------------------------------------
// replaceHostRule
// ---------------------------------------------------------------------------

func TestReplaceHostRuleNoHost(t *testing.T) {
	line := `      - "traefik.http.routers.web.entrypoints=websecure"`
	result := replaceHostRule(line, "new.example.com")
	if result != line {
		t.Errorf("line without Host() should be unchanged, got:\n%s", result)
	}
}

func TestReplaceHostRuleMalformed(t *testing.T) {
	// Has Host(` but no closing backtick+paren.
	line := "      - \"traefik.http.routers.web.rule=Host(`incomplete"
	result := replaceHostRule(line, "new.example.com")
	if result != line {
		t.Errorf("malformed Host rule should be unchanged, got:\n%s", result)
	}
}

// ---------------------------------------------------------------------------
// replaceEnvPrefix
// ---------------------------------------------------------------------------

func TestReplaceEnvPrefixExact(t *testing.T) {
	result := replaceEnvPrefix("staging.app.example.com", "staging", "production")
	expected := "production.app.example.com"
	if result != expected {
		t.Errorf("replaceEnvPrefix() = %q, want %q", result, expected)
	}
}

func TestReplaceEnvPrefixNoMatch(t *testing.T) {
	// Domain does not start with fromEnv prefix, so toEnv is prepended.
	result := replaceEnvPrefix("app.example.com", "staging", "production")
	expected := "production.app.example.com"
	if result != expected {
		t.Errorf("replaceEnvPrefix() = %q, want %q", result, expected)
	}
}

// ---------------------------------------------------------------------------
// Create — error when mkdir fails (read-only dir)
// ---------------------------------------------------------------------------

func TestCreateMkdirFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission tests unreliable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("skipping test when running as root")
	}

	base := t.TempDir()
	projectName := "myapp"
	projectDir := filepath.Join(base, projectName)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte("services: {}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Make the environments parent read-only so MkdirAll fails.
	envsDir := filepath.Join(projectDir, "environments")
	if err := os.MkdirAll(envsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(envsDir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(envsDir, 0755) })

	m := NewManager(base)
	_, err := m.Create(projectName, "staging", "staging.example.com", "develop")
	if err == nil {
		t.Error("Create() should fail when environment directory cannot be created")
	}
}

// ---------------------------------------------------------------------------
// Create — source project has no .env file
// ---------------------------------------------------------------------------

func TestCreateEnvNoSourceEnv(t *testing.T) {
	base := t.TempDir()
	projectName := "noenv"
	projectDir := filepath.Join(base, projectName)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte("services: {}"), 0644); err != nil {
		t.Fatal(err)
	}
	// Deliberately no .env file in the project directory.

	m := NewManager(base)
	env, err := m.Create(projectName, "staging", "staging.example.com", "develop")
	if err != nil {
		t.Fatalf("Create() should succeed without .env, got: %v", err)
	}
	if env == nil {
		t.Fatal("Create() returned nil environment")
	}

	// Verify no .env was created in the environment directory.
	envPath := m.GetEnvPath(projectName, "staging")
	if _, statErr := os.Stat(filepath.Join(envPath, ".env")); !os.IsNotExist(statErr) {
		t.Error("expected no .env in target when source has none")
	}
}

// ---------------------------------------------------------------------------
// List — skips non-directories
// ---------------------------------------------------------------------------

func TestListSkipsNonDirectories(t *testing.T) {
	base := t.TempDir()
	projectName := "myapp"
	envsDir := filepath.Join(base, projectName, "environments")
	if err := os.MkdirAll(envsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a regular file inside environments/ (not a directory).
	if err := os.WriteFile(filepath.Join(envsDir, "stray-file.txt"), []byte("junk"), 0644); err != nil {
		t.Fatal(err)
	}

	// Also create a valid environment directory.
	validEnvDir := filepath.Join(envsDir, "staging")
	if err := os.MkdirAll(validEnvDir, 0755); err != nil {
		t.Fatal(err)
	}
	meta := Environment{
		Name:        "staging",
		Domain:      "staging.example.com",
		Branch:      "develop",
		ProjectName: projectName,
		Status:      "running",
		CreatedAt:   time.Now().UTC(),
	}
	data, _ := json.MarshalIndent(&meta, "", "  ")
	if err := os.WriteFile(filepath.Join(validEnvDir, "environment.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	m := NewManager(base)
	envs, err := m.List(projectName)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	// Should only return the valid directory entry, not the stray file.
	if len(envs) != 1 {
		t.Fatalf("List() returned %d environments, want 1", len(envs))
	}
	if envs[0].Name != "staging" {
		t.Errorf("List()[0].Name = %q, want %q", envs[0].Name, "staging")
	}
}

// ---------------------------------------------------------------------------
// List — skips corrupted metadata
// ---------------------------------------------------------------------------

func TestListSkipsCorruptedMetadata(t *testing.T) {
	base := t.TempDir()
	projectName := "myapp"
	envsDir := filepath.Join(base, projectName, "environments")

	// Create a directory with invalid environment.json.
	corruptDir := filepath.Join(envsDir, "corrupt")
	if err := os.MkdirAll(corruptDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corruptDir, "environment.json"), []byte("not json!!!"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a valid environment alongside.
	validDir := filepath.Join(envsDir, "production")
	if err := os.MkdirAll(validDir, 0755); err != nil {
		t.Fatal(err)
	}
	meta := Environment{
		Name:        "production",
		Domain:      "prod.example.com",
		Branch:      "main",
		ProjectName: projectName,
		Status:      "running",
		CreatedAt:   time.Now().UTC(),
	}
	data, _ := json.MarshalIndent(&meta, "", "  ")
	if err := os.WriteFile(filepath.Join(validDir, "environment.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	m := NewManager(base)
	envs, err := m.List(projectName)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	// Corrupted entry should be skipped, only the valid one returned.
	if len(envs) != 1 {
		t.Fatalf("List() returned %d environments, want 1 (corrupt entry should be skipped)", len(envs))
	}
	if envs[0].Name != "production" {
		t.Errorf("List()[0].Name = %q, want %q", envs[0].Name, "production")
	}
}
