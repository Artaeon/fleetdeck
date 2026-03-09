package cmd

import (
	"fmt"
	"os"

	"github.com/fleetdeck/fleetdeck/internal/discover"
	"github.com/fleetdeck/fleetdeck/internal/project"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync FleetDeck database with actual system state",
	Long: `Compares the FleetDeck database with the actual state of Docker containers
and projects on the server. Reports discrepancies and optionally fixes them.

This is useful after installing FleetDeck on an existing server, or to
catch manual changes that happened outside of FleetDeck.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()
		fix, _ := cmd.Flags().GetBool("fix")

		ui.Info("Syncing FleetDeck state with system...")
		fmt.Println()

		// Get all managed projects
		projects, err := d.ListProjects()
		if err != nil {
			return err
		}

		// Discover all projects on the system
		discovered, err := discover.DiscoverAll(cfg, d)
		if err != nil {
			ui.Warn("Discovery failed: %v", err)
		}

		updatedCount := 0
		orphanCount := 0
		unmanagedCount := 0

		// Check each managed project against reality
		for _, p := range projects {
			// Check if project path still exists
			if _, err := os.Stat(p.ProjectPath); os.IsNotExist(err) {
				ui.Warn("ORPHAN: %s — path %s no longer exists", p.Name, p.ProjectPath)
				orphanCount++
				if fix {
					if err := d.UpdateProjectStatus(p.Name, "error"); err == nil {
						ui.Info("  → Status set to 'error'")
					}
				}
				continue
			}

			// Check actual container state
			running, total := project.CountContainers(p.ProjectPath)
			actualStatus := "stopped"
			if running > 0 {
				actualStatus = "running"
			}

			if p.Status != actualStatus && p.Status != "created" {
				ui.Info("STATUS MISMATCH: %s — DB says '%s', actually '%s' (%d/%d containers)",
					p.Name, p.Status, actualStatus, running, total)
				updatedCount++
				if fix {
					if err := d.UpdateProjectStatus(p.Name, actualStatus); err == nil {
						ui.Success("  → Updated to '%s'", actualStatus)
					}
				}
			}
		}

		// Check for unmanaged projects
		for _, dp := range discovered {
			if !dp.AlreadyManaged {
				unmanagedCount++
			}
		}

		// Summary
		fmt.Println()
		fmt.Println(ui.Bold("Sync Summary:"))
		fmt.Printf("  Managed projects:   %d\n", len(projects))
		fmt.Printf("  Status mismatches:  %d\n", updatedCount)
		fmt.Printf("  Orphaned projects:  %d\n", orphanCount)
		fmt.Printf("  Unmanaged projects: %d\n", unmanagedCount)

		if !fix && (updatedCount > 0 || orphanCount > 0) {
			fmt.Println()
			ui.Info("Run with --fix to apply corrections")
		}

		if unmanagedCount > 0 {
			ui.Info("Run 'fleetdeck discover' to see unmanaged projects")
		}

		return nil
	},
}

// Also add sync as a discover subcommand
var discoverSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync FleetDeck database with actual system state",
	RunE:  syncCmd.RunE,
}

func init() {
	syncCmd.Flags().Bool("fix", false, "Apply corrections to database")
	discoverSyncCmd.Flags().Bool("fix", false, "Apply corrections to database")

	discoverCmd.AddCommand(discoverSyncCmd)
	rootCmd.AddCommand(syncCmd)
}
