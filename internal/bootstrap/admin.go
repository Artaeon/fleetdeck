package bootstrap

import (
	"fmt"
	"golang.org/x/crypto/ssh"
	"regexp"
	"strings"
)

var adminName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,30}$`)

func quoteShell(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

// SudoRunner is for a trusted host administrator, not a project role. Docker
// administration and arbitrary Compose deployment already imply root authority.
type SudoRunner struct{ Runner CommandRunner }

func (r SudoRunner) Run(command string) (string, error) {
	return r.Runner.Run("sudo -n sh -c " + quoteShell(command))
}

func PrepareAdmin(runner CommandRunner, name string, publicKey []byte) error {
	if !adminName.MatchString(name) || name == "root" {
		return fmt.Errorf("invalid non-root administrator name")
	}
	key, _, options, rest, err := ssh.ParseAuthorizedKey(publicKey)
	if err != nil || len(options) != 0 || strings.TrimSpace(string(rest)) != "" {
		return fmt.Errorf("supply exactly one SSH public key without authorized_keys options")
	}
	if key.Type() == "ssh-dss" {
		return fmt.Errorf("DSA keys are not supported")
	}
	canonical := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
	script := `set -eu
command -v useradd >/dev/null
command -v visudo >/dev/null || { apt-get update -qq; apt-get install -y -qq sudo; }
install -d -m 0700 /var/lib/fleetdeck/administrators
name=` + quoteShell(name) + `
if id "$name" >/dev/null 2>&1; then
 test -f "/var/lib/fleetdeck/administrators/$name" || { echo 'Existing user: connect as that user instead of granting new privileges'; exit 1; }
else
 useradd --create-home --shell /bin/bash "$name"
 touch "/var/lib/fleetdeck/administrators/$name"
fi
test "$(id -u "$name")" -ne 0
install -d -m 0700 -o "$name" -g "$name" "/home/$name/.ssh"
printf '%s\n' ` + quoteShell(canonical) + ` > "/home/$name/.ssh/authorized_keys"
chown "$name:$name" "/home/$name/.ssh/authorized_keys"
chmod 0600 "/home/$name/.ssh/authorized_keys"
umask 077
printf '%s ALL=(ALL:ALL) NOPASSWD: ALL\n' "$name" > "/etc/sudoers.d/fleetdeck-$name.pending"
visudo -cf "/etc/sudoers.d/fleetdeck-$name.pending"
chmod 0440 "/etc/sudoers.d/fleetdeck-$name.pending"
mv "/etc/sudoers.d/fleetdeck-$name.pending" "/etc/sudoers.d/fleetdeck-$name"
`
	if _, err := runner.Run(script); err != nil {
		return fmt.Errorf("preparing trusted administrator: %w", err)
	}
	return nil
}
