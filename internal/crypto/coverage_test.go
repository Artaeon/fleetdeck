package crypto

import (
	"bytes"
	"crypto/rand"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Encrypt/Decrypt round-trip with various data sizes
// ---------------------------------------------------------------------------

func TestRoundtripEmptyData(t *testing.T) {
	key := DeriveKeyFromPassphrase("empty-test")
	plaintext := []byte{}

	ct, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(pt, plaintext) {
		t.Errorf("roundtrip empty: got %q, want %q", pt, plaintext)
	}
}

func TestRoundtripSingleByte(t *testing.T) {
	key := DeriveKeyFromPassphrase("single-byte")
	plaintext := []byte{0x42}

	ct, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(pt, plaintext) {
		t.Errorf("roundtrip single byte: got %x, want %x", pt, plaintext)
	}
}

func TestRoundtripSmallData(t *testing.T) {
	key := DeriveKeyFromPassphrase("small-data")
	plaintext := []byte("short")

	ct, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(pt, plaintext) {
		t.Errorf("roundtrip small: got %q, want %q", pt, plaintext)
	}
}

func TestRoundtripMediumData(t *testing.T) {
	key := DeriveKeyFromPassphrase("medium-data")
	// 4KB of data
	plaintext := make([]byte, 4096)
	for i := range plaintext {
		plaintext[i] = byte(i % 256)
	}

	ct, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(pt, plaintext) {
		t.Error("roundtrip medium data mismatch")
	}
}

func TestRoundtripLargeData(t *testing.T) {
	key := DeriveKeyFromPassphrase("large-data")
	// 1MB of random data
	plaintext := make([]byte, 1024*1024)
	if _, err := rand.Read(plaintext); err != nil {
		t.Fatalf("rand: %v", err)
	}

	ct, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Ciphertext should be larger than plaintext (nonce + auth tag)
	if len(ct) <= len(plaintext) {
		t.Errorf("ciphertext (%d bytes) should be larger than plaintext (%d bytes)", len(ct), len(plaintext))
	}

	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(pt, plaintext) {
		t.Error("roundtrip large data mismatch")
	}
}

func TestRoundtripExactBlockSize(t *testing.T) {
	key := DeriveKeyFromPassphrase("block-size")
	// AES block size is 16 bytes
	plaintext := []byte("0123456789abcdef") // exactly 16 bytes

	ct, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(pt, plaintext) {
		t.Errorf("roundtrip block-size: got %q, want %q", pt, plaintext)
	}
}

// ---------------------------------------------------------------------------
// Key derivation: DeriveKeyFromPassphrase consistency
// ---------------------------------------------------------------------------

func TestDeriveKeyFromPassphraseConsistentAcrossCalls(t *testing.T) {
	passphrase := "consistent-key-derivation-test"

	// Derive the key multiple times and verify they are identical
	keys := make([][]byte, 5)
	for i := 0; i < 5; i++ {
		keys[i] = DeriveKeyFromPassphrase(passphrase)
	}

	for i := 1; i < len(keys); i++ {
		if !bytes.Equal(keys[0], keys[i]) {
			t.Errorf("key derivation call %d produced different key", i)
		}
	}
}

func TestDeriveKeyFromPassphraseDifferentPassphrases(t *testing.T) {
	passphrases := []string{
		"alpha",
		"alpha1",
		"Alpha",
		"ALPHA",
		"alpha ",
		" alpha",
	}

	keys := make(map[string]string)
	for _, p := range passphrases {
		k := DeriveKeyFromPassphrase(p)
		kHex := string(k) // using raw bytes as map key
		if prev, exists := keys[kHex]; exists {
			t.Errorf("passphrase %q produced same key as %q", p, prev)
		}
		keys[kHex] = p
	}
}

func TestDeriveKeyFromPassphraseKeyLength(t *testing.T) {
	key := DeriveKeyFromPassphrase("length-test")
	if len(key) != keyLen {
		t.Errorf("expected key length %d, got %d", keyLen, len(key))
	}
}

func TestDeriveKeyWithSaltConsistency(t *testing.T) {
	passphrase := "salt-consistency-test"
	salt := []byte("fixed-salt-value")

	k1 := DeriveKeyWithSalt(passphrase, salt)
	k2 := DeriveKeyWithSalt(passphrase, salt)

	if !bytes.Equal(k1, k2) {
		t.Error("DeriveKeyWithSalt should produce identical keys with same inputs")
	}

	if len(k1) != keyLen {
		t.Errorf("expected key length %d, got %d", keyLen, len(k1))
	}
}

func TestDeriveKeyWithSaltDifferentSalts(t *testing.T) {
	passphrase := "same-passphrase"
	salt1 := []byte("salt-one-here!!!")
	salt2 := []byte("salt-two-here!!!")

	k1 := DeriveKeyWithSalt(passphrase, salt1)
	k2 := DeriveKeyWithSalt(passphrase, salt2)

	if bytes.Equal(k1, k2) {
		t.Error("different salts should produce different keys")
	}
}

func TestDeriveKeyRandomSaltUniqueness(t *testing.T) {
	// DeriveKey uses a random salt each time, so successive calls
	// should produce different keys.
	k1, s1 := DeriveKey("same-pass")
	k2, s2 := DeriveKey("same-pass")

	if bytes.Equal(s1, s2) {
		t.Error("random salts should be unique (extremely unlikely collision)")
	}

	if bytes.Equal(k1, k2) {
		t.Error("keys derived with different random salts should differ")
	}
}

func TestDeriveKeySaltLength(t *testing.T) {
	_, salt := DeriveKey("salt-len-test")
	if len(salt) != saltLen {
		t.Errorf("expected salt length %d, got %d", saltLen, len(salt))
	}
}

// ---------------------------------------------------------------------------
// Error handling: decrypt with wrong key, corrupt data
// ---------------------------------------------------------------------------

func TestDecryptWrongKeyFromPassphrase(t *testing.T) {
	k1 := DeriveKeyFromPassphrase("correct-passphrase")
	k2 := DeriveKeyFromPassphrase("wrong-passphrase")

	ct, err := Encrypt([]byte("sensitive data"), k1)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	_, err = Decrypt(ct, k2)
	if err == nil {
		t.Error("expected decryption to fail with wrong key")
	}
}

func TestDecryptCorruptedCiphertextFlippedBit(t *testing.T) {
	key := DeriveKeyFromPassphrase("corruption-test")
	plaintext := []byte("data to corrupt")

	ct, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Flip a bit in the middle of the ciphertext
	corrupted := make([]byte, len(ct))
	copy(corrupted, ct)
	mid := len(corrupted) / 2
	corrupted[mid] ^= 0xFF

	_, err = Decrypt(corrupted, key)
	if err == nil {
		t.Error("expected decryption to fail with corrupted ciphertext")
	}
}

func TestDecryptCorruptedCiphertextTruncated(t *testing.T) {
	key := DeriveKeyFromPassphrase("truncation-test")
	plaintext := []byte("data that will be truncated during decryption")

	ct, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Truncate the ciphertext (remove the auth tag)
	truncated := ct[:len(ct)-5]

	_, err = Decrypt(truncated, key)
	if err == nil {
		t.Error("expected decryption to fail with truncated ciphertext")
	}
}

func TestDecryptCorruptedNonce(t *testing.T) {
	key := DeriveKeyFromPassphrase("nonce-corruption")
	plaintext := []byte("nonce will be corrupted")

	ct, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Corrupt the nonce (first 12 bytes for GCM)
	corrupted := make([]byte, len(ct))
	copy(corrupted, ct)
	corrupted[0] ^= 0xFF
	corrupted[1] ^= 0xFF

	_, err = Decrypt(corrupted, key)
	if err == nil {
		t.Error("expected decryption to fail with corrupted nonce")
	}
}

func TestDecryptEmptyCiphertext(t *testing.T) {
	key := DeriveKeyFromPassphrase("empty-ct-test")

	_, err := Decrypt([]byte{}, key)
	if err == nil {
		t.Error("expected error for empty ciphertext")
	}
}

func TestDecryptNilCiphertext(t *testing.T) {
	key := DeriveKeyFromPassphrase("nil-ct-test")

	_, err := Decrypt(nil, key)
	if err == nil {
		t.Error("expected error for nil ciphertext")
	}
}

func TestEncryptNilPlaintext(t *testing.T) {
	key := DeriveKeyFromPassphrase("nil-pt-test")

	ct, err := Encrypt(nil, key)
	if err != nil {
		t.Fatalf("Encrypt nil: %v", err)
	}

	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	// nil plaintext should round-trip to empty
	if len(pt) != 0 {
		t.Errorf("expected empty plaintext, got %d bytes", len(pt))
	}
}

func TestEncryptDecryptInvalidKeyLengths(t *testing.T) {
	invalidKeys := [][]byte{
		{},
		{0x01},
		make([]byte, 15),
		make([]byte, 16), // AES-128 key, but we require AES-256
		make([]byte, 24), // AES-192 key
		make([]byte, 31),
		make([]byte, 33),
		make([]byte, 64),
	}

	for _, key := range invalidKeys {
		_, err := Encrypt([]byte("data"), key)
		if err == nil {
			t.Errorf("expected Encrypt error for key length %d", len(key))
		}

		_, err = Decrypt([]byte("some-ciphertext-data-long-enough-for-nonce!!"), key)
		if err == nil {
			t.Errorf("expected Decrypt error for key length %d", len(key))
		}
	}
}

func TestEncryptValidKeyLength(t *testing.T) {
	key := make([]byte, keyLen)
	_, err := Encrypt([]byte("valid key test"), key)
	if err != nil {
		t.Errorf("expected no error for valid key length %d: %v", keyLen, err)
	}
}

// ---------------------------------------------------------------------------
// Edge cases: empty passphrase, very long passphrase, binary data
// ---------------------------------------------------------------------------

func TestDeriveKeyFromEmptyPassphrase(t *testing.T) {
	key := DeriveKeyFromPassphrase("")
	if len(key) != keyLen {
		t.Errorf("expected key length %d for empty passphrase, got %d", keyLen, len(key))
	}

	// Should still produce a valid key that works for encryption
	ct, err := Encrypt([]byte("data"), key)
	if err != nil {
		t.Fatalf("Encrypt with empty-passphrase key: %v", err)
	}

	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt with empty-passphrase key: %v", err)
	}

	if string(pt) != "data" {
		t.Errorf("roundtrip with empty passphrase: got %q, want %q", pt, "data")
	}
}

func TestDeriveKeyFromEmptyPassphraseConsistent(t *testing.T) {
	k1 := DeriveKeyFromPassphrase("")
	k2 := DeriveKeyFromPassphrase("")

	if !bytes.Equal(k1, k2) {
		t.Error("empty passphrase should produce consistent keys")
	}
}

func TestDeriveKeyFromVeryLongPassphrase(t *testing.T) {
	// 10KB passphrase
	longPass := strings.Repeat("a very long passphrase segment ", 350)

	key := DeriveKeyFromPassphrase(longPass)
	if len(key) != keyLen {
		t.Errorf("expected key length %d for long passphrase, got %d", keyLen, len(key))
	}

	// Must be deterministic
	key2 := DeriveKeyFromPassphrase(longPass)
	if !bytes.Equal(key, key2) {
		t.Error("long passphrase key derivation should be deterministic")
	}

	// Must produce a working key
	ct, err := Encrypt([]byte("test"), key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(pt) != "test" {
		t.Errorf("roundtrip failed with long passphrase")
	}
}

func TestDeriveKeyFromUnicodePassphrase(t *testing.T) {
	key := DeriveKeyFromPassphrase("pAssw0rd-\u00e9\u00e0\u00fc-\U0001f512")
	if len(key) != keyLen {
		t.Errorf("expected key length %d, got %d", keyLen, len(key))
	}

	// Must differ from ASCII-only passphrase
	asciiKey := DeriveKeyFromPassphrase("pAssw0rd-eau-lock")
	if bytes.Equal(key, asciiKey) {
		t.Error("unicode passphrase should produce different key than similar ASCII passphrase")
	}
}

func TestEncryptDecryptBinaryData(t *testing.T) {
	key := DeriveKeyFromPassphrase("binary-test")

	// Create binary data with all possible byte values
	plaintext := make([]byte, 256)
	for i := 0; i < 256; i++ {
		plaintext[i] = byte(i)
	}

	ct, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt binary: %v", err)
	}

	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt binary: %v", err)
	}

	if !bytes.Equal(pt, plaintext) {
		t.Error("binary data roundtrip mismatch")
	}
}

func TestEncryptDecryptBinaryWithNullBytes(t *testing.T) {
	key := DeriveKeyFromPassphrase("null-bytes")
	plaintext := []byte{0x00, 0x00, 0x00, 0xFF, 0x00, 0xFF, 0x00}

	ct, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(pt, plaintext) {
		t.Errorf("null bytes roundtrip: got %x, want %x", pt, plaintext)
	}
}

func TestEncryptDecryptRepeatedBytes(t *testing.T) {
	key := DeriveKeyFromPassphrase("repeated-bytes")
	plaintext := bytes.Repeat([]byte{0xAB}, 10000)

	ct, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(pt, plaintext) {
		t.Error("repeated bytes roundtrip mismatch")
	}
}

// ---------------------------------------------------------------------------
// Ciphertext properties
// ---------------------------------------------------------------------------

func TestCiphertextContainsNonce(t *testing.T) {
	key := DeriveKeyFromPassphrase("nonce-in-ct")
	plaintext := []byte("check nonce is prepended")

	ct, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// GCM nonce size is 12 bytes. The ciphertext should be at least
	// nonce (12) + plaintext + auth tag (16) bytes long.
	minLen := 12 + len(plaintext) + 16
	if len(ct) < minLen {
		t.Errorf("ciphertext too short: got %d, want at least %d", len(ct), minLen)
	}
}

func TestMultipleEncryptionsSameKeyDifferentOutput(t *testing.T) {
	key := DeriveKeyFromPassphrase("multi-encrypt")
	plaintext := []byte("same data every time")

	seen := make(map[string]bool)
	for i := 0; i < 10; i++ {
		ct, err := Encrypt(plaintext, key)
		if err != nil {
			t.Fatalf("Encrypt #%d: %v", i, err)
		}
		ctStr := string(ct)
		if seen[ctStr] {
			t.Errorf("encryption #%d produced duplicate ciphertext", i)
		}
		seen[ctStr] = true
	}
}

// ---------------------------------------------------------------------------
// DeriveKey (random salt) round-trip
// ---------------------------------------------------------------------------

func TestDeriveKeyRoundtripWithSalt(t *testing.T) {
	passphrase := "roundtrip-salt-test"
	key, salt := DeriveKey(passphrase)

	plaintext := []byte("data encrypted with random-salt key")
	ct, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Simulate: store salt, re-derive key later
	key2 := DeriveKeyWithSalt(passphrase, salt)
	pt, err := Decrypt(ct, key2)
	if err != nil {
		t.Fatalf("Decrypt with re-derived key: %v", err)
	}

	if !bytes.Equal(pt, plaintext) {
		t.Errorf("roundtrip mismatch: got %q, want %q", pt, plaintext)
	}
}

func TestDeriveKeyWrongPassphraseWithCorrectSalt(t *testing.T) {
	key, salt := DeriveKey("correct-pass")

	ct, err := Encrypt([]byte("secret"), key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	wrongKey := DeriveKeyWithSalt("wrong-pass", salt)
	_, err = Decrypt(ct, wrongKey)
	if err == nil {
		t.Error("expected decryption to fail with wrong passphrase but correct salt")
	}
}
