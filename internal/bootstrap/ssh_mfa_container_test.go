package bootstrap

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestContainerSSHMFAAndRecovery(t *testing.T) {
	if os.Getenv("FLEETDECK_SSH_MFA_CONTAINER_TEST") != "1" {
		t.Skip("opt-in disposable Linux SSH/PAM test")
	}
	name := fmt.Sprintf("fleetdeck-mfa-test-%d", time.Now().UnixNano())
	out, err := exec.Command("docker", "run", "-d", "--rm", "--name", name, "--memory=384m", "--cpus=1", "debian:bookworm-slim", "sleep", "600").CombinedOutput()
	if err != nil {
		t.Fatalf("test container: %s %v", out, err)
	}
	defer exec.Command("docker", "rm", "-f", name).Run()
	r := sshContainerRunner{name}
	run := func(command string) string {
		t.Helper()
		out, err := r.Run(command)
		if err != nil {
			t.Fatalf("fixture operation failed: %s %v", out, err)
		}
		return out
	}
	run(`set -eu
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq openssh-server openssh-client libpam-google-authenticator python3 util-linux >/dev/null
mkdir -p /run/sshd
useradd -m -s /bin/bash testoperator
passwd -d testoperator >/dev/null
install -d -m 0700 -o testoperator -g testoperator /home/testoperator/.ssh
ssh-keygen -q -t ed25519 -N '' -f /run/client-key
cp /run/client-key.pub /home/testoperator/.ssh/authorized_keys
chown testoperator:testoperator /home/testoperator/.ssh/authorized_keys
chmod 0600 /home/testoperator/.ssh/authorized_keys
cat > /etc/ssh/sshd_config <<'CONFIG'
Port 2222
ListenAddress 127.0.0.1
HostKey /etc/ssh/ssh_host_ed25519_key
PidFile /run/sshd.pid
UsePAM yes
PermitRootLogin no
PasswordAuthentication no
KbdInteractiveAuthentication no
PubkeyAuthentication yes
PermitEmptyPasswords no
LoginGraceTime 30
CONFIG
cp /etc/ssh/sshd_config /run/original-sshd
cp /etc/pam.d/sshd /run/original-pam
install -d -m 0700 /etc/fleetdeck/ssh-mfa
python3 - <<'PY'
import base64,os
from pathlib import Path
os.umask(0o077)
secret=base64.b32encode(os.urandom(20)).decode()
Path('/etc/fleetdeck/ssh-mfa/testoperator').write_text(secret+'\n" TOTP_AUTH\n" DISALLOW_REUSE\n" RATE_LIMIT 3 30\n" WINDOW_SIZE 3\n')
PY
/usr/sbin/sshd -E /run/sshd-auth.log
cat > /usr/local/bin/systemctl <<'SH'
#!/bin/sh
case "$1" in
enable|disable|daemon-reload) exit 0;;
reload) kill -HUP "$(cat /run/sshd.pid)";;
is-active) test -e /run/test-mfa-timer;;
stop) if test -e /run/test-mfa-timer; then kill "$(cat /run/test-mfa-timer)" 2>/dev/null || true; rm -f /run/test-mfa-timer; fi;;
*) exit 1;;
esac
SH
cat > /usr/local/bin/systemd-run <<'SH'
#!/bin/sh
for arg; do script=$arg; done
delay=45
test ! -e /run/timer-delay || delay=$(cat /run/timer-delay)
nohup sh -c 'sleep "$1"; sh "$2"; rm -f /run/test-mfa-timer' sh "$delay" "$script" </dev/null >/dev/null 2>&1 9>&- &
echo "$!" > /run/test-mfa-timer
SH
cat > /usr/local/bin/journalctl <<'PY'
#!/usr/bin/python3
import json,re
from pathlib import Path
for line in Path('/run/sshd-auth.log').read_text().splitlines():
    match=re.search(r'Accepted keyboard-interactive/pam for .*',line)
    if match: print(json.dumps({'_UID':'0','MESSAGE':match[0]}))
PY
cat > /run/askpass <<'SH'
#!/bin/sh
cat /run/askpass-value
SH
chmod 0755 /usr/local/bin/systemctl /usr/local/bin/systemd-run /usr/local/bin/journalctl /run/askpass
`)
	keyOnly := `ssh -p 2222 -i /run/client-key -o BatchMode=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null testoperator@127.0.0.1 'id -un'`
	run(keyOnly)
	if err := ApplySSHMFA(r, "testoperator"); err != nil {
		t.Fatal(err)
	}
	run("/usr/sbin/sshd -T | grep -qx 'logingracetime 120'")
	if _, err := r.Run(keyOnly); err == nil {
		t.Fatal("key alone bypassed MFA")
	}
	if err := ConfirmSSHMFA(r); err == nil {
		t.Fatal("confirmed without fresh second-factor login")
	}
	run(`python3 - <<'PY'
import base64,hashlib,hmac,struct,time
from pathlib import Path
key=base64.b32decode(Path('/etc/fleetdeck/ssh-mfa/testoperator').read_text().splitlines()[0])
digest=hmac.new(key,struct.pack('>Q',int(time.time())//30),hashlib.sha1).digest()
offset=digest[-1]&15
code=(struct.unpack('>I',digest[offset:offset+4])[0]&0x7fffffff)%1000000
Path('/run/askpass-value').write_text(f'{code:06d}\n')
PY`)
	login := `DISPLAY=fixture SSH_ASKPASS=/run/askpass SSH_ASKPASS_REQUIRE=force setsid -w ssh -p 2222 -i /run/client-key -o NumberOfPasswordPrompts=1 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null testoperator@127.0.0.1 'id -un' </dev/null`
	if out := run(login); !strings.Contains(out, "testoperator") {
		t.Fatal("key plus code did not reach the account")
	}
	if _, err := r.Run(login); err == nil {
		t.Fatal("reused TOTP accepted")
	}
	if err := ConfirmSSHMFA(r); err != nil {
		t.Fatal(err)
	}
	run("test ! -e /run/test-mfa-timer; test ! -e /var/lib/fleetdeck-ssh-mfa-trial")
	run(`cp /run/original-sshd /etc/ssh/sshd_config; cp /run/original-pam /etc/pam.d/sshd; systemctl reload ssh.service; mv /etc/fleetdeck/ssh-mfa/testoperator /run/enrolled-secret`)
	if err := ApplySSHMFA(r, "testoperator"); err == nil {
		t.Fatal("applied without enrollment")
	}
	run(keyOnly)
	run(`mv /run/enrolled-secret /etc/fleetdeck/ssh-mfa/testoperator; echo 2 > /run/timer-delay`)
	if err := ApplySSHMFA(r, "testoperator"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := r.Run("test ! -e /var/lib/fleetdeck-ssh-mfa-trial"); err == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	run("cmp /run/original-sshd /etc/ssh/sshd_config; cmp /run/original-pam /etc/pam.d/sshd")
	run(keyOnly)
	// Simulate boot: the transient timer and old daemon are gone, while the
	// persistent trial and boot recovery command remain available.
	run("echo 45 > /run/timer-delay")
	if err := ApplySSHMFA(r, "testoperator"); err != nil {
		t.Fatal(err)
	}
	run(`test -f /etc/systemd/system/fleetdeck-ssh-mfa-recovery.service
systemctl stop fleetdeck-ssh-mfa-rollback.timer
kill "$(cat /run/sshd.pid)"
sh /var/lib/fleetdeck-ssh-mfa-trial/rollback.sh
cmp /run/original-sshd /etc/ssh/sshd_config
cmp /run/original-pam /etc/pam.d/sshd
test ! -e /etc/systemd/system/fleetdeck-ssh-mfa-recovery.service
/usr/sbin/sshd -E /run/sshd-auth.log`)
	run(keyOnly)
}
