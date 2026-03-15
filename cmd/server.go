package cmd

import (
	"fmt"
	"os"

	"github.com/fleetdeck/fleetdeck/internal/bootstrap"
	"github.com/fleetdeck/fleetdeck/internal/remote"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Server management commands",
	Long:  `Commands for provisioning and managing remote servers.`,
}

var serverSetupCmd = &cobra.Command{
	Use:   "setup <user@host>",
	Short: "Bootstrap a fresh server for FleetDeck deployments",
	Long: `Provisions a fresh Ubuntu/Debian server with everything needed:
- System updates and essential packages
- Docker Engine and Compose plugin
- Traefik reverse proxy with automatic HTTPS
- UFW firewall (SSH, HTTP, HTTPS only)
- Swap file for memory safety
- SSH hardening (disable password auth)

The server must be accessible via SSH with key-based auth.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		host, user := parseTarget(target)

		port, _ := cmd.Flags().GetString("port")
		keyFile, _ := cmd.Flags().GetString("key")
		domain, _ := cmd.Flags().GetString("domain")
		email, _ := cmd.Flags().GetString("email")
		swapGB, _ := cmd.Flags().GetInt("swap")
		network, _ := cmd.Flags().GetString("traefik-network")

		if domain == "" {
			return fmt.Errorf("--domain is required for Traefik setup")
		}
		if email == "" {
			return fmt.Errorf("--email is required for Let's Encrypt certificates")
		}

		// Read SSH private key
		var keyData []byte
		if keyFile != "" {
			var err error
			keyData, err = os.ReadFile(keyFile)
			if err != nil {
				return fmt.Errorf("reading SSH key %s: %w", keyFile, err)
			}
		} else {
			// Try default SSH key locations
			for _, path := range []string{
				os.ExpandEnv("$HOME/.ssh/id_ed25519"),
				os.ExpandEnv("$HOME/.ssh/id_rsa"),
			} {
				data, err := os.ReadFile(path)
				if err == nil {
					keyData = data
					break
				}
			}
			if keyData == nil {
				return fmt.Errorf("no SSH key found; use --key to specify one")
			}
		}

		// Connect to server
		insecure, _ := cmd.Flags().GetBool("insecure")
		ui.Step(1, 6, "Connecting to %s@%s...", user, host)
		var (
			client  *remote.Client
			connErr error
		)
		if insecure {
			ui.Warn("Skipping SSH host key verification (--insecure)")
			client, connErr = remote.NewClientInsecure(host, port, user, keyData)
		} else {
			client, connErr = remote.NewClient(host, port, user, keyData)
		}
		if connErr != nil {
			return fmt.Errorf("SSH connection failed: %w", connErr)
		}
		defer client.Close()
		ui.Success("Connected to %s", host)

		// Run bootstrap
		ui.Step(2, 6, "Starting server provisioning...")
		fmt.Println()

		bCfg := bootstrap.Config{
			Host:           host,
			Port:           port,
			User:           user,
			Domain:         domain,
			Email:          email,
			SwapSizeGB:     swapGB,
			TraefikNetwork: network,
		}

		result, err := bootstrap.Bootstrap(bCfg, client)
		if err != nil {
			return fmt.Errorf("bootstrap failed: %w", err)
		}

		// Report results
		fmt.Println()
		if result.DockerInstalled {
			ui.Success("Docker installed and verified")
		}
		if result.TraefikConfigured {
			ui.Success("Traefik configured with HTTPS")
		}
		if result.FirewallConfigured {
			ui.Success("Firewall configured (SSH, HTTP, HTTPS)")
		}
		if result.SwapCreated {
			ui.Success("Swap file created (%dGB)", swapGB)
		}

		if len(result.Errors) > 0 {
			fmt.Println()
			ui.Warn("Non-critical issues:")
			for _, e := range result.Errors {
				fmt.Printf("  - %s\n", e)
			}
		}

		fmt.Println()
		ui.Success("Server %s is ready for FleetDeck deployments!", ui.Bold(host))
		fmt.Println()
		ui.Info("Next steps:")
		fmt.Printf("  1. Point your domain DNS to %s\n", host)
		fmt.Printf("  2. Deploy: fleetdeck deploy ./app --server %s@%s --domain %s\n", user, host, domain)
		fmt.Println()

		return nil
	},
}

func parseTarget(target string) (host, user string) {
	user = "root"
	host = target
	for i, c := range target {
		if c == '@' {
			user = target[:i]
			host = target[i+1:]
			return
		}
	}
	return
}

func init() {
	serverSetupCmd.Flags().String("port", "22", "SSH port")
	serverSetupCmd.Flags().String("key", "", "Path to SSH private key")
	serverSetupCmd.Flags().String("domain", "", "Domain for Traefik dashboard (required)")
	serverSetupCmd.Flags().String("email", "", "Email for Let's Encrypt certificates (required)")
	serverSetupCmd.Flags().Int("swap", 2, "Swap file size in GB")
	serverSetupCmd.Flags().String("traefik-network", "traefik_default", "Docker network for Traefik")
	serverSetupCmd.Flags().Bool("insecure", false, "Skip SSH host key verification")

	serverCmd.AddCommand(serverSetupCmd)
	rootCmd.AddCommand(serverCmd)
}
