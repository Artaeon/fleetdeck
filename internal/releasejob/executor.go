package releasejob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrJobInProgress = errors.New("release job is already in progress")
var ErrTargetBusy = errors.New("release target already has a job in progress")

type ReservationState string

const (
	ReservationAcquired ReservationState = "acquired"
	ReservationReplay   ReservationState = "replay"
)

type Reservation struct {
	State  ReservationState
	Result *Result
}

type Result struct {
	SchemaVersion  int                `json:"schema_version"`
	JobID          string             `json:"job_id"`
	IdempotencyKey string             `json:"idempotency_key"`
	ReleaseID      string             `json:"release_id"`
	TargetID       string             `json:"target_id"`
	Status         string             `json:"status"`
	StartedAt      time.Time          `json:"started_at"`
	FinishedAt     *time.Time         `json:"finished_at,omitempty"`
	Preflight      *PreflightEvidence `json:"preflight,omitempty"`
	Backup         *BackupEvidence    `json:"backup,omitempty"`
	Rollback       *RollbackEvidence  `json:"rollback,omitempty"`
	Apply          *ApplyEvidence     `json:"apply,omitempty"`
	Health         *HealthEvidence    `json:"health,omitempty"`
	FailedStep     string             `json:"failed_step,omitempty"`
	ErrorCode      string             `json:"error_code,omitempty"`
}

type PreflightEvidence struct {
	ComposeValid   bool `json:"compose_valid"`
	ImagesResolved bool `json:"images_resolved"`
	MigrationReady bool `json:"migration_ready"`
}

type BackupEvidence struct {
	BackupID        string `json:"backup_id"`
	ManifestSHA256  string `json:"manifest_sha256"`
	Verified        bool   `json:"verified"`
	Encrypted       bool   `json:"encrypted"`
	OffsiteVerified bool   `json:"offsite_verified"`
}

type RollbackEvidence struct {
	BackupID       string `json:"backup_id"`
	Restored       bool   `json:"restored"`
	HealthVerified bool   `json:"health_verified"`
}

type ApplyEvidence struct {
	AppliedComponents int  `json:"applied_components"`
	MigrationApplied  bool `json:"migration_applied"`
}

type HealthEvidence struct {
	Profile      string `json:"profile"`
	ChecksPassed int    `json:"checks_passed"`
	Attempts     int    `json:"attempts,omitempty"`
}

type JobStore interface {
	Reserve(ctx context.Context, request Request, requestSHA256 string, startedAt time.Time) (Reservation, error)
	CurrentRelease(ctx context.Context, targetID string) (*string, error)
	CompleteSuccess(ctx context.Context, result Result, manifestDigest string) error
	CompleteFailure(ctx context.Context, result Result) error
}

type Runtime interface {
	Preflight(ctx context.Context, request Request) (PreflightEvidence, error)
	CreateAndVerifyBackup(ctx context.Context, request Request) (BackupEvidence, error)
	RestoreBackup(ctx context.Context, request Request, backup BackupEvidence) (RollbackEvidence, error)
	Apply(ctx context.Context, request Request) (ApplyEvidence, error)
	VerifyHealth(ctx context.Context, request Request) (HealthEvidence, error)
}

type Executor struct {
	store   JobStore
	runtime Runtime
	now     func() time.Time
}

func NewExecutor(store JobStore, runtime Runtime) *Executor {
	return &Executor{store: store, runtime: runtime, now: func() time.Time { return time.Now().UTC() }}
}

func RequestSHA256(request Request) (string, error) {
	canonical, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("marshal release job: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func (e *Executor) Execute(ctx context.Context, request Request) (Result, error) {
	if err := request.Validate(); err != nil {
		return Result{}, err
	}
	requestHash, err := RequestSHA256(request)
	if err != nil {
		return Result{}, err
	}
	startedAt := e.now()
	reservation, err := e.store.Reserve(ctx, request, requestHash, startedAt)
	if err != nil {
		return Result{}, fmt.Errorf("reserve release job: %w", err)
	}
	if reservation.State == ReservationReplay {
		if reservation.Result == nil {
			return Result{}, errors.New("release job replay has no stored result")
		}
		return *reservation.Result, nil
	}
	if reservation.State != ReservationAcquired {
		return Result{}, ErrJobInProgress
	}

	result := Result{
		SchemaVersion:  SchemaVersion,
		JobID:          request.JobID,
		IdempotencyKey: request.IdempotencyKey,
		ReleaseID:      request.ReleaseID,
		TargetID:       request.Target.ID,
		Status:         "running",
		StartedAt:      startedAt,
	}

	current, err := e.store.CurrentRelease(ctx, request.Target.ID)
	if err != nil {
		return e.fail(ctx, result, "expected-current-release", "CURRENT_RELEASE_UNAVAILABLE")
	}
	if !sameOptionalString(current, request.Target.ExpectedCurrentReleaseID) {
		return e.fail(ctx, result, "expected-current-release", "CURRENT_RELEASE_MISMATCH")
	}

	preflight, err := e.runtime.Preflight(ctx, request)
	if err != nil || !preflight.ComposeValid || !preflight.ImagesResolved || !preflight.MigrationReady {
		result.Preflight = &preflight
		return e.fail(ctx, result, "preflight", "PREFLIGHT_FAILED")
	}
	result.Preflight = &preflight

	if request.Backup.Required {
		backup, err := e.runtime.CreateAndVerifyBackup(ctx, request)
		result.Backup = &backup
		if err != nil || !backup.Verified || !backup.Encrypted || !backup.OffsiteVerified ||
			!digestPattern.MatchString(backup.ManifestSHA256) || backup.BackupID == "" {
			return e.fail(ctx, result, "backup", "BACKUP_NOT_VERIFIED")
		}
	}

	apply, err := e.runtime.Apply(ctx, request)
	result.Apply = &apply
	if err != nil || apply.AppliedComponents != len(request.Images) {
		if result.Backup != nil {
			return e.recoverAndFail(ctx, result, request, *result.Backup, "apply", "APPLY_FAILED")
		}
		return e.fail(ctx, result, "apply", "APPLY_FAILED")
	}
	if request.Migration.Mode == "preflight-and-apply" && !apply.MigrationApplied {
		if result.Backup != nil {
			return e.recoverAndFail(ctx, result, request, *result.Backup, "apply", "MIGRATION_NOT_APPLIED")
		}
		return e.fail(ctx, result, "apply", "MIGRATION_NOT_APPLIED")
	}

	health, err := e.runtime.VerifyHealth(ctx, request)
	result.Health = &health
	if err != nil || health.Profile != request.HealthProfile || health.ChecksPassed < 1 {
		if result.Backup != nil {
			return e.recoverAndFail(ctx, result, request, *result.Backup, "health", "HEALTH_FAILED")
		}
		return e.fail(ctx, result, "health", "HEALTH_FAILED")
	}

	finishedAt := e.now()
	result.Status = "succeeded"
	result.FinishedAt = &finishedAt
	if err := e.store.CompleteSuccess(ctx, result, request.ManifestDigest); err != nil {
		return Result{}, fmt.Errorf("persist successful release job: %w", err)
	}
	return result, nil
}

func (e *Executor) recoverAndFail(
	ctx context.Context,
	result Result,
	request Request,
	backup BackupEvidence,
	originalStep, originalCode string,
) (Result, error) {
	rollback, err := e.runtime.RestoreBackup(ctx, request, backup)
	result.Rollback = &rollback
	if err != nil || rollback.BackupID != backup.BackupID || !rollback.Restored || !rollback.HealthVerified {
		return e.fail(ctx, result, "rollback", "ROLLBACK_FAILED")
	}
	return e.fail(ctx, result, originalStep, originalCode)
}

func (e *Executor) fail(ctx context.Context, result Result, step, code string) (Result, error) {
	finishedAt := e.now()
	result.Status = "failed"
	result.FinishedAt = &finishedAt
	result.FailedStep = step
	result.ErrorCode = code
	if err := e.store.CompleteFailure(ctx, result); err != nil {
		return Result{}, fmt.Errorf("persist failed release job: %w", err)
	}
	return result, fmt.Errorf("release job failed at %s (%s)", step, code)
}

func sameOptionalString(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
