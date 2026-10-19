package releasejob

import (
	"strings"
	"testing"
)

const validJob = `{
  "schema_version": 2,
  "job_id": "job-018f1f4d",
  "idempotency_key": "release-a.target-example.attempt-1",
  "release_id": "release-a",
  "manifest_digest": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
  "target": {
    "id": "target-example",
    "environment": "staging",
    "profile": "standard",
    "expected_current_release_id": null
  },
  "images": [
    {
      "component": "api",
      "reference": "registry.example.com/example/api@sha256:2222222222222222222222222222222222222222222222222222222222222222"
    }
  ],
  "backup": { "required": false },
  "migration": { "mode": "preflight-and-apply", "compatibility": "unreviewed" },
  "recovery": { "mode": "manual" },
  "health_profile": "http-standard"
}`

func configureProductionRequest(job *Request) {
	job.Target.Environment = "production"
	job.Backup.Required = true
	job.Migration.Compatibility = "online-expand-contract"
	job.Recovery.Mode = "runtime-only"
}

func TestDecodeValidBoundedJob(t *testing.T) {
	job, err := Decode(strings.NewReader(validJob))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if job.Target.Environment != "staging" || job.Images[0].Component != "api" {
		t.Fatalf("unexpected decoded job: %+v", job)
	}
}

func TestDecodeRejectsUnknownAndTrailingData(t *testing.T) {
	unknown := strings.Replace(validJob, `"job_id":`, `"command":"rm -rf /", "job_id":`, 1)
	if _, err := Decode(strings.NewReader(unknown)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field error = %v", err)
	}
	if _, err := Decode(strings.NewReader(validJob + `{}`)); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("trailing data error = %v", err)
	}
}

func TestDecodeRejectsOversizedRequest(t *testing.T) {
	if _, err := Decode(strings.NewReader(strings.Repeat(" ", MaxRequestSize+1))); err == nil {
		t.Fatal("oversized request unexpectedly accepted")
	}
}

func TestValidateRejectsMutableOrMalformedImages(t *testing.T) {
	job, err := Decode(strings.NewReader(validJob))
	if err != nil {
		t.Fatal(err)
	}
	for _, reference := range []string{
		"registry.example.com/example/api:latest",
		"registry.example.com/example/api:v1",
		"registry.example.com/example/api@sha256:ABC",
		"https://registry.example.com/example/api@sha256:" + strings.Repeat("2", 64),
	} {
		job.Images[0].Reference = reference
		if err := job.Validate(); err == nil {
			t.Fatalf("mutable or malformed image %q accepted", reference)
		}
	}
}

func TestValidateRejectsDuplicateComponents(t *testing.T) {
	job, err := Decode(strings.NewReader(validJob))
	if err != nil {
		t.Fatal(err)
	}
	job.Images = append(job.Images, job.Images[0])
	if err := job.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate component") {
		t.Fatalf("duplicate component error = %v", err)
	}
}

func TestValidateRequiresProductionBackup(t *testing.T) {
	job, err := Decode(strings.NewReader(validJob))
	if err != nil {
		t.Fatal(err)
	}
	job.Target.Environment = "production"
	job.Backup.Required = false
	if err := job.Validate(); err == nil || !strings.Contains(err.Error(), "must require a backup") {
		t.Fatalf("production backup error = %v", err)
	}
	job.Backup.Required = true
	job.Migration.Compatibility = "online-expand-contract"
	job.Recovery.Mode = "runtime-only"
	if err := job.Validate(); err != nil {
		t.Fatalf("production job with backup rejected: %v", err)
	}
}

func TestValidateRejectsUnboundedValues(t *testing.T) {
	job, err := Decode(strings.NewReader(validJob))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*Request)
	}{
		{"schema", func(r *Request) { r.SchemaVersion = 1 }},
		{"environment", func(r *Request) { r.Target.Environment = "development" }},
		{"migration", func(r *Request) { r.Migration.Mode = "shell" }},
		{"identifier", func(r *Request) { r.Target.ID = "../../target" }},
		{"digest", func(r *Request) { r.ManifestDigest = "sha256:nope" }},
		{"empty-images", func(r *Request) { r.Images = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copy := job
			tt.mutate(&copy)
			if err := copy.Validate(); err == nil {
				t.Fatal("invalid job unexpectedly accepted")
			}
		})
	}
}

func TestValidateRejectsAutomaticDatabaseRestoreForProduction(t *testing.T) {
	job, err := Decode(strings.NewReader(validJob))
	if err != nil {
		t.Fatal(err)
	}
	job.Target.Environment = "production"
	job.Backup.Required = true
	job.Migration.Compatibility = "online-expand-contract"
	job.Recovery.Mode = "restore-backup"
	if err := job.Validate(); err == nil || !strings.Contains(err.Error(), "runtime-only") {
		t.Fatalf("production recovery error = %v", err)
	}
}

func TestValidateRejectsUnreviewedProductionMigration(t *testing.T) {
	job, err := Decode(strings.NewReader(validJob))
	if err != nil {
		t.Fatal(err)
	}
	job.Target.Environment = "production"
	job.Backup.Required = true
	job.Recovery.Mode = "runtime-only"
	if err := job.Validate(); err == nil || !strings.Contains(err.Error(), "online-expand-contract") {
		t.Fatalf("production compatibility error = %v", err)
	}
}

func TestValidateEnforcesEnvironmentSafetyMatrix(t *testing.T) {
	tests := []struct {
		name          string
		environment   string
		migrationMode string
		compatibility string
		recoveryMode  string
		wantError     bool
	}{
		{
			name:        "staging unreviewed migration",
			environment: "staging", migrationMode: "preflight-and-apply",
			compatibility: "unreviewed", recoveryMode: "manual",
		},
		{
			name:        "staging maintenance migration",
			environment: "staging", migrationMode: "preflight-and-apply",
			compatibility: "maintenance-write-freeze", recoveryMode: "manual",
		},
		{
			name:        "staging without migration",
			environment: "staging", migrationMode: "none",
			compatibility: "not-applicable", recoveryMode: "manual",
		},
		{
			name:        "staging automatic recovery",
			environment: "staging", migrationMode: "preflight-and-apply",
			compatibility: "unreviewed", recoveryMode: "runtime-only", wantError: true,
		},
		{
			name:        "production reviewed online migration",
			environment: "production", migrationMode: "preflight-and-apply",
			compatibility: "online-expand-contract", recoveryMode: "runtime-only",
		},
		{
			name:        "production without migration",
			environment: "production", migrationMode: "none",
			compatibility: "not-applicable", recoveryMode: "runtime-only",
		},
		{
			name:        "production maintenance migration",
			environment: "production", migrationMode: "preflight-and-apply",
			compatibility: "maintenance-write-freeze", recoveryMode: "runtime-only", wantError: true,
		},
		{
			name:        "production manual recovery",
			environment: "production", migrationMode: "preflight-and-apply",
			compatibility: "online-expand-contract", recoveryMode: "manual", wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			job, err := Decode(strings.NewReader(validJob))
			if err != nil {
				t.Fatal(err)
			}
			job.Target.Environment = test.environment
			job.Backup.Required = test.environment == "production"
			job.Migration = MigrationPolicy{
				Mode:          test.migrationMode,
				Compatibility: test.compatibility,
			}
			job.Recovery.Mode = test.recoveryMode

			err = job.Validate()
			if test.wantError && err == nil {
				t.Fatal("unsafe environment policy was accepted")
			}
			if !test.wantError && err != nil {
				t.Fatalf("safe environment policy was rejected: %v", err)
			}
		})
	}
}
