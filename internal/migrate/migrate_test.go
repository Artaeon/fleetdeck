package migrate

import (
	"context"
	"errors"
	"os"
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

// TestRunCreatesPreMigrationSnapshot pins the load-bearing behavior:
// when SkipSnapshot is false (the default), Run MUST create a backup
// record before invoking the command. Without this guarantee, the whole
// point of fleetdeck migrate — "one command to rewind a bad migration"
// — is defeated.
func TestRunCreatesPreMigrationSnapshot(t *testing.T) {
	r, proj := newTestRunner(t, func(ctx context.Context, projectPath, service, command string) ([]byte, error) {
		return []byte("ok"), nil
	})
	r.Cfg.Backup.AutoSnapshot = true

	// backup.CreateBackup reads docker-compose.yml and walks volumes;
	// give it a benign project layout so the snapshot succeeds without
	// a real docker daemon.
	writeMinimalProject(t, proj.ProjectPath)

	res, err := r.Run(context.Background(), proj, Options{
		Command: "npm run migrate",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.SnapshotID == "" {
		t.Fatal("expected a pre-migration snapshot ID, got empty")
	}

	// Confirm the snapshot row actually landed in the backups table —
	// not just the migration history row.
	backups, err := r.Database.ListBackupRecords(proj.ID, 0)
	if err != nil {
		t.Fatalf("ListBackupRecords: %v", err)
	}
	var found bool
	for _, b := range backups {
		if b.ID == res.SnapshotID && b.Trigger == "pre-migration" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a backup record with trigger=pre-migration and matching snapshot ID, got %d records", len(backups))
	}

	// And the migration row must point at the snapshot we just created.
	mig, err := r.Database.ListAppMigrations(proj.ID, 0)
	if err != nil {
		t.Fatalf("ListAppMigrations: %v", err)
	}
	if len(mig) != 1 || mig[0].SnapshotID != res.SnapshotID {
		t.Errorf("migration row SnapshotID = %q, want %q", mig[0].SnapshotID, res.SnapshotID)
	}
}

// TestRunUsesServiceOverride verifies that a non-default --service
// flag actually reaches the Exec stub. Regression pin: if the Options
// plumbing breaks silently, the command would always target 'app' and
// mealtime-style multi-service setups (backend + worker) would run
// migrations against the wrong container.
func TestRunUsesServiceOverride(t *testing.T) {
	var sawService string
	r, proj := newTestRunner(t, func(ctx context.Context, projectPath, service, command string) ([]byte, error) {
		sawService = service
		return nil, nil
	})

	if _, err := r.Run(context.Background(), proj, Options{
		Command:      "rails db:migrate",
		Service:      "backend",
		SkipSnapshot: true,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sawService != "backend" {
		t.Errorf("Exec service = %q, want 'backend'", sawService)
	}
}

// TestApplyDefaultsServiceFallsBackToApp pins the default that every
// fleetdeck-generated profile relies on.
func TestApplyDefaultsServiceFallsBackToApp(t *testing.T) {
	got := applyDefaults(Options{})
	if got.Service != "app" {
		t.Errorf("default service = %q, want 'app'", got.Service)
	}
	if got.Timeout <= 0 {
		t.Errorf("default timeout should be positive, got %s", got.Timeout)
	}
}

// writeMinimalProject lays down the files backup.CreateBackup expects:
// a docker-compose.yml (to satisfy parseComposeFile) and nothing else.
// Volumes + databases lookup will produce zero components and return
// successfully — exactly what we want for a test that only cares about
// the manifest row landing in the DB.
func writeMinimalProject(t *testing.T, dir string) {
	t.Helper()
	compose := `services:
  app:
    image: alpine:3
`
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
}
