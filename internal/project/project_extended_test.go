package project

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/profiles"
	"github.com/fleetdeck/fleetdeck/internal/templates"
)

// ---------------------------------------------------------------------------
// ValidateName: exhaustive pattern coverage
// ---------------------------------------------------------------------------

func TestValidateName_AllSingleDigits(t *testing.T) {
	for _, ch := range "0123456789" {
		if err := ValidateName(string(ch)); err != nil {
			t.Errorf("ValidateName(%q) should be valid: %v", string(ch), err)
		}
	}
}

func TestValidateName_HyphenMiddleVariations(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"a-1", true},
		{"1-a", true},
		{"0-0", true},
		{"a-a-a-a-a", true},
		{"1-2-3-4-5", true},
		{"abc-def-ghi", true},
	}
	for _, tt := range tests {
		err := ValidateName(tt.name)
		if tt.valid && err != nil {
			t.Errorf("ValidateName(%q) should be valid: %v", tt.name, err)
		}
		if !tt.valid && err == nil {
			t.Errorf("ValidateName(%q) should be invalid", tt.name)
		}
	}
}

func TestValidateName_ErrorFormatContainsLength(t *testing.T) {
	err := ValidateName("")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "0") {
		t.Errorf("error for empty name should contain '0', got: %v", err)
	}
}

func TestValidateName_UppercaseErrorMessage(t *testing.T) {
	err := ValidateName("MyApp")
	if err == nil {
		t.Fatal("expected error for uppercase")
	}
	if !strings.Contains(err.Error(), "lowercase") {
		t.Errorf("error should mention 'lowercase', got: %v", err)
	}
}

func TestValidateName_ConsecutiveHyphensErrorMessage(t *testing.T) {
	err := ValidateName("a--b")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "consecutive hyphens") {
		t.Errorf("error should mention 'consecutive hyphens', got: %v", err)
	}
}

func TestValidateName_TripleHyphens(t *testing.T) {
	if err := ValidateName("a---b"); err == nil {
		t.Error("triple hyphens should be invalid")
	}
}

// ---------------------------------------------------------------------------
// ComposeUp/Down/Restart/PS: error wrapping with temp dir
// ---------------------------------------------------------------------------

func TestComposeUp_TempDir(t *testing.T) {
	tmpDir := t.TempDir()
	err := ComposeUp(tmpDir)
	if err == nil {
		t.Skip("docker compose is available")
	}
	if !strings.Contains(err.Error(), "docker compose up") {
		t.Errorf("error should mention 'docker compose up', got: %v", err)
	}
}

func TestComposeDown_TempDir(t *testing.T) {
	tmpDir := t.TempDir()
	err := ComposeDown(tmpDir)
	if err == nil {
		t.Skip("docker compose is available")
	}
	if !strings.Contains(err.Error(), "docker compose down") {
		t.Errorf("error should mention 'docker compose down', got: %v", err)
	}
}

func TestComposeRestart_TempDir(t *testing.T) {
	tmpDir := t.TempDir()
	err := ComposeRestart(tmpDir)
	if err == nil {
		t.Skip("docker compose is available")
	}
	if !strings.Contains(err.Error(), "docker compose restart") {
		t.Errorf("error should mention 'docker compose restart', got: %v", err)
	}
}

func TestComposePS_TempDir(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := ComposePS(tmpDir)
	if err == nil {
		t.Skip("docker compose is available")
	}
	if !strings.Contains(err.Error(), "docker compose ps") {
		t.Errorf("error should mention 'docker compose ps', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CountContainers: error path returns 0, 0
// ---------------------------------------------------------------------------

func TestCountContainers_NonexistentPath(t *testing.T) {
	running, total := CountContainers("/nonexistent/path/xyz")
	if running != 0 {
		t.Errorf("running should be 0 for nonexistent path, got %d", running)
	}
	if total != 0 {
		t.Errorf("total should be 0 for nonexistent path, got %d", total)
	}
}

func TestCountContainers_TempDir(t *testing.T) {
	tmpDir := t.TempDir()
	running, total := CountContainers(tmpDir)
	// Without docker, this should return 0, 0.
	if running != 0 || total != 0 {
		t.Logf("CountContainers returned running=%d total=%d (docker may be available)", running, total)
	}
}

// ---------------------------------------------------------------------------
// ComposeLogs: additional edge cases
// ---------------------------------------------------------------------------

func TestComposeLogs_EmptyService(t *testing.T) {
	cmd := ComposeLogs("/tmp/test", "", 0, false)
	// Should have: docker compose logs
	args := cmd.Args[1:]
	if len(args) != 2 || args[0] != "compose" || args[1] != "logs" {
		t.Errorf("unexpected args for empty service: %v", args)
	}
}

func TestComposeLogs_LargeTail(t *testing.T) {
	cmd := ComposeLogs("/tmp/test", "web", 99999, false)
	found := false
	for i, arg := range cmd.Args {
		if arg == "--tail" && i+1 < len(cmd.Args) && cmd.Args[i+1] == "99999" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected --tail 99999 in args: %v", cmd.Args)
	}
}

func TestComposeLogs_DirSet(t *testing.T) {
	cmd := ComposeLogs("/opt/myapp", "api", 10, true)
	if cmd.Dir != "/opt/myapp" {
		t.Errorf("Dir = %q, want %q", cmd.Dir, "/opt/myapp")
	}
}

// ---------------------------------------------------------------------------
// ContainerStatus: JSON parsing of docker output
// ---------------------------------------------------------------------------

func TestContainerStatus_JSONMarshal(t *testing.T) {
	cs := ContainerStatus{
		Name:   "app-web-1",
		State:  "running",
		Status: "Up 5 minutes",
	}
	data, err := json.Marshal(cs)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "app-web-1") {
		t.Error("JSON should contain container name")
	}
}

// ---------------------------------------------------------------------------
// GenerateSecret: edge cases
// ---------------------------------------------------------------------------

func TestGenerateSecret_Length1(t *testing.T) {
	s := GenerateSecret(1)
	if len(s) != 2 {
		t.Errorf("GenerateSecret(1) should produce 2 hex chars, got %d", len(s))
	}
	if _, err := hex.DecodeString(s); err != nil {
		t.Errorf("not valid hex: %v", err)
	}
}

func TestGenerateSecret_NeverEmpty(t *testing.T) {
	for i := 1; i <= 10; i++ {
		s := GenerateSecret(i)
		if len(s) == 0 {
			t.Errorf("GenerateSecret(%d) returned empty string", i)
		}
	}
}

// ---------------------------------------------------------------------------
// replacePasswordPlaceholders: shell command pattern branch
// ---------------------------------------------------------------------------

func TestReplacePasswordPlaceholders_ShellPattern(t *testing.T) {
	// The function has a branch that handles CHANGEME followed by _$(...).
	input := "DB_PASS=CHANGEME_$(openssl rand -hex 16)\nOTHER=static\n"
	result := replacePasswordPlaceholders(input, "app")

	if strings.Contains(result, "CHANGEME") {
		t.Error("CHANGEME should be replaced")
	}
	if strings.Contains(result, "$(openssl") {
		t.Error("shell command should be consumed")
	}
	if !strings.Contains(result, "OTHER=static") {
		t.Error("static values should be preserved")
	}
}

func TestReplacePasswordPlaceholders_MultipleShellPatterns(t *testing.T) {
	input := "A=CHANGEME_$(openssl rand -hex 16)\nB=CHANGEME_$(openssl rand -hex 32)\n"
	result := replacePasswordPlaceholders(input, "app")

	if strings.Contains(result, "CHANGEME") {
		t.Error("all CHANGEME instances should be replaced")
	}
	if strings.Contains(result, "$(openssl") {
		t.Error("all shell commands should be consumed")
	}

	// Extract values.
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d", len(lines))
	}
}

func TestReplacePasswordPlaceholders_NoPlaceholders(t *testing.T) {
	input := "KEY=value\nOTHER=something\n"
	result := replacePasswordPlaceholders(input, "app")
	if result != input {
		t.Errorf("input without CHANGEME should be unchanged, got: %q", result)
	}
}

func TestReplacePasswordPlaceholders_EmptyInput(t *testing.T) {
	result := replacePasswordPlaceholders("", "app")
	if result != "" {
		t.Errorf("empty input should produce empty output, got: %q", result)
	}
}

func TestReplacePasswordPlaceholders_CHANGEMEInValue(t *testing.T) {
	// CHANGEME without underscore after it
	input := "PASS=CHANGEME\n"
	result := replacePasswordPlaceholders(input, "app")
	if strings.Contains(result, "CHANGEME") {
		t.Error("CHANGEME should be replaced")
	}
	parts := strings.SplitN(strings.TrimSpace(result), "=", 2)
	if len(parts) != 2 {
		t.Fatalf("expected key=value, got: %q", result)
	}
	if len(parts[1]) != 32 {
		t.Errorf("secret should be 32 hex chars, got %d", len(parts[1]))
	}
}

// ---------------------------------------------------------------------------
// indexOf: basic tests
// ---------------------------------------------------------------------------

func TestIndexOf_Found(t *testing.T) {
	if got := indexOf("hello world", "world"); got != 6 {
		t.Errorf("indexOf = %d, want 6", got)
	}
}

func TestIndexOf_NotFound(t *testing.T) {
	if got := indexOf("hello", "xyz"); got != -1 {
		t.Errorf("indexOf = %d, want -1", got)
	}
}

func TestIndexOf_EmptySubstr(t *testing.T) {
	if got := indexOf("hello", ""); got != 0 {
		t.Errorf("indexOf = %d, want 0", got)
	}
}

func TestIndexOf_EmptyString(t *testing.T) {
	if got := indexOf("", "x"); got != -1 {
		t.Errorf("indexOf = %d, want -1", got)
	}
}

// ---------------------------------------------------------------------------
// GenerateEnvFile: CHANGEME replacement
// ---------------------------------------------------------------------------

func TestGenerateEnvFile_CHANGEMEReplacement(t *testing.T) {
	tmpDir := t.TempDir()
	tmpl := &templates.Template{
		EnvTemplate: "SECRET={{.Name}}_CHANGEME\nNAME={{.Name}}\n",
	}
	data := templates.TemplateData{Name: "testproject", Domain: "test.dev"}

	if err := GenerateEnvFile(tmpDir, tmpl, data); err != nil {
		t.Fatalf("GenerateEnvFile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".env"))
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}
	s := string(content)

	if strings.Contains(s, "testproject_CHANGEME") {
		t.Error("CHANGEME placeholder should be replaced")
	}
	if !strings.Contains(s, "NAME=testproject") {
		t.Error("non-placeholder values should be preserved")
	}
}

// ---------------------------------------------------------------------------
// GenerateEnvFromProfile: bad template
// ---------------------------------------------------------------------------

func TestGenerateEnvFromProfile_BadTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	profile := &profiles.Profile{
		Name:        "broken",
		EnvTemplate: "APP={{.NonexistentField}}",
	}
	data := profiles.ProfileData{Name: "test"}

	err := GenerateEnvFromProfile(tmpDir, profile, data)
	if err == nil {
		t.Fatal("expected error for bad env template")
	}
	if !strings.Contains(err.Error(), "rendering profile env template") {
		t.Errorf("error should mention 'rendering profile env template', got: %v", err)
	}
}

func TestGenerateEnvFromProfile_InvalidPath(t *testing.T) {
	profile := &profiles.Profile{
		Name:        "server",
		EnvTemplate: "APP={{.Name}}\n",
	}
	data := profiles.ProfileData{Name: "test"}

	err := GenerateEnvFromProfile("/nonexistent/path/xyz", profile, data)
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestGenerateEnvFromProfile_WithCHANGEME(t *testing.T) {
	tmpDir := t.TempDir()
	profile := &profiles.Profile{
		Name:        "server",
		EnvTemplate: "DB_PASS=CHANGEME\nAPI_KEY=CHANGEME\nAPP={{.Name}}\n",
	}
	data := profiles.ProfileData{Name: "myapp", Domain: "myapp.dev"}

	err := GenerateEnvFromProfile(tmpDir, profile, data)
	if err != nil {
		t.Fatalf("GenerateEnvFromProfile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".env"))
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}
	s := string(content)

	if strings.Contains(s, "CHANGEME") {
		t.Error("CHANGEME should be replaced")
	}
	if !strings.Contains(s, "APP=myapp") {
		t.Error("template rendering should work")
	}

	// Both passwords should be unique.
	secrets := make(map[string]bool)
	for _, line := range strings.Split(s, "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 && (parts[0] == "DB_PASS" || parts[0] == "API_KEY") {
			if secrets[parts[1]] {
				t.Errorf("duplicate secret value for %s", parts[0])
			}
			secrets[parts[1]] = true
			if len(parts[1]) != 32 {
				t.Errorf("%s should be 32 hex chars, got %d", parts[0], len(parts[1]))
			}
		}
	}
}

func TestGenerateEnvFromProfile_Permissions(t *testing.T) {
	tmpDir := t.TempDir()
	profile := &profiles.Profile{
		Name:        "server",
		EnvTemplate: "X=1\n",
	}
	data := profiles.ProfileData{Name: "test"}

	if err := GenerateEnvFromProfile(tmpDir, profile, data); err != nil {
		t.Fatalf("GenerateEnvFromProfile: %v", err)
	}

	info, err := os.Stat(filepath.Join(tmpDir, ".env"))
	if err != nil {
		t.Fatalf("stat .env: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf(".env permissions = %o, want 0600", perm)
	}
}

// ---------------------------------------------------------------------------
// ScaffoldFromProfile: additional error paths
// ---------------------------------------------------------------------------

func TestScaffoldFromProfile_InvalidProjectPath(t *testing.T) {
	tmpDir := t.TempDir()
	blockingFile := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blockingFile, []byte("x"), 0644); err != nil {
		t.Fatalf("creating blocking file: %v", err)
	}

	badPath := filepath.Join(blockingFile, "project")
	profile := newMinimalProfile("server")
	tmpl := newMinimalTemplate()
	profileData := profiles.ProfileData{Name: "test"}
	tmplData := templates.TemplateData{Name: "test"}

	err := ScaffoldFromProfile(badPath, profile, tmpl, profileData, tmplData)
	if err == nil {
		t.Fatal("expected error for invalid project path")
	}
}

func TestScaffoldFromProfile_WritesGitignore(t *testing.T) {
	projectPath := t.TempDir()
	profile := newMinimalProfile("server")
	tmpl := &templates.Template{
		Name:       "test",
		Dockerfile: "FROM scratch",
		Compose:    "services: {}",
		Workflow:   "name: CI",
		GitIgnore:  ".env\n*.log\nvendor/\n",
	}
	profileData := profiles.ProfileData{Name: "myapp", Domain: "myapp.dev"}
	tmplData := templates.TemplateData{Name: "myapp", Domain: "myapp.dev"}

	if err := ScaffoldFromProfile(projectPath, profile, tmpl, profileData, tmplData); err != nil {
		t.Fatalf("ScaffoldFromProfile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(projectPath, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if !strings.Contains(string(content), ".env") {
		t.Error(".gitignore should contain .env")
	}
	if !strings.Contains(string(content), "vendor/") {
		t.Error(".gitignore should contain vendor/")
	}
}

// ---------------------------------------------------------------------------
// ScaffoldProject: write failure when directory is read-only
// ---------------------------------------------------------------------------

func TestScaffoldProject_ReadOnlyDir(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatalf("creating dir: %v", err)
	}

	tmpl := testTemplate()
	data := testData()

	// Make directory read-only after creating the subdirs would succeed,
	// but file writes would fail. We need to block the nested dir creation.
	// Create the github/workflows dir first, then make the project dir read-only.
	if err := os.MkdirAll(filepath.Join(projectPath, ".github", "workflows"), 0755); err != nil {
		t.Fatalf("creating nested dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectPath, "deployments"), 0755); err != nil {
		t.Fatalf("creating deployments dir: %v", err)
	}

	// Make the project dir read-only.
	if err := os.Chmod(projectPath, 0555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() {
		os.Chmod(projectPath, 0755)
	})

	err := ScaffoldProject(projectPath, tmpl, data)
	if err == nil {
		t.Fatal("expected error when writing to read-only directory")
	}
}

// ---------------------------------------------------------------------------
// InitAndPushRepo: command sequence verification
// ---------------------------------------------------------------------------

func TestInitAndPushRepo_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	err := InitAndPushRepo(tmpDir, "https://github.com/fake/repo.git")
	if err == nil {
		t.Fatal("expected error (git commit should fail on empty repo)")
	}
	if !strings.Contains(err.Error(), "running git") {
		t.Errorf("error should mention 'running git', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CreateGitHubRepo: error wrapping
// ---------------------------------------------------------------------------

func TestCreateGitHubRepo_WithOrg(t *testing.T) {
	_, err := CreateGitHubRepo("test-org", "test-repo", true)
	if err == nil {
		t.Skip("gh CLI is available")
	}
	if !strings.Contains(err.Error(), "creating GitHub repo") {
		t.Errorf("error should mention 'creating GitHub repo', got: %v", err)
	}
}

func TestCreateGitHubRepo_WithoutOrg(t *testing.T) {
	_, err := CreateGitHubRepo("", "test-repo", false)
	if err == nil {
		t.Skip("gh CLI is available")
	}
	if !strings.Contains(err.Error(), "creating GitHub repo") {
		t.Errorf("error should mention 'creating GitHub repo', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// SetGitHubSecret/DeleteGitHubRepo: error wrapping
// ---------------------------------------------------------------------------

func TestSetGitHubSecret_ErrorWrapping(t *testing.T) {
	err := SetGitHubSecret("fake/repo", "SECRET_KEY", "value123")
	if err == nil {
		t.Skip("gh CLI is available")
	}
	if !strings.Contains(err.Error(), "setting secret SECRET_KEY") {
		t.Errorf("error should mention secret key, got: %v", err)
	}
}

func TestDeleteGitHubRepo_ErrorWrapping(t *testing.T) {
	err := DeleteGitHubRepo("fake-org/fake-repo-xyz")
	if err == nil {
		t.Skip("gh CLI is available")
	}
	if !strings.Contains(err.Error(), "deleting GitHub repo") {
		t.Errorf("error should mention 'deleting GitHub repo', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetServerIP: basic validation
// ---------------------------------------------------------------------------

func TestGetServerIP_ReturnsNonEmpty(t *testing.T) {
	ip, err := GetServerIP()
	if err != nil {
		t.Skip("hostname -I not available: " + err.Error())
	}
	if ip == "" {
		t.Error("GetServerIP should return non-empty string")
	}
	// Should not contain spaces (returns only first IP).
	if strings.Contains(ip, " ") {
		t.Errorf("GetServerIP should return single IP, got: %q", ip)
	}
}

// ---------------------------------------------------------------------------
// ChownProjectDir: error wrapping
// ---------------------------------------------------------------------------

func TestChownProjectDir_NonexistentUser(t *testing.T) {
	tmpDir := t.TempDir()
	err := ChownProjectDir("nonexistent-project-xyz", tmpDir)
	if err == nil {
		t.Skip("chown succeeded (running as root?)")
	}
	if !strings.Contains(err.Error(), "chown") {
		t.Errorf("error should mention 'chown', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CreateLinuxUser/DeleteLinuxUser: error paths
// ---------------------------------------------------------------------------

func TestCreateLinuxUser_ErrorPath(t *testing.T) {
	err := CreateLinuxUser("test-create-project-xyz", "/tmp/nonexistent")
	if err == nil {
		t.Skip("useradd succeeded (running as root?)")
	}
	// Error should either say user exists or be about creating user.
	errMsg := err.Error()
	if !strings.Contains(errMsg, "already exists") && !strings.Contains(errMsg, "creating user") {
		t.Errorf("error should mention user creation, got: %v", err)
	}
}

func TestDeleteLinuxUser_NonexistentUser(t *testing.T) {
	// Deleting a user that doesn't exist should return nil.
	err := DeleteLinuxUser("absolutely-nonexistent-user-xyz123abc")
	if err != nil {
		t.Errorf("deleting nonexistent user should return nil, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// SetupAuthorizedKeys: various key formats
// ---------------------------------------------------------------------------

func TestSetupAuthorizedKeys_RSAKey(t *testing.T) {
	tmpDir := t.TempDir()
	rsaKey := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC+example user@host"

	if err := SetupAuthorizedKeys(tmpDir, rsaKey); err != nil {
		t.Fatalf("SetupAuthorizedKeys: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, ".ssh", "authorized_keys"))
	if err != nil {
		t.Fatalf("reading authorized_keys: %v", err)
	}
	if !strings.Contains(string(data), rsaKey) {
		t.Error("authorized_keys should contain the RSA key")
	}
	if !strings.HasPrefix(string(data), "restrict,command=") {
		t.Error("should have restrict prefix")
	}
}

// ---------------------------------------------------------------------------
// ScaffoldFromProfile: profile with Nginx config
// ---------------------------------------------------------------------------

func TestScaffoldFromProfile_NginxConfigPermissions(t *testing.T) {
	projectPath := t.TempDir()
	profile := &profiles.Profile{
		Name:    "static",
		Compose: "services:\n  nginx:\n    image: nginx:alpine\n",
		Nginx:   "server { listen 80; }\n",
	}
	tmpl := newMinimalTemplate()
	profileData := profiles.ProfileData{Name: "site", Domain: "site.example.com"}
	tmplData := templates.TemplateData{Name: "site", Domain: "site.example.com"}

	if err := ScaffoldFromProfile(projectPath, profile, tmpl, profileData, tmplData); err != nil {
		t.Fatalf("ScaffoldFromProfile: %v", err)
	}

	info, err := os.Stat(filepath.Join(projectPath, "nginx.conf"))
	if err != nil {
		t.Fatalf("nginx.conf should exist: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0644 {
		t.Errorf("nginx.conf permissions = %o, want 0644", perm)
	}

	// Verify public directory was created for "static" profile.
	info, err = os.Stat(filepath.Join(projectPath, "public"))
	if err != nil {
		t.Fatalf("public/ should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("public should be a directory")
	}

	// Verify index.html exists.
	indexData, err := os.ReadFile(filepath.Join(projectPath, "public", "index.html"))
	if err != nil {
		t.Fatalf("index.html should exist: %v", err)
	}
	if !strings.Contains(string(indexData), "site") {
		t.Error("index.html should contain the project name")
	}
}

// ---------------------------------------------------------------------------
// GenerateEnvFile: writing path construction
// ---------------------------------------------------------------------------

func TestGenerateEnvFile_PathConstruction(t *testing.T) {
	// Verify .env is written in projectPath/.env
	tmpDir := t.TempDir()
	tmpl := &templates.Template{
		EnvTemplate: "KEY=VALUE\n",
	}
	data := templates.TemplateData{Name: "test", Domain: "test.dev"}

	if err := GenerateEnvFile(tmpDir, tmpl, data); err != nil {
		t.Fatalf("GenerateEnvFile: %v", err)
	}

	envPath := filepath.Join(tmpDir, ".env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		t.Error(".env should exist at projectPath/.env")
	}
}

// ---------------------------------------------------------------------------
// validNameRe: verify the regex pattern directly
// ---------------------------------------------------------------------------

func TestValidNameRe_Patterns(t *testing.T) {
	tests := []struct {
		name    string
		matches bool
	}{
		{"a", true},
		{"z", true},
		{"0", true},
		{"9", true},
		{"ab", true},
		{"a-b", true},
		{"abc-def", true},
		{"-a", false},
		{"a-", false},
		{"-", false},
		{"A", false},
		{"a.b", false},
		{"a_b", false},
	}
	for _, tt := range tests {
		got := validNameRe.MatchString(tt.name)
		if got != tt.matches {
			t.Errorf("validNameRe.MatchString(%q) = %v, want %v", tt.name, got, tt.matches)
		}
	}
}

// ---------------------------------------------------------------------------
// ScaffoldFromProfile: comprehensive file verification
// ---------------------------------------------------------------------------

func TestScaffoldFromProfile_AllFilesWritten(t *testing.T) {
	projectPath := t.TempDir()
	profile := &profiles.Profile{
		Name:        "server",
		Description: "Server profile",
		Compose:     "services:\n  app:\n    build: .\n    container_name: {{.Name}}-app\n",
		EnvTemplate: "APP={{.Name}}\n",
	}
	tmpl := &templates.Template{
		Name:       "go",
		Dockerfile: "FROM golang:1.22\nWORKDIR /app\nLABEL name={{.Name}}\n",
		Compose:    "services: {}",
		Workflow:   "name: Deploy\non:\n  push:\n    branches: [main]\n",
		GitIgnore:  ".env\nbin/\n",
	}
	profileData := profiles.ProfileData{Name: "webapp", Domain: "webapp.example.com"}
	tmplData := templates.TemplateData{Name: "webapp", Domain: "webapp.example.com"}

	if err := ScaffoldFromProfile(projectPath, profile, tmpl, profileData, tmplData); err != nil {
		t.Fatalf("ScaffoldFromProfile: %v", err)
	}

	// Dockerfile should come from the template, rendered with tmplData.
	df, err := os.ReadFile(filepath.Join(projectPath, "Dockerfile"))
	if err != nil {
		t.Fatalf("reading Dockerfile: %v", err)
	}
	if !strings.Contains(string(df), "LABEL name=webapp") {
		t.Error("Dockerfile should contain rendered template data")
	}

	// docker-compose.yml should come from the PROFILE, rendered with profileData.
	dc, err := os.ReadFile(filepath.Join(projectPath, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("reading docker-compose.yml: %v", err)
	}
	if !strings.Contains(string(dc), "webapp-app") {
		t.Error("docker-compose.yml should contain rendered profile data")
	}

	// Workflow should be written from template.
	wf, err := os.ReadFile(filepath.Join(projectPath, ".github", "workflows", "deploy.yml"))
	if err != nil {
		t.Fatalf("reading deploy.yml: %v", err)
	}
	if !strings.Contains(string(wf), "name: Deploy") {
		t.Error("deploy.yml should contain workflow content")
	}

	// .gitignore should be written from template.
	gi, err := os.ReadFile(filepath.Join(projectPath, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if !strings.Contains(string(gi), "bin/") {
		t.Error(".gitignore should contain 'bin/'")
	}

	// Directories should exist.
	for _, d := range []string{".github/workflows", "deployments"} {
		info, err := os.Stat(filepath.Join(projectPath, d))
		if err != nil {
			t.Errorf("directory %s should exist: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s should be a directory", d)
		}
	}

	// nginx.conf should NOT exist (no Nginx field in profile).
	if _, err := os.Stat(filepath.Join(projectPath, "nginx.conf")); !os.IsNotExist(err) {
		t.Error("nginx.conf should not exist for non-static profile")
	}

	// public/ should NOT exist (not a static profile).
	if _, err := os.Stat(filepath.Join(projectPath, "public")); !os.IsNotExist(err) {
		t.Error("public/ should not exist for non-static profile")
	}
}

// ---------------------------------------------------------------------------
// ScaffoldFromProfile: idempotent (second call overwrites)
// ---------------------------------------------------------------------------

func TestScaffoldFromProfile_Idempotent(t *testing.T) {
	projectPath := t.TempDir()
	profile := newMinimalProfile("server")
	tmpl := newMinimalTemplate()
	profileData := profiles.ProfileData{Name: "app1", Domain: "app1.dev"}
	tmplData := templates.TemplateData{Name: "app1", Domain: "app1.dev"}

	if err := ScaffoldFromProfile(projectPath, profile, tmpl, profileData, tmplData); err != nil {
		t.Fatalf("first ScaffoldFromProfile: %v", err)
	}

	// Second call with different data should overwrite.
	profileData2 := profiles.ProfileData{Name: "app2", Domain: "app2.dev"}
	tmplData2 := templates.TemplateData{Name: "app2", Domain: "app2.dev"}
	if err := ScaffoldFromProfile(projectPath, profile, tmpl, profileData2, tmplData2); err != nil {
		t.Fatalf("second ScaffoldFromProfile: %v", err)
	}

	// Verify files contain app2 data, not app1.
	dc, err := os.ReadFile(filepath.Join(projectPath, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("reading docker-compose.yml: %v", err)
	}
	if strings.Contains(string(dc), "app1-app") {
		t.Error("docker-compose.yml should not contain old data after overwrite")
	}
	if !strings.Contains(string(dc), "app2-app") {
		t.Error("docker-compose.yml should contain new data after overwrite")
	}
}

// ---------------------------------------------------------------------------
// GenerateEnvFromProfile: rendering with Domain field
// ---------------------------------------------------------------------------

func TestGenerateEnvFromProfile_RendersDomain(t *testing.T) {
	tmpDir := t.TempDir()
	profile := &profiles.Profile{
		Name:        "server",
		EnvTemplate: "APP={{.Name}}\nDOMAIN={{.Domain}}\n",
	}
	data := profiles.ProfileData{Name: "mysite", Domain: "mysite.example.com"}

	if err := GenerateEnvFromProfile(tmpDir, profile, data); err != nil {
		t.Fatalf("GenerateEnvFromProfile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".env"))
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, "APP=mysite") {
		t.Error("should contain APP=mysite")
	}
	if !strings.Contains(s, "DOMAIN=mysite.example.com") {
		t.Error("should contain DOMAIN=mysite.example.com")
	}
}

// ---------------------------------------------------------------------------
// ComposeUp/Down/Restart: verify that error includes stderr output
// ---------------------------------------------------------------------------

func TestComposeUp_ErrorIncludesOutput(t *testing.T) {
	err := ComposeUp("/nonexistent/xyz123")
	if err == nil {
		t.Skip("docker compose is available")
	}
	// The error format is "docker compose up: <output>: <err>"
	errMsg := err.Error()
	if !strings.HasPrefix(errMsg, "docker compose up:") {
		t.Errorf("error should start with 'docker compose up:', got: %v", errMsg)
	}
}

func TestComposeDown_ErrorIncludesOutput(t *testing.T) {
	err := ComposeDown("/nonexistent/xyz123")
	if err == nil {
		t.Skip("docker compose is available")
	}
	errMsg := err.Error()
	if !strings.HasPrefix(errMsg, "docker compose down:") {
		t.Errorf("error should start with 'docker compose down:', got: %v", errMsg)
	}
}

func TestComposeRestart_ErrorIncludesOutput(t *testing.T) {
	err := ComposeRestart("/nonexistent/xyz123")
	if err == nil {
		t.Skip("docker compose is available")
	}
	errMsg := err.Error()
	if !strings.HasPrefix(errMsg, "docker compose restart:") {
		t.Errorf("error should start with 'docker compose restart:', got: %v", errMsg)
	}
}

// ---------------------------------------------------------------------------
// ScaffoldFromProfile: env template with CHANGEME and shell pattern
// ---------------------------------------------------------------------------

func TestScaffoldFromProfile_EnvWithShellPattern(t *testing.T) {
	projectPath := t.TempDir()
	profile := &profiles.Profile{
		Name:        "server",
		Compose:     "services: {}\n",
		EnvTemplate: "DB=CHANGEME_$(openssl rand -hex 16)\nAPP={{.Name}}\n",
	}
	tmpl := newMinimalTemplate()
	profileData := profiles.ProfileData{Name: "myapp", Domain: "myapp.dev"}
	tmplData := templates.TemplateData{Name: "myapp", Domain: "myapp.dev"}

	if err := ScaffoldFromProfile(projectPath, profile, tmpl, profileData, tmplData); err != nil {
		t.Fatalf("ScaffoldFromProfile: %v", err)
	}

	// Now generate env file separately via GenerateEnvFromProfile.
	if err := GenerateEnvFromProfile(projectPath, profile, profileData); err != nil {
		t.Fatalf("GenerateEnvFromProfile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(projectPath, ".env"))
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}
	s := string(content)
	if strings.Contains(s, "CHANGEME") {
		t.Error("CHANGEME should be replaced")
	}
	if strings.Contains(s, "$(openssl") {
		t.Error("shell command pattern should be consumed")
	}
	if !strings.Contains(s, "APP=myapp") {
		t.Error("template rendering should work")
	}
}

// ---------------------------------------------------------------------------
// GenerateSSHKeypair: verify key written to correct subdirectory
// ---------------------------------------------------------------------------

func TestGenerateSSHKeypair_SubdirCreation(t *testing.T) {
	tmpDir := t.TempDir()
	// Ensure .ssh doesn't exist yet.
	sshDir := filepath.Join(tmpDir, ".ssh")
	if _, err := os.Stat(sshDir); !os.IsNotExist(err) {
		t.Fatal(".ssh should not exist before test")
	}

	_, _, err := GenerateSSHKeypair(tmpDir)
	if err != nil {
		t.Fatalf("GenerateSSHKeypair: %v", err)
	}

	// Verify .ssh was created.
	info, err := os.Stat(sshDir)
	if err != nil {
		t.Fatalf(".ssh should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error(".ssh should be a directory")
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf(".ssh permissions = %o, want 0700", perm)
	}
}

// ---------------------------------------------------------------------------
// ScaffoldProject: additional error paths and content verification
// ---------------------------------------------------------------------------

func TestScaffoldProject_WorkflowWritten(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "proj")
	os.MkdirAll(projectPath, 0755)

	tmpl := &templates.Template{
		Dockerfile: "FROM alpine",
		Compose:    "services: {}",
		Workflow:   "name: Custom-CI\non: [push]\njobs:\n  build:\n    runs-on: ubuntu-latest\n",
		GitIgnore:  ".env\n",
	}
	data := templates.TemplateData{Name: "proj", Domain: "proj.dev"}

	if err := ScaffoldProject(projectPath, tmpl, data); err != nil {
		t.Fatalf("ScaffoldProject: %v", err)
	}

	wf, err := os.ReadFile(filepath.Join(projectPath, ".github", "workflows", "deploy.yml"))
	if err != nil {
		t.Fatalf("reading deploy.yml: %v", err)
	}
	if !strings.Contains(string(wf), "Custom-CI") {
		t.Error("deploy.yml should contain the custom workflow name")
	}
}

func TestScaffoldProject_DeploymentsDir(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "proj")
	os.MkdirAll(projectPath, 0755)

	tmpl := &templates.Template{
		Dockerfile: "FROM scratch",
		Compose:    "services: {}",
		Workflow:   "name: CI",
		GitIgnore:  ".env\n",
	}
	data := templates.TemplateData{Name: "proj", Domain: "proj.dev"}

	if err := ScaffoldProject(projectPath, tmpl, data); err != nil {
		t.Fatalf("ScaffoldProject: %v", err)
	}

	info, err := os.Stat(filepath.Join(projectPath, "deployments"))
	if err != nil {
		t.Fatalf("deployments dir should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("deployments should be a directory")
	}
}

// ---------------------------------------------------------------------------
// replacePasswordPlaceholders: more edge cases for the shell pattern branch
// ---------------------------------------------------------------------------

func TestReplacePasswordPlaceholders_CHANGEMEFollowedByUnderscore_NoCloseParen(t *testing.T) {
	// CHANGEME_ but no closing parenthesis - should just replace CHANGEME and
	// leave the rest (the underscore doesn't match the shell pattern fully).
	input := "PASS=CHANGEME_noparen\n"
	result := replacePasswordPlaceholders(input, "app")
	if strings.Contains(result, "CHANGEME") {
		t.Error("CHANGEME should be replaced")
	}
	// The underscore triggers the shell pattern branch. Since there is no ")",
	// the function should NOT consume the rest. Check that something sensible happened.
	// Actually looking at the code, indexOf(after, ")") returns -1, so the branch
	// doesn't consume anything - the underscore and "noparen" remain.
	if !strings.Contains(result, "_noparen") {
		t.Error("text after CHANGEME_ without ) should be preserved")
	}
}

func TestReplacePasswordPlaceholders_CHANGEMEFollowedByUnderscoreAndParen(t *testing.T) {
	input := "PASS=CHANGEME_$(some_cmd)\nOTHER=value\n"
	result := replacePasswordPlaceholders(input, "app")
	if strings.Contains(result, "CHANGEME") {
		t.Error("CHANGEME should be replaced")
	}
	if strings.Contains(result, "$(some_cmd)") {
		t.Error("shell command should be consumed")
	}
	if !strings.Contains(result, "OTHER=value") {
		t.Error("other values should be preserved")
	}
}

func TestReplacePasswordPlaceholders_MultipleCHANGEMEMixedStyles(t *testing.T) {
	input := "A=CHANGEME\nB=CHANGEME_$(cmd arg)\nC=CHANGEME\n"
	result := replacePasswordPlaceholders(input, "app")
	if strings.Contains(result, "CHANGEME") {
		t.Error("all CHANGEME should be replaced")
	}
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %v", len(lines), lines)
	}
}

// ---------------------------------------------------------------------------
// GenerateEnvFile: bad template (rendering error path)
// ---------------------------------------------------------------------------

func TestGenerateEnvFile_BadTemplateErrorWrapping(t *testing.T) {
	tmpDir := t.TempDir()
	tmpl := &templates.Template{
		EnvTemplate: "{{.Missing}}",
	}
	data := templates.TemplateData{Name: "x", Domain: "x.dev"}

	err := GenerateEnvFile(tmpDir, tmpl, data)
	if err == nil {
		t.Fatal("expected error for bad template")
	}
	if !strings.Contains(err.Error(), "rendering env template") {
		t.Errorf("error should wrap with 'rendering env template', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ScaffoldFromProfile: static profile creates public dir and index.html
// ---------------------------------------------------------------------------

func TestScaffoldFromProfile_StaticProfileIndexHTMLContent(t *testing.T) {
	projectPath := t.TempDir()
	profile := &profiles.Profile{
		Name:    "static",
		Compose: "services: {}\n",
	}
	tmpl := newMinimalTemplate()
	profileData := profiles.ProfileData{Name: "my-site", Domain: "my-site.example.com"}
	tmplData := templates.TemplateData{Name: "my-site", Domain: "my-site.example.com"}

	if err := ScaffoldFromProfile(projectPath, profile, tmpl, profileData, tmplData); err != nil {
		t.Fatalf("ScaffoldFromProfile: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(projectPath, "public", "index.html"))
	if err != nil {
		t.Fatalf("reading index.html: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "<title>my-site</title>") {
		t.Error("index.html should contain project name in title")
	}
	if !strings.Contains(content, "<h1>Welcome to my-site</h1>") {
		t.Error("index.html should contain project name in h1")
	}
	if !strings.Contains(content, "FleetDeck") {
		t.Error("index.html should mention FleetDeck")
	}
	if !strings.Contains(content, "<!DOCTYPE html>") {
		t.Error("index.html should have DOCTYPE")
	}
}

// ---------------------------------------------------------------------------
// ValidateName: regex vs consecutive hyphens interaction
// ---------------------------------------------------------------------------

func TestValidateName_RegexPassesButConsecutiveHyphensFails(t *testing.T) {
	// Names that pass the regex but fail the consecutive hyphens check.
	names := []string{"a--b", "x--y--z", "abc--def"}
	for _, name := range names {
		err := ValidateName(name)
		if err == nil {
			t.Errorf("ValidateName(%q) should fail for consecutive hyphens", name)
		}
		if !strings.Contains(err.Error(), "consecutive hyphens") {
			t.Errorf("error for %q should mention 'consecutive hyphens', got: %v", name, err)
		}
	}
}

// ---------------------------------------------------------------------------
// LinuxUserName: verify it's a pure function (no side effects)
// ---------------------------------------------------------------------------

func TestLinuxUserName_PureFunction(t *testing.T) {
	// Same input should always produce same output.
	for i := 0; i < 10; i++ {
		got := LinuxUserName("test")
		if got != "fleetdeck-test" {
			t.Errorf("iteration %d: got %q", i, got)
		}
	}
}

// ---------------------------------------------------------------------------
// SetupAuthorizedKeys: key with special characters
// ---------------------------------------------------------------------------

func TestSetupAuthorizedKeys_KeyWithComment(t *testing.T) {
	tmpDir := t.TempDir()
	key := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample user@host.example.com # deploy key"

	if err := SetupAuthorizedKeys(tmpDir, key); err != nil {
		t.Fatalf("SetupAuthorizedKeys: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, ".ssh", "authorized_keys"))
	if err != nil {
		t.Fatalf("reading authorized_keys: %v", err)
	}
	if !strings.Contains(string(data), key) {
		t.Error("authorized_keys should contain the full key including comment")
	}
}

// ---------------------------------------------------------------------------
// GenerateEnvFromProfile: verify CHANGEME in key names (not values) is left alone
// ---------------------------------------------------------------------------

func TestGenerateEnvFromProfile_CHANGEMEInKeyName(t *testing.T) {
	tmpDir := t.TempDir()
	profile := &profiles.Profile{
		Name:        "server",
		EnvTemplate: "CHANGEME_KEY=static_value\nNORMAL=CHANGEME\n",
	}
	data := profiles.ProfileData{Name: "test"}

	if err := GenerateEnvFromProfile(tmpDir, profile, data); err != nil {
		t.Fatalf("GenerateEnvFromProfile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".env"))
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}
	s := string(content)
	// All CHANGEME occurrences should be replaced (both in key and value position).
	if strings.Contains(s, "CHANGEME") {
		t.Error("all CHANGEME should be replaced, even in key names")
	}
}

// ---------------------------------------------------------------------------
// GenerateSecret: verify hex output is lowercase
// ---------------------------------------------------------------------------

func TestGenerateSecret_LowercaseHex(t *testing.T) {
	for i := 0; i < 20; i++ {
		s := GenerateSecret(16)
		if s != strings.ToLower(s) {
			t.Errorf("GenerateSecret should produce lowercase hex, got: %q", s)
		}
	}
}

// ---------------------------------------------------------------------------
// InitAndPushRepo: exercise git init + git add + git commit sequence
// ---------------------------------------------------------------------------

func TestInitAndPushRepo_ProgressesThroughGitCommands(t *testing.T) {
	// Create a temp dir with a file so git add/commit can proceed.
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Test\n"), 0644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	err := InitAndPushRepo(tmpDir, "https://github.com/fake/nonexistent-repo.git")
	if err == nil {
		t.Fatal("expected error (push to fake remote should fail)")
	}
	// The error should come from one of the later git commands (push), not init.
	errMsg := err.Error()
	if !strings.Contains(errMsg, "running git") {
		t.Errorf("error should mention 'running git', got: %v", err)
	}

	// Verify git init succeeded by checking .git directory exists.
	if _, err := os.Stat(filepath.Join(tmpDir, ".git")); os.IsNotExist(err) {
		t.Error(".git directory should exist after InitAndPushRepo (even if push failed)")
	}
}

// ---------------------------------------------------------------------------
// GetServerIP: verify format on Linux
// ---------------------------------------------------------------------------

func TestGetServerIP_Format(t *testing.T) {
	ip, err := GetServerIP()
	if err != nil {
		t.Skip("hostname -I not available: " + err.Error())
	}
	// Should be a single IP (no spaces).
	if strings.Contains(ip, " ") {
		t.Errorf("should return single IP, got: %q", ip)
	}
	// Should not be empty.
	if ip == "" {
		t.Error("should not return empty string")
	}
	// Should not contain newlines.
	if strings.Contains(ip, "\n") {
		t.Errorf("should not contain newlines, got: %q", ip)
	}
}

// ---------------------------------------------------------------------------
// ScaffoldFromProfile: env generation with port field
// ---------------------------------------------------------------------------

func TestScaffoldFromProfile_ProfileDataPort(t *testing.T) {
	projectPath := t.TempDir()
	profile := &profiles.Profile{
		Name:        "server",
		Compose:     "services:\n  app:\n    ports:\n      - \"{{.Port}}:{{.Port}}\"\n",
		EnvTemplate: "PORT={{.Port}}\n",
	}
	tmpl := newMinimalTemplate()
	profileData := profiles.ProfileData{Name: "testapp", Domain: "testapp.dev", Port: 8080}
	tmplData := templates.TemplateData{Name: "testapp", Domain: "testapp.dev"}

	if err := ScaffoldFromProfile(projectPath, profile, tmpl, profileData, tmplData); err != nil {
		t.Fatalf("ScaffoldFromProfile: %v", err)
	}

	dc, err := os.ReadFile(filepath.Join(projectPath, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("reading docker-compose.yml: %v", err)
	}
	if !strings.Contains(string(dc), "8080:8080") {
		t.Error("docker-compose.yml should contain rendered port")
	}
}

// ---------------------------------------------------------------------------
// GenerateEnvFromProfile: env with port rendering
// ---------------------------------------------------------------------------

func TestGenerateEnvFromProfile_WithPort(t *testing.T) {
	tmpDir := t.TempDir()
	profile := &profiles.Profile{
		Name:        "server",
		EnvTemplate: "PORT={{.Port}}\nNAME={{.Name}}\n",
	}
	data := profiles.ProfileData{Name: "myapp", Domain: "myapp.dev", Port: 3000}

	if err := GenerateEnvFromProfile(tmpDir, profile, data); err != nil {
		t.Fatalf("GenerateEnvFromProfile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".env"))
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, "PORT=3000") {
		t.Error("should contain PORT=3000")
	}
	if !strings.Contains(s, "NAME=myapp") {
		t.Error("should contain NAME=myapp")
	}
}

// ---------------------------------------------------------------------------
// ScaffoldProject: gitignore content preserved exactly
// ---------------------------------------------------------------------------

func TestScaffoldProject_GitignoreExact(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "proj")
	os.MkdirAll(projectPath, 0755)

	gitignoreContent := ".env\n*.log\nnode_modules/\ndist/\n.DS_Store\n"
	tmpl := &templates.Template{
		Dockerfile: "FROM scratch",
		Compose:    "services: {}",
		Workflow:   "name: CI",
		GitIgnore:  gitignoreContent,
	}
	data := templates.TemplateData{Name: "p", Domain: "p.dev"}

	if err := ScaffoldProject(projectPath, tmpl, data); err != nil {
		t.Fatalf("ScaffoldProject: %v", err)
	}

	gi, err := os.ReadFile(filepath.Join(projectPath, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if string(gi) != gitignoreContent {
		t.Errorf(".gitignore content mismatch:\ngot:  %q\nwant: %q", string(gi), gitignoreContent)
	}
}

// ---------------------------------------------------------------------------
// ScaffoldProject: verify both template variables in Dockerfile
// ---------------------------------------------------------------------------

func TestScaffoldProject_DockerfileBothVars(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "proj")
	os.MkdirAll(projectPath, 0755)

	tmpl := &templates.Template{
		Dockerfile: "FROM node:20\nENV APP_NAME={{.Name}}\nENV APP_DOMAIN={{.Domain}}\n",
		Compose:    "services: {}",
		Workflow:   "name: CI",
		GitIgnore:  ".env\n",
	}
	data := templates.TemplateData{Name: "myapp", Domain: "myapp.example.com"}

	if err := ScaffoldProject(projectPath, tmpl, data); err != nil {
		t.Fatalf("ScaffoldProject: %v", err)
	}

	df, err := os.ReadFile(filepath.Join(projectPath, "Dockerfile"))
	if err != nil {
		t.Fatalf("reading Dockerfile: %v", err)
	}
	s := string(df)
	if !strings.Contains(s, "ENV APP_NAME=myapp") {
		t.Error("Dockerfile should contain rendered Name")
	}
	if !strings.Contains(s, "ENV APP_DOMAIN=myapp.example.com") {
		t.Error("Dockerfile should contain rendered Domain")
	}
}
