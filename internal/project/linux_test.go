package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupAuthorizedKeys(t *testing.T) {
	tmpDir := t.TempDir()
	publicKey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExamplePublicKeyDataHere test@host"

	err := SetupAuthorizedKeys(tmpDir, publicKey)
	if err != nil {
		t.Fatalf("SetupAuthorizedKeys() error: %v", err)
	}

	// Verify .ssh directory was created
	sshDir := filepath.Join(tmpDir, ".ssh")
	info, err := os.Stat(sshDir)
	if err != nil {
		t.Fatalf("stat .ssh dir: %v", err)
	}
	if !info.IsDir() {
		t.Error(".ssh should be a directory")
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf(".ssh dir permissions = %o, want 0700", perm)
	}

	// Verify authorized_keys file exists and has correct permissions
	authKeysPath := filepath.Join(sshDir, "authorized_keys")
	fileInfo, err := os.Stat(authKeysPath)
	if err != nil {
		t.Fatalf("stat authorized_keys: %v", err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0600 {
		t.Errorf("authorized_keys permissions = %o, want 0600", perm)
	}

	// Verify file contents contain the public key
	data, err := os.ReadFile(authKeysPath)
	if err != nil {
		t.Fatalf("reading authorized_keys: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, publicKey) {
		t.Error("authorized_keys should contain the public key")
	}

	// Verify the file ends with a newline
	if !strings.HasSuffix(content, "\n") {
		t.Error("authorized_keys should end with a newline")
	}
}

func TestSetupAuthorizedKeysRestriction(t *testing.T) {
	tmpDir := t.TempDir()
	publicKey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExamplePublicKeyDataHere test@host"

	err := SetupAuthorizedKeys(tmpDir, publicKey)
	if err != nil {
		t.Fatalf("SetupAuthorizedKeys() error: %v", err)
	}

	authKeysPath := filepath.Join(tmpDir, ".ssh", "authorized_keys")
	data, err := os.ReadFile(authKeysPath)
	if err != nil {
		t.Fatalf("reading authorized_keys: %v", err)
	}
	content := string(data)

	// Verify the restrict prefix is present
	if !strings.HasPrefix(content, "restrict,command=") {
		t.Errorf("authorized_keys should start with 'restrict,command=', got: %q", content[:min(40, len(content))])
	}

	// Verify the specific command restriction
	if !strings.Contains(content, `command="/usr/bin/docker compose"`) {
		t.Error("authorized_keys should contain command restriction for docker compose")
	}

	// Verify the full format: restrict,command="..." <key>
	expectedPrefix := `restrict,command="/usr/bin/docker compose" `
	if !strings.HasPrefix(content, expectedPrefix) {
		t.Errorf("expected prefix %q, got: %q", expectedPrefix, content[:min(len(expectedPrefix)+10, len(content))])
	}
}

func TestSetupAuthorizedKeysIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	publicKey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExamplePublicKeyDataHere test@host"

	// Call twice - should not fail
	if err := SetupAuthorizedKeys(tmpDir, publicKey); err != nil {
		t.Fatalf("first SetupAuthorizedKeys() error: %v", err)
	}
	if err := SetupAuthorizedKeys(tmpDir, publicKey); err != nil {
		t.Fatalf("second SetupAuthorizedKeys() error: %v", err)
	}

	// Verify the file has only one entry (overwrites, not appends)
	authKeysPath := filepath.Join(tmpDir, ".ssh", "authorized_keys")
	data, err := os.ReadFile(authKeysPath)
	if err != nil {
		t.Fatalf("reading authorized_keys: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line after two calls (overwrite), got %d lines", len(lines))
	}
}

func TestLinuxUserNameEdgeCases(t *testing.T) {
	tests := []struct {
		project  string
		expected string
	}{
		{"myapp", "fleetdeck-myapp"},
		{"test-project", "fleetdeck-test-project"},
		{"a", "fleetdeck-a"},
		{"123", "fleetdeck-123"},
		{"a-b-c", "fleetdeck-a-b-c"},
		{"x1", "fleetdeck-x1"},
		{"my-really-long-project-name", "fleetdeck-my-really-long-project-name"},
	}

	for _, tt := range tests {
		t.Run(tt.project, func(t *testing.T) {
			got := LinuxUserName(tt.project)
			if got != tt.expected {
				t.Errorf("LinuxUserName(%q) = %q, want %q", tt.project, got, tt.expected)
			}
			// Verify prefix is always "fleetdeck-"
			if !strings.HasPrefix(got, "fleetdeck-") {
				t.Errorf("LinuxUserName(%q) should have 'fleetdeck-' prefix", tt.project)
			}
			// Verify the project name is preserved after the prefix
			suffix := strings.TrimPrefix(got, "fleetdeck-")
			if suffix != tt.project {
				t.Errorf("LinuxUserName(%q) suffix = %q, want %q", tt.project, suffix, tt.project)
			}
		})
	}
}

func TestValidateNameComprehensive(t *testing.T) {
	validNames := []struct {
		name string
		desc string
	}{
		{"a", "single character"},
		{"z", "single character z"},
		{"0", "single digit"},
		{"9", "single digit 9"},
		{"ab", "two characters"},
		{"a1", "letter and digit"},
		{"1a", "digit and letter"},
		{"my-app", "with hyphen"},
		{"a-b", "minimal with hyphen"},
		{"my-cool-app", "multiple hyphens"},
		{"app123", "letters and digits"},
		{"123app", "digits then letters"},
		{"a1b2c3d4", "alternating"},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "63 chars (max)"},
	}

	for _, tt := range validNames {
		t.Run("valid/"+tt.desc, func(t *testing.T) {
			if err := ValidateName(tt.name); err != nil {
				t.Errorf("ValidateName(%q) should be valid (%s), got: %v", tt.name, tt.desc, err)
			}
		})
	}

	invalidNames := []struct {
		name string
		desc string
	}{
		{"", "empty string"},
		{"-", "just a hyphen"},
		{"-a", "starts with hyphen"},
		{"a-", "ends with hyphen"},
		{"-a-", "starts and ends with hyphen"},
		{"my--app", "consecutive hyphens"},
		{"a--b", "consecutive hyphens minimal"},
		{"---", "all hyphens"},
		{"My-App", "uppercase letters"},
		{"MYAPP", "all uppercase"},
		{"my app", "space in name"},
		{"my_app", "underscore"},
		{"my.app", "dot"},
		{"my@app", "at sign"},
		{"my/app", "slash"},
		{"my\\app", "backslash"},
		{"../etc", "path traversal"},
		{"my app!", "special characters"},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "64 chars (too long)"},
		{strings.Repeat("a", 100), "100 chars (way too long)"},
	}

	for _, tt := range invalidNames {
		t.Run("invalid/"+tt.desc, func(t *testing.T) {
			if err := ValidateName(tt.name); err == nil {
				t.Errorf("ValidateName(%q) should be invalid (%s), got nil error", tt.name, tt.desc)
			}
		})
	}
}

func TestValidateNameErrorMessages(t *testing.T) {
	// Verify error messages are descriptive
	err := ValidateName("")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "1-63 characters") {
		t.Errorf("empty name error should mention length constraint, got: %v", err)
	}

	err = ValidateName(strings.Repeat("a", 64))
	if err == nil {
		t.Fatal("expected error for 64-char name")
	}
	if !strings.Contains(err.Error(), "1-63 characters") {
		t.Errorf("too-long name error should mention length constraint, got: %v", err)
	}

	err = ValidateName("My-App")
	if err == nil {
		t.Fatal("expected error for uppercase name")
	}
	if !strings.Contains(err.Error(), "lowercase") {
		t.Errorf("uppercase name error should mention lowercase, got: %v", err)
	}

	err = ValidateName("my--app")
	if err == nil {
		t.Fatal("expected error for consecutive hyphens")
	}
	if !strings.Contains(err.Error(), "consecutive hyphens") {
		t.Errorf("double hyphen error should mention consecutive hyphens, got: %v", err)
	}
}

func TestSetupAuthorizedKeysInvalidPath(t *testing.T) {
	// Use a file to block MkdirAll
	tmpDir := t.TempDir()
	blockingFile := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blockingFile, []byte("x"), 0644); err != nil {
		t.Fatalf("creating blocking file: %v", err)
	}

	badPath := filepath.Join(blockingFile, "subdir")
	err := SetupAuthorizedKeys(badPath, "ssh-ed25519 AAAA test@host")
	if err == nil {
		t.Fatal("SetupAuthorizedKeys with invalid path should return error")
	}
	if !strings.Contains(err.Error(), "creating .ssh directory") {
		t.Errorf("error should mention 'creating .ssh directory', got: %v", err)
	}
}

func TestSetupAuthorizedKeysEmptyKey(t *testing.T) {
	tmpDir := t.TempDir()

	// Empty key should still write the restrict prefix
	err := SetupAuthorizedKeys(tmpDir, "")
	if err != nil {
		t.Fatalf("SetupAuthorizedKeys with empty key: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, ".ssh", "authorized_keys"))
	if err != nil {
		t.Fatalf("reading authorized_keys: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "restrict,command=") {
		t.Error("authorized_keys should contain restrict prefix even with empty key")
	}
}

func TestSetupAuthorizedKeysOverwrite(t *testing.T) {
	tmpDir := t.TempDir()

	key1 := "ssh-ed25519 AAAA1111 user1@host"
	key2 := "ssh-ed25519 AAAA2222 user2@host"

	if err := SetupAuthorizedKeys(tmpDir, key1); err != nil {
		t.Fatalf("first SetupAuthorizedKeys: %v", err)
	}
	if err := SetupAuthorizedKeys(tmpDir, key2); err != nil {
		t.Fatalf("second SetupAuthorizedKeys: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, ".ssh", "authorized_keys"))
	if err != nil {
		t.Fatalf("reading authorized_keys: %v", err)
	}
	content := string(data)

	// Should contain the second key, not the first (overwrite behavior)
	if strings.Contains(content, key1) {
		t.Error("authorized_keys should not contain the first key after overwrite")
	}
	if !strings.Contains(content, key2) {
		t.Error("authorized_keys should contain the second key")
	}
}

func TestLinuxUserNameConsistency(t *testing.T) {
	// Verify that LinuxUserName is deterministic
	name1 := LinuxUserName("myapp")
	name2 := LinuxUserName("myapp")
	if name1 != name2 {
		t.Errorf("LinuxUserName should be deterministic: %q != %q", name1, name2)
	}
}

func TestLinuxUserNamePrefix(t *testing.T) {
	// All Linux usernames should have the fleetdeck- prefix
	names := []string{"a", "test", "my-app", "123", "x1y2z3"}
	for _, n := range names {
		result := LinuxUserName(n)
		if !strings.HasPrefix(result, "fleetdeck-") {
			t.Errorf("LinuxUserName(%q) = %q, missing 'fleetdeck-' prefix", n, result)
		}
		// The part after the prefix should be exactly the input
		after := strings.TrimPrefix(result, "fleetdeck-")
		if after != n {
			t.Errorf("LinuxUserName(%q) suffix = %q, want %q", n, after, n)
		}
	}
}

func TestChownProjectDirErrorWrapping(t *testing.T) {
	// chown requires root and valid user, so this should fail with error wrapping
	err := ChownProjectDir("nonexistent-project", "/tmp/nonexistent-path")
	if err == nil {
		t.Skip("chown succeeded unexpectedly (running as root?)")
	}
	if !strings.Contains(err.Error(), "chown") {
		t.Errorf("error should mention 'chown', got: %v", err)
	}
}

func TestCreateLinuxUserErrorWrapping(t *testing.T) {
	// useradd requires root, so this should fail
	err := CreateLinuxUser("test-project", "/tmp/nonexistent-path")
	if err == nil {
		t.Skip("useradd succeeded unexpectedly (running as root?)")
	}
	// Error should either say user exists or fail from useradd
	errMsg := err.Error()
	if !strings.Contains(errMsg, "already exists") && !strings.Contains(errMsg, "creating user") {
		t.Errorf("error should mention user creation, got: %v", err)
	}
}

func TestDeleteLinuxUserNonexistent(t *testing.T) {
	// Deleting a user that doesn't exist should return nil (no-op)
	err := DeleteLinuxUser("definitely-nonexistent-project-xyz123")
	if err != nil {
		t.Errorf("DeleteLinuxUser for nonexistent user should return nil, got: %v", err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
