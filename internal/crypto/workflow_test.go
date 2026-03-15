package crypto

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Workflow: encrypt multiple secrets then restore each one
// ---------------------------------------------------------------------------

func TestWorkflowEncryptSecretsThenRestore(t *testing.T) {
	key := DeriveKeyFromPassphrase("ops-team-master-key-2024")

	secrets := map[string]string{
		"db_password":    "P@ssw0rd!SuperSecret#2024",
		"api_key":        "sk-proj-abc123def456ghi789jkl012mno345pqr678stu901vwx234",
		"ssh_private_key": "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAABG5vbmU=\n-----END OPENSSH PRIVATE KEY-----",
	}

	// Encrypt all secrets.
	encrypted := make(map[string][]byte, len(secrets))
	for name, secret := range secrets {
		ct, err := Encrypt([]byte(secret), key)
		if err != nil {
			t.Fatalf("Encrypt(%s): %v", name, err)
		}
		encrypted[name] = ct
	}

	// Verify each encrypted value differs from its plaintext.
	for name, ct := range encrypted {
		if bytes.Equal(ct, []byte(secrets[name])) {
			t.Errorf("ciphertext for %s should differ from plaintext", name)
		}
	}

	// Verify all ciphertexts are distinct from each other.
	names := make([]string, 0, len(encrypted))
	for n := range encrypted {
		names = append(names, n)
	}
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if bytes.Equal(encrypted[names[i]], encrypted[names[j]]) {
				t.Errorf("ciphertext for %s and %s should differ", names[i], names[j])
			}
		}
	}

	// Decrypt each and verify correctness.
	for name, ct := range encrypted {
		pt, err := Decrypt(ct, key)
		if err != nil {
			t.Fatalf("Decrypt(%s): %v", name, err)
		}
		if string(pt) != secrets[name] {
			t.Errorf("decrypted %s: got %q, want %q", name, pt, secrets[name])
		}
	}
}

// ---------------------------------------------------------------------------
// Workflow: key rotation — re-encrypt with a new key
// ---------------------------------------------------------------------------

func TestWorkflowKeyRotation(t *testing.T) {
	oldKey := DeriveKeyFromPassphrase("old-master-key-v1")
	newKey := DeriveKeyFromPassphrase("new-master-key-v2")

	plaintext := []byte("DATABASE_URL=postgres://admin:secret@db:5432/myapp")

	// Encrypt with old key.
	ct, err := Encrypt(plaintext, oldKey)
	if err != nil {
		t.Fatalf("Encrypt with old key: %v", err)
	}

	// Decrypt with old key (simulating migration step).
	recovered, err := Decrypt(ct, oldKey)
	if err != nil {
		t.Fatalf("Decrypt with old key: %v", err)
	}
	if !bytes.Equal(recovered, plaintext) {
		t.Fatalf("intermediate decryption mismatch")
	}

	// Re-encrypt with new key.
	newCT, err := Encrypt(recovered, newKey)
	if err != nil {
		t.Fatalf("Encrypt with new key: %v", err)
	}

	// Verify old key can no longer decrypt the new ciphertext.
	_, err = Decrypt(newCT, oldKey)
	if err == nil {
		t.Error("old key should not decrypt ciphertext encrypted with new key")
	}

	// Verify new key successfully decrypts.
	final, err := Decrypt(newCT, newKey)
	if err != nil {
		t.Fatalf("Decrypt with new key: %v", err)
	}
	if !bytes.Equal(final, plaintext) {
		t.Errorf("final decryption mismatch: got %q, want %q", final, plaintext)
	}
}

// ---------------------------------------------------------------------------
// Workflow: encrypt a large .env file (~100KB)
// ---------------------------------------------------------------------------

func TestWorkflowLargeSecretFiles(t *testing.T) {
	key := DeriveKeyFromPassphrase("large-file-encryption-key")

	// Build a realistic ~100KB .env file.
	var builder strings.Builder
	for i := 0; i < 2000; i++ {
		builder.WriteString(fmt.Sprintf("SECRET_%04d=value_%04d_with_some_padding_to_make_it_larger\n", i, i))
	}
	envContent := []byte(builder.String())

	if len(envContent) < 100_000 {
		t.Fatalf("test .env file too small: %d bytes, expected at least 100000", len(envContent))
	}

	ct, err := Encrypt(envContent, key)
	if err != nil {
		t.Fatalf("Encrypt large file: %v", err)
	}

	// Ciphertext must be larger due to nonce + auth tag.
	if len(ct) <= len(envContent) {
		t.Errorf("ciphertext (%d bytes) should be larger than plaintext (%d bytes)", len(ct), len(envContent))
	}

	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt large file: %v", err)
	}

	if !bytes.Equal(pt, envContent) {
		t.Error("large file round-trip mismatch")
	}
}

// ---------------------------------------------------------------------------
// Workflow: encrypt binary data (simulating SSH private key bytes)
// ---------------------------------------------------------------------------

func TestWorkflowEncryptBinaryData(t *testing.T) {
	key := DeriveKeyFromPassphrase("binary-secret-key")

	// Simulate a 4096-bit RSA private key in DER format (random bytes).
	binaryData := make([]byte, 512)
	if _, err := rand.Read(binaryData); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	// Include some null bytes and high bytes that trip up string handling.
	binaryData[0] = 0x00
	binaryData[1] = 0xFF
	binaryData[100] = 0x00
	binaryData[200] = 0x00
	binaryData[511] = 0xFF

	ct, err := Encrypt(binaryData, key)
	if err != nil {
		t.Fatalf("Encrypt binary: %v", err)
	}

	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt binary: %v", err)
	}

	if !bytes.Equal(pt, binaryData) {
		t.Error("binary data round-trip mismatch")
	}
}

// ---------------------------------------------------------------------------
// Workflow: derive key twice from same passphrase, both decrypt same ciphertext
// ---------------------------------------------------------------------------

func TestWorkflowMultipleKeysFromSamePassphrase(t *testing.T) {
	passphrase := "shared-team-passphrase-2024"

	// First derivation — encrypt.
	key1 := DeriveKeyFromPassphrase(passphrase)
	plaintext := []byte("shared secret across team members")

	ct, err := Encrypt(plaintext, key1)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Second derivation — decrypt (simulating another server or restart).
	key2 := DeriveKeyFromPassphrase(passphrase)

	if !bytes.Equal(key1, key2) {
		t.Fatal("keys derived from same passphrase should be identical")
	}

	pt, err := Decrypt(ct, key2)
	if err != nil {
		t.Fatalf("Decrypt with second key: %v", err)
	}

	if !bytes.Equal(pt, plaintext) {
		t.Errorf("decrypted with second key: got %q, want %q", pt, plaintext)
	}

	// Also verify using random-salt derivation with salt storage.
	key3, salt := DeriveKey(passphrase)
	ct2, err := Encrypt(plaintext, key3)
	if err != nil {
		t.Fatalf("Encrypt with random-salt key: %v", err)
	}

	key4 := DeriveKeyWithSalt(passphrase, salt)
	pt2, err := Decrypt(ct2, key4)
	if err != nil {
		t.Fatalf("Decrypt with re-derived random-salt key: %v", err)
	}

	if !bytes.Equal(pt2, plaintext) {
		t.Errorf("random-salt round-trip mismatch: got %q, want %q", pt2, plaintext)
	}
}

// ---------------------------------------------------------------------------
// Workflow: encrypt empty secret, verify round-trip
// ---------------------------------------------------------------------------

func TestWorkflowEncryptEmptySecret(t *testing.T) {
	key := DeriveKeyFromPassphrase("empty-secret-workflow")

	plaintext := []byte("")

	ct, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}

	// Even an empty plaintext should produce a non-empty ciphertext
	// (nonce + auth tag).
	if len(ct) == 0 {
		t.Error("ciphertext of empty plaintext should not be empty")
	}

	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}

	if len(pt) != 0 {
		t.Errorf("expected empty plaintext, got %d bytes: %q", len(pt), pt)
	}
}

// ---------------------------------------------------------------------------
// Workflow: verify that many wrong keys all fail to decrypt
// ---------------------------------------------------------------------------

func TestWorkflowDecryptWithEveryWrongKeyFails(t *testing.T) {
	correctKey := DeriveKeyFromPassphrase("the-one-true-key")
	plaintext := []byte("this must remain confidential")

	ct, err := Encrypt(plaintext, correctKey)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Generate 10 different wrong keys and verify none can decrypt.
	for i := 0; i < 10; i++ {
		wrongKey := DeriveKeyFromPassphrase(fmt.Sprintf("wrong-key-attempt-%d", i))

		// Verify the wrong key is actually different from the correct key.
		if bytes.Equal(wrongKey, correctKey) {
			t.Fatalf("wrong key %d unexpectedly matches correct key", i)
		}

		_, err := Decrypt(ct, wrongKey)
		if err == nil {
			t.Errorf("wrong key %d should not decrypt the ciphertext", i)
		}
	}

	// Also try with fully random 32-byte keys.
	for i := 0; i < 10; i++ {
		randomKey := make([]byte, keyLen)
		if _, err := rand.Read(randomKey); err != nil {
			t.Fatalf("rand.Read: %v", err)
		}

		_, err := Decrypt(ct, randomKey)
		if err == nil {
			t.Errorf("random key %d should not decrypt the ciphertext", i)
		}
	}

	// Confirm the correct key still works.
	pt, err := Decrypt(ct, correctKey)
	if err != nil {
		t.Fatalf("Decrypt with correct key: %v", err)
	}
	if !bytes.Equal(pt, plaintext) {
		t.Errorf("correct key decryption mismatch: got %q, want %q", pt, plaintext)
	}
}
