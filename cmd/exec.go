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

var execCmd = &cobra.Command{
	Use:   "exec <project> [-- command...]",
	Short: "Run a command in a project's container",
	Long: `Execute a command inside a running container of a project.

If the project is deployed on a remote server, the command is executed
via SSH automatically.

Examples:
  fleetdeck exec myapp -- npm run migrate
  fleetdeck exec myapp -s postgres -- psql -U postgres mydb
  fleetdeck exec myapp -- bash
  fleetdeck exec myapp --server prod -- ls /app`,
	Args:                  cobra.MinimumNArgs(1),
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := args[0]
		service, _ := cmd.Flags().GetString("service")
		serverFlag, _ := cmd.Flags().GetString("server")
		noTTY, _ := cmd.Flags().GetBool("no-tty")

		// Everything after -- is the command to run
		dashArgs := cmd.ArgsLenAtDash()
		var execArgs []string
		if dashArgs >= 0 && dashArgs < len(args) {
			execArgs = args[dashArgs:]
		}
		if len(execArgs) == 0 {
			return fmt.Errorf("no command specified; use -- to separate the command (e.g. fleetdeck exec myapp -- bash)")
		}

		d := openDB()
		p, err := d.GetProject(projectName)
		if err != nil {
			return err
		}

		// Determine if we should run remotely
		isRemote := false
		serverName := serverFlag

		if serverName != "" {
			isRemote = true
		} else if p.ServerID != "" {
			// Look up the server by ID to get its name
			s, err := d.GetServerByID(p.ServerID)
			if err != nil {
				return fmt.Errorf("project has server_id %q but server not found: %w", p.ServerID, err)
			}
			serverName = s.Name
			isRemote = true
		}

		if isRemote {
			return execRemote(cmd, serverName, p.ProjectPath, service, noTTY, execArgs)
		}
		return execLocal(p.ProjectPath, service, noTTY, execArgs)
	},
}

func execLocal(projectPath, service string, noTTY bool, execArgs []string) error {
	composeArgs := []string{"compose", "exec"}
	if noTTY {
		composeArgs = append(composeArgs, "-T")
	}
	composeArgs = append(composeArgs, service)
	composeArgs = append(composeArgs, execArgs...)

	c := exec.Command("docker", composeArgs...)
	c.Dir = projectPath
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func execRemote(cmd *cobra.Command, serverName, projectPath, service string, noTTY bool, execArgs []string) error {
	d := openDB()
	s, err := d.GetServer(serverName)
	if err != nil {
		return fmt.Errorf("server %q not found: %w", serverName, err)
	}

	keyData, err := os.ReadFile(s.KeyPath)
	if err != nil {
		return fmt.Errorf("reading SSH key %s: %w", s.KeyPath, err)
	}

	passphrase, _ := cmd.Flags().GetString("passphrase")
	if envPass := os.Getenv("FLEETDECK_SSH_PASSPHRASE"); envPass != "" {
		passphrase = envPass
	}
	var passphraseBytes []byte
	if passphrase != "" {
		passphraseBytes = []byte(passphrase)
	}

	ui.Info("Connecting to %s (%s@%s)...", s.Name, s.User, s.Host)
	client, err := remote.NewClientTOFU(s.Host, s.Port, s.User, keyData, passphraseBytes)
	if err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}
	defer client.Close()

	// Build the remote docker compose exec command.
	// Remote SSH sessions are non-interactive, so always use -T.
	composeCmd := "cd " + shellQuote(projectPath) + " && docker compose exec -T"
	composeCmd += " " + shellQuote(service)
	for _, arg := range execArgs {
		composeCmd += " " + shellQuote(arg)
	}

	ui.Info("Running on %s: %s", s.Name, strings.Join(execArgs, " "))
	return client.RunStream(composeCmd, os.Stdout, os.Stderr)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func init() {
	execCmd.Flags().StringP("service", "s", "app", "Service to exec into")
	execCmd.Flags().String("server", "", "Force execution on a specific server")
	execCmd.Flags().Bool("no-tty", false, "Disable TTY allocation")
	execCmd.Flags().String("passphrase", "", "Passphrase for encrypted SSH private key")

	rootCmd.AddCommand(execCmd)
}
