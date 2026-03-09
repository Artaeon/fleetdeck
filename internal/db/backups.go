package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

func (db *DB) CreateBackupRecord(b *BackupRecord) error {
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	b.CreatedAt = time.Now()
	if b.Type == "" {
		b.Type = "manual"
	}
	if b.Trigger == "" {
		b.Trigger = "user"
	}

	_, err := db.conn.Exec(
		`INSERT INTO backups (id, project_id, type, trigger_name, path, size_bytes, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		b.ID, b.ProjectID, b.Type, b.Trigger, b.Path, b.SizeBytes, b.CreatedAt,
	)
	return err
}

func (db *DB) GetBackupRecord(id string) (*BackupRecord, error) {
	b := &BackupRecord{}
	err := db.conn.QueryRow(
		`SELECT id, project_id, type, trigger_name, path, size_bytes, created_at
		 FROM backups WHERE id = ?`, id,
	).Scan(&b.ID, &b.ProjectID, &b.Type, &b.Trigger, &b.Path, &b.SizeBytes, &b.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("backup %q not found", id)
	}
	return b, err
}

func (db *DB) ListBackupRecords(projectID string, limit int) ([]*BackupRecord, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := db.conn.Query(
		`SELECT id, project_id, type, trigger_name, path, size_bytes, created_at
		 FROM backups WHERE project_id = ? ORDER BY created_at DESC LIMIT ?`,
		projectID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backups []*BackupRecord
	for rows.Next() {
		b := &BackupRecord{}
		if err := rows.Scan(&b.ID, &b.ProjectID, &b.Type, &b.Trigger, &b.Path, &b.SizeBytes, &b.CreatedAt); err != nil {
			return nil, err
		}
		backups = append(backups, b)
	}
	return backups, rows.Err()
}

func (db *DB) DeleteBackupRecord(id string) error {
	_, err := db.conn.Exec(`DELETE FROM backups WHERE id = ?`, id)
	return err
}

func (db *DB) CountBackupsByType(projectID, backupType string) (int, error) {
	var count int
	err := db.conn.QueryRow(
		`SELECT COUNT(*) FROM backups WHERE project_id = ? AND type = ?`,
		projectID, backupType,
	).Scan(&count)
	return count, err
}

func (db *DB) GetOldestBackups(projectID, backupType string, limit int) ([]*BackupRecord, error) {
	rows, err := db.conn.Query(
		`SELECT id, project_id, type, trigger_name, path, size_bytes, created_at
		 FROM backups WHERE project_id = ? AND type = ? ORDER BY created_at ASC LIMIT ?`,
		projectID, backupType, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backups []*BackupRecord
	for rows.Next() {
		b := &BackupRecord{}
		if err := rows.Scan(&b.ID, &b.ProjectID, &b.Type, &b.Trigger, &b.Path, &b.SizeBytes, &b.CreatedAt); err != nil {
			return nil, err
		}
		backups = append(backups, b)
	}
	return backups, rows.Err()
}

func (db *DB) GetExpiredBackups(projectID string, beforeDate time.Time) ([]*BackupRecord, error) {
	rows, err := db.conn.Query(
		`SELECT id, project_id, type, trigger_name, path, size_bytes, created_at
		 FROM backups WHERE project_id = ? AND created_at < ?
		 ORDER BY created_at ASC`,
		projectID, beforeDate,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backups []*BackupRecord
	for rows.Next() {
		b := &BackupRecord{}
		if err := rows.Scan(&b.ID, &b.ProjectID, &b.Type, &b.Trigger, &b.Path, &b.SizeBytes, &b.CreatedAt); err != nil {
			return nil, err
		}
		backups = append(backups, b)
	}
	return backups, rows.Err()
}

func (db *DB) DeleteBackupsForProject(projectID string) error {
	_, err := db.conn.Exec(`DELETE FROM backups WHERE project_id = ?`, projectID)
	return err
}
