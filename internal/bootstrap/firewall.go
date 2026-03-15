package bootstrap

import "fmt"

// configureFirewall sets up UFW to allow SSH, HTTP, and HTTPS while denying
// all other inbound traffic. Idempotent: re-running applies the same rules.
func configureFirewall(runner CommandRunner) error {
	// Ensure UFW is installed.
	if _, err := runner.Run("export DEBIAN_FRONTEND=noninteractive && apt-get install -y -qq ufw"); err != nil {
		return fmt.Errorf("installing ufw: %w", err)
	}

	rules := []string{
		"ufw default deny incoming",
		"ufw default allow outgoing",
		"ufw allow 22/tcp",  // SSH
		"ufw allow 80/tcp",  // HTTP
		"ufw allow 443/tcp", // HTTPS
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
