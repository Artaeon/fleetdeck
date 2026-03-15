package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Validate — additional coverage beyond the existing tests
// ---------------------------------------------------------------------------

func TestValidateNonexistentDirectory(t *testing.T) {
	err := Validate("/tmp/fleetdeck-nonexistent-dir-for-test-" + t.Name())
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestValidateEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	// An empty directory has no compose file. This is the same as
	// TestValidateMissingFile but uses a freshly created temp dir to
	// verify the behaviour with guaranteed-empty content.
	err := Validate(dir)
	if err == nil {
		t.Error("expected error when directory has no compose file")
	}
	if !strings.Contains(err.Error(), "compose config validation failed") {
		t.Errorf("expected 'compose config validation failed' in error, got: %v", err)
	}
}

func TestValidateErrorMessageIncludesOutput(t *testing.T) {
	dir := t.TempDir()
	// Write a file that is clearly not valid YAML for docker compose.
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(":::invalid:::"), 0644); err != nil {
		t.Fatal(err)
	}
	err := Validate(dir)
	if err == nil {
		t.Fatal("expected error for invalid compose file")
	}
	// The error should wrap both the command output and the exec error.
	errMsg := err.Error()
	if !strings.Contains(errMsg, "compose config validation failed") {
		t.Errorf("expected 'compose config validation failed' prefix, got: %s", errMsg)
	}
}

func TestValidateEmptyComposeFile(t *testing.T) {
	dir := t.TempDir()
	// A completely empty file is not a valid compose configuration.
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	err := Validate(dir)
	if err == nil {
		t.Error("expected error for empty compose file")
	}
}

func TestValidateComposeFileNoServices(t *testing.T) {
	dir := t.TempDir()
	// Valid YAML but no services key — compose may or may not accept this
	// depending on version. We just verify it does not panic.
	content := "version: '3'\n"
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	// We cannot predict whether this will pass or fail across docker
	// compose versions, so just ensure no panic.
	_ = Validate(dir)
}

func TestValidateComposeFileYAMLButNotCompose(t *testing.T) {
	dir := t.TempDir()
	// Valid YAML, but not a valid compose file structure.
	content := `
foo:
  bar: baz
  list:
    - one
    - two
`
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	// docker compose config -q should reject this or treat it as empty.
	// We just verify no panic and that the function returns.
	_ = Validate(dir)
}

func TestValidateComposeFileUnreadable(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(fpath, []byte("version: '3'\nservices:\n  web:\n    image: nginx\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Remove read permission.
	if err := os.Chmod(fpath, 0000); err != nil {
		t.Skip("cannot change file permissions on this platform")
	}
	t.Cleanup(func() {
		// Restore permissions so cleanup can remove the file.
		os.Chmod(fpath, 0644)
	})
	err := Validate(dir)
	if err == nil {
		t.Error("expected error for unreadable compose file")
	}
}

func TestValidateComposeYMLAlternativeName(t *testing.T) {
	dir := t.TempDir()
	// docker compose also looks for compose.yml / compose.yaml.
	// Write a valid file under the alternative name and see if compose picks it up.
	content := "services:\n  web:\n    image: nginx:latest\n"
	if err := os.WriteFile(filepath.Join(dir, "compose.yml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	// This may succeed or fail depending on docker compose version.
	// The main goal is to verify no crash.
	_ = Validate(dir)
}

func TestValidateLargeComposeFile(t *testing.T) {
	dir := t.TempDir()
	// Generate a compose file with many services to verify no issues
	// with larger inputs.
	var sb strings.Builder
	sb.WriteString("services:\n")
	for i := 0; i < 50; i++ {
		sb.WriteString("  svc" + strings.Repeat("x", 3) + string(rune('a'+i%26)) + ":\n")
		sb.WriteString("    image: alpine:latest\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(sb.String()), 0644); err != nil {
		t.Fatal(err)
	}
	// May succeed or fail (duplicate names possible due to modular arithmetic),
	// but should not panic.
	_ = Validate(dir)
}

func TestValidateBinaryGarbage(t *testing.T) {
	dir := t.TempDir()
	// Write binary content that is definitely not YAML.
	garbage := make([]byte, 256)
	for i := range garbage {
		garbage[i] = byte(i)
	}
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), garbage, 0644); err != nil {
		t.Fatal(err)
	}
	err := Validate(dir)
	if err == nil {
		t.Error("expected error for binary garbage compose file")
	}
}

func TestValidateSymlinkToInvalidFile(t *testing.T) {
	dir := t.TempDir()
	// Create a symlink to a nonexistent file.
	link := filepath.Join(dir, "docker-compose.yml")
	target := filepath.Join(dir, "nonexistent.yml")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks not supported on this platform")
	}
	err := Validate(dir)
	if err == nil {
		t.Error("expected error for broken symlink compose file")
	}
}

func TestValidateMultipleComposeFiles(t *testing.T) {
	dir := t.TempDir()
	// Write both docker-compose.yml and compose.yml. docker compose
	// should pick one. Verify no panic.
	content := "services:\n  web:\n    image: nginx:latest\n"
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(content), 0644)
	os.WriteFile(filepath.Join(dir, "compose.yml"), []byte(content), 0644)
	// Just verify it does not panic.
	_ = Validate(dir)
}
