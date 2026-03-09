package cmd

import (
	"fmt"
	"os"

	"github.com/fleetdeck/fleetdeck/internal/health"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var healthCmd = &cobra.Command{
	Use:   "health [project]",
	Short: "Show health status of project containers",
	Long: `Show detailed health status of containers for a specific project,
or a summary of all projects when no argument is given.

Exit code 1 is returned if any project is unhealthy.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return showProjectHealth(args[0])
		}
		return showAllHealth()
	},
}

func showProjectHealth(name string) error {
	d := openDB()
	p, err := d.GetProject(name)
	if err != nil {
		return err
	}

	fmt.Printf("%s  %s\n\n", ui.Bold("Health:"), p.Name)

	report, err := health.CheckProject(p.ProjectPath)
	if err != nil {
		ui.Error("Could not check health: %v", err)
		os.Exit(1)
	}

	for _, svc := range report.Services {
		switch svc.Health {
		case "healthy":
			ui.Success("  %-30s %s  (%s)", svc.Name, ui.StatusColor("running"), svc.Status)
		case "unhealthy":
			ui.Error("  %-30s %s  (%s)", svc.Name, ui.StatusColor("error"), svc.Status)
		case "restarting":
			ui.Warn("  %-30s %s  (%s)", svc.Name, ui.StatusColor("deploying"), svc.Status)
		default:
			ui.Warn("  %-30s %s  (%s)", svc.Name, svc.Health, svc.Status)
		}
	}

	if len(report.Errors) > 0 {
		fmt.Println()
		for _, e := range report.Errors {
			ui.Error("  %s", e)
		}
	}

	if !report.Healthy {
		fmt.Println()
		ui.Error("Project %s is unhealthy", p.Name)
		os.Exit(1)
	}

	fmt.Println()
	ui.Success("Project %s is healthy", p.Name)
	return nil
}

func showAllHealth() error {
	d := openDB()
	projects, err := d.ListProjects()
	if err != nil {
		return err
	}

	if len(projects) == 0 {
		ui.Info("No projects found")
		return nil
	}

	fmt.Println(ui.Bold("Project Health Summary"))
	fmt.Println()

	anyUnhealthy := false
	rows := make([][]string, 0, len(projects))

	for _, p := range projects {
		report, err := health.CheckProject(p.ProjectPath)
		if err != nil {
			rows = append(rows, []string{p.Name, ui.StatusColor("error"), "could not check"})
			anyUnhealthy = true
			continue
		}

		running := 0
		total := len(report.Services)
		for _, svc := range report.Services {
			if svc.Health == "healthy" {
				running++
			}
		}

		var statusStr string
		if report.Healthy {
			statusStr = ui.StatusColor("running")
		} else {
			statusStr = ui.StatusColor("error")
			anyUnhealthy = true
		}

		containers := fmt.Sprintf("%d/%d healthy", running, total)
		rows = append(rows, []string{p.Name, statusStr, containers})
	}

	ui.Table([]string{"PROJECT", "STATUS", "CONTAINERS"}, rows)

	if anyUnhealthy {
		fmt.Println()
		ui.Error("One or more projects are unhealthy")
		os.Exit(1)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(healthCmd)
}
