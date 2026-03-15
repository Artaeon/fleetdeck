package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/profiles"
	"github.com/fleetdeck/fleetdeck/internal/project"
	"github.com/fleetdeck/fleetdeck/internal/templates"
)

// ---------------------------------------------------------------------------
// Workflow 1: Next.js SaaS project detection -> profile recommendation
// ---------------------------------------------------------------------------

func TestWorkflowNextJSSaaS(t *testing.T) {
	dir := t.TempDir()

	// Simulate a realistic Next.js SaaS codebase with Prisma ORM, Redis
	// session store, auth, payments, and TypeScript.
	writePackageJSON(t, dir, packageJSON{
		Name: "acme-dashboard",
		Dependencies: map[string]string{
			"next":            "^14.1.0",
			"react":           "^18.2.0",
			"react-dom":       "^18.2.0",
			"@prisma/client":  "^5.8.0",
			"ioredis":         "^5.3.2",
			"next-auth":       "^4.24.0",
			"stripe":          "^14.12.0",
			"@tanstack/react-query": "^5.17.0",
			"zod":             "^3.22.4",
		},
		DevDependencies: map[string]string{
			"typescript":   "^5.3.3",
			"@types/react": "^18.2.48",
			"@types/node":  "^20.11.0",
			"prisma":       "^5.8.0",
			"eslint":       "^8.56.0",
			"tailwindcss":  "^3.4.1",
		},
		Scripts: map[string]string{
			"dev":   "next dev",
			"build": "next build",
			"start": "next start",
			"lint":  "eslint .",
		},
	})

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	// The project has @prisma/client (DB via NestJS-style detection is not
	// applicable for Next.js, but ioredis triggers Redis). DB is detected
	// because Prisma triggers HasDB in the NestJS detector only when
	// @nestjs/core is present. For Next.js, DB detection relies on
	// docker-compose or language-specific ORM patterns. However, the
	// detectServices function checks redis/ioredis patterns. Let's verify
	// actual behavior.
	if result.AppType != AppTypeNextJS {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypeNextJS)
	}
	if result.Language != "typescript" {
		t.Errorf("Language = %q, want %q", result.Language, "typescript")
	}
	if !result.HasRedis {
		t.Error("HasRedis = false, want true (ioredis in dependencies)")
	}
	if result.Port != 3000 {
		t.Errorf("Port = %d, want %d", result.Port, 3000)
	}
	if result.Confidence < 0.95 {
		t.Errorf("Confidence = %f, want >= 0.95", result.Confidence)
	}

	// Without a docker-compose.yml containing postgres, the DB will not be
	// detected for a Next.js project (Prisma detection only fires inside
	// detectNestJS). Add a compose file to get full SaaS detection.
	writeFile(t, dir, "docker-compose.yml", `services:
  app:
    build: .
  postgres:
    image: postgres:16
  redis:
    image: redis:7-alpine
`)

	result, err = Detect(dir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if !result.HasDB {
		t.Error("HasDB = false, want true (postgres in docker-compose.yml)")
	}
	if !result.HasRedis {
		t.Error("HasRedis = false, want true (ioredis + redis in compose)")
	}
	if result.Profile != "saas" {
		t.Errorf("Profile = %q, want %q", result.Profile, "saas")
	}
}

// ---------------------------------------------------------------------------
// Workflow 2: Go API project with gin + gorm + go-redis -> saas
// ---------------------------------------------------------------------------

func TestWorkflowGoAPI(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "go.mod", `module github.com/acmecorp/billing-api

go 1.22

require (
	github.com/gin-gonic/gin v1.9.1
	gorm.io/gorm v1.25.6
	gorm.io/driver/postgres v1.5.4
	github.com/redis/go-redis/v9 v9.4.0
	github.com/golang-jwt/jwt/v5 v5.2.0
	github.com/swaggo/swag v1.16.2
	golang.org/x/crypto v0.18.0
)
`)
	writeFile(t, dir, "main.go", `package main

import "github.com/gin-gonic/gin"

func main() {
	r := gin.Default()
	r.Run(":8080")
}
`)

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if result.AppType != AppTypeGo {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypeGo)
	}
	if result.Framework != "Gin" {
		t.Errorf("Framework = %q, want %q", result.Framework, "Gin")
	}
	if result.Language != "go" {
		t.Errorf("Language = %q, want %q", result.Language, "go")
	}
	if !result.HasDB {
		t.Error("HasDB = false, want true (gorm.io in go.mod)")
	}
	if !result.HasRedis {
		t.Error("HasRedis = false, want true (go-redis in go.mod)")
	}
	if result.Profile != "saas" {
		t.Errorf("Profile = %q, want %q", result.Profile, "saas")
	}
	if result.EntryPoint != "main.go" {
		t.Errorf("EntryPoint = %q, want %q", result.EntryPoint, "main.go")
	}
	if result.Port != 8080 {
		t.Errorf("Port = %d, want %d", result.Port, 8080)
	}
}

// ---------------------------------------------------------------------------
// Workflow 3: Go API with gin + gorm but NO redis -> server
// ---------------------------------------------------------------------------

func TestWorkflowGoAPINoRedis(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "go.mod", `module github.com/acmecorp/crud-api

go 1.22

require (
	github.com/gin-gonic/gin v1.9.1
	gorm.io/gorm v1.25.6
	gorm.io/driver/postgres v1.5.4
)
`)
	writeFile(t, dir, "main.go", `package main

func main() {}
`)

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if result.AppType != AppTypeGo {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypeGo)
	}
	if result.Framework != "Gin" {
		t.Errorf("Framework = %q, want %q", result.Framework, "Gin")
	}
	if !result.HasDB {
		t.Error("HasDB = false, want true (gorm.io in go.mod)")
	}
	if result.HasRedis {
		t.Error("HasRedis = true, want false (no redis dependency)")
	}
	if result.Profile != "server" {
		t.Errorf("Profile = %q, want %q (DB only, no Redis)", result.Profile, "server")
	}
}

// ---------------------------------------------------------------------------
// Workflow 4: Python Django with celery + redis -> saas
// ---------------------------------------------------------------------------

func TestWorkflowPythonDjango(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "requirements.txt", `django==4.2.9
djangorestframework==3.14.0
psycopg2-binary==2.9.9
celery==5.3.6
redis==5.0.1
django-cors-headers==4.3.1
gunicorn==21.2.0
whitenoise==6.6.0
django-filter==23.5
drf-spectacular==0.27.0
`)
	writeFile(t, dir, "manage.py", `#!/usr/bin/env python
"""Django's command-line utility."""
import os, sys

def main():
    os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'config.settings')
    from django.core.management import execute_from_command_line
    execute_from_command_line(sys.argv)

if __name__ == '__main__':
    main()
`)

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if result.AppType != AppTypePython {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypePython)
	}
	if result.Framework != "Django" {
		t.Errorf("Framework = %q, want %q", result.Framework, "Django")
	}
	if result.Language != "python" {
		t.Errorf("Language = %q, want %q", result.Language, "python")
	}
	if !result.HasDB {
		t.Error("HasDB = false, want true (psycopg2 in requirements)")
	}
	if !result.HasRedis {
		t.Error("HasRedis = false, want true (redis in requirements)")
	}
	if result.Profile != "saas" {
		t.Errorf("Profile = %q, want %q", result.Profile, "saas")
	}
	if result.Port != 8000 {
		t.Errorf("Port = %d, want %d", result.Port, 8000)
	}
	if result.Confidence < 0.90 {
		t.Errorf("Confidence = %f, want >= 0.90", result.Confidence)
	}
}

// ---------------------------------------------------------------------------
// Workflow 5: Static site (HTML/CSS/JS only) -> static
// ---------------------------------------------------------------------------

func TestWorkflowStaticSite(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "index.html", `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Acme Corp</title>
    <link rel="stylesheet" href="css/style.css">
</head>
<body>
    <header><h1>Acme Corp</h1></header>
    <main><p>Welcome to our landing page.</p></main>
    <script src="js/app.js"></script>
</body>
</html>
`)
	writeFile(t, dir, "css/style.css", `body { font-family: sans-serif; }`)
	writeFile(t, dir, "js/app.js", `console.log("hello");`)

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if result.AppType != AppTypeStatic {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypeStatic)
	}
	if result.Language != "html" {
		t.Errorf("Language = %q, want %q", result.Language, "html")
	}
	if result.Profile != "static" {
		t.Errorf("Profile = %q, want %q", result.Profile, "static")
	}
	if result.Port != 80 {
		t.Errorf("Port = %d, want %d", result.Port, 80)
	}
	if result.HasDB {
		t.Error("HasDB should be false for a static site")
	}
	if result.HasRedis {
		t.Error("HasRedis should be false for a static site")
	}
	if result.HasDocker {
		t.Error("HasDocker should be false for a pure static site")
	}
}

// ---------------------------------------------------------------------------
// Workflow 6: Express + pg -> server (DB but no Redis)
// ---------------------------------------------------------------------------

func TestWorkflowExpressAPI(t *testing.T) {
	dir := t.TempDir()

	writePackageJSON(t, dir, packageJSON{
		Name: "orders-api",
		Main: "src/index.js",
		Dependencies: map[string]string{
			"express":    "^4.18.2",
			"pg":         "^8.11.3",
			"cors":       "^2.8.5",
			"helmet":     "^7.1.0",
			"morgan":     "^1.10.0",
			"dotenv":     "^16.3.1",
		},
	})

	// Express + pg does not trigger DB detection via language-level deps
	// because Node DB detection only happens through NestJS-specific ORM
	// deps or docker-compose.yml content. Add a compose file with postgres.
	writeFile(t, dir, "docker-compose.yml", `services:
  app:
    build: .
  postgres:
    image: postgres:16
`)

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if result.AppType != AppTypeNode {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypeNode)
	}
	if result.Framework != "Express" {
		t.Errorf("Framework = %q, want %q", result.Framework, "Express")
	}
	if !result.HasDB {
		t.Error("HasDB = false, want true (postgres in docker-compose.yml)")
	}
	if result.HasRedis {
		t.Error("HasRedis = true, want false")
	}
	if result.Profile != "server" {
		t.Errorf("Profile = %q, want %q", result.Profile, "server")
	}
}

// ---------------------------------------------------------------------------
// Workflow 7: Rust backend with axum + sqlx + redis -> saas
// ---------------------------------------------------------------------------

func TestWorkflowRustBackend(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "Cargo.toml", `[package]
name = "inventory-service"
version = "0.1.0"
edition = "2021"

[dependencies]
axum = "0.7"
sqlx = { version = "0.7", features = ["runtime-tokio-rustls", "postgres"] }
redis = "0.24"
tokio = { version = "1", features = ["full"] }
serde = { version = "1", features = ["derive"] }
serde_json = "1"
tower = "0.4"
tower-http = { version = "0.5", features = ["cors", "trace"] }
tracing = "0.1"
tracing-subscriber = "0.3"
dotenvy = "0.15"
`)

	// Note: matchesAnyInProject does not scan Cargo.toml, so HasRedis will
	// only be true if we also provide a docker-compose.yml with redis. Add
	// one to simulate a realistic Rust project that includes both.
	writeFile(t, dir, "docker-compose.yml", `services:
  app:
    build: .
  postgres:
    image: postgres:16
  redis:
    image: redis:7-alpine
`)

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if result.AppType != AppTypeRust {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypeRust)
	}
	if result.Framework != "Axum" {
		t.Errorf("Framework = %q, want %q", result.Framework, "Axum")
	}
	if result.Language != "rust" {
		t.Errorf("Language = %q, want %q", result.Language, "rust")
	}
	if !result.HasDB {
		t.Error("HasDB = false, want true (sqlx in Cargo.toml)")
	}
	if !result.HasRedis {
		t.Error("HasRedis = false, want true (redis in docker-compose.yml)")
	}
	if result.Profile != "saas" {
		t.Errorf("Profile = %q, want %q", result.Profile, "saas")
	}
	if result.Port != 8080 {
		t.Errorf("Port = %d, want %d", result.Port, 8080)
	}
	if !result.HasDocker {
		t.Error("HasDocker = false, want true")
	}
}

// ---------------------------------------------------------------------------
// Workflow 8: NestJS microservice with bull, typeorm -> detect DB + Redis
// ---------------------------------------------------------------------------

func TestWorkflowNestJSMicroservice(t *testing.T) {
	dir := t.TempDir()

	writePackageJSON(t, dir, packageJSON{
		Name: "notifications-service",
		Dependencies: map[string]string{
			"@nestjs/core":          "^10.3.0",
			"@nestjs/common":        "^10.3.0",
			"@nestjs/microservices": "^10.3.0",
			"@nestjs/typeorm":       "^10.0.1",
			"typeorm":               "^0.3.17",
			"bull":                  "^4.12.0",
			"pg":                    "^8.11.3",
		},
		DevDependencies: map[string]string{
			"typescript": "^5.3.3",
		},
	})

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if result.AppType != AppTypeNestJS {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypeNestJS)
	}
	if result.Framework != "NestJS" {
		t.Errorf("Framework = %q, want %q", result.Framework, "NestJS")
	}
	if !result.HasDB {
		t.Error("HasDB = false, want true (TypeORM via @nestjs/typeorm)")
	}
	if !result.HasRedis {
		t.Error("HasRedis = false, want true (bull implies Redis)")
	}
	if result.Profile != "saas" {
		t.Errorf("Profile = %q, want %q (DB + Redis)", result.Profile, "saas")
	}
	if result.Language != "typescript" {
		t.Errorf("Language = %q, want %q", result.Language, "typescript")
	}
}

// ---------------------------------------------------------------------------
// Workflow 9: Full flow -- detect project, get profile, scaffold, verify files
// ---------------------------------------------------------------------------

func TestWorkflowDetectThenScaffold(t *testing.T) {
	// Step 1: Create a realistic project directory and detect it.
	srcDir := t.TempDir()

	writePackageJSON(t, srcDir, packageJSON{
		Name: "my-saas-app",
		Dependencies: map[string]string{
			"next":           "^14.1.0",
			"react":          "^18.2.0",
			"@prisma/client": "^5.8.0",
			"ioredis":        "^5.3.2",
		},
		DevDependencies: map[string]string{
			"typescript": "^5.3.3",
		},
	})

	writeFile(t, srcDir, "docker-compose.yml", `services:
  app:
    build: .
  postgres:
    image: postgres:16
  redis:
    image: redis:7-alpine
`)

	result, err := Detect(srcDir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}
	if result.Profile != "saas" {
		t.Fatalf("Profile = %q, want %q; cannot proceed with scaffold test", result.Profile, "saas")
	}

	// Step 2: Fetch the recommended profile.
	prof, err := profiles.Get(result.Profile)
	if err != nil {
		t.Fatalf("profiles.Get(%q) returned error: %v", result.Profile, err)
	}
	if prof.Name != "saas" {
		t.Fatalf("profile.Name = %q, want %q", prof.Name, "saas")
	}

	// Step 3: Fetch a code template for the Dockerfile.
	tmpl, err := templates.Get("node")
	if err != nil {
		t.Fatalf("templates.Get(\"node\") returned error: %v", err)
	}

	// Step 4: Scaffold from the profile into a new directory.
	outDir := t.TempDir()
	profileData := profiles.ProfileData{
		Name:            "my-saas-app",
		Domain:          "my-saas-app.example.com",
		Port:            result.Port,
		PostgresVersion: "16-alpine",
		RedisVersion:    "7-alpine",
		AppType:         string(result.AppType),
		Framework:       result.Framework,
	}
	tmplData := templates.TemplateData{
		Name:            "my-saas-app",
		Domain:          "my-saas-app.example.com",
		PostgresVersion: "16-alpine",
	}

	if err := project.ScaffoldFromProfile(outDir, prof, tmpl, profileData, tmplData); err != nil {
		t.Fatalf("ScaffoldFromProfile() returned error: %v", err)
	}

	// Step 5: Verify all expected files were created.
	expectedFiles := []string{
		"Dockerfile",
		"docker-compose.yml",
		".github/workflows/deploy.yml",
		".gitignore",
	}
	for _, f := range expectedFiles {
		path := filepath.Join(outDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file %s was not created", f)
		}
	}

	// Step 6: Verify docker-compose.yml contains correct content from the
	// SaaS profile (postgres, redis, minio, mailpit services).
	composeData, err := os.ReadFile(filepath.Join(outDir, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("reading docker-compose.yml: %v", err)
	}
	compose := string(composeData)

	expectedServices := []string{"postgres", "redis", "minio", "mailpit"}
	for _, svc := range expectedServices {
		if !strings.Contains(compose, svc) {
			t.Errorf("docker-compose.yml missing service %q", svc)
		}
	}

	// Verify the domain is rendered correctly in Host rules.
	if !strings.Contains(compose, "my-saas-app.example.com") {
		t.Error("docker-compose.yml does not contain the expected domain")
	}

	// Verify the project name appears in container names.
	if !strings.Contains(compose, "my-saas-app-app") {
		t.Error("docker-compose.yml does not contain expected container name")
	}

	// Verify Dockerfile was rendered from the node template.
	dockerfileData, err := os.ReadFile(filepath.Join(outDir, "Dockerfile"))
	if err != nil {
		t.Fatalf("reading Dockerfile: %v", err)
	}
	if !strings.Contains(string(dockerfileData), "node:20-alpine") {
		t.Error("Dockerfile does not contain expected base image from node template")
	}
}

// ---------------------------------------------------------------------------
// Workflow 10: Scaffold all profiles and verify correct services
// ---------------------------------------------------------------------------

func TestWorkflowDetectThenScaffoldAllProfiles(t *testing.T) {
	// Expected services per profile (service names that should appear as
	// image or container_name references in the rendered compose).
	profileServices := map[string][]string{
		"bare":      {"app"},
		"server":    {"app", "postgres", "redis"},
		"saas":      {"app", "postgres", "redis", "minio", "mailpit"},
		"static":    {"nginx"},
		"worker":    {"worker", "redis", "postgres"},
		"fullstack": {"frontend", "backend", "postgres", "redis", "minio"},
	}

	tmpl, err := templates.Get("node")
	if err != nil {
		t.Fatalf("templates.Get(\"node\") returned error: %v", err)
	}

	for profileName, expectedSvcs := range profileServices {
		t.Run(profileName, func(t *testing.T) {
			prof, err := profiles.Get(profileName)
			if err != nil {
				t.Fatalf("profiles.Get(%q) returned error: %v", profileName, err)
			}

			outDir := t.TempDir()
			profileData := profiles.ProfileData{
				Name:            "test-project",
				Domain:          "test-project.example.com",
				Port:            3000,
				PostgresVersion: "16-alpine",
				RedisVersion:    "7-alpine",
			}
			tmplData := templates.TemplateData{
				Name:            "test-project",
				Domain:          "test-project.example.com",
				PostgresVersion: "16-alpine",
			}

			if err := project.ScaffoldFromProfile(outDir, prof, tmpl, profileData, tmplData); err != nil {
				t.Fatalf("ScaffoldFromProfile() returned error: %v", err)
			}

			// Read the generated compose file.
			composeData, err := os.ReadFile(filepath.Join(outDir, "docker-compose.yml"))
			if err != nil {
				t.Fatalf("reading docker-compose.yml: %v", err)
			}
			compose := string(composeData)

			// Verify each expected service exists.
			for _, svc := range expectedSvcs {
				// Services should appear as a top-level key or container_name.
				svcKey := svc + ":"
				containerName := "test-project-" + svc
				if !strings.Contains(compose, svcKey) && !strings.Contains(compose, containerName) {
					t.Errorf("profile %q: docker-compose.yml missing service %q", profileName, svc)
				}
			}

			// Verify the profile-declared services match expectations.
			declaredNames := prof.ServiceNames()
			if len(declaredNames) != len(expectedSvcs) {
				t.Errorf("profile %q: has %d declared services %v, want %d services %v",
					profileName, len(declaredNames), declaredNames, len(expectedSvcs), expectedSvcs)
			}

			// Verify Dockerfile exists (all profiles get one).
			if _, err := os.Stat(filepath.Join(outDir, "Dockerfile")); os.IsNotExist(err) {
				t.Errorf("profile %q: Dockerfile was not created", profileName)
			}

			// Verify deploy workflow exists.
			if _, err := os.Stat(filepath.Join(outDir, ".github", "workflows", "deploy.yml")); os.IsNotExist(err) {
				t.Errorf("profile %q: deploy.yml workflow was not created", profileName)
			}

			// For static profile, verify nginx.conf was created.
			if profileName == "static" {
				if _, err := os.Stat(filepath.Join(outDir, "nginx.conf")); os.IsNotExist(err) {
					t.Error("static profile: nginx.conf was not created")
				}
				// Verify public/index.html was created.
				if _, err := os.Stat(filepath.Join(outDir, "public", "index.html")); os.IsNotExist(err) {
					t.Error("static profile: public/index.html was not created")
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers (detection result assertions used across workflows)
// ---------------------------------------------------------------------------

// containsIndicator checks whether the result contains a specific indicator.
func containsIndicator(result *Result, indicator string) bool {
	for _, ind := range result.Indicators {
		if ind == indicator {
			return true
		}
	}
	return false
}

// TestWorkflowDetectResultRoundTrip ensures detection results survive JSON
// serialization, which is important for the API layer that serves detection
// results to the CLI/UI.
func TestWorkflowDetectResultRoundTrip(t *testing.T) {
	dir := t.TempDir()

	writePackageJSON(t, dir, packageJSON{
		Name: "roundtrip-test",
		Dependencies: map[string]string{
			"next":    "^14.0.0",
			"ioredis": "^5.3.2",
		},
		DevDependencies: map[string]string{
			"typescript": "^5.3.0",
		},
	})

	writeFile(t, dir, "docker-compose.yml", `services:
  db:
    image: postgres:16
  cache:
    image: redis:7
`)

	original, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var restored Result
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if restored.AppType != original.AppType {
		t.Errorf("AppType mismatch: got %q, want %q", restored.AppType, original.AppType)
	}
	if restored.Framework != original.Framework {
		t.Errorf("Framework mismatch: got %q, want %q", restored.Framework, original.Framework)
	}
	if restored.Language != original.Language {
		t.Errorf("Language mismatch: got %q, want %q", restored.Language, original.Language)
	}
	if restored.HasDB != original.HasDB {
		t.Errorf("HasDB mismatch: got %v, want %v", restored.HasDB, original.HasDB)
	}
	if restored.HasRedis != original.HasRedis {
		t.Errorf("HasRedis mismatch: got %v, want %v", restored.HasRedis, original.HasRedis)
	}
	if restored.Profile != original.Profile {
		t.Errorf("Profile mismatch: got %q, want %q", restored.Profile, original.Profile)
	}
	if restored.Port != original.Port {
		t.Errorf("Port mismatch: got %d, want %d", restored.Port, original.Port)
	}
	if len(restored.Indicators) != len(original.Indicators) {
		t.Errorf("Indicators length mismatch: got %d, want %d", len(restored.Indicators), len(original.Indicators))
	}
}
