package monitor

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	targets := []Target{
		{Name: "web", URL: "https://example.com", Interval: 30 * time.Second},
	}
	results := []CheckResult{
		{Target: targets[0], Healthy: true, StatusCode: 200, CheckedAt: time.Now()},
	}

	if err := SaveState(path, targets, results); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	state, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	if len(state.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(state.Targets))
	}
	if state.Targets[0].Name != "web" {
		t.Errorf("target name = %q, want %q", state.Targets[0].Name, "web")
	}
	if r, ok := state.Results["web"]; !ok {
		t.Error("result for 'web' not found")
	} else if !r.Healthy {
		t.Error("expected healthy=true")
	}
	if state.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}
}

func TestSaveStateCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "deep", "state.json")

	if err := SaveState(path, nil, nil); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state file not created: %v", err)
	}
}

func TestSaveStateAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	if err := SaveState(path, nil, nil); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	// Verify no .tmp file left behind
	tmp := path + ".tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("temporary file should not exist after successful save")
	}
}

func TestLoadStateNotFound(t *testing.T) {
	_, err := LoadState("/nonexistent/path/state.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadStateMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	os.WriteFile(path, []byte("{invalid"), 0o644)

	_, err := LoadState(path)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestSaveStateToDiskNoPath(t *testing.T) {
	m := New(nil, nil, 3)
	// StatePath is empty, should be a no-op
	if err := m.SaveStateToDisk(); err != nil {
		t.Fatalf("SaveStateToDisk with empty path: %v", err)
	}
}

func TestSaveStateToDiskWithPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	m := NewWithState(
		[]Target{{Name: "test", URL: "http://localhost"}},
		nil, 3, path,
	)

	if err := m.SaveStateToDisk(); err != nil {
		t.Fatalf("SaveStateToDisk: %v", err)
	}

	state, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(state.Targets) != 1 {
		t.Errorf("expected 1 target, got %d", len(state.Targets))
	}
}

func TestNewWithStateSetsPath(t *testing.T) {
	m := NewWithState(nil, nil, 3, "/tmp/test-state.json")
	if m.StatePath != "/tmp/test-state.json" {
		t.Errorf("StatePath = %q, want %q", m.StatePath, "/tmp/test-state.json")
	}
}

func TestSaveStateMultipleResults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	targets := []Target{
		{Name: "a", URL: "http://a.com"},
		{Name: "b", URL: "http://b.com"},
		{Name: "c", URL: "http://c.com"},
	}
	results := []CheckResult{
		{Target: targets[0], Healthy: true, StatusCode: 200},
		{Target: targets[1], Healthy: false, StatusCode: 500},
		{Target: targets[2], Healthy: true, StatusCode: 200},
	}

	if err := SaveState(path, targets, results); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	state, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(state.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(state.Results))
	}
	if state.Results["b"].Healthy {
		t.Error("result 'b' should be unhealthy")
	}
}
