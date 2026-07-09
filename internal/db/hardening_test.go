package db

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDeleteProjectWithMigrationHistory is a regression test: app_migrations
// references projects(id) without ON DELETE CASCADE and foreign keys are
// enforced, so DeleteProject must clear app_migrations rows itself. Before the
// fix, deleting any project that had run a migration failed with a FOREIGN KEY
// constraint error, making it undeletable.
func TestDeleteProjectWithMigrationHistory(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		Name:        "migrated",
		Domain:      "migrated.example.com",
		LinuxUser:   "fleetdeck-migrated",
		ProjectPath: "/opt/fleetdeck/migrated",
		Template:    "node",
		Source:      "created",
	}
	if err := db.CreateProject(p); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := db.CreateAppMigration(&AppMigration{
		ID:        "mig-1",
		ProjectID: p.ID,
		Command:   "npm run migrate",
	}); err != nil {
		t.Fatalf("create migration: %v", err)
	}

	if err := db.DeleteProject("migrated"); err != nil {
		t.Fatalf("DeleteProject with migration history should succeed, got: %v", err)
	}
	if _, err := db.GetProject("migrated"); err == nil {
		t.Error("expected project to be gone after delete")
	}
}

// TestOpenRestrictsDBPermissions verifies the database file is created (or
// tightened) to owner-only, since it can hold deployment logs and plaintext
// secrets.
func TestOpenRestrictsDBPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "perm.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("expected db file to be owner-only, got %o", perm)
	}
}
