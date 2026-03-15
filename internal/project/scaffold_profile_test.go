package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/profiles"
	"github.com/fleetdeck/fleetdeck/internal/templates"
)

func TestScaffoldFromProfile(t *testing.T) {
	projectPath := t.TempDir()

	profile := &profiles.Profile{
		Name:        "server",
		Description: "Basic server profile",
		Compose: `services:
  app:
    build: .
    container_name: {{.Name}}-app
    labels:
      - "traefik.http.routers.{{.Name}}.rule=Host(` + "`{{.Domain}}`" + `)"
`,
		EnvTemplate: "APP_NAME={{.Name}}\n",
	}

	tmpl := &templates.Template{
		Name:        "go",
		Description: "Go application",
		Dockerfile:  "FROM golang:1.22-alpine\nWORKDIR /app\nCOPY . .\nRUN go build -o server .\nCMD [\"./server\"]",
		Compose:     "services: {}", // Not used in ScaffoldFromProfile; profile compose is used instead.
		Workflow:    "name: Deploy\non:\n  push:\n    branches: [main]\njobs:\n  deploy:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4",
		GitIgnore:   ".env\n*.log\ntmp/\n",
		EnvTemplate: "APP={{.Name}}\n",
	}

	profileData := profiles.ProfileData{
		Name:   "testproject",
		Domain: "testproject.example.com",
		Port:   8080,
	}

	tmplData := templates.TemplateData{
		Name:   "testproject",
		Domain: "testproject.example.com",
	}

	err := ScaffoldFromProfile(projectPath, profile, tmpl, profileData, tmplData)
	if err != nil {
		t.Fatalf("ScaffoldFromProfile() error: %v", err)
	}

	// Verify all expected files were created.
	expectedFiles := []struct {
		path    string
		content string // substring to check; empty means just check existence
	}{
		{
			path:    "Dockerfile",
			content: "FROM golang:1.22-alpine",
		},
		{
			path:    "docker-compose.yml",
			content: "testproject-app",
		},
		{
			path:    ".github/workflows/deploy.yml",
			content: "Deploy",
		},
		{
			path:    ".gitignore",
			content: ".env",
		},
	}

	for _, ef := range expectedFiles {
		fullPath := filepath.Join(projectPath, ef.path)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			t.Errorf("expected file %s to exist, got error: %v", ef.path, err)
			continue
		}
		if ef.content != "" && !strings.Contains(string(data), ef.content) {
			t.Errorf("file %s should contain %q, got:\n%s", ef.path, ef.content, string(data))
		}
	}

	// Verify docker-compose.yml was rendered from the profile (not the template).
	composeData, _ := os.ReadFile(filepath.Join(projectPath, "docker-compose.yml"))
	composeStr := string(composeData)
	if !strings.Contains(composeStr, "testproject.example.com") {
		t.Error("docker-compose.yml should contain the rendered domain from profile")
	}

	// Verify the directory structure was created.
	dirs := []string{
		filepath.Join(projectPath, ".github", "workflows"),
		filepath.Join(projectPath, "deployments"),
	}
	for _, d := range dirs {
		info, err := os.Stat(d)
		if err != nil {
			t.Errorf("expected directory %s to exist: %v", d, err)
		} else if !info.IsDir() {
			t.Errorf("%s should be a directory", d)
		}
	}
}

func TestGenerateEnvFromProfile(t *testing.T) {
	projectPath := t.TempDir()

	profile := &profiles.Profile{
		Name: "server",
		EnvTemplate: `APP_NAME={{.Name}}
APP_DOMAIN={{.Domain}}
DB_PASSWORD=CHANGEME
REDIS_PASSWORD=CHANGEME
STATIC_VALUE=keep-this
`,
	}

	data := profiles.ProfileData{
		Name:   "myapp",
		Domain: "myapp.example.com",
	}

	err := GenerateEnvFromProfile(projectPath, profile, data)
	if err != nil {
		t.Fatalf("GenerateEnvFromProfile() error: %v", err)
	}

	envPath := filepath.Join(projectPath, ".env")
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}

	envStr := string(content)

	// Template variables should be rendered.
	if !strings.Contains(envStr, "APP_NAME=myapp") {
		t.Error(".env should contain APP_NAME=myapp")
	}
	if !strings.Contains(envStr, "APP_DOMAIN=myapp.example.com") {
		t.Error(".env should contain APP_DOMAIN=myapp.example.com")
	}

	// CHANGEME placeholders should be replaced with generated secrets.
	if strings.Contains(envStr, "CHANGEME") {
		t.Error(".env should not contain any CHANGEME placeholders after generation")
	}

	// Static values should remain.
	if !strings.Contains(envStr, "STATIC_VALUE=keep-this") {
		t.Error(".env should preserve static values")
	}

	// File should have restrictive permissions (0600).
	info, err := os.Stat(envPath)
	if err != nil {
		t.Fatalf("stat .env: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf(".env permissions = %o, want 0600", perm)
	}

	// Each replaced password line should have a unique secret.
	var dbPass, redisPass string
	for _, line := range strings.Split(envStr, "\n") {
		if strings.HasPrefix(line, "DB_PASSWORD=") {
			dbPass = strings.TrimPrefix(line, "DB_PASSWORD=")
		}
		if strings.HasPrefix(line, "REDIS_PASSWORD=") {
			redisPass = strings.TrimPrefix(line, "REDIS_PASSWORD=")
		}
	}
	if dbPass == "" {
		t.Error("DB_PASSWORD should have a generated value")
	}
	if redisPass == "" {
		t.Error("REDIS_PASSWORD should have a generated value")
	}
	if dbPass == redisPass {
		t.Error("DB_PASSWORD and REDIS_PASSWORD should have different generated secrets")
	}
}

func TestReplacePasswordPlaceholders(t *testing.T) {
	tests := []struct {
		name    string
		content string
		project string
		check   func(t *testing.T, result string)
	}{
		{
			name:    "single CHANGEME placeholder",
			content: "DB_PASSWORD=CHANGEME\n",
			project: "myapp",
			check: func(t *testing.T, result string) {
				if strings.Contains(result, "CHANGEME") {
					t.Error("CHANGEME should be replaced")
				}
				if !strings.HasPrefix(result, "DB_PASSWORD=") {
					t.Error("DB_PASSWORD key should be preserved")
				}
			},
		},
		{
			name:    "multiple CHANGEME placeholders",
			content: "PASS1=CHANGEME\nPASS2=CHANGEME\n",
			project: "myapp",
			check: func(t *testing.T, result string) {
				if strings.Contains(result, "CHANGEME") {
					t.Error("all CHANGEME instances should be replaced")
				}
				// Extract the two passwords and verify they are different.
				lines := strings.Split(result, "\n")
				var vals []string
				for _, line := range lines {
					if strings.Contains(line, "=") {
						parts := strings.SplitN(line, "=", 2)
						if len(parts) == 2 && parts[1] != "" {
							vals = append(vals, parts[1])
						}
					}
				}
				if len(vals) >= 2 && vals[0] == vals[1] {
					t.Error("each CHANGEME should be replaced with a unique secret")
				}
			},
		},
		{
			name:    "no CHANGEME placeholders",
			content: "DB_HOST=localhost\nDB_PORT=5432\n",
			project: "myapp",
			check: func(t *testing.T, result string) {
				if result != "DB_HOST=localhost\nDB_PORT=5432\n" {
					t.Errorf("content without CHANGEME should be unchanged, got:\n%s", result)
				}
			},
		},
		{
			name:    "empty content",
			content: "",
			project: "myapp",
			check: func(t *testing.T, result string) {
				if result != "" {
					t.Errorf("empty content should remain empty, got %q", result)
				}
			},
		},
		{
			name:    "CHANGEME with openssl pattern",
			content: "SECRET=CHANGEME_$(openssl rand -hex 16)\n",
			project: "myapp",
			check: func(t *testing.T, result string) {
				if strings.Contains(result, "CHANGEME") {
					t.Error("CHANGEME with openssl pattern should be replaced")
				}
				if strings.Contains(result, "openssl") {
					t.Error("openssl command should be consumed")
				}
				if !strings.HasPrefix(result, "SECRET=") {
					t.Error("SECRET key should be preserved")
				}
			},
		},
		{
			name:    "CHANGEME mixed with static values",
			content: "STATIC=keep\nDYNAMIC=CHANGEME\nANOTHER_STATIC=also-keep\n",
			project: "app",
			check: func(t *testing.T, result string) {
				if !strings.Contains(result, "STATIC=keep") {
					t.Error("STATIC value should be preserved")
				}
				if !strings.Contains(result, "ANOTHER_STATIC=also-keep") {
					t.Error("ANOTHER_STATIC value should be preserved")
				}
				if strings.Contains(result, "CHANGEME") {
					t.Error("CHANGEME should be replaced")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := replacePasswordPlaceholders(tt.content, tt.project)
			tt.check(t, result)
		})
	}
}

func TestScaffoldStaticProfile(t *testing.T) {
	projectPath := t.TempDir()

	profile := &profiles.Profile{
		Name:        "static",
		Description: "Static site with Nginx",
		Compose: `services:
  nginx:
    image: nginx:alpine
    container_name: {{.Name}}-nginx
`,
		EnvTemplate: "# Static site\n",
		Nginx: `server {
    listen 80;
    server_name _;
    root /usr/share/nginx/html;
    index index.html;
}
`,
	}

	tmpl := &templates.Template{
		Name:        "static",
		Description: "Static site template",
		Dockerfile:  "FROM nginx:alpine\nCOPY public/ /usr/share/nginx/html/",
		Compose:     "services: {}",
		Workflow:    "name: Deploy Static\non:\n  push:\n    branches: [main]",
		GitIgnore:   ".env\nnode_modules/\n",
		EnvTemplate: "",
	}

	profileData := profiles.ProfileData{
		Name:   "my-static-site",
		Domain: "static.example.com",
	}

	tmplData := templates.TemplateData{
		Name:   "my-static-site",
		Domain: "static.example.com",
	}

	err := ScaffoldFromProfile(projectPath, profile, tmpl, profileData, tmplData)
	if err != nil {
		t.Fatalf("ScaffoldFromProfile() with static profile error: %v", err)
	}

	// Verify nginx.conf was created.
	nginxPath := filepath.Join(projectPath, "nginx.conf")
	nginxData, err := os.ReadFile(nginxPath)
	if err != nil {
		t.Fatalf("nginx.conf should exist for static profile: %v", err)
	}
	if !strings.Contains(string(nginxData), "listen 80") {
		t.Error("nginx.conf should contain 'listen 80'")
	}
	if !strings.Contains(string(nginxData), "index index.html") {
		t.Error("nginx.conf should contain 'index index.html'")
	}

	// Verify public/ directory and index.html were created.
	publicDir := filepath.Join(projectPath, "public")
	info, err := os.Stat(publicDir)
	if err != nil {
		t.Fatalf("public/ directory should exist for static profile: %v", err)
	}
	if !info.IsDir() {
		t.Error("public should be a directory")
	}

	indexPath := filepath.Join(publicDir, "index.html")
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("public/index.html should exist: %v", err)
	}
	indexStr := string(indexData)

	if !strings.Contains(indexStr, "my-static-site") {
		t.Error("index.html should contain the project name")
	}
	if !strings.Contains(indexStr, "<!DOCTYPE html>") {
		t.Error("index.html should contain DOCTYPE")
	}
	if !strings.Contains(indexStr, "FleetDeck") {
		t.Error("index.html should mention FleetDeck")
	}

	// Standard files should also exist.
	standardFiles := []string{"Dockerfile", "docker-compose.yml", ".github/workflows/deploy.yml", ".gitignore"}
	for _, f := range standardFiles {
		if _, err := os.Stat(filepath.Join(projectPath, f)); err != nil {
			t.Errorf("expected standard file %s to exist: %v", f, err)
		}
	}
}

func TestScaffoldFromProfileNonStaticNoPublicDir(t *testing.T) {
	projectPath := t.TempDir()

	profile := &profiles.Profile{
		Name:        "server",
		Description: "Server profile",
		Compose:     "services:\n  app:\n    build: .\n",
		EnvTemplate: "",
		// No Nginx field set.
	}

	tmpl := &templates.Template{
		Name:       "go",
		Dockerfile: "FROM golang:1.22\nCMD [\"./app\"]",
		Compose:    "services: {}",
		Workflow:   "name: Deploy",
		GitIgnore:  ".env\n",
	}

	profileData := profiles.ProfileData{Name: "backend", Domain: "backend.test.com"}
	tmplData := templates.TemplateData{Name: "backend", Domain: "backend.test.com"}

	err := ScaffoldFromProfile(projectPath, profile, tmpl, profileData, tmplData)
	if err != nil {
		t.Fatalf("ScaffoldFromProfile() error: %v", err)
	}

	// Non-static profiles should not create nginx.conf.
	if _, err := os.Stat(filepath.Join(projectPath, "nginx.conf")); !os.IsNotExist(err) {
		t.Error("non-static profile should not create nginx.conf")
	}

	// Non-static profiles should not create public/ directory.
	if _, err := os.Stat(filepath.Join(projectPath, "public")); !os.IsNotExist(err) {
		t.Error("non-static profile should not create public/ directory")
	}
}

func TestGenerateEnvFromProfileBadTemplate(t *testing.T) {
	projectPath := t.TempDir()

	profile := &profiles.Profile{
		Name:        "broken",
		EnvTemplate: "{{.InvalidField}}",
	}

	data := profiles.ProfileData{Name: "myapp"}

	err := GenerateEnvFromProfile(projectPath, profile, data)
	if err == nil {
		t.Fatal("GenerateEnvFromProfile with invalid template field should return error")
	}
	if !strings.Contains(err.Error(), "rendering profile env template") {
		t.Errorf("error should mention rendering, got: %v", err)
	}
}

func TestScaffoldFromProfileRendersDockerfileFromTemplate(t *testing.T) {
	projectPath := t.TempDir()

	profile := &profiles.Profile{
		Name:        "server",
		Compose:     "services:\n  app:\n    build: .\n",
		EnvTemplate: "",
	}

	// Dockerfile template uses TemplateData fields.
	tmpl := &templates.Template{
		Name:       "custom",
		Dockerfile: "FROM node:20\nLABEL app={{.Name}}\nLABEL domain={{.Domain}}",
		Compose:    "services: {}",
		Workflow:   "name: CI",
		GitIgnore:  ".env\n",
	}

	profileData := profiles.ProfileData{Name: "frontend", Domain: "frontend.io"}
	tmplData := templates.TemplateData{Name: "frontend", Domain: "frontend.io"}

	err := ScaffoldFromProfile(projectPath, profile, tmpl, profileData, tmplData)
	if err != nil {
		t.Fatalf("ScaffoldFromProfile() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(projectPath, "Dockerfile"))
	if err != nil {
		t.Fatalf("reading Dockerfile: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "LABEL app=frontend") {
		t.Error("Dockerfile should render template data for Name")
	}
	if !strings.Contains(content, "LABEL domain=frontend.io") {
		t.Error("Dockerfile should render template data for Domain")
	}
}
