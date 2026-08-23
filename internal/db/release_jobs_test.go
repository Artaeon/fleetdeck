package db

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/releasejob"
)

func openReleaseJobDB(t *testing.T) *DB {
	t.Helper()
	database, err := Open(filepath.Join(t.TempDir(), "fleetdeck.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func releaseJobRequest() releasejob.Request {
	return releasejob.Request{
		SchemaVersion:  1,
		JobID:          "job-1",
		IdempotencyKey: "release-1.target-1.attempt-1",
		ReleaseID:      "release-1",
		ManifestDigest: "sha256:" + strings.Repeat("1", 64),
		Target: releasejob.Target{
			ID: "target-1", Environment: "staging", Profile: "standard",
		},
		Images: []releasejob.Image{{
			Component: "api",
			Reference: "registry.example.com/example/api@sha256:" + strings.Repeat("2", 64),
		}},
		Backup:        releasejob.BackupPolicy{Required: false},
		Migration:     releasejob.MigrationPolicy{Mode: "preflight-and-apply"},
		HealthProfile: "http-standard",
	}
}

func TestReleaseJobReservationIsIdempotentAndRejectsCollisions(t *testing.T) {
	database := openReleaseJobDB(t)
	ctx := context.Background()
	request := releaseJobRequest()
	hash, _ := releasejob.RequestSHA256(request)

	reservation, err := database.Reserve(ctx, request, hash, time.Now().UTC())
	if err != nil || reservation.State != releasejob.ReservationAcquired {
		t.Fatalf("first reservation = %+v, %v", reservation, err)
	}
	if _, err := database.Reserve(ctx, request, hash, time.Now().UTC()); err != releasejob.ErrJobInProgress {
		t.Fatalf("in-progress replay error = %v", err)
	}
	if _, err := database.Reserve(ctx, request, "sha256:"+strings.Repeat("9", 64), time.Now().UTC()); err == nil {
		t.Fatal("idempotency collision unexpectedly accepted")
	}
}

func TestReleaseJobReservationSerializesEachTarget(t *testing.T) {
	database := openReleaseJobDB(t)
	ctx := context.Background()
	first := releaseJobRequest()
	firstHash, _ := releasejob.RequestSHA256(first)
	if _, err := database.Reserve(ctx, first, firstHash, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	second := releaseJobRequest()
	second.JobID = "job-2"
	second.IdempotencyKey = "release-2.target-1.attempt-1"
	second.ReleaseID = "release-2"
	secondHash, _ := releasejob.RequestSHA256(second)
	if _, err := database.Reserve(ctx, second, secondHash, time.Now().UTC()); err != releasejob.ErrTargetBusy {
		t.Fatalf("second target reservation error = %v", err)
	}
}

func TestReleaseJobSuccessAtomicallyAdvancesCurrentReleaseAndReplays(t *testing.T) {
	database := openReleaseJobDB(t)
	ctx := context.Background()
	request := releaseJobRequest()
	hash, _ := releasejob.RequestSHA256(request)
	if _, err := database.Reserve(ctx, request, hash, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	finished := time.Now().UTC()
	result := releasejob.Result{
		SchemaVersion: 1, JobID: request.JobID, IdempotencyKey: request.IdempotencyKey,
		ReleaseID: request.ReleaseID, TargetID: request.Target.ID, Status: "succeeded",
		StartedAt: time.Now().UTC(), FinishedAt: &finished,
	}
	if err := database.CompleteSuccess(ctx, result, request.ManifestDigest); err != nil {
		t.Fatal(err)
	}
	current, err := database.CurrentRelease(ctx, request.Target.ID)
	if err != nil || current == nil || *current != request.ReleaseID {
		t.Fatalf("current release = %v, %v", current, err)
	}
	replayed, err := database.Reserve(ctx, request, hash, time.Now().UTC())
	if err != nil || replayed.State != releasejob.ReservationReplay || replayed.Result == nil {
		t.Fatalf("replay = %+v, %v", replayed, err)
	}
	if replayed.Result.ReleaseID != request.ReleaseID || replayed.Result.Status != "succeeded" {
		t.Fatalf("stored result = %+v", replayed.Result)
	}
}

func TestFailedReleaseDoesNotAdvanceCurrentRelease(t *testing.T) {
	database := openReleaseJobDB(t)
	ctx := context.Background()
	request := releaseJobRequest()
	hash, _ := releasejob.RequestSHA256(request)
	if _, err := database.Reserve(ctx, request, hash, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	finished := time.Now().UTC()
	result := releasejob.Result{
		SchemaVersion: 1, JobID: request.JobID, IdempotencyKey: request.IdempotencyKey,
		ReleaseID: request.ReleaseID, TargetID: request.Target.ID, Status: "failed",
		StartedAt: time.Now().UTC(), FinishedAt: &finished, FailedStep: "health",
		ErrorCode: "HEALTH_FAILED",
	}
	if err := database.CompleteFailure(ctx, result); err != nil {
		t.Fatal(err)
	}
	current, err := database.CurrentRelease(ctx, request.Target.ID)
	if err != nil || current != nil {
		t.Fatalf("failed job advanced current release: %v, %v", current, err)
	}
	replayed, err := database.Reserve(ctx, request, hash, time.Now().UTC())
	if err != nil || replayed.Result == nil || replayed.Result.ErrorCode != "HEALTH_FAILED" {
		t.Fatalf("failed replay = %+v, %v", replayed, err)
	}
}

func TestReleaseJobCompletionIsCompareAndSet(t *testing.T) {
	database := openReleaseJobDB(t)
	finished := time.Now().UTC()
	missing := releasejob.Result{
		SchemaVersion: 1, JobID: "missing", IdempotencyKey: "missing",
		ReleaseID: "release-1", TargetID: "target-1", Status: "failed", FinishedAt: &finished,
	}
	if err := database.CompleteFailure(context.Background(), missing); err == nil || !strings.Contains(err.Error(), "compare-and-set") {
		t.Fatalf("missing completion error = %v", err)
	}
}
