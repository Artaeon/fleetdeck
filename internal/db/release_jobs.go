package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/releasejob"
)

func (db *DB) Reserve(
	ctx context.Context,
	request releasejob.Request,
	requestSHA256 string,
	startedAt time.Time,
) (releasejob.Reservation, error) {
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return releasejob.Reservation{}, fmt.Errorf("marshal release job request: %w", err)
	}

	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return releasejob.Reservation{}, err
	}
	defer tx.Rollback()

	var storedHash, status string
	var resultJSON sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT request_sha256, status, result_json FROM release_jobs WHERE idempotency_key = ?`,
		request.IdempotencyKey,
	).Scan(&storedHash, &status, &resultJSON)
	if err == nil {
		if storedHash != requestSHA256 {
			return releasejob.Reservation{}, errors.New("idempotency key was already used for a different release job")
		}
		if status == "running" {
			return releasejob.Reservation{}, releasejob.ErrJobInProgress
		}
		if status != "succeeded" && status != "failed" {
			return releasejob.Reservation{}, fmt.Errorf("stored release job has invalid status %q", status)
		}
		if !resultJSON.Valid {
			return releasejob.Reservation{}, errors.New("terminal release job has no stored result")
		}
		var result releasejob.Result
		if err := json.Unmarshal([]byte(resultJSON.String), &result); err != nil {
			return releasejob.Reservation{}, fmt.Errorf("decode stored release job result: %w", err)
		}
		return releasejob.Reservation{State: releasejob.ReservationReplay, Result: &result}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return releasejob.Reservation{}, err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO release_jobs (
			idempotency_key, job_id, release_id, manifest_digest, target_id,
			environment, request_sha256, request_json, status, started_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'running', ?)`,
		request.IdempotencyKey,
		request.JobID,
		request.ReleaseID,
		request.ManifestDigest,
		request.Target.ID,
		request.Target.Environment,
		requestSHA256,
		string(requestJSON),
		startedAt,
	)
	if err != nil {
		return releasejob.Reservation{}, fmt.Errorf("insert release job reservation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return releasejob.Reservation{}, err
	}
	return releasejob.Reservation{State: releasejob.ReservationAcquired}, nil
}

func (db *DB) CurrentRelease(ctx context.Context, targetID string) (*string, error) {
	var releaseID string
	err := db.conn.QueryRowContext(ctx,
		`SELECT current_release_id FROM release_targets WHERE target_id = ?`,
		targetID,
	).Scan(&releaseID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &releaseID, nil
}

func (db *DB) CompleteSuccess(
	ctx context.Context,
	result releasejob.Result,
	manifestDigest string,
) error {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return err
	}
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	updated, err := tx.ExecContext(ctx,
		`UPDATE release_jobs SET status = 'succeeded', result_json = ?, finished_at = ?
		 WHERE idempotency_key = ? AND job_id = ? AND status = 'running'`,
		string(resultJSON), result.FinishedAt, result.IdempotencyKey, result.JobID,
	)
	if err != nil {
		return err
	}
	if err := requireOneUpdated(updated, "release job success"); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO release_targets (target_id, current_release_id, manifest_digest, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(target_id) DO UPDATE SET
			current_release_id = excluded.current_release_id,
			manifest_digest = excluded.manifest_digest,
			updated_at = excluded.updated_at`,
		result.TargetID, result.ReleaseID, manifestDigest, result.FinishedAt,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) CompleteFailure(ctx context.Context, result releasejob.Result) error {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return err
	}
	updated, err := db.conn.ExecContext(ctx,
		`UPDATE release_jobs SET status = 'failed', result_json = ?, finished_at = ?
		 WHERE idempotency_key = ? AND job_id = ? AND status = 'running'`,
		string(resultJSON), result.FinishedAt, result.IdempotencyKey, result.JobID,
	)
	if err != nil {
		return err
	}
	return requireOneUpdated(updated, "release job failure")
}

func requireOneUpdated(result sql.Result, operation string) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("%s lost its compare-and-set race", operation)
	}
	return nil
}
