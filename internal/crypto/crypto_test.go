package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	key, _ := DeriveKey("test-passphrase")
	plaintext := []byte("hello, world!")

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if bytes.Equal(ciphertext, plaintext) {
		t.Error("ciphertext should differ from plaintext")
	}

	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("roundtrip mismatch: got %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	key1, _ := DeriveKey("passphrase-one")
	key2, _ := DeriveKey("passphrase-two")

	ciphertext, err := Encrypt([]byte("secret data"), key1)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	_, err = Decrypt(ciphertext, key2)
	if err == nil {
		t.Error("expected decryption to fail with wrong key")
	}
}

func TestEncryptDecryptEmptyData(t *testing.T) {
	key, _ := DeriveKey("some-passphrase")
	plaintext := []byte{}

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt empty data failed: %v", err)
	}

	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt empty data failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("roundtrip mismatch for empty data: got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptInvalidKeyLength(t *testing.T) {
	shortKey := []byte("too-short")
	_, err := Encrypt([]byte("data"), shortKey)
	if err == nil {
		t.Error("expected error for invalid key length")
	}
}

func TestDecryptInvalidKeyLength(t *testing.T) {
	_, err := Decrypt([]byte("some-ciphertext-data-here!!"), []byte("short"))
	if err == nil {
		t.Error("expected error for invalid key length")
	}
}

func TestDecryptTooShortCiphertext(t *testing.T) {
	key, _ := DeriveKey("passphrase")
	_, err := Decrypt([]byte("short"), key)
	if err == nil {
		t.Error("expected error for too-short ciphertext")
	}
}

func TestDeriveKeyWithSalt(t *testing.T) {
	key1, salt := DeriveKey("my-passphrase")
	key2 := DeriveKeyWithSalt("my-passphrase", salt)

	if !bytes.Equal(key1, key2) {
		t.Error("DeriveKeyWithSalt should produce the same key given same passphrase and salt")
	}
}

func TestDeriveKeyDifferentPassphrases(t *testing.T) {
	key1, _ := DeriveKey("passphrase-a")
	key2, _ := DeriveKey("passphrase-b")

	if bytes.Equal(key1, key2) {
		t.Error("different passphrases should produce different keys")
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	key, _ := DeriveKey("nonce-test")
	plaintext := []byte("same input")

	ct1, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("first Encrypt failed: %v", err)
	}

	ct2, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("second Encrypt failed: %v", err)
	}

	if bytes.Equal(ct1, ct2) {
		t.Error("encrypting the same plaintext twice should produce different ciphertexts (random nonce)")
	}
}
