package deploy

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Strategy defines how a project is deployed.
type Strategy interface {
	Deploy(ctx context.Context, opts DeployOptions) (*DeployResult, error)
}

// DeployOptions holds the parameters for a deployment.
type DeployOptions struct {
	ProjectPath    string
	ProjectName    string
	ComposeFile    string
	HealthCheckURL string
	Timeout        time.Duration
	PreDeployHook  string // Command to run before deploy (e.g. "npm run migrate")
	PostDeployHook string // Command to run after deploy (e.g. "npm run seed")
	NoCache        bool   // Pass --no-cache to docker compose build
}

// DeployResult captures the outcome of a deployment.
type DeployResult struct {
	Success       bool          `json:"success"`
	Duration      time.Duration `json:"duration"`
	OldContainers []string      `json:"old_containers,omitempty"`
	NewContainers []string      `json:"new_containers,omitempty"`
	Logs          []string      `json:"logs,omitempty"`
}

// GetStrategy returns a deployment strategy by name.
func GetStrategy(name string) (Strategy, error) {
	switch name {
	case "basic", "":
		return &BasicStrategy{}, nil
	case "bluegreen":
		return &BlueGreenStrategy{}, nil
	case "rolling":
		return &RollingStrategy{}, nil
	default:
		return nil, fmt.Errorf("unknown deploy strategy %q", name)
	}
}

// runHook executes a deploy hook inside the app container and appends
// the output to the result logs.
func runHook(ctx context.Context, label, command, projectPath string, result *DeployResult) error {
	hookCmd := exec.CommandContext(ctx, "docker", "compose", "exec", "-T", "app", "sh", "-c", command)
	hookCmd.Dir = projectPath
	out, err := hookCmd.CombinedOutput()
	result.Logs = append(result.Logs, fmt.Sprintf("[%s] %s", label, strings.TrimSpace(string(out))))
	if err != nil {
		return fmt.Errorf("%s hook failed: %s: %w", label, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// buildNoCache runs "docker compose build --no-cache" for the given options.
func buildNoCache(ctx context.Context, opts DeployOptions) error {
	args := []string{"compose"}
	if opts.ComposeFile != "" {
		args = append(args, "-f", opts.ComposeFile)
	}
	args = append(args, "build", "--no-cache")

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = opts.ProjectPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose build --no-cache: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// BasicStrategy performs a simple docker compose up -d deployment.
type BasicStrategy struct{}

func (s *BasicStrategy) Deploy(ctx context.Context, opts DeployOptions) (*DeployResult, error) {
	start := time.Now()
	result := &DeployResult{}

	// Capture old containers before deploy.
	old, _ := listContainers(opts.ProjectPath)
	result.OldContainers = old

	// Build without cache if requested.
	if opts.NoCache {
		if err := buildNoCache(ctx, opts); err != nil {
			result.Duration = time.Since(start)
			return result, err
		}
	}

	if opts.PreDeployHook != "" {
		if err := runHook(ctx, "pre-deploy", opts.PreDeployHook, opts.ProjectPath, result); err != nil {
			result.Duration = time.Since(start)
			return result, err
		}
	}

	args := []string{"compose"}
	if opts.ComposeFile != "" {
		args = append(args, "-f", opts.ComposeFile)
	}
	args = append(args, "up", "-d", "--pull", "always")

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = opts.ProjectPath
	out, err := cmd.CombinedOutput()
	result.Logs = append(result.Logs, strings.TrimSpace(string(out)))
	if err != nil {
		result.Duration = time.Since(start)
		return result, fmt.Errorf("docker compose up: %s: %w", strings.TrimSpace(string(out)), err)
	}

	if opts.PostDeployHook != "" {
		if err := runHook(ctx, "post-deploy", opts.PostDeployHook, opts.ProjectPath, result); err != nil {
			result.Duration = time.Since(start)
			return result, err
		}
	}

	// Capture new containers after deploy.
	result.NewContainers, _ = listContainers(opts.ProjectPath)
	result.Success = true
	result.Duration = time.Since(start)
	return result, nil
}

// RollingStrategy performs a rolling update one service at a time.
type RollingStrategy struct{}

func (s *RollingStrategy) Deploy(ctx context.Context, opts DeployOptions) (*DeployResult, error) {
	start := time.Now()
	result := &DeployResult{}

	result.OldContainers, _ = listContainers(opts.ProjectPath)

	// Build without cache if requested.
	if opts.NoCache {
		if err := buildNoCache(ctx, opts); err != nil {
			result.Duration = time.Since(start)
			return result, err
		}
	}

	if opts.PreDeployHook != "" {
		if err := runHook(ctx, "pre-deploy", opts.PreDeployHook, opts.ProjectPath, result); err != nil {
			result.Duration = time.Since(start)
			return result, err
		}
	}

	// Pull new images first.
	pullArgs := []string{"compose"}
	if opts.ComposeFile != "" {
		pullArgs = append(pullArgs, "-f", opts.ComposeFile)
	}
	pullArgs = append(pullArgs, "pull")

	pullCmd := exec.CommandContext(ctx, "docker", pullArgs...)
	pullCmd.Dir = opts.ProjectPath
	if out, err := pullCmd.CombinedOutput(); err != nil {
		result.Logs = append(result.Logs, strings.TrimSpace(string(out)))
		result.Duration = time.Since(start)
		return result, fmt.Errorf("docker compose pull: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Get list of services.
	services, err := listServices(ctx, opts)
	if err != nil {
		result.Duration = time.Since(start)
		return result, fmt.Errorf("listing services: %w", err)
	}

	// Restart each service one at a time.
	for _, svc := range services {
		args := []string{"compose"}
		if opts.ComposeFile != "" {
			args = append(args, "-f", opts.ComposeFile)
		}
		args = append(args, "up", "-d", "--no-deps", svc)

		cmd := exec.CommandContext(ctx, "docker", args...)
		cmd.Dir = opts.ProjectPath
		out, err := cmd.CombinedOutput()
		result.Logs = append(result.Logs, fmt.Sprintf("[%s] %s", svc, strings.TrimSpace(string(out))))
		if err != nil {
			result.Duration = time.Since(start)
			return result, fmt.Errorf("updating service %s: %s: %w", svc, strings.TrimSpace(string(out)), err)
		}
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
	return result, nil
}

// listContainers returns the names of containers for a compose project.
func listContainers(projectPath string) ([]string, error) {
	cmd := exec.Command("docker", "compose", "ps", "--format", "{{.Name}}")
	cmd.Dir = projectPath
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

// listServices returns the service names defined in a compose project.
func listServices(ctx context.Context, opts DeployOptions) ([]string, error) {
	args := []string{"compose"}
	if opts.ComposeFile != "" {
		args = append(args, "-f", opts.ComposeFile)
	}
	args = append(args, "config", "--services")

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = opts.ProjectPath
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var services []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			services = append(services, line)
		}
	}
	return services, nil
}
