package releasejob

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type memoryStore struct {
	reservation Reservation
	reserveErr  error
	current     *string
	currentErr  error
	succeeded   *Result
	failed      *Result
}

func (s *memoryStore) Reserve(context.Context, Request, string, time.Time) (Reservation, error) {
	return s.reservation, s.reserveErr
}
func (s *memoryStore) CurrentRelease(context.Context, string) (*string, error) {
	return s.current, s.currentErr
}
func (s *memoryStore) CompleteSuccess(_ context.Context, result Result, _ string) error {
	s.succeeded = &result
	return nil
}
func (s *memoryStore) CompleteFailure(_ context.Context, result Result) error {
	s.failed = &result
	return nil
}

type fakeRuntime struct {
	calls     []string
	preflight PreflightEvidence
	backup    BackupEvidence
	apply     ApplyEvidence
	health    HealthEvidence
	failStep  string
}

func (r *fakeRuntime) step(name string) error {
	r.calls = append(r.calls, name)
	if r.failStep == name {
		return errors.New("runtime failed")
	}
	return nil
}
func (r *fakeRuntime) Preflight(context.Context, Request) (PreflightEvidence, error) {
	return r.preflight, r.step("preflight")
}
func (r *fakeRuntime) CreateAndVerifyBackup(context.Context, Request) (BackupEvidence, error) {
	return r.backup, r.step("backup")
}
func (r *fakeRuntime) Apply(context.Context, Request) (ApplyEvidence, error) {
	return r.apply, r.step("apply")
}
func (r *fakeRuntime) VerifyHealth(context.Context, Request) (HealthEvidence, error) {
	return r.health, r.step("health")
}

func validRequest(t *testing.T) Request {
	t.Helper()
	r, err := Decode(strings.NewReader(validJob))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func readyHarness(t *testing.T) (*Executor, *memoryStore, *fakeRuntime) {
	t.Helper()
	store := &memoryStore{reservation: Reservation{State: ReservationAcquired}}
	runtime := &fakeRuntime{
		preflight: PreflightEvidence{ComposeValid: true, ImagesResolved: true, MigrationReady: true},
		backup: BackupEvidence{
			BackupID:        "backup-1",
			ManifestSHA256:  "sha256:" + strings.Repeat("a", 64),
			Verified:        true,
			Encrypted:       true,
			OffsiteVerified: true,
		},
		apply:  ApplyEvidence{AppliedComponents: 1, MigrationApplied: true},
		health: HealthEvidence{Profile: "http-standard", ChecksPassed: 3},
	}
	executor := NewExecutor(store, runtime)
	now := time.Date(2026, 8, 23, 20, 0, 0, 0, time.UTC)
	executor.now = func() time.Time { now = now.Add(time.Second); return now }
	return executor, store, runtime
}

func TestExecutorRunsBoundedStagingSequence(t *testing.T) {
	executor, store, runtime := readyHarness(t)
	result, err := executor.Execute(context.Background(), validRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" || store.succeeded == nil || store.failed != nil {
		t.Fatalf("unexpected result: %+v", result)
	}
	want := []string{"preflight", "apply", "health"}
	if strings.Join(runtime.calls, ",") != strings.Join(want, ",") {
		t.Fatalf("calls = %v, want %v", runtime.calls, want)
	}
}

func TestExecutorRequiresVerifiedBackupBeforeProductionSideEffects(t *testing.T) {
	executor, store, runtime := readyHarness(t)
	request := validRequest(t)
	request.Target.Environment = "production"
	request.Backup.Required = true
	runtime.backup.Verified = false

	result, err := executor.Execute(context.Background(), request)
	if err == nil || result.ErrorCode != "BACKUP_NOT_VERIFIED" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if strings.Join(runtime.calls, ",") != "preflight,backup" {
		t.Fatalf("unexpected calls after backup failure: %v", runtime.calls)
	}
	if store.failed == nil || store.succeeded != nil {
		t.Fatal("failed result was not persisted")
	}
}

func TestExecutorRequiresEncryptedOffsiteProductionBackup(t *testing.T) {
	for _, field := range []string{"encrypted", "offsite"} {
		t.Run(field, func(t *testing.T) {
			executor, _, runtime := readyHarness(t)
			request := validRequest(t)
			request.Target.Environment = "production"
			request.Backup.Required = true
			if field == "encrypted" {
				runtime.backup.Encrypted = false
			} else {
				runtime.backup.OffsiteVerified = false
			}

			result, err := executor.Execute(context.Background(), request)
			if err == nil || result.ErrorCode != "BACKUP_NOT_VERIFIED" {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if strings.Join(runtime.calls, ",") != "preflight,backup" {
				t.Fatalf("production continued after incomplete backup evidence: %v", runtime.calls)
			}
		})
	}
}

func TestExecutorRejectsStaleExpectedReleaseBeforeRuntime(t *testing.T) {
	executor, store, runtime := readyHarness(t)
	request := validRequest(t)
	expected := "release-previous"
	actual := "release-other"
	request.Target.ExpectedCurrentReleaseID = &expected
	store.current = &actual

	result, err := executor.Execute(context.Background(), request)
	if err == nil || result.ErrorCode != "CURRENT_RELEASE_MISMATCH" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(runtime.calls) != 0 {
		t.Fatalf("runtime called before compare-and-set check: %v", runtime.calls)
	}
}

func TestExecutorStopsAtFirstRuntimeFailure(t *testing.T) {
	executor, store, runtime := readyHarness(t)
	runtime.failStep = "apply"

	result, err := executor.Execute(context.Background(), validRequest(t))
	if err == nil || result.FailedStep != "apply" || result.ErrorCode != "APPLY_FAILED" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if strings.Join(runtime.calls, ",") != "preflight,apply" || store.failed == nil {
		t.Fatalf("calls=%v failed=%v", runtime.calls, store.failed)
	}
}

func TestExecutorReturnsStoredReplayWithoutSideEffects(t *testing.T) {
	executor, store, runtime := readyHarness(t)
	stored := Result{SchemaVersion: 1, JobID: "job-018f1f4d", Status: "succeeded"}
	store.reservation = Reservation{State: ReservationReplay, Result: &stored}

	result, err := executor.Execute(context.Background(), validRequest(t))
	if err != nil || result.Status != "succeeded" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(runtime.calls) != 0 {
		t.Fatalf("replayed job caused side effects: %v", runtime.calls)
	}
}

func TestRequestHashIsDeterministicAndSensitive(t *testing.T) {
	request := validRequest(t)
	first, err := RequestSHA256(request)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := RequestSHA256(request)
	if first != second || !digestPattern.MatchString(first) {
		t.Fatalf("hashes = %q / %q", first, second)
	}
	request.ReleaseID = "release-b"
	changed, _ := RequestSHA256(request)
	if changed == first {
		t.Fatal("request hash did not change")
	}
}
