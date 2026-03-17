package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllTemplatesRegistered(t *testing.T) {
	expected := []string{"node", "python", "go", "static", "nextjs", "nestjs", "custom"}
	for _, name := range expected {
		tmpl, err := Get(name)
		if err != nil {
			t.Errorf("template %q not registered: %v", name, err)
			continue
		}
		if tmpl.Name != name {
			t.Errorf("template name mismatch: expected %q, got %q", name, tmpl.Name)
		}
		if tmpl.Description == "" {
			t.Errorf("template %q has empty description", name)
		}
		if tmpl.Dockerfile == "" {
			t.Errorf("template %q has empty Dockerfile", name)
		}
		if tmpl.Compose == "" {
			t.Errorf("template %q has empty Compose", name)
		}
		if tmpl.Workflow == "" {
			t.Errorf("template %q has empty Workflow", name)
		}
		if tmpl.GitIgnore == "" {
			t.Errorf("template %q has empty GitIgnore", name)
		}
	}
}

func TestListTemplates(t *testing.T) {
	templates := List()
	if len(templates) < 7 {
		t.Errorf("expected at least 7 templates, got %d", len(templates))
	}
}

func TestGetNonexistentTemplate(t *testing.T) {
	_, err := Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent template")
	}
}

func TestRenderTemplate(t *testing.T) {
	data := TemplateData{
		Name:            "myapp",
		Domain:          "myapp.example.com",
		PostgresVersion: "15-alpine",
	}

	tmpl, _ := Get("node")

	// Render compose
	result, err := Render(tmpl.Compose, data)
	if err != nil {
		t.Fatalf("render compose: %v", err)
	}
	if !strings.Contains(result, "myapp") {
		t.Error("expected rendered compose to contain project name")
	}
	if !strings.Contains(result, "myapp.example.com") {
		t.Error("expected rendered compose to contain domain")
	}
	if !strings.Contains(result, "15-alpine") {
		t.Error("expected rendered compose to contain postgres version")
	}

	// Render dockerfile
	result, err = Render(tmpl.Dockerfile, data)
	if err != nil {
		t.Fatalf("render dockerfile: %v", err)
	}
	if !strings.Contains(result, "FROM") {
		t.Error("expected Dockerfile to contain FROM")
	}
}

func TestRenderEnvTemplate(t *testing.T) {
	data := TemplateData{Name: "testapp"}
	tmpl, _ := Get("node")

	result, err := Render(tmpl.EnvTemplate, data)
	if err != nil {
		t.Fatalf("render env: %v", err)
	}
	if !strings.Contains(result, "POSTGRES_USER=testapp") {
		t.Error("expected env to contain POSTGRES_USER")
	}
}

func TestRenderInvalidTemplate(t *testing.T) {
	_, err := Render("{{.Invalid", TemplateData{})
	if err == nil {
		t.Error("expected error for invalid template syntax")
	}
}

func TestTemplatesHaveTraefikLabels(t *testing.T) {
	for _, name := range []string{"node", "python", "go", "nextjs", "nestjs", "static", "custom"} {
		tmpl, _ := Get(name)
		if !strings.Contains(tmpl.Compose, "traefik.enable=true") {
			t.Errorf("template %q is missing traefik.enable label", name)
		}
		if !strings.Contains(tmpl.Compose, "traefik.http.routers") {
			t.Errorf("template %q is missing traefik router label", name)
		}
	}
}

func TestTemplatesUseLetsencryptCertResolver(t *testing.T) {
	for _, name := range []string{"node", "python", "go", "nextjs", "nestjs", "static", "custom"} {
		tmpl, _ := Get(name)
		if !strings.Contains(tmpl.Compose, "certresolver=letsencrypt") {
			t.Errorf("template %q should use certresolver=letsencrypt", name)
		}
		if strings.Contains(tmpl.Compose, "certresolver=myresolver") {
			t.Errorf("template %q still uses deprecated certresolver=myresolver", name)
		}
	}
}

func TestTemplatesWithPostgres(t *testing.T) {
	dbTemplates := []string{"node", "python", "go", "nextjs", "nestjs"}
	for _, name := range dbTemplates {
		tmpl, _ := Get(name)
		if !strings.Contains(tmpl.Compose, "postgres") {
			t.Errorf("template %q should include postgres service", name)
		}
		if !strings.Contains(tmpl.Compose, "healthcheck") {
			t.Errorf("template %q should include healthcheck for postgres", name)
		}
	}
}

func TestStaticTemplateNoDatabase(t *testing.T) {
	tmpl, _ := Get("static")
	if strings.Contains(tmpl.Compose, "postgres") {
		t.Error("static template should not include postgres")
	}
}

func TestRegisterCustomTemplate(t *testing.T) {
	custom := &Template{
		Name:        "test-custom",
		Description: "Test custom template",
		Dockerfile:  "FROM alpine",
		Compose:     "services: {}",
		Workflow:     SharedWorkflow,
		GitIgnore:   ".env",
	}
	Register(custom)

	got, err := Get("test-custom")
	if err != nil {
		t.Fatalf("get custom: %v", err)
	}
	if got.Dockerfile != "FROM alpine" {
		t.Error("custom template Dockerfile mismatch")
	}

	// Cleanup: remove from registry to not affect other tests
	delete(registry, "test-custom")
}

// --- New tests for improved coverage ---

func TestRenderWithEmptyData(t *testing.T) {
	result, err := Render("Hello {{.Name}}", TemplateData{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello " {
		t.Errorf("expected 'Hello ', got %q", result)
	}
}

func TestRenderWithAllFieldsEmpty(t *testing.T) {
	tmpl := "{{.Name}}-{{.Domain}}-{{.PostgresVersion}}"
	result, err := Render(tmpl, TemplateData{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "--" {
		t.Errorf("expected '--', got %q", result)
	}
}

func TestRenderPlainStringNoTemplateActions(t *testing.T) {
	result, err := Render("no template actions here", TemplateData{Name: "ignored"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "no template actions here" {
		t.Errorf("expected plain string, got %q", result)
	}
}

func TestRenderEmptyTemplate(t *testing.T) {
	result, err := Render("", TemplateData{Name: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestRenderHTMLEscaping(t *testing.T) {
	// text/template does NOT escape HTML, so angle brackets should pass through
	data := TemplateData{Name: "<script>alert('xss')</script>"}
	result, err := Render("Name: {{.Name}}", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "<script>") {
		t.Error("text/template should not HTML-escape; expected <script> to be present")
	}
}

func TestRenderSpecialCharactersInFields(t *testing.T) {
	data := TemplateData{
		Name:   "my-app_v2.0",
		Domain: "sub.domain.example.com",
	}
	result, err := Render("{{.Name}} at {{.Domain}}", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "my-app_v2.0 at sub.domain.example.com" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestRenderInvalidFieldReference(t *testing.T) {
	// Referencing a nonexistent field should cause an execute error
	_, err := Render("{{.NonExistentField}}", TemplateData{})
	if err == nil {
		t.Error("expected error for nonexistent field reference")
	}
}

func TestRenderUnclosedAction(t *testing.T) {
	_, err := Render("{{.Name}", TemplateData{})
	if err == nil {
		t.Error("expected parse error for unclosed action")
	}
}

func TestRenderAllTemplateTypes(t *testing.T) {
	data := TemplateData{
		Name:            "testproject",
		Domain:          "test.example.com",
		PostgresVersion: "16-alpine",
	}

	templateNames := []string{"node", "python", "go", "static", "nextjs", "nestjs", "custom"}
	for _, name := range templateNames {
		tmpl, err := Get(name)
		if err != nil {
			t.Fatalf("get %q: %v", name, err)
		}

		// Render Dockerfile (should not contain template actions)
		result, err := Render(tmpl.Dockerfile, data)
		if err != nil {
			t.Errorf("render Dockerfile for %q: %v", name, err)
		}
		if strings.Contains(result, "{{") {
			t.Errorf("Dockerfile for %q has unrendered template actions", name)
		}

		// Render Compose
		result, err = Render(tmpl.Compose, data)
		if err != nil {
			t.Errorf("render Compose for %q: %v", name, err)
		}
		if !strings.Contains(result, "testproject") {
			t.Errorf("Compose for %q should contain project name", name)
		}
		if !strings.Contains(result, "test.example.com") {
			t.Errorf("Compose for %q should contain domain", name)
		}

		// Render Workflow
		result, err = Render(tmpl.Workflow, data)
		if err != nil {
			t.Errorf("render Workflow for %q: %v", name, err)
		}
		if !strings.Contains(result, "Deploy") {
			t.Errorf("Workflow for %q should contain 'Deploy'", name)
		}

		// Render EnvTemplate
		result, err = Render(tmpl.EnvTemplate, data)
		if err != nil {
			t.Errorf("render EnvTemplate for %q: %v", name, err)
		}
		_ = result // Just verifying no error

		// Render GitIgnore (usually no template actions)
		result, err = Render(tmpl.GitIgnore, data)
		if err != nil {
			t.Errorf("render GitIgnore for %q: %v", name, err)
		}
		if strings.Contains(result, "{{") {
			t.Errorf("GitIgnore for %q has unrendered template actions", name)
		}
	}
}

func TestListReturnsAllRegistered(t *testing.T) {
	templates := List()
	names := make(map[string]bool)
	for _, tmpl := range templates {
		names[tmpl.Name] = true
	}

	expected := []string{"node", "python", "go", "static", "nextjs", "nestjs", "custom"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("List() missing template %q", name)
		}
	}
}

func TestGetErrorMessage(t *testing.T) {
	_, err := Get("does-not-exist")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("error message should contain template name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error message should contain 'not found', got: %v", err)
	}
}

func TestRegisterOverwritesExisting(t *testing.T) {
	original := &Template{
		Name:        "overwrite-test",
		Description: "original",
		Dockerfile:  "FROM original",
		Compose:     "original",
		Workflow:    SharedWorkflow,
		GitIgnore:   ".env",
	}
	Register(original)

	replacement := &Template{
		Name:        "overwrite-test",
		Description: "replacement",
		Dockerfile:  "FROM replacement",
		Compose:     "replacement",
		Workflow:    SharedWorkflow,
		GitIgnore:   ".env",
	}
	Register(replacement)

	got, err := Get("overwrite-test")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Description != "replacement" {
		t.Errorf("expected overwritten template, got description %q", got.Description)
	}

	delete(registry, "overwrite-test")
}

// --- LoadCustomTemplates tests ---

func TestLoadCustomTemplatesNonexistentDir(t *testing.T) {
	// Should return nil when the templates directory doesn't exist
	err := LoadCustomTemplates("/nonexistent/path/that/does/not/exist")
	if err != nil {
		t.Errorf("expected nil for nonexistent base path, got: %v", err)
	}
}

func TestLoadCustomTemplatesEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	templatesDir := filepath.Join(tmpDir, "templates")
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	err := LoadCustomTemplates(tmpDir)
	if err != nil {
		t.Errorf("expected nil for empty templates dir, got: %v", err)
	}
}

func TestLoadCustomTemplatesWithFiles(t *testing.T) {
	tmpDir := t.TempDir()
	templatesDir := filepath.Join(tmpDir, "templates")

	// Create a template directory with all files
	tmplDir := filepath.Join(templatesDir, "mytemplate")
	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	os.WriteFile(filepath.Join(tmplDir, "Dockerfile"), []byte("FROM alpine\nRUN echo hello"), 0o644)
	os.WriteFile(filepath.Join(tmplDir, "docker-compose.yml"), []byte("services:\n  app:\n    build: ."), 0o644)
	os.WriteFile(filepath.Join(tmplDir, "deploy.yml"), []byte("name: Custom Deploy"), 0o644)
	os.WriteFile(filepath.Join(tmplDir, ".env.template"), []byte("KEY=value"), 0o644)
	os.WriteFile(filepath.Join(tmplDir, ".gitignore"), []byte(".env\n"), 0o644)

	err := LoadCustomTemplates(tmpDir)
	if err != nil {
		t.Fatalf("LoadCustomTemplates: %v", err)
	}

	tmpl, err := Get("mytemplate")
	if err != nil {
		t.Fatalf("get mytemplate: %v", err)
	}

	if tmpl.Dockerfile != "FROM alpine\nRUN echo hello" {
		t.Errorf("unexpected Dockerfile: %q", tmpl.Dockerfile)
	}
	if tmpl.Compose != "services:\n  app:\n    build: ." {
		t.Errorf("unexpected Compose: %q", tmpl.Compose)
	}
	if tmpl.Workflow != "name: Custom Deploy" {
		t.Errorf("unexpected Workflow: %q", tmpl.Workflow)
	}
	if tmpl.EnvTemplate != "KEY=value" {
		t.Errorf("unexpected EnvTemplate: %q", tmpl.EnvTemplate)
	}
	if tmpl.GitIgnore != ".env\n" {
		t.Errorf("unexpected GitIgnore: %q", tmpl.GitIgnore)
	}
	if tmpl.Description != "Custom template: mytemplate" {
		t.Errorf("unexpected Description: %q", tmpl.Description)
	}

	delete(registry, "mytemplate")
}

func TestLoadCustomTemplatesPartialFiles(t *testing.T) {
	tmpDir := t.TempDir()
	templatesDir := filepath.Join(tmpDir, "templates")

	// Template directory with only a Dockerfile
	tmplDir := filepath.Join(templatesDir, "partial-tmpl")
	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	os.WriteFile(filepath.Join(tmplDir, "Dockerfile"), []byte("FROM scratch"), 0o644)

	err := LoadCustomTemplates(tmpDir)
	if err != nil {
		t.Fatalf("LoadCustomTemplates: %v", err)
	}

	tmpl, err := Get("partial-tmpl")
	if err != nil {
		t.Fatalf("get partial-tmpl: %v", err)
	}

	if tmpl.Dockerfile != "FROM scratch" {
		t.Errorf("unexpected Dockerfile: %q", tmpl.Dockerfile)
	}
	if tmpl.Compose != "" {
		t.Errorf("expected empty Compose, got %q", tmpl.Compose)
	}
	// Without deploy.yml, should fall back to SharedWorkflow
	if tmpl.Workflow != SharedWorkflow {
		t.Error("expected SharedWorkflow fallback when deploy.yml is missing")
	}
	if tmpl.EnvTemplate != "" {
		t.Errorf("expected empty EnvTemplate, got %q", tmpl.EnvTemplate)
	}
	if tmpl.GitIgnore != "" {
		t.Errorf("expected empty GitIgnore, got %q", tmpl.GitIgnore)
	}

	delete(registry, "partial-tmpl")
}

func TestLoadCustomTemplatesSkipsBuiltIn(t *testing.T) {
	tmpDir := t.TempDir()
	templatesDir := filepath.Join(tmpDir, "templates")

	// Create a directory named "node" which is a built-in template
	tmplDir := filepath.Join(templatesDir, "node")
	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	os.WriteFile(filepath.Join(tmplDir, "Dockerfile"), []byte("FROM custom-node"), 0o644)

	// Get original node template
	original, err := Get("node")
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	originalDockerfile := original.Dockerfile

	err = LoadCustomTemplates(tmpDir)
	if err != nil {
		t.Fatalf("LoadCustomTemplates: %v", err)
	}

	// Node template should not have been overridden
	current, _ := Get("node")
	if current.Dockerfile != originalDockerfile {
		t.Error("built-in 'node' template should not be overridden by custom template")
	}
}

func TestLoadCustomTemplatesSkipsFiles(t *testing.T) {
	tmpDir := t.TempDir()
	templatesDir := filepath.Join(tmpDir, "templates")
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create a regular file (not a directory) inside templates/
	os.WriteFile(filepath.Join(templatesDir, "not-a-dir.txt"), []byte("hello"), 0o644)

	err := LoadCustomTemplates(tmpDir)
	if err != nil {
		t.Fatalf("LoadCustomTemplates: %v", err)
	}

	// The file should not have been registered as a template
	_, err = Get("not-a-dir.txt")
	if err == nil {
		t.Error("regular files should not be loaded as templates")
		delete(registry, "not-a-dir.txt")
	}
}

func TestLoadCustomTemplatesUnreadableDir(t *testing.T) {
	tmpDir := t.TempDir()
	templatesDir := filepath.Join(tmpDir, "templates")
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Remove read permissions
	os.Chmod(templatesDir, 0o000)
	defer os.Chmod(templatesDir, 0o755) // restore for cleanup

	err := LoadCustomTemplates(tmpDir)
	if err == nil {
		t.Error("expected error for unreadable templates directory")
	}
}

func TestRenderMultilineTemplate(t *testing.T) {
	tmpl := `Line 1: {{.Name}}
Line 2: {{.Domain}}
Line 3: {{.PostgresVersion}}`

	data := TemplateData{
		Name:            "app",
		Domain:          "app.test",
		PostgresVersion: "16",
	}

	result, err := Render(tmpl, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(result, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "Line 1: app" {
		t.Errorf("line 1: %q", lines[0])
	}
	if lines[1] != "Line 2: app.test" {
		t.Errorf("line 2: %q", lines[1])
	}
	if lines[2] != "Line 3: 16" {
		t.Errorf("line 3: %q", lines[2])
	}
}

func TestRenderWithUnicodeData(t *testing.T) {
	data := TemplateData{Name: "applikation-uberall"}
	result, err := Render("Project: {{.Name}}", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Project: applikation-uberall" {
		t.Errorf("unexpected result: %q", result)
	}
}
