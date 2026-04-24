package migrate

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/db"
)

// newTestRunner builds a Runner backed by a temp SQLite DB and a stub
// Exec so tests drive the success/failure paths without shelling out to
// docker. The returned project row is already inserted and ready to use.
func newTestRunner(t *testing.T, stub func(ctx context.Context, projectPath, service, command string) ([]byte, error)) (*Runner, *db.Project) {
	t.Helper()
	dir := t.TempDir()

	cfg := config.DefaultConfig()
	cfg.Server.BasePath = dir
	cfg.Backup.BasePath = filepath.Join(dir, "backups")
	cfg.Backup.AutoSnapshot = false // tests drive snapshotting explicitly

	database, err := db.Open(cfg.DBPath())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	proj := &db.Project{
		ID:          "p-test",
		Name:        "testproj",
		Domain:      "test.example.com",
		LinuxUser:   "fleetdeck-test",
		ProjectPath: dir,
	}
	if err := database.CreateProject(proj); err != nil {
		t.Fatalf("create project: %v", err)
	}

	r := &Runner{Cfg: cfg, Database: database, Exec: stub}
	return r, proj
}

// TestRunSuccessRecordsMigration verifies a passing command produces a
// migration row in status=succeeded with output captured.
func TestRunSuccessRecordsMigration(t *testing.T) {
	r, proj := newTestRunner(t, func(ctx context.Context, projectPath, service, command string) ([]byte, error) {
		return []byte("migrated 3 files\n"), nil
	})

	res, err := r.Run(context.Background(), proj, Options{
		Command:      "npm run migrate",
		SkipSnapshot: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Output == "" {
		t.Error("expected captured output")
	}

	hist, err := r.Database.ListAppMigrations(proj.ID, 0)
	if err != nil {
		t.Fatalf("ListAppMigrations: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("expected 1 migration row, got %d", len(hist))
	}
	if hist[0].Status != "succeeded" {
		t.Errorf("status = %q, want succeeded", hist[0].Status)
	}
	if hist[0].Command != "npm run migrate" {
		t.Errorf("command = %q, want 'npm run migrate'", hist[0].Command)
	}
}

// TestRunFailureRecordsFailedStatus verifies a non-zero command leaves
// status=failed so the operator can diagnose via `fleetdeck migrate history`.
func TestRunFailureRecordsFailedStatus(t *testing.T) {
	r, proj := newTestRunner(t, func(ctx context.Context, projectPath, service, command string) ([]byte, error) {
		return []byte("syntax error near line 42\n"), errors.New("exit 1")
	})

	_, err := r.Run(context.Background(), proj, Options{
		Command:      "npm run migrate",
		SkipSnapshot: true,
	})
	if err == nil {
		t.Fatal("expected error for failing command")
	}

	hist, err := r.Database.ListAppMigrations(proj.ID, 0)
	if err != nil {
		t.Fatalf("ListAppMigrations: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("expected 1 migration row, got %d", len(hist))
	}
	if hist[0].Status != "failed" {
		t.Errorf("status = %q, want failed", hist[0].Status)
	}
}

// TestRunCapturesTimeout verifies the default timeout propagates and
// errors out rather than hanging. Uses a short explicit timeout.
func TestRunCapturesTimeout(t *testing.T) {
	r, proj := newTestRunner(t, func(ctx context.Context, projectPath, service, command string) ([]byte, error) {
		<-ctx.Done() // block until context cancels
		return nil, ctx.Err()
	})

	start := time.Now()
	_, err := r.Run(context.Background(), proj, Options{
		Command:      "sleep 600",
		Timeout:      80 * time.Millisecond,
		SkipSnapshot: true,
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed > 2*time.Second {
		t.Errorf("timeout should have fired quickly, took %s", elapsed)
	}
}

// TestTruncate keeps the output column from blowing up the SQLite DB
// with megabytes of SQL echo.
func TestTruncate(t *testing.T) {
	short := truncate("hello", 100)
	if short != "hello" {
		t.Errorf("short string should pass through, got %q", short)
	}
	big := truncate(string(make([]byte, 10000)), 100)
	if len(big) <= 100 || len(big) > 200 {
		t.Errorf("truncated size = %d, want just over 100", len(big))
	}
}
