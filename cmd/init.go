package cmd

import "github.com/spf13/cobra"

// Keep the familiar entry point without the old implicit network creation or
// unsafe raw Docker-socket proxy suggestion. Adoption starts with inspection.
var initCmd *cobra.Command

func init() {
	initCmd = newSetupCommand()
	initCmd.Use = "init"
	initCmd.Short = "Inspect this server before initializing FleetDeck (alias for setup)"
	rootCmd.AddCommand(initCmd)
}
