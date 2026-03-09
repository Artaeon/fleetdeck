package schedule

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubSystemctl replaces systemctlRun and systemctlProperty with no-op stubs
// for the duration of a test. It also redirects unitDir to a temp directory.
// The returned cleanup function restores the originals.
func stubSystemctl(t *testing.T) string {
	t.Helper()

	origRun := systemctlRun
	origProp := systemctlProperty
	origDir := unitDir

	tmpDir := t.TempDir()
	unitDir = tmpDir

	systemctlRun = func(args ...string) error { return nil }
	systemctlProperty = func(unit, property string) string { return "" }

	t.Cleanup(func() {
		systemctlRun = origRun
		systemctlProperty = origProp
		unitDir = origDir
	})

	return tmpDir
}

// ---------------------------------------------------------------------------
// Unit name helpers
// ---------------------------------------------------------------------------

func TestServiceUnitName(t *testing.T) {
	tests := []struct {
		project string
		want    string
	}{
		{"myapp", "fleetdeck-backup-myapp.service"},
		{"web-server", "fleetdeck-backup-web-server.service"},
		{"", "fleetdeck-backup-.service"},
	}
	for _, tc := range tests {
		got := serviceUnitName(tc.project)
		if got != tc.want {
			t.Errorf("serviceUnitName(%q) = %q, want %q", tc.project, got, tc.want)
		}
	}
}

func TestTimerUnitName(t *testing.T) {
	tests := []struct {
		project string
		want    string
	}{
		{"myapp", "fleetdeck-backup-myapp.timer"},
		{"web-server", "fleetdeck-backup-web-server.timer"},
		{"", "fleetdeck-backup-.timer"},
	}
	for _, tc := range tests {
		got := timerUnitName(tc.project)
		if got != tc.want {
			t.Errorf("timerUnitName(%q) = %q, want %q", tc.project, got, tc.want)
		}
	}
}

func TestServicePath(t *testing.T) {
	tmpDir := stubSystemctl(t)
	got := servicePath("proj")
	want := filepath.Join(tmpDir, "fleetdeck-backup-proj.service")
	if got != want {
		t.Errorf("servicePath = %q, want %q", got, want)
	}
}

func TestTimerPath(t *testing.T) {
	tmpDir := stubSystemctl(t)
	got := timerPath("proj")
	want := filepath.Join(tmpDir, "fleetdeck-backup-proj.timer")
	if got != want {
		t.Errorf("timerPath = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Service unit content generation
// ---------------------------------------------------------------------------

func TestInstallTimer_ServiceUnitContent(t *testing.T) {
	tmpDir := stubSystemctl(t)

	if err := InstallTimer("myapp", "daily"); err != nil {
		t.Fatalf("InstallTimer failed: %v", err)
	}

	svcBytes, err := os.ReadFile(filepath.Join(tmpDir, "fleetdeck-backup-myapp.service"))
	if err != nil {
		t.Fatalf("reading service unit: %v", err)
	}
	svc := string(svcBytes)

	// Verify key sections and fields
	checks := []struct {
		label    string
		contains string
	}{
		{"Unit section", "[Unit]"},
		{"Description", "Description=FleetDeck scheduled backup for myapp"},
		{"After network", "After=network.target"},
		{"Service section", "[Service]"},
		{"Type oneshot", "Type=oneshot"},
		{"ExecStart", "backup create myapp --type scheduled"},
		{"StandardOutput", "StandardOutput=journal"},
		{"StandardError", "StandardError=journal"},
	}
	for _, c := range checks {
		if !strings.Contains(svc, c.contains) {
			t.Errorf("service unit missing %s: expected to contain %q", c.label, c.contains)
		}
	}
}

func TestInstallTimer_TimerUnitContent(t *testing.T) {
	tmpDir := stubSystemctl(t)

	if err := InstallTimer("myapp", "*-*-* 02:00:00"); err != nil {
		t.Fatalf("InstallTimer failed: %v", err)
	}

	tmrBytes, err := os.ReadFile(filepath.Join(tmpDir, "fleetdeck-backup-myapp.timer"))
	if err != nil {
		t.Fatalf("reading timer unit: %v", err)
	}
	tmr := string(tmrBytes)

	checks := []struct {
		label    string
		contains string
	}{
		{"Unit section", "[Unit]"},
		{"Description", "Description=FleetDeck backup timer for myapp"},
		{"Timer section", "[Timer]"},
		{"OnCalendar", "OnCalendar=*-*-* 02:00:00"},
		{"Persistent", "Persistent=true"},
		{"RandomizedDelay", "RandomizedDelaySec=300"},
		{"Install section", "[Install]"},
		{"WantedBy", "WantedBy=timers.target"},
	}
	for _, c := range checks {
		if !strings.Contains(tmr, c.contains) {
			t.Errorf("timer unit missing %s: expected to contain %q", c.label, c.contains)
		}
	}
}

func TestInstallTimer_DailySchedule(t *testing.T) {
	tmpDir := stubSystemctl(t)

	if err := InstallTimer("proj", "daily"); err != nil {
		t.Fatalf("InstallTimer failed: %v", err)
	}

	tmr, _ := os.ReadFile(filepath.Join(tmpDir, "fleetdeck-backup-proj.timer"))
	if !strings.Contains(string(tmr), "OnCalendar=daily") {
		t.Error("timer should contain OnCalendar=daily")
	}
}

func TestInstallTimer_WeeklySchedule(t *testing.T) {
	tmpDir := stubSystemctl(t)

	if err := InstallTimer("proj", "weekly"); err != nil {
		t.Fatalf("InstallTimer failed: %v", err)
	}

	tmr, _ := os.ReadFile(filepath.Join(tmpDir, "fleetdeck-backup-proj.timer"))
	if !strings.Contains(string(tmr), "OnCalendar=weekly") {
		t.Error("timer should contain OnCalendar=weekly")
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestInstallTimer_EmptyProjectName(t *testing.T) {
	tmpDir := stubSystemctl(t)

	// Empty project name is technically allowed at this layer (no validation).
	if err := InstallTimer("", "daily"); err != nil {
		t.Fatalf("InstallTimer with empty project: %v", err)
	}

	// Both files should exist with the degenerate names.
	svcPath := filepath.Join(tmpDir, "fleetdeck-backup-.service")
	tmrPath := filepath.Join(tmpDir, "fleetdeck-backup-.timer")
	if _, err := os.Stat(svcPath); err != nil {
		t.Errorf("expected service file at %s: %v", svcPath, err)
	}
	if _, err := os.Stat(tmrPath); err != nil {
		t.Errorf("expected timer file at %s: %v", tmrPath, err)
	}
}

func TestInstallTimer_EmptySchedule(t *testing.T) {
	tmpDir := stubSystemctl(t)

	if err := InstallTimer("proj", ""); err != nil {
		t.Fatalf("InstallTimer with empty schedule: %v", err)
	}

	tmr, _ := os.ReadFile(filepath.Join(tmpDir, "fleetdeck-backup-proj.timer"))
	// The OnCalendar line should be present but with an empty value.
	if !strings.Contains(string(tmr), "OnCalendar=\n") {
		t.Error("timer should contain OnCalendar= with empty value")
	}
}

func TestInstallTimer_SpecialCharactersInSchedule(t *testing.T) {
	tmpDir := stubSystemctl(t)

	schedule := "Mon *-*-* 03:30:00"
	if err := InstallTimer("proj", schedule); err != nil {
		t.Fatalf("InstallTimer: %v", err)
	}

	tmr, _ := os.ReadFile(filepath.Join(tmpDir, "fleetdeck-backup-proj.timer"))
	if !strings.Contains(string(tmr), "OnCalendar="+schedule) {
		t.Errorf("timer should contain OnCalendar=%s", schedule)
	}
}

func TestInstallTimer_DaemonReloadCalled(t *testing.T) {
	stubSystemctl(t)

	var calls []string
	systemctlRun = func(args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		return nil
	}

	if err := InstallTimer("proj", "daily"); err != nil {
		t.Fatal(err)
	}

	if len(calls) != 1 || calls[0] != "daemon-reload" {
		t.Errorf("expected single daemon-reload call, got %v", calls)
	}
}

func TestInstallTimer_DaemonReloadFails(t *testing.T) {
	stubSystemctl(t)

	systemctlRun = func(args ...string) error {
		return fmt.Errorf("mock daemon-reload failure")
	}

	err := InstallTimer("proj", "daily")
	if err == nil {
		t.Fatal("expected error when daemon-reload fails")
	}
	if !strings.Contains(err.Error(), "reloading systemd") {
		t.Errorf("error should mention reloading systemd, got: %v", err)
	}
}

func TestInstallTimer_ServiceWriteFails(t *testing.T) {
	stubSystemctl(t)
	// Point unitDir to a path that does not exist so WriteFile fails.
	unitDir = "/nonexistent-dir-for-test"

	err := InstallTimer("proj", "daily")
	if err == nil {
		t.Fatal("expected error when service write fails")
	}
	if !strings.Contains(err.Error(), "writing service unit") {
		t.Errorf("error should mention writing service unit, got: %v", err)
	}
}

func TestInstallTimer_TimerWriteFails_CleansUpService(t *testing.T) {
	tmpDir := stubSystemctl(t)

	// Create the service file location as a directory so the timer write
	// will fail (we make the timer path a directory so WriteFile fails on it).
	tmrFilePath := filepath.Join(tmpDir, "fleetdeck-backup-proj.timer")
	if err := os.MkdirAll(tmrFilePath, 0755); err != nil {
		t.Fatal(err)
	}

	err := InstallTimer("proj", "daily")
	if err == nil {
		t.Fatal("expected error when timer write fails")
	}
	if !strings.Contains(err.Error(), "writing timer unit") {
		t.Errorf("error should mention writing timer unit, got: %v", err)
	}

	// Service file should have been cleaned up.
	svcPath := filepath.Join(tmpDir, "fleetdeck-backup-proj.service")
	if _, statErr := os.Stat(svcPath); !os.IsNotExist(statErr) {
		t.Error("service file should have been removed after timer write failure")
	}
}

// ---------------------------------------------------------------------------
// RemoveTimer
// ---------------------------------------------------------------------------

func TestRemoveTimer_RemovesFiles(t *testing.T) {
	tmpDir := stubSystemctl(t)

	// Create unit files first.
	svcPath := filepath.Join(tmpDir, "fleetdeck-backup-proj.service")
	tmrPath := filepath.Join(tmpDir, "fleetdeck-backup-proj.timer")
	os.WriteFile(svcPath, []byte("svc"), 0644)
	os.WriteFile(tmrPath, []byte("tmr"), 0644)

	if err := RemoveTimer("proj"); err != nil {
		t.Fatalf("RemoveTimer: %v", err)
	}

	if _, err := os.Stat(svcPath); !os.IsNotExist(err) {
		t.Error("service file should have been removed")
	}
	if _, err := os.Stat(tmrPath); !os.IsNotExist(err) {
		t.Error("timer file should have been removed")
	}
}

func TestRemoveTimer_NoFilesExist(t *testing.T) {
	stubSystemctl(t)

	// Should not error if files don't exist.
	if err := RemoveTimer("nonexistent"); err != nil {
		t.Errorf("RemoveTimer on nonexistent project: %v", err)
	}
}

func TestRemoveTimer_SystemctlCalls(t *testing.T) {
	tmpDir := stubSystemctl(t)

	// Create files so removal proceeds.
	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-proj.service"), []byte("svc"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-proj.timer"), []byte("tmr"), 0644)

	var calls []string
	systemctlRun = func(args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		return nil
	}

	if err := RemoveTimer("proj"); err != nil {
		t.Fatal(err)
	}

	// Should call stop, disable, and daemon-reload.
	expected := []string{
		"stop fleetdeck-backup-proj.timer",
		"disable fleetdeck-backup-proj.timer",
		"daemon-reload",
	}
	if len(calls) != len(expected) {
		t.Fatalf("expected %d systemctl calls, got %d: %v", len(expected), len(calls), calls)
	}
	for i, want := range expected {
		if calls[i] != want {
			t.Errorf("call[%d] = %q, want %q", i, calls[i], want)
		}
	}
}

func TestRemoveTimer_DaemonReloadFails(t *testing.T) {
	stubSystemctl(t)

	callCount := 0
	systemctlRun = func(args ...string) error {
		callCount++
		// Fail only on daemon-reload (3rd call).
		if callCount == 3 {
			return fmt.Errorf("mock fail")
		}
		return nil
	}

	err := RemoveTimer("proj")
	if err == nil {
		t.Fatal("expected error when daemon-reload fails")
	}
	if !strings.Contains(err.Error(), "reloading systemd") {
		t.Errorf("error should mention reloading systemd, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// EnableTimer / DisableTimer
// ---------------------------------------------------------------------------

func TestEnableTimer(t *testing.T) {
	stubSystemctl(t)

	var calls []string
	systemctlRun = func(args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		return nil
	}

	if err := EnableTimer("proj"); err != nil {
		t.Fatal(err)
	}

	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	want := "enable --now fleetdeck-backup-proj.timer"
	if calls[0] != want {
		t.Errorf("call = %q, want %q", calls[0], want)
	}
}

func TestEnableTimer_Error(t *testing.T) {
	stubSystemctl(t)

	systemctlRun = func(args ...string) error {
		return fmt.Errorf("mock enable failure")
	}

	err := EnableTimer("proj")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "enabling timer") {
		t.Errorf("error should mention enabling timer, got: %v", err)
	}
}

func TestDisableTimer(t *testing.T) {
	stubSystemctl(t)

	var calls []string
	systemctlRun = func(args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		return nil
	}

	if err := DisableTimer("proj"); err != nil {
		t.Fatal(err)
	}

	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	want := "disable --now fleetdeck-backup-proj.timer"
	if calls[0] != want {
		t.Errorf("call = %q, want %q", calls[0], want)
	}
}

func TestDisableTimer_Error(t *testing.T) {
	stubSystemctl(t)

	systemctlRun = func(args ...string) error {
		return fmt.Errorf("mock disable failure")
	}

	err := DisableTimer("proj")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "disabling timer") {
		t.Errorf("error should mention disabling timer, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetTimerStatus
// ---------------------------------------------------------------------------

func TestGetTimerStatus_NoTimer(t *testing.T) {
	stubSystemctl(t)

	_, err := GetTimerStatus("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent timer")
	}
	if !strings.Contains(err.Error(), "no backup timer found") {
		t.Errorf("error should mention no timer found, got: %v", err)
	}
}

func TestGetTimerStatus_Active(t *testing.T) {
	tmpDir := stubSystemctl(t)

	// Create a timer file.
	tmrContent := "[Timer]\nOnCalendar=daily\nPersistent=true\n"
	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-proj.timer"), []byte(tmrContent), 0644)

	systemctlProperty = func(unit, property string) string {
		switch property {
		case "ActiveState":
			return "active"
		case "NextElapseUSecRealtime":
			return "Mon 2025-01-06 02:00:00 UTC"
		case "LastTriggerUSec":
			return "Sun 2025-01-05 02:00:00 UTC"
		}
		return ""
	}

	status, err := GetTimerStatus("proj")
	if err != nil {
		t.Fatalf("GetTimerStatus: %v", err)
	}

	if status.ProjectName != "proj" {
		t.Errorf("ProjectName = %q, want %q", status.ProjectName, "proj")
	}
	if status.Schedule != "daily" {
		t.Errorf("Schedule = %q, want %q", status.Schedule, "daily")
	}
	if !status.Active {
		t.Error("expected Active=true")
	}
	if status.NextRun != "2025-01-06 02:00:00" {
		t.Errorf("NextRun = %q, want %q", status.NextRun, "2025-01-06 02:00:00")
	}
	if status.LastRun != "2025-01-05 02:00:00" {
		t.Errorf("LastRun = %q, want %q", status.LastRun, "2025-01-05 02:00:00")
	}
}

func TestGetTimerStatus_Waiting(t *testing.T) {
	tmpDir := stubSystemctl(t)

	tmrContent := "[Timer]\nOnCalendar=weekly\n"
	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-proj.timer"), []byte(tmrContent), 0644)

	systemctlProperty = func(unit, property string) string {
		if property == "ActiveState" {
			return "waiting"
		}
		return ""
	}

	status, err := GetTimerStatus("proj")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Active {
		t.Error("expected Active=true for 'waiting' state")
	}
}

func TestGetTimerStatus_Inactive(t *testing.T) {
	tmpDir := stubSystemctl(t)

	tmrContent := "[Timer]\nOnCalendar=daily\n"
	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-proj.timer"), []byte(tmrContent), 0644)

	systemctlProperty = func(unit, property string) string {
		if property == "ActiveState" {
			return "inactive"
		}
		return ""
	}

	status, err := GetTimerStatus("proj")
	if err != nil {
		t.Fatal(err)
	}
	if status.Active {
		t.Error("expected Active=false for 'inactive' state")
	}
}

func TestGetTimerStatus_EmptyProperties(t *testing.T) {
	tmpDir := stubSystemctl(t)

	tmrContent := "[Timer]\nOnCalendar=daily\n"
	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-proj.timer"), []byte(tmrContent), 0644)

	// systemctlProperty already returns "" from the stub.

	status, err := GetTimerStatus("proj")
	if err != nil {
		t.Fatal(err)
	}
	if status.Active {
		t.Error("expected Active=false when property is empty")
	}
	if status.NextRun != "" {
		t.Errorf("NextRun should be empty, got %q", status.NextRun)
	}
	if status.LastRun != "" {
		t.Errorf("LastRun should be empty, got %q", status.LastRun)
	}
}

// ---------------------------------------------------------------------------
// ListTimers
// ---------------------------------------------------------------------------

func TestListTimers_NoTimers(t *testing.T) {
	stubSystemctl(t)

	timers, err := ListTimers()
	if err != nil {
		t.Fatal(err)
	}
	if timers != nil {
		t.Errorf("expected nil, got %v", timers)
	}
}

func TestListTimers_MultipleTimers(t *testing.T) {
	tmpDir := stubSystemctl(t)

	// Create two timer files.
	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-alpha.timer"),
		[]byte("[Timer]\nOnCalendar=daily\n"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-beta.timer"),
		[]byte("[Timer]\nOnCalendar=weekly\n"), 0644)

	systemctlProperty = func(unit, property string) string {
		if property == "ActiveState" {
			if strings.Contains(unit, "alpha") {
				return "active"
			}
			return "inactive"
		}
		return ""
	}

	timers, err := ListTimers()
	if err != nil {
		t.Fatal(err)
	}
	if len(timers) != 2 {
		t.Fatalf("expected 2 timers, got %d", len(timers))
	}

	// Build a map for easy lookup (order not guaranteed by Glob).
	byName := make(map[string]TimerStatus)
	for _, ts := range timers {
		byName[ts.ProjectName] = ts
	}

	alpha, ok := byName["alpha"]
	if !ok {
		t.Fatal("missing alpha timer")
	}
	if alpha.Schedule != "daily" {
		t.Errorf("alpha.Schedule = %q, want daily", alpha.Schedule)
	}
	if !alpha.Active {
		t.Error("alpha should be active")
	}

	beta, ok := byName["beta"]
	if !ok {
		t.Fatal("missing beta timer")
	}
	if beta.Schedule != "weekly" {
		t.Errorf("beta.Schedule = %q, want weekly", beta.Schedule)
	}
	if beta.Active {
		t.Error("beta should be inactive")
	}
}

func TestListTimers_IgnoresNonFleetdeckFiles(t *testing.T) {
	tmpDir := stubSystemctl(t)

	// Create a non-fleetdeck timer file.
	os.WriteFile(filepath.Join(tmpDir, "other-thing.timer"),
		[]byte("[Timer]\nOnCalendar=daily\n"), 0644)
	// And one fleetdeck timer.
	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-only.timer"),
		[]byte("[Timer]\nOnCalendar=hourly\n"), 0644)

	timers, err := ListTimers()
	if err != nil {
		t.Fatal(err)
	}
	if len(timers) != 1 {
		t.Fatalf("expected 1 timer, got %d", len(timers))
	}
	if timers[0].ProjectName != "only" {
		t.Errorf("ProjectName = %q, want %q", timers[0].ProjectName, "only")
	}
}

func TestListTimers_WithTimestamps(t *testing.T) {
	tmpDir := stubSystemctl(t)

	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-proj.timer"),
		[]byte("[Timer]\nOnCalendar=daily\n"), 0644)

	systemctlProperty = func(unit, property string) string {
		switch property {
		case "ActiveState":
			return "active"
		case "NextElapseUSecRealtime":
			return "Tue 2025-03-11 02:00:00 UTC"
		case "LastTriggerUSec":
			return "Mon 2025-03-10 02:00:00 UTC"
		}
		return ""
	}

	timers, err := ListTimers()
	if err != nil {
		t.Fatal(err)
	}
	if len(timers) != 1 {
		t.Fatalf("expected 1 timer, got %d", len(timers))
	}
	if timers[0].NextRun != "2025-03-11 02:00:00" {
		t.Errorf("NextRun = %q, want %q", timers[0].NextRun, "2025-03-11 02:00:00")
	}
	if timers[0].LastRun != "2025-03-10 02:00:00" {
		t.Errorf("LastRun = %q, want %q", timers[0].LastRun, "2025-03-10 02:00:00")
	}
}

// ---------------------------------------------------------------------------
// readOnCalendar
// ---------------------------------------------------------------------------

func TestReadOnCalendar(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name: "daily schedule",
			content: "[Unit]\nDescription=Test timer\n\n[Timer]\n" +
				"OnCalendar=daily\nPersistent=true\n\n[Install]\nWantedBy=timers.target\n",
			expected: "daily",
		},
		{
			name: "custom schedule",
			content: "[Unit]\nDescription=Test timer\n\n[Timer]\n" +
				"OnCalendar=*-*-* 02:00:00\nPersistent=true\n\n[Install]\nWantedBy=timers.target\n",
			expected: "*-*-* 02:00:00",
		},
		{
			name:     "no OnCalendar line",
			content:  "[Timer]\nPersistent=true\n",
			expected: "",
		},
		{
			name:     "empty file",
			content:  "",
			expected: "",
		},
		{
			name:     "OnCalendar with empty value",
			content:  "[Timer]\nOnCalendar=\nPersistent=true\n",
			expected: "",
		},
		{
			name:     "multiple OnCalendar lines returns first",
			content:  "[Timer]\nOnCalendar=daily\nOnCalendar=weekly\n",
			expected: "daily",
		},
		{
			name:     "OnCalendar with whitespace around line",
			content:  "[Timer]\n  OnCalendar=monthly  \n",
			expected: "monthly",
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

func TestReadOnCalendar_NonexistentFile(t *testing.T) {
	got := readOnCalendar("/nonexistent/path/timer.unit")
	if got != "" {
		t.Errorf("readOnCalendar on nonexistent file = %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// formatSystemdTimestamp
// ---------------------------------------------------------------------------

func TestFormatSystemdTimestamp(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", "n/a"},
		{"n/a literal", "n/a", "n/a"},
		{"zero", "0", "n/a"},
		{"systemd format with timezone", "Mon 2025-01-06 02:00:00 UTC", "2025-01-06 02:00:00"},
		{"systemd format without timezone", "Mon 2025-01-06 02:00:00", "2025-01-06 02:00:00"},
		{"RFC3339", "2025-01-06T02:00:00Z", "2025-01-06 02:00:00"},
		{"unknown format passthrough", "some-unknown-format", "some-unknown-format"},
		{"partial date passthrough", "2025-01-06", "2025-01-06"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatSystemdTimestamp(tc.input)
			if got != tc.expected {
				t.Errorf("formatSystemdTimestamp(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// removeIfExists
// ---------------------------------------------------------------------------

func TestRemoveIfExists(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("nonexistent file", func(t *testing.T) {
		err := removeIfExists(filepath.Join(tmpDir, "nonexistent"))
		if err != nil {
			t.Errorf("removeIfExists on nonexistent file: %v", err)
		}
	})

	t.Run("existing file", func(t *testing.T) {
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
	})

	t.Run("directory fails", func(t *testing.T) {
		dirPath := filepath.Join(tmpDir, "subdir")
		os.Mkdir(dirPath, 0755)
		// Removing a non-empty or even empty dir with os.Remove on Linux
		// returns an error for directories in some cases; for empty dirs it
		// may succeed. We just verify no panic.
		_ = removeIfExists(dirPath)
	})
}

// ---------------------------------------------------------------------------
// fleetdeckBinary
// ---------------------------------------------------------------------------

func TestFleetdeckBinary(t *testing.T) {
	binary := fleetdeckBinary()
	if binary == "" {
		t.Error("fleetdeckBinary() returned empty string")
	}
	// The binary path should be an absolute path in test context.
	if !filepath.IsAbs(binary) {
		t.Errorf("fleetdeckBinary() = %q, expected absolute path", binary)
	}
}

// ---------------------------------------------------------------------------
// Full round-trip: install then remove
// ---------------------------------------------------------------------------

func TestInstallThenRemoveRoundTrip(t *testing.T) {
	tmpDir := stubSystemctl(t)

	// Install.
	if err := InstallTimer("roundtrip", "daily"); err != nil {
		t.Fatal(err)
	}

	svcPath := filepath.Join(tmpDir, "fleetdeck-backup-roundtrip.service")
	tmrPath := filepath.Join(tmpDir, "fleetdeck-backup-roundtrip.timer")

	// Verify files exist.
	if _, err := os.Stat(svcPath); err != nil {
		t.Errorf("service file missing after install: %v", err)
	}
	if _, err := os.Stat(tmrPath); err != nil {
		t.Errorf("timer file missing after install: %v", err)
	}

	// Remove.
	if err := RemoveTimer("roundtrip"); err != nil {
		t.Fatal(err)
	}

	// Verify files gone.
	if _, err := os.Stat(svcPath); !os.IsNotExist(err) {
		t.Error("service file should be gone after remove")
	}
	if _, err := os.Stat(tmrPath); !os.IsNotExist(err) {
		t.Error("timer file should be gone after remove")
	}
}

// ---------------------------------------------------------------------------
// Overwrite existing timer
// ---------------------------------------------------------------------------

func TestInstallTimer_OverwriteExisting(t *testing.T) {
	tmpDir := stubSystemctl(t)

	// Install with daily schedule.
	if err := InstallTimer("proj", "daily"); err != nil {
		t.Fatal(err)
	}

	// Overwrite with weekly schedule.
	if err := InstallTimer("proj", "weekly"); err != nil {
		t.Fatal(err)
	}

	tmr, _ := os.ReadFile(filepath.Join(tmpDir, "fleetdeck-backup-proj.timer"))
	if !strings.Contains(string(tmr), "OnCalendar=weekly") {
		t.Error("timer should contain updated schedule")
	}
	if strings.Contains(string(tmr), "OnCalendar=daily") {
		t.Error("timer should not contain old schedule")
	}
}
