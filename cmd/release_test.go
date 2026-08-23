package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/releasejob"
)

type releaseExecutorStub struct {
	result releasejob.Result
	err    error
}

func (s releaseExecutorStub) Execute(context.Context, releasejob.Request) (releasejob.Result, error) {
	return s.result, s.err
}

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

func TestApplyReleaseJobWritesMachineReadableSuccess(t *testing.T) {
	var output bytes.Buffer
	result := releasejob.Result{
		SchemaVersion: 1,
		JobID:         "job-example",
		ReleaseID:     "release-example",
		TargetID:      "target-example",
		Status:        "succeeded",
	}
	request, got, err := applyReleaseJob(
		context.Background(),
		strings.NewReader(validReleaseJob),
		&output,
		releaseExecutorStub{result: result},
	)
	if err != nil || got.Status != "succeeded" || request.Target.ID != "target-example" {
		t.Fatalf("request=%+v result=%+v err=%v", request, got, err)
	}
	if !strings.Contains(output.String(), `"status":"succeeded"`) {
		t.Fatalf("machine result = %s", output.String())
	}
}

func TestApplyReleaseJobWritesBoundedFailureEvidence(t *testing.T) {
	var output bytes.Buffer
	result := releasejob.Result{
		SchemaVersion: 1,
		JobID:         "job-example",
		ReleaseID:     "release-example",
		TargetID:      "target-example",
		Status:        "failed",
		FailedStep:    "health",
		ErrorCode:     "HEALTH_FAILED",
	}
	_, got, err := applyReleaseJob(
		context.Background(),
		strings.NewReader(validReleaseJob),
		&output,
		releaseExecutorStub{result: result, err: errors.New("bounded failure")},
	)
	if err == nil || got.ErrorCode != "HEALTH_FAILED" {
		t.Fatalf("result=%+v err=%v", got, err)
	}
	if strings.Contains(output.String(), "bounded failure") || !strings.Contains(output.String(), "HEALTH_FAILED") {
		t.Fatalf("unsafe or missing failure output: %s", output.String())
	}
}
