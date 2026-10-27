package remote

import "testing"

type versionRunner string

func (r versionRunner) Run(cmd string) (string, error) { return string(r), nil }
func TestRemoteVersionObservation(t *testing.T) {
	good := ReadVersion(versionRunner(`{"version":"1.2.3","commit":"abcdef1234567"}`))
	if good.Version != "1.2.3" || good.Commit != "abcdef1234567" {
		t.Fatal("lost version observation")
	}
	for _, bad := range []string{`fleetdeck 1.2.3`, `{"version":"x\nforged"}`, `{"version":"1.2.3","commit":"secret"}`} {
		if ReadVersion(versionRunner(bad)).Version != "" {
			t.Fatal("untrusted output accepted")
		}
	}
}
