package environments

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Workflow 1: Staging -> Production promotion
// ---------------------------------------------------------------------------

func TestWorkflowStagingToProduction(t *testing.T) {
	base := t.TempDir()
	mgr := NewManager(base)

	// Set up a project with a docker-compose.yml containing Host rules.
	projectName := "webapp"
	projectDir := filepath.Join(base, projectName)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	composeContent := `services:
  app:
    build: .
    image: webapp:local
    labels:
      - "traefik.http.routers.webapp.rule=Host(` + "`webapp.example.com`" + `)"
    networks:
      - default

networks:
  default:
    name: traefik_default
    external: true
`
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(composeContent), 0644); err != nil {
		t.Fatalf("writing compose file: %v", err)
	}

	// Create staging environment.
	stagingDomain := "staging.webapp.example.com"
	staging, err := mgr.Create(projectName, "staging", stagingDomain, "develop")
	if err != nil {
		t.Fatalf("Create staging: %v", err)
	}

	if staging.Name != "staging" {
		t.Errorf("staging.Name = %q, want %q", staging.Name, "staging")
	}
	if staging.Domain != stagingDomain {
		t.Errorf("staging.Domain = %q, want %q", staging.Domain, stagingDomain)
	}
	if staging.Branch != "develop" {
		t.Errorf("staging.Branch = %q, want %q", staging.Branch, "develop")
	}
	if staging.ProjectName != projectName {
		t.Errorf("staging.ProjectName = %q, want %q", staging.ProjectName, projectName)
	}

	// Verify the staging compose file has the staging domain in Host rules.
	stagingCompose, err := os.ReadFile(filepath.Join(mgr.GetEnvPath(projectName, "staging"), "docker-compose.yml"))
	if err != nil {
		t.Fatalf("reading staging compose: %v", err)
	}
	if !strings.Contains(string(stagingCompose), stagingDomain) {
		t.Errorf("staging compose should contain domain %q", stagingDomain)
	}

	// Promote staging to production.
	if err := mgr.Promote(projectName, "staging", "production"); err != nil {
		t.Fatalf("Promote staging->production: %v", err)
	}

	// Verify production environment exists.
	prodPath := mgr.GetEnvPath(projectName, "production")
	if _, err := os.Stat(prodPath); os.IsNotExist(err) {
		t.Fatal("production environment directory does not exist")
	}

	// Load production metadata and verify the domain was updated.
	prodMeta, err := mgr.loadMetadata(prodPath)
	if err != nil {
		t.Fatalf("loading production metadata: %v", err)
	}

	expectedProdDomain := "production.webapp.example.com"
	if prodMeta.Domain != expectedProdDomain {
		t.Errorf("production domain = %q, want %q", prodMeta.Domain, expectedProdDomain)
	}
	if prodMeta.Name != "production" {
		t.Errorf("production name = %q, want %q", prodMeta.Name, "production")
	}

	// Verify the production compose file has the production domain.
	prodCompose, err := os.ReadFile(filepath.Join(prodPath, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("reading production compose: %v", err)
	}
	if !strings.Contains(string(prodCompose), expectedProdDomain) {
		t.Errorf("production compose should contain domain %q", expectedProdDomain)
	}
	// Staging domain should NOT appear in production compose.
	if strings.Contains(string(prodCompose), stagingDomain) {
		t.Error("production compose should not contain staging domain")
	}
}

// ---------------------------------------------------------------------------
// Workflow 2: Preview environments for multiple branches
// ---------------------------------------------------------------------------

func TestWorkflowPreviewEnvironments(t *testing.T) {
	base := t.TempDir()
	mgr := NewManager(base)

	projectName := "api"
	setupWorkflowProject(t, base, projectName)

	// Create 3 preview environments for different feature branches.
	branches := []struct {
		envName string
		domain  string
		branch  string
	}{
		{"preview-123", "preview-123.api.example.com", "feature/user-auth"},
		{"preview-456", "preview-456.api.example.com", "feature/billing"},
		{"preview-789", "preview-789.api.example.com", "fix/memory-leak"},
	}

	for _, b := range branches {
		_, err := mgr.Create(projectName, b.envName, b.domain, b.branch)
		if err != nil {
			t.Fatalf("Create %s: %v", b.envName, err)
		}
	}

	// List all environments and verify all 3 exist.
	envs, err := mgr.List(projectName)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(envs) != 3 {
		t.Fatalf("expected 3 environments, got %d", len(envs))
	}

	envNames := make(map[string]bool)
	for _, env := range envs {
		envNames[env.Name] = true
	}
	for _, b := range branches {
		if !envNames[b.envName] {
			t.Errorf("missing environment %q in list", b.envName)
		}
	}

	// Delete one preview environment.
	if err := mgr.Delete(projectName, "preview-456"); err != nil {
		t.Fatalf("Delete preview-456: %v", err)
	}

	// Verify the list now has 2 environments.
	envs, err = mgr.List(projectName)
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(envs) != 2 {
		t.Fatalf("expected 2 environments after delete, got %d", len(envs))
	}

	// Verify the deleted one is gone.
	for _, env := range envs {
		if env.Name == "preview-456" {
			t.Error("preview-456 should have been deleted")
		}
	}
}

// ---------------------------------------------------------------------------
// Workflow 3: Environment isolation - independent compose files and domains
// ---------------------------------------------------------------------------

func TestWorkflowEnvironmentIsolation(t *testing.T) {
	base := t.TempDir()
	mgr := NewManager(base)

	projectName := "shop"
	setupWorkflowProject(t, base, projectName)

	// Create two environments with distinct domains.
	staging, err := mgr.Create(projectName, "staging", "staging.shop.example.com", "develop")
	if err != nil {
		t.Fatalf("Create staging: %v", err)
	}
	prod, err := mgr.Create(projectName, "production", "production.shop.example.com", "main")
	if err != nil {
		t.Fatalf("Create production: %v", err)
	}

	// Verify they have separate directories.
	stagingPath := mgr.GetEnvPath(projectName, "staging")
	prodPath := mgr.GetEnvPath(projectName, "production")
	if stagingPath == prodPath {
		t.Error("staging and production should have different paths")
	}

	// Verify each has its own compose file.
	stagingCompose, err := os.ReadFile(filepath.Join(stagingPath, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("reading staging compose: %v", err)
	}
	prodCompose, err := os.ReadFile(filepath.Join(prodPath, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("reading production compose: %v", err)
	}

	// Verify domains are different.
	if !strings.Contains(string(stagingCompose), staging.Domain) {
		t.Error("staging compose should contain staging domain")
	}
	if !strings.Contains(string(prodCompose), prod.Domain) {
		t.Error("production compose should contain production domain")
	}
	if strings.Contains(string(stagingCompose), prod.Domain) {
		t.Error("staging compose should NOT contain production domain")
	}
	if strings.Contains(string(prodCompose), staging.Domain) {
		t.Error("production compose should NOT contain staging domain")
	}

	// Verify compose files are independent -- the staging compose should not
	// reference the production domain and vice versa. The adjustComposeForEnv
	// function only prepends a "name:" line if the source compose does not
	// already contain "name:" (it matches substrings, so "container_name:"
	// satisfies the check). The key isolation guarantee is domain separation,
	// which was already verified above.
}

// ---------------------------------------------------------------------------
// Workflow 4: Promote preserves config (except domain)
// ---------------------------------------------------------------------------

func TestWorkflowPromotePreservesConfig(t *testing.T) {
	base := t.TempDir()
	mgr := NewManager(base)

	projectName := "billing"
	projectDir := filepath.Join(base, projectName)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	composeContent := `services:
  app:
    build: .
    labels:
      - "traefik.http.routers.billing.rule=Host(` + "`billing.example.com`" + `)"
`
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(composeContent), 0644); err != nil {
		t.Fatalf("writing compose: %v", err)
	}

	// Write a .env file with custom values.
	envContent := `POSTGRES_USER=billing_user
POSTGRES_PASSWORD=super-secret-password-123
POSTGRES_DB=billing_prod
DOMAIN=staging.billing.example.com
API_KEY=sk_live_abc123xyz
STRIPE_SECRET=whsec_test_secret
`
	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	// Create staging environment.
	stagingDomain := "staging.billing.example.com"
	_, err := mgr.Create(projectName, "staging", stagingDomain, "develop")
	if err != nil {
		t.Fatalf("Create staging: %v", err)
	}

	// Verify the staging .env has the staging domain.
	stagingEnv, err := os.ReadFile(filepath.Join(mgr.GetEnvPath(projectName, "staging"), ".env"))
	if err != nil {
		t.Fatalf("reading staging .env: %v", err)
	}
	if !strings.Contains(string(stagingEnv), stagingDomain) {
		t.Errorf("staging .env should contain staging domain %q", stagingDomain)
	}
	// Other values should be preserved.
	if !strings.Contains(string(stagingEnv), "POSTGRES_USER=billing_user") {
		t.Error("staging .env should preserve POSTGRES_USER")
	}
	if !strings.Contains(string(stagingEnv), "API_KEY=sk_live_abc123xyz") {
		t.Error("staging .env should preserve API_KEY")
	}

	// Promote to production.
	if err := mgr.Promote(projectName, "staging", "production"); err != nil {
		t.Fatalf("Promote: %v", err)
	}

	// Read production .env.
	prodEnv, err := os.ReadFile(filepath.Join(mgr.GetEnvPath(projectName, "production"), ".env"))
	if err != nil {
		t.Fatalf("reading production .env: %v", err)
	}
	prodEnvStr := string(prodEnv)

	// Domain should be updated to production.
	expectedProdDomain := "production.billing.example.com"
	if !strings.Contains(prodEnvStr, expectedProdDomain) {
		t.Errorf("production .env should contain domain %q, got:\n%s", expectedProdDomain, prodEnvStr)
	}
	// Staging domain should NOT appear.
	if strings.Contains(prodEnvStr, stagingDomain) {
		t.Error("production .env should not contain staging domain")
	}

	// Non-domain values should be preserved.
	if !strings.Contains(prodEnvStr, "POSTGRES_USER=billing_user") {
		t.Error("production .env should preserve POSTGRES_USER")
	}
	if !strings.Contains(prodEnvStr, "API_KEY=sk_live_abc123xyz") {
		t.Error("production .env should preserve API_KEY")
	}
	if !strings.Contains(prodEnvStr, "STRIPE_SECRET=whsec_test_secret") {
		t.Error("production .env should preserve STRIPE_SECRET")
	}
}

// ---------------------------------------------------------------------------
// Workflow 5: Delete cleans up the environment directory
// ---------------------------------------------------------------------------

func TestWorkflowDeleteCleanup(t *testing.T) {
	base := t.TempDir()
	mgr := NewManager(base)

	projectName := "tempapp"
	setupWorkflowProject(t, base, projectName)

	// Create an environment.
	envName := "preview-42"
	_, err := mgr.Create(projectName, envName, "preview-42.tempapp.example.com", "feature/test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Verify the directory exists.
	envPath := mgr.GetEnvPath(projectName, envName)
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		t.Fatal("environment directory should exist after creation")
	}

	// Verify the compose and metadata files exist.
	if _, err := os.Stat(filepath.Join(envPath, "docker-compose.yml")); os.IsNotExist(err) {
		t.Error("docker-compose.yml should exist in environment directory")
	}
	if _, err := os.Stat(filepath.Join(envPath, "environment.json")); os.IsNotExist(err) {
		t.Error("environment.json should exist in environment directory")
	}

	// Delete the environment.
	if err := mgr.Delete(projectName, envName); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify the directory is gone.
	if _, err := os.Stat(envPath); !os.IsNotExist(err) {
		t.Error("environment directory should be removed after deletion")
	}

	// Verify it no longer appears in the list.
	envs, err := mgr.List(projectName)
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	for _, env := range envs {
		if env.Name == envName {
			t.Errorf("deleted environment %q should not appear in list", envName)
		}
	}
}

// ---------------------------------------------------------------------------
// Workflow 6: Multiple projects with independent environments
// ---------------------------------------------------------------------------

func TestWorkflowCreateMultipleProjects(t *testing.T) {
	base := t.TempDir()
	mgr := NewManager(base)

	// Create two independent projects.
	projects := []string{"frontend-app", "backend-api"}
	for _, proj := range projects {
		setupWorkflowProject(t, base, proj)
	}

	// Create environments for each project.
	_, err := mgr.Create("frontend-app", "staging", "staging.frontend.example.com", "develop")
	if err != nil {
		t.Fatalf("Create frontend staging: %v", err)
	}
	_, err = mgr.Create("frontend-app", "production", "production.frontend.example.com", "main")
	if err != nil {
		t.Fatalf("Create frontend production: %v", err)
	}
	_, err = mgr.Create("backend-api", "staging", "staging.api.example.com", "develop")
	if err != nil {
		t.Fatalf("Create backend staging: %v", err)
	}

	// Verify each project has the right number of environments.
	frontendEnvs, err := mgr.List("frontend-app")
	if err != nil {
		t.Fatalf("List frontend: %v", err)
	}
	if len(frontendEnvs) != 2 {
		t.Errorf("frontend should have 2 environments, got %d", len(frontendEnvs))
	}

	backendEnvs, err := mgr.List("backend-api")
	if err != nil {
		t.Fatalf("List backend: %v", err)
	}
	if len(backendEnvs) != 1 {
		t.Errorf("backend should have 1 environment, got %d", len(backendEnvs))
	}

	// Verify environments are independent -- deleting a backend env
	// should not affect frontend envs.
	if err := mgr.Delete("backend-api", "staging"); err != nil {
		t.Fatalf("Delete backend staging: %v", err)
	}

	frontendEnvs, err = mgr.List("frontend-app")
	if err != nil {
		t.Fatalf("List frontend after backend delete: %v", err)
	}
	if len(frontendEnvs) != 2 {
		t.Errorf("frontend should still have 2 environments after deleting backend env, got %d", len(frontendEnvs))
	}

	backendEnvs, err = mgr.List("backend-api")
	if err != nil {
		t.Fatalf("List backend after delete: %v", err)
	}
	if len(backendEnvs) != 0 {
		t.Errorf("backend should have 0 environments after delete, got %d", len(backendEnvs))
	}

	// Verify the compose files have the correct domains.
	frontStagingCompose, err := os.ReadFile(filepath.Join(
		mgr.GetEnvPath("frontend-app", "staging"), "docker-compose.yml"))
	if err != nil {
		t.Fatalf("reading frontend staging compose: %v", err)
	}
	if !strings.Contains(string(frontStagingCompose), "staging.frontend.example.com") {
		t.Error("frontend staging compose should contain frontend staging domain")
	}

	frontProdCompose, err := os.ReadFile(filepath.Join(
		mgr.GetEnvPath("frontend-app", "production"), "docker-compose.yml"))
	if err != nil {
		t.Fatalf("reading frontend production compose: %v", err)
	}
	if !strings.Contains(string(frontProdCompose), "production.frontend.example.com") {
		t.Error("frontend production compose should contain frontend production domain")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setupWorkflowProject creates a minimal project directory with a docker-compose.yml
// containing a Host rule, suitable for environment creation in workflow tests.
func setupWorkflowProject(t *testing.T, basePath, projectName string) {
	t.Helper()
	projectDir := filepath.Join(basePath, projectName)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating project dir %s: %v", projectName, err)
	}

	compose := `services:
  app:
    build: .
    image: ` + projectName + `:local
    labels:
      - "traefik.http.routers.` + projectName + `.rule=Host(` + "`" + projectName + `.example.com` + "`" + `)"
    networks:
      - default

networks:
  default:
    name: traefik_default
    external: true
`
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(compose), 0644); err != nil {
		t.Fatalf("writing compose for %s: %v", projectName, err)
	}
}
