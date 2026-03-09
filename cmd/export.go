package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/backup"
	"github.com/fleetdeck/fleetdeck/internal/disaster"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export full FleetDeck state for disaster recovery",
	Long: `Creates a tar.gz archive containing the complete FleetDeck state:
- SQLite database (consistent snapshot)
- Current configuration
- Latest backup for each project
- State manifest with export metadata

The archive can be imported on another server with 'fleetdeck import-state'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()

		outputPath, _ := cmd.Flags().GetString("output")
		if outputPath == "" {
			exportDir := filepath.Join(cfg.Server.BasePath, "exports")
			timestamp := time.Now().Format("20060102-150405")
			outputPath = filepath.Join(exportDir, fmt.Sprintf("fleetdeck-export-%s.tar.gz", timestamp))
		}

		totalSteps := 4
		step := 0

		// Step 1: Database
		step++
		ui.Step(step, totalSteps, "Exporting database...")

		// Step 2: Configuration
		step++
		ui.Step(step, totalSteps, "Exporting configuration...")

		// Step 3: Project backups
		step++
		ui.Step(step, totalSteps, "Exporting project backups...")

		// Step 4: Creating archive
		step++
		ui.Step(step, totalSteps, "Creating archive at %s...", outputPath)

		if err := disaster.ExportState(cfg, d, outputPath, Version); err != nil {
			return fmt.Errorf("export failed: %w", err)
		}

		// Print summary
		info, err := os.Stat(outputPath)
		if err != nil {
			return fmt.Errorf("reading export file: %w", err)
		}

		manifest, err := disaster.ReadStateManifest(outputPath)
		if err != nil {
			ui.Warn("Could not read export manifest: %v", err)
		}

		fmt.Println()
		ui.Success("Export completed successfully!")
		ui.Info("Archive: %s", outputPath)
		ui.Info("Size:    %s", backup.FormatSize(info.Size()))
		if manifest != nil {
			ui.Info("Projects: %d", manifest.ProjectCount)
			ui.Info("Backups:  %d", manifest.BackupCount)
			ui.Info("Version:  %s", manifest.FleetDeckVersion)
		}

		return nil
	},
}

func init() {
	exportCmd.Flags().StringP("output", "o", "", "Output path for the export archive")
	rootCmd.AddCommand(exportCmd)
}
