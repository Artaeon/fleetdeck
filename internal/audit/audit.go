package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sync"
	"time"
)

const (
	DefaultLogPath  = "/var/log/fleetdeck/audit.log"
	maxFileSize     = 10 * 1024 * 1024 // 10MB
	maxRotatedFiles = 5
)

// AuditEntry represents a single audit log entry.
type AuditEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"`
	Project   string    `json:"project,omitempty"`
	User      string    `json:"user"`
	Details   string    `json:"details,omitempty"`
	Success   bool      `json:"success"`
}

var (
	mu      sync.Mutex
	logFile *os.File
	logPath string
)

// Init opens or creates the audit log file at the given path.
func Init(path string) error {
	mu.Lock()
	defer mu.Unlock()

	if path == "" {
		path = DefaultLogPath
	}
	logPath = path

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating audit log directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening audit log: %w", err)
	}
	logFile = f
	return nil
}

// Log writes a structured JSON audit entry to the log file.
func Log(action, project, details string, success bool) {
	mu.Lock()
	defer mu.Unlock()

	if logFile == nil {
		return
	}

	entry := AuditEntry{
		Timestamp: time.Now().UTC(),
		Action:    action,
		Project:   project,
		User:      currentUser(),
		Details:   details,
		Success:   success,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	data = append(data, '\n')

	_, _ = logFile.Write(data)

	// Check if rotation is needed
	if info, err := logFile.Stat(); err == nil && info.Size() >= maxFileSize {
		rotate()
	}
}

// Close closes the audit log file handle.
func Close() {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
}

// rotate rotates log files: audit.log -> audit.log.1, .1 -> .2, etc.
// Must be called while holding mu.
func rotate() {
	if logFile == nil || logPath == "" {
		return
	}

	logFile.Close()
	logFile = nil

	// Shift existing rotated files
	for i := maxRotatedFiles - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", logPath, i)
		dst := fmt.Sprintf("%s.%d", logPath, i+1)
		os.Rename(src, dst)
	}

	// Rotate current file to .1
	os.Rename(logPath, logPath+".1")

	// Open a fresh log file
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	logFile = f
}

// ReadRecent reads the most recent audit log entries (up to limit).
// Returns entries in reverse chronological order (newest first).
func ReadRecent(limit int) ([]AuditEntry, error) {
	mu.Lock()
	path := logPath
	mu.Unlock()

	if path == "" {
		path = DefaultLogPath
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	var entries []AuditEntry

	// Read from end for reverse chronological order
	for i := len(lines) - 1; i >= 0 && len(entries) < limit; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}
		var entry AuditEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // skip malformed entries
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func currentUser() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return "unknown"
}
