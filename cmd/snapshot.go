package cmd

import (
	"fmt"

	"github.com/fleetdeck/fleetdeck/internal/backup"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot <project-name>",
	Short: "Create a quick snapshot of a project (shortcut for backup create --type snapshot)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()
		p, err := d.GetProject(args[0])
		if err != nil {
			return err
		}

		ui.Info("Creating snapshot of %s...", ui.Bold(p.Name))
		fmt.Println()

		record, err := backup.CreateBackup(cfg, d, p, "snapshot", "user", backup.Options{})
		if err != nil {
			return fmt.Errorf("creating snapshot: %w", err)
		}

		// Enforce retention
		backup.EnforceRetention(cfg, d, p.ID)

		fmt.Println()
		ui.Success("Snapshot created: %s", record.ID[:12])
		ui.Info("Size: %s", backup.FormatSize(record.SizeBytes))
		ui.Info("Path: %s", record.Path)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(snapshotCmd)
}
