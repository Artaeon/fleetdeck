package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

const (
	// keyLen is the required AES-256 key length in bytes.
	keyLen = 32
	// saltLen is the length of the PBKDF2 salt.
	saltLen = 16
	// pbkdf2Iterations is the number of PBKDF2 iterations.
	pbkdf2Iterations = 100_000
)

// DeriveKey derives a 32-byte AES-256 key from a passphrase using PBKDF2
// with a random salt. It returns the key and the salt used.
func DeriveKey(passphrase string) (key []byte, salt []byte) {
	salt = make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	key = pbkdf2.Key([]byte(passphrase), salt, pbkdf2Iterations, keyLen, sha256.New)
	return key, salt
}

// DeriveKeyWithSalt derives a 32-byte AES-256 key from a passphrase and
// an existing salt. This is used when decrypting data where the salt is
// already known.
func DeriveKeyWithSalt(passphrase string, salt []byte) []byte {
	return pbkdf2.Key([]byte(passphrase), salt, pbkdf2Iterations, keyLen, sha256.New)
}

// fleetdeckSalt is a fixed application-specific salt used for deterministic
// key derivation from a passphrase. This allows the same key to be derived
// on every application start without storing additional state.
var fleetdeckSalt = []byte("fleetdeck-secret-encryption-v1")

// DeriveKeyFromPassphrase derives a deterministic 32-byte AES-256 key from a
// passphrase using PBKDF2 with a fixed application-specific salt. Use this
// when the same key must be derived consistently across application restarts.
func DeriveKeyFromPassphrase(passphrase string) []byte {
	return pbkdf2.Key([]byte(passphrase), fleetdeckSalt, pbkdf2Iterations, keyLen, sha256.New)
}

// Encrypt encrypts plaintext using AES-256-GCM with the provided 32-byte key.
// The returned ciphertext has the nonce prepended: [nonce | encrypted data + tag].
func Encrypt(plaintext []byte, key []byte) ([]byte, error) {
	if len(key) != keyLen {
		return nil, fmt.Errorf("invalid key length: got %d, want %d", len(key), keyLen)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	// Seal appends the encrypted data to nonce, so result is [nonce | ciphertext + tag]
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext that was encrypted with Encrypt.
// It expects the nonce prepended to the ciphertext: [nonce | encrypted data + tag].
func Decrypt(ciphertext []byte, key []byte) ([]byte, error) {
	if len(key) != keyLen {
		return nil, fmt.Errorf("invalid key length: got %d, want %d", len(key), keyLen)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, encrypted := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}
