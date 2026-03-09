package templates

import (
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
