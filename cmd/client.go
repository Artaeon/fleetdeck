package cmd

import (
	"fmt"
	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/spf13/cobra"
)

var clientCmd = &cobra.Command{Use: "client", Short: "Configure this workstation for named remote deployments"}
var clientInitCmd = &cobra.Command{
	Use: "init", Short: "Create private local FleetDeck configuration outside project repositories",
	Args: cobra.NoArgs,
	// Initializing client state must not initialize /var/log or server services.
	PersistentPreRun: func(*cobra.Command, []string) {},
	RunE: func(cmd *cobra.Command, args []string) error {
		path := cfgFile
		if path == "" {
			var err error
			path, err = config.UserConfigPath()
			if err != nil {
				return err
			}
		}
		c, err := config.InitClient(path)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Client configuration:", path)
		fmt.Fprintln(cmd.OutOrStdout(), "Private server inventory:", c.DBPath())
		fmt.Fprintln(cmd.OutOrStdout(), "Next: fleetdeck server add <name> <user@host>")
		return nil
	},
}

func init() { clientCmd.AddCommand(clientInitCmd); rootCmd.AddCommand(clientCmd) }
