package db

import (
	"database/sql"
	"fmt"
	"time"
)

// AppMigration records a single application-level migration run. This is
// NOT about fleetdeck's own database schema — it tracks commands the
// operator ran inside the project container (e.g. "npm run migrate") so
// there is an audit trail of what was applied and when, and a pointer to
// the pre-migration snapshot so rollback is one command.
type AppMigration struct {
	ID         string
	ProjectID  string
	Command    string
	SnapshotID string // backup record ID taken before the migration
	Status     string // "running", "succeeded", "failed"
	Output     string // captured stdout+stderr (truncated by caller if needed)
	StartedAt  time.Time
	FinishedAt sql.NullTime
}

// CreateAppMigration inserts a new migration row in the "running" state.
// Call MarkAppMigration once the command finishes to transition to
// "succeeded" or "failed".
func (db *DB) CreateAppMigration(m *AppMigration) error {
	if m.Status == "" {
		m.Status = "running"
	}
	if m.StartedAt.IsZero() {
		m.StartedAt = time.Now().UTC()
	}
	_, err := db.conn.Exec(
		`INSERT INTO app_migrations (id, project_id, command, snapshot_id, status, output, started_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.ProjectID, m.Command, m.SnapshotID, m.Status, m.Output, m.StartedAt,
	)
	return err
}

// MarkAppMigration transitions a running migration to its terminal state
// and records output + finish time.
func (db *DB) MarkAppMigration(id, status, output string) error {
	if status != "succeeded" && status != "failed" {
		return fmt.Errorf("invalid terminal status %q (want succeeded|failed)", status)
	}
	_, err := db.conn.Exec(
		`UPDATE app_migrations SET status = ?, output = ?, finished_at = ? WHERE id = ?`,
		status, output, time.Now().UTC(), id,
	)
	return err
}

// ListAppMigrations returns migrations for a project, newest first.
// limit=0 returns all rows.
func (db *DB) ListAppMigrations(projectID string, limit int) ([]*AppMigration, error) {
	q := `SELECT id, project_id, command, COALESCE(snapshot_id, ''), status,
	             COALESCE(output, ''), started_at, finished_at
	      FROM app_migrations WHERE project_id = ? ORDER BY started_at DESC`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := db.conn.Query(q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*AppMigration
	for rows.Next() {
		var m AppMigration
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.Command, &m.SnapshotID,
			&m.Status, &m.Output, &m.StartedAt, &m.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}
