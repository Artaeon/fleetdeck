package environments

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewManager(t *testing.T) {
	m := NewManager("/tmp/test-base")
	if m == nil {
		t.Fatal("NewManager() returned nil")
	}
	if m.basePath != "/tmp/test-base" {
		t.Errorf("basePath = %q, want %q", m.basePath, "/tmp/test-base")
	}
}

// setupProject creates a minimal project structure with docker-compose.yml and
// .env in the given base directory, returning the Manager.
func setupProject(t *testing.T, basePath, projectName string) *Manager {
	t.Helper()

	projectDir := filepath.Join(basePath, projectName)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	composeContent := `services:
  app:
    image: myapp:latest
    labels:
      - "traefik.http.routers.myapp.rule=Host(` + "`prod.example.com`" + `)"
`
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(composeContent), 0644); err != nil {
		t.Fatalf("writing docker-compose.yml: %v", err)
	}

	envContent := "APP_DOMAIN=prod.example.com\nSECRET_KEY=abc123\n"
	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	return NewManager(basePath)
}

func TestCreateEnvironment(t *testing.T) {
	tmpDir := t.TempDir()
	m := setupProject(t, tmpDir, "myapp")

	env, err := m.Create("myapp", "staging", "staging.example.com", "develop")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// Verify returned environment fields.
	if env.Name != "staging" {
		t.Errorf("Name = %q, want %q", env.Name, "staging")
	}
	if env.Domain != "staging.example.com" {
		t.Errorf("Domain = %q, want %q", env.Domain, "staging.example.com")
	}
	if env.Branch != "develop" {
		t.Errorf("Branch = %q, want %q", env.Branch, "develop")
	}
	if env.ProjectName != "myapp" {
		t.Errorf("ProjectName = %q, want %q", env.ProjectName, "myapp")
	}
	if env.Status != "creating" {
		t.Errorf("Status = %q, want %q", env.Status, "creating")
	}
	if env.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}

	// Verify environment directory was created.
	envPath := m.GetEnvPath("myapp", "staging")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		t.Fatalf("environment directory not created at %s", envPath)
	}

	// Verify docker-compose.yml was written to the environment directory.
	composePath := filepath.Join(envPath, "docker-compose.yml")
	composeData, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("reading environment compose file: %v", err)
	}
	composeStr := string(composeData)

	// The compose file should reference the staging domain.
	if !strings.Contains(composeStr, "staging.example.com") {
		t.Error("environment compose should contain staging domain")
	}

	// Verify .env was written with adjusted domain.
	envFilePath := filepath.Join(envPath, ".env")
	envData, err := os.ReadFile(envFilePath)
	if err != nil {
		t.Fatalf("reading environment .env file: %v", err)
	}
	envStr := string(envData)
	if !strings.Contains(envStr, "staging.example.com") {
		t.Error("environment .env should contain staging domain")
	}

	// Verify metadata was persisted.
	metaPath := filepath.Join(envPath, "environment.json")
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("reading environment.json: %v", err)
	}
	var meta Environment
	if err := json.Unmarshal(metaData, &meta); err != nil {
		t.Fatalf("parsing environment.json: %v", err)
	}
	if meta.Name != "staging" {
		t.Errorf("metadata Name = %q, want %q", meta.Name, "staging")
	}
	if meta.Domain != "staging.example.com" {
		t.Errorf("metadata Domain = %q, want %q", meta.Domain, "staging.example.com")
	}
}

func TestListEnvironments(t *testing.T) {
	tmpDir := t.TempDir()
	m := setupProject(t, tmpDir, "myapp")

	// Create multiple environments.
	envConfigs := []struct {
		name   string
		domain string
		branch string
	}{
		{"staging", "staging.example.com", "develop"},
		{"production", "prod.example.com", "main"},
		{"preview-42", "preview-42.example.com", "feature/login"},
	}

	for _, cfg := range envConfigs {
		if _, err := m.Create("myapp", cfg.name, cfg.domain, cfg.branch); err != nil {
			t.Fatalf("Create(%q) error: %v", cfg.name, err)
		}
	}

	envs, err := m.List("myapp")
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	if len(envs) != 3 {
		t.Fatalf("List() returned %d environments, want 3", len(envs))
	}

	// Collect returned names.
	names := map[string]bool{}
	for _, e := range envs {
		names[e.Name] = true
	}

	for _, cfg := range envConfigs {
		if !names[cfg.name] {
			t.Errorf("List() missing environment %q", cfg.name)
		}
	}
}

func TestListEnvironmentsEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// Listing environments for a project with no environments directory
	// should return nil, nil (not an error).
	envs, err := m.List("nonexistent")
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if envs != nil {
		t.Errorf("List() should return nil for nonexistent project, got %v", envs)
	}
}

func TestDeleteEnvironment(t *testing.T) {
	tmpDir := t.TempDir()
	m := setupProject(t, tmpDir, "myapp")

	// Create an environment first.
	_, err := m.Create("myapp", "staging", "staging.example.com", "develop")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	envPath := m.GetEnvPath("myapp", "staging")

	// Verify the environment directory exists before deletion.
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		t.Fatal("environment directory should exist before deletion")
	}

	// Delete the environment.
	if err := m.Delete("myapp", "staging"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	// Verify the directory was removed.
	if _, err := os.Stat(envPath); !os.IsNotExist(err) {
		t.Error("environment directory should be removed after deletion")
	}

	// Listing should no longer include the deleted environment.
	envs, err := m.List("myapp")
	if err != nil {
		t.Fatalf("List() after delete error: %v", err)
	}
	for _, e := range envs {
		if e.Name == "staging" {
			t.Error("deleted environment should not appear in List()")
		}
	}
}

func TestDeleteEnvironmentNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// Deleting a non-existent environment should not error since
	// os.RemoveAll on a non-existent path returns nil.
	err := m.Delete("myapp", "nonexistent")
	if err != nil {
		t.Errorf("Delete() on nonexistent env should not error, got: %v", err)
	}
}

func TestGetEnvPath(t *testing.T) {
	tests := []struct {
		name        string
		basePath    string
		projectName string
		envName     string
		expected    string
	}{
		{
			name:        "simple path",
			basePath:    "/data/projects",
			projectName: "myapp",
			envName:     "staging",
			expected:    "/data/projects/myapp/environments/staging",
		},
		{
			name:        "production environment",
			basePath:    "/var/fleetdeck",
			projectName: "webapp",
			envName:     "production",
			expected:    "/var/fleetdeck/webapp/environments/production",
		},
		{
			name:        "preview environment with ID",
			basePath:    "/home/user/projects",
			projectName: "api-service",
			envName:     "preview-123",
			expected:    "/home/user/projects/api-service/environments/preview-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(tt.basePath)
			got := m.GetEnvPath(tt.projectName, tt.envName)
			if got != tt.expected {
				t.Errorf("GetEnvPath(%q, %q) = %q, want %q", tt.projectName, tt.envName, got, tt.expected)
			}
		})
	}
}

func TestCreateEnvironmentInvalidName(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// Creating an environment for a project that does not exist
	// (no docker-compose.yml) should fail.
	_, err := m.Create("", "staging", "staging.example.com", "develop")
	if err == nil {
		t.Error("Create() with empty project name should fail (no source project)")
	}
}

func TestCreateEnvironmentMissingProject(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// Creating an environment for a project with no docker-compose.yml
	// should return an error about the source project not being found.
	_, err := m.Create("nonexistent-project", "staging", "staging.example.com", "main")
	if err == nil {
		t.Fatal("Create() should fail when source project does not exist")
	}
	if !strings.Contains(err.Error(), "source project not found") {
		t.Errorf("error should mention source project not found, got: %v", err)
	}
}

func TestCreateMultipleEnvironmentsSameProject(t *testing.T) {
	tmpDir := t.TempDir()
	m := setupProject(t, tmpDir, "myapp")

	env1, err := m.Create("myapp", "staging", "staging.example.com", "develop")
	if err != nil {
		t.Fatalf("Create staging: %v", err)
	}

	env2, err := m.Create("myapp", "production", "prod.example.com", "main")
	if err != nil {
		t.Fatalf("Create production: %v", err)
	}

	// Verify both environments have distinct paths and metadata.
	path1 := m.GetEnvPath("myapp", "staging")
	path2 := m.GetEnvPath("myapp", "production")
	if path1 == path2 {
		t.Error("staging and production should have different paths")
	}

	if env1.Domain == env2.Domain {
		t.Error("staging and production should have different domains")
	}

	// Both should appear in the list.
	envs, err := m.List("myapp")
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(envs) != 2 {
		t.Errorf("expected 2 environments, got %d", len(envs))
	}
}

func TestCreateEnvironmentComposeAdjustment(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "webapp")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a compose file with a Host() rule referencing the production domain.
	compose := `services:
  app:
    image: webapp:latest
    labels:
      - "traefik.http.routers.webapp.rule=Host(` + "`prod.webapp.com`" + `)"
`
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(compose), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewManager(tmpDir)
	_, err := m.Create("webapp", "staging", "staging.webapp.com", "develop")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	envComposePath := filepath.Join(m.GetEnvPath("webapp", "staging"), "docker-compose.yml")
	data, err := os.ReadFile(envComposePath)
	if err != nil {
		t.Fatalf("reading env compose: %v", err)
	}

	content := string(data)

	// The Host() rule should now reference the staging domain.
	if !strings.Contains(content, "staging.webapp.com") {
		t.Error("compose should contain the staging domain in Host() rule")
	}

	// The compose should include a project name prefix for namespacing.
	if !strings.Contains(content, "name: staging-webapp") {
		t.Error("compose should include environment-prefixed project name")
	}
}

func TestCreateEnvironmentEnvFileAdjustment(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "myapp")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte("services: {}"), 0644); err != nil {
		t.Fatal(err)
	}

	envContent := "APP_DOMAIN=prod.example.com\nAPI_URL=https://prod.example.com/api\nDB_HOST=localhost\n"
	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewManager(tmpDir)
	_, err := m.Create("myapp", "staging", "staging.example.com", "develop")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	envData, err := os.ReadFile(filepath.Join(m.GetEnvPath("myapp", "staging"), ".env"))
	if err != nil {
		t.Fatalf("reading env file: %v", err)
	}

	content := string(envData)

	// Lines with DOMAIN in the key should have the domain replaced.
	if !strings.Contains(content, "APP_DOMAIN=staging.example.com") {
		t.Errorf("APP_DOMAIN should be updated to staging domain, got:\n%s", content)
	}

	// Non-DOMAIN lines should remain unchanged.
	if !strings.Contains(content, "DB_HOST=localhost") {
		t.Error("DB_HOST should remain unchanged")
	}
}

func TestCreateEnvironmentNoEnvFile(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "myapp")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Only create docker-compose.yml, no .env file.
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte("services: {}"), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewManager(tmpDir)
	_, err := m.Create("myapp", "staging", "staging.example.com", "develop")
	if err != nil {
		t.Fatalf("Create() should succeed without .env file, got: %v", err)
	}

	// The environment directory should exist but with no .env file
	// (since source project has no .env to copy).
	envPath := m.GetEnvPath("myapp", "staging")
	if _, err := os.Stat(filepath.Join(envPath, ".env")); !os.IsNotExist(err) {
		t.Error("environment should not have .env when source project has none")
	}
}
