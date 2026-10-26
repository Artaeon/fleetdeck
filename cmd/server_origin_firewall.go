package cmd

import (
	"fmt"
	"github.com/fleetdeck/fleetdeck/internal/bootstrap"
	"github.com/spf13/cobra"
	"os"
	"runtime"
)

func init() {
	command := &cobra.Command{
		Use:   "origin-firewall [preview|apply|confirm|rollback]",
		Short: "Restrict public web ingress to Cloudflare with a five-minute rollback trial",
		Long:  "Opt-in Cloudflare web ingress protection for dedicated Linux/systemd hosts.\nList ALL public interfaces with --interface. Covers host TCP 80/443 and Docker\npublished web ports; blocks public UDP 443, preserves SSH and other firewall rules.\nAll public sites must already be proxied and certificate renewal tested.\nRequires nftables with destroy support (1.0.9+) and systemd. Apply is temporary\nuntil confirmed within five minutes. Reapply to refresh provider IP ranges.",
		Args:  cobra.ExactArgs(1),
		// Recovery must work without project config; preview emits only nft syntax.
		PersistentPreRun: func(cmd *cobra.Command, args []string) {},
		RunE: func(cmd *cobra.Command, args []string) error {
			action := args[0]
			if action != "preview" && action != "apply" && action != "confirm" && action != "rollback" {
				return fmt.Errorf("expected preview, apply, confirm or rollback")
			}
			if action != "preview" && (runtime.GOOS != "linux" || os.Geteuid() != 0) {
				return fmt.Errorf("run with sudo on the target Linux VPS")
			}
			if action == "confirm" {
				if err := bootstrap.ConfirmOriginFirewall(localFirewallRunner{}); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Origin firewall confirmed and enabled at boot.")
				return nil
			}
			if action == "rollback" {
				return bootstrap.RollbackOriginFirewall(localFirewallRunner{})
			}
			interfaces, _ := cmd.Flags().GetStringSlice("interface")
			if len(interfaces) == 0 {
				return fmt.Errorf("--interface is required; list every public ingress interface")
			}
			prefixes, err := bootstrap.CloudflareRanges(cmd.Context())
			if err != nil {
				return err
			}
			rules, err := bootstrap.RenderOriginFirewall(interfaces, prefixes)
			if err != nil {
				return err
			}
			if action == "preview" {
				_, err = fmt.Fprint(cmd.OutOrStdout(), rules)
				return err
			}
			// A typo must never report successful protection on a nonexistent interface.
			for _, name := range interfaces {
				if _, err := os.Stat("/sys/class/net/" + name); err != nil {
					return fmt.Errorf("interface %s does not exist", name)
				}
			}
			if err := bootstrap.ApplyOriginFirewall(localFirewallRunner{}, rules); err != nil {
				return fmt.Errorf("origin firewall trial failed (armed rollback is retained): %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Trial active for five minutes. Verify access, then run: fleetdeck server origin-firewall confirm")
			return nil
		},
	}
	command.Flags().StringSlice("interface", nil, "Public ingress interface(s), e.g. eth0; repeat or comma-separate")
	serverCmd.AddCommand(command)
}
