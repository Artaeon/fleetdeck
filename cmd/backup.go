package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/backup"
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
		return nil
	},
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

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
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

	backupCmd.AddCommand(backupCreateCmd)
	backupCmd.AddCommand(backupListCmd)
	backupCmd.AddCommand(backupRestoreCmd)
	backupCmd.AddCommand(backupDeleteCmd)
	rootCmd.AddCommand(backupCmd)
}
