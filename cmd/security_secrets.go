package cmd

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/secretscan"
	"github.com/fleetdeck/fleetdeck/internal/sourcedeploy"
	"github.com/spf13/cobra"
)

var securitySecretsCmd = &cobra.Command{
	Use:   "secrets [directory]",
	Short: "Check a Compose project's deployable source for potential secrets, offline",
	Long: `Check the same source bundle used by remote deployment, with embedded
Gitleaks content rules. Requires one Compose file. Existing filename exclusions
and Compose private references are respected. Git history, private env files,
runtime secrets and built images are outside this check's scope.

Results contain relative file paths, line numbers and rule IDs, never matched
values or source lines. Repository allowlists cannot disable this check.
Findings and incomplete checks exit unsuccessfully. No files are uploaded.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		if len(args) == 1 {
			dir = args[0]
		}
		archive, report, err := checkedSourceArchive(cmd.Context(), dir, "")
		if archive != nil {
			defer os.Remove(archive.Name())
			defer archive.Close()
		}
		if report.Scanner != "" {
			if encodeErr := json.NewEncoder(cmd.OutOrStdout()).Encode(report); encodeErr != nil {
				return encodeErr
			}
		}
		return err
	},
}

// checkedSourceArchive returns the very same private bytes that were checked.
// Callers must not rebuild the archive from possibly changed source afterwards.
func checkedSourceArchive(ctx context.Context, dir, envFile string) (*os.File, secretscan.Report, error) {
	var report secretscan.Report
	f, err := os.CreateTemp("", "fleetdeck-source-*.tar")
	if err != nil {
		return nil, report, err
	}
	keep := false
	defer func() {
		if !keep {
			f.Close()
			os.Remove(f.Name())
		}
	}()
	if err = sourcedeploy.ArchiveWithEnv(dir, f, envFile); err != nil {
		return nil, report, err
	}
	if _, err = f.Seek(0, 0); err != nil {
		return nil, report, err
	}
	scanCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	report = secretscan.Archive(scanCtx, f)
	if err = report.RequireClean(); err != nil {
		return nil, report, err
	}
	if _, err = f.Seek(0, 0); err != nil {
		return nil, report, err
	}
	keep = true
	return f, report, nil
}

func init() { securityCmd.AddCommand(securitySecretsCmd) }
