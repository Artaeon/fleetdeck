package cmd

import (
	"fmt"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/profiles"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var profilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "List available deployment profiles",
	Long: `Deployment profiles define what services your application needs.
Each profile is a composable stack of containers:

  bare       - App only, no extras
  server     - App + PostgreSQL + Redis
  saas       - App + PostgreSQL + Redis + S3 + Email
  static     - Nginx with CDN headers
  worker     - Background jobs with Redis queue
  fullstack  - Frontend + Backend + DB + Redis + S3`,
	RunE: func(cmd *cobra.Command, args []string) error {
		allProfiles := profiles.List()

		fmt.Println()
		ui.Info("Available deployment profiles:")
		fmt.Println()

		headers := []string{"Profile", "Description", "Services"}
		var rows [][]string

		for _, p := range allProfiles {
			var serviceNames []string
			for _, s := range p.Services {
				name := s.Name
				if !s.Required {
					name += " (opt)"
				}
				serviceNames = append(serviceNames, name)
			}
			rows = append(rows, []string{
				ui.Bold(p.Name),
				p.Description,
				strings.Join(serviceNames, ", "),
			})
		}

		ui.Table(headers, rows)
		fmt.Println()
		ui.Info("Use with: fleetdeck create <name> --profile <profile> --domain <domain>")
		fmt.Println()

		return nil
	},
}

var profileInfoCmd = &cobra.Command{
	Use:   "profile <name>",
	Short: "Show details of a deployment profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := profiles.Get(args[0])
		if err != nil {
			return err
		}

		showCompose, _ := cmd.Flags().GetBool("compose")

		fmt.Println()
		ui.Success("Profile: %s", ui.Bold(p.Name))
		fmt.Println()
		ui.Info(p.Description)
		fmt.Println()

		ui.Info("Services:")
		for _, s := range p.Services {
			required := "required"
			if !s.Required {
				required = "optional"
			}
			fmt.Printf("  - %-12s  %-35s  (%s)\n", s.Name, s.Description, required)
		}
		fmt.Println()

		if showCompose {
			ui.Info("Docker Compose template:")
			fmt.Println()
			fmt.Println(p.Compose)
		}

		return nil
	},
}

func init() {
	profileInfoCmd.Flags().Bool("compose", false, "Show the Docker Compose template")

	rootCmd.AddCommand(profilesCmd)
	rootCmd.AddCommand(profileInfoCmd)
}
