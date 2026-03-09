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
