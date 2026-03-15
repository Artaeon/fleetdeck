package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/monitor"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Health monitoring and alerting",
	Long:  `Monitor project health and send alerts on failures.`,
}

var monitorStartCmd = &cobra.Command{
	Use:   "start <name> [name...]",
	Short: "Start monitoring one or more projects",
	Long: `Starts continuous health monitoring for one or more projects.

Checks each project's URL at regular intervals and sends
alerts via configured providers (webhook, Slack, email)
when health status changes.

Examples:
  fleetdeck monitor start myapp
  fleetdeck monitor start myapp api blog --slack https://hooks.slack.com/xxx`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()

		interval, _ := cmd.Flags().GetDuration("interval")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		webhookURL, _ := cmd.Flags().GetString("webhook")
		slackURL, _ := cmd.Flags().GetString("slack")
		threshold, _ := cmd.Flags().GetInt("threshold")

		var targets []monitor.Target
		for _, name := range args {
			proj, err := d.GetProject(name)
			if err != nil {
				return fmt.Errorf("project %q not found: %w", name, err)
			}
			targets = append(targets, monitor.Target{
				Name:           proj.Name,
				URL:            fmt.Sprintf("https://%s", proj.Domain),
				Method:         "GET",
				ExpectedStatus: 200,
				Timeout:        timeout,
				Interval:       interval,
			})
		}

		var providers []monitor.AlertProvider
		if webhookURL != "" {
			providers = append(providers, monitor.NewWebhookProvider(webhookURL))
		}
		if slackURL != "" {
			providers = append(providers, monitor.NewSlackProvider(slackURL))
		}

		statePath := filepath.Join("/opt/fleetdeck/monitor", "monitor-state.json")
		if len(args) == 1 {
			statePath = filepath.Join("/opt/fleetdeck/monitor", args[0]+".json")
		}
		mon := monitor.NewWithState(targets, providers, threshold, statePath)

		// Try to restore previous state from disk.
		if prev, err := monitor.LoadState(statePath); err == nil {
			ui.Info("Restored previous monitor state (saved %s)", prev.UpdatedAt.Format(time.RFC3339))
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		for _, t := range targets {
			ui.Success("Monitoring %s at %s (every %s)", t.Name, t.URL, interval)
		}
		ui.Info("Press Ctrl+C to stop")
		fmt.Println()

		mon.Start(ctx)

		<-sigCh
		fmt.Println()
		ui.Info("Stopping monitor...")
		mon.Stop()
		ui.Success("Monitor stopped")

		return nil
	},
}

var monitorCheckCmd = &cobra.Command{
	Use:   "check <name>",
	Short: "Run a single health check",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		d := openDB()
		proj, err := d.GetProject(name)
		if err != nil {
			return fmt.Errorf("project %q not found: %w", name, err)
		}

		timeout, _ := cmd.Flags().GetDuration("timeout")

		target := monitor.Target{
			Name:           proj.Name,
			URL:            fmt.Sprintf("https://%s", proj.Domain),
			Method:         "GET",
			ExpectedStatus: 200,
			Timeout:        timeout,
		}

		mon := monitor.New(nil, nil, 3)
		result := mon.CheckOnce(target)

		fmt.Println()
		if result.Healthy {
			ui.Success("%s is healthy", name)
		} else {
			ui.Error("%s is unhealthy", name)
		}

		headers := []string{"Property", "Value"}
		rows := [][]string{
			{"URL", target.URL},
			{"Status", fmt.Sprintf("%d", result.StatusCode)},
			{"Response Time", result.ResponseTime.Round(time.Millisecond).String()},
			{"Healthy", boolToYesNo(result.Healthy)},
		}
		if result.Error != "" {
			rows = append(rows, []string{"Error", result.Error})
		}
		ui.Table(headers, rows)
		fmt.Println()

		if !result.Healthy {
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	monitorStartCmd.Flags().Duration("interval", 30*time.Second, "Check interval")
	monitorStartCmd.Flags().Duration("timeout", 10*time.Second, "HTTP timeout per check")
	monitorStartCmd.Flags().String("webhook", "", "Webhook URL for alerts")
	monitorStartCmd.Flags().String("slack", "", "Slack webhook URL for alerts")
	monitorStartCmd.Flags().Int("threshold", 3, "Failures before alerting")

	monitorCheckCmd.Flags().Duration("timeout", 10*time.Second, "HTTP timeout")

	monitorCmd.AddCommand(monitorStartCmd)
	monitorCmd.AddCommand(monitorCheckCmd)
	rootCmd.AddCommand(monitorCmd)
}
