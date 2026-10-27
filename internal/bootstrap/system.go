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

// configureSSH applies a validated, reversible key-only policy. Confirmation
// must follow a new authenticated connection, never an existing SSH session.
func configureSSH(runner CommandRunner) error {
	_, err := runner.Run(sshTrialScript)
	if err != nil {
		return fmt.Errorf("applying SSH policy: %w", err)
	}
	return nil
}

func ConfirmSSH(runner CommandRunner) error {
	_, err := runner.Run(`set -eu
 exec 9>/run/fleetdeck-ssh.lock
 flock -x 9
 test -f /run/fleetdeck-ssh-trial/previous
 systemctl is-active --quiet fleetdeck-ssh-rollback.timer
 systemctl stop fleetdeck-ssh-rollback.timer
 rm -rf /run/fleetdeck-ssh-trial
`)
	return err
}

func RollbackSSH(runner CommandRunner) error {
	_, err := runner.Run(`if test -f /run/fleetdeck-ssh-trial/rollback.sh; then sh /run/fleetdeck-ssh-trial/rollback.sh; fi`)
	return err
}

const sshTrialScript = `set -eu
exec 9>/run/fleetdeck-ssh.lock
flock -x 9
trial=/run/fleetdeck-ssh-trial
# Never overwrite another operator's pending access trial.
mkdir -m 0700 "$trial"
cp -p /etc/ssh/sshd_config "$trial/previous"
cat > "$trial/rollback.sh" <<'ROLLBACK'
#!/bin/sh
set -eu
exec 9>/run/fleetdeck-ssh.lock
flock -x 9
if test -f /run/fleetdeck-ssh-trial/previous; then
 cp -p /run/fleetdeck-ssh-trial/previous /etc/ssh/sshd_config
 /usr/sbin/sshd -t
 systemctl reload ssh.service || systemctl reload sshd.service
 rm -rf /run/fleetdeck-ssh-trial
fi
ROLLBACK
chmod 0700 "$trial/rollback.sh"
trap 'flock -u 9; sh "$trial/rollback.sh"; systemctl stop fleetdeck-ssh-rollback.timer || true' EXIT
# Arm rollback BEFORE modifying active policy.
systemd-run --quiet --collect --unit=fleetdeck-ssh-rollback --on-active=5m --timer-property=AccuracySec=1s /bin/sh "$trial/rollback.sh"
{
 cat <<'POLICY'
# BEGIN FLEETDECK SSH POLICY
PermitRootLogin no
PasswordAuthentication no
KbdInteractiveAuthentication no
PubkeyAuthentication yes
PermitEmptyPasswords no
X11Forwarding no
AllowAgentForwarding no
# END FLEETDECK SSH POLICY
POLICY
 sed '/^# BEGIN FLEETDECK SSH POLICY$/,/^# END FLEETDECK SSH POLICY$/d' "$trial/previous"
} > "$trial/candidate"
chmod 0600 "$trial/candidate"
/usr/sbin/sshd -t -f "$trial/candidate"
/usr/sbin/sshd -T -f "$trial/candidate" -C user=root,host=localhost,addr=127.0.0.1 | grep -qx 'permitrootlogin no'
/usr/sbin/sshd -T -f "$trial/candidate" | grep -qx 'passwordauthentication no'
/usr/sbin/sshd -T -f "$trial/candidate" | grep -qx 'kbdinteractiveauthentication no'
cp "$trial/candidate" /etc/ssh/sshd_config
/usr/sbin/sshd -t
systemctl reload ssh.service || systemctl reload sshd.service
trap - EXIT
`
