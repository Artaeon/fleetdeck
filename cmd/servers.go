package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/db"
	"github.com/fleetdeck/fleetdeck/internal/remote"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var serverAddCmd = &cobra.Command{
	Use:   "add <name> <user@host>",
	Short: "Register a remote server for deployments",
	Long: `Register a server so you can refer to it by name in deploy commands.

The server is tested for SSH connectivity during registration.

Examples:
  fleetdeck server add prod root@164.68.121.198
  fleetdeck server add staging deploy@staging.example.com --port 2222
  fleetdeck server add dev root@10.0.0.5 --key ~/.ssh/dev_key`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		target := args[1]
		host, user := parseTarget(target)

		port, _ := cmd.Flags().GetString("port")
		keyFile, _ := cmd.Flags().GetString("key")
		passphrase, _ := cmd.Flags().GetString("passphrase")

		// Resolve SSH key path
		if keyFile == "" {
			for _, path := range []string{
				os.ExpandEnv("$HOME/.ssh/id_ed25519"),
				os.ExpandEnv("$HOME/.ssh/id_rsa"),
			} {
				if _, err := os.Stat(path); err == nil {
					keyFile = path
					break
				}
			}
			if keyFile == "" {
				return fmt.Errorf("no SSH key found; use --key to specify one")
			}
		}

		d := openDB()

		// Check for duplicate
		if _, err := d.GetServer(name); err == nil {
			return fmt.Errorf("server %q already exists; use 'fleetdeck server remove %s' first", name, name)
		}

		// Test connectivity
		ui.Info("Testing SSH connection to %s@%s:%s...", user, host, port)
		keyData, err := os.ReadFile(keyFile)
		if err != nil {
			return fmt.Errorf("reading SSH key %s: %w", keyFile, err)
		}

		var passphraseBytes []byte
		if passphrase != "" {
			passphraseBytes = []byte(passphrase)
		}
		client, err := remote.NewClientTOFU(host, port, user, keyData, passphraseBytes)
		if err != nil {
			return fmt.Errorf("SSH connection failed: %w", err)
		}
		client.Close()
		ui.Success("Connection successful")

		// Store in database
		s := &db.Server{
			Name:    name,
			Host:    host,
			Port:    port,
			User:    user,
			KeyPath: keyFile,
			Status:  "active",
		}
		if err := d.CreateServer(s); err != nil {
			return fmt.Errorf("saving server: %w", err)
		}

		audit.Log("server.add", name, fmt.Sprintf("host=%s user=%s port=%s", host, user, port), true)
		ui.Success("Server %s registered (%s@%s)", ui.Bold(name), user, host)
		return nil
	},
}

var serverListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered servers",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()
		servers, err := d.ListServers()
		if err != nil {
			return err
		}

		if len(servers) == 0 {
			ui.Info("No servers registered. Use 'fleetdeck server add <name> <user@host>' to add one.")
			return nil
		}

		headers := []string{"Name", "Host", "Port", "User", "Status", "Key"}
		var rows [][]string
		for _, s := range servers {
			rows = append(rows, []string{
				s.Name,
				s.Host,
				s.Port,
				s.User,
				s.Status,
				s.KeyPath,
			})
		}
		fmt.Println()
		ui.Table(headers, rows)
		fmt.Println()

		return nil
	},
}

var serverRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a registered server",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		d := openDB()

		if err := d.DeleteServer(name); err != nil {
			return err
		}

		audit.Log("server.remove", name, "removed", true)
		ui.Success("Server %s removed", name)
		return nil
	},
}

var serverStatusCmd = &cobra.Command{
	Use:   "status [name]",
	Short: "Check connectivity of registered server(s)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()

		var servers []*db.Server
		if len(args) > 0 {
			s, err := d.GetServer(args[0])
			if err != nil {
				return err
			}
			servers = []*db.Server{s}
		} else {
			var err error
			servers, err = d.ListServers()
			if err != nil {
				return err
			}
		}

		if len(servers) == 0 {
			ui.Info("No servers registered.")
			return nil
		}

		for _, s := range servers {
			ui.Info("Checking %s (%s@%s:%s)...", s.Name, s.User, s.Host, s.Port)

			keyData, err := os.ReadFile(s.KeyPath)
			if err != nil {
				ui.Error("  Cannot read key %s: %v", s.KeyPath, err)
				d.UpdateServerStatus(s.Name, "unreachable")
				continue
			}

			passphrase, _ := cmd.Flags().GetString("passphrase")
			var passphraseBytes []byte
			if passphrase != "" {
				passphraseBytes = []byte(passphrase)
			}
			client, err := remote.NewClientTOFU(s.Host, s.Port, s.User, keyData, passphraseBytes)
			if err != nil {
				ui.Error("  Connection failed: %v", err)
				d.UpdateServerStatus(s.Name, "unreachable")
				continue
			}

			// Quick test: run hostname
			hostname, err := client.Run("hostname")
			client.Close()
			if err != nil {
				ui.Error("  Connected but command failed: %v", err)
				d.UpdateServerStatus(s.Name, "unreachable")
				continue
			}

			d.UpdateServerStatus(s.Name, "active")
			ui.Success("  %s is reachable (hostname: %s)", s.Name, strings.TrimSpace(hostname))
		}

		return nil
	},
}

func init() {
	serverAddCmd.Flags().String("port", "22", "SSH port")
	serverAddCmd.Flags().String("key", "", "Path to SSH private key")
	serverAddCmd.Flags().String("passphrase", "", "Passphrase for encrypted SSH private key")

	serverStatusCmd.Flags().String("passphrase", "", "Passphrase for encrypted SSH private key")

	serverCmd.AddCommand(serverAddCmd)
	serverCmd.AddCommand(serverListCmd)
	serverCmd.AddCommand(serverRemoveCmd)
	serverCmd.AddCommand(serverStatusCmd)
}
