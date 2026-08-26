package releasejob

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/config"
)

type projectStoreStub struct{ project Project }

func (s projectStoreStub) GetReleaseProject(string) (Project, error) { return s.project, nil }

type commandCall struct {
	directory  string
	executable string
	args       []string
}

type commandRunnerStub struct {
	calls     []commandCall
	beforeRun func(commandCall) error
	failAt    int
}

func (r *commandRunnerStub) Run(_ context.Context, directory, executable string, args ...string) (string, error) {
	call := commandCall{directory: directory, executable: executable, args: args}
	r.calls = append(r.calls, call)
	if r.beforeRun != nil {
		if err := r.beforeRun(call); err != nil {
			return "", err
		}
	}
	if r.failAt > 0 && len(r.calls) == r.failAt {
		return "", errors.New("forced command failure")
	}
	if len(args) >= 2 && args[len(args)-2] == "config" && args[len(args)-1] == "--services" {
		return "api\nweb\n", nil
	}
	return "", nil
}

type httpStub struct{ status int }

func (h httpStub) Do(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: h.status, Body: io.NopCloser(strings.NewReader("ok"))}, nil
}

type httpSequenceStub struct {
	statuses []int
	calls    int
}

func (h *httpSequenceStub) Do(*http.Request) (*http.Response, error) {
	index := h.calls
	h.calls++
	if index >= len(h.statuses) {
		index = len(h.statuses) - 1
	}
	return &http.Response{
		StatusCode: h.statuses[index],
		Body:       io.NopCloser(strings.NewReader("ok")),
	}, nil
}

type blockingHTTPStub struct{}

func (blockingHTTPStub) Do(request *http.Request) (*http.Response, error) {
	<-request.Context().Done()
	return nil, request.Context().Err()
}

type productionBackupStub struct {
	evidence BackupEvidence
	calls    int
	onCreate func(Project, Request) error
}

func (s *productionBackupStub) CreateAndVerify(
	_ context.Context,
	project Project,
	request Request,
) (BackupEvidence, error) {
	s.calls++
	if s.onCreate != nil {
		if err := s.onCreate(project, request); err != nil {
			return BackupEvidence{}, err
		}
	}
	return s.evidence, nil
}

func (s *productionBackupStub) Restore(
	context.Context,
	Project,
	Request,
	BackupEvidence,
) (RollbackEvidence, error) {
	return RollbackEvidence{BackupID: s.evidence.BackupID, Restored: true, HealthVerified: true}, nil
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
	runtime.preflightRuntime = testReleasePreflightRuntime(t)
	return runtime, request, commands, projectPath
}

func testReleasePreflightRuntime(t *testing.T) releasePreflightRuntime {
	t.Helper()
	parent := t.TempDir()
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		t.Fatal(err)
	}
	return releasePreflightRuntime{
		directory: filepath.Join(resolvedParent, "release-preflight"),
		ownerUID:  uint32(os.Geteuid()),
	}
}

func TestComposeRuntimePreflightUsesOnlyDerivedArgumentsAndTemporaryDigestOverride(t *testing.T) {
	runtime, request, commands, projectPath := composeRuntimeHarness(t)
	canonicalOverride := filepath.Join(projectPath, ".fleetdeck", "release-overrides", "job-example.json")
	var preflightOverride string
	commands.beforeRun = func(call commandCall) error {
		if len(call.args) < 5 {
			return errors.New("compose arguments do not contain an override")
		}
		observed := call.args[4]
		if observed == canonicalOverride {
			return errors.New("preflight used the persistent apply override")
		}
		if !pathInside(runtime.preflightRuntime.directory, observed) || pathInside(projectPath, observed) {
			return errors.New("preflight override is not isolated in the runtime directory")
		}
		if !strings.HasPrefix(filepath.Base(observed), ".release-preflight-") {
			return errors.New("preflight override does not use the bounded temporary prefix")
		}
		if preflightOverride == "" {
			preflightOverride = observed
		} else if observed != preflightOverride {
			return errors.New("preflight changed override paths between commands")
		}
		payload, err := os.ReadFile(observed)
		if err != nil {
			return err
		}
		if !strings.Contains(string(payload), "@sha256:") || strings.Contains(string(payload), ":latest") {
			return errors.New("preflight override did not contain immutable image references")
		}
		info, err := os.Stat(observed)
		if err != nil {
			return err
		}
		if info.Mode().Perm() != 0600 {
			return errors.New("preflight override permissions are not 0600")
		}
		return nil
	}
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
	if preflightOverride == "" {
		t.Fatal("preflight override was not observed")
	}
	if _, err := os.Stat(preflightOverride); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary preflight override remains after success: %v", err)
	}
	if _, err := os.Stat(canonicalOverride); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preflight created the persistent apply override: %v", err)
	}
}

func TestComposeRuntimeAppliesMigrationAndImagesWithoutShell(t *testing.T) {
	runtime, request, commands, projectPath := composeRuntimeHarness(t)
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
	overridePath := filepath.Join(projectPath, ".fleetdeck", "release-overrides", "job-example.json")
	payload, err := os.ReadFile(overridePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), "@sha256:") || strings.Contains(string(payload), ":latest") {
		t.Fatalf("unsafe apply override: %s", payload)
	}
	info, err := os.Stat(overridePath)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("apply override permissions = %v, %v", info.Mode().Perm(), err)
	}
}

func TestComposeRuntimePreflightRemovesTemporaryOverrideOnEveryFailurePath(t *testing.T) {
	tests := []struct {
		name      string
		failAt    int
		configure func(*ComposeRuntime, *Request)
	}{
		{name: "compose validation", failAt: 1},
		{name: "service discovery", failAt: 2},
		{
			name: "unknown candidate service",
			configure: func(_ *ComposeRuntime, request *Request) {
				request.Images[0].Component = "unknown"
			},
		},
		{
			name: "migration policy",
			configure: func(runtime *ComposeRuntime, request *Request) {
				policy := runtime.config.Release.Targets[request.Target.ID]
				policy.MigrationArgs = nil
				runtime.config.Release.Targets[request.Target.ID] = policy
			},
		},
		{name: "candidate image pull", failAt: 3},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime, request, commands, projectPath := composeRuntimeHarness(t)
			commands.failAt = test.failAt
			var preflightOverride string
			commands.beforeRun = func(call commandCall) error {
				if len(call.args) >= 5 {
					preflightOverride = call.args[4]
				}
				return nil
			}
			if test.configure != nil {
				test.configure(runtime, &request)
			}

			if _, err := runtime.Preflight(context.Background(), request); err == nil {
				t.Fatal("preflight failure was not returned")
			}
			if preflightOverride == "" {
				t.Fatal("temporary preflight override was not observed")
			}
			if pathInside(projectPath, preflightOverride) {
				t.Fatalf("temporary override entered the project tree: %s", preflightOverride)
			}
			if _, err := os.Stat(preflightOverride); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("temporary override remains after preflight failure: %v", err)
			}
			canonical := filepath.Join(projectPath, ".fleetdeck", "release-overrides", "job-example.json")
			if _, err := os.Stat(canonical); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed preflight created the persistent apply override: %v", err)
			}
		})
	}
}

func TestComposeRuntimePreflightFailsClosedWhenTemporaryOverrideCannotBeRemoved(t *testing.T) {
	runtime, request, commands, projectPath := composeRuntimeHarness(t)
	var preflightOverride string
	commands.beforeRun = func(call commandCall) error {
		if len(call.args) >= 5 {
			preflightOverride = call.args[4]
		}
		return nil
	}
	runtime.removeFile = func(string) error { return errors.New("forced cleanup failure") }

	evidence, err := runtime.Preflight(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "remove preflight compose override") {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}
	if evidence.ComposeValid || evidence.ImagesResolved || evidence.MigrationReady {
		t.Fatalf("cleanup failure returned successful evidence: %+v", evidence)
	}
	if preflightOverride == "" {
		t.Fatal("temporary preflight override was not observed")
	}
	if pathInside(projectPath, preflightOverride) {
		t.Fatalf("orphaned temporary override entered the project tree: %s", preflightOverride)
	}
	if err := os.Remove(preflightOverride); err != nil {
		t.Fatal(err)
	}
}

func TestComposeRuntimeRemovesPreflightOverrideBeforeProductionBackup(t *testing.T) {
	runtime, request, _, projectPath := composeRuntimeHarness(t)
	runtimeDirectory := runtime.preflightRuntime.directory
	runtime.config.Release.AllowProduction = true
	policy := runtime.config.Release.Targets[request.Target.ID]
	policy.Environment = "production"
	runtime.config.Release.Targets[request.Target.ID] = policy
	request.Target.Environment = "production"
	request.Backup.Required = true
	provider := &productionBackupStub{
		evidence: BackupEvidence{
			BackupID:        "backup-1",
			ManifestSHA256:  "sha256:" + strings.Repeat("a", 64),
			Verified:        true,
			Encrypted:       true,
			OffsiteVerified: true,
		},
		onCreate: func(project Project, _ Request) error {
			patterns := []string{
				filepath.Join(project.Path, ".fleetdeck", "release-overrides", "*"),
				filepath.Join(runtimeDirectory, "*"),
			}
			for _, pattern := range patterns {
				matches, err := filepath.Glob(pattern)
				if err != nil {
					return err
				}
				if len(matches) != 0 {
					return errors.New("candidate override was visible to the production backup provider")
				}
			}
			return nil
		},
	}
	runtime.backups = provider

	if _, err := runtime.Preflight(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateAndVerifyBackup(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 {
		t.Fatalf("backup provider calls = %d", provider.calls)
	}
	canonical := filepath.Join(projectPath, ".fleetdeck", "release-overrides", "job-example.json")
	if _, err := os.Stat(canonical); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("persistent apply override exists before apply: %v", err)
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
	if err != nil || evidence.Profile != "http-standard" || evidence.ChecksPassed != 1 || evidence.Attempts != 1 {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}
	runtime.http = httpStub{status: 503}
	if _, err := runtime.VerifyHealth(context.Background(), request); err == nil || !strings.Contains(err.Error(), "HTTP 503") {
		t.Fatalf("health error = %v", err)
	}
}

func TestComposeRuntimeHealthRetriesUntilEveryConfiguredURLIsReady(t *testing.T) {
	runtime, request, _, _ := composeRuntimeHarness(t)
	target := runtime.config.Release.Targets[request.Target.ID]
	target.HealthURLs = []string{
		"https://api.staging.example.com/health",
		"https://web.staging.example.com/health",
	}
	target.HealthMaxAttempts = 3
	target.HealthRetryInterval = "100ms"
	target.HealthTimeout = "2s"
	runtime.config.Release.Targets[request.Target.ID] = target
	responses := &httpSequenceStub{statuses: []int{200, 503, 200, 200}}
	runtime.http = responses
	waits := 0
	runtime.wait = func(context.Context, time.Duration) error {
		waits++
		return nil
	}

	evidence, err := runtime.VerifyHealth(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Attempts != 2 || evidence.ChecksPassed != 2 || responses.calls != 4 || waits != 1 {
		t.Fatalf("evidence=%+v calls=%d waits=%d", evidence, responses.calls, waits)
	}
}

func TestComposeRuntimeHealthFailsAfterConfiguredAttempts(t *testing.T) {
	runtime, request, _, _ := composeRuntimeHarness(t)
	target := runtime.config.Release.Targets[request.Target.ID]
	target.HealthMaxAttempts = 3
	target.HealthRetryInterval = "100ms"
	target.HealthTimeout = "2s"
	runtime.config.Release.Targets[request.Target.ID] = target
	responses := &httpSequenceStub{statuses: []int{503}}
	runtime.http = responses
	waits := 0
	runtime.wait = func(context.Context, time.Duration) error {
		waits++
		return nil
	}

	evidence, err := runtime.VerifyHealth(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "after 3 attempt(s)") || !strings.Contains(err.Error(), "HTTP 503") {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}
	if evidence.Attempts != 3 || evidence.ChecksPassed != 0 || responses.calls != 3 || waits != 2 {
		t.Fatalf("evidence=%+v calls=%d waits=%d", evidence, responses.calls, waits)
	}
}

func TestComposeRuntimeHealthStopsWhenContextIsCancelled(t *testing.T) {
	runtime, request, _, _ := composeRuntimeHarness(t)
	target := runtime.config.Release.Targets[request.Target.ID]
	target.HealthMaxAttempts = 3
	target.HealthRetryInterval = "100ms"
	target.HealthTimeout = "2s"
	runtime.config.Release.Targets[request.Target.ID] = target
	responses := &httpSequenceStub{statuses: []int{503}}
	runtime.http = responses
	ctx, cancel := context.WithCancel(context.Background())
	runtime.wait = func(context.Context, time.Duration) error {
		cancel()
		return ctx.Err()
	}

	evidence, err := runtime.VerifyHealth(ctx, request)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}
	if evidence.Attempts != 1 || responses.calls != 1 {
		t.Fatalf("evidence=%+v calls=%d", evidence, responses.calls)
	}
}

func TestComposeRuntimeHealthStopsAtConfiguredOverallTimeout(t *testing.T) {
	runtime, request, _, _ := composeRuntimeHarness(t)
	target := runtime.config.Release.Targets[request.Target.ID]
	target.HealthMaxAttempts = 3
	target.HealthRetryInterval = "100ms"
	target.HealthTimeout = "1s"
	runtime.config.Release.Targets[request.Target.ID] = target
	runtime.http = blockingHTTPStub{}

	evidence, err := runtime.VerifyHealth(context.Background(), request)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}
	if evidence.Attempts != 1 {
		t.Fatalf("evidence=%+v", evidence)
	}
}

func TestComposeRuntimeHealthDefaultsToOneAttempt(t *testing.T) {
	runtime, request, _, _ := composeRuntimeHarness(t)
	responses := &httpSequenceStub{statuses: []int{503, 200}}
	runtime.http = responses

	evidence, err := runtime.VerifyHealth(context.Background(), request)
	if err == nil || evidence.Attempts != 1 || responses.calls != 1 {
		t.Fatalf("evidence=%+v calls=%d err=%v", evidence, responses.calls, err)
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

func TestComposeRuntimeUsesConfiguredProviderForProductionOnly(t *testing.T) {
	runtime, request, _, _ := composeRuntimeHarness(t)
	runtime.config.Release.AllowProduction = true
	policy := runtime.config.Release.Targets[request.Target.ID]
	policy.Environment = "production"
	runtime.config.Release.Targets[request.Target.ID] = policy
	request.Target.Environment = "production"
	request.Backup.Required = true
	provider := &productionBackupStub{evidence: BackupEvidence{
		BackupID:        "backup-1",
		ManifestSHA256:  "sha256:" + strings.Repeat("a", 64),
		Verified:        true,
		Encrypted:       true,
		OffsiteVerified: true,
	}}
	runtime.backups = provider

	evidence, err := runtime.CreateAndVerifyBackup(context.Background(), request)
	if err != nil || evidence.BackupID != "backup-1" || provider.calls != 1 {
		t.Fatalf("evidence=%+v calls=%d err=%v", evidence, provider.calls, err)
	}

	request.Target.Environment = "staging"
	if _, err := runtime.CreateAndVerifyBackup(context.Background(), request); err == nil {
		t.Fatal("staging request unexpectedly reached the production backup provider")
	}
	if provider.calls != 1 {
		t.Fatalf("provider called for staging: %d", provider.calls)
	}
}
