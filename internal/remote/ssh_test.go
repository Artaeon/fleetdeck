package remote

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func generateTestEncryptedED25519Key(t *testing.T, passphrase []byte) []byte {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generating ed25519 key: %v", err)
	}

	pemBlock, err := ssh.MarshalPrivateKeyWithPassphrase(priv, "", passphrase)
	if err != nil {
		t.Fatalf("marshalling encrypted private key: %v", err)
	}

	return pem.EncodeToMemory(pemBlock)
}

func TestParsePrivateKeyUnencrypted(t *testing.T) {
	keyData := generateTestED25519Key(t)

	signer, err := ParsePrivateKey(keyData, nil)
	if err != nil {
		t.Fatalf("ParsePrivateKey() returned unexpected error: %v", err)
	}
	if signer == nil {
		t.Fatal("ParsePrivateKey() returned nil signer")
	}

	pub := signer.PublicKey()
	if pub == nil {
		t.Fatal("signer.PublicKey() returned nil")
	}
	if pub.Type() != "ssh-ed25519" {
		t.Errorf("expected key type ssh-ed25519, got %s", pub.Type())
	}
}

func TestParsePrivateKeyEncryptedWithPassphrase(t *testing.T) {
	passphrase := []byte("test-passphrase-123")
	keyData := generateTestEncryptedED25519Key(t, passphrase)

	signer, err := ParsePrivateKey(keyData, passphrase)
	if err != nil {
		t.Fatalf("ParsePrivateKey() with correct passphrase returned error: %v", err)
	}
	if signer == nil {
		t.Fatal("ParsePrivateKey() returned nil signer")
	}

	pub := signer.PublicKey()
	if pub == nil {
		t.Fatal("signer.PublicKey() returned nil")
	}
	if pub.Type() != "ssh-ed25519" {
		t.Errorf("expected key type ssh-ed25519, got %s", pub.Type())
	}
}

func TestParsePrivateKeyEncryptedWrongPassphrase(t *testing.T) {
	passphrase := []byte("correct-passphrase")
	keyData := generateTestEncryptedED25519Key(t, passphrase)

	signer, err := ParsePrivateKey(keyData, []byte("wrong-passphrase"))
	if err == nil {
		t.Fatal("ParsePrivateKey() should return error with wrong passphrase")
	}
	if signer != nil {
		t.Error("ParsePrivateKey() should return nil signer on error")
	}
}

func TestParsePrivateKeyEncryptedNoPassphrase(t *testing.T) {
	passphrase := []byte("my-secret")
	keyData := generateTestEncryptedED25519Key(t, passphrase)

	signer, err := ParsePrivateKey(keyData, nil)
	if err == nil {
		t.Fatal("ParsePrivateKey() should return error for encrypted key with no passphrase")
	}
	if signer != nil {
		t.Error("ParsePrivateKey() should return nil signer on error")
	}
	if !strings.Contains(err.Error(), "--passphrase") {
		t.Errorf("error should mention --passphrase flag, got: %v", err)
	}
}

func TestParsePrivateKeyInvalidData(t *testing.T) {
	signer, err := ParsePrivateKey([]byte("this is not a valid key"), nil)
	if err == nil {
		t.Fatal("ParsePrivateKey() should return error for garbage data")
	}
	if signer != nil {
		t.Error("ParsePrivateKey() should return nil signer on error")
	}
}
