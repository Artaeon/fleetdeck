package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/fleetdeck/fleetdeck/internal/releasejob"
	"github.com/spf13/cobra"
)

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Validate and execute bounded immutable release jobs",
}

var releaseValidateCmd = &cobra.Command{
	Use:   "validate <job.json|->",
	Short: "Validate a release job without causing side effects",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var input io.Reader
		if args[0] == "-" {
			input = cmd.InOrStdin()
		} else {
			file, err := os.Open(args[0])
			if err != nil {
				return fmt.Errorf("open release job: %w", err)
			}
			defer file.Close()
			input = file
		}
		return validateReleaseJob(input, cmd.OutOrStdout())
	},
}

func validateReleaseJob(input io.Reader, output io.Writer) error {
	request, err := releasejob.Decode(input)
	if err != nil {
		return err
	}
	requestSHA256, err := releasejob.RequestSHA256(request)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(struct {
		Status         string `json:"status"`
		SchemaVersion  int    `json:"schema_version"`
		JobID          string `json:"job_id"`
		ReleaseID      string `json:"release_id"`
		TargetID       string `json:"target_id"`
		RequestSHA256  string `json:"request_sha256"`
		ComponentCount int    `json:"component_count"`
	}{
		Status:         "valid",
		SchemaVersion:  request.SchemaVersion,
		JobID:          request.JobID,
		ReleaseID:      request.ReleaseID,
		TargetID:       request.Target.ID,
		RequestSHA256:  requestSHA256,
		ComponentCount: len(request.Images),
	})
}

func init() {
	releaseCmd.AddCommand(releaseValidateCmd)
	rootCmd.AddCommand(releaseCmd)
}
