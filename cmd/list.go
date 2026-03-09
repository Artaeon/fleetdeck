package cmd

import (
	"fmt"

	"github.com/fleetdeck/fleetdeck/internal/project"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()
		projects, err := d.ListProjects()
		if err != nil {
			return fmt.Errorf("listing projects: %w", err)
		}

		if len(projects) == 0 {
			ui.Info("No projects found. Create one with: fleetdeck create <name> --domain <domain>")
			return nil
		}

		headers := []string{"NAME", "DOMAIN", "STATUS", "CONTAINERS", "TEMPLATE", "CREATED"}
		var rows [][]string

		for _, p := range projects {
			running, total := project.CountContainers(p.ProjectPath)
			containers := fmt.Sprintf("%d/%d", running, total)

			rows = append(rows, []string{
				p.Name,
				p.Domain,
				ui.StatusColor(p.Status),
				containers,
				p.Template,
				p.CreatedAt.Format("2006-01-02"),
			})
		}

		ui.Table(headers, rows)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
