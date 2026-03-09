package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/templates"
)

// testTemplate returns a minimal template for testing scaffold logic.
func testTemplate() *templates.Template {
	return &templates.Template{
		Name:        "test",
		Description: "Test template",
		Dockerfile: `FROM alpine:latest
WORKDIR /app
LABEL project="{{.Name}}"
`,
		Compose: `services:
  app:
    build: .
    image: {{.Name}}:local
    container_name: {{.Name}}-app
    labels:
      - "traefik.http.routers.{{.Name}}.rule=Host(` + "`{{.Domain}}`" + `)"
`,
		Workflow: `name: Deploy
on:
  push:
    branches: [main]
`,
		EnvTemplate: `APP_NAME={{.Name}}
APP_DOMAIN={{.Domain}}
`,
		GitIgnore: `.env
*.log
node_modules/
`,
	}
}

func testData() templates.TemplateData {
	return templates.TemplateData{
		Name:   "myproject",
		Domain: "myproject.example.com",
	}
}

func TestScaffoldProject(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	tmpl := testTemplate()
	data := testData()

	if err := ScaffoldProject(projectPath, tmpl, data); err != nil {
		t.Fatalf("ScaffoldProject() error: %v", err)
	}

	// Verify expected files exist
	expectedFiles := []string{
		"Dockerfile",
		"docker-compose.yml",
		".gitignore",
		filepath.Join(".github", "workflows", "deploy.yml"),
	}
	for _, f := range expectedFiles {
		path := filepath.Join(projectPath, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file %q does not exist", f)
		}
	}
}

func TestScaffoldProjectCreatesDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	tmpl := testTemplate()
	data := testData()

	if err := ScaffoldProject(projectPath, tmpl, data); err != nil {
		t.Fatalf("ScaffoldProject() error: %v", err)
	}

	// Verify nested directories were created
	expectedDirs := []string{
		filepath.Join(projectPath, ".github"),
		filepath.Join(projectPath, ".github", "workflows"),
		filepath.Join(projectPath, "deployments"),
	}
	for _, d := range expectedDirs {
		info, err := os.Stat(d)
		if os.IsNotExist(err) {
			t.Errorf("expected directory %q does not exist", d)
			continue
		}
		if err != nil {
			t.Errorf("stat %q: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%q should be a directory", d)
		}
	}
}

func TestScaffoldProjectTemplateRendering(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	tmpl := testTemplate()
	data := testData()

	if err := ScaffoldProject(projectPath, tmpl, data); err != nil {
		t.Fatalf("ScaffoldProject() error: %v", err)
	}

	// Verify Dockerfile template variables are replaced
	dockerfile, err := os.ReadFile(filepath.Join(projectPath, "Dockerfile"))
	if err != nil {
		t.Fatalf("reading Dockerfile: %v", err)
	}
	if strings.Contains(string(dockerfile), "{{.Name}}") {
		t.Error("Dockerfile still contains unrendered {{.Name}} template variable")
	}
	if !strings.Contains(string(dockerfile), "myproject") {
		t.Error("Dockerfile should contain rendered project name 'myproject'")
	}

	// Verify docker-compose.yml has both Name and Domain rendered
	compose, err := os.ReadFile(filepath.Join(projectPath, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("reading docker-compose.yml: %v", err)
	}
	if strings.Contains(string(compose), "{{.Name}}") {
		t.Error("docker-compose.yml still contains unrendered {{.Name}}")
	}
	if strings.Contains(string(compose), "{{.Domain}}") {
		t.Error("docker-compose.yml still contains unrendered {{.Domain}}")
	}
	if !strings.Contains(string(compose), "myproject") {
		t.Error("docker-compose.yml should contain rendered project name")
	}
	if !strings.Contains(string(compose), "myproject.example.com") {
		t.Error("docker-compose.yml should contain rendered domain")
	}

	// Verify .gitignore is written as-is (no templates to render)
	gitignore, err := os.ReadFile(filepath.Join(projectPath, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if !strings.Contains(string(gitignore), ".env") {
		t.Error(".gitignore should contain .env")
	}

	// Verify workflow is written as-is
	workflow, err := os.ReadFile(filepath.Join(projectPath, ".github", "workflows", "deploy.yml"))
	if err != nil {
		t.Fatalf("reading deploy.yml: %v", err)
	}
	if !strings.Contains(string(workflow), "name: Deploy") {
		t.Error("deploy.yml should contain workflow name")
	}
}

func TestScaffoldProjectFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	tmpl := testTemplate()
	data := testData()

	if err := ScaffoldProject(projectPath, tmpl, data); err != nil {
		t.Fatalf("ScaffoldProject() error: %v", err)
	}

	// Verify files have 0644 permissions
	filesToCheck := []string{
		"Dockerfile",
		"docker-compose.yml",
		".gitignore",
		filepath.Join(".github", "workflows", "deploy.yml"),
	}
	for _, f := range filesToCheck {
		info, err := os.Stat(filepath.Join(projectPath, f))
		if err != nil {
			t.Errorf("stat %q: %v", f, err)
			continue
		}
		perm := info.Mode().Perm()
		if perm != 0644 {
			t.Errorf("%q permissions = %o, want 0644", f, perm)
		}
	}
}

func TestScaffoldProjectDifferentData(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "webapp")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	tmpl := testTemplate()
	data := templates.TemplateData{
		Name:   "webapp",
		Domain: "app.staging.dev",
	}

	if err := ScaffoldProject(projectPath, tmpl, data); err != nil {
		t.Fatalf("ScaffoldProject() error: %v", err)
	}

	compose, err := os.ReadFile(filepath.Join(projectPath, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("reading docker-compose.yml: %v", err)
	}
	if !strings.Contains(string(compose), "webapp") {
		t.Error("docker-compose.yml should contain 'webapp'")
	}
	if !strings.Contains(string(compose), "app.staging.dev") {
		t.Error("docker-compose.yml should contain 'app.staging.dev'")
	}
}
