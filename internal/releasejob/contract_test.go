package releasejob

import (
	"strings"
	"testing"
)

const validJob = `{
  "schema_version": 1,
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
  "migration": { "mode": "preflight-and-apply" },
  "health_profile": "http-standard"
}`

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
		{"schema", func(r *Request) { r.SchemaVersion = 2 }},
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
