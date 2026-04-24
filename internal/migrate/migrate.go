// Package migrate executes application-level database migrations for a
// fleetdeck project and records the outcome.
//
// "Application-level" here means the migrations defined by the project
// itself — `npm run migrate`, `rails db:migrate`, `flyway migrate`, etc.
// Fleetdeck does not invent a migration format; it wraps whatever the
// project already uses so there is a single entry point that:
//
//  1. snapshots the DB before running (rollback is one command)
//  2. executes the command inside the 'app' service container
//  3. captures the output
//  4. records the result in SQLite with a pointer to the snapshot
//
// The "snapshot before run" step is the load-bearing bit for mealtime
// and similar apps — when a migration half-applies and leaves the DB in
// an inconsistent state, you want `fleetdeck rollback --latest` to
// restore a known-good dump, not to tail the logs and guess.
package migrate

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/fleetdeck/fleetdeck/internal/backup"
	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/db"
)

// Options configures a single migration run.
type Options struct {
	// Service is the docker compose service that the migration command
	// runs inside. Defaults to "app" — the convention used by every
	// fleetdeck-generated profile.
	Service string

	// Command is the shell command executed inside the container. Treated
	// as a single argument to `sh -c`, so operators can write pipelines
	// and && chains naturally.
	Command string

	// SkipSnapshot disables the automatic pre-migration DB snapshot. Off
	// by default — keeping the snapshot is the whole point of going
	// through this codepath instead of plain `docker compose exec`.
	SkipSnapshot bool

	// Timeout caps the migration command's wall clock. Defaults to 10 min
	// — long enough for realistic ALTER TABLE / data backfill work but
	// short enough that a stuck migration doesn't hang CI forever.
	Timeout time.Duration
}

// Result captures the outcome of a Run call.
type Result struct {
	MigrationID string
	SnapshotID  string // empty if SkipSnapshot was set or snapshot failed
	Output      string // combined stdout+stderr from the command
	Duration    time.Duration
}

// Runner encapsulates the collaborators Run needs. Accepting them via
// the constructor rather than as package globals keeps Run testable —
// callers can plug in a stub Database / execer in unit tests.
type Runner struct {
	Cfg      *config.Config
	Database *db.DB

	// Exec is the function that runs `docker compose exec -T <svc> sh -c <cmd>`
	// with a context-scoped timeout. Exposed as a field so tests can
	// substitute a fake that never shells out.
	Exec func(ctx context.Context, projectPath, service, command string) ([]byte, error)
}

// New builds a Runner wired to the real docker compose exec implementation.
func New(cfg *config.Config, database *db.DB) *Runner {
	return &Runner{
		Cfg:      cfg,
		Database: database,
		Exec:     defaultExec,
	}
}

// Run takes a pre-migration snapshot (unless disabled), executes the
// command inside the target container, and records the outcome in the
// database. It returns the Result on success OR the Result with output
// populated plus a non-nil error on failure — the operator usually wants
// to see the output from a failed migration.
func (r *Runner) Run(ctx context.Context, project *db.Project, opts Options) (*Result, error) {
	opts = applyDefaults(opts)

	res := &Result{
		MigrationID: uuid.New().String(),
	}

	// 1. Snapshot the current state so a botched migration is one
	//    `fleetdeck rollback` away from recovery. We take the snapshot
	//    BEFORE recording the migration row so that if snapshotting
	//    itself fails, there is no orphan "running" row in the DB.
	if !opts.SkipSnapshot {
		snap, err := backup.CreateBackup(r.Cfg, r.Database, project, "snapshot", "pre-migration", backup.Options{})
		if err != nil {
			return res, fmt.Errorf("pre-migration snapshot: %w", err)
		}
		res.SnapshotID = snap.ID
	}

	// 2. Record the migration as "running". If the process crashes or the
	//    operator Ctrl+Cs before the command completes, the row remains
	//    in "running" — that is a feature: it tells the operator the DB
	//    state is indeterminate and they should restore the snapshot.
	record := &db.AppMigration{
		ID:         res.MigrationID,
		ProjectID:  project.ID,
		Command:    opts.Command,
		SnapshotID: res.SnapshotID,
		Status:     "running",
	}
	if err := r.Database.CreateAppMigration(record); err != nil {
		return res, fmt.Errorf("recording migration: %w", err)
	}

	// 3. Execute. The context timeout applies to the whole command.
	start := time.Now()
	execCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	out, err := r.Exec(execCtx, project.ProjectPath, opts.Service, opts.Command)
	res.Output = string(out)
	res.Duration = time.Since(start)

	status := "succeeded"
	if err != nil {
		status = "failed"
	}
	if markErr := r.Database.MarkAppMigration(record.ID, status, truncate(res.Output, 64*1024)); markErr != nil {
		// Record-update failure is non-fatal — log-worthy but the real
		// signal is whether the migration itself worked.
		err = fmt.Errorf("%w (and failed to update migration record: %v)", err, markErr)
	}
	if err != nil {
		return res, fmt.Errorf("migration command failed after %s: %w", res.Duration.Round(time.Millisecond), err)
	}
	return res, nil
}

// applyDefaults fills in the options that weren't explicitly set.
func applyDefaults(o Options) Options {
	if o.Service == "" {
		o.Service = "app"
	}
	if o.Timeout <= 0 {
		o.Timeout = 10 * time.Minute
	}
	return o
}

// truncate caps a string at max bytes, appending a note if it was cut.
// Migration output is often megabytes of SQL echo; we don't want to bloat
// the SQLite DB with it.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("\n... (output truncated to %d bytes)", max)
}

// defaultExec runs `docker compose exec -T <service> sh -c <command>` in
// the given project directory. Captures both stdout and stderr.
func defaultExec(ctx context.Context, projectPath, service, command string) ([]byte, error) {
	cmd := exec.CommandContext(ctx,
		"docker", "compose", "exec", "-T", service,
		"sh", "-c", command,
	)
	cmd.Dir = projectPath
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("migration timed out: %w", ctx.Err())
	}
	if err != nil {
		// Include the tail of output in the error so callers who don't
		// inspect Result.Output still see why it failed.
		tail := strings.TrimSpace(string(out))
		if len(tail) > 400 {
			tail = "..." + tail[len(tail)-400:]
		}
		return out, fmt.Errorf("exec: %w: %s", err, tail)
	}
	return out, nil
}
