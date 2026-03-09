package cmd

import (
	"fmt"
	"os"

	"github.com/fleetdeck/fleetdeck/internal/db"
	"github.com/fleetdeck/fleetdeck/internal/project"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var importCmd = &cobra.Command{
	Use:   "import <name>",
	Short: "Import an existing project into FleetDeck",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		path, _ := cmd.Flags().GetString("path")
		domain, _ := cmd.Flags().GetString("domain")
		template, _ := cmd.Flags().GetString("template")

		if path == "" {
			return fmt.Errorf("--path is required")
		}
		if domain == "" {
			return fmt.Errorf("--domain is required")
		}

		// Verify path exists and has docker-compose.yml
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return fmt.Errorf("path %s does not exist", path)
		}
		if _, err := os.Stat(path + "/docker-compose.yml"); os.IsNotExist(err) {
			ui.Warn("No docker-compose.yml found at %s", path)
		}

		linuxUser := project.LinuxUserName(name)

		// Check if user exists, if not inform
		ui.Info("Importing project %s from %s", name, path)

		d := openDB()
		proj := &db.Project{
			Name:        name,
			Domain:      domain,
			LinuxUser:   linuxUser,
			ProjectPath: path,
			Template:    template,
			Status:      "stopped",
			Source:      "imported",
		}

		if err := d.CreateProject(proj); err != nil {
			return fmt.Errorf("saving project: %w", err)
		}

		// Try to detect current state
		running, total := project.CountContainers(path)
		if total > 0 {
			status := "stopped"
			if running > 0 {
				status = "running"
			}
			_ = d.UpdateProjectStatus(name, status)
			ui.Info("Detected %d/%d containers running", running, total)
		}

		ui.Success("Project %s imported from %s", ui.Bold(name), path)
		ui.Info("Domain: %s", domain)
		ui.Info("Run 'fleetdeck info %s' to see project details", name)
		return nil
	},
}

func init() {
	importCmd.Flags().String("path", "", "Path to existing project (required)")
	importCmd.Flags().String("domain", "", "Domain for the project (required)")
	importCmd.Flags().String("template", "custom", "Template type for metadata")

	_ = importCmd.MarkFlagRequired("path")
	_ = importCmd.MarkFlagRequired("domain")

	rootCmd.AddCommand(importCmd)
}
