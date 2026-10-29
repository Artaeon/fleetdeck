package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/db"
	"github.com/fleetdeck/fleetdeck/internal/onboarding"
	"github.com/spf13/cobra"
)

func newSetupCommand() *cobra.Command {
	c := &cobra.Command{
		Use: "setup", Short: "Inspect this server and guide safe FleetDeck adoption",
		Long: `Inspect this Linux server before installing or adopting workloads.
By default this command only reports a plan. --interactive walks through it and
can initialize the private FleetDeck registry. Existing applications, routing,
firewall rules, SSH policy and secrets are never rewritten by this assistant.
Run as root on the target server; Python 3 and systemd are required.`,
		Args: cobra.NoArgs,
		// The inventory must not initialize audit files, custom templates or a DB.
		PersistentPreRun: func(*cobra.Command, []string) {},
		RunE: func(c *cobra.Command, args []string) error {
			if runtime.GOOS != "linux" || os.Geteuid() != 0 {
				return fmt.Errorf("run sudo fleetdeck setup on the target Linux server")
			}
			port, _ := c.Flags().GetString("ssh-port")
			interactive, _ := c.Flags().GetBool("interactive")
			initialize, _ := c.Flags().GetBool("initialize")
			asJSON, _ := c.Flags().GetBool("json")
			if asJSON && (interactive || initialize) {
				return fmt.Errorf("--json is read-only and cannot be combined with --interactive or --initialize")
			}
			inv, err := onboarding.Inspect(localFirewallRunner{})
			if err != nil {
				return err
			}
			plan := onboarding.MakePlan(inv, port)
			if asJSON {
				return json.NewEncoder(c.OutOrStdout()).Encode(plan)
			}
			fmt.Fprintf(c.OutOrStdout(), "FleetDeck setup: %s\nOS: %s %s | containers: %d | other services: %d | listeners: %d\n", plan.Mode, inv.OS, inv.Version, inv.Containers, len(inv.OtherServices()), len(inv.Listeners))
			for _, reason := range plan.FreshBlockers {
				fmt.Fprintf(c.OutOrStdout(), "  Fresh installation blocked: %s\n", reason)
			}
			if len(inv.Unknown) > 0 {
				fmt.Fprintf(c.OutOrStdout(), "Incomplete probes: %s\n", strings.Join(inv.Unknown, ", "))
			}
			for n, step := range plan.Steps {
				fmt.Fprintf(c.OutOrStdout(), "\n%d. %s\n", n+1, step.Action)
				if step.Command != "" {
					fmt.Fprintf(c.OutOrStdout(), "   %s\n", step.Command)
				}
			}
			if !inv.Complete {
				return fmt.Errorf("inspection incomplete; no setup changes made")
			}
			if interactive && !initialize {
				fmt.Fprint(c.OutOrStdout(), "\nInitialize only the FleetDeck registry now? [y/N] ")
				answer, err := bufio.NewReader(c.InOrStdin()).ReadString('\n')
				if err != nil {
					return fmt.Errorf("no complete answer received; no setup changes made")
				}
				initialize = strings.EqualFold(strings.TrimSpace(answer), "y")
			}
			if initialize {
				conf, err := config.Load(cfgFile)
				if err != nil {
					return err
				}
				if err = initializeRegistry(conf); err != nil {
					return err
				}
				fmt.Fprintln(c.OutOrStdout(), "\nRegistry ready. Workload adoption and protection still require the steps above.")
			} else {
				fmt.Fprintln(c.OutOrStdout(), "\nInspection complete. No setup changes made.")
			}
			return nil
		},
	}
	c.Flags().String("ssh-port", "22", "Existing SSH listener port used for the inventory")
	c.Flags().Bool("json", false, "Return the read-only inventory and plan as JSON; keep this operator data private")
	c.Flags().Bool("interactive", false, "Walk through the plan and offer registry initialization")
	c.Flags().Bool("initialize", false, "Initialize only the FleetDeck registry after a complete inventory")
	return c
}

func initializeRegistry(conf *config.Config) error {
	path := conf.DBPath()
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("existing registry is not a regular file")
		}
		return nil // Leave an existing registry, including its schema, untouched.
	} else if !os.IsNotExist(err) {
		return err
	}
	// Refuse linked parent directories rather than following another user's path.
	for p := filepath.Dir(path); p != "/" && p != "."; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("registry parent must be a real directory")
		}
		if info.Mode().Perm()&0022 != 0 {
			return fmt.Errorf("registry parent must not be writable by other users")
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	// Reserve a private inode: db.Open must not create a world-readable file.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	d, err := db.Open(path)
	if err != nil {
		return err
	}
	return d.Close()
}
func init() { rootCmd.AddCommand(newSetupCommand()) }
