package cmd

import (
	"fmt"
	"github.com/fleetdeck/fleetdeck/internal/monitor"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
	"time"
)

var smtpCmd = &cobra.Command{Use: "smtp", Short: "Validate encrypted notification delivery"}
var smtpTestCmd = &cobra.Command{Use: "test", Short: "Send one test message using a private SMTP configuration", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
	path, _ := cmd.Flags().GetString("smtp-config")
	to, _ := cmd.Flags().GetStringSlice("to")
	if path == "" {
		path = cfg.Monitoring.SMTPConfigFile
	}
	if len(to) == 0 {
		to = cfg.Monitoring.EmailTo
	}
	if path == "" {
		return fmt.Errorf("specify --smtp-config or monitoring.smtp_config_file")
	}
	provider, err := monitor.LoadEmailProvider(path, to)
	if err != nil {
		return err
	}
	if err := provider.Send(monitor.Alert{Level: "info", Title: "Notification delivery test", Target: "FleetDeck", Message: "FleetDeck connected using verified TLS and authenticated successfully. This test contains no project secrets.", Timestamp: time.Now()}); err != nil {
		return err
	}
	ui.Success("The SMTP server accepted the test message over verified TLS. Check the recipient inbox to confirm delivery.")
	return nil
}}

func init() {
	smtpTestCmd.Flags().String("smtp-config", "", "Private SMTP JSON file")
	smtpTestCmd.Flags().StringSlice("to", nil, "Test recipient addresses")
	smtpCmd.AddCommand(smtpTestCmd)
	rootCmd.AddCommand(smtpCmd)
}
