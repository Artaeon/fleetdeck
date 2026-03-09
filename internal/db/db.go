package db

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn          *sql.DB
	encryptionKey []byte // optional 32-byte AES-256 key for secret encryption
}

// SetEncryptionKey configures an AES-256 key for encrypting secret values
// at rest. Pass nil to disable encryption (plaintext storage).
func (db *DB) SetEncryptionKey(key []byte) {
	db.encryptionKey = key
}

type Project struct {
	ID          string
	Name        string
	Domain      string
	GitHubRepo  string
	LinuxUser   string
	ProjectPath string
	Template    string
	Status      string
	Source      string // "created", "imported", "discovered"
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type BackupRecord struct {
	ID          string
	ProjectID   string
	Type        string // "manual", "snapshot", "scheduled"
	Trigger     string // "user", "pre-stop", "pre-restart", "pre-destroy"
	Path        string
	SizeBytes   int64
	CreatedAt   time.Time
}

type Deployment struct {
	ID         string
	ProjectID  string
	CommitSHA  string
	Status     string
	StartedAt  time.Time
	FinishedAt *time.Time
	Log        string
}

type Secret struct {
	ID        string
	ProjectID string
	Key       string
	Value     string
}

func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, err
	}

	if err := conn.Ping(); err != nil {
		return nil, err
	}

	db := &DB{conn: conn}

	// Run integrity check on startup
	if err := db.checkIntegrity(); err != nil {
		log.Printf("WARNING: database integrity check failed: %v", err)
		log.Printf("Attempting WAL recovery...")
		if walErr := db.walCheckpoint(); walErr != nil {
			log.Printf("WAL checkpoint failed: %v", walErr)
		} else {
			// Re-check after WAL recovery
			if err := db.checkIntegrity(); err != nil {
				log.Printf("WARNING: database still has integrity issues after WAL recovery: %v", err)
			} else {
				log.Printf("Database integrity restored after WAL recovery")
			}
		}
	}

	if err := db.migrate(); err != nil {
		return nil, err
	}

	return db, nil
}

func (db *DB) Close() error {
	// Checkpoint WAL on close for a clean shutdown
	if err := db.walCheckpoint(); err != nil {
		log.Printf("WAL checkpoint on close failed: %v", err)
	}
	return db.conn.Close()
}

// checkIntegrity runs PRAGMA integrity_check and returns an error if the
// database reports any issues.
func (db *DB) checkIntegrity() error {
	var result string
	if err := db.conn.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("integrity_check query failed: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("integrity_check returned: %s", result)
	}
	return nil
}

// walCheckpoint forces a WAL checkpoint with TRUNCATE mode, which writes
// all WAL frames back to the database file and truncates the WAL.
func (db *DB) walCheckpoint() error {
	_, err := db.conn.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}

// Snapshot creates a consistent copy of the database at the given path using
// SQLite's VACUUM INTO, which works correctly even in WAL mode.
func (db *DB) Snapshot(destPath string) error {
	_, err := db.conn.Exec(fmt.Sprintf(`VACUUM INTO '%s'`, destPath))
	return err
}

func (db *DB) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS projects (
			id TEXT PRIMARY KEY,
			name TEXT UNIQUE NOT NULL,
			domain TEXT NOT NULL,
			github_repo TEXT,
			linux_user TEXT NOT NULL,
			project_path TEXT NOT NULL,
			template TEXT DEFAULT 'custom',
			status TEXT DEFAULT 'created',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS deployments (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(id),
			commit_sha TEXT,
			status TEXT DEFAULT 'pending',
			started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			finished_at DATETIME,
			log TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS secrets (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(id),
			key TEXT NOT NULL,
			value TEXT NOT NULL,
			UNIQUE(project_id, key)
		)`,
		`CREATE TABLE IF NOT EXISTS backups (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(id),
			type TEXT NOT NULL DEFAULT 'manual',
			trigger_name TEXT DEFAULT 'user',
			path TEXT NOT NULL,
			size_bytes INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, m := range migrations {
		if _, err := db.conn.Exec(m); err != nil {
			return err
		}
	}

	// Column additions for existing databases (safe to re-run)
	alterStatements := []string{
		`ALTER TABLE projects ADD COLUMN source TEXT DEFAULT 'created'`,
	}
	for _, stmt := range alterStatements {
		db.conn.Exec(stmt) // ignore "duplicate column" errors
	}

	return nil
}
