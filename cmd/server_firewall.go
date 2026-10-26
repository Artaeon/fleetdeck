package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/fleetdeck/fleetdeck/internal/bootstrap"
	"github.com/spf13/cobra"
)

type localFirewallRunner struct{}

func (localFirewallRunner) Run(command string) (string, error) {
	out, err := exec.Command("sh", "-c", command).CombinedOutput()
	return string(out), err
}

var serverFirewallCmd = &cobra.Command{
	Use:   "firewall",
	Short: "Apply host and Docker ingress protection on this VPS",
	Long: `Apply IPv4/IPv6 Docker ingress protection and UFW host rules on this server.
Run with sudo on a dedicated Docker VPS. Preserves existing UFW rules and services.
Only TCP host ports 80 and 443 are permitted for new external forwarded connections.
Standard Docker bridges are supported; routers, VPN gateways, Kubernetes, Swarm,
and custom bridge interface names require an operator-managed firewall.
Use --docker-only to preserve an already managed host firewall completely.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Geteuid() != 0 {
			return fmt.Errorf("run this command with sudo on the target VPS")
		}
		dockerOnly, _ := cmd.Flags().GetBool("docker-only")
		port, _ := cmd.Flags().GetString("ssh-port")
		if dockerOnly {
			return bootstrap.ConfigureDockerFirewall(localFirewallRunner{})
		}
		return bootstrap.ConfigureFirewall(localFirewallRunner{}, port)
	},
}

func init() {
	serverFirewallCmd.Flags().String("ssh-port", "22", "SSH listener port to allow before enabling UFW")
	serverFirewallCmd.Flags().Bool("docker-only", false, "Keep host firewall unchanged; protect forwarded Docker traffic only")
	serverCmd.AddCommand(serverFirewallCmd)
}
