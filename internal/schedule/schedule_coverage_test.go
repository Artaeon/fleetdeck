package schedule

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// Timer file generation: verify service and timer unit content details
// ---------------------------------------------------------------------------

func TestInstallTimerServiceExecStartContainsBinaryPath(t *testing.T) {
	tmpDir := stubSystemctl(t)

	if err := InstallTimer("webapp", "daily"); err != nil {
		t.Fatalf("InstallTimer: %v", err)
	}

	svc, err := os.ReadFile(filepath.Join(tmpDir, "fleetdeck-backup-webapp.service"))
	if err != nil {
		t.Fatalf("reading service unit: %v", err)
	}

	// ExecStart should reference the binary path followed by "backup create <project> --type scheduled"
	content := string(svc)
	if !strings.Contains(content, "backup create webapp --type scheduled") {
		t.Error("service unit ExecStart should contain 'backup create webapp --type scheduled'")
	}

	// The ExecStart line should contain an absolute path to the binary
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "ExecStart=") {
			execStart := strings.TrimPrefix(strings.TrimSpace(line), "ExecStart=")
			// The binary path should be absolute (starts with /)
			if !strings.HasPrefix(execStart, "/") {
				t.Errorf("ExecStart binary path should be absolute, got: %s", execStart)
			}
			break
		}
	}
}

func TestInstallTimerGeneratesCorrectUnitSections(t *testing.T) {
	tmpDir := stubSystemctl(t)

	if err := InstallTimer("api-server", "Mon *-*-* 04:00:00"); err != nil {
		t.Fatalf("InstallTimer: %v", err)
	}

	svc, _ := os.ReadFile(filepath.Join(tmpDir, "fleetdeck-backup-api-server.service"))
	tmr, _ := os.ReadFile(filepath.Join(tmpDir, "fleetdeck-backup-api-server.timer"))

	svcContent := string(svc)
	tmrContent := string(tmr)

	// Service unit should have exactly [Unit] and [Service] sections (no [Install])
	if strings.Contains(svcContent, "[Install]") {
		t.Error("service unit should not contain [Install] section (oneshot services don't need it)")
	}

	// Timer unit should have [Unit], [Timer], and [Install]
	for _, section := range []string{"[Unit]", "[Timer]", "[Install]"} {
		if !strings.Contains(tmrContent, section) {
			t.Errorf("timer unit missing section %s", section)
		}
	}

	// Timer should have the custom schedule
	if !strings.Contains(tmrContent, "OnCalendar=Mon *-*-* 04:00:00") {
		t.Error("timer should contain custom schedule")
	}
}

// ---------------------------------------------------------------------------
// Schedule parsing: various cron/calendar expressions
// ---------------------------------------------------------------------------

func TestInstallTimerWithHourlySchedule(t *testing.T) {
	tmpDir := stubSystemctl(t)

	if err := InstallTimer("proj", "hourly"); err != nil {
		t.Fatalf("InstallTimer: %v", err)
	}

	tmr, _ := os.ReadFile(filepath.Join(tmpDir, "fleetdeck-backup-proj.timer"))
	if !strings.Contains(string(tmr), "OnCalendar=hourly") {
		t.Error("timer should contain OnCalendar=hourly")
	}
}

func TestInstallTimerWithMonthlySchedule(t *testing.T) {
	tmpDir := stubSystemctl(t)

	if err := InstallTimer("proj", "monthly"); err != nil {
		t.Fatalf("InstallTimer: %v", err)
	}

	tmr, _ := os.ReadFile(filepath.Join(tmpDir, "fleetdeck-backup-proj.timer"))
	if !strings.Contains(string(tmr), "OnCalendar=monthly") {
		t.Error("timer should contain OnCalendar=monthly")
	}
}

func TestInstallTimerWithComplexCalendarExpression(t *testing.T) {
	tmpDir := stubSystemctl(t)

	schedule := "Sat *-*-1..7 18:00:00"
	if err := InstallTimer("proj", schedule); err != nil {
		t.Fatalf("InstallTimer: %v", err)
	}

	tmr, _ := os.ReadFile(filepath.Join(tmpDir, "fleetdeck-backup-proj.timer"))
	if !strings.Contains(string(tmr), "OnCalendar="+schedule) {
		t.Errorf("timer should contain OnCalendar=%s", schedule)
	}
}

func TestInstallTimerWithMultipleTimesPerDay(t *testing.T) {
	tmpDir := stubSystemctl(t)

	// Systemd supports time ranges like this
	schedule := "*-*-* 06,12,18:00:00"
	if err := InstallTimer("proj", schedule); err != nil {
		t.Fatalf("InstallTimer: %v", err)
	}

	tmr, _ := os.ReadFile(filepath.Join(tmpDir, "fleetdeck-backup-proj.timer"))
	if !strings.Contains(string(tmr), "OnCalendar="+schedule) {
		t.Errorf("timer should contain OnCalendar=%s", schedule)
	}
}

// ---------------------------------------------------------------------------
// Timer status: parse systemctl output edge cases
// ---------------------------------------------------------------------------

func TestGetTimerStatusFailedState(t *testing.T) {
	tmpDir := stubSystemctl(t)

	tmrContent := "[Timer]\nOnCalendar=daily\nPersistent=true\n"
	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-proj.timer"), []byte(tmrContent), 0644)

	systemctlProperty = func(unit, property string) string {
		if property == "ActiveState" {
			return "failed"
		}
		return ""
	}

	status, err := GetTimerStatus("proj")
	if err != nil {
		t.Fatalf("GetTimerStatus: %v", err)
	}

	if status.Active {
		t.Error("expected Active=false for 'failed' state")
	}
}

func TestGetTimerStatusDeadState(t *testing.T) {
	tmpDir := stubSystemctl(t)

	tmrContent := "[Timer]\nOnCalendar=weekly\n"
	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-proj.timer"), []byte(tmrContent), 0644)

	systemctlProperty = func(unit, property string) string {
		if property == "ActiveState" {
			return "dead"
		}
		return ""
	}

	status, err := GetTimerStatus("proj")
	if err != nil {
		t.Fatal(err)
	}
	if status.Active {
		t.Error("expected Active=false for 'dead' state")
	}
}

func TestGetTimerStatusNATimestamps(t *testing.T) {
	tmpDir := stubSystemctl(t)

	tmrContent := "[Timer]\nOnCalendar=daily\n"
	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-proj.timer"), []byte(tmrContent), 0644)

	systemctlProperty = func(unit, property string) string {
		switch property {
		case "ActiveState":
			return "active"
		case "NextElapseUSecRealtime":
			return "n/a"
		case "LastTriggerUSec":
			return "0"
		}
		return ""
	}

	status, err := GetTimerStatus("proj")
	if err != nil {
		t.Fatal(err)
	}

	if status.NextRun != "n/a" {
		t.Errorf("NextRun = %q, want 'n/a'", status.NextRun)
	}
	if status.LastRun != "n/a" {
		t.Errorf("LastRun = %q, want 'n/a'", status.LastRun)
	}
}

func TestGetTimerStatusUnknownTimestampFormat(t *testing.T) {
	tmpDir := stubSystemctl(t)

	tmrContent := "[Timer]\nOnCalendar=daily\n"
	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-proj.timer"), []byte(tmrContent), 0644)

	systemctlProperty = func(unit, property string) string {
		switch property {
		case "ActiveState":
			return "active"
		case "NextElapseUSecRealtime":
			return "some-unknown-format-123"
		case "LastTriggerUSec":
			return "another unknown"
		}
		return ""
	}

	status, err := GetTimerStatus("proj")
	if err != nil {
		t.Fatal(err)
	}

	// Unknown formats should be returned as-is
	if status.NextRun != "some-unknown-format-123" {
		t.Errorf("NextRun = %q, expected passthrough of unknown format", status.NextRun)
	}
	if status.LastRun != "another unknown" {
		t.Errorf("LastRun = %q, expected passthrough of unknown format", status.LastRun)
	}
}

// ---------------------------------------------------------------------------
// formatSystemdTimestamp additional edge cases
// ---------------------------------------------------------------------------

func TestFormatSystemdTimestampVariousDays(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Tuesday", "Tue 2025-06-10 14:30:00 UTC", "2025-06-10 14:30:00"},
		{"Wednesday", "Wed 2025-12-31 23:59:59 UTC", "2025-12-31 23:59:59"},
		{"Thursday", "Thu 2025-03-15 00:00:00 EST", "2025-03-15 00:00:00"},
		{"Friday", "Fri 2025-07-04 12:00:00 PST", "2025-07-04 12:00:00"},
		{"Saturday", "Sat 2025-01-01 01:00:00 UTC", "2025-01-01 01:00:00"},
		{"Sunday", "Sun 2025-11-30 06:30:00 UTC", "2025-11-30 06:30:00"},
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

func TestFormatSystemdTimestampWithoutTimezone(t *testing.T) {
	got := formatSystemdTimestamp("Wed 2025-06-15 08:45:30")
	if got != "2025-06-15 08:45:30" {
		t.Errorf("got %q, want '2025-06-15 08:45:30'", got)
	}
}

func TestFormatSystemdTimestampRFC3339Variants(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"2025-06-15T08:45:30Z", "2025-06-15 08:45:30"},
		{"2025-01-01T00:00:00Z", "2025-01-01 00:00:00"},
	}
	for _, tc := range tests {
		got := formatSystemdTimestamp(tc.input)
		if got != tc.expected {
			t.Errorf("formatSystemdTimestamp(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// readOnCalendar additional edge cases
// ---------------------------------------------------------------------------

func TestReadOnCalendarWithCommentsInFile(t *testing.T) {
	tmpDir := t.TempDir()

	content := `[Unit]
Description=Test timer
# This is a comment

[Timer]
# OnCalendar=commented-out
OnCalendar=*-*-* 03:00:00
Persistent=true

[Install]
WantedBy=timers.target
`
	path := filepath.Join(tmpDir, "test.timer")
	os.WriteFile(path, []byte(content), 0644)

	got := readOnCalendar(path)
	if got != "*-*-* 03:00:00" {
		t.Errorf("readOnCalendar() = %q, want '*-*-* 03:00:00'", got)
	}
}

func TestReadOnCalendarWithTabIndentation(t *testing.T) {
	tmpDir := t.TempDir()

	// OnCalendar with tab prefix -- TrimSpace should handle it
	content := "[Timer]\n\tOnCalendar=daily\n"
	path := filepath.Join(tmpDir, "tab.timer")
	os.WriteFile(path, []byte(content), 0644)

	got := readOnCalendar(path)
	if got != "daily" {
		t.Errorf("readOnCalendar() = %q, want 'daily'", got)
	}
}

// ---------------------------------------------------------------------------
// Concurrent InstallTimer calls for different projects
// ---------------------------------------------------------------------------

func TestInstallTimerConcurrentDifferentProjects(t *testing.T) {
	tmpDir := stubSystemctl(t)

	// Use a mutex-protected slice to capture daemon-reload calls
	var mu sync.Mutex
	var reloadCount int
	systemctlRun = func(args ...string) error {
		if len(args) > 0 && args[0] == "daemon-reload" {
			mu.Lock()
			reloadCount++
			mu.Unlock()
		}
		return nil
	}

	var wg sync.WaitGroup
	projects := []string{"proj-a1", "proj-b2", "proj-c3", "proj-d4", "proj-e5"}
	errs := make([]error, len(projects))

	for i, name := range projects {
		wg.Add(1)
		go func(idx int, n string) {
			defer wg.Done()
			errs[idx] = InstallTimer(n, "daily")
		}(i, name)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("InstallTimer(%s) failed: %v", projects[i], err)
		}
	}

	// Verify all unit files were created
	for _, name := range projects {
		svcPath := filepath.Join(tmpDir, "fleetdeck-backup-"+name+".service")
		tmrPath := filepath.Join(tmpDir, "fleetdeck-backup-"+name+".timer")
		if _, err := os.Stat(svcPath); err != nil {
			t.Errorf("missing service file for %s: %v", name, err)
		}
		if _, err := os.Stat(tmrPath); err != nil {
			t.Errorf("missing timer file for %s: %v", name, err)
		}
	}

	mu.Lock()
	if reloadCount != len(projects) {
		t.Errorf("expected %d daemon-reload calls, got %d", len(projects), reloadCount)
	}
	mu.Unlock()
}

// ---------------------------------------------------------------------------
// ListTimers with projects that have different properties
// ---------------------------------------------------------------------------

func TestListTimersWithMixedStates(t *testing.T) {
	tmpDir := stubSystemctl(t)

	// Create timers with different schedules
	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-active-proj.timer"),
		[]byte("[Timer]\nOnCalendar=daily\n"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-inactive-proj.timer"),
		[]byte("[Timer]\nOnCalendar=weekly\n"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-waiting-proj.timer"),
		[]byte("[Timer]\nOnCalendar=*-*-* 02:00:00\n"), 0644)

	systemctlProperty = func(unit, property string) string {
		if property == "ActiveState" {
			if strings.Contains(unit, "waiting-proj") {
				return "waiting"
			}
			if strings.Contains(unit, "inactive-proj") {
				return "inactive"
			}
			if strings.Contains(unit, "active-proj") {
				return "active"
			}
		}
		return ""
	}

	timers, err := ListTimers()
	if err != nil {
		t.Fatal(err)
	}
	if len(timers) != 3 {
		t.Fatalf("expected 3 timers, got %d", len(timers))
	}

	byName := make(map[string]TimerStatus)
	for _, ts := range timers {
		byName[ts.ProjectName] = ts
	}

	if !byName["active-proj"].Active {
		t.Error("active-proj should be active")
	}
	if byName["inactive-proj"].Active {
		t.Error("inactive-proj should not be active")
	}
	if !byName["waiting-proj"].Active {
		t.Error("waiting-proj should be active (waiting counts as active)")
	}
	if byName["waiting-proj"].Schedule != "*-*-* 02:00:00" {
		t.Errorf("waiting-proj schedule = %q, want '*-*-* 02:00:00'", byName["waiting-proj"].Schedule)
	}
}

func TestListTimersWithTimerMissingOnCalendar(t *testing.T) {
	tmpDir := stubSystemctl(t)

	// Timer file without OnCalendar line
	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-no-cal.timer"),
		[]byte("[Timer]\nPersistent=true\n"), 0644)

	timers, err := ListTimers()
	if err != nil {
		t.Fatal(err)
	}
	if len(timers) != 1 {
		t.Fatalf("expected 1 timer, got %d", len(timers))
	}
	if timers[0].Schedule != "" {
		t.Errorf("Schedule should be empty for missing OnCalendar, got %q", timers[0].Schedule)
	}
}

// ---------------------------------------------------------------------------
// Multiple install + list round trip
// ---------------------------------------------------------------------------

func TestInstallMultipleThenList(t *testing.T) {
	tmpDir := stubSystemctl(t)
	_ = tmpDir

	projects := []struct {
		name     string
		schedule string
	}{
		{"web-app", "daily"},
		{"api-svc", "weekly"},
		{"db-back", "*-*-* 01:00:00"},
	}

	for _, p := range projects {
		if err := InstallTimer(p.name, p.schedule); err != nil {
			t.Fatalf("InstallTimer(%s): %v", p.name, err)
		}
	}

	timers, err := ListTimers()
	if err != nil {
		t.Fatal(err)
	}
	if len(timers) != 3 {
		t.Fatalf("expected 3 timers, got %d", len(timers))
	}

	byName := make(map[string]TimerStatus)
	for _, ts := range timers {
		byName[ts.ProjectName] = ts
	}

	for _, p := range projects {
		ts, ok := byName[p.name]
		if !ok {
			t.Errorf("missing timer for %s", p.name)
			continue
		}
		if ts.Schedule != p.schedule {
			t.Errorf("%s: Schedule = %q, want %q", p.name, ts.Schedule, p.schedule)
		}
	}
}

// ---------------------------------------------------------------------------
// RemoveTimer edge cases
// ---------------------------------------------------------------------------

func TestRemoveTimerStopAndDisableErrorsIgnored(t *testing.T) {
	tmpDir := stubSystemctl(t)

	// Create files
	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-proj.service"), []byte("svc"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-proj.timer"), []byte("tmr"), 0644)

	callIdx := 0
	systemctlRun = func(args ...string) error {
		callIdx++
		// Stop and disable fail, but daemon-reload succeeds
		if callIdx <= 2 {
			return fmt.Errorf("mock stop/disable failure")
		}
		return nil
	}

	// Should succeed despite stop and disable failures
	err := RemoveTimer("proj")
	if err != nil {
		t.Errorf("RemoveTimer should succeed even if stop/disable fail: %v", err)
	}

	// Files should still be removed
	if _, err := os.Stat(filepath.Join(tmpDir, "fleetdeck-backup-proj.service")); !os.IsNotExist(err) {
		t.Error("service file should be removed")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "fleetdeck-backup-proj.timer")); !os.IsNotExist(err) {
		t.Error("timer file should be removed")
	}
}

func TestRemoveTimerOnlyServiceExists(t *testing.T) {
	tmpDir := stubSystemctl(t)

	// Only service file exists, timer already gone
	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-partial.service"), []byte("svc"), 0644)

	err := RemoveTimer("partial")
	if err != nil {
		t.Errorf("RemoveTimer with missing timer file should not error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "fleetdeck-backup-partial.service")); !os.IsNotExist(err) {
		t.Error("service file should be removed")
	}
}

func TestRemoveTimerOnlyTimerExists(t *testing.T) {
	tmpDir := stubSystemctl(t)

	// Only timer file exists, service already gone
	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-partial2.timer"), []byte("tmr"), 0644)

	err := RemoveTimer("partial2")
	if err != nil {
		t.Errorf("RemoveTimer with missing service file should not error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "fleetdeck-backup-partial2.timer")); !os.IsNotExist(err) {
		t.Error("timer file should be removed")
	}
}

// ---------------------------------------------------------------------------
// removeIfExists permission error
// ---------------------------------------------------------------------------

func TestRemoveIfExistsPermissionError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test when running as root")
	}

	tmpDir := t.TempDir()
	protectedDir := filepath.Join(tmpDir, "protected")
	os.Mkdir(protectedDir, 0755)

	filePath := filepath.Join(protectedDir, "test-file")
	os.WriteFile(filePath, []byte("data"), 0644)

	// Make the directory read-only so removal fails
	os.Chmod(protectedDir, 0555)
	t.Cleanup(func() {
		os.Chmod(protectedDir, 0755)
	})

	err := removeIfExists(filePath)
	if err == nil {
		t.Error("expected error when removing file in read-only directory")
	}
}

// ---------------------------------------------------------------------------
// EnableTimer / DisableTimer with specific unit names
// ---------------------------------------------------------------------------

func TestEnableTimerUsesCorrectUnitName(t *testing.T) {
	stubSystemctl(t)

	var captured string
	systemctlRun = func(args ...string) error {
		captured = strings.Join(args, " ")
		return nil
	}

	if err := EnableTimer("my-web-app"); err != nil {
		t.Fatal(err)
	}

	expected := "enable --now fleetdeck-backup-my-web-app.timer"
	if captured != expected {
		t.Errorf("systemctl call = %q, want %q", captured, expected)
	}
}

func TestDisableTimerUsesCorrectUnitName(t *testing.T) {
	stubSystemctl(t)

	var captured string
	systemctlRun = func(args ...string) error {
		captured = strings.Join(args, " ")
		return nil
	}

	if err := DisableTimer("my-web-app"); err != nil {
		t.Fatal(err)
	}

	expected := "disable --now fleetdeck-backup-my-web-app.timer"
	if captured != expected {
		t.Errorf("systemctl call = %q, want %q", captured, expected)
	}
}

// ---------------------------------------------------------------------------
// GetTimerStatus reads schedule from file correctly
// ---------------------------------------------------------------------------

func TestGetTimerStatusReadsScheduleFromFile(t *testing.T) {
	tmpDir := stubSystemctl(t)

	// Write a timer with a specific complex schedule
	schedule := "Mon,Thu *-*-* 06:00:00"
	tmrContent := fmt.Sprintf("[Unit]\nDescription=Test\n\n[Timer]\nOnCalendar=%s\nPersistent=true\n\n[Install]\nWantedBy=timers.target\n", schedule)
	os.WriteFile(filepath.Join(tmpDir, "fleetdeck-backup-complex.timer"), []byte(tmrContent), 0644)

	status, err := GetTimerStatus("complex")
	if err != nil {
		t.Fatal(err)
	}

	if status.Schedule != schedule {
		t.Errorf("Schedule = %q, want %q", status.Schedule, schedule)
	}
	if status.ProjectName != "complex" {
		t.Errorf("ProjectName = %q, want 'complex'", status.ProjectName)
	}
}

// ---------------------------------------------------------------------------
// Install timer with project name containing numbers
// ---------------------------------------------------------------------------

func TestInstallTimerProjectNameWithNumbers(t *testing.T) {
	tmpDir := stubSystemctl(t)

	if err := InstallTimer("app-v2-prod", "daily"); err != nil {
		t.Fatalf("InstallTimer: %v", err)
	}

	svcPath := filepath.Join(tmpDir, "fleetdeck-backup-app-v2-prod.service")
	tmrPath := filepath.Join(tmpDir, "fleetdeck-backup-app-v2-prod.timer")

	svc, err := os.ReadFile(svcPath)
	if err != nil {
		t.Fatalf("reading service unit: %v", err)
	}
	if !strings.Contains(string(svc), "Description=FleetDeck scheduled backup for app-v2-prod") {
		t.Error("service description should contain project name with numbers")
	}

	tmr, err := os.ReadFile(tmrPath)
	if err != nil {
		t.Fatalf("reading timer unit: %v", err)
	}
	if !strings.Contains(string(tmr), "Description=FleetDeck backup timer for app-v2-prod") {
		t.Error("timer description should contain project name with numbers")
	}
}

// ---------------------------------------------------------------------------
// Unit name helper consistency
// ---------------------------------------------------------------------------

func TestUnitNameConsistencyWithPaths(t *testing.T) {
	tmpDir := stubSystemctl(t)

	name := "consistency-test"

	// Verify servicePath uses serviceUnitName
	sPath := servicePath(name)
	expectedSvcPath := filepath.Join(tmpDir, serviceUnitName(name))
	if sPath != expectedSvcPath {
		t.Errorf("servicePath(%q) = %q, want %q", name, sPath, expectedSvcPath)
	}

	// Verify timerPath uses timerUnitName
	tPath := timerPath(name)
	expectedTmrPath := filepath.Join(tmpDir, timerUnitName(name))
	if tPath != expectedTmrPath {
		t.Errorf("timerPath(%q) = %q, want %q", name, tPath, expectedTmrPath)
	}
}

// ---------------------------------------------------------------------------
// Install then GetTimerStatus integration
// ---------------------------------------------------------------------------

func TestInstallThenGetStatus(t *testing.T) {
	stubSystemctl(t)

	if err := InstallTimer("status-check", "*-*-* 22:00:00"); err != nil {
		t.Fatal(err)
	}

	systemctlProperty = func(unit, property string) string {
		if property == "ActiveState" {
			return "active"
		}
		if property == "NextElapseUSecRealtime" {
			return "Sat 2025-06-14 22:00:00 UTC"
		}
		return ""
	}

	status, err := GetTimerStatus("status-check")
	if err != nil {
		t.Fatal(err)
	}

	if status.ProjectName != "status-check" {
		t.Errorf("ProjectName = %q", status.ProjectName)
	}
	if status.Schedule != "*-*-* 22:00:00" {
		t.Errorf("Schedule = %q, want '*-*-* 22:00:00'", status.Schedule)
	}
	if !status.Active {
		t.Error("expected Active=true")
	}
	if status.NextRun != "2025-06-14 22:00:00" {
		t.Errorf("NextRun = %q", status.NextRun)
	}
}

// ---------------------------------------------------------------------------
// Install, remove, verify GetTimerStatus fails
// ---------------------------------------------------------------------------

func TestInstallRemoveThenGetStatusFails(t *testing.T) {
	stubSystemctl(t)

	if err := InstallTimer("temp-proj", "daily"); err != nil {
		t.Fatal(err)
	}

	if err := RemoveTimer("temp-proj"); err != nil {
		t.Fatal(err)
	}

	_, err := GetTimerStatus("temp-proj")
	if err == nil {
		t.Error("GetTimerStatus should fail after timer is removed")
	}
	if !strings.Contains(err.Error(), "no backup timer found") {
		t.Errorf("error should mention no timer found, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ListTimers with empty directory (no glob matches)
// ---------------------------------------------------------------------------

func TestListTimersEmptyDirectory(t *testing.T) {
	stubSystemctl(t)
	// tmpDir is empty, no timer files

	timers, err := ListTimers()
	if err != nil {
		t.Fatal(err)
	}
	if timers != nil {
		t.Errorf("expected nil for empty directory, got %v", timers)
	}
}

// ---------------------------------------------------------------------------
// Verify timer unit file permissions
// ---------------------------------------------------------------------------

func TestInstallTimerFilePermissions(t *testing.T) {
	tmpDir := stubSystemctl(t)

	if err := InstallTimer("perm-test", "daily"); err != nil {
		t.Fatal(err)
	}

	svcPath := filepath.Join(tmpDir, "fleetdeck-backup-perm-test.service")
	tmrPath := filepath.Join(tmpDir, "fleetdeck-backup-perm-test.timer")

	svcInfo, err := os.Stat(svcPath)
	if err != nil {
		t.Fatal(err)
	}
	tmrInfo, err := os.Stat(tmrPath)
	if err != nil {
		t.Fatal(err)
	}

	// Files should be written with 0644 permissions
	if svcInfo.Mode().Perm() != 0644 {
		t.Errorf("service file permissions = %o, want 0644", svcInfo.Mode().Perm())
	}
	if tmrInfo.Mode().Perm() != 0644 {
		t.Errorf("timer file permissions = %o, want 0644", tmrInfo.Mode().Perm())
	}
}
