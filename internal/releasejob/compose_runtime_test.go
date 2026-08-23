package releasejob

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/config"
)

type projectStoreStub struct{ project Project }

func (s projectStoreStub) GetReleaseProject(string) (Project, error) { return s.project, nil }

type commandCall struct {
	directory  string
	executable string
	args       []string
}

type commandRunnerStub struct{ calls []commandCall }

func (r *commandRunnerStub) Run(_ context.Context, directory, executable string, args ...string) (string, error) {
	r.calls = append(r.calls, commandCall{directory: directory, executable: executable, args: args})
	if len(args) >= 2 && args[len(args)-2] == "config" && args[len(args)-1] == "--services" {
		return "api\nweb\n", nil
	}
	return "", nil
}

type httpStub struct{ status int }

func (h httpStub) Do(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: h.status, Body: io.NopCloser(strings.NewReader("ok"))}, nil
}

func composeRuntimeHarness(t *testing.T) (*ComposeRuntime, Request, *commandRunnerStub, string) {
	t.Helper()
	projectPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectPath, "docker-compose.yml"), []byte("services: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.Release = config.ReleaseConfig{
		Enabled: true,
		Targets: map[string]config.ReleaseTargetConfig{
			"target-example": {
				Project: "target-example", Environment: "staging", Profile: "standard",
				ComposeFile: "docker-compose.yml", MigrationService: "api",
				MigrationArgs: []string{"app", "migrate", "apply"},
				HealthProfile: "http-standard",
				HealthURLs:    []string{"https://staging.example.com/health"},
			},
		},
	}
	request := Request{
		SchemaVersion: 1, JobID: "job-example", IdempotencyKey: "release.target.attempt-1",
		ReleaseID: "release", ManifestDigest: "sha256:" + strings.Repeat("1", 64),
		Target: Target{ID: "target-example", Environment: "staging", Profile: "standard"},
		Images: []Image{
			{Component: "api", Reference: "registry.example.com/example/api@sha256:" + strings.Repeat("2", 64)},
			{Component: "web", Reference: "registry.example.com/example/web@sha256:" + strings.Repeat("3", 64)},
		},
		Migration: MigrationPolicy{Mode: "preflight-and-apply"}, HealthProfile: "http-standard",
	}
	commands := &commandRunnerStub{}
	runtime := NewComposeRuntime(cfg, projectStoreStub{project: Project{Name: "target-example", Path: projectPath}})
	runtime.commands = commands
	runtime.http = httpStub{status: 200}
	return runtime, request, commands, projectPath
}

func TestComposeRuntimePreflightUsesOnlyDerivedArgumentsAndImmutableOverride(t *testing.T) {
	runtime, request, commands, projectPath := composeRuntimeHarness(t)
	evidence, err := runtime.Preflight(context.Background(), request)
	if err != nil || !evidence.ComposeValid || !evidence.ImagesResolved || !evidence.MigrationReady {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}
	if len(commands.calls) != 3 {
		t.Fatalf("command calls = %v", commands.calls)
	}
	for _, call := range commands.calls {
		if call.executable != "docker" || strings.Contains(strings.Join(call.args, " "), "sh -c") {
			t.Fatalf("unbounded command call: %+v", call)
		}
	}
	overridePath := filepath.Join(projectPath, ".fleetdeck", "release-overrides", "job-example.json")
	payload, err := os.ReadFile(overridePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), "@sha256:") || strings.Contains(string(payload), ":latest") {
		t.Fatalf("unsafe override: %s", payload)
	}
	info, err := os.Stat(overridePath)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("override permissions = %v, %v", info.Mode().Perm(), err)
	}
}

func TestComposeRuntimeAppliesMigrationAndImagesWithoutShell(t *testing.T) {
	runtime, request, commands, _ := composeRuntimeHarness(t)
	if _, err := runtime.Preflight(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	commands.calls = nil
	evidence, err := runtime.Apply(context.Background(), request)
	if err != nil || evidence.AppliedComponents != 2 || !evidence.MigrationApplied {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}
	if len(commands.calls) != 2 {
		t.Fatalf("calls=%v", commands.calls)
	}
	migration := strings.Join(commands.calls[0].args, " ")
	if !strings.Contains(migration, "run --rm --no-deps api app migrate apply") || strings.Contains(migration, "sh -c") {
		t.Fatalf("migration args = %s", migration)
	}
	if !strings.HasSuffix(strings.Join(commands.calls[1].args, " "), "up -d --remove-orphans") {
		t.Fatalf("apply args = %v", commands.calls[1].args)
	}
}

func TestComposeRuntimeRejectsPolicyMismatchAndUnknownServices(t *testing.T) {
	runtime, request, commands, _ := composeRuntimeHarness(t)
	request.Target.Environment = "production"
	if _, err := runtime.Preflight(context.Background(), request); err == nil {
		t.Fatal("policy mismatch unexpectedly accepted")
	}
	if len(commands.calls) != 0 {
		t.Fatalf("commands ran before policy rejection: %v", commands.calls)
	}

	runtime, request, commands, _ = composeRuntimeHarness(t)
	request.Images[0].Component = "unknown"
	if _, err := runtime.Preflight(context.Background(), request); err == nil || !strings.Contains(err.Error(), "not an allowlisted compose service") {
		t.Fatalf("unknown service error = %v", err)
	}
	if len(commands.calls) != 2 {
		t.Fatalf("pull should not run after unknown service: %v", commands.calls)
	}
}

func TestComposeRuntimeHealthIsBoundedToConfiguredURLs(t *testing.T) {
	runtime, request, _, _ := composeRuntimeHarness(t)
	evidence, err := runtime.VerifyHealth(context.Background(), request)
	if err != nil || evidence.Profile != "http-standard" || evidence.ChecksPassed != 1 {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}
	runtime.http = httpStub{status: 503}
	if _, err := runtime.VerifyHealth(context.Background(), request); err == nil || !strings.Contains(err.Error(), "HTTP 503") {
		t.Fatalf("health error = %v", err)
	}
}

func TestComposeRuntimeKeepsProductionDisabledWithoutVerifiedProvider(t *testing.T) {
	runtime, request, _, _ := composeRuntimeHarness(t)
	runtime.config.Release.AllowProduction = true
	policy := runtime.config.Release.Targets[request.Target.ID]
	policy.Environment = "production"
	runtime.config.Release.Targets[request.Target.ID] = policy
	request.Target.Environment = "production"
	request.Backup.Required = true
	if _, err := runtime.Preflight(context.Background(), request); err == nil || !strings.Contains(err.Error(), "backup provider") {
		t.Fatalf("production runtime error = %v", err)
	}
}
