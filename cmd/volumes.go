package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/remote"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var volumesCmd = &cobra.Command{
	Use:   "volumes",
	Short: "Manage Docker volumes locally or on remote servers",
	Long: `Manage Docker volumes on the local machine or a remote server via SSH.

Examples:
  fleetdeck volumes list
  fleetdeck volumes list --server root@1.2.3.4
  fleetdeck volumes rm my-volume --force
  fleetdeck volumes rm my-volume --server root@1.2.3.4 --force`,
}

var volumesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List Docker volumes",
	RunE: func(cmd *cobra.Command, args []string) error {
		server, _ := cmd.Flags().GetString("server")

		dockerCmd := `docker volume ls --format "{{.Name}}\t{{.Driver}}\t{{.Mountpoint}}"`

		var output string
		if server != "" {
			client, err := connectForVolumes(cmd, server)
			if err != nil {
				return err
			}
			defer client.Close()

			out, err := client.Run(dockerCmd)
			if err != nil {
				return fmt.Errorf("listing remote volumes: %w", err)
			}
			output = out
		} else {
			out, err := exec.Command("sh", "-c", dockerCmd).CombinedOutput()
			if err != nil {
				return fmt.Errorf("listing local volumes: %w", err)
			}
			output = string(out)
		}

		output = strings.TrimSpace(output)
		if output == "" {
			ui.Info("No volumes found")
			return nil
		}

		headers := []string{"NAME", "DRIVER", "MOUNTPOINT"}
		var rows [][]string
		for _, line := range strings.Split(output, "\n") {
			parts := strings.SplitN(line, "\t", 3)
			if len(parts) == 3 {
				rows = append(rows, parts)
			}
		}

		ui.Table(headers, rows)
		return nil
	},
}

var volumesRmCmd = &cobra.Command{
	Use:   "rm <volume-name>",
	Short: "Remove a Docker volume",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		volumeName := args[0]
		server, _ := cmd.Flags().GetString("server")
		force, _ := cmd.Flags().GetBool("force")

		if !force {
			ui.Warn("Removing volume %q will permanently delete its data.", volumeName)
			ui.Info("Use --force to confirm removal.")
			return nil
		}

		dockerCmd := fmt.Sprintf("docker volume rm %s", shellQuote(volumeName))

		if server != "" {
			client, err := connectForVolumes(cmd, server)
			if err != nil {
				return err
			}
			defer client.Close()

			output, err := client.Run(dockerCmd)
			if err != nil {
				ui.Error("Failed to remove volume: %s", output)
				return fmt.Errorf("removing remote volume: %w", err)
			}
		} else {
			out, err := exec.Command("sh", "-c", dockerCmd).CombinedOutput()
			if err != nil {
				ui.Error("Failed to remove volume: %s", string(out))
				return fmt.Errorf("removing local volume: %w", err)
			}
		}

		ui.Success("Volume %q removed", volumeName)
		return nil
	},
}

// connectForVolumes establishes an SSH connection using the shared flag pattern.
func connectForVolumes(cmd *cobra.Command, server string) (*remote.Client, error) {
	port, _ := cmd.Flags().GetString("port")
	keyFile, _ := cmd.Flags().GetString("key")
	passphrase, _ := cmd.Flags().GetString("passphrase")
	if envPass := os.Getenv("FLEETDECK_SSH_PASSPHRASE"); envPass != "" {
		passphrase = envPass
	}

	host, user := parseTarget(server)

	var keyData []byte
	if keyFile != "" {
		data, err := os.ReadFile(keyFile)
		if err != nil {
			return nil, fmt.Errorf("reading SSH key: %w", err)
		}
		keyData = data
	} else {
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
			return nil, fmt.Errorf("no SSH key found; use --key to specify one")
		}
	}

	var passphraseBytes []byte
	if passphrase != "" {
		passphraseBytes = []byte(passphrase)
	}

	ui.Info("Connecting to %s@%s...", user, host)
	client, err := remote.NewClient(host, port, user, keyData, passphraseBytes)
	if err != nil {
		return nil, fmt.Errorf("SSH connection failed: %w", err)
	}
	ui.Success("Connected to %s", host)
	return client, nil
}

func init() {
	volumesCmd.PersistentFlags().String("server", "", "Remote server (user@host) for remote operations")
	volumesCmd.PersistentFlags().String("port", "22", "SSH port for remote operations")
	volumesCmd.PersistentFlags().String("key", "", "Path to SSH private key")
	volumesCmd.PersistentFlags().String("passphrase", "", "Passphrase for encrypted SSH private key")

	volumesRmCmd.Flags().Bool("force", false, "Skip confirmation and remove volume immediately")

	volumesCmd.AddCommand(volumesListCmd)
	volumesCmd.AddCommand(volumesRmCmd)
	rootCmd.AddCommand(volumesCmd)
}
