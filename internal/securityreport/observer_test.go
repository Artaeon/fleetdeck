package securityreport

import (
	"bytes"
	"encoding/json"
	"github.com/fleetdeck/fleetdeck/internal/fleet"
	"os"
	"testing"
	"time"
)

func TestObserverBindsCurrentSnapshotAndRetainsHistoryWithoutSecrets(t *testing.T) {
	now := time.Now().UTC()
	path := privateState(t)
	at := now.Add(-time.Hour)
	snap := fleet.Snapshot{Schema: 1, NodeID: "node-a", Version: "1.2.3", GeneratedAt: now, Monitoring: fleet.Checks{Status: "unknown"}, Security: fleet.Security{Status: "attention", CheckedAt: &at, Critical: 2, High: 3, RuntimeFindings: 1}, Projects: []fleet.Project{{Name: "confidential-project"}}}
	capture := func(host string) error {
		b, _ := json.Marshal(snap)
		return CaptureObserver(path, host, bytes.NewReader(b), now)
	}
	if capture("other") == nil {
		t.Fatal("accepted wrong enrolled host")
	}
	if err := capture("node-a"); err != nil {
		t.Fatal(err)
	}
	if err := capture("node-a"); err != nil {
		t.Fatal(err)
	}
	bytesOnDisk, _ := os.ReadFile(path)
	if bytes.Contains(bytesOnDisk, []byte("confidential-project")) {
		t.Fatal("saved raw project data")
	}
	in := Input{"node-a", path}
	r, err := BuildSources(nil, []Input{in}, now, 26*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	n := r.Nodes[0]
	if n.Critical != 2 || n.Warnings != 4 || n.Unknown != 1 || n.Status != "attention" || len(n.history) != 1 || n.Version != "1.2.3" {
		t.Fatalf("invalid summary %+v", n)
	}
	stale, _ := BuildSources(nil, []Input{in}, now.Add(3*time.Hour), 26*time.Hour)
	if stale.Nodes[0].Status != "unknown" || len(stale.Nodes[0].history) != 1 {
		t.Fatal("stale reader marked current")
	}
	if _, err = BuildSources([]Input{in}, []Input{in}, now, 26*time.Hour); err == nil {
		t.Fatal("duplicate enrollment")
	}
	snap.GeneratedAt = now.Add(-5 * time.Minute)
	if capture("node-a") == nil {
		t.Fatal("accepted obsolete export")
	}
}
