package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/fleetdeck/fleetdeck/internal/monitor"
	"github.com/fleetdeck/fleetdeck/internal/notify"
	"github.com/fleetdeck/fleetdeck/internal/securityreport"
	"github.com/spf13/cobra"
)

func newAlertDeliveriesCmd() *cobra.Command {
	var health, relay, report string
	c := &cobra.Command{Use: "deliveries", Short: "List waiting notifications, channels and queue times from private state files", Args: cobra.NoArgs,
		PersistentPreRun: func(*cobra.Command, []string) {},
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := map[string]monitor.Backlog{}
			if health != "" {
				s, err := monitor.ReadDeliveryStatus(health)
				if err != nil {
					return fmt.Errorf("health queue: %w", err)
				}
				out["health"] = s.Backlog
			}
			if relay != "" {
				s, err := notify.ReadBacklog(relay)
				if err != nil {
					return fmt.Errorf("host activity queue: %w", err)
				}
				out["host_activity"] = s
			}
			if report != "" {
				s, err := securityreport.ReadBacklog(report)
				if err != nil {
					return fmt.Errorf("report queue: %w", err)
				}
				out["security_report"] = s
			}
			if len(out) == 0 {
				return fmt.Errorf("supply at least one of --health-state, --relay-state or --report-state")
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		}}
	c.Flags().StringVar(&health, "health-state", "", "Private monitor alert queue")
	c.Flags().StringVar(&relay, "relay-state", "", "Private alert watch state file")
	c.Flags().StringVar(&report, "report-state", "", "Private security report delivery state file")
	return c
}
func init() { alertCmd.AddCommand(newAlertDeliveriesCmd()) }
