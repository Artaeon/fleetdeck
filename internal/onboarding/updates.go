package onboarding

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
)

//go:embed updates.py
var updatesScript string
var updateWindow = regexp.MustCompile(`^(?:[01][0-9]|2[0-3]):[0-5][0-9]$`)

// ConfigureUpdates manages only the explicitly owned APT policy and timer
// override. Package installation itself belongs to unattended-upgrades.
func ConfigureUpdates(r Runner, mode, window string) (string, error) {
	if mode != "plan" && mode != "enable" && mode != "disable" {
		return "", fmt.Errorf("invalid update operation")
	}
	if !updateWindow.MatchString(window) {
		return "", fmt.Errorf("maintenance time must be HH:MM in UTC")
	}
	args, _ := json.Marshal(map[string]string{"mode": mode, "window": window})
	// JSON is placed in a quoted heredoc; neither value can contain delimiters.
	script := "options = " + string(args) + "\n" + updatesScript
	out, err := r.Run("python3 - <<'FLEETDECK_UPDATES'\n" + script + "\nFLEETDECK_UPDATES\n")
	if err != nil {
		return "", fmt.Errorf("host update configuration failed; previous policy was retained or recovery details are in /var/lib/fleetdeck/host-updates (check privately)")
	}
	var result map[string]any
	if len(out) > 1<<20 || json.Unmarshal([]byte(out), &result) != nil {
		return "", fmt.Errorf("invalid host update report")
	}
	return out, nil
}
