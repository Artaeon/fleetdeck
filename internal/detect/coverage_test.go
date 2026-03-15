package detect

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// dirExists / fileExists
// ---------------------------------------------------------------------------

func TestDirExists(t *testing.T) {
	t.Run("existing directory returns true", func(t *testing.T) {
		dir := t.TempDir()
		if !dirExists(dir) {
			t.Errorf("dirExists(%q) = false, want true", dir)
		}
	})

	t.Run("non-existing path returns false", func(t *testing.T) {
		if dirExists("/tmp/nonexistent-path-should-not-exist-12345") {
			t.Error("dirExists() = true for non-existing path, want false")
		}
	})

	t.Run("regular file returns false", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "afile.txt")
		if err := os.WriteFile(f, []byte("hello"), 0644); err != nil {
			t.Fatal(err)
		}
		if dirExists(f) {
			t.Errorf("dirExists(%q) = true for a regular file, want false", f)
		}
	})
}

func TestFileExists(t *testing.T) {
	t.Run("existing file returns true", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "exists.txt")
		if err := os.WriteFile(f, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
		if !fileExists(f) {
			t.Errorf("fileExists(%q) = false, want true", f)
		}
	})

	t.Run("non-existing path returns false", func(t *testing.T) {
		if fileExists("/tmp/nonexistent-file-12345.txt") {
			t.Error("fileExists() = true for non-existing path, want false")
		}
	})

	t.Run("directory returns false", func(t *testing.T) {
		dir := t.TempDir()
		if fileExists(dir) {
			t.Errorf("fileExists(%q) = true for a directory, want false", dir)
		}
	})
}

// ---------------------------------------------------------------------------
// detectServices — database detection via docker-compose.yml
// ---------------------------------------------------------------------------

func TestDetectMariaDB(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "docker-compose.yml", `version: "3"
services:
  app:
    build: .
  db:
    image: mariadb:10
`)
	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if !result.HasDB {
		t.Error("HasDB = false, want true (mariadb in docker-compose.yml)")
	}
}

func TestDetectMySQL(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "docker-compose.yml", `version: "3"
services:
  app:
    build: .
  db:
    image: mysql:8
`)
	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if !result.HasDB {
		t.Error("HasDB = false, want true (mysql in docker-compose.yml)")
	}
}

// ---------------------------------------------------------------------------
// matchesAnyInProject
// ---------------------------------------------------------------------------

func TestMatchesAnyInProjectGoMod(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", `module example.com/app

go 1.21

require github.com/redis/go-redis/v9 v9.3.0
`)
	patterns := []string{"redis"}
	if !matchesAnyInProject(dir, patterns) {
		t.Error("matchesAnyInProject() = false, want true (redis in go.mod)")
	}
}

func TestMatchesAnyInProjectRequirements(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "requirements.txt", "flask==3.0\nredis==5.0.1\n")
	patterns := []string{"redis"}
	if !matchesAnyInProject(dir, patterns) {
		t.Error("matchesAnyInProject() = false, want true (redis in requirements.txt)")
	}
}

func TestMatchesAnyInProjectNoMatch(t *testing.T) {
	dir := t.TempDir()
	// Empty directory — no package.json, requirements.txt, or go.mod.
	patterns := []string{"redis", "ioredis"}
	if matchesAnyInProject(dir, patterns) {
		t.Error("matchesAnyInProject() = true, want false (empty dir)")
	}
}

// ---------------------------------------------------------------------------
// Database detection via language-level dependencies
// ---------------------------------------------------------------------------

func TestDetectGoWithPgx(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", `module example.com/app

go 1.21

require github.com/jackc/pgx/v5 v5.4.0
`)
	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if result.AppType != AppTypeGo {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypeGo)
	}
	if !result.HasDB {
		t.Error("HasDB = false, want true (pgx in go.mod)")
	}
}

func TestDetectGoWithSqlx(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", `module example.com/app

go 1.21

require github.com/jmoiron/sqlx v1.3.5
`)
	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if result.AppType != AppTypeGo {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypeGo)
	}
	if !result.HasDB {
		t.Error("HasDB = false, want true (sqlx in go.mod)")
	}
}

func TestDetectRustWithSeaOrm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Cargo.toml", `[package]
name = "myapp"
version = "0.1.0"
edition = "2021"

[dependencies]
sea-orm = { version = "0.12", features = ["sqlx-postgres"] }
`)
	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if result.AppType != AppTypeRust {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypeRust)
	}
	if !result.HasDB {
		t.Error("HasDB = false, want true (sea-orm in Cargo.toml)")
	}
}

func TestDetectRustWithDiesel(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Cargo.toml", `[package]
name = "myapp"
version = "0.1.0"
edition = "2021"

[dependencies]
diesel = { version = "2", features = ["postgres"] }
`)
	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if result.AppType != AppTypeRust {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypeRust)
	}
	if !result.HasDB {
		t.Error("HasDB = false, want true (diesel in Cargo.toml)")
	}
}

// ---------------------------------------------------------------------------
// Framework detection — additional frameworks
// ---------------------------------------------------------------------------

func TestDetectNodeFastify(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, packageJSON{
		Name:         "fastify-app",
		Dependencies: map[string]string{"fastify": "^4.0.0"},
	})
	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if result.AppType != AppTypeNode {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypeNode)
	}
	if result.Framework != "Fastify" {
		t.Errorf("Framework = %q, want %q", result.Framework, "Fastify")
	}
}

func TestDetectNodeKoa(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, packageJSON{
		Name:         "koa-app",
		Dependencies: map[string]string{"koa": "^2.14.0"},
	})
	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if result.AppType != AppTypeNode {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypeNode)
	}
	if result.Framework != "Koa" {
		t.Errorf("Framework = %q, want %q", result.Framework, "Koa")
	}
}

func TestDetectGoEcho(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", `module example.com/app

go 1.21

require github.com/labstack/echo/v4 v4.11.0
`)
	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if result.AppType != AppTypeGo {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypeGo)
	}
	if result.Framework != "Echo" {
		t.Errorf("Framework = %q, want %q", result.Framework, "Echo")
	}
}

func TestDetectGoFiber(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", `module example.com/app

go 1.21

require github.com/gofiber/fiber/v2 v2.51.0
`)
	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if result.AppType != AppTypeGo {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypeGo)
	}
	if result.Framework != "Fiber" {
		t.Errorf("Framework = %q, want %q", result.Framework, "Fiber")
	}
}

func TestDetectRustRocket(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Cargo.toml", `[package]
name = "myapp"
version = "0.1.0"
edition = "2021"

[dependencies]
rocket = "0.5"
`)
	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if result.AppType != AppTypeRust {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypeRust)
	}
	if result.Framework != "Rocket" {
		t.Errorf("Framework = %q, want %q", result.Framework, "Rocket")
	}
}

// ---------------------------------------------------------------------------
// Python: setup.py detection
// ---------------------------------------------------------------------------

func TestDetectPythonSetupPy(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "setup.py", `from setuptools import setup
setup(name="myapp", version="1.0")
`)
	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if result.AppType != AppTypePython {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypePython)
	}
	if result.Language != "python" {
		t.Errorf("Language = %q, want %q", result.Language, "python")
	}
}

// ---------------------------------------------------------------------------
// readPackageJSON — malformed input
// ---------------------------------------------------------------------------

func TestReadPackageJSONMalformed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", "this is not valid json {{{")
	pkg := readPackageJSON(dir)
	if pkg != nil {
		t.Errorf("readPackageJSON() = %+v, want nil for malformed JSON", pkg)
	}
}
