package cmd

import (
	"fmt"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/backup"
	"github.com/fleetdeck/fleetdeck/internal/db"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var rollbackCmd = &cobra.Command{
	Use:   "rollback <project-name>",
	Short: "Roll back a project to a previous backup or snapshot",
	Long: `Rolls back a project to a previous state by restoring from a backup.

By default, shows the 10 most recent backups and prompts you to pick one.
Use --latest to automatically select the most recent backup, or --backup-id
to specify a particular backup.

A pre-rollback snapshot is created automatically before restoring, unless
--no-snapshot is specified.`,
	Args: cobra.ExactArgs(1),
	RunE: runRollback,
}

func init() {
	rollbackCmd.Flags().Bool("latest", false, "Automatically use the most recent backup")
	rollbackCmd.Flags().String("backup-id", "", "Use a specific backup ID")
	rollbackCmd.Flags().Bool("no-snapshot", false, "Skip creating a pre-rollback snapshot")

	rootCmd.AddCommand(rollbackCmd)
}

func runRollback(cmd *cobra.Command, args []string) error {
	d := openDB()
	projectName := args[0]

	p, err := d.GetProject(projectName)
	if err != nil {
		return err
	}

	// Fetch last 10 backups
	backups, err := d.ListBackupRecords(p.ID, 10)
	if err != nil {
		return fmt.Errorf("listing backups: %w", err)
	}
	if len(backups) == 0 {
		ui.Warn("No backups found for %s", ui.Bold(p.Name))
		ui.Info("Create one with: fleetdeck backup create %s", p.Name)
		return nil
	}

	useLatest, _ := cmd.Flags().GetBool("latest")
	backupID, _ := cmd.Flags().GetString("backup-id")

	var selected *db.BackupRecord

	switch {
	case backupID != "":
		// Find by ID prefix
		selected = findBackupByPrefix(backups, backupID)
		if selected == nil {
			return fmt.Errorf("backup %q not found for project %s", backupID, p.Name)
		}

	case useLatest:
		// The list is ordered by created_at DESC, so index 0 is the most recent
		selected = backups[0]
		ui.Info("Using most recent backup: %s (%s, %s)",
			selected.ID[:minInt(12, len(selected.ID))],
			selected.Type,
			selected.CreatedAt.Format("2006-01-02 15:04"))

	default:
		// Interactive selection
		selected, err = promptBackupSelection(backups)
		if err != nil {
			return err
		}
	}

	// Create pre-rollback snapshot unless --no-snapshot
	noSnapshot, _ := cmd.Flags().GetBool("no-snapshot")
	if !noSnapshot {
		ui.Info("Creating pre-rollback snapshot of %s...", ui.Bold(p.Name))
		snapRecord, err := backup.CreateBackup(cfg, d, p, "snapshot", "pre-rollback", backup.Options{})
		if err != nil {
			ui.Warn("Could not create pre-rollback snapshot: %v", err)
		} else {
			ui.Success("Pre-rollback snapshot created: %s", snapRecord.ID[:minInt(12, len(snapRecord.ID))])
			fmt.Println()
		}
	}

	// Perform the restore
	shortID := selected.ID[:minInt(12, len(selected.ID))]
	ui.Info("Restoring %s from backup %s...", ui.Bold(p.Name), shortID)
	fmt.Println()

	if err := backup.RestoreBackup(selected.Path, p.ProjectPath, backup.RestoreOptions{}); err != nil {
		return fmt.Errorf("restoring backup: %w", err)
	}

	// Update project status
	if err := d.UpdateProjectStatus(p.Name, "running"); err != nil {
		ui.Warn("Could not update project status: %v", err)
	}

	fmt.Println()
	ui.Success("Project %s rolled back to backup %s (%s from %s)",
		ui.Bold(p.Name),
		shortID,
		selected.Type,
		selected.CreatedAt.Format("2006-01-02 15:04"))

	return nil
}

func findBackupByPrefix(backups []*db.BackupRecord, prefix string) *db.BackupRecord {
	for _, b := range backups {
		if strings.HasPrefix(b.ID, prefix) {
			return b
		}
	}
	return nil
}

func promptBackupSelection(backups []*db.BackupRecord) (*db.BackupRecord, error) {
	ui.Info("Recent backups:")
	fmt.Println()

	headers := []string{"#", "ID", "TYPE", "TRIGGER", "SIZE", "DATE"}
	var rows [][]string
	for i, b := range backups {
		id := b.ID
		if len(id) > 12 {
			id = id[:12]
		}
		rows = append(rows, []string{
			fmt.Sprintf("%d", i+1),
			id,
			b.Type,
			b.Trigger,
			backup.FormatSize(b.SizeBytes),
			b.CreatedAt.Format("2006-01-02 15:04"),
		})
	}
	ui.Table(headers, rows)
	fmt.Println()

	fmt.Print("Select backup number (or 'q' to cancel): ")
	var input string
	fmt.Scanln(&input)

	input = strings.TrimSpace(input)
	if input == "q" || input == "" {
		ui.Info("Aborted")
		return nil, fmt.Errorf("rollback cancelled")
	}

	var choice int
	if _, err := fmt.Sscanf(input, "%d", &choice); err != nil || choice < 1 || choice > len(backups) {
		return nil, fmt.Errorf("invalid selection: %s", input)
	}

	return backups[choice-1], nil
}
