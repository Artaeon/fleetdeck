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
