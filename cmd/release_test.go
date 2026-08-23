package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

const validReleaseJob = `{
  "schema_version": 1,
  "job_id": "job-example",
  "idempotency_key": "release-example.target-example.attempt-1",
  "release_id": "release-example",
  "manifest_digest": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
  "target": {
    "id": "target-example",
    "environment": "staging",
    "profile": "standard",
    "expected_current_release_id": null
  },
  "images": [{
    "component": "api",
    "reference": "registry.example.com/example/api@sha256:2222222222222222222222222222222222222222222222222222222222222222"
  }],
  "backup": { "required": false },
  "migration": { "mode": "preflight-and-apply" },
  "health_profile": "http-standard"
}`

func TestValidateReleaseJobPrintsBoundedMachineResult(t *testing.T) {
	var output bytes.Buffer
	if err := validateReleaseJob(strings.NewReader(validReleaseJob), &output); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "valid" || result["target_id"] != "target-example" {
		t.Fatalf("unexpected validation result: %v", result)
	}
	if !strings.HasPrefix(result["request_sha256"].(string), "sha256:") {
		t.Fatalf("missing request digest: %v", result)
	}
	if strings.Contains(output.String(), "registry.example.com") {
		t.Fatal("validation output exposed full image references")
	}
}

func TestValidateReleaseJobRejectsCommandInjectionField(t *testing.T) {
	malicious := strings.Replace(validReleaseJob, `"job_id":`, `"command":"echo unsafe", "job_id":`, 1)
	if err := validateReleaseJob(strings.NewReader(malicious), &bytes.Buffer{}); err == nil {
		t.Fatal("unknown command field unexpectedly accepted")
	}
}
