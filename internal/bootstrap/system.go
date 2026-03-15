package bootstrap

import "fmt"

// prepareSystem updates packages and installs essential tools.
func prepareSystem(runner CommandRunner) error {
	commands := []string{
		"export DEBIAN_FRONTEND=noninteractive && apt-get update -qq",
		"export DEBIAN_FRONTEND=noninteractive && apt-get upgrade -y -qq",
		"export DEBIAN_FRONTEND=noninteractive && apt-get install -y -qq curl git htop unzip fail2ban",
	}
	for _, cmd := range commands {
		if _, err := runner.Run(cmd); err != nil {
			return fmt.Errorf("running %q: %w", cmd, err)
		}
	}
	return nil
}

// configureSwap creates a swap file if one does not already exist.
func configureSwap(runner CommandRunner, sizeGB int) error {
	// Check if swap is already active.
	out, _ := runner.Run("swapon --show --noheadings")
	if out != "" {
		return nil // swap already configured
	}

	commands := []string{
		fmt.Sprintf("fallocate -l %dG /swapfile", sizeGB),
		"chmod 600 /swapfile",
		"mkswap /swapfile",
		"swapon /swapfile",
	}
	for _, cmd := range commands {
		if _, err := runner.Run(cmd); err != nil {
			return fmt.Errorf("running %q: %w", cmd, err)
		}
	}

	// Persist across reboots if not already in fstab.
	if _, err := runner.Run("grep -q /swapfile /etc/fstab || echo '/swapfile none swap sw 0 0' >> /etc/fstab"); err != nil {
		return fmt.Errorf("persisting swap in fstab: %w", err)
	}

	return nil
}

// setTimezone sets the system timezone to UTC.
func setTimezone(runner CommandRunner) error {
	if _, err := runner.Run("timedatectl set-timezone UTC"); err != nil {
		return fmt.Errorf("setting timezone: %w", err)
	}
	return nil
}

// configureSSH hardens the SSH daemon by disabling password authentication
// and root login.
func configureSSH(runner CommandRunner) error {
	sedCmds := []string{
		"sed -i 's/^#\\?PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config",
		"sed -i 's/^#\\?PermitRootLogin.*/PermitRootLogin no/' /etc/ssh/sshd_config",
	}
	for _, cmd := range sedCmds {
		if _, err := runner.Run(cmd); err != nil {
			return fmt.Errorf("running %q: %w", cmd, err)
		}
	}

	// Restart SSH daemon. Try systemctl first; fall back to service command.
	if _, err := runner.Run("systemctl restart sshd || service ssh restart"); err != nil {
		return fmt.Errorf("restarting sshd: %w", err)
	}

	return nil
}
