package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var domainCmd = &cobra.Command{
	Use:   "domain",
	Short: "Manage project domains",
}

var domainSetCmd = &cobra.Command{
	Use:   "set <project> <new-domain>",
	Short: "Change the domain for a project",
	Long: `Updates a project's domain by rewriting the Traefik labels in its
docker-compose.yml and updating the database record.

After changing the domain, you need to:
  1. Point the new domain's DNS to your server
  2. Restart the project: fleetdeck restart <project>

Examples:
  fleetdeck domain set myapp app.newdomain.com
  fleetdeck domain set seitenwind seitenwind.at`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := args[0]
		newDomain := args[1]

		if err := validateDomain(newDomain); err != nil {
			return err
		}

		d := openDB()
		p, err := d.GetProject(projectName)
		if err != nil {
			return err
		}

		oldDomain := p.Domain
		if oldDomain == newDomain {
			ui.Info("Project %s already uses domain %s", projectName, newDomain)
			return nil
		}

		// Update docker-compose.yml — replace all occurrences of the old domain
		composePath := filepath.Join(p.ProjectPath, "docker-compose.yml")
		data, err := os.ReadFile(composePath)
		if err != nil {
			return fmt.Errorf("reading docker-compose.yml: %w", err)
		}

		original := string(data)
		updated := strings.ReplaceAll(original, oldDomain, newDomain)
		if original == updated {
			ui.Warn("No domain references found in docker-compose.yml")
		}

		if err := os.WriteFile(composePath, []byte(updated), 0644); err != nil {
			return fmt.Errorf("writing docker-compose.yml: %w", err)
		}

		// Update .env if it references the old domain
		envPath := filepath.Join(p.ProjectPath, ".env")
		if envData, err := os.ReadFile(envPath); err == nil {
			envOriginal := string(envData)
			envUpdated := strings.ReplaceAll(envOriginal, oldDomain, newDomain)
			if envOriginal != envUpdated {
				if err := os.WriteFile(envPath, []byte(envUpdated), 0600); err != nil {
					ui.Warn("Could not update .env: %v", err)
				}
			}
		}

		// Update database
		p.Domain = newDomain
		if err := d.UpdateProject(p); err != nil {
			return fmt.Errorf("updating database: %w", err)
		}

		audit.Log("domain.set", projectName, fmt.Sprintf("%s -> %s", oldDomain, newDomain), true)

		fmt.Println()
		ui.Success("Domain changed: %s -> %s", ui.Bold(oldDomain), ui.Bold(newDomain))
		fmt.Println()
		ui.Info("Next steps:")
		fmt.Printf("  1. Point %s DNS to your server\n", newDomain)
		fmt.Printf("  2. Restart: fleetdeck restart %s\n", projectName)
		fmt.Println()

		return nil
	},
}

var domainGetCmd = &cobra.Command{
	Use:   "get <project>",
	Short: "Show the current domain for a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()
		p, err := d.GetProject(args[0])
		if err != nil {
			return err
		}
		fmt.Println(p.Domain)
		return nil
	},
}

func init() {
	domainCmd.AddCommand(domainSetCmd)
	domainCmd.AddCommand(domainGetCmd)
	rootCmd.AddCommand(domainCmd)
}
