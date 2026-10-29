package cmd

import (
	"fmt"
	"github.com/fleetdeck/fleetdeck/internal/onboarding"
	"github.com/spf13/cobra"
	"os"
	"runtime"
)

func newServerUpdatesCommand() *cobra.Command {
	parent := &cobra.Command{Use: "updates", Short: "Plan or explicitly enable OS security updates", PersistentPreRun: func(*cobra.Command, []string) {}}
	for _, mode := range []string{"plan", "enable", "disable"} {
		mode := mode
		c := &cobra.Command{Use: mode, Short: map[string]string{"plan": "Show the supported OS security-update policy without changing it", "enable": "Enable security updates after backup and restart acknowledgement", "disable": "Restore the timer configuration from before FleetDeck management"}[mode], Args: cobra.NoArgs,
			RunE: func(c *cobra.Command, args []string) error {
				if runtime.GOOS != "linux" || os.Geteuid() != 0 {
					return fmt.Errorf("run this command with sudo on the target Linux server")
				}
				if mode == "enable" {
					backup, _ := c.Flags().GetBool("backup-verified")
					restarts, _ := c.Flags().GetBool("allow-service-restarts")
					if !backup || !restarts {
						return fmt.Errorf("enable requires --backup-verified and --allow-service-restarts; package scripts can restart services, but FleetDeck never schedules automatic reboots")
					}
				}
				window, _ := c.Flags().GetString("window")
				out, err := onboarding.ConfigureUpdates(localFirewallRunner{}, mode, window)
				if err != nil {
					return err
				}
				_, err = fmt.Fprint(c.OutOrStdout(), out)
				return err
			},
		}
		c.Flags().String("window", "04:00", "UTC maintenance start (HH:MM); randomized by up to 15 minutes")
		if mode == "enable" {
			c.Flags().Bool("backup-verified", false, "Confirm independent host/application recovery has been tested")
			c.Flags().Bool("allow-service-restarts", false, "Acknowledge package scripts may restart affected services")
		}
		parent.AddCommand(c)
	}
	return parent
}
func init() { serverCmd.AddCommand(newServerUpdatesCommand()) }
