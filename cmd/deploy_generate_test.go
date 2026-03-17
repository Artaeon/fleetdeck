package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/detect"
)

func TestGenerateMissingFilesCreatesDockerfile(t *testing.T) {
	dir := t.TempDir()

	// Create a minimal package.json for detection context
	result := &detect.Result{
		AppType:  detect.AppTypeNextJS,
		Language: "typescript",
		Port:     3000,
	}

	generated := generateMissingFiles(dir, "test-app", "test.example.com", result)

	if len(generated) == 0 {
		t.Fatal("expected files to be generated")
	}

	// Check Dockerfile was created
	if _, err := os.Stat(filepath.Join(dir, "Dockerfile")); os.IsNotExist(err) {
		t.Error("expected Dockerfile to be generated")
	}

	// Check docker-compose.yml was created
	if _, err := os.Stat(filepath.Join(dir, "docker-compose.yml")); os.IsNotExist(err) {
		t.Error("expected docker-compose.yml to be generated")
	}

	// Check .dockerignore was created
	if _, err := os.Stat(filepath.Join(dir, ".dockerignore")); os.IsNotExist(err) {
		t.Error("expected .dockerignore to be generated")
	}
}

func TestGenerateMissingFilesSkipsExisting(t *testing.T) {
	dir := t.TempDir()

	// Pre-create Dockerfile
	existingContent := "FROM alpine\n"
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(existingContent), 0644); err != nil {
		t.Fatal(err)
	}

	result := &detect.Result{
		AppType:  detect.AppTypeNode,
		Language: "javascript",
		Port:     3000,
	}

	generated := generateMissingFiles(dir, "test-app", "test.example.com", result)

	// Dockerfile should NOT be in generated list
	for _, g := range generated {
		if g == "Dockerfile" {
			t.Error("should not regenerate existing Dockerfile")
		}
	}

	// Verify original content preserved
	data, _ := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	if string(data) != existingContent {
		t.Error("existing Dockerfile was overwritten")
	}
}

func TestGenerateMissingFilesComposeHasDomain(t *testing.T) {
	dir := t.TempDir()

	result := &detect.Result{
		AppType: detect.AppTypeGo,
		Port:    8080,
	}

	generateMissingFiles(dir, "my-go-app", "go.example.com", result)

	data, err := os.ReadFile(filepath.Join(dir, "docker-compose.yml"))
	if err != nil {
		t.Fatal("docker-compose.yml not generated")
	}

	content := string(data)
	if !contains(content, "go.example.com") {
		t.Error("docker-compose.yml should contain the domain")
	}
	if !contains(content, "127.0.0.1:8080") {
		t.Error("docker-compose.yml healthcheck should use 127.0.0.1")
	}
	if !contains(content, "traefik.enable=true") {
		t.Error("docker-compose.yml should contain Traefik labels")
	}
}

func TestGenerateMissingFilesUnknownTypeNoDockerfile(t *testing.T) {
	dir := t.TempDir()

	result := &detect.Result{
		AppType: detect.AppTypeUnknown,
	}

	generated := generateMissingFiles(dir, "unknown-app", "test.example.com", result)

	// Dockerfile should not be generated for unknown types
	for _, g := range generated {
		if g == "Dockerfile" {
			t.Error("should not generate Dockerfile for unknown app type")
		}
	}
}

func TestGetWarningsReturnsWarnings(t *testing.T) {
	result := &detect.Result{
		Warnings: []string{"test warning"},
	}

	warnings, ok := getWarnings(result)
	if !ok {
		t.Error("expected warnings to be present")
	}
	if len(warnings) != 1 || warnings[0] != "test warning" {
		t.Error("unexpected warnings content")
	}
}

func TestGetWarningsNoWarnings(t *testing.T) {
	result := &detect.Result{}

	_, ok := getWarnings(result)
	if ok {
		t.Error("expected no warnings")
	}
}

func TestFileExistsInDir(t *testing.T) {
	dir := t.TempDir()

	if fileExistsInDir(dir, "nonexistent") {
		t.Error("expected false for nonexistent file")
	}

	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)
	if !fileExistsInDir(dir, "test.txt") {
		t.Error("expected true for existing file")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && // avoid false positives
		filepath.Base(s) != s && // not a path
		len(s) >= len(substr) &&
		findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
