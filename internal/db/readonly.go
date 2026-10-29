package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

// OpenReadOnly observes an existing registry without creating it, migrating its
// schema, changing its journal mode or opening the encryption secret store.
func OpenReadOnly(path string) (*DB, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	i, err := os.Stat(path)
	if err != nil || !i.Mode().IsRegular() {
		return nil, fmt.Errorf("an existing registry file is required")
	}
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("mode", "ro")
	q.Set("_query_only", "on")
	q.Set("_busy_timeout", "5000")
	u.RawQuery = q.Encode()
	c, err := sql.Open("sqlite3", u.String())
	if err != nil {
		return nil, err
	}
	if err = c.Ping(); err != nil {
		c.Close()
		return nil, err
	}
	return &DB{conn: c}, nil
}
