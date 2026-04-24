package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/backup"
	"github.com/fleetdeck/fleetdeck/internal/backup/remote"
	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/db"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Manage project backups and snapshots",
}

var backupCreateCmd = &cobra.Command{
	Use:   "create <project-name>",
	Short: "Create a backup of a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()
		p, err := d.GetProject(args[0])
		if err != nil {
			return err
		}

		skipDB, _ := cmd.Flags().GetBool("skip-db")
		skipVolumes, _ := cmd.Flags().GetBool("skip-volumes")
		backupType, _ := cmd.Flags().GetString("type")

		ui.Info("Creating %s backup for %s...", backupType, ui.Bold(p.Name))
		fmt.Println()

		record, err := backup.CreateBackup(cfg, d, p, backupType, "user", backup.Options{
			SkipDB:      skipDB,
			SkipVolumes: skipVolumes,
		})
		if err != nil {
			audit.Log("backup.create", p.Name, err.Error(), false)
			return fmt.Errorf("creating backup: %w", err)
		}

		// Enforce retention
		backup.EnforceRetention(cfg, d, p.ID)

		audit.Log("backup.create", p.Name, fmt.Sprintf("id=%s type=%s", record.ID[:12], backupType), true)
		fmt.Println()
		ui.Success("Backup created: %s", record.ID[:12])
		ui.Info("Size: %s", backup.FormatSize(record.SizeBytes))
		ui.Info("Path: %s", record.Path)

		// Auto-push to off-server storage if the operator opted in via
		// [backup.remote] auto_push = true. We run this synchronously and
		// report failures as warnings rather than hard errors — the local
		// backup succeeded, which is still useful.
		if cfg.Backup.Remote.AutoPush {
			if err := autoPushBackup(cmd.Context(), p.Name, record); err != nil {
				ui.Warn("Auto-push failed: %v", err)
				ui.Warn("Run 'fleetdeck backup push %s %s' to retry.", p.Name, record.ID[:12])
				audit.Log("backup.push.auto", p.Name, err.Error(), false)
			}
		}
		return nil
	},
}

// autoPushBackup uploads the freshly-created backup to the configured
// remote. Extracted so future call sites (scheduled snapshots, deploy-time
// safety backups) can opt in without duplicating driver handling.
func autoPushBackup(ctx context.Context, projectName string, record *db.BackupRecord) error {
	driver, err := remote.Open(cfg.Backup.Remote)
	if errors.Is(err, remote.ErrNoDriver) {
		return nil
	}
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	ui.Info("Pushing backup to %s...", driver.Name())
	dest, err := driver.Push(ctx, record.Path, record.ID)
	if err != nil {
		return err
	}
	audit.Log("backup.push.auto", projectName, fmt.Sprintf("id=%s dest=%s", record.ID[:minInt(12, len(record.ID))], dest), true)
	ui.Success("Pushed to %s", dest)
	return nil
}

var backupListCmd = &cobra.Command{
	Use:   "list <project-name>",
	Short: "List backups for a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()
		p, err := d.GetProject(args[0])
		if err != nil {
			return err
		}

		backups, err := d.ListBackupRecords(p.ID, 0)
		if err != nil {
			return err
		}

		if len(backups) == 0 {
			ui.Info("No backups found for %s", p.Name)
			ui.Info("Create one with: fleetdeck backup create %s", p.Name)
			return nil
		}

		showFull, _ := cmd.Flags().GetBool("all")

		headers := []string{"ID", "TYPE", "TRIGGER", "SIZE", "DATE"}
		var rows [][]string
		for _, b := range backups {
			id := b.ID
			if !showFull && len(id) > 12 {
				id = id[:12]
			}
			rows = append(rows, []string{
				id,
				b.Type,
				b.Trigger,
				backup.FormatSize(b.SizeBytes),
				b.CreatedAt.Format("2006-01-02 15:04"),
			})
		}

		ui.Table(headers, rows)
		return nil
	},
}

var backupRestoreCmd = &cobra.Command{
	Use:   "restore <project-name> <backup-id>",
	Short: "Restore a project from a backup",
	Long: `Restores a project to a previous state from a backup.

Before restoring, an automatic snapshot of the current state is created
so you can always go back.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()
		p, err := d.GetProject(args[0])
		if err != nil {
			return err
		}

		backupID := args[1]

		// Find the backup — try prefix match
		backups, err := d.ListBackupRecords(p.ID, 0)
		if err != nil {
			return err
		}
		var found *backup.Manifest
		var foundPath string
		for _, b := range backups {
			if strings.HasPrefix(b.ID, backupID) {
				m, err := backup.ReadManifest(b.Path)
				if err != nil {
					continue
				}
				found = m
				foundPath = b.Path
				break
			}
		}
		if found == nil {
			return fmt.Errorf("backup %q not found for project %s", backupID, p.Name)
		}

		force, _ := cmd.Flags().GetBool("force")
		if !force {
			fmt.Printf("Restore %s from backup %s (%s)? [y/N] ", ui.Bold(p.Name), backupID[:minInt(12, len(backupID))], found.CreatedAt)
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(answer)) != "y" {
				ui.Info("Aborted")
				return nil
			}
		}

		// Auto-snapshot current state before restoring
		if cfg.Backup.AutoSnapshot {
			ui.Info("Creating snapshot of current state...")
			_, err := backup.CreateBackup(cfg, d, p, "snapshot", "pre-restore", backup.Options{})
			if err != nil {
				ui.Warn("Could not create pre-restore snapshot: %v", err)
			} else {
				ui.Success("Pre-restore snapshot created")
			}
			fmt.Println()
		}

		noStart, _ := cmd.Flags().GetBool("no-start")
		filesOnly, _ := cmd.Flags().GetBool("files-only")
		volumesOnly, _ := cmd.Flags().GetBool("volumes-only")
		dbOnly, _ := cmd.Flags().GetBool("db-only")

		if err := backup.RestoreBackup(foundPath, p.ProjectPath, backup.RestoreOptions{
			FilesOnly:   filesOnly,
			VolumesOnly: volumesOnly,
			DBOnly:      dbOnly,
			NoStart:     noStart,
		}); err != nil {
			audit.Log("backup.restore", p.Name, err.Error(), false)
			return fmt.Errorf("restoring backup: %w", err)
		}

		audit.Log("backup.restore", p.Name, fmt.Sprintf("backup=%s", backupID[:minInt(12, len(backupID))]), true)
		fmt.Println()
		ui.Success("Project %s restored from backup %s", ui.Bold(p.Name), backupID[:minInt(12, len(backupID))])
		return nil
	},
}

var backupDeleteCmd = &cobra.Command{
	Use:   "delete <project-name> <backup-id>",
	Short: "Delete a specific backup",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()
		p, err := d.GetProject(args[0])
		if err != nil {
			return err
		}

		backupID := args[1]
		force, _ := cmd.Flags().GetBool("force")

		// Find backup by prefix
		backups, err := d.ListBackupRecords(p.ID, 0)
		if err != nil {
			return err
		}

		for _, b := range backups {
			if !strings.HasPrefix(b.ID, backupID) {
				continue
			}

			if !force {
				fmt.Printf("Delete backup %s (%s, %s)? [y/N] ",
					b.ID[:minInt(12, len(b.ID))], b.Type, b.CreatedAt.Format("2006-01-02 15:04"))
				reader := bufio.NewReader(os.Stdin)
				answer, _ := reader.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(answer)) != "y" {
					ui.Info("Aborted")
					return nil
				}
			}

			if err := os.RemoveAll(b.Path); err != nil {
				ui.Warn("Could not remove backup files: %v", err)
			}
			if err := d.DeleteBackupRecord(b.ID); err != nil {
				return err
			}

			audit.Log("backup.delete", p.Name, fmt.Sprintf("id=%s", b.ID[:minInt(12, len(b.ID))]), true)
			ui.Success("Backup %s deleted", b.ID[:minInt(12, len(b.ID))])
			return nil
		}

		return fmt.Errorf("backup %q not found for project %s", backupID, p.Name)
	},
}

var backupAuditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Report projects whose most recent backup is older than --max-age",
	Long: `Lists every registered project alongside the age of its most recent
backup. Exits non-zero when any project has no backup at all, or when
its most recent backup is older than --max-age — suitable for a cron
that pings an uptime monitor ("dead man's switch") if backups silently
stop being taken.

Use 'fleetdeck backup audit --max-age 48h --quiet' in a monitor.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		maxAge, _ := cmd.Flags().GetDuration("max-age")
		quiet, _ := cmd.Flags().GetBool("quiet")

		d := openDB()
		projects, err := d.ListProjects()
		if err != nil {
			return err
		}
		if len(projects) == 0 {
			if !quiet {
				ui.Info("No projects registered.")
			}
			return nil
		}

		now := time.Now()
		headers := []string{"PROJECT", "LAST BACKUP", "AGE", "STATUS"}
		var rows [][]string
		stale := 0

		for _, p := range projects {
			backups, err := d.ListBackupRecords(p.ID, 1)
			if err != nil {
				rows = append(rows, []string{p.Name, "error", "-", "DB error"})
				stale++
				continue
			}
			if len(backups) == 0 {
				rows = append(rows, []string{p.Name, "none", "-", "MISSING"})
				stale++
				continue
			}
			last := backups[0]
			age := now.Sub(last.CreatedAt)
			status := "ok"
			if age > maxAge {
				status = "STALE"
				stale++
			}
			rows = append(rows, []string{
				p.Name,
				last.CreatedAt.Format("2006-01-02 15:04"),
				age.Round(time.Minute).String(),
				status,
			})
		}

		if !quiet {
			ui.Table(headers, rows)
			fmt.Println()
		}

		if stale > 0 {
			if !quiet {
				ui.Error("%d project(s) have stale or missing backups (threshold: %s)",
					stale, maxAge)
			}
			// Non-zero exit so cron/uptime monitors can fire an alert
			// without having to parse the table output.
			os.Exit(1)
		}
		if !quiet {
			ui.Success("All %d project(s) have backups newer than %s",
				len(projects), maxAge)
		}
		return nil
	},
}

var backupPushCmd = &cobra.Command{
	Use:   "push <project-name> [backup-id]",
	Short: "Push a backup to the configured off-server remote",
	Long: `Uploads a local backup to the remote configured under [backup.remote]
in config.toml. Without a backup-id, pushes the most recent backup.

Requires the 'rclone' binary on PATH and a remote pre-configured via
'rclone config'. Example config.toml entry:

  [backup.remote]
  driver = "rclone"
  target = "b2:my-fleet-backups"`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()
		p, err := d.GetProject(args[0])
		if err != nil {
			return err
		}

		driver, err := remote.Open(cfg.Backup.Remote)
		if errors.Is(err, remote.ErrNoDriver) {
			// Point the user at the TOML config, not the SQLite DB — the
			// first cut of this error message was copy-pasted wrong.
			return fmt.Errorf("no backup remote configured; set [backup.remote] driver and target in %s (override with --config)", cfgFileForError())
		}
		if err != nil {
			return err
		}

		backups, err := d.ListBackupRecords(p.ID, 0)
		if err != nil {
			return err
		}
		if len(backups) == 0 {
			return fmt.Errorf("no backups for %s; create one with 'fleetdeck backup create %s'", p.Name, p.Name)
		}

		record := backups[0] // default: most recent
		recordPath := backups[0].Path
		if len(args) == 2 {
			found := false
			for _, b := range backups {
				if strings.HasPrefix(b.ID, args[1]) {
					record = b
					recordPath = b.Path
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("backup %q not found for project %s", args[1], p.Name)
			}
		}
		// ReadManifest validates the manifest on disk before we start
		// shipping bytes across the network — fail fast on a corrupt local
		// backup rather than uploading garbage.
		if _, err := backup.ReadManifest(recordPath); err != nil {
			return fmt.Errorf("reading manifest for backup %s: %w", record.ID[:minInt(12, len(record.ID))], err)
		}

		timeout, _ := cmd.Flags().GetDuration("timeout")
		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		defer cancel()

		ui.Info("Pushing %s to %s...", record.ID[:minInt(12, len(record.ID))], driver.Name())
		dest, err := driver.Push(ctx, recordPath, record.ID)
		if err != nil {
			audit.Log("backup.push", p.Name, err.Error(), false)
			return fmt.Errorf("pushing backup: %w", err)
		}

		audit.Log("backup.push", p.Name, fmt.Sprintf("id=%s dest=%s", record.ID[:minInt(12, len(record.ID))], dest), true)
		ui.Success("Pushed to %s", dest)
		return nil
	},
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// cfgFileForError returns the config file path that was in effect for the
// current invocation: the --config flag value if set, otherwise the
// compiled-in default. Used only in error messages so we don't send the
// user to the wrong file.
func cfgFileForError() string {
	if cfgFile != "" {
		return cfgFile
	}
	return config.DefaultConfigPath
}

func init() {
	backupCreateCmd.Flags().Bool("skip-db", false, "Skip database dump")
	backupCreateCmd.Flags().Bool("skip-volumes", false, "Skip volume backup")
	backupCreateCmd.Flags().String("type", "manual", "Backup type (manual, snapshot)")

	backupListCmd.Flags().Bool("all", false, "Show full backup IDs")

	backupRestoreCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")
	backupRestoreCmd.Flags().Bool("no-start", false, "Don't start project after restore")
	backupRestoreCmd.Flags().Bool("files-only", false, "Restore only config files")
	backupRestoreCmd.Flags().Bool("volumes-only", false, "Restore only volumes")
	backupRestoreCmd.Flags().Bool("db-only", false, "Restore only databases")

	backupDeleteCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")

	backupPushCmd.Flags().Duration("timeout", 30*time.Minute, "Upload timeout (large backups over slow links may need longer)")

	backupAuditCmd.Flags().Duration("max-age", 48*time.Hour, "Maximum acceptable age for the most recent backup")
	backupAuditCmd.Flags().Bool("quiet", false, "Suppress output; only set exit code")

	backupCmd.AddCommand(backupCreateCmd)
	backupCmd.AddCommand(backupListCmd)
	backupCmd.AddCommand(backupRestoreCmd)
	backupCmd.AddCommand(backupDeleteCmd)
	backupCmd.AddCommand(backupPushCmd)
	backupCmd.AddCommand(backupAuditCmd)
	rootCmd.AddCommand(backupCmd)
}
