package releasejob

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

const maxBackupProviderOutput = 64 * 1024

type externalBackupRequest struct {
	SchemaVersion  int    `json:"schema_version"`
	Operation      string `json:"operation"`
	ReleaseID      string `json:"release_id"`
	ManifestDigest string `json:"manifest_digest"`
	TargetID       string `json:"target_id"`
	Project        string `json:"project"`
	Environment    string `json:"environment"`
	BackupID       string `json:"backup_id,omitempty"`
	BackupManifest string `json:"backup_manifest_sha256,omitempty"`
}

type externalBackupResult struct {
	SchemaVersion   int    `json:"schema_version"`
	ReleaseID       string `json:"release_id"`
	TargetID        string `json:"target_id"`
	BackupID        string `json:"backup_id"`
	ManifestSHA256  string `json:"manifest_sha256"`
	Verified        bool   `json:"verified"`
	Encrypted       bool   `json:"encrypted"`
	OffsiteVerified bool   `json:"offsite_verified"`
}

type externalRollbackResult struct {
	SchemaVersion  int    `json:"schema_version"`
	ReleaseID      string `json:"release_id"`
	TargetID       string `json:"target_id"`
	BackupID       string `json:"backup_id"`
	Restored       bool   `json:"restored"`
	HealthVerified bool   `json:"health_verified"`
}

type externalBackupRunner func(context.Context, string, []byte) ([]byte, error)

type ExternalProductionBackupProvider struct {
	command string
	run     externalBackupRunner
}

func NewExternalProductionBackupProvider(command string) *ExternalProductionBackupProvider {
	return &ExternalProductionBackupProvider{command: command, run: runExternalBackupCommand}
}

func (p *ExternalProductionBackupProvider) CreateAndVerify(
	ctx context.Context,
	project Project,
	request Request,
) (BackupEvidence, error) {
	command, err := validateExternalBackupCommand(p.command)
	if err != nil {
		return BackupEvidence{}, err
	}
	payload, err := json.Marshal(externalBackupRequest{
		SchemaVersion:  SchemaVersion,
		Operation:      "create-and-verify",
		ReleaseID:      request.ReleaseID,
		ManifestDigest: request.ManifestDigest,
		TargetID:       request.Target.ID,
		Project:        project.Name,
		Environment:    request.Target.Environment,
	})
	if err != nil {
		return BackupEvidence{}, errors.New("encode production backup request")
	}
	output, err := p.run(ctx, command, append(payload, '\n'))
	if err != nil {
		return BackupEvidence{}, errors.New("production backup provider failed")
	}
	result, err := decodeExternalBackupResult(output)
	if err != nil {
		return BackupEvidence{}, err
	}
	if result.SchemaVersion != SchemaVersion || result.ReleaseID != request.ReleaseID ||
		result.TargetID != request.Target.ID {
		return BackupEvidence{}, errors.New("production backup evidence identity mismatch")
	}
	if !identifierPattern.MatchString(result.BackupID) || !digestPattern.MatchString(result.ManifestSHA256) {
		return BackupEvidence{}, errors.New("production backup evidence is malformed")
	}
	return BackupEvidence{
		BackupID:        result.BackupID,
		ManifestSHA256:  result.ManifestSHA256,
		Verified:        result.Verified,
		Encrypted:       result.Encrypted,
		OffsiteVerified: result.OffsiteVerified,
	}, nil
}

func (p *ExternalProductionBackupProvider) Restore(
	ctx context.Context,
	project Project,
	request Request,
	backup BackupEvidence,
) (RollbackEvidence, error) {
	command, err := validateExternalBackupCommand(p.command)
	if err != nil {
		return RollbackEvidence{}, err
	}
	payload, err := json.Marshal(externalBackupRequest{
		SchemaVersion:  SchemaVersion,
		Operation:      "restore-and-verify",
		ReleaseID:      request.ReleaseID,
		ManifestDigest: request.ManifestDigest,
		TargetID:       request.Target.ID,
		Project:        project.Name,
		Environment:    request.Target.Environment,
		BackupID:       backup.BackupID,
		BackupManifest: backup.ManifestSHA256,
	})
	if err != nil {
		return RollbackEvidence{}, errors.New("encode production rollback request")
	}
	output, err := p.run(ctx, command, append(payload, '\n'))
	if err != nil {
		return RollbackEvidence{}, errors.New("production rollback provider failed")
	}
	result, err := decodeExternalRollbackResult(output)
	if err != nil {
		return RollbackEvidence{}, err
	}
	if result.SchemaVersion != SchemaVersion || result.ReleaseID != request.ReleaseID ||
		result.TargetID != request.Target.ID || result.BackupID != backup.BackupID {
		return RollbackEvidence{}, errors.New("production rollback evidence identity mismatch")
	}
	return RollbackEvidence{
		BackupID:       result.BackupID,
		Restored:       result.Restored,
		HealthVerified: result.HealthVerified,
	}, nil
}

func validateExternalBackupCommand(command string) (string, error) {
	if command == "" || !filepath.IsAbs(command) {
		return "", errors.New("production backup command must be an absolute path")
	}
	resolved, err := filepath.EvalSymlinks(command)
	if err != nil {
		return "", errors.New("production backup command is unavailable")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Mode().Perm()&0111 == 0 {
		return "", errors.New("production backup command has unsafe ownership or permissions")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 {
		return "", errors.New("production backup command must be owned by root")
	}
	return resolved, nil
}

func runExternalBackupCommand(ctx context.Context, command string, payload []byte) ([]byte, error) {
	process := exec.CommandContext(ctx, command)
	process.Stdin = bytes.NewReader(payload)
	process.Stderr = io.Discard
	stdout, err := process.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := process.Start(); err != nil {
		return nil, err
	}
	output, readErr := io.ReadAll(io.LimitReader(stdout, maxBackupProviderOutput+1))
	if len(output) > maxBackupProviderOutput {
		_ = process.Process.Kill()
		_ = process.Wait()
		return nil, errors.New("production backup provider output exceeds limit")
	}
	waitErr := process.Wait()
	if readErr != nil {
		return nil, readErr
	}
	if waitErr != nil {
		return nil, waitErr
	}
	return output, nil
}

func decodeExternalBackupResult(output []byte) (externalBackupResult, error) {
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.DisallowUnknownFields()
	var result externalBackupResult
	if err := decoder.Decode(&result); err != nil {
		return externalBackupResult{}, fmt.Errorf("decode production backup evidence: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return externalBackupResult{}, errors.New("production backup evidence contains trailing JSON")
	}
	return result, nil
}

func decodeExternalRollbackResult(output []byte) (externalRollbackResult, error) {
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.DisallowUnknownFields()
	var result externalRollbackResult
	if err := decoder.Decode(&result); err != nil {
		return externalRollbackResult{}, fmt.Errorf("decode production rollback evidence: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return externalRollbackResult{}, errors.New("production rollback evidence contains trailing JSON")
	}
	return result, nil
}
