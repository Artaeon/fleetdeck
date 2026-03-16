package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Server represents a registered remote server for deployments.
type Server struct {
	ID        string
	Name      string
	Host      string
	Port      string
	User      string
	KeyPath   string
	Status    string // "active", "unreachable", "setup-pending"
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (db *DB) CreateServer(s *Server) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	now := time.Now()
	s.CreatedAt = now
	s.UpdatedAt = now
	if s.Status == "" {
		s.Status = "active"
	}
	if s.Port == "" {
		s.Port = "22"
	}

	_, err := db.conn.Exec(
		`INSERT INTO servers (id, name, host, port, user, key_path, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Name, s.Host, s.Port, s.User, s.KeyPath, s.Status, s.CreatedAt, s.UpdatedAt,
	)
	return err
}

func (db *DB) GetServer(name string) (*Server, error) {
	s := &Server{}
	err := db.conn.QueryRow(
		`SELECT id, name, host, port, user, key_path, status, created_at, updated_at
		 FROM servers WHERE name = ?`, name,
	).Scan(&s.ID, &s.Name, &s.Host, &s.Port, &s.User, &s.KeyPath, &s.Status, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("server %q not found", name)
	}
	return s, err
}

func (db *DB) GetServerByID(id string) (*Server, error) {
	s := &Server{}
	err := db.conn.QueryRow(
		`SELECT id, name, host, port, user, key_path, status, created_at, updated_at
		 FROM servers WHERE id = ?`, id,
	).Scan(&s.ID, &s.Name, &s.Host, &s.Port, &s.User, &s.KeyPath, &s.Status, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("server with id %q not found", id)
	}
	return s, err
}

func (db *DB) ListServers() ([]*Server, error) {
	rows, err := db.conn.Query(
		`SELECT id, name, host, port, user, key_path, status, created_at, updated_at
		 FROM servers ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []*Server
	for rows.Next() {
		s := &Server{}
		if err := rows.Scan(&s.ID, &s.Name, &s.Host, &s.Port, &s.User, &s.KeyPath, &s.Status, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		servers = append(servers, s)
	}
	return servers, rows.Err()
}

func (db *DB) UpdateServerStatus(name, status string) error {
	res, err := db.conn.Exec(
		`UPDATE servers SET status = ?, updated_at = ? WHERE name = ?`,
		status, time.Now(), name,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("server %q not found", name)
	}
	return nil
}

func (db *DB) DeleteServer(name string) error {
	// Check if any projects reference this server
	var count int
	db.conn.QueryRow(`SELECT COUNT(*) FROM projects WHERE server_id = (SELECT id FROM servers WHERE name = ?)`, name).Scan(&count)
	if count > 0 {
		return fmt.Errorf("server %q has %d project(s) assigned; reassign or remove them first", name, count)
	}

	res, err := db.conn.Exec(`DELETE FROM servers WHERE name = ?`, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("server %q not found", name)
	}
	return nil
}
