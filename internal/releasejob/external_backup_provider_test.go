package releasejob

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExternalBackupProviderBindsEvidenceToReleaseAndTarget(t *testing.T) {
	request := validRequest(t)
	request.Target.Environment = "production"
	request.Backup.Required = true
	provider := NewExternalProductionBackupProvider("/bin/sh")
	provider.run = func(_ context.Context, command string, payload []byte) ([]byte, error) {
		if command == "" || !strings.Contains(string(payload), request.ManifestDigest) {
			t.Fatalf("command=%q payload=%s", command, payload)
		}
		return []byte(`{
			"schema_version":1,
			"release_id":"release-a",
			"target_id":"target-example",
			"backup_id":"backup-verified-1",
			"manifest_sha256":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"verified":true,
			"encrypted":true,
			"offsite_verified":true
		}`), nil
	}

	evidence, err := provider.CreateAndVerify(
		context.Background(),
		Project{Name: "target-example", Path: "/not-exposed"},
		request,
	)
	if err != nil || !evidence.Verified || !evidence.Encrypted || !evidence.OffsiteVerified {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}
}

func TestExternalBackupProviderRestoresOnlyTheBoundBackup(t *testing.T) {
	request := validRequest(t)
	request.Target.Environment = "production"
	provider := NewExternalProductionBackupProvider("/bin/sh")
	provider.run = func(_ context.Context, _ string, payload []byte) ([]byte, error) {
		if !strings.Contains(string(payload), `"operation":"restore-and-verify"`) ||
			!strings.Contains(string(payload), `"backup_id":"backup-verified-1"`) {
			t.Fatalf("restore payload=%s", payload)
		}
		return []byte(`{
			"schema_version":1,
			"release_id":"release-a",
			"target_id":"target-example",
			"backup_id":"backup-verified-1",
			"restored":true,
			"health_verified":true
		}`), nil
	}
	evidence, err := provider.Restore(
		context.Background(),
		Project{Name: "target-example"},
		request,
		BackupEvidence{
			BackupID:       "backup-verified-1",
			ManifestSHA256: "sha256:" + strings.Repeat("a", 64),
		},
	)
	if err != nil || !evidence.Restored || !evidence.HealthVerified {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}
}

func TestExternalBackupProviderRejectsMismatchedOrUnboundedEvidence(t *testing.T) {
	request := validRequest(t)
	request.Target.Environment = "production"
	provider := NewExternalProductionBackupProvider("/bin/sh")
	provider.run = func(context.Context, string, []byte) ([]byte, error) {
		return []byte(`{"schema_version":1,"release_id":"other","target_id":"target-example","backup_id":"backup-1","manifest_sha256":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","verified":true,"encrypted":true,"offsite_verified":true}`), nil
	}
	if _, err := provider.CreateAndVerify(context.Background(), Project{Name: "target-example"}, request); err == nil || !strings.Contains(err.Error(), "identity mismatch") {
		t.Fatalf("identity error=%v", err)
	}

	provider.run = func(context.Context, string, []byte) ([]byte, error) {
		return []byte(`{"schema_version":1,"release_id":"release-a","target_id":"target-example","backup_id":"backup-1","manifest_sha256":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","verified":true,"encrypted":true,"offsite_verified":true,"secret":"must-not-pass"}`), nil
	}
	if _, err := provider.CreateAndVerify(context.Background(), Project{Name: "target-example"}, request); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("strict evidence error=%v", err)
	}
}

func TestExternalBackupProviderRejectsWritableCommand(t *testing.T) {
	command := filepath.Join(t.TempDir(), "unsafe-backup")
	if err := os.WriteFile(command, []byte("#!/bin/sh\n"), 0777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(command, 0777); err != nil {
		t.Fatal(err)
	}
	provider := NewExternalProductionBackupProvider(command)
	if _, err := provider.CreateAndVerify(context.Background(), Project{}, validRequest(t)); err == nil || !strings.Contains(err.Error(), "unsafe ownership or permissions") {
		t.Fatalf("permission error=%v", err)
	}
}
