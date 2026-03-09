package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/disaster"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var importStateCmd = &cobra.Command{
	Use:   "import-state <archive-path>",
	Short: "Import FleetDeck state from an export archive",
	Long: `Restores the complete FleetDeck state from a previously created export archive.

This will overwrite:
- The FleetDeck database
- Project backups contained in the archive
- The configuration file

A backup of the current state is created automatically before importing.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		archivePath := args[0]

		// Verify archive exists
		if _, err := os.Stat(archivePath); os.IsNotExist(err) {
			return fmt.Errorf("archive not found: %s", archivePath)
		}

		// Read manifest to show what will be imported
		manifest, err := disaster.ReadStateManifest(archivePath)
		if err != nil {
			return fmt.Errorf("reading archive manifest: %w", err)
		}

		ui.Info("Import archive: %s", archivePath)
		ui.Info("Exported at:    %s", manifest.ExportTimestamp)
		ui.Info("Version:        %s", manifest.FleetDeckVersion)
		ui.Info("Projects:       %d", manifest.ProjectCount)
		ui.Info("Backups:        %d", manifest.BackupCount)
		fmt.Println()

		force, _ := cmd.Flags().GetBool("force")
		if !force {
			ui.Warn("This will overwrite the current FleetDeck database and backups.")
			fmt.Print("Continue? [y/N] ")
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(answer)) != "y" {
				ui.Info("Aborted")
				return nil
			}
		}

		totalSteps := 3
		step := 0

		// Step 1: Back up current state
		step++
		ui.Step(step, totalSteps, "Backing up current state...")
		backupDir := filepath.Join(cfg.Server.BasePath, "pre-import-backups")
		timestamp := time.Now().Format("20060102-150405")
		currentBackupPath := filepath.Join(backupDir, fmt.Sprintf("pre-import-%s.tar.gz", timestamp))

		// Try to back up current database before overwriting
		if d := openDB(); d != nil {
			if err := disaster.ExportState(cfg, d, currentBackupPath, Version); err != nil {
				ui.Warn("Could not back up current state: %v", err)
				ui.Warn("Proceeding with import anyway...")
			} else {
				ui.Success("Current state backed up to %s", currentBackupPath)
			}
			d.Close()
			database = nil // reset so it gets reopened
		}

		// Step 2: Import archive
		step++
		ui.Step(step, totalSteps, "Importing state from archive...")
		if err := disaster.ImportState(archivePath, cfg.Server.BasePath); err != nil {
			return fmt.Errorf("import failed: %w", err)
		}
		ui.Success("State imported successfully")

		// Step 3: Verify
		step++
		ui.Step(step, totalSteps, "Verifying imported state...")
		verifyDB := openDB()
		projects, err := verifyDB.ListProjects()
		if err != nil {
			ui.Warn("Could not verify imported database: %v", err)
		} else {
			ui.Success("Verified: %d projects in imported database", len(projects))
		}

		fmt.Println()
		ui.Success("Import completed successfully!")
		ui.Info("Run 'fleetdeck list' to see imported projects")
		return nil
	},
}

func init() {
	importStateCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")
	rootCmd.AddCommand(importStateCmd)
}
