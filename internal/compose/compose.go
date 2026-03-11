// Package compose provides helpers for running docker compose commands.
package compose

import (
	"fmt"
	"os/exec"
	"strings"
)

// Validate runs "docker compose config -q" in the given project directory
// to check that the compose file is syntactically valid. It returns nil if
// the configuration is valid, or an error containing the command output if
// validation fails.
func Validate(projectDir string) error {
	cmd := exec.Command("docker", "compose", "config", "-q")
	cmd.Dir = projectDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("compose config validation failed: %s: %w", msg, err)
		}
		return fmt.Errorf("compose config validation failed: %w", err)
	}
	return nil
}
