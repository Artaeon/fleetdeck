package db

import (
	"database/sql"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
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
	if err := db.migrate(); err != nil {
		return nil, err
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
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
