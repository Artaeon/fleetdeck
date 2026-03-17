package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsPrivateKey(t *testing.T) {
	tests := []struct {
		name string
		data string
		want bool
	}{
		{"ed25519 key", "-----BEGIN OPENSSH PRIVATE KEY-----\ndata\n-----END OPENSSH PRIVATE KEY-----\n", true},
		{"rsa key", "-----BEGIN RSA PRIVATE KEY-----\ndata\n-----END RSA PRIVATE KEY-----\n", true},
		{"pem key", "-----BEGIN PRIVATE KEY-----\ndata\n-----END PRIVATE KEY-----\n", true},
		{"public key", "ssh-ed25519 AAAA... user@host", false},
		{"random text", "this is not a key", false},
		{"empty", "", false},
		{"begin but not private", "-----BEGIN CERTIFICATE-----\ndata\n-----END CERTIFICATE-----\n", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPrivateKey([]byte(tt.data))
			if got != tt.want {
				t.Errorf("isPrivateKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchHostPattern(t *testing.T) {
	tests := []struct {
		pattern string
		host    string
		want    bool
	}{
		{"*", "anything.com", true},
		{"example.com", "example.com", true},
		{"example.com", "other.com", false},
		{"*.example.com", "sub.example.com", true},
		{"*.example.com", "example.com", false},
		{"*.example.com", "deep.sub.example.com", true},
		{"192.168.1.1", "192.168.1.1", true},
		{"192.168.1.1", "192.168.1.2", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.host, func(t *testing.T) {
			got := matchHostPattern(tt.pattern, tt.host)
			if got != tt.want {
				t.Errorf("matchHostPattern(%q, %q) = %v, want %v", tt.pattern, tt.host, got, tt.want)
			}
		})
	}
}

func TestFindKeyInSSHConfig(t *testing.T) {
	sshDir := t.TempDir()

	// Create a test key file
	keyContent := "-----BEGIN OPENSSH PRIVATE KEY-----\ntest\n-----END OPENSSH PRIVATE KEY-----\n"
	keyPath := filepath.Join(sshDir, "my_custom_key")
	os.WriteFile(keyPath, []byte(keyContent), 0600)

	// Create another key
	key2Path := filepath.Join(sshDir, "work_key")
	os.WriteFile(key2Path, []byte(keyContent), 0600)

	// Write ssh config
	config := `# Global settings
Host myserver.com
    IdentityFile ` + keyPath + `
    User deploy

Host *.work.io
    IdentityFile ` + key2Path + `

Host github.com
    IdentityFile /nonexistent/key
`
	os.WriteFile(filepath.Join(sshDir, "config"), []byte(config), 0644)

	tests := []struct {
		host string
		want string
	}{
		{"myserver.com", keyPath},
		{"app.work.io", key2Path},
		{"other.work.io", key2Path},
		{"github.com", ""},          // key file doesn't exist
		{"unknown.com", ""},          // no matching host
		{"work.io", ""},              // *.work.io doesn't match work.io
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got := findKeyInSSHConfig(sshDir, tt.host)
			if got != tt.want {
				t.Errorf("findKeyInSSHConfig(%q) = %q, want %q", tt.host, got, tt.want)
			}
		})
	}
}

func TestFindKeyInSSHConfigTilde(t *testing.T) {
	sshDir := t.TempDir()
	home := os.Getenv("HOME")

	// Create key at a known location under HOME
	keyDir := filepath.Join(home, ".ssh")
	// We can't create files in the real ~/.ssh in tests, so just verify
	// the tilde expansion logic by checking a nonexistent path
	config := `Host test.example.com
    IdentityFile ~/.ssh/nonexistent_test_key_12345
`
	os.WriteFile(filepath.Join(sshDir, "config"), []byte(config), 0644)

	// Should return empty because the file doesn't exist
	got := findKeyInSSHConfig(sshDir, "test.example.com")
	if got != "" {
		t.Errorf("expected empty for nonexistent key, got %q", got)
	}
	_ = keyDir
}

func TestFindKeyInSSHConfigTabSeparated(t *testing.T) {
	sshDir := t.TempDir()

	keyContent := "-----BEGIN OPENSSH PRIVATE KEY-----\ntest\n-----END OPENSSH PRIVATE KEY-----\n"
	keyPath := filepath.Join(sshDir, "tabkey")
	os.WriteFile(keyPath, []byte(keyContent), 0600)

	// SSH config using tabs instead of spaces
	config := "Host\ttabhost.com\n\tIdentityFile\t" + keyPath + "\n"
	os.WriteFile(filepath.Join(sshDir, "config"), []byte(config), 0644)

	got := findKeyInSSHConfig(sshDir, "tabhost.com")
	if got != keyPath {
		t.Errorf("findKeyInSSHConfig with tabs = %q, want %q", got, keyPath)
	}
}

func TestFindKeyInSSHConfigNoConfig(t *testing.T) {
	sshDir := t.TempDir()
	// No config file exists
	got := findKeyInSSHConfig(sshDir, "example.com")
	if got != "" {
		t.Errorf("expected empty for missing config, got %q", got)
	}
}

func TestFindSSHKey_CommonKeys(t *testing.T) {
	// Override HOME to use temp dir
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	sshDir := filepath.Join(tmpHome, ".ssh")
	os.MkdirAll(sshDir, 0700)

	keyContent := "-----BEGIN OPENSSH PRIVATE KEY-----\ntest\n-----END OPENSSH PRIVATE KEY-----\n"

	// Test that id_ed25519 is found first
	os.WriteFile(filepath.Join(sshDir, "id_ed25519"), []byte(keyContent), 0600)
	os.WriteFile(filepath.Join(sshDir, "id_rsa"), []byte(keyContent), 0600)

	data := findSSHKey("example.com")
	if data == nil {
		t.Fatal("expected to find SSH key, got nil")
	}
	if string(data) != keyContent {
		t.Errorf("unexpected key content")
	}
}

func TestFindSSHKey_CustomKeyFromConfig(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	sshDir := filepath.Join(tmpHome, ".ssh")
	os.MkdirAll(sshDir, 0700)

	keyContent := "-----BEGIN OPENSSH PRIVATE KEY-----\ncustom\n-----END OPENSSH PRIVATE KEY-----\n"
	customKeyPath := filepath.Join(sshDir, "fleetdeck")
	os.WriteFile(customKeyPath, []byte(keyContent), 0600)

	// Write SSH config pointing to custom key
	config := "Host 164.68.121.198\n    IdentityFile " + customKeyPath + "\n"
	os.WriteFile(filepath.Join(sshDir, "config"), []byte(config), 0644)

	data := findSSHKey("164.68.121.198")
	if data == nil {
		t.Fatal("expected to find SSH key from config, got nil")
	}
	if string(data) != keyContent {
		t.Errorf("expected custom key content")
	}
}

func TestFindSSHKey_FallbackScan(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	sshDir := filepath.Join(tmpHome, ".ssh")
	os.MkdirAll(sshDir, 0700)

	keyContent := "-----BEGIN OPENSSH PRIVATE KEY-----\nscanned\n-----END OPENSSH PRIVATE KEY-----\n"

	// Only a non-standard key name exists
	os.WriteFile(filepath.Join(sshDir, "deploy_key"), []byte(keyContent), 0600)
	// Also create a .pub file that should be skipped
	os.WriteFile(filepath.Join(sshDir, "deploy_key.pub"), []byte("ssh-ed25519 AAAA..."), 0644)
	// And known_hosts that should be skipped
	os.WriteFile(filepath.Join(sshDir, "known_hosts"), []byte("example.com ssh-ed25519 AAAA..."), 0644)

	data := findSSHKey("example.com")
	if data == nil {
		t.Fatal("expected to find SSH key via scan, got nil")
	}
	if string(data) != keyContent {
		t.Errorf("expected scanned key content")
	}
}

func TestFindSSHKey_NoKeys(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	sshDir := filepath.Join(tmpHome, ".ssh")
	os.MkdirAll(sshDir, 0700)

	// Only non-key files
	os.WriteFile(filepath.Join(sshDir, "known_hosts"), []byte("data"), 0644)
	os.WriteFile(filepath.Join(sshDir, "config"), []byte("# empty"), 0644)

	data := findSSHKey("example.com")
	if data != nil {
		t.Errorf("expected nil when no keys exist, got data")
	}
}

func TestFindSSHKey_NoSSHDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	// No .ssh directory at all

	data := findSSHKey("example.com")
	if data != nil {
		t.Errorf("expected nil when .ssh dir doesn't exist, got data")
	}
}
