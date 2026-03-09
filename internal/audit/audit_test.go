package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer Close()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected audit log file to be created")
	}
}

func TestInitCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "nested", "audit.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer Close()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected audit log file to be created in nested directory")
	}
}

func TestLogWritesJSONEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	Log("project.create", "myapp", "created project", true)
	Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}

	var entry AuditEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("parsing JSON entry: %v", err)
	}

	if entry.Action != "project.create" {
		t.Errorf("expected action project.create, got %s", entry.Action)
	}
	if entry.Project != "myapp" {
		t.Errorf("expected project myapp, got %s", entry.Project)
	}
	if entry.Details != "created project" {
		t.Errorf("expected details 'created project', got %s", entry.Details)
	}
	if !entry.Success {
		t.Error("expected success to be true")
	}
	if entry.User == "" {
		t.Error("expected user to be set")
	}
	if entry.Timestamp.IsZero() {
		t.Error("expected timestamp to be set")
	}
}

func TestLogMultipleEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	Log("project.start", "app1", "started", true)
	Log("project.stop", "app2", "stopped", true)
	Log("project.destroy", "app3", "failed to destroy", false)
	Close()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening log file: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry AuditEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("line %d: invalid JSON: %v", count+1, err)
		}
		count++
	}

	if count != 3 {
		t.Errorf("expected 3 entries, got %d", count)
	}
}

func TestLogNoopWhenNotInitialized(t *testing.T) {
	// Reset state
	mu.Lock()
	logFile = nil
	mu.Unlock()

	// Should not panic
	Log("test", "proj", "details", true)
}

func TestCloseIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	Close()
	Close() // Should not panic
}

func TestRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Write enough data to trigger rotation (>10MB)
	bigDetails := strings.Repeat("x", 1024)
	for i := 0; i < 11000; i++ {
		Log("test.action", "proj", bigDetails, true)
	}
	Close()

	// Check that the rotated file exists
	if _, err := os.Stat(path + ".1"); os.IsNotExist(err) {
		t.Error("expected rotated file audit.log.1 to exist")
	}

	// The main log file should still exist
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected main audit.log to still exist after rotation")
	}
}

func TestLogFailureEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	Log("backup.create", "webapp", "disk full", false)
	Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}

	var entry AuditEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}

	if entry.Success {
		t.Error("expected success to be false")
	}
	if entry.Action != "backup.create" {
		t.Errorf("expected action backup.create, got %s", entry.Action)
	}
}
