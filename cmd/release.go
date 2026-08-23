package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/db"
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
		input, closeInput, err := releaseJobInput(cmd, args[0])
		if err != nil {
			return err
		}
		defer closeInput()
		return validateReleaseJob(input, cmd.OutOrStdout())
	},
}

var releaseApplyCmd = &cobra.Command{
	Use:   "apply <job.json|->",
	Short: "Execute an allowlisted, idempotent release job",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg == nil || !cfg.Release.Enabled {
			return fmt.Errorf("bounded release execution is disabled in FleetDeck configuration")
		}
		input, closeInput, err := releaseJobInput(cmd, args[0])
		if err != nil {
			return err
		}
		defer closeInput()

		database := openDB()
		runtime := releasejob.NewComposeRuntime(cfg, releaseProjectStore{database: database})
		executor := releasejob.NewExecutor(database, runtime)
		request, result, err := applyReleaseJob(cmd.Context(), input, cmd.OutOrStdout(), executor)
		if err != nil {
			audit.Log("release.apply", request.Target.ID, result.ErrorCode, false)
			return err
		}
		audit.Log("release.apply", request.Target.ID, "release="+request.ReleaseID, true)
		return nil
	},
}

type releaseProjectStore struct{ database *db.DB }

func (s releaseProjectStore) GetReleaseProject(name string) (releasejob.Project, error) {
	project, err := s.database.GetProject(name)
	if err != nil {
		return releasejob.Project{}, err
	}
	return releasejob.Project{Name: project.Name, Path: project.ProjectPath}, nil
}

type releaseExecutor interface {
	Execute(context.Context, releasejob.Request) (releasejob.Result, error)
}

func applyReleaseJob(
	ctx context.Context,
	input io.Reader,
	output io.Writer,
	executor releaseExecutor,
) (releasejob.Request, releasejob.Result, error) {
	request, err := releasejob.Decode(input)
	if err != nil {
		return releasejob.Request{}, releasejob.Result{}, err
	}
	result, executionErr := executor.Execute(ctx, request)
	if result.JobID != "" {
		if err := json.NewEncoder(output).Encode(result); err != nil {
			return request, result, fmt.Errorf("encode release result: %w", err)
		}
	}
	return request, result, executionErr
}

func releaseJobInput(cmd *cobra.Command, path string) (io.Reader, func(), error) {
	if path == "-" {
		return cmd.InOrStdin(), func() {}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, func() {}, fmt.Errorf("open release job: %w", err)
	}
	return file, func() { _ = file.Close() }, nil
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
	releaseCmd.AddCommand(releaseApplyCmd)
	rootCmd.AddCommand(releaseCmd)
}
