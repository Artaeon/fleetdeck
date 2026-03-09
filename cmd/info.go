package cmd

import (
	"fmt"

	"github.com/fleetdeck/fleetdeck/internal/backup"
	"github.com/fleetdeck/fleetdeck/internal/project"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show project details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()
		p, err := d.GetProject(args[0])
		if err != nil {
			return err
		}

		running, total := project.CountContainers(p.ProjectPath)

		fmt.Println(ui.Bold("Project: " + p.Name))
		fmt.Println()
		fmt.Printf("  Domain:      %s\n", p.Domain)
		fmt.Printf("  Status:      %s\n", ui.StatusColor(p.Status))
		fmt.Printf("  Source:      %s\n", p.Source)
		fmt.Printf("  Template:    %s\n", p.Template)
		fmt.Printf("  Path:        %s\n", p.ProjectPath)
		fmt.Printf("  Linux User:  %s\n", p.LinuxUser)
		fmt.Printf("  Containers:  %d/%d\n", running, total)
		if p.GitHubRepo != "" {
			fmt.Printf("  GitHub:      %s\n", p.GitHubRepo)
		}

		// Show backup count
		backups, _ := d.ListBackupRecords(p.ID, 0)
		if len(backups) > 0 {
			var totalSize int64
			for _, b := range backups {
				totalSize += b.SizeBytes
			}
			fmt.Printf("  Backups:     %d (%s total)\n", len(backups), backup.FormatSize(totalSize))
		}

		fmt.Printf("  Created:     %s\n", p.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Updated:     %s\n", p.UpdatedAt.Format("2006-01-02 15:04:05"))

		// Show container details
		containers, err := project.ComposePS(p.ProjectPath)
		if err == nil && len(containers) > 0 {
			fmt.Println()
			fmt.Println(ui.Bold("Containers:"))
			headers := []string{"NAME", "STATE", "STATUS"}
			var rows [][]string
			for _, c := range containers {
				rows = append(rows, []string{c.Name, ui.StatusColor(c.State), c.Status})
			}
			ui.Table(headers, rows)
		}

		// Show recent deployments
		deployments, err := d.ListDeployments(p.ID, 5)
		if err == nil && len(deployments) > 0 {
			fmt.Println()
			fmt.Println(ui.Bold("Recent Deployments:"))
			headers := []string{"SHA", "STATUS", "STARTED", "FINISHED"}
			var rows [][]string
			for _, dep := range deployments {
				sha := dep.CommitSHA
				if len(sha) > 7 {
					sha = sha[:7]
				}
				finished := "-"
				if dep.FinishedAt != nil {
					finished = dep.FinishedAt.Format("15:04:05")
				}
				rows = append(rows, []string{
					sha,
					ui.StatusColor(dep.Status),
					dep.StartedAt.Format("2006-01-02 15:04:05"),
					finished,
				})
			}
			ui.Table(headers, rows)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(infoCmd)
}
