package monitor

import (
	"testing"
	"time"
)

func TestSummaryFreshnessAndLegacyResults(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name    string
		at      time.Time
		healthy bool
		kind    string
		want    string
	}{
		{"legacy success", now, true, "", "healthy"},
		{"ordinary failure", now, false, "", "unhealthy"},
		{"blocked overrides healthy flag", now, true, FailureCloudflareChallenge, "blocked"},
		{"stale", now.Add(-101 * time.Second), true, "", "stale"},
		{"future", now.Add(2 * time.Minute), true, "", "unknown"},
		{"missing", time.Time{}, false, "", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := Target{Name: "web"}
			state := &MonitorState{Targets: []Target{target}, Results: map[string]CheckResult{"web": {Target: target, CheckedAt: tc.at, Healthy: tc.healthy, FailureKind: tc.kind}}}
			result, err := Summarize(state, now)
			if err != nil || len(result) != 1 || result[0].Status != tc.want {
				t.Fatalf("summary=%+v error=%v", result, err)
			}
		})
	}
}
