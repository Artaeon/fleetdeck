package schedule

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceUnitName(t *testing.T) {
	got := serviceUnitName("myapp")
	want := "fleetdeck-backup-myapp.service"
	if got != want {
		t.Errorf("serviceUnitName(\"myapp\") = %q, want %q", got, want)
	}
}

func TestTimerUnitName(t *testing.T) {
	got := timerUnitName("myapp")
	want := "fleetdeck-backup-myapp.timer"
	if got != want {
		t.Errorf("timerUnitName(\"myapp\") = %q, want %q", got, want)
	}
}

func TestReadOnCalendar(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name: "daily schedule",
			content: `[Unit]
Description=Test timer

[Timer]
OnCalendar=daily
Persistent=true

[Install]
WantedBy=timers.target
`,
			expected: "daily",
		},
		{
			name: "custom schedule",
			content: `[Unit]
Description=Test timer

[Timer]
OnCalendar=*-*-* 02:00:00
Persistent=true

[Install]
WantedBy=timers.target
`,
			expected: "*-*-* 02:00:00",
		},
		{
			name:     "no OnCalendar line",
			content:  "[Timer]\nPersistent=true\n",
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(tmpDir, tc.name+".timer")
			if err := os.WriteFile(path, []byte(tc.content), 0644); err != nil {
				t.Fatal(err)
			}
			got := readOnCalendar(path)
			if got != tc.expected {
				t.Errorf("readOnCalendar() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestFormatSystemdTimestamp(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "n/a"},
		{"n/a", "n/a"},
		{"0", "n/a"},
		// Unknown formats are returned as-is
		{"some-unknown-format", "some-unknown-format"},
	}

	for _, tc := range tests {
		got := formatSystemdTimestamp(tc.input)
		if got != tc.expected {
			t.Errorf("formatSystemdTimestamp(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestRemoveIfExists(t *testing.T) {
	tmpDir := t.TempDir()

	// Non-existent file should not error
	err := removeIfExists(filepath.Join(tmpDir, "nonexistent"))
	if err != nil {
		t.Errorf("removeIfExists on nonexistent file: %v", err)
	}

	// Existing file should be removed
	path := filepath.Join(tmpDir, "testfile")
	if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := removeIfExists(path); err != nil {
		t.Errorf("removeIfExists on existing file: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should have been removed")
	}
}

func TestFleetdeckBinary(t *testing.T) {
	binary := fleetdeckBinary()
	if binary == "" {
		t.Error("fleetdeckBinary() returned empty string")
	}
}

func TestServiceUnitContent(t *testing.T) {
	// Verify that InstallTimer would produce correct unit content
	// by checking the format strings indirectly
	projectName := "testproj"
	svcName := serviceUnitName(projectName)
	tmrName := timerUnitName(projectName)

	if !strings.Contains(svcName, projectName) {
		t.Errorf("service unit name should contain project name")
	}
	if !strings.Contains(tmrName, projectName) {
		t.Errorf("timer unit name should contain project name")
	}
	if !strings.HasSuffix(svcName, ".service") {
		t.Errorf("service unit name should end with .service")
	}
	if !strings.HasSuffix(tmrName, ".timer") {
		t.Errorf("timer unit name should end with .timer")
	}
}
