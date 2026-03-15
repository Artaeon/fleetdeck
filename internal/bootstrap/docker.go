package bootstrap

import "fmt"

// installDocker installs Docker Engine and the Compose plugin from the
// official Docker repository. Idempotent: skips if docker is already present.
func installDocker(runner CommandRunner) error {
	// If docker is already installed, nothing to do.
	if _, err := runner.Run("command -v docker"); err == nil {
		return nil
	}

	commands := []string{
		// Install prerequisites
		"export DEBIAN_FRONTEND=noninteractive && apt-get install -y -qq ca-certificates curl gnupg",

		// Add Docker GPG key
		"install -m 0755 -d /etc/apt/keyrings",
		"curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc",
		"chmod a+r /etc/apt/keyrings/docker.asc",

		// Add Docker repository
		`echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" > /etc/apt/sources.list.d/docker.list`,

		// Install Docker
		"export DEBIAN_FRONTEND=noninteractive && apt-get update -qq",
		"export DEBIAN_FRONTEND=noninteractive && apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin",

		// Enable and start
		"systemctl enable docker",
		"systemctl start docker",
	}

	for _, cmd := range commands {
		if _, err := runner.Run(cmd); err != nil {
			return fmt.Errorf("running %q: %w", cmd, err)
		}
	}

	return nil
}

// verifyDocker checks that docker and docker compose are functional.
func verifyDocker(runner CommandRunner) error {
	if _, err := runner.Run("docker version"); err != nil {
		return fmt.Errorf("docker version check failed: %w", err)
	}
	if _, err := runner.Run("docker compose version"); err != nil {
		return fmt.Errorf("docker compose version check failed: %w", err)
	}
	return nil
}
