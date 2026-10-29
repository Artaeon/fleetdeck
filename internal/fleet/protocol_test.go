package fleet

import (
	"strings"
	"testing"
	"time"
)

func TestSnapshotRejectsMisleadingFreshness(t *testing.T) {
	now := time.Now()
	base := Snapshot{Schema: 1, NodeID: "alpha", GeneratedAt: now, Monitoring: Checks{Status: "unknown"}, Security: Security{Status: "unknown"}}
	if err := base.Validate("alpha", now); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-48 * time.Hour)
	for _, change := range []func(*Snapshot){
		func(s *Snapshot) { s.NodeID = "beta" }, func(s *Snapshot) { s.Schema = 2 }, func(s *Snapshot) { s.GeneratedAt = old }, func(s *Snapshot) { s.GeneratedAt = now.Add(time.Hour) },
		func(s *Snapshot) { s.Security = Security{Status: "ok", CheckedAt: &old} }, func(s *Snapshot) { s.Security = Security{Status: "ok", CheckedAt: &now, High: 1} },
		func(s *Snapshot) { s.Monitoring = Checks{Status: "ok", CheckedAt: &now, Unknown: 1} }, func(s *Snapshot) { s.Monitoring.Healthy = -1 },
		func(s *Snapshot) { s.Projects = []Project{{Name: "duplicate"}, {Name: "duplicate"}} },
	} {
		s := base
		change(&s)
		if s.Validate("alpha", now) == nil {
			t.Fatal("invalid snapshot accepted")
		}
	}
	if _, err := Decode(strings.NewReader(strings.Repeat(" ", MaxBytes+1)), "alpha", now); err == nil {
		t.Fatal("unbounded response accepted")
	}
}

func TestPermissionsAreExactAndDefaultDeny(t *testing.T) {
	n := Node{ViewGroups: []string{"read"}, DetailGroups: []string{"details"}}
	if n.CanView(nil) || n.CanView([]string{"READ"}) || n.CanDetail([]string{"read"}) {
		t.Fatal("implicit grant")
	}
	if !n.CanView([]string{"read"}) || !n.CanView([]string{"details"}) || !n.CanDetail([]string{"details"}) {
		t.Fatal("explicit grant ignored")
	}
}
