package project

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/templates"
)

func TestGenerateSecret(t *testing.T) {
	s1 := GenerateSecret(16)
	s2 := GenerateSecret(16)

	if len(s1) != 32 { // hex encoding doubles the length
		t.Errorf("expected 32 char hex string, got %d chars", len(s1))
	}
	if s1 == s2 {
		t.Error("two generated secrets should not be identical")
	}
}

func TestGenerateSecretLengths(t *testing.T) {
	tests := []struct {
		byteLen    int
		expectedHex int
	}{
		{8, 16},
		{16, 32},
		{32, 64},
	}

	for _, tt := range tests {
		s := GenerateSecret(tt.byteLen)
		if len(s) != tt.expectedHex {
			t.Errorf("GenerateSecret(%d): expected %d hex chars, got %d", tt.byteLen, tt.expectedHex, len(s))
		}
	}
}

func TestValidateName(t *testing.T) {
	valid := []string{"myapp", "a", "test-app", "app123", "a1b2c3"}
	for _, name := range valid {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) should be valid, got error: %v", name, err)
		}
	}

	invalid := []string{
		"",                                                                  // empty
		"-myapp",                                                            // starts with hyphen
		"myapp-",                                                            // ends with hyphen
		"my--app",                                                           // consecutive hyphens
		"My App",                                                            // spaces and uppercase
		"my_app",                                                            // underscore
		"my.app",                                                            // dot
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",    // 64 chars — too long
		"../etc/passwd",                                                     // path traversal
	}
	for _, name := range invalid {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) should be invalid, got nil error", name)
		}
	}
}

func TestLinuxUserName(t *testing.T) {
	tests := []struct {
		project  string
		expected string
	}{
		{"myapp", "fleetdeck-myapp"},
		{"test-project", "fleetdeck-test-project"},
		{"a", "fleetdeck-a"},
	}

	for _, tt := range tests {
		got := LinuxUserName(tt.project)
		if got != tt.expected {
			t.Errorf("LinuxUserName(%q) = %q, want %q", tt.project, got, tt.expected)
		}
	}
}

func TestGenerateSecretHexOnly(t *testing.T) {
	// Verify the output contains only valid hex characters.
	for i := 0; i < 10; i++ {
		s := GenerateSecret(16)
		if _, err := hex.DecodeString(s); err != nil {
			t.Errorf("GenerateSecret(16) = %q is not valid hex: %v", s, err)
		}
	}
}

func TestGenerateSecretZeroLength(t *testing.T) {
	s := GenerateSecret(0)
	if s != "" {
		t.Errorf("GenerateSecret(0) should return empty string, got %q", s)
	}
}

func TestGenerateSecretLargeLength(t *testing.T) {
	s := GenerateSecret(128)
	if len(s) != 256 {
		t.Errorf("GenerateSecret(128) should return 256 hex chars, got %d", len(s))
	}
	if _, err := hex.DecodeString(s); err != nil {
		t.Errorf("GenerateSecret(128) is not valid hex: %v", err)
	}
}

func TestGenerateEnvFile(t *testing.T) {
	tmpDir := t.TempDir()

	tmpl := &templates.Template{
		EnvTemplate: `APP_NAME={{.Name}}
APP_DOMAIN={{.Domain}}
SECRET_KEY=placeholder
`,
	}
	data := templates.TemplateData{
		Name:   "testapp",
		Domain: "testapp.example.com",
	}

	if err := GenerateEnvFile(tmpDir, tmpl, data); err != nil {
		t.Fatalf("GenerateEnvFile() error: %v", err)
	}

	envPath := filepath.Join(tmpDir, ".env")
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading .env file: %v", err)
	}

	// Template variables should be rendered
	if !strings.Contains(string(content), "APP_NAME=testapp") {
		t.Error(".env should contain rendered APP_NAME=testapp")
	}
	if !strings.Contains(string(content), "APP_DOMAIN=testapp.example.com") {
		t.Error(".env should contain rendered APP_DOMAIN=testapp.example.com")
	}
}

func TestGenerateEnvFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()

	tmpl := &templates.Template{
		EnvTemplate: `APP_NAME={{.Name}}
`,
	}
	data := templates.TemplateData{
		Name:   "testapp",
		Domain: "testapp.example.com",
	}

	if err := GenerateEnvFile(tmpDir, tmpl, data); err != nil {
		t.Fatalf("GenerateEnvFile() error: %v", err)
	}

	envPath := filepath.Join(tmpDir, ".env")
	info, err := os.Stat(envPath)
	if err != nil {
		t.Fatalf("stat .env: %v", err)
	}

	// .env files should have restrictive permissions (0600)
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf(".env permissions = %o, want 0600", perm)
	}
}

func TestGenerateEnvFileSecretReplacement(t *testing.T) {
	tmpDir := t.TempDir()

	// The function replaces occurrences of "<Name>_CHANGEME" with generated secrets.
	tmpl := &templates.Template{
		EnvTemplate: `DB_PASSWORD={{.Name}}_CHANGEME
REDIS_PASSWORD=static-value
API_KEY={{.Name}}_CHANGEME
`,
	}
	data := templates.TemplateData{
		Name:   "myapp",
		Domain: "myapp.example.com",
	}

	if err := GenerateEnvFile(tmpDir, tmpl, data); err != nil {
		t.Fatalf("GenerateEnvFile() error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".env"))
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}

	// The placeholder should be replaced
	if strings.Contains(string(content), "myapp_CHANGEME") {
		t.Error(".env should not contain 'myapp_CHANGEME' placeholder after generation")
	}

	// Static values should remain
	if !strings.Contains(string(content), "REDIS_PASSWORD=static-value") {
		t.Error(".env should preserve static values like 'REDIS_PASSWORD=static-value'")
	}

	// All replaced secrets should be valid hex
	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(line, "DB_PASSWORD=") {
			val := strings.TrimPrefix(line, "DB_PASSWORD=")
			if _, err := hex.DecodeString(val); err != nil {
				t.Errorf("DB_PASSWORD value %q is not valid hex: %v", val, err)
			}
			if len(val) != 32 { // GenerateSecret(16) produces 32 hex chars
				t.Errorf("DB_PASSWORD value length = %d, want 32", len(val))
			}
		}
	}
}

func TestGenerateEnvFileBadTemplate(t *testing.T) {
	tmpDir := t.TempDir()

	tmpl := &templates.Template{
		EnvTemplate: `APP={{.NonexistentField}}`,
	}
	data := templates.TemplateData{
		Name:   "myapp",
		Domain: "myapp.example.com",
	}

	err := GenerateEnvFile(tmpDir, tmpl, data)
	if err == nil {
		t.Fatal("GenerateEnvFile with bad template should return error")
	}
	if !strings.Contains(err.Error(), "rendering env template") {
		t.Errorf("error should mention 'rendering env template', got: %v", err)
	}
}

func TestGenerateEnvFileInvalidPath(t *testing.T) {
	tmpl := &templates.Template{
		EnvTemplate: `APP_NAME={{.Name}}`,
	}
	data := templates.TemplateData{
		Name:   "myapp",
		Domain: "myapp.example.com",
	}

	err := GenerateEnvFile("/nonexistent/path/that/does/not/exist", tmpl, data)
	if err == nil {
		t.Fatal("GenerateEnvFile with invalid path should return error")
	}
}

func TestValidateNameBoundaryLengths(t *testing.T) {
	// Test exact boundary: 1 char (minimum valid)
	if err := ValidateName("a"); err != nil {
		t.Errorf("ValidateName('a') single char should be valid: %v", err)
	}

	// Test exact boundary: 63 chars (maximum valid)
	name63 := strings.Repeat("a", 63)
	if err := ValidateName(name63); err != nil {
		t.Errorf("ValidateName(63 chars) should be valid: %v", err)
	}

	// Test exact boundary: 64 chars (one over max)
	name64 := strings.Repeat("a", 64)
	if err := ValidateName(name64); err == nil {
		t.Error("ValidateName(64 chars) should be invalid")
	}

	// Test 62 chars (one under max)
	name62 := strings.Repeat("a", 62)
	if err := ValidateName(name62); err != nil {
		t.Errorf("ValidateName(62 chars) should be valid: %v", err)
	}

	// Test 2 chars
	if err := ValidateName("ab"); err != nil {
		t.Errorf("ValidateName('ab') should be valid: %v", err)
	}
}

func TestValidateNameSpecialCharacters(t *testing.T) {
	specialChars := []string{
		"app!name",
		"app@name",
		"app#name",
		"app$name",
		"app%name",
		"app^name",
		"app&name",
		"app*name",
		"app(name",
		"app)name",
		"app+name",
		"app=name",
		"app{name",
		"app}name",
		"app[name",
		"app]name",
		"app|name",
		"app:name",
		"app;name",
		"app'name",
		"app\"name",
		"app<name",
		"app>name",
		"app,name",
		"app?name",
		"app`name",
		"app~name",
		"app name",
		"app\tname",
		"app\nname",
	}

	for _, name := range specialChars {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) with special char should be invalid", name)
		}
	}
}

func TestValidateNameDoubleHyphens(t *testing.T) {
	tests := []struct {
		name    string
		valid   bool
	}{
		{"a-b", true},
		{"a-b-c", true},
		{"a--b", false},
		{"a---b", false},
		{"--ab", false},
		{"ab--", false},
		{"a-b--c", false},
		{"a--b-c", false},
		{"a-b-c-d", true},
	}

	for _, tt := range tests {
		err := ValidateName(tt.name)
		if tt.valid && err != nil {
			t.Errorf("ValidateName(%q) should be valid, got: %v", tt.name, err)
		}
		if !tt.valid && err == nil {
			t.Errorf("ValidateName(%q) should be invalid", tt.name)
		}
	}
}

func TestValidateNameHyphenPosition(t *testing.T) {
	// Single hyphen at start
	if err := ValidateName("-a"); err == nil {
		t.Error("ValidateName('-a') starting with hyphen should be invalid")
	}

	// Single hyphen at end
	if err := ValidateName("a-"); err == nil {
		t.Error("ValidateName('a-') ending with hyphen should be invalid")
	}

	// Hyphen only
	if err := ValidateName("-"); err == nil {
		t.Error("ValidateName('-') should be invalid")
	}

	// Multiple hyphens only
	if err := ValidateName("---"); err == nil {
		t.Error("ValidateName('---') should be invalid")
	}

	// Hyphen in middle is fine
	if err := ValidateName("a-b"); err != nil {
		t.Errorf("ValidateName('a-b') should be valid: %v", err)
	}
}

func TestValidateNameUnicode(t *testing.T) {
	unicodeNames := []string{
		"app-nàme",
		"appüber",
		"app-日本",
		"приложение",
		"app-名前",
	}

	for _, name := range unicodeNames {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) with unicode chars should be invalid", name)
		}
	}
}
