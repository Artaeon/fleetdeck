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
		Use:   "ssh-mfa [preview|apply|confirm|rollback]",
		Short: "Trial key-plus-authenticator SSH access on a prepared Debian/Ubuntu host",
		Args:  cobra.ExactArgs(1), PersistentPreRun: func(*cobra.Command, []string) {},
		RunE: func(cmd *cobra.Command, args []string) error {
			user, _ := cmd.Flags().GetString("user")
			if args[0] == "preview" {
				text, err := bootstrap.SSHMFASettings(user)
				if err != nil {
					return err
				}
				fmt.Fprint(cmd.OutOrStdout(), text)
				return nil
			}
			if runtime.GOOS != "linux" || os.Geteuid() != 0 {
				return fmt.Errorf("run with sudo on the target Linux host")
			}
			switch args[0] {
			case "apply":
				if err := bootstrap.ApplySSHMFA(localFirewallRunner{}, user); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Five-minute SSH MFA trial active. Open a fresh key-plus-code SSH connection, then run: sudo fleetdeck server ssh-mfa confirm")
			case "confirm":
				return bootstrap.ConfirmSSHMFA(localFirewallRunner{})
			case "rollback":
				return bootstrap.RollbackSSHMFA(localFirewallRunner{})
			default:
				return fmt.Errorf("expected preview, apply, confirm or rollback")
			}
			return nil
		},
	}
	command.Flags().String("user", "", "Enrolled non-root administrator")
	serverCmd.AddCommand(command)
}
