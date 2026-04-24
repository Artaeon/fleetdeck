package db

import (
	"path/filepath"
	"testing"
	"time"
)

// TestPruneDeploymentsKeepsNewest pins the newest-N-survive contract.
// A mealtime-style project with 50 recorded deploys should end up with
// the 10 most recent after PruneDeployments(10).
func TestPruneDeploymentsKeepsNewest(t *testing.T) {
	database := openTempDB(t)
	proj := insertTestProject(t, database, "mealtime")

	// 50 deployments, staggered so started_at differs on each.
	for i := 0; i < 50; i++ {
		d := &Deployment{
			ID:        randID(),
			ProjectID: proj.ID,
			CommitSHA: "sha-" + padN(i),
			Status:    "success",
			StartedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := database.CreateDeployment(d); err != nil {
			t.Fatalf("CreateDeployment %d: %v", i, err)
		}
	}

	removed, err := database.PruneDeployments(proj.ID, 10)
	if err != nil {
		t.Fatalf("PruneDeployments: %v", err)
	}
	if removed != 40 {
		t.Errorf("expected 40 rows removed, got %d", removed)
	}

	remaining, err := database.ListDeployments(proj.ID, 0)
	if err != nil {
		t.Fatalf("ListDeployments: %v", err)
	}
	if len(remaining) != 10 {
		t.Fatalf("expected 10 remaining, got %d", len(remaining))
	}
	// ListDeployments returns newest-first, so the first entry should
	// be the last one we inserted (i=49).
	if remaining[0].CommitSHA != "sha-"+padN(49) {
		t.Errorf("newest entry should be sha-%s, got %s", padN(49), remaining[0].CommitSHA)
	}
}

// TestPruneDeploymentsIsolatesProjects guards against a SQL bug that
// deletes across project boundaries. Pruning project A must not touch
// project B's rows — a multi-tenant production host would be one
// errant query away from losing every peer's deploy history.
func TestPruneDeploymentsIsolatesProjects(t *testing.T) {
	database := openTempDB(t)
	a := insertTestProject(t, database, "project-a")
	b := insertTestProject(t, database, "project-b")

	for i := 0; i < 20; i++ {
		database.CreateDeployment(&Deployment{
			ID: randID(), ProjectID: a.ID, Status: "success",
			StartedAt: time.Now().Add(time.Duration(i) * time.Second),
		})
		database.CreateDeployment(&Deployment{
			ID: randID(), ProjectID: b.ID, Status: "success",
			StartedAt: time.Now().Add(time.Duration(i) * time.Second),
		})
	}

	removed, err := database.PruneDeployments(a.ID, 5)
	if err != nil {
		t.Fatalf("PruneDeployments: %v", err)
	}
	if removed != 15 {
		t.Errorf("expected to remove 15 a-rows, got %d", removed)
	}

	bRows, _ := database.ListDeployments(b.ID, 0)
	if len(bRows) != 20 {
		t.Errorf("project-b should still have 20 deploys, got %d — pruning crossed project boundary!", len(bRows))
	}
}

// TestPruneDeploymentsZeroKeepIsNoop keeps retention opt-in. Passing
// keep=0 must not wipe every row — otherwise a misconfigured scheduler
// with retention=0 is a 'delete everything' button.
func TestPruneDeploymentsZeroKeepIsNoop(t *testing.T) {
	database := openTempDB(t)
	proj := insertTestProject(t, database, "keep-zero")
	for i := 0; i < 5; i++ {
		database.CreateDeployment(&Deployment{
			ID: randID(), ProjectID: proj.ID, Status: "success",
			StartedAt: time.Now().Add(time.Duration(i) * time.Second),
		})
	}

	removed, err := database.PruneDeployments(proj.ID, 0)
	if err != nil {
		t.Fatalf("PruneDeployments: %v", err)
	}
	if removed != 0 {
		t.Errorf("keep=0 should remove nothing, got %d", removed)
	}
	rows, _ := database.ListDeployments(proj.ID, 0)
	if len(rows) != 5 {
		t.Errorf("keep=0 should preserve all rows, have %d", len(rows))
	}
}

// TestPruneAllDeploymentsSumsPerProject verifies the multi-project
// convenience wrapper returns the total across every project.
func TestPruneAllDeploymentsSumsPerProject(t *testing.T) {
	database := openTempDB(t)
	insertTestProject(t, database, "a")
	insertTestProject(t, database, "b")

	for _, name := range []string{"a", "b"} {
		p, _ := database.GetProject(name)
		for i := 0; i < 8; i++ {
			database.CreateDeployment(&Deployment{
				ID: randID(), ProjectID: p.ID, Status: "success",
				StartedAt: time.Now().Add(time.Duration(i) * time.Second),
			})
		}
	}

	removed, err := database.PruneAllDeployments(3)
	if err != nil {
		t.Fatalf("PruneAllDeployments: %v", err)
	}
	// 8 rows per project * 2 projects = 16; we keep 3 per project = 6.
	if removed != 10 {
		t.Errorf("expected 10 rows removed (8-3 per project x 2), got %d", removed)
	}
}

// --- helpers ---

func openTempDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	database, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func insertTestProject(t *testing.T, database *DB, name string) *Project {
	t.Helper()
	p := &Project{
		ID:          randID(),
		Name:        name,
		Domain:      name + ".example.com",
		LinuxUser:   "fleetdeck-" + name,
		ProjectPath: "/tmp/" + name,
		Template:    "node",
	}
	if err := database.CreateProject(p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return p
}

func randID() string {
	// Use the uuid package via a Deployment indirectly — but CreateDeployment
	// generates one if empty, so the tests don't strictly need distinct IDs.
	// Use time.Now().UnixNano() as a simple unique-enough string.
	return "d-" + time.Now().Format("20060102T150405.000000000") + "-" + padN(int(time.Now().UnixNano()%1000))
}

func padN(n int) string {
	s := ""
	if n < 10 {
		s += "0"
	}
	if n < 100 {
		s += "0"
	}
	if n < 1000 {
		s += "0"
	}
	return s + itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
