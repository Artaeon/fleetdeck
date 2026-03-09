package cmd

import (
	"time"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/backup"
	"github.com/fleetdeck/fleetdeck/internal/health"
	"github.com/fleetdeck/fleetdeck/internal/project"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

func autoSnapshot(projectName, trigger string) {
	if cfg == nil || !cfg.Backup.AutoSnapshot {
		return
	}
	d := openDB()
	p, err := d.GetProject(projectName)
	if err != nil {
		return
	}
	ui.Info("Creating auto-snapshot before %s...", trigger)
	if _, err := backup.CreateBackup(cfg, d, p, "snapshot", "pre-"+trigger, backup.Options{}); err != nil {
		ui.Warn("Auto-snapshot failed: %v", err)
	} else {
		ui.Success("Auto-snapshot created")
	}
	backup.EnforceRetention(cfg, d, p.ID)
}

var startCmd = &cobra.Command{
	Use:   "start <name>",
	Short: "Start a stopped project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()
		p, err := d.GetProject(args[0])
		if err != nil {
			return err
		}

		// Auto-snapshot before start (protects against bad deployments)
		autoSnapshot(p.Name, "start")

		ui.Info("Starting %s...", p.Name)
		if err := project.ComposeUp(p.ProjectPath); err != nil {
			d.UpdateProjectStatus(p.Name, "error")
			audit.Log("project.start", p.Name, err.Error(), false)
			return err
		}

		// Wait for containers to become healthy (up to 30s)
		ui.Info("Waiting for services to become healthy...")
		report := health.WaitForHealthy(p.ProjectPath, 30*time.Second)
		printHealthReport(report)

		if err := d.UpdateProjectStatus(p.Name, "running"); err != nil {
			ui.Warn("Could not update status: %v", err)
		}

		audit.Log("project.start", p.Name, "started", true)
		ui.Success("Project %s started", p.Name)
		return nil
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop <name>",
	Short: "Stop a running project (keeps data)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()
		p, err := d.GetProject(args[0])
		if err != nil {
			return err
		}

		autoSnapshot(p.Name, "stop")

		ui.Info("Stopping %s...", p.Name)
		if err := project.ComposeDown(p.ProjectPath); err != nil {
			d.UpdateProjectStatus(p.Name, "error")
			audit.Log("project.stop", p.Name, err.Error(), false)
			return err
		}

		if err := d.UpdateProjectStatus(p.Name, "stopped"); err != nil {
			ui.Warn("Could not update status: %v", err)
		}

		audit.Log("project.stop", p.Name, "stopped", true)
		ui.Success("Project %s stopped", p.Name)
		return nil
	},
}

var restartCmd = &cobra.Command{
	Use:   "restart <name>",
	Short: "Restart all services for a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()
		p, err := d.GetProject(args[0])
		if err != nil {
			return err
		}

		autoSnapshot(p.Name, "restart")

		ui.Info("Restarting %s...", p.Name)
		if err := project.ComposeRestart(p.ProjectPath); err != nil {
			d.UpdateProjectStatus(p.Name, "error")
			audit.Log("project.restart", p.Name, err.Error(), false)
			return err
		}

		// Wait for containers to become healthy (up to 30s)
		ui.Info("Waiting for services to become healthy...")
		report := health.WaitForHealthy(p.ProjectPath, 30*time.Second)
		printHealthReport(report)

		if err := d.UpdateProjectStatus(p.Name, "running"); err != nil {
			ui.Warn("Could not update status: %v", err)
		}

		audit.Log("project.restart", p.Name, "restarted", true)
		ui.Success("Project %s restarted", p.Name)
		return nil
	},
}

func printHealthReport(report *health.HealthReport) {
	if report == nil {
		ui.Warn("Could not determine health status")
		return
	}
	for _, svc := range report.Services {
		switch svc.Health {
		case "healthy":
			ui.Success("  %s: %s", svc.Name, svc.Health)
		case "restarting":
			ui.Warn("  %s: %s (crash loop detected)", svc.Name, svc.Health)
		case "unhealthy":
			ui.Error("  %s: %s", svc.Name, svc.Health)
		default:
			ui.Warn("  %s: %s", svc.Name, svc.Health)
		}
	}
	if !report.Healthy {
		ui.Warn("Some services are not healthy yet — they may still be starting")
	}
}

func init() {
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(restartCmd)
}
