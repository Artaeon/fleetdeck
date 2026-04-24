package cmd

import (
	"fmt"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Trim old rows from fleetdeck's database",
	Long: `Fleetdeck records every deployment in a 'deployments' table.
On a busy project the log column holds 5-50 KB of compose output per
row, so a year of daily deploys to mealtime would add hundreds of MB
of SQLite data and slow every SELECT. 'prune' removes old rows while
keeping the most recent N per project.

Safe to run frequently — deployment history older than the keep
window is almost never consulted, and a pre-prune copy of the
database is already written by the startup .bak rotation.`,
}

var pruneDeploymentsCmd = &cobra.Command{
	Use:   "deployments",
	Short: "Keep only the newest N deployments per project",
	RunE: func(cmd *cobra.Command, args []string) error {
		keep, _ := cmd.Flags().GetInt("keep")
		project, _ := cmd.Flags().GetString("project")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		if keep <= 0 {
			return fmt.Errorf("--keep must be > 0 (got %d); pass a positive retention count", keep)
		}

		d := openDB()

		if dryRun {
			// Dry-run inspects counts without deleting. We query per
			// project because ListDeployments returns newest-first so
			// the 'would delete' number is straightforward to compute.
			var total int
			if project != "" {
				p, err := d.GetProject(project)
				if err != nil {
					return err
				}
				rows, err := d.ListDeployments(p.ID, 0)
				if err != nil {
					return err
				}
				if len(rows) > keep {
					total = len(rows) - keep
				}
				ui.Info("Dry-run: would remove %d deployment row(s) for %s", total, p.Name)
				return nil
			}
			projects, err := d.ListProjects()
			if err != nil {
				return err
			}
			for _, p := range projects {
				rows, _ := d.ListDeployments(p.ID, 0)
				if len(rows) > keep {
					total += len(rows) - keep
				}
			}
			ui.Info("Dry-run: would remove %d deployment row(s) across %d project(s)", total, len(projects))
			return nil
		}

		var removed int64
		var err error
		if project != "" {
			p, perr := d.GetProject(project)
			if perr != nil {
				return perr
			}
			removed, err = d.PruneDeployments(p.ID, keep)
			if err == nil {
				audit.Log("prune.deployments", project, fmt.Sprintf("removed=%d keep=%d", removed, keep), true)
			}
		} else {
			removed, err = d.PruneAllDeployments(keep)
			if err == nil {
				audit.Log("prune.deployments", "*", fmt.Sprintf("removed=%d keep=%d", removed, keep), true)
			}
		}
		if err != nil {
			return err
		}

		ui.Success("Removed %d old deployment row(s); kept newest %d per project.", removed, keep)
		return nil
	},
}

func init() {
	pruneDeploymentsCmd.Flags().Int("keep", 50, "Keep this many newest deployments per project")
	pruneDeploymentsCmd.Flags().String("project", "", "Limit pruning to a single project (default: all projects)")
	pruneDeploymentsCmd.Flags().Bool("dry-run", false, "Report what would be removed without deleting")

	pruneCmd.AddCommand(pruneDeploymentsCmd)
	rootCmd.AddCommand(pruneCmd)
}
