package cmd

import (
	"fmt"
	"io"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/fleet"
	"github.com/fleetdeck/fleetdeck/internal/monitor"
	"github.com/spf13/cobra"
)

func deliveryCondition(input io.Reader, now time.Time, maxAge time.Duration) string {
	runs, err := fleet.DecodeReportRuns(input, now)
	switch {
	case err != nil:
		return "unavailable"
	case now.Sub(runs.Daily.LastEvaluation) > maxAge || now.Sub(runs.Weekly.LastEvaluation) > maxAge:
		return "stale"
	case runs.Daily.Pending > 0 || runs.Weekly.Pending > 0:
		return "pending"
	default:
		return "current"
	}
}

func deliveryNotice(status, language string) *monitor.NotificationText {
	if status == "current" {
		if language == "de" {
			return &monitor.NotificationText{Level: "info", Title: "Berichtsversand wieder erreichbar", Message: "Tages- und Wochenbericht wurden rechtzeitig geprüft. Beide Versandlisten sind leer."}
		}
		return &monitor.NotificationText{Level: "info", Title: "Report delivery recovered", Message: "Both report jobs have evaluated recently and their delivery queues are empty."}
	}
	if language == "de" {
		msg := "Der Berichtsstatus ist nicht erreichbar. Erreichbarkeit des Servers und die geschützte Prüfverbindung kontrollieren."
		if status == "stale" {
			msg = "Der letzte Prüflauf ist überfällig. Die Zeitgeber und letzten Läufe des Tages- und Wochenberichts prüfen."
		} else if status == "pending" {
			msg = "Mindestens ein Bericht wartet auf Zustellung. Die Versandliste und den Zugang zum eingerichteten Empfänger prüfen."
		}
		return &monitor.NotificationText{Level: "warning", Title: "Berichtsversand prüfen", Message: msg}
	}
	msg := "The reporting host could not be verified. Check the host and its private status connection."
	if status == "stale" {
		msg = "A reporting job missed its evaluation deadline. Check the daily and weekly timers and their last runs."
	} else if status == "pending" {
		msg = "At least one report is waiting for delivery. Check the delivery queue and the configured recipient."
	}
	return &monitor.NotificationText{Level: "warning", Title: "Report delivery needs attention", Message: msg}
}

// Failed authenticated fetches must pass empty input so an unavailable host is observed.
func newDeliveryCheckCmd() *cobra.Command {
	var node, state, language, channel string
	var maxAge time.Duration
	var threshold int
	c := &cobra.Command{Use: "check-report-delivery", Short: "Independently check the daily and weekly reporting heartbeat from stdin", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if !fleet.ValidID(node) || maxAge < time.Minute || maxAge > 24*time.Hour || threshold < 1 || threshold > 12 {
			return fmt.Errorf("provide a neutral node ID, a valid maximum age and a failure threshold")
		}
		if (language != "de" && language != "en") || (channel != "" && channel != "telegram" && channel != "email") {
			return fmt.Errorf("select a supported language and notification channel")
		}
		now := time.Now().UTC()
		cause := deliveryCondition(cmd.InOrStdin(), now, maxAge)
		fmt.Fprintln(cmd.OutOrStdout(), cause)
		if state != "" {
			providers, err := alertProviders(cmd)
			if err != nil {
				return err
			}
			manager, err := monitor.NewDurableAlertManager(providers, threshold, state)
			if err != nil {
				return err
			}
			defer manager.Close()
			result := monitor.CheckResult{Target: monitor.Target{Name: node, URL: "fleetdeck-report://" + node}, CheckedAt: now, Healthy: cause == "current", Notification: deliveryNotice(cause, language)}
			if err := manager.Process(result); err != nil {
				return err
			}
			if err := manager.RetryPending(now); err != nil {
				return err
			}
		}
		if cause != "current" {
			return fmt.Errorf("report delivery check: %s", cause)
		}
		return nil
	}}
	c.Flags().StringVar(&node, "node-id", "", "Enrolled reporting host")
	c.Flags().DurationVar(&maxAge, "max-age", 2*time.Hour, "Maximum time since either job evaluated")
	c.Flags().IntVar(&threshold, "failure-threshold", 2, "Consecutive failed checks needed to notify")
	c.Flags().StringVar(&state, "alert-state", "", "Enable alerts using a separate private absolute delivery state")
	c.Flags().StringVar(&language, "language", "en", "Alert language: en or de")
	c.Flags().StringVar(&channel, "channel", "", "Select telegram or email; defaults to all configured channels")
	c.Flags().String("telegram-config", "", "Private Telegram settings")
	c.Flags().String("smtp-config", "", "Private SMTP settings")
	return c
}

func init() { securityCmd.AddCommand(newDeliveryCheckCmd()) }
