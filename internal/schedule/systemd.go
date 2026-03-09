package schedule

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	unitDir    = "/etc/systemd/system"
	unitPrefix = "fleetdeck-backup-"
)

// TimerStatus represents the state of a systemd backup timer.
type TimerStatus struct {
	ProjectName string
	Schedule    string
	NextRun     string
	LastRun     string
	Active      bool
}

func serviceUnitName(projectName string) string {
	return unitPrefix + projectName + ".service"
}

func timerUnitName(projectName string) string {
	return unitPrefix + projectName + ".timer"
}

func servicePath(projectName string) string {
	return filepath.Join(unitDir, serviceUnitName(projectName))
}

func timerPath(projectName string) string {
	return filepath.Join(unitDir, timerUnitName(projectName))
}

// FleetDeckBinary returns the absolute path of the currently running fleetdeck binary.
func fleetdeckBinary() string {
	exe, err := os.Executable()
	if err != nil {
		return "fleetdeck"
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe
	}
	return resolved
}

// InstallTimer generates and writes a systemd service and timer unit for
// scheduled backups of the given project. The schedule string is passed
// directly to OnCalendar= (e.g. "daily", "weekly", "*-*-* 02:00:00").
func InstallTimer(projectName, schedule string) error {
	binary := fleetdeckBinary()

	serviceContent := fmt.Sprintf(`[Unit]
Description=FleetDeck scheduled backup for %s
After=network.target

[Service]
Type=oneshot
ExecStart=%s backup create %s --type scheduled
StandardOutput=journal
StandardError=journal
`, projectName, binary, projectName)

	timerContent := fmt.Sprintf(`[Unit]
Description=FleetDeck backup timer for %s

[Timer]
OnCalendar=%s
Persistent=true
RandomizedDelaySec=300

[Install]
WantedBy=timers.target
`, projectName, schedule)

	if err := os.WriteFile(servicePath(projectName), []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("writing service unit: %w", err)
	}

	if err := os.WriteFile(timerPath(projectName), []byte(timerContent), 0644); err != nil {
		// Clean up the service file if timer write fails
		os.Remove(servicePath(projectName))
		return fmt.Errorf("writing timer unit: %w", err)
	}

	// Reload systemd to pick up new units
	if err := systemctlRun("daemon-reload"); err != nil {
		return fmt.Errorf("reloading systemd: %w", err)
	}

	return nil
}

// RemoveTimer stops the timer, disables it, and removes the unit files.
func RemoveTimer(projectName string) error {
	timerUnit := timerUnitName(projectName)

	// Stop and disable (ignore errors — unit may not be active)
	_ = systemctlRun("stop", timerUnit)
	_ = systemctlRun("disable", timerUnit)

	// Remove unit files
	svcErr := removeIfExists(servicePath(projectName))
	tmrErr := removeIfExists(timerPath(projectName))

	if svcErr != nil {
		return fmt.Errorf("removing service unit: %w", svcErr)
	}
	if tmrErr != nil {
		return fmt.Errorf("removing timer unit: %w", tmrErr)
	}

	// Reload systemd
	if err := systemctlRun("daemon-reload"); err != nil {
		return fmt.Errorf("reloading systemd: %w", err)
	}

	return nil
}

// EnableTimer enables and starts the timer immediately.
func EnableTimer(projectName string) error {
	timerUnit := timerUnitName(projectName)
	if err := systemctlRun("enable", "--now", timerUnit); err != nil {
		return fmt.Errorf("enabling timer: %w", err)
	}
	return nil
}

// DisableTimer disables and stops the timer.
func DisableTimer(projectName string) error {
	timerUnit := timerUnitName(projectName)
	if err := systemctlRun("disable", "--now", timerUnit); err != nil {
		return fmt.Errorf("disabling timer: %w", err)
	}
	return nil
}

// ListTimers returns the status of all fleetdeck backup timers by parsing
// systemctl list-timers output and checking individual unit properties.
func ListTimers() ([]TimerStatus, error) {
	// Find all fleetdeck backup timer unit files on disk
	pattern := filepath.Join(unitDir, unitPrefix+"*.timer")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("globbing timer units: %w", err)
	}

	if len(matches) == 0 {
		return nil, nil
	}

	var timers []TimerStatus
	for _, match := range matches {
		base := filepath.Base(match)
		// Extract project name from "fleetdeck-backup-<project>.timer"
		name := strings.TrimPrefix(base, unitPrefix)
		name = strings.TrimSuffix(name, ".timer")

		status := TimerStatus{
			ProjectName: name,
		}

		// Read schedule from the timer unit file
		status.Schedule = readOnCalendar(match)

		// Query systemctl for active state
		timerUnit := timerUnitName(name)
		activeState := systemctlProperty(timerUnit, "ActiveState")
		status.Active = activeState == "active" || activeState == "waiting"

		// Query next and last trigger times
		if nextMono := systemctlProperty(timerUnit, "NextElapseUSecRealtime"); nextMono != "" {
			status.NextRun = formatSystemdTimestamp(nextMono)
		}
		if lastTrigger := systemctlProperty(timerUnit, "LastTriggerUSec"); lastTrigger != "" {
			status.LastRun = formatSystemdTimestamp(lastTrigger)
		}

		timers = append(timers, status)
	}

	return timers, nil
}

// GetTimerStatus returns the status of a single project's backup timer.
func GetTimerStatus(projectName string) (*TimerStatus, error) {
	timerFile := timerPath(projectName)
	if _, err := os.Stat(timerFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("no backup timer found for project %s", projectName)
	}

	status := &TimerStatus{
		ProjectName: projectName,
		Schedule:    readOnCalendar(timerFile),
	}

	timerUnit := timerUnitName(projectName)
	activeState := systemctlProperty(timerUnit, "ActiveState")
	status.Active = activeState == "active" || activeState == "waiting"

	if nextMono := systemctlProperty(timerUnit, "NextElapseUSecRealtime"); nextMono != "" {
		status.NextRun = formatSystemdTimestamp(nextMono)
	}
	if lastTrigger := systemctlProperty(timerUnit, "LastTriggerUSec"); lastTrigger != "" {
		status.LastRun = formatSystemdTimestamp(lastTrigger)
	}

	return status, nil
}

// systemctlRun executes a systemctl command with the given arguments.
func systemctlRun(args ...string) error {
	cmd := exec.Command("systemctl", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return nil
}

// systemctlProperty queries a systemd unit property via systemctl show.
func systemctlProperty(unit, property string) string {
	cmd := exec.Command("systemctl", "show", unit, "--property="+property, "--value")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// readOnCalendar parses the OnCalendar= value from a timer unit file.
func readOnCalendar(timerFile string) string {
	f, err := os.Open(timerFile)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "OnCalendar=") {
			return strings.TrimPrefix(line, "OnCalendar=")
		}
	}
	return ""
}

// formatSystemdTimestamp attempts to convert a systemd timestamp string
// into a more readable format. If parsing fails, it returns the raw string.
func formatSystemdTimestamp(raw string) string {
	if raw == "" || raw == "n/a" || raw == "0" {
		return "n/a"
	}

	// systemctl show returns timestamps like "Mon 2025-01-06 02:00:00 UTC"
	// Try common systemd timestamp formats
	formats := []string{
		"Mon 2006-01-02 15:04:05 MST",
		"Mon 2006-01-02 15:04:05",
		time.RFC3339,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, raw); err == nil {
			return t.Format("2006-01-02 15:04:05")
		}
	}

	return raw
}

// removeIfExists removes a file if it exists, returning nil if it doesn't.
func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
