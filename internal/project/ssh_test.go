package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestGenerateSSHKeypair(t *testing.T) {
	tmpDir := t.TempDir()

	privKeyPath, pubKeyStr, err := GenerateSSHKeypair(tmpDir)
	if err != nil {
		t.Fatalf("GenerateSSHKeypair() error: %v", err)
	}

	// Verify private key file exists at expected path
	expectedPrivPath := filepath.Join(tmpDir, ".ssh", "deploy_key")
	if privKeyPath != expectedPrivPath {
		t.Errorf("private key path = %q, want %q", privKeyPath, expectedPrivPath)
	}
	if _, err := os.Stat(privKeyPath); os.IsNotExist(err) {
		t.Fatal("private key file does not exist")
	}

	// Verify public key file exists
	pubKeyPath := filepath.Join(tmpDir, ".ssh", "deploy_key.pub")
	if _, err := os.Stat(pubKeyPath); os.IsNotExist(err) {
		t.Fatal("public key file does not exist")
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

	// Verify private key starts with OpenSSH PEM header
	privKeyData, err := os.ReadFile(privKeyPath)
	if err != nil {
		t.Fatalf("reading private key: %v", err)
	}
	if !strings.HasPrefix(string(privKeyData), "-----BEGIN OPENSSH PRIVATE KEY-----") {
		t.Error("private key should start with OPENSSH PRIVATE KEY header")
	}
	if !strings.Contains(string(privKeyData), "-----END OPENSSH PRIVATE KEY-----") {
		t.Error("private key should contain OPENSSH PRIVATE KEY footer")
	}

	// Verify public key is a valid SSH authorized key
	if !strings.HasPrefix(pubKeyStr, "ssh-ed25519 ") {
		t.Errorf("public key should start with 'ssh-ed25519 ', got prefix: %q", pubKeyStr[:20])
	}

	// Verify public key can be parsed
	_, _, _, _, err = ssh.ParseAuthorizedKey([]byte(pubKeyStr))
	if err != nil {
		t.Errorf("public key is not valid SSH authorized key format: %v", err)
	}

	// Verify public key file contents match returned string
	pubKeyFileData, err := os.ReadFile(pubKeyPath)
	if err != nil {
		t.Fatalf("reading public key file: %v", err)
	}
	if string(pubKeyFileData) != pubKeyStr {
		t.Error("public key file contents should match returned public key string")
	}
}

func TestGenerateSSHKeypairPermissions(t *testing.T) {
	tmpDir := t.TempDir()

	privKeyPath, _, err := GenerateSSHKeypair(tmpDir)
	if err != nil {
		t.Fatalf("GenerateSSHKeypair() error: %v", err)
	}

	info, err := os.Stat(privKeyPath)
	if err != nil {
		t.Fatalf("stat private key: %v", err)
	}

	// Verify private key has 0600 permissions
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("private key permissions = %o, want 0600", perm)
	}

	// Verify .ssh directory has 0700 permissions
	sshDir := filepath.Join(tmpDir, ".ssh")
	dirInfo, err := os.Stat(sshDir)
	if err != nil {
		t.Fatalf("stat .ssh dir: %v", err)
	}
	dirPerm := dirInfo.Mode().Perm()
	if dirPerm != 0700 {
		t.Errorf(".ssh dir permissions = %o, want 0700", dirPerm)
	}
}

func TestGenerateSSHKeypairDifferentKeys(t *testing.T) {
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	privPath1, pubKey1, err := GenerateSSHKeypair(tmpDir1)
	if err != nil {
		t.Fatalf("first GenerateSSHKeypair() error: %v", err)
	}

	privPath2, pubKey2, err := GenerateSSHKeypair(tmpDir2)
	if err != nil {
		t.Fatalf("second GenerateSSHKeypair() error: %v", err)
	}

	// Public keys should differ
	if pubKey1 == pubKey2 {
		t.Error("two generated keypairs should have different public keys")
	}

	// Private keys should differ
	priv1, err := os.ReadFile(privPath1)
	if err != nil {
		t.Fatalf("reading first private key: %v", err)
	}
	priv2, err := os.ReadFile(privPath2)
	if err != nil {
		t.Fatalf("reading second private key: %v", err)
	}
	if string(priv1) == string(priv2) {
		t.Error("two generated keypairs should have different private keys")
	}
}

func TestGenerateSSHKeypairInvalidPath(t *testing.T) {
	// Use a file path as directory to block MkdirAll
	tmpDir := t.TempDir()
	blockingFile := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blockingFile, []byte("x"), 0644); err != nil {
		t.Fatalf("creating blocking file: %v", err)
	}

	badPath := filepath.Join(blockingFile, "subdir")
	_, _, err := GenerateSSHKeypair(badPath)
	if err == nil {
		t.Fatal("GenerateSSHKeypair with invalid path should return error")
	}
	if !strings.Contains(err.Error(), "creating .ssh dir") {
		t.Errorf("error should mention 'creating .ssh dir', got: %v", err)
	}
}

func TestGenerateSSHKeypairIdempotent(t *testing.T) {
	tmpDir := t.TempDir()

	// Generate once
	privPath1, pubKey1, err := GenerateSSHKeypair(tmpDir)
	if err != nil {
		t.Fatalf("first GenerateSSHKeypair() error: %v", err)
	}

	// Generate again in same dir (overwrites)
	privPath2, pubKey2, err := GenerateSSHKeypair(tmpDir)
	if err != nil {
		t.Fatalf("second GenerateSSHKeypair() error: %v", err)
	}

	// Paths should be the same
	if privPath1 != privPath2 {
		t.Errorf("private key paths should be identical, got %q and %q", privPath1, privPath2)
	}

	// Keys should differ (new generation)
	if pubKey1 == pubKey2 {
		t.Error("regenerated keys should differ")
	}

	// Verify the file on disk matches the second generation
	pubKeyData, err := os.ReadFile(filepath.Join(tmpDir, ".ssh", "deploy_key.pub"))
	if err != nil {
		t.Fatalf("reading public key: %v", err)
	}
	if string(pubKeyData) != pubKey2 {
		t.Error("public key file should contain the latest generated key")
	}
}

func TestGenerateSSHKeypairPublicKeyFormat(t *testing.T) {
	tmpDir := t.TempDir()

	_, pubKey, err := GenerateSSHKeypair(tmpDir)
	if err != nil {
		t.Fatalf("GenerateSSHKeypair() error: %v", err)
	}

	// Public key should be ssh-ed25519 format
	if !strings.HasPrefix(pubKey, "ssh-ed25519 ") {
		t.Errorf("public key should start with 'ssh-ed25519 ', got: %q", pubKey[:30])
	}

	// Should end with newline (MarshalAuthorizedKey adds it)
	if !strings.HasSuffix(pubKey, "\n") {
		t.Error("public key should end with newline")
	}

	// Parse and verify it's a valid SSH public key
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pubKey))
	if err != nil {
		t.Fatalf("parsing public key: %v", err)
	}
	if key.Type() != "ssh-ed25519" {
		t.Errorf("key type = %q, want %q", key.Type(), "ssh-ed25519")
	}
}

func TestGenerateSSHKeypairPublicKeyFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()

	_, _, err := GenerateSSHKeypair(tmpDir)
	if err != nil {
		t.Fatalf("GenerateSSHKeypair() error: %v", err)
	}

	pubKeyPath := filepath.Join(tmpDir, ".ssh", "deploy_key.pub")
	info, err := os.Stat(pubKeyPath)
	if err != nil {
		t.Fatalf("stat public key: %v", err)
	}

	// Public key should have 0644 permissions
	perm := info.Mode().Perm()
	if perm != 0644 {
		t.Errorf("public key permissions = %o, want 0644", perm)
	}
}
