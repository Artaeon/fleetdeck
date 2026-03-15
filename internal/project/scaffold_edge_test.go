package project

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/profiles"
	"github.com/fleetdeck/fleetdeck/internal/templates"
)

// newMinimalProfile creates a profile with sensible defaults for testing.
func newMinimalProfile(name string) *profiles.Profile {
	return &profiles.Profile{
		Name:        name,
		Description: "Test profile",
		Compose:     "services:\n  app:\n    build: .\n    container_name: {{.Name}}-app\n",
		EnvTemplate: "APP_NAME={{.Name}}\n",
	}
}

// newMinimalTemplate creates a template with sensible defaults for testing.
func newMinimalTemplate() *templates.Template {
	return &templates.Template{
		Name:       "test",
		Dockerfile: "FROM alpine:latest\nCMD [\"echo\", \"hello\"]",
		Compose:    "services: {}",
		Workflow:   "name: Test CI\non:\n  push:\n    branches: [main]",
		GitIgnore:  ".env\n",
	}
}

func TestScaffoldFromProfileNilProfile(t *testing.T) {
	projectPath := t.TempDir()
	tmpl := newMinimalTemplate()
	profileData := profiles.ProfileData{Name: "testapp", Domain: "testapp.example.com"}
	tmplData := templates.TemplateData{Name: "testapp", Domain: "testapp.example.com"}

	// Passing a nil profile should cause a panic or error, not silently succeed.
	// We use recover to catch a nil pointer dereference panic.
	panicked := false
	var panicVal interface{}
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
				panicVal = r
			}
		}()
		_ = ScaffoldFromProfile(projectPath, nil, tmpl, profileData, tmplData)
	}()

	// The function should either panic (nil deref) or return an error.
	// We just verify it does not silently succeed and create files.
	if !panicked {
		// If it didn't panic, it should have returned an error or at minimum
		// not created any files. Check that no files were created.
		entries, _ := os.ReadDir(projectPath)
		// The temp dir might have some default entries; check for our files.
		for _, e := range entries {
			if e.Name() == "Dockerfile" || e.Name() == "docker-compose.yml" {
				t.Error("nil profile should not produce scaffold files without panicking or erroring")
			}
		}
	}
	_ = panicVal // acknowledged but not asserted on the specific value
}

func TestScaffoldFromProfileNilTemplate(t *testing.T) {
	projectPath := t.TempDir()
	profile := newMinimalProfile("server")
	profileData := profiles.ProfileData{Name: "testapp", Domain: "testapp.example.com"}
	tmplData := templates.TemplateData{Name: "testapp", Domain: "testapp.example.com"}

	// Passing a nil template should cause a panic or error.
	panicked := false
	var returnedErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		returnedErr = ScaffoldFromProfile(projectPath, profile, nil, profileData, tmplData)
	}()

	if !panicked && returnedErr == nil {
		// Verify no Dockerfile was created (template provides the Dockerfile).
		if _, err := os.Stat(filepath.Join(projectPath, "Dockerfile")); err == nil {
			t.Error("nil template should not produce a Dockerfile without panicking or erroring")
		}
	}
}

func TestScaffoldFromProfileEmptyCompose(t *testing.T) {
	projectPath := t.TempDir()
	profile := &profiles.Profile{
		Name:        "server",
		Description: "Server profile",
		Compose:     "", // Empty compose template
		EnvTemplate: "APP={{.Name}}\n",
	}
	tmpl := newMinimalTemplate()
	profileData := profiles.ProfileData{Name: "testapp", Domain: "testapp.example.com"}
	tmplData := templates.TemplateData{Name: "testapp", Domain: "testapp.example.com"}

	err := ScaffoldFromProfile(projectPath, profile, tmpl, profileData, tmplData)
	if err != nil {
		t.Fatalf("ScaffoldFromProfile() with empty Compose should not error: %v", err)
	}

	// docker-compose.yml should exist but be empty.
	data, err := os.ReadFile(filepath.Join(projectPath, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("docker-compose.yml should exist: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("docker-compose.yml should be empty for empty Compose template, got %d bytes: %q", len(data), string(data))
	}
}

func TestScaffoldFromProfileEmptyDockerfile(t *testing.T) {
	projectPath := t.TempDir()
	profile := newMinimalProfile("server")
	tmpl := &templates.Template{
		Name:       "empty-docker",
		Dockerfile: "", // Empty Dockerfile template
		Compose:    "services: {}",
		Workflow:   "name: CI",
		GitIgnore:  ".env\n",
	}
	profileData := profiles.ProfileData{Name: "testapp", Domain: "testapp.example.com"}
	tmplData := templates.TemplateData{Name: "testapp", Domain: "testapp.example.com"}

	err := ScaffoldFromProfile(projectPath, profile, tmpl, profileData, tmplData)
	if err != nil {
		t.Fatalf("ScaffoldFromProfile() with empty Dockerfile should not error: %v", err)
	}

	// Dockerfile should exist but be empty.
	data, err := os.ReadFile(filepath.Join(projectPath, "Dockerfile"))
	if err != nil {
		t.Fatalf("Dockerfile should exist: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("Dockerfile should be empty for empty Dockerfile template, got %d bytes", len(data))
	}
}

func TestScaffoldFromProfilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permissions test not applicable on Windows")
	}

	projectPath := t.TempDir()
	profile := newMinimalProfile("server")
	tmpl := newMinimalTemplate()
	profileData := profiles.ProfileData{Name: "testapp", Domain: "testapp.example.com"}
	tmplData := templates.TemplateData{Name: "testapp", Domain: "testapp.example.com"}

	err := ScaffoldFromProfile(projectPath, profile, tmpl, profileData, tmplData)
	if err != nil {
		t.Fatalf("ScaffoldFromProfile(): %v", err)
	}

	// Verify regular file permissions (0644).
	regularFiles := []string{
		"Dockerfile",
		"docker-compose.yml",
		".github/workflows/deploy.yml",
		".gitignore",
	}
	for _, f := range regularFiles {
		info, err := os.Stat(filepath.Join(projectPath, f))
		if err != nil {
			t.Errorf("stat %s: %v", f, err)
			continue
		}
		if perm := info.Mode().Perm(); perm != 0644 {
			t.Errorf("%s permissions = %o, want 0644", f, perm)
		}
	}

	// Verify directory permissions (0755).
	dirs := []string{
		".github/workflows",
		"deployments",
	}
	for _, d := range dirs {
		info, err := os.Stat(filepath.Join(projectPath, d))
		if err != nil {
			t.Errorf("stat dir %s: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s should be a directory", d)
			continue
		}
		if perm := info.Mode().Perm(); perm != 0755 {
			t.Errorf("dir %s permissions = %o, want 0755", d, perm)
		}
	}
}

func TestGenerateEnvFromProfilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permissions test not applicable on Windows")
	}

	projectPath := t.TempDir()
	profile := &profiles.Profile{
		Name:        "server",
		EnvTemplate: "APP_NAME={{.Name}}\nSECRET=CHANGEME\n",
	}
	data := profiles.ProfileData{Name: "testapp", Domain: "testapp.example.com"}

	err := GenerateEnvFromProfile(projectPath, profile, data)
	if err != nil {
		t.Fatalf("GenerateEnvFromProfile(): %v", err)
	}

	info, err := os.Stat(filepath.Join(projectPath, ".env"))
	if err != nil {
		t.Fatalf("stat .env: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf(".env permissions = %o, want 0600", perm)
	}
}

func TestReplacePasswordPlaceholdersUniqueness(t *testing.T) {
	// Create a profile with many CHANGEME placeholders.
	envTemplate := `DB_PASSWORD=CHANGEME
REDIS_PASSWORD=CHANGEME
SECRET_KEY=CHANGEME
JWT_SECRET=CHANGEME
API_KEY=CHANGEME
ADMIN_PASSWORD=CHANGEME
`
	profile := &profiles.Profile{
		Name:        "server",
		EnvTemplate: envTemplate,
	}
	data := profiles.ProfileData{Name: "testapp", Domain: "testapp.example.com"}

	projectPath := t.TempDir()
	err := GenerateEnvFromProfile(projectPath, profile, data)
	if err != nil {
		t.Fatalf("GenerateEnvFromProfile(): %v", err)
	}

	content, err := os.ReadFile(filepath.Join(projectPath, ".env"))
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}
	envStr := string(content)

	// None of the CHANGEME placeholders should remain.
	if strings.Contains(envStr, "CHANGEME") {
		t.Error(".env should not contain any CHANGEME placeholders")
	}

	// Extract all generated secret values.
	secrets := make(map[string]bool)
	var secretValues []string
	for _, line := range strings.Split(envStr, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, val := parts[0], parts[1]
		// These are the keys that had CHANGEME values.
		passwordKeys := map[string]bool{
			"DB_PASSWORD":    true,
			"REDIS_PASSWORD": true,
			"SECRET_KEY":     true,
			"JWT_SECRET":     true,
			"API_KEY":        true,
			"ADMIN_PASSWORD": true,
		}
		if passwordKeys[key] {
			if val == "" {
				t.Errorf("%s should have a generated value, got empty", key)
			}
			if secrets[val] {
				t.Errorf("%s has a duplicate secret value %q - all secrets should be unique", key, val)
			}
			secrets[val] = true
			secretValues = append(secretValues, val)
		}
	}

	if len(secretValues) != 6 {
		t.Errorf("expected 6 generated secrets, got %d", len(secretValues))
	}
}

func TestReplacePasswordPlaceholdersLength(t *testing.T) {
	content := "PASS1=CHANGEME\nPASS2=CHANGEME\nPASS3=CHANGEME\n"
	result := replacePasswordPlaceholders(content, "testapp")

	for _, line := range strings.Split(result, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		val := parts[1]
		// GenerateSecret(16) produces 16 bytes = 32 hex characters.
		if len(val) != 32 {
			t.Errorf("%s: secret length = %d, want 32 hex chars (got %q)", parts[0], len(val), val)
		}
		// Verify it's valid hex.
		for _, c := range val {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("%s: secret contains non-hex character %q in %q", parts[0], string(c), val)
				break
			}
		}
	}
}

func TestScaffoldFromProfileCreatesDirStructure(t *testing.T) {
	projectPath := t.TempDir()
	profile := newMinimalProfile("server")
	tmpl := newMinimalTemplate()
	profileData := profiles.ProfileData{Name: "testapp", Domain: "testapp.example.com"}
	tmplData := templates.TemplateData{Name: "testapp", Domain: "testapp.example.com"}

	err := ScaffoldFromProfile(projectPath, profile, tmpl, profileData, tmplData)
	if err != nil {
		t.Fatalf("ScaffoldFromProfile(): %v", err)
	}

	// Verify .github/workflows/ exists and is a directory.
	workflowDir := filepath.Join(projectPath, ".github", "workflows")
	info, err := os.Stat(workflowDir)
	if err != nil {
		t.Fatalf(".github/workflows/ should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error(".github/workflows should be a directory")
	}

	// Verify .github/ is also a directory.
	githubDir := filepath.Join(projectPath, ".github")
	info, err = os.Stat(githubDir)
	if err != nil {
		t.Fatalf(".github/ should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error(".github should be a directory")
	}

	// Verify deployments/ exists and is a directory.
	deploymentsDir := filepath.Join(projectPath, "deployments")
	info, err = os.Stat(deploymentsDir)
	if err != nil {
		t.Fatalf("deployments/ should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("deployments should be a directory")
	}

	// Verify deploy.yml was written inside .github/workflows/.
	deployYml := filepath.Join(workflowDir, "deploy.yml")
	data, err := os.ReadFile(deployYml)
	if err != nil {
		t.Fatalf("deploy.yml should exist: %v", err)
	}
	if !strings.Contains(string(data), "Test CI") {
		t.Error("deploy.yml should contain the workflow content from the template")
	}
}

func TestScaffoldStaticProfilePublicDirContent(t *testing.T) {
	projectPath := t.TempDir()

	projectName := "my-cool-site"
	profile := &profiles.Profile{
		Name:        "static",
		Description: "Static site with Nginx",
		Compose:     "services:\n  nginx:\n    image: nginx:alpine\n    container_name: {{.Name}}-nginx\n",
		EnvTemplate: "# Static\n",
		Nginx:       "server {\n    listen 80;\n    root /usr/share/nginx/html;\n}\n",
	}
	tmpl := &templates.Template{
		Name:       "static",
		Dockerfile: "FROM nginx:alpine\nCOPY public/ /usr/share/nginx/html/",
		Compose:    "services: {}",
		Workflow:   "name: Deploy Static",
		GitIgnore:  ".env\n",
	}
	profileData := profiles.ProfileData{Name: projectName, Domain: projectName + ".example.com"}
	tmplData := templates.TemplateData{Name: projectName, Domain: projectName + ".example.com"}

	err := ScaffoldFromProfile(projectPath, profile, tmpl, profileData, tmplData)
	if err != nil {
		t.Fatalf("ScaffoldFromProfile(): %v", err)
	}

	// Verify public/index.html contains the project name.
	indexPath := filepath.Join(projectPath, "public", "index.html")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("public/index.html should exist: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, projectName) {
		t.Errorf("index.html should contain the project name %q", projectName)
	}

	// The project name should appear in the <title> tag.
	if !strings.Contains(content, "<title>"+projectName+"</title>") {
		t.Error("index.html should contain the project name in the <title> tag")
	}

	// The project name should appear in the <h1> tag.
	if !strings.Contains(content, "<h1>Welcome to "+projectName+"</h1>") {
		t.Error("index.html should contain the project name in the <h1> tag")
	}

	// Should have proper HTML structure.
	if !strings.Contains(content, "<!DOCTYPE html>") {
		t.Error("index.html should have DOCTYPE declaration")
	}
	if !strings.Contains(content, "<html") {
		t.Error("index.html should have <html> tag")
	}
	if !strings.Contains(content, "</html>") {
		t.Error("index.html should have closing </html> tag")
	}
	if !strings.Contains(content, "FleetDeck") {
		t.Error("index.html should mention FleetDeck")
	}
}

func TestScaffoldFromProfileComposeRendersProfileData(t *testing.T) {
	projectPath := t.TempDir()

	profile := &profiles.Profile{
		Name:    "server",
		Compose: "services:\n  app:\n    container_name: {{.Name}}-app\n    labels:\n      - \"domain={{.Domain}}\"\n",
	}
	tmpl := newMinimalTemplate()
	profileData := profiles.ProfileData{
		Name:   "render-test",
		Domain: "render-test.example.com",
		Port:   3000,
	}
	tmplData := templates.TemplateData{Name: "render-test", Domain: "render-test.example.com"}

	err := ScaffoldFromProfile(projectPath, profile, tmpl, profileData, tmplData)
	if err != nil {
		t.Fatalf("ScaffoldFromProfile(): %v", err)
	}

	data, err := os.ReadFile(filepath.Join(projectPath, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("reading docker-compose.yml: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "render-test-app") {
		t.Error("docker-compose.yml should contain rendered container name")
	}
	if !strings.Contains(content, "render-test.example.com") {
		t.Error("docker-compose.yml should contain rendered domain")
	}
}

func TestScaffoldFromProfileBadComposeTemplate(t *testing.T) {
	projectPath := t.TempDir()

	profile := &profiles.Profile{
		Name:    "broken",
		Compose: "services:\n  app: {{.NonexistentField}}\n",
	}
	tmpl := newMinimalTemplate()
	profileData := profiles.ProfileData{Name: "testapp"}
	tmplData := templates.TemplateData{Name: "testapp"}

	err := ScaffoldFromProfile(projectPath, profile, tmpl, profileData, tmplData)
	if err == nil {
		t.Fatal("ScaffoldFromProfile() should return error for bad compose template")
	}
	if !strings.Contains(err.Error(), "rendering docker-compose.yml from profile") {
		t.Errorf("error should mention compose rendering, got: %v", err)
	}
}

func TestScaffoldFromProfileBadDockerfileTemplate(t *testing.T) {
	projectPath := t.TempDir()

	profile := newMinimalProfile("server")
	tmpl := &templates.Template{
		Name:       "broken",
		Dockerfile: "FROM {{.MissingField}}",
		Compose:    "services: {}",
		Workflow:   "name: CI",
		GitIgnore:  ".env\n",
	}
	profileData := profiles.ProfileData{Name: "testapp"}
	tmplData := templates.TemplateData{Name: "testapp"}

	err := ScaffoldFromProfile(projectPath, profile, tmpl, profileData, tmplData)
	if err == nil {
		t.Fatal("ScaffoldFromProfile() should return error for bad Dockerfile template")
	}
	if !strings.Contains(err.Error(), "rendering Dockerfile") {
		t.Errorf("error should mention Dockerfile rendering, got: %v", err)
	}
}

func TestScaffoldFromProfileWithNginxConfig(t *testing.T) {
	projectPath := t.TempDir()

	nginxConf := "server {\n    listen 80;\n    server_name _;\n    root /var/www/html;\n}\n"
	profile := &profiles.Profile{
		Name:    "static",
		Compose: "services:\n  nginx:\n    image: nginx:alpine\n",
		Nginx:   nginxConf,
	}
	tmpl := newMinimalTemplate()
	profileData := profiles.ProfileData{Name: "static-test", Domain: "static.example.com"}
	tmplData := templates.TemplateData{Name: "static-test", Domain: "static.example.com"}

	err := ScaffoldFromProfile(projectPath, profile, tmpl, profileData, tmplData)
	if err != nil {
		t.Fatalf("ScaffoldFromProfile(): %v", err)
	}

	// Verify nginx.conf was created with the correct content.
	data, err := os.ReadFile(filepath.Join(projectPath, "nginx.conf"))
	if err != nil {
		t.Fatalf("nginx.conf should exist: %v", err)
	}
	if string(data) != nginxConf {
		t.Errorf("nginx.conf content mismatch: got %q, want %q", string(data), nginxConf)
	}
}

func TestScaffoldFromProfileNoNginxForNonStaticWithoutNginxField(t *testing.T) {
	projectPath := t.TempDir()

	profile := &profiles.Profile{
		Name:    "server",
		Compose: "services:\n  app:\n    build: .\n",
		// Nginx field is empty.
	}
	tmpl := newMinimalTemplate()
	profileData := profiles.ProfileData{Name: "backend", Domain: "backend.example.com"}
	tmplData := templates.TemplateData{Name: "backend", Domain: "backend.example.com"}

	err := ScaffoldFromProfile(projectPath, profile, tmpl, profileData, tmplData)
	if err != nil {
		t.Fatalf("ScaffoldFromProfile(): %v", err)
	}

	// nginx.conf should NOT be created when Nginx field is empty.
	if _, err := os.Stat(filepath.Join(projectPath, "nginx.conf")); !os.IsNotExist(err) {
		t.Error("nginx.conf should not be created when Nginx field is empty")
	}

	// public/ directory should NOT be created for non-static profiles.
	if _, err := os.Stat(filepath.Join(projectPath, "public")); !os.IsNotExist(err) {
		t.Error("public/ should not be created for non-static profiles")
	}
}

func TestGenerateEnvFromProfileEmptyTemplate(t *testing.T) {
	projectPath := t.TempDir()

	profile := &profiles.Profile{
		Name:        "bare",
		EnvTemplate: "",
	}
	data := profiles.ProfileData{Name: "testapp"}

	err := GenerateEnvFromProfile(projectPath, profile, data)
	if err != nil {
		t.Fatalf("GenerateEnvFromProfile() with empty template: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(projectPath, ".env"))
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}
	if len(content) != 0 {
		t.Errorf(".env should be empty for empty template, got %d bytes: %q", len(content), string(content))
	}
}

func TestGenerateEnvFromProfileNoPlaceholders(t *testing.T) {
	projectPath := t.TempDir()

	profile := &profiles.Profile{
		Name:        "server",
		EnvTemplate: "APP_NAME={{.Name}}\nPORT=8080\nDEBUG=false\n",
	}
	data := profiles.ProfileData{Name: "myapp", Domain: "myapp.example.com"}

	err := GenerateEnvFromProfile(projectPath, profile, data)
	if err != nil {
		t.Fatalf("GenerateEnvFromProfile(): %v", err)
	}

	content, err := os.ReadFile(filepath.Join(projectPath, ".env"))
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}
	envStr := string(content)

	if !strings.Contains(envStr, "APP_NAME=myapp") {
		t.Error(".env should contain rendered APP_NAME")
	}
	if !strings.Contains(envStr, "PORT=8080") {
		t.Error(".env should contain PORT=8080")
	}
	if !strings.Contains(envStr, "DEBUG=false") {
		t.Error(".env should contain DEBUG=false")
	}
}
