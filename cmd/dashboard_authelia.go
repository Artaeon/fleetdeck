package cmd

import (
	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/server"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var dashboardAutheliaCmd = &cobra.Command{
	Use:   "setup-authelia",
	Short: "Create private Authelia client settings without changing a running service",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		issuer, _ := cmd.Flags().GetString("issuer")
		publicURL, _ := cmd.Flags().GetString("public-url")
		group, _ := cmd.Flags().GetString("admin-group")
		method, _ := cmd.Flags().GetString("authentication-method")
		output, _ := cmd.Flags().GetString("output")
		if err := server.WriteAutheliaSetup(output, config.OIDCConfig{Issuer: issuer, PublicURL: publicURL, AdminGroup: group, AuthenticationMethod: method}); err != nil {
			return err
		}
		ui.Success("Created private authentication files in %s", output)
		ui.Info("Merge authelia-client.yml into your Authelia OIDC clients and fleetdeck-oidc.toml into your FleetDeck config.")
		ui.Info("Keep client-secret private. Validate Authelia and enroll two security keys before enabling the dashboard.")
		return nil
	},
}

func init() {
	dashboardAutheliaCmd.Flags().String("issuer", "", "Authelia HTTPS issuer URL")
	dashboardAutheliaCmd.Flags().String("public-url", "", "FleetDeck HTTPS public origin")
	dashboardAutheliaCmd.Flags().String("admin-group", "fleetdeck-admins", "Exact Authelia group allowed to administer FleetDeck")
	dashboardAutheliaCmd.Flags().String("authentication-method", "webauthn", "Required MFA method: webauthn, hardware, or mfa")
	dashboardAutheliaCmd.Flags().String("output", "", "New private directory outside any Git repository")
	for _, name := range []string{"issuer", "public-url", "output"} {
		_ = dashboardAutheliaCmd.MarkFlagRequired(name)
	}
	dashboardCmd.AddCommand(dashboardAutheliaCmd)
}
