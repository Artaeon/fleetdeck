package environments

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromoteEnvironment(t *testing.T) {
	base := t.TempDir()
	projectName := "myapp"

	// Set up the project structure with a source compose file.
	projectDir := filepath.Join(base, projectName)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	srcCompose := `version: "3.8"
services:
  web:
    image: myapp:latest
    labels:
      - "traefik.http.routers.web.rule=Host(` + "`staging.app.example.com`" + `)"
`
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(srcCompose), 0644); err != nil {
		t.Fatal(err)
	}

	// Create the source (staging) environment using Manager.Create.
	mgr := NewManager(base)
	srcEnv, err := mgr.Create(projectName, "staging", "staging.app.example.com", "develop")
	if err != nil {
		t.Fatalf("failed to create staging environment: %v", err)
	}

	// Write a .env file into the staging environment.
	stagingPath := mgr.GetEnvPath(projectName, "staging")
	envContent := "APP_DOMAIN=staging.app.example.com\nDATABASE_URL=postgres://localhost/myapp\nDEBUG=true\n"
	if err := os.WriteFile(filepath.Join(stagingPath, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Promote staging to production.
	if err := mgr.Promote(projectName, "staging", "production"); err != nil {
		t.Fatalf("failed to promote: %v", err)
	}

	// Verify the production environment directory exists.
	prodPath := mgr.GetEnvPath(projectName, "production")
	if _, err := os.Stat(prodPath); err != nil {
		t.Fatalf("production environment directory not created: %v", err)
	}

	// Verify the compose file was adjusted: domain should be replaced.
	prodCompose, err := os.ReadFile(filepath.Join(prodPath, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("failed to read production compose: %v", err)
	}
	if strings.Contains(string(prodCompose), srcEnv.Domain) {
		t.Error("production compose file still contains staging domain")
	}
	if !strings.Contains(string(prodCompose), "production.app.example.com") {
		t.Error("production compose file does not contain production domain")
	}

	// Verify project name was adjusted.
	if strings.Contains(string(prodCompose), "staging-"+projectName) {
		t.Error("production compose file still contains staging project name prefix")
	}

	// Verify the .env file was adjusted.
	prodEnvData, err := os.ReadFile(filepath.Join(prodPath, ".env"))
	if err != nil {
		t.Fatalf("failed to read production .env: %v", err)
	}
	if strings.Contains(string(prodEnvData), "staging.app.example.com") {
		t.Error("production .env still contains staging domain")
	}
	if !strings.Contains(string(prodEnvData), "production.app.example.com") {
		t.Error("production .env does not contain production domain")
	}
	// Non-domain lines should be preserved.
	if !strings.Contains(string(prodEnvData), "DATABASE_URL=postgres://localhost/myapp") {
		t.Error("production .env lost non-domain line DATABASE_URL")
	}

	// Verify metadata was saved correctly.
	metaData, err := os.ReadFile(filepath.Join(prodPath, "environment.json"))
	if err != nil {
		t.Fatalf("failed to read production metadata: %v", err)
	}
	var prodMeta Environment
	if err := json.Unmarshal(metaData, &prodMeta); err != nil {
		t.Fatalf("failed to unmarshal production metadata: %v", err)
	}
	if prodMeta.Name != "production" {
		t.Errorf("expected env name %q, got %q", "production", prodMeta.Name)
	}
	if prodMeta.Domain != "production.app.example.com" {
		t.Errorf("expected domain %q, got %q", "production.app.example.com", prodMeta.Domain)
	}
	if prodMeta.Branch != "develop" {
		t.Errorf("expected branch %q (inherited from staging), got %q", "develop", prodMeta.Branch)
	}
}

func TestReplaceEnvPrefix(t *testing.T) {
	tests := []struct {
		name     string
		domain   string
		fromEnv  string
		toEnv    string
		expected string
	}{
		{
			name:     "standard prefix replacement",
			domain:   "staging.app.example.com",
			fromEnv:  "staging",
			toEnv:    "production",
			expected: "production.app.example.com",
		},
		{
			name:     "preview to staging",
			domain:   "preview.myapp.io",
			fromEnv:  "preview",
			toEnv:    "staging",
			expected: "staging.myapp.io",
		},
		{
			name:     "domain without matching prefix gets prefix prepended",
			domain:   "app.example.com",
			fromEnv:  "staging",
			toEnv:    "production",
			expected: "production.app.example.com",
		},
		{
			name:     "identical from and to with matching prefix",
			domain:   "staging.test.dev",
			fromEnv:  "staging",
			toEnv:    "staging",
			expected: "staging.test.dev",
		},
		{
			name:     "single-level domain with matching prefix",
			domain:   "staging.localhost",
			fromEnv:  "staging",
			toEnv:    "production",
			expected: "production.localhost",
		},
		{
			name:     "partial prefix mismatch prepends",
			domain:   "stage.app.example.com",
			fromEnv:  "staging",
			toEnv:    "production",
			expected: "production.stage.app.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := replaceEnvPrefix(tt.domain, tt.fromEnv, tt.toEnv)
			if result != tt.expected {
				t.Errorf("replaceEnvPrefix(%q, %q, %q) = %q, want %q",
					tt.domain, tt.fromEnv, tt.toEnv, result, tt.expected)
			}
		})
	}
}

func TestReplaceHostRule(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		domain   string
		expected string
	}{
		{
			name:     "standard Host rule",
			line:     `      - "traefik.http.routers.web.rule=Host(` + "`old.example.com`" + `)"`,
			domain:   "new.example.com",
			expected: `      - "traefik.http.routers.web.rule=Host(` + "`new.example.com`" + `)"`,
		},
		{
			name:     "Host rule with different indentation",
			line:     `    - "traefik.http.routers.api.rule=Host(` + "`api.staging.dev`" + `)"`,
			domain:   "api.production.dev",
			expected: `    - "traefik.http.routers.api.rule=Host(` + "`api.production.dev`" + `)"`,
		},
		{
			name:     "line without Host rule unchanged",
			line:     `    - "traefik.http.routers.web.entrypoints=websecure"`,
			domain:   "new.example.com",
			expected: `    - "traefik.http.routers.web.entrypoints=websecure"`,
		},
		{
			name:     "Host rule with long domain",
			line:     `      - "traefik.http.routers.web.rule=Host(` + "`staging.subdomain.very-long-domain.example.co.uk`" + `)"`,
			domain:   "production.subdomain.very-long-domain.example.co.uk",
			expected: `      - "traefik.http.routers.web.rule=Host(` + "`production.subdomain.very-long-domain.example.co.uk`" + `)"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := replaceHostRule(tt.line, tt.domain)
			if result != tt.expected {
				t.Errorf("replaceHostRule(%q, %q) =\n  %q\nwant:\n  %q",
					tt.line, tt.domain, result, tt.expected)
			}
		})
	}
}

func TestAdjustComposeForEnv(t *testing.T) {
	compose := `version: "3.8"
services:
  web:
    image: myapp:latest
    labels:
      - "traefik.http.routers.web.rule=Host(` + "`app.example.com`" + `)"
      - "traefik.http.routers.web.entrypoints=websecure"
  worker:
    image: myapp-worker:latest
`
	envName := "staging"
	projectName := "myapp"
	domain := "staging.app.example.com"

	result := adjustComposeForEnv(compose, projectName, envName, domain)

	// Verify the project name prefix is prepended.
	expectedPrefix := "name: staging-myapp\n"
	if !strings.HasPrefix(result, expectedPrefix) {
		t.Errorf("expected compose to start with %q, got prefix: %q",
			expectedPrefix, result[:min(len(result), len(expectedPrefix)+20)])
	}

	// Verify Host rule was replaced with the environment domain.
	if !strings.Contains(result, "Host(`staging.app.example.com`)") {
		t.Error("expected Host rule to contain staging domain")
	}

	// Verify non-Host labels are preserved unchanged.
	if !strings.Contains(result, "traefik.http.routers.web.entrypoints=websecure") {
		t.Error("non-Host label should be preserved")
	}

	// Verify service definitions are preserved.
	if !strings.Contains(result, "image: myapp:latest") {
		t.Error("service image should be preserved")
	}
	if !strings.Contains(result, "image: myapp-worker:latest") {
		t.Error("worker service should be preserved")
	}
}

func TestAdjustComposeForEnv_ExistingName(t *testing.T) {
	// Compose file that already has a name: field should not get a second one.
	compose := `name: existing-project
version: "3.8"
services:
  web:
    image: myapp:latest
`
	result := adjustComposeForEnv(compose, "myapp", "staging", "staging.example.com")

	// The existing name: line prevents prepending a new one.
	if strings.Count(result, "name:") != 1 {
		t.Errorf("expected exactly one name: field, got %d occurrences", strings.Count(result, "name:"))
	}
}

func TestAdjustEnvFile(t *testing.T) {
	envContent := `APP_DOMAIN=old.example.com
DATABASE_URL=postgres://localhost:5432/mydb
REDIS_URL=redis://localhost:6379
MY_DOMAIN_VAR=should-be-replaced
SECRET_KEY=supersecret
LOG_LEVEL=debug
`
	domain := "new.example.com"

	result := adjustEnvFile(envContent, domain)
	lines := strings.Split(result, "\n")

	expectations := map[string]string{
		"APP_DOMAIN":     "new.example.com",
		"DATABASE_URL":   "postgres://localhost:5432/mydb",
		"REDIS_URL":      "redis://localhost:6379",
		"MY_DOMAIN_VAR":  "new.example.com",
		"SECRET_KEY":     "supersecret",
		"LOG_LEVEL":      "debug",
	}

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, value := parts[0], parts[1]
		if expected, ok := expectations[key]; ok {
			if value != expected {
				t.Errorf("for key %q: expected value %q, got %q", key, expected, value)
			}
		}
	}

	// Verify non-domain lines are truly unmodified.
	if !strings.Contains(result, "DATABASE_URL=postgres://localhost:5432/mydb") {
		t.Error("DATABASE_URL should be preserved exactly")
	}
	if !strings.Contains(result, "SECRET_KEY=supersecret") {
		t.Error("SECRET_KEY should be preserved exactly")
	}
}

func TestAdjustEnvFile_NoDomainLines(t *testing.T) {
	envContent := `PORT=3000
DATABASE_URL=postgres://localhost/mydb
SECRET=abc123
`
	result := adjustEnvFile(envContent, "new.example.com")

	// No lines contain "DOMAIN" in the key, so nothing should change.
	if result != envContent {
		t.Errorf("expected env file to be unchanged when no DOMAIN keys exist\ngot:\n%s\nwant:\n%s", result, envContent)
	}
}

func TestDeleteNonexistentEnvironment(t *testing.T) {
	base := t.TempDir()
	mgr := NewManager(base)

	// Deleting an environment that does not exist should not error,
	// because os.RemoveAll on a non-existent path returns nil.
	err := mgr.Delete("nonexistent-project", "nonexistent-env")
	if err != nil {
		t.Errorf("expected no error when deleting nonexistent environment, got: %v", err)
	}

	// Also verify the base directory is still intact.
	if _, err := os.Stat(base); err != nil {
		t.Errorf("base directory should still exist after delete: %v", err)
	}
}

func TestDeleteExistingEnvironment(t *testing.T) {
	base := t.TempDir()
	projectName := "myapp"

	// Set up project with compose file.
	projectDir := filepath.Join(base, projectName)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}
	compose := `version: "3.8"
services:
  web:
    image: myapp:latest
`
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(compose), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(base)
	_, err := mgr.Create(projectName, "staging", "staging.example.com", "develop")
	if err != nil {
		t.Fatalf("failed to create environment: %v", err)
	}

	envPath := mgr.GetEnvPath(projectName, "staging")
	if _, err := os.Stat(envPath); err != nil {
		t.Fatalf("environment directory should exist before delete: %v", err)
	}

	// Delete the environment (docker compose down will fail since docker is not
	// running, but Delete should still succeed because it's best-effort).
	if err := mgr.Delete(projectName, "staging"); err != nil {
		t.Errorf("unexpected error deleting environment: %v", err)
	}

	// Verify the environment directory was removed.
	if _, err := os.Stat(envPath); !os.IsNotExist(err) {
		t.Error("expected environment directory to be removed after delete")
	}
}

func TestListEmptyProject(t *testing.T) {
	base := t.TempDir()
	mgr := NewManager(base)

	envs, err := mgr.List("nonexistent-project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if envs != nil {
		t.Errorf("expected nil for nonexistent project, got %v", envs)
	}
}

func TestCreateAndList(t *testing.T) {
	base := t.TempDir()
	projectName := "webapp"

	// Set up project.
	projectDir := filepath.Join(base, projectName)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}
	compose := `version: "3.8"
services:
  app:
    image: webapp:latest
`
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(compose), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(base)

	// Create two environments.
	env1, err := mgr.Create(projectName, "staging", "staging.webapp.dev", "develop")
	if err != nil {
		t.Fatalf("failed to create staging: %v", err)
	}
	if env1.Name != "staging" {
		t.Errorf("expected env name %q, got %q", "staging", env1.Name)
	}
	if env1.Status != "creating" {
		t.Errorf("expected status %q, got %q", "creating", env1.Status)
	}

	env2, err := mgr.Create(projectName, "production", "production.webapp.dev", "main")
	if err != nil {
		t.Fatalf("failed to create production: %v", err)
	}
	if env2.Name != "production" {
		t.Errorf("expected env name %q, got %q", "production", env2.Name)
	}

	// List environments.
	envs, err := mgr.List(projectName)
	if err != nil {
		t.Fatalf("failed to list environments: %v", err)
	}
	if len(envs) != 2 {
		t.Fatalf("expected 2 environments, got %d", len(envs))
	}

	// Verify both environments are present (order may vary by readdir).
	names := make(map[string]bool)
	for _, e := range envs {
		names[e.Name] = true
	}
	if !names["staging"] {
		t.Error("expected staging environment in list")
	}
	if !names["production"] {
		t.Error("expected production environment in list")
	}
}

func TestGetEnvPathFormat(t *testing.T) {
	mgr := NewManager("/data/projects")
	path := mgr.GetEnvPath("myapp", "staging")
	expected := filepath.Join("/data/projects", "myapp", "environments", "staging")
	if path != expected {
		t.Errorf("expected path %q, got %q", expected, path)
	}

	// Verify different env names produce different paths.
	prodPath := mgr.GetEnvPath("myapp", "production")
	if prodPath == path {
		t.Error("staging and production paths should differ")
	}
	if !strings.HasSuffix(prodPath, "production") {
		t.Errorf("expected production path to end with 'production', got %q", prodPath)
	}
}
