package cmd

import (
	"fmt"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/migrate"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run and track application database migrations",
	Long: `Run, inspect, and roll back application-level migrations.

Unlike plain 'docker compose exec', 'fleetdeck migrate run' takes a
pre-migration snapshot automatically, records the command in the
project database, and gives you a single-command rollback to the
pre-migration state via 'fleetdeck migrate rollback'.`,
}

var migrateRunCmd = &cobra.Command{
	Use:   "run <project-name>",
	Short: "Run a migration command inside the project container",
	Long: `Snapshots the project DB, runs the migration command inside
the target docker compose service, and records the outcome.

Examples:
  fleetdeck migrate run mealtime --command "npm run migrate"
  fleetdeck migrate run klassenpilot --command "rails db:migrate" --service backend
  fleetdeck migrate run trippin --command "npx prisma migrate deploy" --timeout 15m`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()
		p, err := d.GetProject(args[0])
		if err != nil {
			return err
		}

		command, _ := cmd.Flags().GetString("command")
		service, _ := cmd.Flags().GetString("service")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		skipSnap, _ := cmd.Flags().GetBool("skip-snapshot")

		if command == "" {
			return fmt.Errorf("--command is required (e.g. --command=\"npm run migrate\")")
		}

		ui.Info("Project: %s", ui.Bold(p.Name))
		ui.Info("Service: %s", service)
		ui.Info("Command: %s", command)
		if skipSnap {
			ui.Warn("Skipping pre-migration snapshot (no rollback will be possible)")
		}
		fmt.Println()

		runner := migrate.New(cfg, d)
		res, err := runner.Run(cmd.Context(), p, migrate.Options{
			Service:      service,
			Command:      command,
			SkipSnapshot: skipSnap,
			Timeout:      timeout,
		})
		if err != nil {
			if res != nil && res.Output != "" {
				fmt.Println(res.Output)
			}
			audit.Log("migrate.run", p.Name, err.Error(), false)
			return err
		}

		audit.Log("migrate.run", p.Name,
			fmt.Sprintf("id=%s snapshot=%s duration=%s", shortID(res.MigrationID), shortID(res.SnapshotID), res.Duration.Round(time.Millisecond)),
			true)

		if res.Output != "" {
			fmt.Println(res.Output)
		}
		ui.Success("Migration succeeded in %s", res.Duration.Round(time.Millisecond))
		if res.SnapshotID != "" {
			ui.Info("Pre-migration snapshot: %s", shortID(res.SnapshotID))
			ui.Info("Roll back with: fleetdeck migrate rollback %s", p.Name)
		}
		return nil
	},
}

var migrateHistoryCmd = &cobra.Command{
	Use:   "history <project-name>",
	Short: "Show the migration history for a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()
		p, err := d.GetProject(args[0])
		if err != nil {
			return err
		}
		limit, _ := cmd.Flags().GetInt("limit")

		rows, err := d.ListAppMigrations(p.ID, limit)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			ui.Info("No migrations recorded for %s", p.Name)
			ui.Info("Run one with: fleetdeck migrate run %s --command \"...\"", p.Name)
			return nil
		}

		headers := []string{"ID", "STATUS", "STARTED", "DURATION", "SNAPSHOT", "COMMAND"}
		var tableRows [][]string
		for _, m := range rows {
			duration := "-"
			if m.FinishedAt.Valid {
				duration = m.FinishedAt.Time.Sub(m.StartedAt).Round(time.Millisecond).String()
			} else if m.Status == "running" {
				duration = "(in progress)"
			}
			tableRows = append(tableRows, []string{
				shortID(m.ID),
				m.Status,
				m.StartedAt.Format("2006-01-02 15:04"),
				duration,
				shortID(m.SnapshotID),
				m.Command,
			})
		}
		ui.Table(headers, tableRows)
		return nil
	},
}

var migrateRollbackCmd = &cobra.Command{
	Use:   "rollback <project-name>",
	Short: "Restore the snapshot from the most recent migration",
	Long: `Finds the most recent app_migrations row with a recorded
pre-migration snapshot and restores it. Use this when a migration
left the DB in a bad state and you want to rewind to the moment
before it ran.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()
		p, err := d.GetProject(args[0])
		if err != nil {
			return err
		}

		rows, err := d.ListAppMigrations(p.ID, 0)
		if err != nil {
			return err
		}
		var snapshotID string
		for _, m := range rows {
			if m.SnapshotID != "" {
				snapshotID = m.SnapshotID
				break
			}
		}
		if snapshotID == "" {
			return fmt.Errorf("no migration with a pre-migration snapshot found for %s", p.Name)
		}

		ui.Info("Restoring %s from pre-migration snapshot %s...", ui.Bold(p.Name), shortID(snapshotID))
		// Reuse the existing rollback-to-specific-backup path. findBackupByPrefix
		// is shared with cmd/rollback.go.
		backups, err := d.ListBackupRecords(p.ID, 0)
		if err != nil {
			return err
		}
		match := findBackupByPrefix(backups, snapshotID)
		if match == nil {
			return fmt.Errorf("snapshot %s no longer exists (retention may have pruned it)", shortID(snapshotID))
		}
		return restoreBackupRecord(p, match)
	},
}

// shortID returns the first 12 characters of a UUID-style ID, or "-" if
// the input is empty. Keeps the history table from line-wrapping.
func shortID(id string) string {
	if id == "" {
		return "-"
	}
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func init() {
	migrateRunCmd.Flags().String("command", "", "Migration command to run inside the container (required)")
	migrateRunCmd.Flags().String("service", "app", "docker compose service the command runs inside")
	migrateRunCmd.Flags().Duration("timeout", 10*time.Minute, "Migration command timeout")
	migrateRunCmd.Flags().Bool("skip-snapshot", false, "Skip the pre-migration snapshot (disables rollback)")

	migrateHistoryCmd.Flags().Int("limit", 20, "Maximum rows to display (0 = all)")

	migrateCmd.AddCommand(migrateRunCmd)
	migrateCmd.AddCommand(migrateHistoryCmd)
	migrateCmd.AddCommand(migrateRollbackCmd)
	rootCmd.AddCommand(migrateCmd)
}
