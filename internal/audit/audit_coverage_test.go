package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// Log rotation: verify files are rotated at size limit
// ---------------------------------------------------------------------------

func TestRotationCreatesNumberedFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Write enough data to trigger multiple rotations.
	// Each entry with ~1 KB detail, 11000 entries -> ~11 MB > 10 MB limit.
	// After two rotations we should see .1 and potentially .2.
	bigDetails := strings.Repeat("R", 1024)
	for i := 0; i < 22000; i++ {
		Log("rotation.test", "proj", bigDetails, true)
	}
	Close()

	// .1 must exist after at least one rotation.
	if _, err := os.Stat(path + ".1"); os.IsNotExist(err) {
		t.Error("expected audit.log.1 after rotation")
	}

	// .2 should exist after a second rotation.
	if _, err := os.Stat(path + ".2"); os.IsNotExist(err) {
		t.Error("expected audit.log.2 after second rotation")
	}

	// Main file should still exist and be smaller than the 10 MB limit.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("main log should still exist: %v", err)
	}
	if info.Size() >= maxFileSize {
		t.Errorf("main log should be smaller than maxFileSize after rotation, got %d", info.Size())
	}
}

func TestRotationShiftsFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Force three rotations by writing large bursts.
	bigDetails := strings.Repeat("S", 2048)
	for round := 0; round < 3; round++ {
		for i := 0; i < 6000; i++ {
			Log("shift.test", "proj", bigDetails, true)
		}
	}
	Close()

	// Verify that .1 and .2 both exist and .1 is a newer rotation than .2.
	info1, err1 := os.Stat(path + ".1")
	info2, err2 := os.Stat(path + ".2")
	if err1 != nil {
		t.Fatal("expected .1 to exist after multiple rotations")
	}
	if err2 != nil {
		t.Fatal("expected .2 to exist after multiple rotations")
	}
	// .1 should generally be at least as large or larger than .2 depending on
	// timing, but both should be non-empty.
	if info1.Size() == 0 {
		t.Error(".1 should not be empty")
	}
	if info2.Size() == 0 {
		t.Error(".2 should not be empty")
	}
}

func TestRotationMaxFilesRespected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Write a huge amount to trigger many rotations.
	bigDetails := strings.Repeat("M", 2048)
	for i := 0; i < 60000; i++ {
		Log("max.test", "proj", bigDetails, true)
	}
	Close()

	// Files .1 through .5 (maxRotatedFiles) may exist, but .6 should not
	// because rotate() only shifts up to maxRotatedFiles-1.
	if _, err := os.Stat(path + ".6"); err == nil {
		t.Error("should not create more than maxRotatedFiles rotated files")
	}
}

// ---------------------------------------------------------------------------
// JSON structure: verify log entries have all required fields
// ---------------------------------------------------------------------------

func TestLogEntryHasAllFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init: %v", err)
	}

	Log("deploy.start", "webapp", "deploying v2.0", true)
	Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("JSON unmarshal: %v", err)
	}

	requiredFields := []string{"timestamp", "action", "project", "user", "details", "success"}
	for _, field := range requiredFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("log entry missing required field %q", field)
		}
	}
}

func TestLogEntryTimestampIsUTC(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init: %v", err)
	}

	Log("test.utc", "proj", "", true)
	Close()

	data, _ := os.ReadFile(path)
	var entry AuditEntry
	json.Unmarshal(data, &entry)

	if entry.Timestamp.IsZero() {
		t.Fatal("timestamp should not be zero")
	}
	// The timestamp should be in UTC (offset name "UTC" or +00:00).
	if entry.Timestamp.Location().String() != "UTC" {
		t.Errorf("timestamp should be UTC, got %s", entry.Timestamp.Location())
	}
}

func TestLogEntryEmptyProject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init: %v", err)
	}

	Log("system.startup", "", "system booting", true)
	Close()

	data, _ := os.ReadFile(path)
	var entry AuditEntry
	json.Unmarshal(data, &entry)

	if entry.Project != "" {
		t.Errorf("expected empty project, got %q", entry.Project)
	}
	// When project is empty, the JSON should use omitempty and exclude it.
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)
	if _, exists := raw["project"]; exists {
		t.Error("project field with empty value should be omitted (omitempty)")
	}
}

func TestLogEntrySuccessFalse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init: %v", err)
	}

	Log("deploy.fail", "app", "out of memory", false)
	Close()

	data, _ := os.ReadFile(path)
	var entry AuditEntry
	json.Unmarshal(data, &entry)

	if entry.Success {
		t.Error("expected success=false")
	}
	if entry.Details != "out of memory" {
		t.Errorf("details = %q, want 'out of memory'", entry.Details)
	}
}

// ---------------------------------------------------------------------------
// Concurrent logging: verify thread safety
// ---------------------------------------------------------------------------

func TestConcurrentLogging(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init: %v", err)
	}

	var wg sync.WaitGroup
	numGoroutines := 20
	entriesPerGoroutine := 50

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < entriesPerGoroutine; i++ {
				Log("concurrent.test", "proj", "goroutine log", true)
			}
		}(g)
	}
	wg.Wait()
	Close()

	// Read back and verify we have the expected number of valid JSON lines.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	validCount := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry AuditEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("invalid JSON line: %v", err)
			continue
		}
		validCount++
	}

	expected := numGoroutines * entriesPerGoroutine
	if validCount != expected {
		t.Errorf("expected %d valid entries, got %d", expected, validCount)
	}
}

// ---------------------------------------------------------------------------
// Close and reopen: verify audit system can be restarted
// ---------------------------------------------------------------------------

func TestCloseAndReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	// First session.
	if err := Init(path); err != nil {
		t.Fatalf("Init (1st): %v", err)
	}
	Log("session1.action", "proj1", "first session", true)
	Close()

	// Second session (reopen the same path).
	if err := Init(path); err != nil {
		t.Fatalf("Init (2nd): %v", err)
	}
	Log("session2.action", "proj2", "second session", true)
	Close()

	// Read back and verify both entries are present.
	entries, err := ReadRecent(10)
	if err != nil {
		t.Fatalf("ReadRecent: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries after reopen, got %d", len(entries))
	}

	// Newest first.
	if entries[0].Action != "session2.action" {
		t.Errorf("expected newest entry first, got %s", entries[0].Action)
	}
	if entries[1].Action != "session1.action" {
		t.Errorf("expected oldest entry last, got %s", entries[1].Action)
	}
}

func TestLogAfterCloseIsNoop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init: %v", err)
	}
	Log("before.close", "proj", "before", true)
	Close()

	// Logging after Close should be a no-op (logFile == nil).
	Log("after.close", "proj", "should not appear", true)

	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "after.close") {
		t.Error("entries logged after Close should not be written")
	}
}

// ---------------------------------------------------------------------------
// Edge cases: very long messages, special characters in project names
// ---------------------------------------------------------------------------

func TestLogVeryLongMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init: %v", err)
	}

	longDetails := strings.Repeat("Z", 100_000) // 100 KB message
	Log("long.message", "proj", longDetails, true)
	Close()

	data, _ := os.ReadFile(path)
	var entry AuditEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("JSON unmarshal: %v", err)
	}
	if len(entry.Details) != 100_000 {
		t.Errorf("details length = %d, want 100000", len(entry.Details))
	}
}

func TestLogSpecialCharactersInProject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// JSON special characters: quotes, backslashes, newlines, angle brackets.
	specialProject := `my"project\with<special>&chars`
	Log("special.chars", specialProject, "details with \"quotes\" and\nnewlines", true)
	Close()

	data, _ := os.ReadFile(path)
	var entry AuditEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("JSON with special chars should be valid: %v\nraw: %s", err, string(data))
	}
	if entry.Project != specialProject {
		t.Errorf("project = %q, want %q", entry.Project, specialProject)
	}
}

func TestLogEmptyAction(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init: %v", err)
	}

	Log("", "proj", "empty action", true)
	Close()

	data, _ := os.ReadFile(path)
	var entry AuditEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("JSON unmarshal: %v", err)
	}
	if entry.Action != "" {
		t.Errorf("expected empty action, got %q", entry.Action)
	}
}

func TestLogEmptyDetails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init: %v", err)
	}

	Log("action", "proj", "", true)
	Close()

	data, _ := os.ReadFile(path)
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	// Details with omitempty should be omitted when empty.
	if _, exists := raw["details"]; exists {
		t.Error("empty details should be omitted due to omitempty tag")
	}
}

// ---------------------------------------------------------------------------
// ReadRecent edge cases
// ---------------------------------------------------------------------------

func TestReadRecentMalformedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	// Write a mix of valid and invalid JSON lines.
	content := `{"timestamp":"2025-01-01T00:00:00Z","action":"good1","user":"u","success":true}
not json at all
{"timestamp":"2025-01-02T00:00:00Z","action":"good2","user":"u","success":true}
{broken
`
	os.WriteFile(path, []byte(content), 0644)

	mu.Lock()
	logPath = path
	mu.Unlock()

	entries, err := ReadRecent(10)
	if err != nil {
		t.Fatalf("ReadRecent: %v", err)
	}
	// Only the two valid entries should be returned.
	if len(entries) != 2 {
		t.Errorf("expected 2 valid entries, got %d", len(entries))
	}
}

func TestReadRecentZeroLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init: %v", err)
	}
	Log("test", "p", "d", true)
	Close()

	entries, err := ReadRecent(0)
	if err != nil {
		t.Fatalf("ReadRecent(0): %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("ReadRecent(0) should return 0 entries, got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// Init edge cases
// ---------------------------------------------------------------------------

func TestInitDefaultPath(t *testing.T) {
	// Passing empty string should use DefaultLogPath, which likely fails
	// due to permission issues in test, but should not panic.
	err := Init("")
	if err != nil {
		// Expected: permission denied creating /var/log/fleetdeck/.
		// Just verify it does not panic.
		return
	}
	Close()
}

func TestInitInvalidPath(t *testing.T) {
	// A path under a file (not a directory) should fail.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	os.WriteFile(blocker, []byte("x"), 0644)

	err := Init(filepath.Join(blocker, "subdir", "audit.log"))
	if err == nil {
		Close()
		t.Fatal("Init with blocked path should return error")
	}
}
