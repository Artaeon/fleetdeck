package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

func (db *DB) CreateProject(p *Project) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	if p.Status == "" {
		p.Status = "created"
	}

	_, err := db.conn.Exec(
		`INSERT INTO projects (id, name, domain, github_repo, linux_user, project_path, template, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Domain, p.GitHubRepo, p.LinuxUser, p.ProjectPath, p.Template, p.Status, p.CreatedAt, p.UpdatedAt,
	)
	return err
}

func (db *DB) GetProject(name string) (*Project, error) {
	p := &Project{}
	err := db.conn.QueryRow(
		`SELECT id, name, domain, github_repo, linux_user, project_path, template, status, created_at, updated_at
		 FROM projects WHERE name = ?`, name,
	).Scan(&p.ID, &p.Name, &p.Domain, &p.GitHubRepo, &p.LinuxUser, &p.ProjectPath, &p.Template, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("project %q not found", name)
	}
	return p, err
}

func (db *DB) GetProjectByID(id string) (*Project, error) {
	p := &Project{}
	err := db.conn.QueryRow(
		`SELECT id, name, domain, github_repo, linux_user, project_path, template, status, created_at, updated_at
		 FROM projects WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &p.Domain, &p.GitHubRepo, &p.LinuxUser, &p.ProjectPath, &p.Template, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("project with id %q not found", id)
	}
	return p, err
}

func (db *DB) ListProjects() ([]*Project, error) {
	rows, err := db.conn.Query(
		`SELECT id, name, domain, github_repo, linux_user, project_path, template, status, created_at, updated_at
		 FROM projects ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []*Project
	for rows.Next() {
		p := &Project{}
		if err := rows.Scan(&p.ID, &p.Name, &p.Domain, &p.GitHubRepo, &p.LinuxUser, &p.ProjectPath, &p.Template, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

func (db *DB) UpdateProjectStatus(name, status string) error {
	res, err := db.conn.Exec(
		`UPDATE projects SET status = ?, updated_at = ? WHERE name = ?`,
		status, time.Now(), name,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("project %q not found", name)
	}
	return nil
}

func (db *DB) DeleteProject(name string) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var id string
	if err := tx.QueryRow(`SELECT id FROM projects WHERE name = ?`, name).Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("project %q not found", name)
		}
		return err
	}

	if _, err := tx.Exec(`DELETE FROM secrets WHERE project_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM deployments WHERE project_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM projects WHERE id = ?`, id); err != nil {
		return err
	}

	return tx.Commit()
}
