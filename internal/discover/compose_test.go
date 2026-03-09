package discover

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanComposeFiles(t *testing.T) {
	dir := t.TempDir()

	// Create a fake project with docker-compose.yml
	projectDir := filepath.Join(dir, "myproject")
	os.MkdirAll(projectDir, 0755)
	composeContent := "services:\n  app:\n    image: node:20\n    labels:\n      - \"traefik.enable=true\"\n      - \"traefik.http.routers.myproject.rule=Host(`myproject.com`)\"\n  postgres:\n    image: postgres:15-alpine\n"
	os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(composeContent), 0644)

	// Create another project
	project2Dir := filepath.Join(dir, "api")
	os.MkdirAll(project2Dir, 0755)
	os.WriteFile(filepath.Join(project2Dir, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx:alpine\n"), 0644)

	// Create a directory that should be skipped
	nodeModules := filepath.Join(dir, "node_modules", "somepackage")
	os.MkdirAll(nodeModules, 0755)
	os.WriteFile(filepath.Join(nodeModules, "docker-compose.yml"), []byte("services:\n  test:\n    image: test\n"), 0644)

	projects, err := ScanComposeFiles([]string{dir}, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	// Should find 2 projects (myproject and api, but NOT node_modules)
	if len(projects) < 2 {
		names := make([]string, len(projects))
		for i, p := range projects {
			names[i] = p.Name + " @ " + p.Dir
		}
		t.Fatalf("expected at least 2 projects, got %d: %v", len(projects), names)
	}

	// Find myproject
	var found *ComposeProject
	for i := range projects {
		if projects[i].Name == "myproject" {
			found = &projects[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected to find myproject")
	}

	if found.Domain != "myproject.com" {
		t.Errorf("expected domain myproject.com, got %q", found.Domain)
	}
	if !found.HasDB {
		t.Error("expected HasDB to be true (postgres detected)")
	}
	if found.DBType != "postgres" {
		t.Errorf("expected DBType postgres, got %s", found.DBType)
	}
	if len(found.Services) != 2 {
		t.Errorf("expected 2 services, got %d", len(found.Services))
	}
}

func TestScanComposeFilesEmpty(t *testing.T) {
	dir := t.TempDir()
	projects, err := ScanComposeFiles([]string{dir}, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(projects))
	}
}

func TestScanComposeFilesNonexistentPath(t *testing.T) {
	projects, err := ScanComposeFiles([]string{"/nonexistent/path"}, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(projects))
	}
}

func TestIsComposeFile(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"docker-compose.yml", true},
		{"docker-compose.yaml", true},
		{"compose.yml", true},
		{"compose.yaml", true},
		{"docker-compose.json", false},
		{"Dockerfile", false},
		{"random.yml", false},
	}

	for _, tt := range tests {
		if got := isComposeFile(tt.name); got != tt.expected {
			t.Errorf("isComposeFile(%q) = %v, want %v", tt.name, got, tt.expected)
		}
	}
}

func TestParseComposeWithMapLabels(t *testing.T) {
	dir := t.TempDir()
	content := "services:\n  app:\n    image: myapp:latest\n    labels:\n      traefik.enable: \"true\"\n      traefik.http.routers.myapp.rule: \"Host(`myapp.io`)\"\n"
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(content), 0644)

	cp, err := parseComposeProject(filepath.Join(dir, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if cp.Domain != "myapp.io" {
		t.Errorf("expected domain myapp.io, got %q", cp.Domain)
	}
}
