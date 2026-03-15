package detect

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDetectEmptyPackageJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// An empty package.json has no deps so NestJS/Next.js won't match,
	// but detectNode will fire because package.json exists.
	if result.AppType != AppTypeNode {
		t.Errorf("expected AppType %q for empty package.json, got %q", AppTypeNode, result.AppType)
	}
	if result.Framework != "" {
		t.Errorf("expected empty Framework for empty package.json, got %q", result.Framework)
	}
	if result.Port != 3000 {
		t.Errorf("expected port 3000, got %d", result.Port)
	}
}

func TestDetectMalformedPackageJSON(t *testing.T) {
	dir := t.TempDir()
	// Write invalid JSON that cannot be unmarshalled.
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{not valid json!!!"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// readPackageJSON returns nil on unmarshal error, so no Node detector
	// should match. The project remains unknown.
	if result.AppType != AppTypeUnknown {
		t.Errorf("expected AppType %q for malformed package.json, got %q", AppTypeUnknown, result.AppType)
	}
	if result.Confidence != 0 {
		t.Errorf("expected zero confidence, got %f", result.Confidence)
	}
}

func TestDetectMultipleSignals(t *testing.T) {
	dir := t.TempDir()

	// Create both package.json (with next dependency) and go.mod.
	// The detector list runs NextJS before Go, so NextJS should win.
	pkgJSON := `{
		"dependencies": {
			"next": "14.0.0",
			"react": "18.0.0"
		}
	}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatal(err)
	}
	goMod := `module example.com/myproject

go 1.21
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Next.js is checked before Go, so it should take priority.
	if result.AppType != AppTypeNextJS {
		t.Errorf("expected AppType %q (more specific match), got %q", AppTypeNextJS, result.AppType)
	}
	if result.Framework != "Next.js" {
		t.Errorf("expected Framework %q, got %q", "Next.js", result.Framework)
	}
	if result.Language != "javascript" {
		t.Errorf("expected language javascript, got %q", result.Language)
	}
}

func TestDetectSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks may require elevated permissions on Windows")
	}

	// Create source directory with the actual package.json.
	srcDir := t.TempDir()
	pkgJSON := `{
		"dependencies": {
			"express": "4.18.0"
		}
	}`
	srcFile := filepath.Join(srcDir, "real-package.json")
	if err := os.WriteFile(srcFile, []byte(pkgJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Create target directory with a symlink to the package.json.
	dir := t.TempDir()
	if err := os.Symlink(srcFile, filepath.Join(dir, "package.json")); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.AppType != AppTypeNode {
		t.Errorf("expected AppType %q via symlink, got %q", AppTypeNode, result.AppType)
	}
	if result.Framework != "Express" {
		t.Errorf("expected Framework %q, got %q", "Express", result.Framework)
	}
}

func TestDetectLargeProject(t *testing.T) {
	dir := t.TempDir()

	// Create a deep directory tree (simulating a large project structure).
	deepPath := filepath.Join(dir, "src", "components", "features", "auth", "utils")
	if err := os.MkdirAll(deepPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Create some files in the deep structure.
	for _, f := range []string{
		filepath.Join(deepPath, "login.ts"),
		filepath.Join(dir, "src", "components", "Header.tsx"),
		filepath.Join(dir, "src", "index.ts"),
	} {
		if err := os.WriteFile(f, []byte("// placeholder"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Place package.json at the root with Next.js deps.
	pkgJSON := `{
		"dependencies": {
			"next": "14.0.0",
			"react": "18.0.0"
		},
		"devDependencies": {
			"typescript": "5.0.0"
		}
	}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.AppType != AppTypeNextJS {
		t.Errorf("expected AppType %q from project root, got %q", AppTypeNextJS, result.AppType)
	}
	// Note: fileExists returns false for directories, so the app/ directory
	// alone does not trigger App Router detection. The base framework is detected.
	if result.Framework != "Next.js" {
		t.Errorf("expected Framework %q, got %q", "Next.js", result.Framework)
	}
	if result.Language != "typescript" {
		t.Errorf("expected language typescript, got %q", result.Language)
	}
}

func TestDetectNoDatabaseOrRedis(t *testing.T) {
	dir := t.TempDir()

	// Plain Go project with no database or Redis dependencies.
	goMod := `module example.com/simple

go 1.21

require github.com/labstack/echo/v4 v4.11.0
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.HasDB {
		t.Error("expected HasDB=false for project without database deps")
	}
	if result.HasRedis {
		t.Error("expected HasRedis=false for project without Redis deps")
	}
	if result.HasDocker {
		t.Error("expected HasDocker=false when no Dockerfile or docker-compose.yml exists")
	}
	if result.Profile != "bare" {
		t.Errorf("expected profile %q, got %q", "bare", result.Profile)
	}
}

func TestDetectDockerComposeWithServices(t *testing.T) {
	dir := t.TempDir()

	// Create a Go project.
	goMod := `module example.com/fullstack

go 1.21
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	// Create docker-compose.yml with postgres, redis, and minio services.
	compose := `version: "3.8"
services:
  app:
    build: .
    ports:
      - "8080:8080"
  postgres:
    image: postgres:16
    environment:
      POSTGRES_PASSWORD: secret
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
  minio:
    image: minio/minio:latest
    ports:
      - "9000:9000"
`
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.HasDocker {
		t.Error("expected HasDocker=true when docker-compose.yml exists")
	}
	if !result.HasDB {
		t.Error("expected HasDB=true when docker-compose.yml contains postgres")
	}
	if !result.HasRedis {
		t.Error("expected HasRedis=true when docker-compose.yml contains redis")
	}
	if result.Profile != "saas" {
		t.Errorf("expected profile %q (has both DB and Redis), got %q", "saas", result.Profile)
	}
}

func TestDetectPythonWithPipfile(t *testing.T) {
	dir := t.TempDir()

	pipfile := `[[source]]
url = "https://pypi.org/simple"

[packages]
flask = "*"
sqlalchemy = "*"

[dev-packages]
pytest = "*"
`
	if err := os.WriteFile(filepath.Join(dir, "Pipfile"), []byte(pipfile), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.AppType != AppTypePython {
		t.Errorf("expected AppType %q with Pipfile, got %q", AppTypePython, result.AppType)
	}
	if result.Language != "python" {
		t.Errorf("expected language python, got %q", result.Language)
	}
	// Pipfile alone does not trigger framework detection (requires requirements.txt
	// content or specific marker files like manage.py).
	if result.Confidence < 0.80 {
		t.Errorf("expected confidence >= 0.80, got %f", result.Confidence)
	}
}

func TestDetectPythonWithPyproject(t *testing.T) {
	dir := t.TempDir()

	pyproject := `[project]
name = "myapp"
version = "0.1.0"
dependencies = [
    "fastapi>=0.100",
    "uvicorn>=0.23",
]

[build-system]
requires = ["setuptools>=68"]
build-backend = "setuptools.backends._legacy:_Backend"
`
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(pyproject), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.AppType != AppTypePython {
		t.Errorf("expected AppType %q with pyproject.toml, got %q", AppTypePython, result.AppType)
	}
	if result.Language != "python" {
		t.Errorf("expected language python, got %q", result.Language)
	}
}

func TestDetectGoWithMultipleDBLibs(t *testing.T) {
	dir := t.TempDir()

	goMod := `module example.com/multidb

go 1.21

require (
	gorm.io/gorm v1.25.0
	gorm.io/driver/postgres v1.5.0
	github.com/jackc/pgx/v5 v5.4.0
	github.com/gin-gonic/gin v1.9.0
)
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.AppType != AppTypeGo {
		t.Errorf("expected AppType %q, got %q", AppTypeGo, result.AppType)
	}
	if !result.HasDB {
		t.Error("expected HasDB=true when go.mod has both gorm and pgx")
	}
	if result.Framework != "Gin" {
		t.Errorf("expected Framework %q, got %q", "Gin", result.Framework)
	}
	if result.EntryPoint != "main.go" {
		t.Errorf("expected EntryPoint %q, got %q", "main.go", result.EntryPoint)
	}
	if result.Profile != "server" {
		t.Errorf("expected profile %q (has DB but no Redis), got %q", "server", result.Profile)
	}

	// Verify indicators contain evidence of database libs.
	foundDBIndicator := false
	for _, ind := range result.Indicators {
		if ind == "uses database libraries" {
			foundDBIndicator = true
			break
		}
	}
	if !foundDBIndicator {
		t.Errorf("expected indicator about database libraries, got indicators: %v", result.Indicators)
	}
}
