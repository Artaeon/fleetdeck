package cmd

import (
	"fmt"

	"github.com/fleetdeck/fleetdeck/internal/schedule"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var scheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Manage scheduled backup timers (systemd)",
}

var scheduleEnableCmd = &cobra.Command{
	Use:   "enable <project>",
	Short: "Install and enable a systemd backup timer for a project",
	Long: `Installs a systemd service and timer unit that will run
'fleetdeck backup create <project> --type scheduled' on the configured schedule.

Examples:
  fleetdeck schedule enable myapp
  fleetdeck schedule enable myapp --schedule "weekly"
  fleetdeck schedule enable myapp --schedule "*-*-* 02:00:00"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := args[0]
		sched, _ := cmd.Flags().GetString("schedule")

		// Verify the project exists in the database
		d := openDB()
		if _, err := d.GetProject(projectName); err != nil {
			return fmt.Errorf("project %q not found: %w", projectName, err)
		}

		ui.Info("Installing backup timer for %s (schedule: %s)...", ui.Bold(projectName), sched)

		if err := schedule.InstallTimer(projectName, sched); err != nil {
			return fmt.Errorf("installing timer: %w", err)
		}
		ui.Success("Timer unit files created")

		if err := schedule.EnableTimer(projectName); err != nil {
			return fmt.Errorf("enabling timer: %w", err)
		}
		ui.Success("Backup timer enabled for %s", ui.Bold(projectName))

		fmt.Println()
		ui.Info("Schedule: %s", sched)
		ui.Info("View status: fleetdeck schedule status %s", projectName)
		ui.Info("Disable: fleetdeck schedule disable %s", projectName)

		return nil
	},
}

var scheduleDisableCmd = &cobra.Command{
	Use:   "disable <project>",
	Short: "Disable and remove the backup timer for a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := args[0]

		ui.Info("Removing backup timer for %s...", ui.Bold(projectName))

		if err := schedule.RemoveTimer(projectName); err != nil {
			return fmt.Errorf("removing timer: %w", err)
		}

		ui.Success("Backup timer removed for %s", ui.Bold(projectName))
		return nil
	},
}

var scheduleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all backup timers and their status",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		timers, err := schedule.ListTimers()
		if err != nil {
			return fmt.Errorf("listing timers: %w", err)
		}

		if len(timers) == 0 {
			ui.Info("No backup timers configured")
			ui.Info("Enable one with: fleetdeck schedule enable <project>")
			return nil
		}

		headers := []string{"PROJECT", "SCHEDULE", "ACTIVE", "NEXT RUN", "LAST RUN"}
		var rows [][]string
		for _, t := range timers {
			active := "no"
			if t.Active {
				active = ui.StatusColor("running")
			}
			nextRun := t.NextRun
			if nextRun == "" {
				nextRun = "n/a"
			}
			lastRun := t.LastRun
			if lastRun == "" {
				lastRun = "n/a"
			}
			rows = append(rows, []string{
				t.ProjectName,
				t.Schedule,
				active,
				nextRun,
				lastRun,
			})
		}

		ui.Table(headers, rows)
		return nil
	},
}

var scheduleStatusCmd = &cobra.Command{
	Use:   "status <project>",
	Short: "Show details of a project's backup timer",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := args[0]

		status, err := schedule.GetTimerStatus(projectName)
		if err != nil {
			return err
		}

		active := "inactive"
		if status.Active {
			active = "active"
		}
		nextRun := status.NextRun
		if nextRun == "" {
			nextRun = "n/a"
		}
		lastRun := status.LastRun
		if lastRun == "" {
			lastRun = "n/a"
		}

		ui.Info("Backup timer for %s", ui.Bold(projectName))
		fmt.Println()
		fmt.Printf("  Schedule:  %s\n", status.Schedule)
		fmt.Printf("  Active:    %s\n", active)
		fmt.Printf("  Next run:  %s\n", nextRun)
		fmt.Printf("  Last run:  %s\n", lastRun)

		return nil
	},
}

func init() {
	scheduleEnableCmd.Flags().String("schedule", "daily", "Backup schedule (e.g. daily, weekly, \"*-*-* 02:00:00\")")

	scheduleCmd.AddCommand(scheduleEnableCmd)
	scheduleCmd.AddCommand(scheduleDisableCmd)
	scheduleCmd.AddCommand(scheduleListCmd)
	scheduleCmd.AddCommand(scheduleStatusCmd)
	rootCmd.AddCommand(scheduleCmd)
}
