package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/releasejob"
	"github.com/spf13/cobra"
)

// Install behind an operator-owned OpenSSH ForceCommand and an exact sudoers
// entry. Never interpolate SSH_ORIGINAL_COMMAND into a shell or arguments.
var releaseReceiveCmd = &cobra.Command{
	Use: "receive", Short: "Receive a bounded staging job for one fixed CI target", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		target, _ := cmd.Flags().GetString("target")
		policy, err := releasejob.ReceiverPolicy(cfg, target)
		if err != nil {
			return err
		}
		action := os.Getenv("SSH_ORIGINAL_COMMAND")
		if action != "apply" && action != "status" {
			return fmt.Errorf("CI accepts only the literal commands apply and status")
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Minute)
		defer cancel()
		if action == "status" {
			current, err := openDB().CurrentRelease(ctx, target)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
				Target  string  `json:"target"`
				Current *string `json:"current_release_id"`
			}{target, current})
		}
		request, err := releasejob.Decode(cmd.InOrStdin())
		if err != nil {
			return err
		}
		if err = releasejob.ValidateReceived(request, target, policy); err != nil {
			return err
		}
		database := openDB()
		runtime := releasejob.NewComposeRuntime(cfg, releaseProjectStore{database: database})
		result, executionErr := releasejob.NewExecutor(database, runtime).Execute(ctx, request)
		audit.Log("release.receive", target, result.ErrorCode, executionErr == nil)
		if result.JobID != "" {
			if err = json.NewEncoder(cmd.OutOrStdout()).Encode(result); err != nil {
				return err
			}
		}
		return executionErr
	},
}

func init() {
	releaseReceiveCmd.Flags().String("target", "", "Fixed target selected by the server's forced command")
	releaseCmd.AddCommand(releaseReceiveCmd)
}
