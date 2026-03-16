package deploy

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// BlueGreenStrategy deploys by bringing up a parallel set of containers,
// health-checking them, and then switching traffic over from the old set.
type BlueGreenStrategy struct{}

func (s *BlueGreenStrategy) Deploy(ctx context.Context, opts DeployOptions) (*DeployResult, error) {
	start := time.Now()
	result := &DeployResult{}

	newProject := opts.ProjectName + "-new"
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	// Capture old containers.
	result.OldContainers, _ = listContainers(opts.ProjectPath)

	if opts.PreDeployHook != "" {
		if err := runHook(ctx, "pre-deploy", opts.PreDeployHook, opts.ProjectPath, result); err != nil {
			result.Duration = time.Since(start)
			return result, err
		}
	}

	// Step 1: Bring up new containers under a different project name.
	result.Logs = append(result.Logs, "starting new containers")
	if err := s.startNew(ctx, opts, newProject); err != nil {
		result.Duration = time.Since(start)
		return result, fmt.Errorf("starting new containers: %w", err)
	}

	// Step 2: Health check the new containers.
	result.Logs = append(result.Logs, "running health checks")
	if err := s.waitHealthy(ctx, opts, timeout); err != nil {
		// Unhealthy: tear down the new set and keep the old running.
		result.Logs = append(result.Logs, fmt.Sprintf("health check failed: %v", err))
		result.Logs = append(result.Logs, "rolling back: removing new containers")
		if err := s.removeProject(opts.ProjectPath, newProject); err != nil {
			result.Logs = append(result.Logs, fmt.Sprintf("warning: rollback cleanup failed: %v", err))
		}
		result.Duration = time.Since(start)
		return result, fmt.Errorf("health check failed, rolled back: %w", err)
	}
	result.Logs = append(result.Logs, "health checks passed")

	// Step 3: Stop old containers.
	result.Logs = append(result.Logs, "stopping old containers")
	if err := s.stopOld(ctx, opts); err != nil {
		result.Logs = append(result.Logs, fmt.Sprintf("warning: failed to stop old containers: %v", err))
	}

	// Step 4: Rename new project to take over the original project name.
	// Docker Compose doesn't support renaming, so we stop the new project
	// and bring it back up under the original name.
	result.Logs = append(result.Logs, "promoting new containers")
	if err := s.removeProject(opts.ProjectPath, newProject); err != nil {
		result.Logs = append(result.Logs, fmt.Sprintf("warning: failed to remove temp project: %v", err))
	}
	if err := s.startOriginal(ctx, opts); err != nil {
		result.Duration = time.Since(start)
		return result, fmt.Errorf("promoting new containers: %w", err)
	}

	if opts.PostDeployHook != "" {
		if err := runHook(ctx, "post-deploy", opts.PostDeployHook, opts.ProjectPath, result); err != nil {
			result.Duration = time.Since(start)
			return result, err
		}
	}

	result.NewContainers, _ = listContainers(opts.ProjectPath)
	result.Success = true
	result.Duration = time.Since(start)
	result.Logs = append(result.Logs, "deployment complete")
	return result, nil
}

// startNew brings up the new containers under a temporary project name.
func (s *BlueGreenStrategy) startNew(ctx context.Context, opts DeployOptions, projectName string) error {
	args := []string{"compose", "-p", projectName}
	if opts.ComposeFile != "" {
		args = append(args, "-f", opts.ComposeFile)
	}
	args = append(args, "up", "-d", "--pull", "always")

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = opts.ProjectPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose up: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// waitHealthy polls the health check URL until it returns 200 or the timeout
// elapses. If no health check URL is configured it waits briefly and returns.
func (s *BlueGreenStrategy) waitHealthy(ctx context.Context, opts DeployOptions, timeout time.Duration) error {
	if opts.HealthCheckURL == "" {
		// No explicit URL; give containers a moment to settle.
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	}

	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resp, err := client.Get(opts.HealthCheckURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				return nil
			}
		}

		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return fmt.Errorf("health check timed out after %s", timeout)
}

// stopOld stops the original project's containers.
func (s *BlueGreenStrategy) stopOld(ctx context.Context, opts DeployOptions) error {
	args := []string{"compose"}
	if opts.ComposeFile != "" {
		args = append(args, "-f", opts.ComposeFile)
	}
	args = append(args, "down")

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = opts.ProjectPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose down: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// startOriginal brings up containers under the original project name.
func (s *BlueGreenStrategy) startOriginal(ctx context.Context, opts DeployOptions) error {
	args := []string{"compose"}
	if opts.ComposeFile != "" {
		args = append(args, "-f", opts.ComposeFile)
	}
	args = append(args, "up", "-d", "--pull", "always")

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = opts.ProjectPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose up: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// removeProject tears down a compose project entirely.
func (s *BlueGreenStrategy) removeProject(projectPath, projectName string) error {
	cmd := exec.Command("docker", "compose", "-p", projectName, "down", "--remove-orphans")
	cmd.Dir = projectPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("removing project %s: %s: %w", projectName, strings.TrimSpace(string(out)), err)
	}
	return nil
}
