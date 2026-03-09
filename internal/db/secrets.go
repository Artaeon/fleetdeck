package db

import (
	"database/sql"
	"encoding/base64"
	"fmt"

	"github.com/fleetdeck/fleetdeck/internal/crypto"
	"github.com/google/uuid"
)

// encryptedPrefix is prepended to encrypted values stored in the database
// so they can be distinguished from plaintext values during reads.
const encryptedPrefix = "enc:"

// SetSecret creates or updates a secret for a project. If an encryption key
// is configured, the value is encrypted before storage. Otherwise it is
// stored in plaintext.
func (db *DB) SetSecret(projectID, key, value string) error {
	storedValue, err := db.encryptValue(value)
	if err != nil {
		return fmt.Errorf("encrypting secret: %w", err)
	}

	id := uuid.New().String()
	_, err = db.conn.Exec(
		`INSERT INTO secrets (id, project_id, key, value) VALUES (?, ?, ?, ?)
		 ON CONFLICT(project_id, key) DO UPDATE SET value = excluded.value`,
		id, projectID, key, storedValue,
	)
	return err
}

// GetSecret retrieves a single secret by project ID and key. If the value
// is encrypted, it is decrypted before being returned.
func (db *DB) GetSecret(projectID, key string) (*Secret, error) {
	s := &Secret{}
	err := db.conn.QueryRow(
		`SELECT id, project_id, key, value FROM secrets WHERE project_id = ? AND key = ?`,
		projectID, key,
	).Scan(&s.ID, &s.ProjectID, &s.Key, &s.Value)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("secret %q not found for project %q", key, projectID)
	}
	if err != nil {
		return nil, err
	}

	decrypted, err := db.decryptValue(s.Value)
	if err != nil {
		return nil, fmt.Errorf("decrypting secret %q: %w", key, err)
	}
	s.Value = decrypted

	return s, nil
}

// ListSecrets returns all secrets for a project. Encrypted values are
// decrypted before being returned.
func (db *DB) ListSecrets(projectID string) ([]*Secret, error) {
	rows, err := db.conn.Query(
		`SELECT id, project_id, key, value FROM secrets WHERE project_id = ? ORDER BY key`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var secrets []*Secret
	for rows.Next() {
		s := &Secret{}
		if err := rows.Scan(&s.ID, &s.ProjectID, &s.Key, &s.Value); err != nil {
			return nil, err
		}
		decrypted, err := db.decryptValue(s.Value)
		if err != nil {
			return nil, fmt.Errorf("decrypting secret %q: %w", s.Key, err)
		}
		s.Value = decrypted
		secrets = append(secrets, s)
	}
	return secrets, rows.Err()
}

// DeleteSecret removes a secret by project ID and key.
func (db *DB) DeleteSecret(projectID, key string) error {
	res, err := db.conn.Exec(
		`DELETE FROM secrets WHERE project_id = ? AND key = ?`,
		projectID, key,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("secret %q not found for project %q", key, projectID)
	}
	return nil
}

// encryptValue encrypts a plaintext value if an encryption key is set.
// Returns the value with an "enc:" prefix and base64-encoded ciphertext.
// If no key is set, returns the value unchanged.
func (db *DB) encryptValue(value string) (string, error) {
	if len(db.encryptionKey) == 0 {
		return value, nil
	}

	ciphertext, err := crypto.Encrypt([]byte(value), db.encryptionKey)
	if err != nil {
		return "", err
	}

	return encryptedPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptValue decrypts a stored value if it has the "enc:" prefix.
// Plaintext values (without the prefix) are returned unchanged, providing
// backwards compatibility with data stored before encryption was enabled.
func (db *DB) decryptValue(stored string) (string, error) {
	if len(stored) <= len(encryptedPrefix) || stored[:len(encryptedPrefix)] != encryptedPrefix {
		// Not encrypted — return as-is for backwards compatibility
		return stored, nil
	}

	if len(db.encryptionKey) == 0 {
		return "", fmt.Errorf("value is encrypted but no encryption key is configured")
	}

	encoded := stored[len(encryptedPrefix):]
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decoding encrypted value: %w", err)
	}

	plaintext, err := crypto.Decrypt(ciphertext, db.encryptionKey)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
