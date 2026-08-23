package releasejob

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/config"
)

type Project struct {
	Name string
	Path string
}

type ProjectStore interface {
	GetReleaseProject(name string) (Project, error)
}

type CommandRunner interface {
	Run(ctx context.Context, directory, executable string, args ...string) (string, error)
}

type HTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

type ComposeRuntime struct {
	config   *config.Config
	projects ProjectStore
	commands CommandRunner
	http     HTTPDoer
}

type OSCommandRunner struct{}

func (OSCommandRunner) Run(
	ctx context.Context,
	directory, executable string,
	args ...string,
) (string, error) {
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s failed: %w", executable, err)
	}
	return strings.TrimSpace(string(output)), nil
}

func NewComposeRuntime(cfg *config.Config, projects ProjectStore) *ComposeRuntime {
	return &ComposeRuntime{
		config:   cfg,
		projects: projects,
		commands: OSCommandRunner{},
		http: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

type resolvedTarget struct {
	project  Project
	policy   config.ReleaseTargetConfig
	baseFile string
	override string
}

func (r *ComposeRuntime) resolve(request Request) (resolvedTarget, error) {
	if r.config == nil || !r.config.Release.Enabled {
		return resolvedTarget{}, errors.New("bounded release execution is disabled")
	}
	policy, ok := r.config.Release.Targets[request.Target.ID]
	if !ok {
		return resolvedTarget{}, errors.New("release target is not allowlisted")
	}
	if policy.Environment != request.Target.Environment ||
		policy.Profile != request.Target.Profile ||
		policy.HealthProfile != request.HealthProfile {
		return resolvedTarget{}, errors.New("release target policy does not match the request")
	}
	if request.Target.Environment == "production" {
		return resolvedTarget{}, errors.New("production backup provider is not configured for bounded release execution")
	}
	project, err := r.projects.GetReleaseProject(policy.Project)
	if err != nil {
		return resolvedTarget{}, fmt.Errorf("load allowlisted project: %w", err)
	}
	if project.Name != policy.Project {
		return resolvedTarget{}, errors.New("allowlisted project identity mismatch")
	}

	projectPath, err := filepath.EvalSymlinks(project.Path)
	if err != nil {
		return resolvedTarget{}, fmt.Errorf("resolve project path: %w", err)
	}
	projectPath, err = filepath.Abs(projectPath)
	if err != nil {
		return resolvedTarget{}, err
	}
	baseFile := filepath.Join(projectPath, policy.ComposeFile)
	resolvedBase, err := filepath.EvalSymlinks(baseFile)
	if err != nil {
		return resolvedTarget{}, fmt.Errorf("resolve compose file: %w", err)
	}
	if !pathInside(projectPath, resolvedBase) {
		return resolvedTarget{}, errors.New("compose file escapes the allowlisted project path")
	}

	overrideDirectory := filepath.Join(projectPath, ".fleetdeck", "release-overrides")
	return resolvedTarget{
		project:  project,
		policy:   policy,
		baseFile: resolvedBase,
		override: filepath.Join(overrideDirectory, request.JobID+".json"),
	}, nil
}

func pathInside(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}

func (r *ComposeRuntime) Preflight(ctx context.Context, request Request) (PreflightEvidence, error) {
	target, err := r.resolve(request)
	if err != nil {
		return PreflightEvidence{}, err
	}
	if err := writeComposeOverride(target.override, request.Images); err != nil {
		return PreflightEvidence{}, err
	}
	args := composeArgs(target)
	if _, err := r.commands.Run(ctx, target.project.Path, "docker", append(args, "config", "--quiet")...); err != nil {
		return PreflightEvidence{}, err
	}
	servicesOutput, err := r.commands.Run(ctx, target.project.Path, "docker", append(args, "config", "--services")...)
	if err != nil {
		return PreflightEvidence{}, err
	}
	services := linesSet(servicesOutput)
	for _, image := range request.Images {
		if _, ok := services[image.Component]; !ok {
			return PreflightEvidence{}, fmt.Errorf("component %s is not an allowlisted compose service", image.Component)
		}
	}
	migrationReady := request.Migration.Mode == "none"
	if request.Migration.Mode == "preflight-and-apply" {
		_, migrationReady = services[target.policy.MigrationService]
		migrationReady = migrationReady && len(target.policy.MigrationArgs) > 0
	}
	if !migrationReady {
		return PreflightEvidence{}, errors.New("configured migration service is not available")
	}
	if _, err := r.commands.Run(ctx, target.project.Path, "docker", append(args, "pull", "--quiet")...); err != nil {
		return PreflightEvidence{}, err
	}
	return PreflightEvidence{ComposeValid: true, ImagesResolved: true, MigrationReady: true}, nil
}

func (r *ComposeRuntime) CreateAndVerifyBackup(
	context.Context,
	Request,
) (BackupEvidence, error) {
	return BackupEvidence{}, errors.New("production backup provider is not configured for bounded release execution")
}

func (r *ComposeRuntime) Apply(ctx context.Context, request Request) (ApplyEvidence, error) {
	target, err := r.resolve(request)
	if err != nil {
		return ApplyEvidence{}, err
	}
	if err := writeComposeOverride(target.override, request.Images); err != nil {
		return ApplyEvidence{}, err
	}
	args := composeArgs(target)
	migrationApplied := false
	if request.Migration.Mode == "preflight-and-apply" {
		migrationArgs := append(append(args, "run", "--rm", "--no-deps", target.policy.MigrationService), target.policy.MigrationArgs...)
		if _, err := r.commands.Run(ctx, target.project.Path, "docker", migrationArgs...); err != nil {
			return ApplyEvidence{}, err
		}
		migrationApplied = true
	}
	if _, err := r.commands.Run(ctx, target.project.Path, "docker", append(args, "up", "-d", "--remove-orphans")...); err != nil {
		return ApplyEvidence{}, err
	}
	return ApplyEvidence{AppliedComponents: len(request.Images), MigrationApplied: migrationApplied}, nil
}

func (r *ComposeRuntime) VerifyHealth(ctx context.Context, request Request) (HealthEvidence, error) {
	target, err := r.resolve(request)
	if err != nil {
		return HealthEvidence{}, err
	}
	passed := 0
	for _, healthURL := range target.policy.HealthURLs {
		httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			return HealthEvidence{}, err
		}
		httpRequest.Header.Set("user-agent", "fleetdeck-release-health/1")
		response, err := r.http.Do(httpRequest)
		if err != nil {
			return HealthEvidence{}, errors.New("configured health check request failed")
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
		response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return HealthEvidence{}, fmt.Errorf("configured health check returned HTTP %d", response.StatusCode)
		}
		passed++
	}
	return HealthEvidence{Profile: request.HealthProfile, ChecksPassed: passed}, nil
}

func composeArgs(target resolvedTarget) []string {
	return []string{"compose", "-f", target.baseFile, "-f", target.override}
}

func linesSet(value string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result[line] = struct{}{}
		}
	}
	return result
}

func writeComposeOverride(path string, images []Image) error {
	services := make(map[string]map[string]string, len(images))
	for _, image := range images {
		services[image.Component] = map[string]string{"image": image.Reference}
	}
	payload, err := json.Marshal(struct {
		Services map[string]map[string]string `json:"services"`
	}{Services: services})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".release-override-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := io.Copy(temporary, bytes.NewReader(payload)); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
