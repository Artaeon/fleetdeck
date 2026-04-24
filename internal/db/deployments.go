package db

import (
	"time"

	"github.com/google/uuid"
)

func (db *DB) CreateDeployment(d *Deployment) error {
	if d.ID == "" {
		d.ID = uuid.New().String()
	}
	d.StartedAt = time.Now()
	if d.Status == "" {
		d.Status = "pending"
	}

	_, err := db.conn.Exec(
		`INSERT INTO deployments (id, project_id, commit_sha, status, started_at, finished_at, log)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.ProjectID, d.CommitSHA, d.Status, d.StartedAt, d.FinishedAt, d.Log,
	)
	return err
}

func (db *DB) UpdateDeployment(id, status, log string) error {
	now := time.Now()
	_, err := db.conn.Exec(
		`UPDATE deployments SET status = ?, finished_at = ?, log = ? WHERE id = ?`,
		status, now, log, id,
	)
	return err
}

func (db *DB) ListDeployments(projectID string, limit int) ([]*Deployment, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := db.conn.Query(
		`SELECT id, project_id, commit_sha, status, started_at, finished_at, log
		 FROM deployments WHERE project_id = ? ORDER BY started_at DESC LIMIT ?`,
		projectID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deployments []*Deployment
	for rows.Next() {
		d := &Deployment{}
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.CommitSHA, &d.Status, &d.StartedAt, &d.FinishedAt, &d.Log); err != nil {
			return nil, err
		}
		deployments = append(deployments, d)
	}
	return deployments, rows.Err()
}

// PruneDeployments keeps the newest `keep` deployment rows per project
// and deletes everything older. Returns the number of rows removed so
// callers can surface the count or audit-log it.
//
// A busy project on CI runs 10+ deploys a day; the 'log' column often
// holds 5-50 KB of compose output. Without pruning, the deployments
// table grows unboundedly — a year of daily mealtime deploys would add
// hundreds of MB of SQLite data and drag every SELECT slower every week.
// Call this from a retention sweep, scheduled backup job, or an
// operator CLI; we don't prune on every Insert because the 'DELETE …
// WHERE id NOT IN' costs more than we save for individual writes.
//
// keep <= 0 is treated as 'no-op' — callers disable pruning by passing
// 0, which is what we want in unit tests that create/list exactly N
// records and expect all of them to survive.
func (db *DB) PruneDeployments(projectID string, keep int) (int64, error) {
	if keep <= 0 {
		return 0, nil
	}
	// Correlated subquery: delete rows whose id is NOT in the most
	// recent `keep` ids for this project. Using started_at DESC ties
	// the pruning to 'newest by start' rather than id insertion order,
	// which matters if ids are ever set by the caller out of sequence.
	result, err := db.conn.Exec(
		`DELETE FROM deployments
		 WHERE project_id = ?
		   AND id NOT IN (
		     SELECT id FROM deployments
		     WHERE project_id = ?
		     ORDER BY started_at DESC
		     LIMIT ?
		   )`,
		projectID, projectID, keep,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// PruneAllDeployments applies PruneDeployments to every project in the
// DB and sums the rows removed. Intended for a periodic retention
// sweep (CLI or scheduled job) rather than per-deploy cleanup.
func (db *DB) PruneAllDeployments(keep int) (int64, error) {
	if keep <= 0 {
		return 0, nil
	}
	projects, err := db.ListProjects()
	if err != nil {
		return 0, err
	}
	var total int64
	for _, p := range projects {
		n, err := db.PruneDeployments(p.ID, keep)
		if err != nil {
			// Continue through the list even if one project errors —
			// the other projects shouldn't be punished for one bad
			// row. Caller decides whether to care about partial progress.
			return total, err
		}
		total += n
	}
	return total, nil
}
