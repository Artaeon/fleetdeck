package detect

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDetectFullNodeProject creates a realistic Node.js project with express,
// pg, ioredis, typescript in devDeps, a Dockerfile, and docker-compose.yml
// with postgres+redis. Verifies all detection fields.
func TestDetectFullNodeProject(t *testing.T) {
	dir := t.TempDir()

	writePackageJSON(t, dir, packageJSON{
		Name: "my-express-api",
		Main: "dist/index.js",
		Dependencies: map[string]string{
			"express":  "^4.18.2",
			"pg":       "^8.11.3",
			"ioredis":  "^5.3.2",
			"cors":     "^2.8.5",
			"dotenv":   "^16.3.1",
			"helmet":   "^7.1.0",
			"morgan":   "^1.10.0",
		},
		DevDependencies: map[string]string{
			"typescript":       "^5.3.3",
			"@types/express":   "^4.17.21",
			"@types/node":      "^20.10.0",
			"@types/cors":      "^2.8.17",
			"@types/morgan":    "^1.9.9",
			"ts-node":          "^10.9.2",
			"nodemon":          "^3.0.2",
		},
		Scripts: map[string]string{
			"build": "tsc",
			"start": "node dist/index.js",
			"dev":   "nodemon src/index.ts",
		},
	})

	writeFile(t, dir, "Dockerfile", `FROM node:20-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:20-alpine
WORKDIR /app
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/node_modules ./node_modules
COPY package*.json ./
EXPOSE 3000
CMD ["node", "dist/index.js"]
`)

	writeFile(t, dir, "docker-compose.yml", `version: "3.8"
services:
  app:
    build: .
    ports:
      - "3000:3000"
    environment:
      DATABASE_URL: postgresql://app:secret@postgres:5432/app_db
      REDIS_URL: redis://redis:6379
    depends_on:
      - postgres
      - redis

  postgres:
    image: postgres:16
    environment:
      POSTGRES_USER: app
      POSTGRES_PASSWORD: secret
      POSTGRES_DB: app_db
    volumes:
      - postgres_data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    command: redis-server --appendonly yes
    volumes:
      - redis_data:/data

volumes:
  postgres_data:
  redis_data:
`)

	writeFile(t, dir, "tsconfig.json", `{
  "compilerOptions": {
    "target": "ES2020",
    "module": "commonjs",
    "outDir": "./dist",
    "rootDir": "./src",
    "strict": true
  }
}
`)

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	// Verify AppType.
	if result.AppType != AppTypeNode {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypeNode)
	}

	// Verify Framework.
	if result.Framework != "Express" {
		t.Errorf("Framework = %q, want %q", result.Framework, "Express")
	}

	// Verify Language (TypeScript from devDeps).
	if result.Language != "typescript" {
		t.Errorf("Language = %q, want %q", result.Language, "typescript")
	}

	// Verify HasDB (from docker-compose.yml postgres).
	if !result.HasDB {
		t.Error("HasDB = false, want true (postgres in docker-compose.yml)")
	}

	// Verify HasRedis (from ioredis in deps + redis in docker-compose.yml).
	if !result.HasRedis {
		t.Error("HasRedis = false, want true (ioredis in deps, redis in docker-compose.yml)")
	}

	// Verify HasDocker (Dockerfile and docker-compose.yml).
	if !result.HasDocker {
		t.Error("HasDocker = false, want true (Dockerfile present)")
	}

	// Verify Profile (has DB + Redis = saas).
	if result.Profile != "saas" {
		t.Errorf("Profile = %q, want %q", result.Profile, "saas")
	}

	// Verify EntryPoint.
	if result.EntryPoint != "dist/index.js" {
		t.Errorf("EntryPoint = %q, want %q", result.EntryPoint, "dist/index.js")
	}

	// Verify Confidence.
	if result.Confidence < 0.90 {
		t.Errorf("Confidence = %f, want >= 0.90", result.Confidence)
	}

	// Verify Port.
	if result.Port != 3000 {
		t.Errorf("Port = %d, want %d", result.Port, 3000)
	}

	// Verify indicators contain meaningful entries.
	if len(result.Indicators) == 0 {
		t.Error("Indicators should not be empty")
	}
	expectedIndicators := []string{
		"has existing Docker configuration",
		"found package.json",
		"uses Express framework",
		"TypeScript project",
		"uses Redis",
	}
	for _, expected := range expectedIndicators {
		found := false
		for _, ind := range result.Indicators {
			if ind == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing expected indicator %q, got: %v", expected, result.Indicators)
		}
	}
}

// TestDetectFullPythonProject creates a realistic Python project with FastAPI,
// uvicorn, sqlalchemy, redis, and main.py. Verifies all detection fields.
func TestDetectFullPythonProject(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "requirements.txt", `fastapi==0.104.1
uvicorn[standard]==0.24.0
sqlalchemy==2.0.23
psycopg2-binary==2.9.9
redis==5.0.1
pydantic==2.5.2
python-dotenv==1.0.0
alembic==1.13.0
httpx==0.25.2
`)

	writeFile(t, dir, "main.py", `from fastapi import FastAPI
from sqlalchemy import create_engine

app = FastAPI()

@app.get("/health")
async def health():
    return {"status": "ok"}
`)

	writeFile(t, dir, "Dockerfile", `FROM python:3.12-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
EXPOSE 8000
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
`)

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if result.AppType != AppTypePython {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypePython)
	}
	if result.Framework != "FastAPI" {
		t.Errorf("Framework = %q, want %q", result.Framework, "FastAPI")
	}
	if result.Language != "python" {
		t.Errorf("Language = %q, want %q", result.Language, "python")
	}
	if result.Port != 8000 {
		t.Errorf("Port = %d, want %d", result.Port, 8000)
	}
	if !result.HasDB {
		t.Error("HasDB = false, want true (sqlalchemy in requirements.txt)")
	}
	if !result.HasRedis {
		t.Error("HasRedis = false, want true (redis in requirements.txt)")
	}
	if !result.HasDocker {
		t.Error("HasDocker = false, want true (Dockerfile present)")
	}
	if result.Profile != "saas" {
		t.Errorf("Profile = %q, want %q (has DB + Redis)", result.Profile, "saas")
	}
	if result.Confidence < 0.90 {
		t.Errorf("Confidence = %f, want >= 0.90", result.Confidence)
	}
}

// TestDetectFullGoProject creates a realistic Go project with gin, gorm,
// go-redis, main.go, and cmd/ directory. Verifies all detection fields.
func TestDetectFullGoProject(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "go.mod", `module github.com/mycompany/api-server

go 1.21

require (
	github.com/gin-gonic/gin v1.9.1
	gorm.io/gorm v1.25.5
	gorm.io/driver/postgres v1.5.4
	github.com/redis/go-redis/v9 v9.3.0
	github.com/joho/godotenv v1.5.1
	github.com/golang-jwt/jwt/v5 v5.2.0
	golang.org/x/crypto v0.16.0
)
`)

	writeFile(t, dir, "main.go", `package main

import (
	"log"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	log.Fatal(r.Run(":8080"))
}
`)

	// Create cmd/ directory with a file inside.
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "server"), 0755); err != nil {
		t.Fatalf("failed to create cmd/server directory: %v", err)
	}
	writeFile(t, dir, "cmd/server/main.go", `package main

func main() {}
`)

	writeFile(t, dir, "Dockerfile", `FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o server .

FROM alpine:3.19
COPY --from=builder /app/server /server
EXPOSE 8080
CMD ["/server"]
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
	if result.Port != 8080 {
		t.Errorf("Port = %d, want %d", result.Port, 8080)
	}
	if !result.HasDB {
		t.Error("HasDB = false, want true (gorm.io in go.mod)")
	}
	if !result.HasRedis {
		t.Error("HasRedis = false, want true (go-redis in go.mod)")
	}
	if !result.HasDocker {
		t.Error("HasDocker = false, want true (Dockerfile present)")
	}
	if result.Profile != "saas" {
		t.Errorf("Profile = %q, want %q (has DB + Redis)", result.Profile, "saas")
	}
	if result.Confidence < 0.90 {
		t.Errorf("Confidence = %f, want >= 0.90", result.Confidence)
	}
	// main.go at root should be the entry point (takes priority over cmd/).
	if result.EntryPoint != "main.go" {
		t.Errorf("EntryPoint = %q, want %q", result.EntryPoint, "main.go")
	}

	// Verify indicators.
	expectedIndicators := []string{
		"has existing Docker configuration",
		"found go.mod",
		"uses Gin framework",
		"uses database libraries",
		"uses Redis",
	}
	for _, expected := range expectedIndicators {
		found := false
		for _, ind := range result.Indicators {
			if ind == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing expected indicator %q, got: %v", expected, result.Indicators)
		}
	}
}

// TestDetectFullNextJSProject creates a realistic Next.js project with
// @prisma/client, ioredis, and TypeScript. Verifies detection fields.
func TestDetectFullNextJSProject(t *testing.T) {
	dir := t.TempDir()

	writePackageJSON(t, dir, packageJSON{
		Name: "my-saas-app",
		Dependencies: map[string]string{
			"next":            "^14.0.4",
			"react":           "^18.2.0",
			"react-dom":       "^18.2.0",
			"@prisma/client":  "^5.7.1",
			"ioredis":         "^5.3.2",
			"@auth/core":      "^0.18.0",
			"zod":             "^3.22.4",
			"tailwindcss":     "^3.4.0",
		},
		DevDependencies: map[string]string{
			"typescript":       "^5.3.3",
			"@types/react":     "^18.2.43",
			"@types/node":      "^20.10.4",
			"prisma":           "^5.7.1",
			"eslint":           "^8.55.0",
		},
		Scripts: map[string]string{
			"dev":   "next dev",
			"build": "next build",
			"start": "next start",
		},
	})

	writeFile(t, dir, "docker-compose.yml", `version: "3.8"
services:
  app:
    build: .
    ports:
      - "3000:3000"
    depends_on:
      - postgres
      - redis

  postgres:
    image: postgres:16
    environment:
      POSTGRES_PASSWORD: secret

  redis:
    image: redis:7-alpine
`)

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if result.AppType != AppTypeNextJS {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypeNextJS)
	}
	if result.Language != "typescript" {
		t.Errorf("Language = %q, want %q", result.Language, "typescript")
	}
	if result.Port != 3000 {
		t.Errorf("Port = %d, want %d", result.Port, 3000)
	}
	if !result.HasDB {
		t.Error("HasDB = false, want true (postgres in docker-compose.yml)")
	}
	if !result.HasRedis {
		t.Error("HasRedis = false, want true (ioredis in deps + redis in docker-compose.yml)")
	}
	if !result.HasDocker {
		t.Error("HasDocker = false, want true (docker-compose.yml present)")
	}
	if result.Profile != "saas" {
		t.Errorf("Profile = %q, want %q (has DB + Redis)", result.Profile, "saas")
	}
	if result.Confidence < 0.95 {
		t.Errorf("Confidence = %f, want >= 0.95", result.Confidence)
	}
}

// TestDetectFullStaticProject creates a minimal static website with just
// index.html and style.css. Verifies detection as a static site.
func TestDetectFullStaticProject(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "index.html", `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>My Landing Page</title>
    <link rel="stylesheet" href="style.css">
</head>
<body>
    <header>
        <h1>Welcome to My Site</h1>
        <nav>
            <a href="#features">Features</a>
            <a href="#pricing">Pricing</a>
            <a href="#contact">Contact</a>
        </nav>
    </header>
    <main>
        <section id="features">
            <h2>Features</h2>
            <p>Amazing features here.</p>
        </section>
    </main>
    <script src="app.js"></script>
</body>
</html>
`)

	writeFile(t, dir, "style.css", `* {
    margin: 0;
    padding: 0;
    box-sizing: border-box;
}

body {
    font-family: system-ui, -apple-system, sans-serif;
    line-height: 1.6;
    color: #333;
}

header {
    background: #1a1a2e;
    color: white;
    padding: 2rem;
}
`)

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
	if result.Port != 80 {
		t.Errorf("Port = %d, want %d", result.Port, 80)
	}
	if result.Framework != "" {
		t.Errorf("Framework = %q, want empty string", result.Framework)
	}
	if result.HasDB {
		t.Error("HasDB = true, want false for static site")
	}
	if result.HasRedis {
		t.Error("HasRedis = true, want false for static site")
	}
	if result.HasDocker {
		t.Error("HasDocker = true, want false for static site with no Docker files")
	}
	if result.Profile != "static" {
		t.Errorf("Profile = %q, want %q", result.Profile, "static")
	}
	if result.Confidence < 0.70 {
		t.Errorf("Confidence = %f, want >= 0.70", result.Confidence)
	}

	// Verify indicators.
	foundHTMLIndicator := false
	for _, ind := range result.Indicators {
		if ind == "found index.html" {
			foundHTMLIndicator = true
			break
		}
	}
	if !foundHTMLIndicator {
		t.Errorf("missing expected indicator 'found index.html', got: %v", result.Indicators)
	}
}

// TestDetectRealWorldProject simulates a real monorepo with both
// package.json and go.mod along with a Dockerfile. Verifies that detection
// picks the more specific type (Next.js takes priority over Node, which takes
// priority over Go when package.json has "next").
func TestDetectRealWorldProject(t *testing.T) {
	t.Run("nextjs wins over go in monorepo", func(t *testing.T) {
		dir := t.TempDir()

		writePackageJSON(t, dir, packageJSON{
			Name: "monorepo-app",
			Dependencies: map[string]string{
				"next":    "^14.0.0",
				"react":   "^18.2.0",
				"express": "^4.18.2",
			},
			DevDependencies: map[string]string{
				"typescript": "^5.3.0",
			},
		})

		writeFile(t, dir, "go.mod", `module github.com/mycompany/monorepo

go 1.21

require (
	github.com/gin-gonic/gin v1.9.1
	gorm.io/gorm v1.25.5
)
`)

		writeFile(t, dir, "Dockerfile", `FROM node:20-alpine
WORKDIR /app
COPY . .
RUN npm install && npm run build
CMD ["npm", "start"]
`)

		writeFile(t, dir, "docker-compose.yml", `version: "3.8"
services:
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

		// Next.js should win because it is checked before Node and Go.
		if result.AppType != AppTypeNextJS {
			t.Errorf("AppType = %q, want %q (Next.js should take priority)", result.AppType, AppTypeNextJS)
		}
		if result.Language != "typescript" {
			t.Errorf("Language = %q, want %q", result.Language, "typescript")
		}
		if !result.HasDocker {
			t.Error("HasDocker = false, want true")
		}
		if !result.HasDB {
			t.Error("HasDB = false, want true (postgres in docker-compose.yml)")
		}
		if !result.HasRedis {
			t.Error("HasRedis = false, want true (redis in docker-compose.yml)")
		}
		if result.Profile != "saas" {
			t.Errorf("Profile = %q, want %q", result.Profile, "saas")
		}
	})

	t.Run("nestjs wins over plain node", func(t *testing.T) {
		dir := t.TempDir()

		writePackageJSON(t, dir, packageJSON{
			Name: "nestjs-api",
			Dependencies: map[string]string{
				"@nestjs/core":     "^10.0.0",
				"@nestjs/common":   "^10.0.0",
				"@nestjs/typeorm":  "^10.0.0",
				"express":          "^4.18.0",
				"ioredis":          "^5.3.0",
			},
		})

		result, err := Detect(dir)
		if err != nil {
			t.Fatalf("Detect() returned error: %v", err)
		}

		if result.AppType != AppTypeNestJS {
			t.Errorf("AppType = %q, want %q (NestJS should take priority over Express)", result.AppType, AppTypeNestJS)
		}
		if result.Framework != "NestJS" {
			t.Errorf("Framework = %q, want %q", result.Framework, "NestJS")
		}
		if !result.HasDB {
			t.Error("HasDB = false, want true (TypeORM detected)")
		}
		if !result.HasRedis {
			t.Error("HasRedis = false, want true (ioredis in deps)")
		}
		if result.Profile != "saas" {
			t.Errorf("Profile = %q, want %q", result.Profile, "saas")
		}
	})

	t.Run("go project with no web framework", func(t *testing.T) {
		dir := t.TempDir()

		writeFile(t, dir, "go.mod", `module github.com/mycompany/cli-tool

go 1.21

require (
	github.com/spf13/cobra v1.8.0
	golang.org/x/text v0.14.0
)
`)

		writeFile(t, dir, "main.go", `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`)

		result, err := Detect(dir)
		if err != nil {
			t.Fatalf("Detect() returned error: %v", err)
		}

		if result.AppType != AppTypeGo {
			t.Errorf("AppType = %q, want %q", result.AppType, AppTypeGo)
		}
		if result.Framework != "" {
			t.Errorf("Framework = %q, want empty string (no web framework)", result.Framework)
		}
		if result.HasDB {
			t.Error("HasDB = true, want false")
		}
		if result.HasRedis {
			t.Error("HasRedis = true, want false")
		}
		if result.Profile != "bare" {
			t.Errorf("Profile = %q, want %q", result.Profile, "bare")
		}
	})

	t.Run("python django with full stack", func(t *testing.T) {
		dir := t.TempDir()

		writeFile(t, dir, "requirements.txt", `django==4.2.8
djangorestframework==3.14.0
psycopg2-binary==2.9.9
redis==5.0.1
celery==5.3.6
django-cors-headers==4.3.1
gunicorn==21.2.0
`)

		writeFile(t, dir, "manage.py", `#!/usr/bin/env python
"""Django's command-line utility for administrative tasks."""
import os
import sys

def main():
    os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'config.settings')
    from django.core.management import execute_from_command_line
    execute_from_command_line(sys.argv)

if __name__ == '__main__':
    main()
`)

		writeFile(t, dir, "Dockerfile", `FROM python:3.12-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install -r requirements.txt
COPY . .
CMD ["gunicorn", "config.wsgi:application", "--bind", "0.0.0.0:8000"]
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
		if !result.HasDB {
			t.Error("HasDB = false, want true (psycopg2 in requirements)")
		}
		if !result.HasRedis {
			t.Error("HasRedis = false, want true (redis in requirements)")
		}
		if !result.HasDocker {
			t.Error("HasDocker = false, want true")
		}
		if result.Profile != "saas" {
			t.Errorf("Profile = %q, want %q", result.Profile, "saas")
		}
	})

	t.Run("rust with actix and diesel", func(t *testing.T) {
		dir := t.TempDir()

		writeFile(t, dir, "Cargo.toml", `[package]
name = "my-api"
version = "0.1.0"
edition = "2021"

[dependencies]
actix-web = "4"
actix-rt = "2"
diesel = { version = "2", features = ["postgres"] }
serde = { version = "1", features = ["derive"] }
serde_json = "1"
dotenvy = "0.15"
`)

		result, err := Detect(dir)
		if err != nil {
			t.Fatalf("Detect() returned error: %v", err)
		}

		if result.AppType != AppTypeRust {
			t.Errorf("AppType = %q, want %q", result.AppType, AppTypeRust)
		}
		if result.Framework != "Actix Web" {
			t.Errorf("Framework = %q, want %q", result.Framework, "Actix Web")
		}
		if !result.HasDB {
			t.Error("HasDB = false, want true (diesel in Cargo.toml)")
		}
		if result.Profile != "server" {
			t.Errorf("Profile = %q, want %q (has DB but no Redis)", result.Profile, "server")
		}
	})
}

// TestDetectFullGoProjectDBOnly verifies that a Go project with database but
// no Redis gets the "server" profile (not "saas").
func TestDetectFullGoProjectDBOnly(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "go.mod", `module github.com/mycompany/api

go 1.21

require (
	github.com/labstack/echo/v4 v4.11.4
	gorm.io/gorm v1.25.5
	gorm.io/driver/postgres v1.5.4
)
`)

	writeFile(t, dir, "main.go", `package main

import "github.com/labstack/echo/v4"

func main() {
	e := echo.New()
	e.Logger.Fatal(e.Start(":8080"))
}
`)

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if result.AppType != AppTypeGo {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypeGo)
	}
	if result.Framework != "Echo" {
		t.Errorf("Framework = %q, want %q", result.Framework, "Echo")
	}
	if !result.HasDB {
		t.Error("HasDB = false, want true")
	}
	if result.HasRedis {
		t.Error("HasRedis = true, want false (no redis deps)")
	}
	if result.Profile != "server" {
		t.Errorf("Profile = %q, want %q (DB only)", result.Profile, "server")
	}
}

// TestDetectFullNodeProjectBare verifies that a Node project with no database
// or Redis gets the "bare" profile.
func TestDetectFullNodeProjectBare(t *testing.T) {
	dir := t.TempDir()

	writePackageJSON(t, dir, packageJSON{
		Name: "simple-api",
		Main: "index.js",
		Dependencies: map[string]string{
			"fastify": "^4.25.0",
			"cors":    "^2.8.5",
		},
	})

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if result.AppType != AppTypeNode {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypeNode)
	}
	if result.Framework != "Fastify" {
		t.Errorf("Framework = %q, want %q", result.Framework, "Fastify")
	}
	if result.HasDB {
		t.Error("HasDB = true, want false")
	}
	if result.HasRedis {
		t.Error("HasRedis = true, want false")
	}
	if result.Profile != "bare" {
		t.Errorf("Profile = %q, want %q", result.Profile, "bare")
	}
}

// TestDetectStaticInBuildDir verifies detection of a static site when
// index.html is in the build/ directory (common for CRA apps).
func TestDetectStaticInBuildDir(t *testing.T) {
	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "build"), 0755); err != nil {
		t.Fatalf("failed to create build/ dir: %v", err)
	}
	writeFile(t, dir, "build/index.html", `<!DOCTYPE html>
<html><head><title>Built App</title></head><body></body></html>`)

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if result.AppType != AppTypeStatic {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypeStatic)
	}
	if result.Profile != "static" {
		t.Errorf("Profile = %q, want %q", result.Profile, "static")
	}
}

// TestDetectStaticInDistDir verifies detection of a static site when
// index.html is in the dist/ directory (common for Vite apps).
func TestDetectStaticInDistDir(t *testing.T) {
	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "dist"), 0755); err != nil {
		t.Fatalf("failed to create dist/ dir: %v", err)
	}
	writeFile(t, dir, "dist/index.html", `<!DOCTYPE html>
<html><head><title>Dist App</title></head><body></body></html>`)

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if result.AppType != AppTypeStatic {
		t.Errorf("AppType = %q, want %q", result.AppType, AppTypeStatic)
	}
	if result.Profile != "static" {
		t.Errorf("Profile = %q, want %q", result.Profile, "static")
	}
}

// TestDetectPythonDBOnlyGetServerProfile verifies that a Python project with
// database but no Redis gets the "server" profile.
func TestDetectPythonDBOnlyGetServerProfile(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "requirements.txt", `django==4.2.8
psycopg2-binary==2.9.9
gunicorn==21.2.0
`)

	writeFile(t, dir, "manage.py", `#!/usr/bin/env python
import sys
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
	if !result.HasDB {
		t.Error("HasDB = false, want true")
	}
	if result.HasRedis {
		t.Error("HasRedis = true, want false")
	}
	if result.Profile != "server" {
		t.Errorf("Profile = %q, want %q (has DB but no Redis)", result.Profile, "server")
	}
}

// TestDetectProfileAssignment exercises the recommendProfile logic through
// full detection for various combinations.
func TestDetectProfileAssignment(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, dir string)
		profile string
	}{
		{
			name: "static: plain HTML",
			setup: func(t *testing.T, dir string) {
				writeFile(t, dir, "index.html", "<html></html>")
			},
			profile: "static",
		},
		{
			name: "bare: node with no services",
			setup: func(t *testing.T, dir string) {
				writePackageJSON(t, dir, packageJSON{
					Dependencies: map[string]string{"koa": "^2.15.0"},
				})
			},
			profile: "bare",
		},
		{
			name: "server: node with DB only",
			setup: func(t *testing.T, dir string) {
				writePackageJSON(t, dir, packageJSON{
					Dependencies: map[string]string{
						"@nestjs/core":    "^10.0.0",
						"@nestjs/typeorm": "^10.0.0",
					},
				})
			},
			profile: "server",
		},
		{
			name: "saas: node with DB and Redis",
			setup: func(t *testing.T, dir string) {
				writePackageJSON(t, dir, packageJSON{
					Dependencies: map[string]string{
						"express": "^4.18.0",
						"bull":    "^4.12.0",
					},
				})
				writeFile(t, dir, "docker-compose.yml", `services:
  app:
    build: .
  postgres:
    image: postgres:16
`)
			},
			profile: "saas",
		},
		{
			name: "bare: go with no deps",
			setup: func(t *testing.T, dir string) {
				writeFile(t, dir, "go.mod", `module example.com/bare-go
go 1.21
require github.com/gofiber/fiber/v2 v2.51.0
`)
			},
			profile: "bare",
		},
		{
			name: "server: python with sqlalchemy only",
			setup: func(t *testing.T, dir string) {
				writeFile(t, dir, "requirements.txt", "fastapi\nsqlalchemy\nuvicorn\n")
			},
			profile: "server",
		},
		{
			name: "static with docker still returns static",
			setup: func(t *testing.T, dir string) {
				writeFile(t, dir, "index.html", "<html></html>")
				writeFile(t, dir, "Dockerfile", "FROM nginx:alpine\n")
			},
			profile: "static",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)

			result, err := Detect(dir)
			if err != nil {
				t.Fatalf("Detect() returned error: %v", err)
			}
			if result.Profile != tt.profile {
				t.Errorf("Profile = %q, want %q (AppType=%q, HasDB=%v, HasRedis=%v)",
					result.Profile, tt.profile, result.AppType, result.HasDB, result.HasRedis)
			}
		})
	}
}

// TestDetectRedisViaBullMQ verifies that bullmq dependency is detected as
// Redis usage.
func TestDetectRedisViaBullMQ(t *testing.T) {
	dir := t.TempDir()

	writePackageJSON(t, dir, packageJSON{
		Name: "queue-worker",
		Dependencies: map[string]string{
			"express": "^4.18.0",
			"bullmq":  "^4.14.0",
		},
	})

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if !result.HasRedis {
		t.Error("HasRedis = false, want true (bullmq implies Redis)")
	}
}

// TestDetectRedisViaPythonRequirements verifies that "redis" in
// requirements.txt is detected as Redis usage.
func TestDetectRedisViaPythonRequirements(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "requirements.txt", `flask==3.0.0
redis==5.0.1
celery==5.3.6
`)

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if !result.HasRedis {
		t.Error("HasRedis = false, want true (redis in requirements.txt)")
	}
}

// TestDetectGoRedisLibrary verifies that go-redis or similar Redis library
// in go.mod is detected as Redis usage.
func TestDetectGoRedisLibrary(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "go.mod", `module example.com/redis-app

go 1.21

require (
	github.com/gofiber/fiber/v2 v2.51.0
	github.com/redis/go-redis/v9 v9.3.0
)
`)

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if !result.HasRedis {
		t.Error("HasRedis = false, want true (redis in go.mod)")
	}
}

// TestDetectMultipleDockerFiles verifies detection when both Dockerfile and
// docker-compose.yml exist.
func TestDetectMultipleDockerFiles(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "go.mod", `module example.com/app
go 1.21
`)
	writeFile(t, dir, "main.go", "package main\nfunc main() {}\n")
	writeFile(t, dir, "Dockerfile", "FROM golang:1.21-alpine\n")
	writeFile(t, dir, "docker-compose.yml", `services:
  app:
    build: .
`)

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if !result.HasDocker {
		t.Error("HasDocker = false, want true (both Dockerfile and docker-compose.yml)")
	}
}

// TestDetectServiceDetectionFromComposeOnly verifies that database and Redis
// services are detected from docker-compose.yml even when no language-level
// dependencies reference them.
func TestDetectServiceDetectionFromComposeOnly(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "go.mod", `module example.com/plainapp
go 1.21
require github.com/labstack/echo/v4 v4.11.0
`)

	writeFile(t, dir, "docker-compose.yml", `services:
  app:
    build: .
  db:
    image: postgres:16
  cache:
    image: redis:7-alpine
`)

	result, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if !result.HasDB {
		t.Error("HasDB = false, want true (postgres in docker-compose.yml)")
	}
	if !result.HasRedis {
		t.Error("HasRedis = false, want true (redis in docker-compose.yml)")
	}
	if !result.HasDocker {
		t.Error("HasDocker = false, want true (docker-compose.yml exists)")
	}
	if result.Profile != "saas" {
		t.Errorf("Profile = %q, want %q", result.Profile, "saas")
	}
}
