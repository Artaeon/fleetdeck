package bootstrap

import "fmt"

func SSHMFASettings(user string) (string, error) {
	if !adminName.MatchString(user) || user == "root" {
		return "", fmt.Errorf("select a non-root administrator")
	}
	return "\nMatch User " + user + "\n    AuthenticationMethods publickey,keyboard-interactive:pam\n    KbdInteractiveAuthentication yes\n", nil
}

// ApplySSHMFA supports the standard Debian/Ubuntu PAM stack on a dedicated
// key-only host. Enrollment is deliberately a separate human terminal action.
func ApplySSHMFA(runner CommandRunner, user string) error {
	settings, err := SSHMFASettings(user)
	if err != nil {
		return err
	}
	script := `set -eu
exec 9>/run/fleetdeck-ssh.lock
flock -x 9
test ! -e /run/fleetdeck-ssh-trial
test ! -e /var/lib/fleetdeck-ssh-mfa-trial
test ! -e /etc/systemd/system/fleetdeck-ssh-mfa-recovery.service
name=` + quoteShell(user) + `
id "$name" >/dev/null
dpkg-query -W -f='${Status}' libpam-google-authenticator | grep -qx 'install ok installed'
/usr/sbin/sshd -T | grep -qx 'usepam yes'
/usr/sbin/sshd -T | grep -qx 'passwordauthentication no'
/usr/sbin/sshd -T | grep -qx 'kbdinteractiveauthentication no'
/usr/sbin/sshd -T | grep -qx 'permitrootlogin no'
python3 - "$name" <<'CHECK'
import pathlib,re,stat,sys
user=sys.argv[1]
root=pathlib.Path('/etc/fleetdeck/ssh-mfa')
for path in [root,root/user]:
    s=path.lstat()
    if path.is_symlink() or s.st_uid!=0 or s.st_mode&0o077: raise SystemExit('MFA enrollment must be private and root-owned')
if not (root/user).is_file(): raise SystemExit('Missing enrolled authenticator')
seed=(root/user).read_text()
if not re.match(r'^[A-Z2-7]{16,128}\n',seed) or '" TOTP_AUTH' not in seed or '" DISALLOW_REUSE' not in seed or '" RATE_LIMIT 3 30' not in seed:
    raise SystemExit('Enrollment must use TOTP, replay prevention and rate limiting')
paths=[pathlib.Path('/etc/ssh/sshd_config'),*pathlib.Path('/etc/ssh/sshd_config.d').glob('*.conf')]
for path in paths:
    for line in path.read_text().splitlines():
        line=line.strip()
        if not line or line.startswith('#'): continue
        parts=line.split()
        if parts[0].lower()=='match': raise SystemExit('Existing Match policy needs manual integration')
        if parts[0].lower()=='include' and (path.name!='sshd_config' or parts[1:]!=['/etc/ssh/sshd_config.d/*.conf']):
            raise SystemExit('Custom SSH include needs manual integration')
pam=pathlib.Path('/etc/pam.d/sshd').read_text()
auth=[line.strip() for line in pam.splitlines() if re.match(r'\s*(auth\s|@include\s+common-auth)',line)]
if auth!=['@include common-auth']: raise SystemExit('Custom PAM authentication stack needs manual integration')
CHECK
trial=/var/lib/fleetdeck-ssh-mfa-trial
mkdir -m 0700 "$trial"
cp -p /etc/ssh/sshd_config "$trial/sshd.previous"
cp -p /etc/pam.d/sshd "$trial/pam.previous"
printf '%s\n' "$name" > "$trial/user"
date --iso-8601=seconds > "$trial/since"
cat > "$trial/rollback.sh" <<'ROLLBACK'
#!/bin/sh
set -eu
exec 9>/run/fleetdeck-ssh.lock
flock -x 9
trial=/var/lib/fleetdeck-ssh-mfa-trial
if test -f "$trial/sshd.previous"; then
 cp -p "$trial/sshd.previous" /etc/ssh/sshd_config
 cp -p "$trial/pam.previous" /etc/pam.d/sshd
 /usr/sbin/sshd -t
 if systemctl is-active --quiet ssh.service; then systemctl reload ssh.service;
 elif systemctl is-active --quiet sshd.service; then systemctl reload sshd.service; fi
 systemctl disable fleetdeck-ssh-mfa-recovery.service || true
 rm -f /etc/systemd/system/fleetdeck-ssh-mfa-recovery.service
 systemctl daemon-reload
 rm -rf "$trial"
fi
ROLLBACK
chmod 0700 "$trial/rollback.sh"
trap 'flock -u 9; sh "$trial/rollback.sh"; systemctl stop fleetdeck-ssh-mfa-rollback.timer || true' EXIT
cat > /etc/systemd/system/fleetdeck-ssh-mfa-recovery.service <<'BOOT'
[Unit]
Description=Restore an unconfirmed FleetDeck SSH MFA trial after reboot
Before=ssh.service sshd.service
ConditionPathExists=/var/lib/fleetdeck-ssh-mfa-trial/sshd.previous
[Service]
Type=oneshot
ExecStart=/bin/sh /var/lib/fleetdeck-ssh-mfa-trial/rollback.sh
[Install]
WantedBy=multi-user.target
BOOT
systemctl daemon-reload
systemctl enable fleetdeck-ssh-mfa-recovery.service
systemd-run --quiet --collect --unit=fleetdeck-ssh-mfa-rollback --on-active=5m --timer-property=AccuracySec=1s /bin/sh "$trial/rollback.sh"
cp "$trial/sshd.previous" "$trial/candidate"
printf '%s' ` + quoteShell(settings) + ` >> "$trial/candidate"
/usr/sbin/sshd -t -f "$trial/candidate"
/usr/sbin/sshd -T -f "$trial/candidate" -C "user=$name,host=localhost,addr=127.0.0.1" | grep -qx 'authenticationmethods publickey,keyboard-interactive:pam'
/usr/sbin/sshd -T -f "$trial/candidate" -C "user=$name,host=localhost,addr=127.0.0.1" | grep -qx 'kbdinteractiveauthentication yes'
python3 - <<'PAM'
from pathlib import Path
path=Path('/etc/pam.d/sshd')
lines=path.read_text().splitlines()
path.write_text('\n'.join('auth required pam_google_authenticator.so secret=/etc/fleetdeck/ssh-mfa/${USER} user=root' if line.strip()=='@include common-auth' else line for line in lines)+'\n')
PAM
cp "$trial/candidate" /etc/ssh/sshd_config
/usr/sbin/sshd -t
systemctl reload ssh.service || systemctl reload sshd.service
trap - EXIT
`
	if _, err := runner.Run(script); err != nil {
		return fmt.Errorf("SSH MFA trial rejected or rolled back; check enrollment and supported SSH/PAM policy")
	}
	return nil
}

func ConfirmSSHMFA(runner CommandRunner) error {
	_, err := runner.Run(`set -eu
exec 9>/run/fleetdeck-ssh.lock
flock -x 9
trial=/var/lib/fleetdeck-ssh-mfa-trial
test -f "$trial/user"
systemctl is-active --quiet fleetdeck-ssh-mfa-rollback.timer
name=$(cat "$trial/user")
/usr/sbin/sshd -T -C "user=$name,host=localhost,addr=127.0.0.1" | grep -qx 'authenticationmethods publickey,keyboard-interactive:pam'
python3 - <<'VERIFY'
import json,subprocess
from pathlib import Path
root=Path('/var/lib/fleetdeck-ssh-mfa-trial')
user=(root/'user').read_text().strip()
result=subprocess.run(['journalctl','--no-pager','--output=json','--lines=50','--grep=^Accepted keyboard-interactive/pam for '+user+' from ','--since', (root/'since').read_text().strip(),'_UID=0','SYSLOG_IDENTIFIER=sshd','SYSLOG_IDENTIFIER=sshd-session'],capture_output=True,check=True,timeout=15)
accepted=False
for line in result.stdout.splitlines():
    event=json.loads(line)
    if event.get('_UID')=='0' and event.get('MESSAGE','').startswith('Accepted keyboard-interactive/pam for '+user+' from '): accepted=True
if not accepted: raise SystemExit('A fresh successful SSH MFA login is required before confirmation')
VERIFY
systemctl stop fleetdeck-ssh-mfa-rollback.timer
systemctl disable fleetdeck-ssh-mfa-recovery.service
rm -f /etc/systemd/system/fleetdeck-ssh-mfa-recovery.service
systemctl daemon-reload
rm -rf "$trial"
`)
	if err != nil {
		return fmt.Errorf("MFA not confirmed: a fresh successful key-plus-code login and active rollback timer are required")
	}
	return nil
}

func RollbackSSHMFA(runner CommandRunner) error {
	_, err := runner.Run(`if test -f /var/lib/fleetdeck-ssh-mfa-trial/rollback.sh; then sh /var/lib/fleetdeck-ssh-mfa-trial/rollback.sh; systemctl stop fleetdeck-ssh-mfa-rollback.timer; fi`)
	return err
}
