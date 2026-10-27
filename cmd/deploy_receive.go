package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/sourcedeploy"
	"github.com/spf13/cobra"
)

var deployReceiveCmd = &cobra.Command{
	Use: "receive", Short: "Receive a source archive over authenticated SSH and deploy with a verified server backup", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if runtime.GOOS != "linux" || os.Geteuid() != 0 {
			return fmt.Errorf("source receive runs as the trusted administrator with sudo on a Linux server")
		}
		name, _ := cmd.Flags().GetString("name")
		domain, _ := cmd.Flags().GetString("domain")
		profile, _ := cmd.Flags().GetString("profile")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		noCache, _ := cmd.Flags().GetBool("no-cache")
		if err := validateDomain(domain); err != nil {
			return err
		}
		result, err := sourcedeploy.New(cfg, openDB()).Apply(cmd.Context(), sourcedeploy.Options{Name: name, Domain: domain, Profile: profile, Timeout: timeout, NoCache: noCache}, cmd.InOrStdin(), cmd.ErrOrStderr())
		if encodeErr := json.NewEncoder(cmd.OutOrStdout()).Encode(result); encodeErr != nil {
			return encodeErr
		}
		return err
	},
}

func init() {
	deployReceiveCmd.Flags().String("name", "", "Project name")
	deployReceiveCmd.Flags().String("domain", "", "Project hostname")
	deployReceiveCmd.Flags().String("profile", "", "Project profile label")
	deployReceiveCmd.Flags().Duration("timeout", 10*time.Minute, "Maximum build and readiness window")
	deployReceiveCmd.Flags().Bool("no-cache", false, "Rebuild without cache")
	deployCmd.AddCommand(deployReceiveCmd)
}
