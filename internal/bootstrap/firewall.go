package bootstrap

import (
	"fmt"
	"strconv"
)

// configureFirewall sets up UFW to allow SSH, HTTP, and HTTPS while denying
// all other inbound traffic. Idempotent: re-running applies the same rules.
func configureFirewall(runner CommandRunner, sshPort string) error {
	port, err := validatedSSHPort(sshPort)
	if err != nil {
		return err
	}
	// Ensure UFW is installed.
	if _, err := runner.Run("export DEBIAN_FRONTEND=noninteractive && apt-get install -y -qq ufw"); err != nil {
		return fmt.Errorf("installing ufw: %w", err)
	}

	rules := []string{
		"ufw default deny incoming",
		"ufw default allow outgoing",
		fmt.Sprintf("ufw allow %s/tcp", port), // Preserve the actual SSH transport port.
		"ufw allow 80/tcp",                    // HTTP
		"ufw allow 443/tcp",                   // HTTPS
	}
	for _, cmd := range rules {
		if _, err := runner.Run(cmd); err != nil {
			return fmt.Errorf("running %q: %w", cmd, err)
		}
	}

	// Enable UFW non-interactively.
	if _, err := runner.Run("echo 'y' | ufw enable"); err != nil {
		return fmt.Errorf("enabling ufw: %w", err)
	}

	return nil
}

// Validate before any remote mutation; never interpolate an unchecked port.
func validatedSSHPort(port string) (string, error) {
	if port == "" {
		return "22", nil
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
		return "", fmt.Errorf("invalid SSH port %q: expected 1–65535", port)
	}
	return port, nil
}
