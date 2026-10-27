package health

import "testing"

func TestExplicitOneShotReadiness(t *testing.T) {
	for _, tc := range []struct {
		name, entry string
		healthy     bool
	}{
		{"completed migration", `{"State":"exited","ExitCode":0,"Labels":"com.docker.compose.service=migrate,io.fleetdeck.lifecycle=oneshot"}`, true},
		{"running migration", `{"State":"running","ExitCode":0,"Labels":"io.fleetdeck.lifecycle=oneshot"}`, false},
		{"failed migration", `{"State":"exited","ExitCode":1,"Labels":"io.fleetdeck.lifecycle=oneshot"}`, false},
		{"unknown exit", `{"State":"exited","Labels":"io.fleetdeck.lifecycle=oneshot"}`, false},
		{"stopped daemon", `{"State":"exited","ExitCode":0}`, false},
		{"similar label", `{"State":"exited","ExitCode":0,"Labels":"other.io.fleetdeck.lifecycle=oneshot"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := ParseHealthReport(tc.entry)
			if err != nil || r.Healthy != tc.healthy {
				t.Fatalf("readiness: %v %v", r, err)
			}
		})
	}
}
