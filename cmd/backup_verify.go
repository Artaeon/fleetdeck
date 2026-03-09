package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/backup"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var backupVerifyCmd = &cobra.Command{
	Use:   "verify <project-name> <backup-id>",
	Short: "Verify the integrity of a backup",
	Long: `Verifies that all component files in a backup exist and pass integrity checks.

Config files are verified by recomputing SHA256 checksums.
Database dumps and volume archives are verified as valid gzip streams.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()
		p, err := d.GetProject(args[0])
		if err != nil {
			return err
		}

		backupID := args[1]

		// Find the backup by prefix match
		backups, err := d.ListBackupRecords(p.ID, 0)
		if err != nil {
			return err
		}

		var foundPath string
		var foundID string
		for _, b := range backups {
			if strings.HasPrefix(b.ID, backupID) {
				foundPath = b.Path
				foundID = b.ID
				break
			}
		}
		if foundPath == "" {
			return fmt.Errorf("backup %q not found for project %s", backupID, p.Name)
		}

		ui.Info("Verifying backup %s for %s...", foundID[:minInt(12, len(foundID))], ui.Bold(p.Name))
		fmt.Println()

		results, err := backup.VerifyBackup(foundPath)
		if err != nil {
			return fmt.Errorf("verification failed: %w", err)
		}

		// Print results
		for _, r := range results {
			switch r.Status {
			case backup.VerifyOK:
				ui.Success("%-40s %s", r.Component.Name, "OK")
			case backup.VerifyMissing:
				ui.Error("%-40s %s", r.Component.Name, "MISSING")
			case backup.VerifyFailed:
				ui.Error("%-40s %s (%v)", r.Component.Name, "FAILED", r.Error)
			}
		}

		total, ok, failed, missing := backup.CountResults(results)
		fmt.Println()
		ui.Info("Total: %d  OK: %d  Failed: %d  Missing: %d", total, ok, failed, missing)

		if backup.HasFailures(results) {
			fmt.Println()
			ui.Error("Backup verification FAILED")
			os.Exit(1)
		}

		fmt.Println()
		ui.Success("Backup verification passed")
		return nil
	},
}

func init() {
	backupCmd.AddCommand(backupVerifyCmd)
}
