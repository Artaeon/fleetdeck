package environments

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupUseCaseProject creates a minimal project structure with docker-compose.yml
// and optionally a .env file for use-case scenario tests.
func setupUseCaseProject(t *testing.T, basePath, projectName string, withEnv bool) *Manager {
	t.Helper()

	projectDir := filepath.Join(basePath, projectName)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	composeContent := `services:
  app:
    build: .
    image: ` + projectName + `:latest
    labels:
      - "traefik.http.routers.` + projectName + `.rule=Host(` + "`" + projectName + `.app.com` + "`" + `)"
`
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(composeContent), 0644); err != nil {
		t.Fatalf("writing docker-compose.yml: %v", err)
	}

	if withEnv {
		envContent := "APP_DOMAIN=" + projectName + ".app.com\nSECRET_KEY=abc123\nDATABASE_URL=postgres://localhost/mydb\n"
		if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte(envContent), 0644); err != nil {
			t.Fatalf("writing .env: %v", err)
		}
	}

	return NewManager(basePath)
}

// TestUseCaseCreateStagingEnvironment simulates a developer creating a staging
// environment for their project. This is the most common daily workflow: the
// source project has docker-compose.yml and .env, and the staging env should
// get copies of both with domains adjusted.
func TestUseCaseCreateStagingEnvironment(t *testing.T) {
	base := t.TempDir()
	mgr := setupUseCaseProject(t, base, "webapp", true)

	stagingDomain := "staging.app.com"
	env, err := mgr.Create("webapp", "staging", stagingDomain, "develop")
	if err != nil {
		t.Fatalf("Create staging: %v", err)
	}

	// Verify returned environment fields match what the user specified.
	if env.Name != "staging" {
		t.Errorf("Name = %q, want %q", env.Name, "staging")
	}
	if env.Domain != stagingDomain {
		t.Errorf("Domain = %q, want %q", env.Domain, stagingDomain)
	}
	if env.Branch != "develop" {
		t.Errorf("Branch = %q, want %q", env.Branch, "develop")
	}
	if env.ProjectName != "webapp" {
		t.Errorf("ProjectName = %q, want %q", env.ProjectName, "webapp")
	}
	if env.Status != "creating" {
		t.Errorf("Status = %q, want %q", env.Status, "creating")
	}
	if env.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}

	// Verify all files were created in the environment directory.
	envPath := mgr.GetEnvPath("webapp", "staging")

	// docker-compose.yml should exist and contain the staging domain.
	composeData, err := os.ReadFile(filepath.Join(envPath, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("reading environment compose: %v", err)
	}
	if !strings.Contains(string(composeData), stagingDomain) {
		t.Error("compose should contain staging domain in Host rule")
	}
	if !strings.Contains(string(composeData), "name: staging-webapp") {
		t.Error("compose should have environment-prefixed project name")
	}

	// .env should exist and have the domain adjusted.
	envData, err := os.ReadFile(filepath.Join(envPath, ".env"))
	if err != nil {
		t.Fatalf("reading environment .env: %v", err)
	}
	if !strings.Contains(string(envData), "APP_DOMAIN="+stagingDomain) {
		t.Errorf(".env APP_DOMAIN should be %s, got:\n%s", stagingDomain, string(envData))
	}
	// Non-domain values should be preserved.
	if !strings.Contains(string(envData), "DATABASE_URL=postgres://localhost/mydb") {
		t.Error(".env should preserve DATABASE_URL unchanged")
	}

	// Metadata (environment.json) should be persisted and loadable.
	metaData, err := os.ReadFile(filepath.Join(envPath, "environment.json"))
	if err != nil {
		t.Fatalf("reading environment.json: %v", err)
	}
	var meta Environment
	if err := json.Unmarshal(metaData, &meta); err != nil {
		t.Fatalf("parsing environment.json: %v", err)
	}
	if meta.Name != "staging" || meta.Domain != stagingDomain || meta.ProjectName != "webapp" {
		t.Errorf("metadata mismatch: %+v", meta)
	}
}

// TestUseCaseDuplicateEnvironment verifies that trying to create an environment
// that already exists returns a clear error with both the project and environment
// names in the message, so the user knows exactly what the conflict is.
func TestUseCaseDuplicateEnvironment(t *testing.T) {
	base := t.TempDir()
	mgr := setupUseCaseProject(t, base, "myapp", true)

	// Create staging the first time -- should succeed.
	_, err := mgr.Create("myapp", "staging", "staging.app.com", "develop")
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}

	// Try to create staging again -- should fail with a clear message.
	_, err = mgr.Create("myapp", "staging", "staging.app.com", "develop")
	if err == nil {
		t.Fatal("duplicate Create should return an error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "already exists") {
		t.Errorf("error should mention 'already exists', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "staging") {
		t.Errorf("error should mention env name 'staging', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "myapp") {
		t.Errorf("error should mention project name 'myapp', got: %s", errMsg)
	}
}

// TestUseCaseDeleteThenRecreate simulates a developer deleting a broken staging
// environment and recreating it with a different domain (e.g. after a DNS change).
func TestUseCaseDeleteThenRecreate(t *testing.T) {
	base := t.TempDir()
	mgr := setupUseCaseProject(t, base, "myapp", true)

	// Create the original staging environment.
	_, err := mgr.Create("myapp", "staging", "staging.old-domain.com", "develop")
	if err != nil {
		t.Fatalf("initial Create: %v", err)
	}

	// Delete it.
	if err := mgr.Delete("myapp", "staging"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify the directory is gone.
	envPath := mgr.GetEnvPath("myapp", "staging")
	if _, err := os.Stat(envPath); !os.IsNotExist(err) {
		t.Fatal("environment directory should be removed after delete")
	}

	// Recreate with a different domain.
	newDomain := "staging.new-domain.com"
	env, err := mgr.Create("myapp", "staging", newDomain, "develop")
	if err != nil {
		t.Fatalf("recreate Create: %v", err)
	}

	// Verify the new domain is used everywhere.
	if env.Domain != newDomain {
		t.Errorf("Domain = %q, want %q", env.Domain, newDomain)
	}

	composeData, err := os.ReadFile(filepath.Join(envPath, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("reading compose: %v", err)
	}
	if !strings.Contains(string(composeData), newDomain) {
		t.Error("recreated compose should contain the new domain")
	}
	if strings.Contains(string(composeData), "staging.old-domain.com") {
		t.Error("recreated compose should NOT contain the old domain")
	}

	envData, err := os.ReadFile(filepath.Join(envPath, ".env"))
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}
	if !strings.Contains(string(envData), newDomain) {
		t.Error("recreated .env should contain the new domain")
	}
}

// TestUseCasePromoteStagingToProduction simulates the standard promotion
// workflow: a staging environment with a specific config is promoted to
// production, and the domain prefix changes from staging.* to production.*.
func TestUseCasePromoteStagingToProduction(t *testing.T) {
	base := t.TempDir()

	// Set up the source project.
	projectName := "myapp"
	projectDir := filepath.Join(base, projectName)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	composeContent := `services:
  app:
    build: .
    image: myapp:latest
    labels:
      - "traefik.http.routers.myapp.rule=Host(` + "`staging.app.com`" + `)"
`
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(composeContent), 0644); err != nil {
		t.Fatal(err)
	}

	envContent := "APP_DOMAIN=staging.app.com\nSECRET_KEY=s3cret\n"
	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(base)

	// Create staging environment.
	_, err := mgr.Create(projectName, "staging", "staging.app.com", "develop")
	if err != nil {
		t.Fatalf("Create staging: %v", err)
	}

	// Write a specific .env into the staging env (the manager creates one, but
	// we overwrite to be explicit about what we want to test).
	stagingPath := mgr.GetEnvPath(projectName, "staging")
	stagingEnv := "APP_DOMAIN=staging.app.com\nSECRET_KEY=s3cret\nDB_URL=postgres://localhost/mydb\n"
	if err := os.WriteFile(filepath.Join(stagingPath, ".env"), []byte(stagingEnv), 0644); err != nil {
		t.Fatal(err)
	}

	// Promote staging to production.
	if err := mgr.Promote(projectName, "staging", "production"); err != nil {
		t.Fatalf("Promote: %v", err)
	}

	// Verify domain was changed from staging.app.com to production.app.com.
	prodPath := mgr.GetEnvPath(projectName, "production")
	prodMeta, err := mgr.loadMetadata(prodPath)
	if err != nil {
		t.Fatalf("loading production metadata: %v", err)
	}
	if prodMeta.Domain != "production.app.com" {
		t.Errorf("production domain = %q, want %q", prodMeta.Domain, "production.app.com")
	}

	// Verify the compose file has the production domain.
	prodCompose, err := os.ReadFile(filepath.Join(prodPath, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("reading production compose: %v", err)
	}
	if !strings.Contains(string(prodCompose), "production.app.com") {
		t.Error("production compose should contain production.app.com")
	}
	if strings.Contains(string(prodCompose), "staging.app.com") {
		t.Error("production compose should NOT contain staging.app.com")
	}

	// Verify the .env has the production domain.
	prodEnvData, err := os.ReadFile(filepath.Join(prodPath, ".env"))
	if err != nil {
		t.Fatalf("reading production .env: %v", err)
	}
	if !strings.Contains(string(prodEnvData), "production.app.com") {
		t.Error("production .env should contain production.app.com")
	}
	if strings.Contains(string(prodEnvData), "staging.app.com") {
		t.Error("production .env should NOT contain staging.app.com")
	}
	// Non-domain values should be preserved.
	if !strings.Contains(string(prodEnvData), "DB_URL=postgres://localhost/mydb") {
		t.Error("production .env should preserve DB_URL")
	}
}

// TestUseCasePreviewBranch simulates creating a preview environment for a
// feature branch. The branch should be stored in metadata so CI/CD can
// deploy the correct branch to this environment.
func TestUseCasePreviewBranch(t *testing.T) {
	base := t.TempDir()
	mgr := setupUseCaseProject(t, base, "api", true)

	branchName := "feature/user-auth"
	env, err := mgr.Create("api", "preview-42", "preview-42.api.app.com", branchName)
	if err != nil {
		t.Fatalf("Create preview: %v", err)
	}

	// Verify the branch is stored in the returned environment.
	if env.Branch != branchName {
		t.Errorf("Branch = %q, want %q", env.Branch, branchName)
	}

	// Verify the branch is persisted in metadata.
	envPath := mgr.GetEnvPath("api", "preview-42")
	loaded, err := mgr.loadMetadata(envPath)
	if err != nil {
		t.Fatalf("loading metadata: %v", err)
	}
	if loaded.Branch != branchName {
		t.Errorf("persisted Branch = %q, want %q", loaded.Branch, branchName)
	}
	if loaded.Name != "preview-42" {
		t.Errorf("persisted Name = %q, want %q", loaded.Name, "preview-42")
	}
}

// TestUseCaseListAfterCreateDeletePromote exercises a complex lifecycle:
// create 3 environments, delete 1, promote 1 to a new name, then list and
// verify correct count and names. This simulates a typical multi-environment
// management workflow.
func TestUseCaseListAfterCreateDeletePromote(t *testing.T) {
	base := t.TempDir()
	mgr := setupUseCaseProject(t, base, "shop", true)

	// Create 3 environments.
	for _, cfg := range []struct {
		name   string
		domain string
		branch string
	}{
		{"staging", "staging.shop.app.com", "develop"},
		{"preview-1", "preview-1.shop.app.com", "feature/cart"},
		{"preview-2", "preview-2.shop.app.com", "feature/checkout"},
	} {
		if _, err := mgr.Create("shop", cfg.name, cfg.domain, cfg.branch); err != nil {
			t.Fatalf("Create %s: %v", cfg.name, err)
		}
	}

	// Verify all 3 exist.
	envs, err := mgr.List("shop")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(envs) != 3 {
		t.Fatalf("expected 3 environments, got %d", len(envs))
	}

	// Delete preview-1.
	if err := mgr.Delete("shop", "preview-1"); err != nil {
		t.Fatalf("Delete preview-1: %v", err)
	}

	// Promote staging to production.
	if err := mgr.Promote("shop", "staging", "production"); err != nil {
		t.Fatalf("Promote staging->production: %v", err)
	}

	// List and verify: should have staging, preview-2, production (3 total).
	// preview-1 was deleted, production was added via promote.
	envs, err = mgr.List("shop")
	if err != nil {
		t.Fatalf("List after operations: %v", err)
	}
	if len(envs) != 3 {
		t.Fatalf("expected 3 environments after delete+promote, got %d", len(envs))
	}

	names := make(map[string]bool)
	for _, e := range envs {
		names[e.Name] = true
	}

	expectedNames := []string{"staging", "preview-2", "production"}
	for _, name := range expectedNames {
		if !names[name] {
			t.Errorf("expected environment %q in list, not found", name)
		}
	}
	if names["preview-1"] {
		t.Error("deleted environment preview-1 should not be in the list")
	}

	// Verify the production environment has the correct domain.
	for _, e := range envs {
		if e.Name == "production" {
			if e.Domain != "production.shop.app.com" {
				t.Errorf("production domain = %q, want %q", e.Domain, "production.shop.app.com")
			}
		}
	}
}

// TestUseCaseCreateWithoutSourceEnvFile verifies that creating an environment
// from a source project that has docker-compose.yml but no .env file works
// without error. This is common for simple projects that use inline env vars.
func TestUseCaseCreateWithoutSourceEnvFile(t *testing.T) {
	base := t.TempDir()
	mgr := setupUseCaseProject(t, base, "simple-app", false) // no .env

	env, err := mgr.Create("simple-app", "staging", "staging.simple.com", "develop")
	if err != nil {
		t.Fatalf("Create should succeed without .env, got: %v", err)
	}

	if env.Name != "staging" {
		t.Errorf("Name = %q, want %q", env.Name, "staging")
	}

	// Verify docker-compose.yml was created.
	envPath := mgr.GetEnvPath("simple-app", "staging")
	if _, err := os.Stat(filepath.Join(envPath, "docker-compose.yml")); os.IsNotExist(err) {
		t.Error("docker-compose.yml should exist in env directory")
	}

	// Verify no .env was created (source has none).
	if _, err := os.Stat(filepath.Join(envPath, ".env")); !os.IsNotExist(err) {
		t.Error(".env should NOT exist when source has none")
	}

	// Verify environment.json metadata was still created.
	if _, err := os.Stat(filepath.Join(envPath, "environment.json")); os.IsNotExist(err) {
		t.Error("environment.json should exist")
	}
}

// TestUseCaseCreateWithRichComposeFile verifies that a source compose file
// with multiple Host() rules and traefik labels has all Host rules adjusted
// in the staging environment.
func TestUseCaseCreateWithRichComposeFile(t *testing.T) {
	base := t.TempDir()
	projectName := "multiservice"
	projectDir := filepath.Join(base, projectName)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Rich compose file with multiple services, each with Host() rules.
	richCompose := `services:
  web:
    image: web:latest
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.web.rule=Host(` + "`prod.example.com`" + `)"
      - "traefik.http.routers.web.entrypoints=websecure"
      - "traefik.http.routers.web.tls.certresolver=letsencrypt"
  api:
    image: api:latest
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.api.rule=Host(` + "`api.prod.example.com`" + `)"
      - "traefik.http.routers.api.entrypoints=websecure"
  worker:
    image: worker:latest
    labels:
      - "traefik.enable=false"
`
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(richCompose), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(base)
	stagingDomain := "staging.example.com"
	_, err := mgr.Create(projectName, "staging", stagingDomain, "develop")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	envComposePath := filepath.Join(mgr.GetEnvPath(projectName, "staging"), "docker-compose.yml")
	data, err := os.ReadFile(envComposePath)
	if err != nil {
		t.Fatalf("reading env compose: %v", err)
	}
	content := string(data)

	// Both Host() rules should be adjusted to the staging domain.
	// replaceHostRule replaces the domain inside each Host() with the target domain.
	hostCount := strings.Count(content, "Host(`"+stagingDomain+"`)")
	if hostCount != 2 {
		t.Errorf("expected 2 Host() rules with staging domain, got %d.\nCompose:\n%s", hostCount, content)
	}

	// Non-Host labels should be preserved unchanged.
	if !strings.Contains(content, "traefik.http.routers.web.entrypoints=websecure") {
		t.Error("web entrypoints label should be preserved")
	}
	if !strings.Contains(content, "traefik.http.routers.web.tls.certresolver=letsencrypt") {
		t.Error("web tls certresolver label should be preserved")
	}
	if !strings.Contains(content, "traefik.enable=false") {
		t.Error("worker traefik.enable=false should be preserved")
	}

	// The compose should include a project name prefix.
	if !strings.Contains(content, "name: staging-multiservice") {
		t.Error("compose should include environment-prefixed project name")
	}
}
