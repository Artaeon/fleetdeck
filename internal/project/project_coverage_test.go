package project

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/templates"
)

// ---------------------------------------------------------------------------
// ValidateName: additional boundary and regression tests
// ---------------------------------------------------------------------------

func TestValidateNameExactBoundary63(t *testing.T) {
	// 63 lowercase letters — exactly at the upper bound.
	name := strings.Repeat("b", 63)
	if err := ValidateName(name); err != nil {
		t.Errorf("63-char name should be valid: %v", err)
	}
}

func TestValidateNameExactBoundary64(t *testing.T) {
	// 64 lowercase letters — one over the upper bound.
	name := strings.Repeat("b", 64)
	if err := ValidateName(name); err == nil {
		t.Error("64-char name should be invalid")
	}
}

func TestValidateNameSingleCharEdgeCases(t *testing.T) {
	// Every single-digit and single-letter name should be valid.
	for _, ch := range "abcdefghijklmnopqrstuvwxyz0123456789" {
		if err := ValidateName(string(ch)); err != nil {
			t.Errorf("ValidateName(%q) should be valid: %v", string(ch), err)
		}
	}
}

func TestValidateNameTwoCharWithHyphen(t *testing.T) {
	// Two-char names that start or end with hyphen must be invalid.
	for _, n := range []string{"-a", "a-", "-1", "1-"} {
		if err := ValidateName(n); err == nil {
			t.Errorf("ValidateName(%q) should be invalid", n)
		}
	}
}

func TestValidateNameMaxWithHyphen(t *testing.T) {
	// 63 chars: starts/ends with letter, hyphen in the middle.
	name := "a" + strings.Repeat("-b", 31) // len = 1+62 = 63
	if err := ValidateName(name); err != nil {
		t.Errorf("63-char name with hyphens should be valid: %v", err)
	}
}

func TestValidateNameErrorMessageLength(t *testing.T) {
	// Verify the error message includes the actual length.
	err := ValidateName(strings.Repeat("z", 100))
	if err == nil {
		t.Fatal("expected error for 100-char name")
	}
	if !strings.Contains(err.Error(), "100") {
		t.Errorf("error should include actual length 100, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GenerateSecret: output length, hex validity, uniqueness
// ---------------------------------------------------------------------------

func TestGenerateSecretMultipleLengths(t *testing.T) {
	lengths := []int{1, 2, 4, 8, 16, 32, 64, 128, 256}
	for _, n := range lengths {
		s := GenerateSecret(n)
		expectedHexLen := n * 2
		if len(s) != expectedHexLen {
			t.Errorf("GenerateSecret(%d): got %d hex chars, want %d", n, len(s), expectedHexLen)
		}
		if _, err := hex.DecodeString(s); err != nil {
			t.Errorf("GenerateSecret(%d) = %q is not valid hex: %v", n, s, err)
		}
	}
}

func TestGenerateSecretUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		s := GenerateSecret(16)
		if seen[s] {
			t.Fatalf("GenerateSecret produced duplicate after %d iterations: %s", i, s)
		}
		seen[s] = true
	}
}

func TestGenerateSecretConcurrent(t *testing.T) {
	// Verify GenerateSecret is safe to call concurrently (crypto/rand is
	// goroutine-safe, but we verify no panics or data races occur).
	var wg sync.WaitGroup
	results := make([]string, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = GenerateSecret(16)
		}(i)
	}
	wg.Wait()

	seen := make(map[string]bool)
	for _, s := range results {
		if len(s) != 32 {
			t.Errorf("concurrent GenerateSecret: got %d hex chars, want 32", len(s))
		}
		if seen[s] {
			t.Error("concurrent GenerateSecret produced duplicate")
		}
		seen[s] = true
	}
}

// ---------------------------------------------------------------------------
// LinuxUserName: prefix and various inputs
// ---------------------------------------------------------------------------

func TestLinuxUserNameNumericInput(t *testing.T) {
	got := LinuxUserName("12345")
	if got != "fleetdeck-12345" {
		t.Errorf("LinuxUserName('12345') = %q, want 'fleetdeck-12345'", got)
	}
}

func TestLinuxUserNameHyphenatedInput(t *testing.T) {
	got := LinuxUserName("my-cool-app")
	want := "fleetdeck-my-cool-app"
	if got != want {
		t.Errorf("LinuxUserName('my-cool-app') = %q, want %q", got, want)
	}
}

func TestLinuxUserNameLongInput(t *testing.T) {
	long := strings.Repeat("a", 63)
	got := LinuxUserName(long)
	if !strings.HasPrefix(got, "fleetdeck-") {
		t.Error("long input should still have fleetdeck- prefix")
	}
	suffix := strings.TrimPrefix(got, "fleetdeck-")
	if suffix != long {
		t.Errorf("suffix mismatch: got len %d, want %d", len(suffix), len(long))
	}
}

func TestLinuxUserNameEmptyInput(t *testing.T) {
	// While ValidateName would reject "", LinuxUserName is a pure function
	// and should still return the prefix even for edge-case empty input.
	got := LinuxUserName("")
	if got != "fleetdeck-" {
		t.Errorf("LinuxUserName('') = %q, want 'fleetdeck-'", got)
	}
}

// ---------------------------------------------------------------------------
// GenerateEnvFile: file permissions, content rendering, secret replacement
// ---------------------------------------------------------------------------

func TestGenerateEnvFileRendersAllFields(t *testing.T) {
	tmpDir := t.TempDir()

	tmpl := &templates.Template{
		EnvTemplate: "NAME={{.Name}}\nDOMAIN={{.Domain}}\n",
	}
	data := templates.TemplateData{
		Name:   "alpha",
		Domain: "alpha.test.dev",
	}

	if err := GenerateEnvFile(tmpDir, tmpl, data); err != nil {
		t.Fatalf("GenerateEnvFile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".env"))
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, "NAME=alpha") {
		t.Error("expected NAME=alpha")
	}
	if !strings.Contains(s, "DOMAIN=alpha.test.dev") {
		t.Error("expected DOMAIN=alpha.test.dev")
	}
}

func TestGenerateEnvFilePermissionsAre0600(t *testing.T) {
	tmpDir := t.TempDir()
	tmpl := &templates.Template{EnvTemplate: "X=1\n"}
	data := templates.TemplateData{Name: "p", Domain: "p.dev"}

	if err := GenerateEnvFile(tmpDir, tmpl, data); err != nil {
		t.Fatalf("GenerateEnvFile: %v", err)
	}

	info, err := os.Stat(filepath.Join(tmpDir, ".env"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf(".env permissions = %o, want 0600", perm)
	}
}

func TestGenerateEnvFileMultipleSecretReplacements(t *testing.T) {
	tmpDir := t.TempDir()

	// GenerateEnvFile uses strings.ReplaceAll with a single GenerateSecret
	// call for the "<Name>_CHANGEME" pattern, so all occurrences of the same
	// placeholder get the SAME secret. Verify that behaviour.
	tmpl := &templates.Template{
		EnvTemplate: "A={{.Name}}_CHANGEME\nB={{.Name}}_CHANGEME\nC=static\n",
	}
	data := templates.TemplateData{Name: "proj", Domain: "proj.dev"}

	if err := GenerateEnvFile(tmpDir, tmpl, data); err != nil {
		t.Fatalf("GenerateEnvFile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".env"))
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}
	s := string(content)

	// No placeholders should remain.
	if strings.Contains(s, "CHANGEME") {
		t.Error(".env still contains CHANGEME placeholder")
	}
	// Static value must survive.
	if !strings.Contains(s, "C=static") {
		t.Error("static value C=static should be preserved")
	}
	// Both A and B should have valid 32-char hex secrets.
	for _, prefix := range []string{"A=", "B="} {
		for _, line := range strings.Split(s, "\n") {
			if strings.HasPrefix(line, prefix) {
				val := strings.TrimPrefix(line, prefix)
				if len(val) != 32 {
					t.Errorf("%s secret length = %d, want 32", prefix, len(val))
				}
				if _, err := hex.DecodeString(val); err != nil {
					t.Errorf("%s secret is not valid hex: %v", prefix, err)
				}
			}
		}
	}
}

func TestGenerateEnvFileEmptyTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	tmpl := &templates.Template{EnvTemplate: ""}
	data := templates.TemplateData{Name: "x", Domain: "x.dev"}

	if err := GenerateEnvFile(tmpDir, tmpl, data); err != nil {
		t.Fatalf("GenerateEnvFile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".env"))
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}
	if len(content) != 0 {
		t.Errorf("empty template should produce empty .env, got %d bytes", len(content))
	}
}

// ---------------------------------------------------------------------------
// ScaffoldProject: verify all files created with correct content
// ---------------------------------------------------------------------------

func TestScaffoldProjectVerifyDockerfileContent(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "proj")
	os.MkdirAll(projectPath, 0755)

	tmpl := &templates.Template{
		Dockerfile: "FROM node:20\nLABEL app={{.Name}}\nLABEL domain={{.Domain}}",
		Compose:    "services:\n  app:\n    image: {{.Name}}",
		Workflow:   "name: CI\n",
		GitIgnore:  ".env\nnode_modules/\n",
	}
	data := templates.TemplateData{Name: "myproj", Domain: "myproj.io"}

	if err := ScaffoldProject(projectPath, tmpl, data); err != nil {
		t.Fatalf("ScaffoldProject: %v", err)
	}

	// Dockerfile
	df, err := os.ReadFile(filepath.Join(projectPath, "Dockerfile"))
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	if !strings.Contains(string(df), "LABEL app=myproj") {
		t.Error("Dockerfile missing rendered LABEL app=myproj")
	}
	if !strings.Contains(string(df), "LABEL domain=myproj.io") {
		t.Error("Dockerfile missing rendered LABEL domain=myproj.io")
	}

	// docker-compose.yml
	dc, err := os.ReadFile(filepath.Join(projectPath, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("read docker-compose.yml: %v", err)
	}
	if !strings.Contains(string(dc), "image: myproj") {
		t.Error("docker-compose.yml missing rendered image name")
	}

	// .gitignore
	gi, err := os.ReadFile(filepath.Join(projectPath, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(gi), "node_modules/") {
		t.Error(".gitignore missing node_modules/")
	}

	// workflow
	wf, err := os.ReadFile(filepath.Join(projectPath, ".github", "workflows", "deploy.yml"))
	if err != nil {
		t.Fatalf("read deploy.yml: %v", err)
	}
	if !strings.Contains(string(wf), "name: CI") {
		t.Error("deploy.yml missing workflow name")
	}

	// deployments dir
	info, err := os.Stat(filepath.Join(projectPath, "deployments"))
	if err != nil {
		t.Fatalf("stat deployments: %v", err)
	}
	if !info.IsDir() {
		t.Error("deployments should be a directory")
	}
}

func TestScaffoldProjectNoTemplateVariables(t *testing.T) {
	// Templates with zero Go template variables should still work.
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "plain")
	os.MkdirAll(projectPath, 0755)

	tmpl := &templates.Template{
		Dockerfile: "FROM scratch\nCMD [\"hello\"]\n",
		Compose:    "services: {}\n",
		Workflow:   "name: noop\n",
		GitIgnore:  "*.log\n",
	}
	data := templates.TemplateData{Name: "plain", Domain: "plain.dev"}

	if err := ScaffoldProject(projectPath, tmpl, data); err != nil {
		t.Fatalf("ScaffoldProject: %v", err)
	}

	df, _ := os.ReadFile(filepath.Join(projectPath, "Dockerfile"))
	if string(df) != "FROM scratch\nCMD [\"hello\"]\n" {
		t.Errorf("Dockerfile content mismatch: %q", string(df))
	}
}

// ---------------------------------------------------------------------------
// InitAndPushRepo: verify function exists and returns wrapped errors
// ---------------------------------------------------------------------------

func TestInitAndPushRepoSignatureAndErrorWrapping(t *testing.T) {
	// We cannot test real git operations, but we can verify error wrapping
	// for a directory that exists but has no remote.
	tmpDir := t.TempDir()

	err := InitAndPushRepo(tmpDir, "https://github.com/fake/repo.git")
	if err == nil {
		t.Fatal("InitAndPushRepo should fail with fake remote")
	}
	if !strings.Contains(err.Error(), "running git") {
		t.Errorf("error should wrap with 'running git', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// indexOf (internal helper in scaffold_profile.go)
// ---------------------------------------------------------------------------

func TestIndexOfHelper(t *testing.T) {
	tests := []struct {
		s      string
		substr string
		want   int
	}{
		{"hello world", "world", 6},
		{"hello world", "nope", -1},
		{"", "x", -1},
		{"x", "", 0},
		{"abcabc", "abc", 0},
	}
	for _, tt := range tests {
		got := indexOf(tt.s, tt.substr)
		if got != tt.want {
			t.Errorf("indexOf(%q, %q) = %d, want %d", tt.s, tt.substr, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// replacePasswordPlaceholders: edge cases
// ---------------------------------------------------------------------------

func TestReplacePasswordPlaceholdersOnlyChangeme(t *testing.T) {
	// Content is exactly "CHANGEME" with no key.
	result := replacePasswordPlaceholders("CHANGEME", "app")
	if strings.Contains(result, "CHANGEME") {
		t.Error("CHANGEME should be replaced even when it is the entire content")
	}
	if len(result) != 32 {
		t.Errorf("expected 32 hex chars, got %d: %q", len(result), result)
	}
}

func TestReplacePasswordPlaceholdersManyConsecutive(t *testing.T) {
	// Multiple CHANGEME on separate lines.
	input := "P1=CHANGEME\nP2=CHANGEME\nP3=CHANGEME\nP4=CHANGEME\nP5=CHANGEME\n"
	result := replacePasswordPlaceholders(input, "app")
	if strings.Contains(result, "CHANGEME") {
		t.Error("all CHANGEME instances should be replaced")
	}
	// Extract values and verify uniqueness.
	seen := make(map[string]bool)
	for _, line := range strings.Split(result, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 && parts[1] != "" {
			if seen[parts[1]] {
				t.Errorf("duplicate secret found: %s", parts[1])
			}
			seen[parts[1]] = true
		}
	}
}

// ---------------------------------------------------------------------------
// SetupAuthorizedKeys: verify content format
// ---------------------------------------------------------------------------

func TestSetupAuthorizedKeysFormat(t *testing.T) {
	tmpDir := t.TempDir()
	pubKey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAISample deploy@fleetdeck"

	if err := SetupAuthorizedKeys(tmpDir, pubKey); err != nil {
		t.Fatalf("SetupAuthorizedKeys: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, ".ssh", "authorized_keys"))
	if err != nil {
		t.Fatalf("reading authorized_keys: %v", err)
	}
	content := string(data)

	// Must include the restriction prefix, the command, then the key.
	expected := `restrict,command="/usr/bin/docker compose" ` + pubKey + "\n"
	if content != expected {
		t.Errorf("authorized_keys content mismatch:\ngot:  %q\nwant: %q", content, expected)
	}
}
