package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "fleetdeck",
	Short: "Lightweight self-hosted deployment platform",
	Long: `FleetDeck is a CLI-first deployment platform for developers who run
multiple Docker projects on a single server with Traefik.

It automates Linux user creation, SSH key generation, GitHub repo setup,
Docker Compose configuration, and CI/CD workflow generation.`,
}

func Execute() error {
	return rootCmd.Execute()
}
