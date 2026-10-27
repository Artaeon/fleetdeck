package cmd

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/monitor"
	"github.com/fleetdeck/fleetdeck/internal/notify"
	"github.com/spf13/cobra"
)

var alertCmd = &cobra.Command{Use: "alert", Short: "Send operational notifications and relay trusted host events"}

func alertProviders(cmd *cobra.Command) ([]monitor.AlertProvider, error) {
	var providers []monitor.AlertProvider
	telegram, _ := cmd.Flags().GetString("telegram-config")
	if telegram == "" {
		telegram = cfg.Monitoring.TelegramConfigFile
	}
	if telegram != "" {
		p, err := monitor.LoadTelegramProvider(telegram)
		if err != nil {
			return nil, err
		}
		providers = append(providers, p)
	}
	smtp, _ := cmd.Flags().GetString("smtp-config")
	if smtp == "" {
		smtp = cfg.Monitoring.SMTPConfigFile
	}
	if smtp != "" {
		p, err := monitor.LoadEmailProvider(smtp, cfg.Monitoring.EmailTo)
		if err != nil {
			return nil, err
		}
		providers = append(providers, p)
	}
	if len(providers) == 0 {
		return nil, fmt.Errorf("configure a private Telegram or SMTP file first")
	}
	return providers, nil
}

var alertSendCmd = &cobra.Command{
	Use: "send", Short: "Send an operational message through configured notification channels",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		providers, err := alertProviders(cmd)
		if err != nil {
			return err
		}
		title, _ := cmd.Flags().GetString("title")
		message, _ := cmd.Flags().GetString("message")
		target, _ := cmd.Flags().GetString("target")
		level, _ := cmd.Flags().GetString("level")
		if title == "" || (level != "info" && level != "warning" && level != "critical") {
			return fmt.Errorf("provide a title and level info, warning or critical")
		}
		failed := false
		for _, p := range providers {
			if p.Send(monitor.Alert{Level: level, Title: title, Message: message, Target: target, Timestamp: time.Now()}) != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "Notification delivery failed for", p.Name())
				failed = true
			}
		}
		if failed {
			return fmt.Errorf("one or more notification channels failed; successful channels are not rolled back")
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Notification accepted by all configured providers")
		return nil
	},
}

var alertWatchCmd = &cobra.Command{
	Use: "watch", Short: "Relay SSH and FleetDeck audit events from the local system journal",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		providers, err := alertProviders(cmd)
		if err != nil {
			return err
		}
		path, _ := cmd.Flags().GetString("state-file")
		label, _ := cmd.Flags().GetString("server-label")
		interval, _ := cmd.Flags().GetDuration("interval")
		if label == "" || path == "" || interval < 10*time.Second || interval > time.Hour {
			return fmt.Errorf("provide server-label, a private state-file and an interval between 10s and 1h")
		}
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
		defer stop()
		return notify.Watch(ctx, path, label, interval, providers)
	},
}

func init() {
	for _, cmd := range []*cobra.Command{alertSendCmd, alertWatchCmd} {
		cmd.Flags().String("telegram-config", "", "Private Telegram JSON file")
		cmd.Flags().String("smtp-config", "", "Private SMTP JSON file")
	}
	alertSendCmd.Flags().String("title", "", "Notification title (no secrets)")
	alertSendCmd.Flags().String("message", "", "Notification text (no secrets)")
	alertSendCmd.Flags().String("target", "host", "Server or project label")
	alertSendCmd.Flags().String("level", "info", "info, warning or critical")
	alertWatchCmd.Flags().String("state-file", "", "Private relay cursor and pending-delivery file")
	alertWatchCmd.Flags().String("server-label", "", "Operator-chosen server label")
	alertWatchCmd.Flags().Duration("interval", 30*time.Second, "Maximum polling frequency; events are summarized per batch")
	alertCmd.AddCommand(alertSendCmd, alertWatchCmd)
	rootCmd.AddCommand(alertCmd)
}
