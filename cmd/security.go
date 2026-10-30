package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/deploy"
	"github.com/fleetdeck/fleetdeck/internal/securitywatch"
	"github.com/spf13/cobra"
)

var securityCmd = &cobra.Command{Use: "security", Short: "Check source secrets, updates, image vulnerabilities and runtime settings"}
var securityCheckCmd = &cobra.Command{
	Use: "check", Short: "Run read-only security checks; use the supplied systemd timer for recurring checks", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		notify, _ := cmd.Flags().GetBool("notify")
		path, _ := cmd.Flags().GetString("state-file")
		label, _ := cmd.Flags().GetString("server-label")
		repo, _ := cmd.Flags().GetString("repository")
		failFindings, _ := cmd.Flags().GetBool("fail-on-findings")
		scanTimeout, _ := cmd.Flags().GetDuration("scan-timeout")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		if err := validateSecurityTimeouts(scanTimeout, timeout); err != nil {
			return err
		}
		if repo == "" {
			repo = ReleaseRepository
		}
		if repo == "" {
			repo = "Artaeon/fleetdeck"
		}
		if notify && (path == "" || label == "") {
			return fmt.Errorf("notifications require a private --state-file and --server-label")
		}
		var state securitywatch.State
		if path != "" {
			var err error
			if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return err
			}

			lock, err := deploy.AcquireLock(filepath.Dir(path))
			if err != nil {
				return err
			}
			defer lock.Release()
			state, err = securitywatch.Load(path)
			if err != nil {
				return err
			}
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		defer cancel()
		checker := securitywatch.New()
		checker.ScanTimeout = scanTimeout
		report := checker.Run(ctx, repo, Version)
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(report); err != nil {
			return err
		}
		var notificationErr error
		if notify {
			providers, err := alertProviders(cmd)
			if err != nil {
				return err
			}
			notificationErr = securitywatch.Notify(&state, report, label, providers)
		} else {
			state.Report = report
		}
		if path != "" {
			if err := securitywatch.Save(path, state); err != nil {
				return err
			}
		}
		if notificationErr != nil {
			return notificationErr
		}
		critical, warning, unknown := report.Counts()
		if unknown > 0 {
			return fmt.Errorf("%d security checks are incomplete; current safety is unknown", unknown)
		}
		if failFindings && critical+warning > 0 {
			return fmt.Errorf("security findings require review")
		}
		return nil
	},
}

func validateSecurityTimeouts(scan, total time.Duration) error {
	if scan < time.Minute || scan > 30*time.Minute {
		return fmt.Errorf("--scan-timeout must be between 1m and 30m")
	}
	if total < 5*time.Minute || total > 2*time.Hour || total < scan {
		return fmt.Errorf("--timeout must be between 5m and 2h and at least --scan-timeout")
	}
	return nil
}

func init() {
	securityCheckCmd.Flags().Duration("scan-timeout", 5*time.Minute, "Maximum time for each image scan (1m–30m)")
	securityCheckCmd.Flags().Duration("timeout", 30*time.Minute, "Maximum time for all checks (5m–2h, at least scan-timeout)")
	securityCheckCmd.Flags().String("repository", "", "FleetDeck release repository (defaults to this build's repository)")
	securityCheckCmd.Flags().String("state-file", "", "Private report and notification delivery state")
	securityCheckCmd.Flags().String("server-label", "", "Operator-chosen label for security notifications")
	securityCheckCmd.Flags().Bool("notify", false, "Notify configured Telegram/SMTP channels on changed findings")
	securityCheckCmd.Flags().Bool("fail-on-findings", false, "Exit unsuccessfully on findings as well as incomplete checks")
	securityCheckCmd.Flags().String("telegram-config", "", "Private Telegram JSON file")
	securityCheckCmd.Flags().String("smtp-config", "", "Private SMTP JSON file")
	securityCmd.AddCommand(securityCheckCmd)
	rootCmd.AddCommand(securityCmd)
}
