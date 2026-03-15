package cmd

import (
	"fmt"
	"net"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/dns"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

// validateDomain checks that a domain looks reasonable.
func validateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain must not be empty")
	}
	if !strings.Contains(domain, ".") {
		return fmt.Errorf("domain %q must contain at least one dot", domain)
	}
	if strings.ContainsAny(domain, " \t\n\"'`;$\\{}()") {
		return fmt.Errorf("domain %q contains invalid characters", domain)
	}
	return nil
}

// validateIP checks that an IP address is valid.
func validateIP(ip string) error {
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("%q is not a valid IP address", ip)
	}
	return nil
}

var dnsCmd = &cobra.Command{
	Use:   "dns",
	Short: "DNS record management",
	Long: `Manage DNS records for your projects via supported providers.

Currently supports:
  - Cloudflare (--provider cloudflare --token <api-token>)`,
}

var dnsSetupCmd = &cobra.Command{
	Use:   "setup <domain> <server-ip>",
	Short: "Auto-configure DNS records for a domain",
	Long: `Creates A records for the root domain and wildcard subdomain
pointing to the specified server IP.

Example:
  fleetdeck dns setup example.com 143.198.1.1 --provider cloudflare --token cf_xxx`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		serverIP := args[1]

		if err := validateDomain(domain); err != nil {
			return err
		}
		if err := validateIP(serverIP); err != nil {
			return err
		}

		providerName, _ := cmd.Flags().GetString("provider")
		token, _ := cmd.Flags().GetString("token")

		if token == "" {
			return fmt.Errorf("--token is required for DNS provider authentication")
		}

		provider, err := dns.GetProvider(providerName, token)
		if err != nil {
			return err
		}

		ui.Info("Configuring DNS for %s via %s...", domain, provider.Name())
		fmt.Println()

		if err := dns.AutoConfigure(domain, serverIP, provider); err != nil {
			return fmt.Errorf("DNS auto-configuration failed: %w", err)
		}

		fmt.Println()
		ui.Success("DNS configured for %s", ui.Bold(domain))
		ui.Info("  %s → %s", domain, serverIP)
		ui.Info("  *.%s → %s", domain, serverIP)
		fmt.Println()
		ui.Info("DNS propagation may take a few minutes.")
		fmt.Println()

		return nil
	},
}

var dnsListCmd = &cobra.Command{
	Use:   "list <domain>",
	Short: "List DNS records for a domain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]

		providerName, _ := cmd.Flags().GetString("provider")
		token, _ := cmd.Flags().GetString("token")

		if token == "" {
			return fmt.Errorf("--token is required")
		}

		provider, err := dns.GetProvider(providerName, token)
		if err != nil {
			return err
		}

		records, err := provider.ListRecords(domain)
		if err != nil {
			return fmt.Errorf("listing records: %w", err)
		}

		if len(records) == 0 {
			ui.Info("No DNS records found for %s", domain)
			return nil
		}

		fmt.Println()
		headers := []string{"Type", "Name", "Value", "TTL", "Proxied"}
		var rows [][]string
		for _, r := range records {
			proxied := ""
			if r.Proxied {
				proxied = "yes"
			}
			rows = append(rows, []string{
				r.Type,
				r.Name,
				r.Value,
				fmt.Sprintf("%d", r.TTL),
				proxied,
			})
		}
		ui.Table(headers, rows)
		fmt.Println()

		return nil
	},
}

var dnsDeleteCmd = &cobra.Command{
	Use:   "delete <domain> <record-type> <name>",
	Short: "Delete a DNS record",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		recordType := args[1]
		name := args[2]

		providerName, _ := cmd.Flags().GetString("provider")
		token, _ := cmd.Flags().GetString("token")

		if token == "" {
			return fmt.Errorf("--token is required")
		}

		provider, err := dns.GetProvider(providerName, token)
		if err != nil {
			return err
		}

		if err := provider.DeleteRecord(domain, recordType, name); err != nil {
			return fmt.Errorf("deleting record: %w", err)
		}

		ui.Success("Deleted %s record %s from %s", recordType, name, domain)
		return nil
	},
}

func init() {
	// Add persistent flags to dns command (inherited by subcommands)
	dnsCmd.PersistentFlags().String("provider", "cloudflare", "DNS provider (cloudflare)")
	dnsCmd.PersistentFlags().String("token", "", "API token for the DNS provider")

	dnsCmd.AddCommand(dnsSetupCmd)
	dnsCmd.AddCommand(dnsListCmd)
	dnsCmd.AddCommand(dnsDeleteCmd)
	rootCmd.AddCommand(dnsCmd)
}
