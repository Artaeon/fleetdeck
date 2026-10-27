package bootstrap

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type sshContainerRunner struct{ name string }

func (r sshContainerRunner) Run(command string) (string, error) {
	out, err := exec.Command("docker", "exec", r.name, "sh", "-c", command).CombinedOutput()
	return string(out), err
}

// This opt-in test changes SSH only inside an ephemeral container. The service
// shim accelerates the rollback timer; real sshd validates policy and logins.
func TestContainerSSHPolicyAndRollback(t *testing.T) {
	if os.Getenv("FLEETDECK_SSH_CONTAINER_TEST") != "1" {
		t.Skip("set FLEETDECK_SSH_CONTAINER_TEST=1 on a Docker test host")
	}
	name := fmt.Sprintf("fleetdeck-ssh-test-%d", time.Now().UnixNano())
	out, err := exec.Command("docker", "run", "-d", "--rm", "--name", name, "--memory=384m", "--cpus=1", "debian:bookworm-slim", "sleep", "600").CombinedOutput()
	if err != nil {
		t.Fatalf("test container: %s: %v", out, err)
	}
	defer exec.Command("docker", "rm", "-f", name).Run()
	r := sshContainerRunner{name}
	run := func(command string) string {
		t.Helper()
		out, err := r.Run(command)
		if err != nil {
			t.Fatalf("fixture command failed: %s: %v", out, err)
		}
		return out
	}
	run(`set -eu
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq openssh-server openssh-client sudo util-linux >/dev/null
mkdir -p /run/sshd /root/.ssh
ssh-keygen -q -t ed25519 -N '' -f /run/client-key
cp /run/client-key.pub /root/.ssh/authorized_keys
chmod 700 /root/.ssh
chmod 600 /root/.ssh/authorized_keys
cat > /etc/ssh/sshd_config <<'CONFIG'
Port 2222
ListenAddress 127.0.0.1
HostKey /etc/ssh/ssh_host_ed25519_key
PidFile /run/sshd.pid
PermitRootLogin yes
PasswordAuthentication yes
UsePAM yes
CONFIG
cp /etc/ssh/sshd_config /run/original-sshd
/usr/sbin/sshd
cat > /usr/local/bin/systemctl <<'SH'
#!/bin/sh
case "$1" in
 reload) kill -HUP "$(cat /run/sshd.pid)";;
 is-active) test -e /run/test-timer;;
 stop) if test -e /run/test-timer; then kill "$(cat /run/test-timer)" 2>/dev/null || true; rm -f /run/test-timer; fi;;
 *) exit 1;;
esac
SH
cat > /usr/local/bin/systemd-run <<'SH'
#!/bin/sh
for arg; do script=$arg; done
nohup sh -c 'sleep 8; sh "$1"; rm -f /run/test-timer' sh "$script" </dev/null >/dev/null 2>&1 9>&- &
echo "$!" > /run/test-timer
SH
chmod 755 /usr/local/bin/systemctl /usr/local/bin/systemd-run
`)
	login := func(user string) (string, error) {
		return r.Run("ssh -p 2222 -i /run/client-key -o BatchMode=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null " + user + "@127.0.0.1 'id -u'")
	}
	if _, err := login("root"); err != nil {
		t.Fatal("fixture root login failed", err)
	}
	publicKey := run("cat /run/client-key.pub")
	if err := PrepareAdmin(r, "fleetdeck-test", []byte(publicKey)); err != nil {
		t.Fatal(err)
	}
	if _, err := login("fleetdeck-test"); err != nil {
		t.Fatal("administrator login before policy failed", err)
	}
	if err := configureSSH(r); err != nil {
		t.Fatal(err)
	}
	if _, err := login("fleetdeck-test"); err != nil {
		t.Fatal("fresh administrator login failed", err)
	}
	if _, err := login("root"); err == nil {
		t.Fatal("root login survived hardening")
	}
	effective := run("/usr/sbin/sshd -T -C user=fleetdeck-test,host=localhost,addr=127.0.0.1")
	for _, policy := range []string{"passwordauthentication no", "kbdinteractiveauthentication no", "allowagentforwarding no"} {
		if !strings.Contains(effective, policy) {
			t.Fatal("missing policy", policy)
		}
	}
	if err := ConfirmSSH(r); err != nil {
		t.Fatal(err)
	}
	run("test ! -e /run/fleetdeck-ssh-trial; cp /run/original-sshd /etc/ssh/sshd_config; systemctl reload ssh.service")
	if err := configureSSH(r); err != nil {
		t.Fatal(err)
	}
	if err := RollbackSSH(r); err != nil {
		t.Fatal(err)
	}
	run("cmp /run/original-sshd /etc/ssh/sshd_config; systemctl stop fleetdeck-ssh-rollback.timer")
	if err := configureSSH(r); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := r.Run("test ! -e /run/fleetdeck-ssh-trial"); err == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	run("cmp /run/original-sshd /etc/ssh/sshd_config; test ! -e /run/fleetdeck-ssh-trial")
	if _, err := login("root"); err != nil {
		t.Fatal("automatic rollback did not restore login", err)
	}
}
